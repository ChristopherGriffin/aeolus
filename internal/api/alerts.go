package api

import (
	"net/http"
	"slices"
	"sort"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/alerts"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// alertsList is what needs attention on the APs the caller may view (0099),
// below one Locations node where ?under= names it: GET /v1/alerts. It is
// worked out on each call from what the manager has, so an alert ends when
// its cause does.
func (s *Server) alertsList(w http.ResponseWriter, r *http.Request, c call) error {
	t := c.state.Org.Locations
	under := hierarchy.NodeID(r.URL.Query().Get("under"))
	now := time.Now()
	out := []alerts.Alert{}
	for _, id := range t.APs() {
		if roleOn(c, change.Locations, t, id) < access.Viewer {
			continue
		}
		if under != "" && under != id && !slices.Contains(t.Ancestry(id), under) {
			continue
		}
		n, _ := t.Node(id)
		version, _ := s.log.Version(id)
		res, err := s.compose(c.state, id, s.reveal)
		if err != nil {
			return err
		}
		l, err := s.conds.Latest(id)
		if err != nil {
			return err
		}
		poll := time.Minute
		if sys, _ := res.Doc["system"].(map[string]any); sys != nil {
			if p, ok := sys["poll"].(float64); ok && p > 0 {
				poll = time.Duration(p) * time.Second
			}
		}
		out = append(out, alerts.For(alerts.Input{
			AP: id, Name: n.Name, Now: now, Poll: poll, Unassigned: res.Unassigned,
			Version: version, Problems: res.Problems, Latest: l,
		})...)
	}
	rank := map[string]int{alerts.Critical: 0, alerts.Warning: 1, alerts.Info: 2}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := rank[out[i].Severity], rank[out[j].Severity]; a != b {
			return a < b
		}
		return out[i].Name < out[j].Name
	})
	counts := map[string]int{alerts.Critical: 0, alerts.Warning: 0, alerts.Info: 0}
	for _, a := range out {
		counts[a.Severity]++
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": out, "counts": counts})
	return nil
}
