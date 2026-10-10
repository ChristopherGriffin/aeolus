package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// usageOf is what an AP's clients moved since its previous report (0108),
// down being what the AP sent them and up what it received: for a client
// in both reports, how much its counters grew; for one that joined since,
// all it moved. A client whose counters went back without its joining
// again, as a 32-bit counter does when it wraps, counts nothing this time.
// With no previous report, or one older than usageGap, only the clients
// that joined in the last report interval count: what the others moved in
// the gap would all fall in this report's bucket, a spike that never was.
func usageOf(prev *conditions.State, now time.Time, clients []wifiClient) (down, up int64) {
	for _, c := range clientsMoved(prev, now, clients) {
		down += c.Down
		up += c.Up
	}
	return down, up
}

// clientsMoved is what each client moved, as usageOf counts it, each that
// moved anything once (0110).
func clientsMoved(prev *conditions.State, now time.Time, clients []wifiClient) []conditions.ClientUse {
	type counters struct{ rx, tx, connected int64 }
	before := map[string]counters{}
	since := float64(stateEvery / time.Second)
	if prev != nil && now.Sub(prev.At) <= usageGap {
		since = now.Sub(prev.At).Seconds()
		var r struct {
			Clients []wifiClient `json:"clients"`
		}
		if json.Unmarshal(prev.Report, &r) == nil {
			for _, c := range r.Clients {
				before[c.MAC] = counters{c.RxBytes, c.TxBytes, c.Connected}
			}
		}
	}
	var out []conditions.ClientUse
	for _, c := range clients {
		b, ok := before[c.MAC]
		var down, up int64
		switch {
		case ok && c.Connected >= b.connected && c.RxBytes >= b.rx && c.TxBytes >= b.tx:
			down, up = c.TxBytes-b.tx, c.RxBytes-b.rx
		case (!ok || c.Connected < b.connected) && float64(c.Connected) <= since+60:
			down, up = c.TxBytes, c.RxBytes
		}
		if down+up > 0 {
			out = append(out, conditions.ClientUse{MAC: c.MAC, Host: c.Host, Down: down, Up: up})
		}
	}
	return out
}

// usageClient is one client's part of an AP's usage (0110): what it moved
// over the span, on the APs the caller may view, and which those were.
type usageClient struct {
	MAC  string   `json:"mac"`
	Host string   `json:"host,omitempty"`
	Down int64    `json:"down"`
	Up   int64    `json:"up"`
	APs  []string `json:"aps"`
}

// topClients is the clients that moved the most over the span on the APs
// in mine, at most 10, the most first.
func (s *Server) topClients(since time.Time, mine map[hierarchy.NodeID]*usageAP) ([]usageClient, error) {
	totals, err := s.conds.ClientTotals(since)
	if err != nil {
		return nil, err
	}
	by := map[string]*usageClient{}
	named := map[string]time.Time{} // when each client's host name was given
	for _, t := range totals {
		a := mine[t.AP]
		if a == nil {
			continue
		}
		c := by[t.MAC]
		if c == nil {
			c = &usageClient{MAC: t.MAC, APs: []string{}}
			by[t.MAC] = c
		}
		// The name it gave last, on whichever AP.
		if t.Host != "" && !t.HostAt.Before(named[t.MAC]) {
			c.Host, named[t.MAC] = t.Host, t.HostAt
		}
		c.Down += t.Down
		c.Up += t.Up
		c.APs = append(c.APs, a.Name)
	}
	out := make([]usageClient, 0, len(by))
	for _, c := range by {
		sort.Strings(c.APs)
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if x, y := out[i].Down+out[i].Up, out[j].Down+out[j].Up; x != y {
			return x > y
		}
		return out[i].MAC < out[j].MAC
	})
	return out[:min(len(out), 10)], nil
}

// stateEvery is how often an agent reports its state (0040); usageGap is
// how long since its last report a report's counters still count from it:
// three intervals, as an alert takes a report to be too old (0099).
const (
	stateEvery = 5 * time.Minute
	usageGap   = 3 * stateEvery
)

// usageBucket is one stretch of a usage chart: the clients the APs had at
// most in it, summed over the APs, and what they moved in it.
type usageBucket struct {
	At      time.Time `json:"at"`
	Clients int       `json:"clients"`
	Down    int64     `json:"down"`
	Up      int64     `json:"up"`
}

// usageAP is one AP's part of it.
type usageAP struct {
	AP    hierarchy.NodeID `json:"ap"`
	Name  string           `json:"name"`
	Peak  int              `json:"peak"`
	Down  int64            `json:"down"`
	Up    int64            `json:"up"`
	Heard []bool           `json:"heard"` // whether it reported in each bucket
}

// usageView is the Wi-Fi clients and traffic of the APs the caller may
// view (0108), below one Locations node where ?under= names it, over the
// last ?hours= (24 unless set, at most 720): GET /v1/usage. It is in at
// most 48 buckets of whole minutes, each the clients the APs had at most in
// it, summed over the APs, and the bytes they moved; with each AP's peak and
// totals, the busiest first, and whether it reported in each bucket, its
// connectivity; and the ten clients that moved the most (0110).
func (s *Server) usageView(w http.ResponseWriter, r *http.Request, c call) error {
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
	mine := map[hierarchy.NodeID]*usageAP{}
	for _, id := range t.APs() {
		if roleOn(c, change.Locations, t, id) < access.Viewer {
			continue
		}
		if under != "" && under != id && !slices.Contains(t.Ancestry(id), under) {
			continue
		}
		n, _ := t.Node(id)
		mine[id] = &usageAP{AP: id, Name: n.Name, Heard: []bool{}}
	}
	span := time.Duration(hours) * time.Hour
	// Whole minutes, rounded up, so there are never more than 48.
	bucket := max(stateEvery, ((span+47)/48 + time.Minute - 1).Truncate(time.Minute))
	end := time.Now().UTC().Truncate(bucket).Add(bucket)
	count := int((span + bucket - 1) / bucket)
	start := end.Add(-time.Duration(count) * bucket)
	buckets := make([]usageBucket, count)
	for i := range buckets {
		buckets[i].At = start.Add(time.Duration(i) * bucket)
	}
	for _, a := range mine {
		a.Heard = make([]bool, count)
	}
	// The most clients each AP had in each bucket.
	peaks := map[hierarchy.NodeID][]int{}
	err := s.conds.Uses(start, func(u conditions.Use) error {
		a := mine[u.AP]
		i := int(u.At.Sub(start) / bucket)
		if a == nil || i < 0 || i >= count {
			return nil
		}
		p := peaks[u.AP]
		if p == nil {
			p = make([]int, count)
			peaks[u.AP] = p
		}
		p[i] = max(p[i], u.Clients)
		a.Heard[i] = true
		a.Peak = max(a.Peak, u.Clients)
		a.Down += u.Down
		a.Up += u.Up
		buckets[i].Down += u.Down
		buckets[i].Up += u.Up
		return nil
	})
	if err != nil {
		return err
	}
	var down, up int64
	peak := 0
	for i := range buckets {
		for _, p := range peaks {
			buckets[i].Clients += p[i]
		}
		peak = max(peak, buckets[i].Clients)
		down += buckets[i].Down
		up += buckets[i].Up
	}
	top, err := s.topClients(start, mine)
	if err != nil {
		return err
	}
	// Before the oldest row kept, a bucket says nothing of an AP.
	var from any
	if t, ok, err := s.conds.UsageFrom(); err != nil {
		return err
	} else if ok {
		from = t
	}
	aps := make([]*usageAP, 0, len(mine))
	for _, a := range mine {
		aps = append(aps, a)
	}
	sort.Slice(aps, func(i, j int) bool {
		if x, y := aps[i].Down+aps[i].Up, aps[j].Down+aps[j].Up; x != y {
			return x > y
		}
		return aps[i].Name < aps[j].Name
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"from": start, "to": end, "bucket": int(bucket / time.Second), "buckets": buckets,
		"down": down, "up": up, "peak": peak, "aps": aps, "clients": top, "recorded_from": from,
	})
	return nil
}
