package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/alerts"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// fleetOwn is what the APs together call their own (0105): every BSSID an
// AP's latest report names as its own, and every SSID of Aeolus's networks,
// as an AP hears it (alerts.Heard).
func (s *Server) fleetOwn(state *change.State) *alerts.Fleet {
	f := &alerts.Fleet{BSSIDs: map[string]bool{}, SSIDs: map[string]bool{}}
	for b := range s.fleetBSSIDs(state) {
		f.BSSIDs[b] = true
	}
	state.Org.Services.EachSet(func(_ hierarchy.NodeID, p hierarchy.Path, v hierarchy.Value) {
		if ssid, ok := v.(string); ok && strings.HasPrefix(string(p), "network.") && strings.HasSuffix(string(p), ".ssid") {
			f.SSIDs[alerts.Heard(ssid)] = true
		}
	})
	return f
}

// fleetBSSIDs is every BSSID an AP's latest report names as its own, and
// the AP's.
func (s *Server) fleetBSSIDs(state *change.State) map[string]hierarchy.NodeID {
	out := map[string]hierarchy.NodeID{}
	for _, id := range state.Org.Locations.APs() {
		st, err := s.conds.LatestState(id)
		if err != nil || st == nil {
			continue
		}
		var r struct {
			BSSIDs []string `json:"bssids"`
		}
		if json.Unmarshal(st.Report, &r) == nil {
			for _, b := range r.BSSIDs {
				out[b] = id
			}
		}
	}
	return out
}

// knownRogues is the BSSIDs an AP's folders say are no rogues, by
// rogues.known (0106), in lower case as the APs report them.
func knownRogues(state *change.State, id hierarchy.NodeID) map[string]bool {
	out := map[string]bool{}
	cfg, err := state.Org.ResolveAP(id)
	if err != nil {
		return out
	}
	r, ok := cfg.Location["rogues.known"]
	if !ok {
		return out
	}
	switch v := r.Value.(type) {
	case []any:
		for _, b := range v {
			if s, ok := b.(string); ok {
				out[strings.ToLower(s)] = true
			}
		}
	case []string:
		for _, b := range v {
			out[strings.ToLower(b)] = true
		}
	}
	return out
}

// alertsOf is what needs attention on one AP now (0099).
func (s *Server) alertsOf(state *change.State, id hierarchy.NodeID, now time.Time, fleet *alerts.Fleet) ([]alerts.Alert, error) {
	n, _ := state.Org.Locations.Node(id)
	version, _ := s.log.Version(id)
	res, err := s.compose(state, id, s.reveal)
	if err != nil {
		return nil, err
	}
	l, err := s.conds.Latest(id)
	if err != nil {
		return nil, err
	}
	poll := time.Minute
	if sys, _ := res.Doc["system"].(map[string]any); sys != nil {
		if p, ok := sys["poll"].(float64); ok && p > 0 {
			poll = time.Duration(p) * time.Second
		}
	}
	// The agent bundle it should run, as the fleet view has it (0079).
	wants := ""
	if s.agents != nil {
		if b, _, ok := s.agentFor(state, id); ok {
			wants = b.Hash
		}
	}
	return alerts.For(alerts.Input{
		AP: id, Name: n.Name, Now: now, Poll: poll, Unassigned: res.Unassigned,
		Version: version, Since: s.log.VersionSince(id), WantsAgent: wants, Problems: res.Problems, Latest: l, Fleet: fleet,
		Known: knownRogues(state, id),
	}), nil
}

// alertsList is what needs attention on the APs the caller may view (0099),
// below one Locations node where ?under= names it: GET /v1/alerts. It is
// worked out on each call from what the manager has, so an alert ends when
// its cause does.
func (s *Server) alertsList(w http.ResponseWriter, r *http.Request, c call) error {
	t := c.state.Org.Locations
	under := hierarchy.NodeID(r.URL.Query().Get("under"))
	now := time.Now()
	fleet := s.fleetOwn(c.state)
	out := []alerts.Alert{}
	for _, id := range t.APs() {
		if roleOn(c, change.Locations, t, id) < access.Viewer {
			continue
		}
		if under != "" && under != id && !slices.Contains(t.Ancestry(id), under) {
			continue
		}
		as, err := s.alertsOf(c.state, id, now, fleet)
		if err != nil {
			return err
		}
		out = append(out, as...)
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

// notifyTarget is where an AP's alerts go, as the notify.* fields of its
// folders say, nearest first (0101), the sealed URLs opened.
func (s *Server) notifyTarget(state *change.State, id hierarchy.NodeID) alerts.Target {
	cfg, err := state.Org.ResolveAP(id)
	if err != nil {
		return alerts.Target{}
	}
	// The URLs are sealed (0027): opened here, to send, and nowhere else.
	str := func(path string) string {
		r, ok := cfg.Location[hierarchy.Path(path)]
		if !ok {
			return ""
		}
		v, err := s.reveal(path, r.Value)
		if err != nil {
			slog.Warn("alerts: a notify setting cannot be opened", "path", path, "err", err)
			return ""
		}
		out, _ := v.(string)
		return out
	}
	t := alerts.Target{Ntfy: str("notify.ntfy"), Webhook: str("notify.webhook"), Resolved: true}
	if r, ok := cfg.Location["notify.severity"]; ok {
		t.Least, _ = r.Value.(string)
	}
	if r, ok := cfg.Location["notify.resolved"]; ok {
		if b, ok := r.Value.(bool); ok {
			t.Resolved = b
		}
	}
	return t
}

// Notify sends alerts where each AP's folders say (0101), looking every
// interval until ctx ends. A send that fails is logged and not tried again.
func (s *Server) Notify(ctx context.Context, every time.Duration) {
	n := alerts.NewNotifier()
	client := &http.Client{Timeout: 10 * time.Second}
	look := func() {
		now := time.Now()
		state := s.log.Snapshot()
		var all []alerts.Alert
		targets := map[hierarchy.NodeID]alerts.Target{}
		fleet := s.fleetOwn(state)
		for _, id := range state.Org.Locations.APs() {
			as, err := s.alertsOf(state, id, now, fleet)
			if err != nil {
				slog.Warn("alerts: cannot work out an AP's", "ap", id, "err", err)
				continue
			}
			all = append(all, as...)
			targets[id] = s.notifyTarget(state, id)
		}
		for _, e := range n.Step(now, all, func(a alerts.Alert) alerts.Target { return targets[a.AP] }) {
			if err := alerts.Send(ctx, client, e); err != nil {
				slog.Warn("alerts: not sent", "ap", e.Alert.Name, "event", e.Kind, "key", e.Alert.Key, "err", err)
			} else {
				slog.Info("alerts: sent", "ap", e.Alert.Name, "event", e.Kind, "key", e.Alert.Key)
			}
		}
	}
	look()
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			look()
		}
	}
}
