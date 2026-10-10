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
// With no previous report, only the clients that joined in the last report
// interval count.
func usageOf(prev *conditions.State, now time.Time, clients []wifiClient) (down, up int64) {
	type counters struct{ rx, tx, connected int64 }
	before := map[string]counters{}
	since := float64(stateEvery / time.Second)
	if prev != nil {
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
	for _, c := range clients {
		b, ok := before[c.MAC]
		switch {
		case ok && c.Connected >= b.connected && c.RxBytes >= b.rx && c.TxBytes >= b.tx:
			down += c.TxBytes - b.tx
			up += c.RxBytes - b.rx
		case (!ok || c.Connected < b.connected) && float64(c.Connected) <= since+60:
			down += c.TxBytes
			up += c.RxBytes
		}
	}
	return down, up
}

// stateEvery is how often an agent reports its state (0040).
const stateEvery = 5 * time.Minute

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
	AP   hierarchy.NodeID `json:"ap"`
	Name string           `json:"name"`
	Peak int              `json:"peak"`
	Down int64            `json:"down"`
	Up   int64            `json:"up"`
}

// usageView is the Wi-Fi clients and traffic of the APs the caller may
// view (0108), below one Locations node where ?under= names it, over the
// last ?hours= (24 unless set, at most 720): GET /v1/usage. It is in at
// most 48 buckets of whole minutes, each the clients the APs had at most in
// it, summed over the APs, and the bytes they moved; with each AP's peak and
// totals, the busiest first.
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
		mine[id] = &usageAP{AP: id, Name: n.Name}
	}
	span := time.Duration(hours) * time.Hour
	bucket := max(stateEvery, (span / 48).Truncate(time.Minute))
	end := time.Now().UTC().Truncate(bucket).Add(bucket)
	count := int((span + bucket - 1) / bucket)
	start := end.Add(-time.Duration(count) * bucket)
	buckets := make([]usageBucket, count)
	for i := range buckets {
		buckets[i].At = start.Add(time.Duration(i) * bucket)
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
		"down": down, "up": up, "peak": peak, "aps": aps,
	})
	return nil
}
