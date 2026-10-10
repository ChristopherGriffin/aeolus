package api

import (
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/alerts"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// alertRecorder keeps the alert log (0109) from the alerts at each look:
// an alert that has lasted alerts.Hold is logged as beginning when it was
// first seen, or when it says it began if that is earlier, and its end
// when it is gone. A blip shorter than that is not logged. At its first
// look it takes up the alerts the log has open: those still alerting go on,
// the others end then.
type alertRecorder struct {
	started bool
	first   map[string]time.Time // key -> when first seen, not yet logged
	open    map[string]int64     // key -> its row in the log
}

func newAlertRecorder() *alertRecorder {
	return &alertRecorder{first: map[string]time.Time{}, open: map[string]int64{}}
}

func (s *Server) recordAlerts(r *alertRecorder, now time.Time, current []alerts.Alert) {
	if !r.started {
		open, err := s.conds.OpenAlerts()
		if err != nil {
			slog.Warn("alert log: cannot read what is open", "err", err)
			return
		}
		for _, a := range open {
			r.open[string(a.AP)+"|"+a.Key] = a.ID
		}
		r.started = true
	}
	seen := map[string]bool{}
	for _, a := range current {
		k := string(a.AP) + "|" + a.Key
		seen[k] = true
		if _, ok := r.open[k]; ok {
			continue
		}
		began, ok := r.first[k]
		if !ok {
			began = now
			r.first[k] = now
		}
		if a.Since != nil && a.Since.Before(began) {
			began = *a.Since
		}
		if now.Sub(began) < alerts.Hold {
			continue
		}
		id, err := s.conds.BeginAlert(conditions.LoggedAlert{AP: a.AP, Key: a.Key, Kind: a.Kind, Severity: a.Severity, Message: a.Message, Began: began})
		if err != nil {
			slog.Warn("alert log: cannot log one", "ap", a.AP, "key", a.Key, "err", err)
			continue
		}
		r.open[k] = id
		delete(r.first, k)
	}
	for k := range r.first {
		if !seen[k] {
			delete(r.first, k)
		}
	}
	for k, id := range r.open {
		if seen[k] {
			continue
		}
		if err := s.conds.EndAlert(id, now); err != nil {
			slog.Warn("alert log: cannot end one", "id", id, "err", err)
			continue
		}
		delete(r.open, k)
	}
}

// alertHistory is the alerts that lasted on the APs the caller may view
// (0109), below one Locations node where ?under= names it, at some time in
// the last ?hours= (24 unless set, at most 720), newest first, at most 500:
// GET /v1/alerts/history. Each says when it began and, unless it lasts
// still, when it ended.
func (s *Server) alertHistory(w http.ResponseWriter, r *http.Request, c call) error {
	hours := 24
	if q := r.URL.Query().Get("hours"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 || n > 720 {
			return badRequest("hours: from 1 to 720")
		}
		hours = n
	}
	t := c.state.Org.Locations
	under := hierarchy.NodeID(r.URL.Query().Get("under"))
	logged, err := s.conds.AlertLog(time.Now().Add(-time.Duration(hours)*time.Hour), 2000)
	if err != nil {
		return err
	}
	type entry struct {
		conditions.LoggedAlert
		Name string `json:"name"`
	}
	out := []entry{}
	for _, a := range logged {
		n, ok := t.Node(a.AP)
		if !ok || roleOn(c, change.Locations, t, a.AP) < access.Viewer {
			continue
		}
		if under != "" && under != a.AP && !slices.Contains(t.Ancestry(a.AP), under) {
			continue
		}
		out = append(out, entry{a, n.Name})
		if len(out) == 500 {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": out})
	return nil
}
