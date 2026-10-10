package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// What a network heard is (0106), most pressing first: a rogue takes one of
// Aeolus's SSIDs from a BSSID no AP names as its own (0105); known is one
// marked as no rogue, by rogues.known; aeolus is one of the APs' own, heard
// by an AP it is not a radio neighbour of; other is any other network.
var airspaceKinds = []string{"rogue", "known", "aeolus", "other"}

// hearer is one AP that hears a network: how strongly, and when it last
// did.
type hearer struct {
	AP     hierarchy.NodeID `json:"ap"`
	Name   string           `json:"name"`
	Signal float64          `json:"signal"`
	Seen   time.Time        `json:"seen"`
}

// heardNetwork is a network the APs hear, by BSSID.
type heardNetwork struct {
	BSSID   string           `json:"bssid"`
	SSID    string           `json:"ssid"`
	Band    string           `json:"band"`
	Channel int              `json:"channel"`
	Kind    string           `json:"kind"`
	AP      hierarchy.NodeID `json:"ap,omitempty"` // whose it is, for one of the APs' own the caller may view
	HeardBy []hearer         `json:"heard_by"`
}

// airspaceList is the networks the APs the caller may view hear (0106),
// below one Locations node where ?under= names it: GET /v1/airspace. Each
// is listed once, by BSSID, with the APs that hear it, the strongest first;
// rogues come first, then known ones, the APs' own, and the others, each
// the strongest heard first. A network that is a rogue to one AP that
// hears it is a rogue, though another AP's folders know it.
func (s *Server) airspaceList(w http.ResponseWriter, r *http.Request, c call) error {
	t := c.state.Org.Locations
	under := hierarchy.NodeID(r.URL.Query().Get("under"))
	fleet := s.fleetOwn(c.state)
	owners := s.fleetBSSIDs(c.state)
	by := map[string]*heardNetwork{}
	for _, id := range t.APs() {
		if roleOn(c, change.Locations, t, id) < access.Viewer {
			continue
		}
		if under != "" && under != id && !slices.Contains(t.Ancestry(id), under) {
			continue
		}
		st, err := s.conds.LatestState(id)
		if err != nil {
			return err
		}
		if st == nil {
			continue
		}
		var rep struct {
			RRM *struct {
				Others []rrmOther `json:"others"`
			} `json:"rrm"`
		}
		if json.Unmarshal(st.Report, &rep) != nil || rep.RRM == nil {
			continue
		}
		known := knownRogues(c.state, id)
		n, _ := t.Node(id)
		for _, o := range rep.RRM.Others {
			kind := "other"
			switch {
			case owners[o.BSSID] != "":
				kind = "aeolus"
			case fleet.SSIDs[o.SSID] && known[o.BSSID]:
				kind = "known"
			case fleet.SSIDs[o.SSID]:
				kind = "rogue"
			}
			x := by[o.BSSID]
			if x == nil {
				x = &heardNetwork{BSSID: o.BSSID, SSID: o.SSID, Band: o.Band, Channel: o.Channel, Kind: kind}
				if owner := owners[o.BSSID]; owner != "" && roleOn(c, change.Locations, t, owner) >= access.Viewer {
					x.AP = owner
				}
				by[o.BSSID] = x
			} else if kind == "rogue" {
				x.Kind = kind
			}
			x.HeardBy = append(x.HeardBy, hearer{AP: id, Name: n.Name, Signal: o.Signal, Seen: st.At.Add(-time.Duration(o.Ago) * time.Second)})
		}
	}
	out := make([]*heardNetwork, 0, len(by))
	counts := map[string]int{}
	for _, k := range airspaceKinds {
		counts[k] = 0
	}
	for _, x := range by {
		sort.SliceStable(x.HeardBy, func(i, j int) bool { return x.HeardBy[i].Signal > x.HeardBy[j].Signal })
		counts[x.Kind]++
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := slices.Index(airspaceKinds, out[i].Kind), slices.Index(airspaceKinds, out[j].Kind); a != b {
			return a < b
		}
		if a, b := out[i].HeardBy[0].Signal, out[j].HeardBy[0].Signal; a != b {
			return a > b
		}
		return out[i].BSSID < out[j].BSSID
	})
	writeJSON(w, http.StatusOK, map[string]any{"networks": out, "counts": counts})
	return nil
}
