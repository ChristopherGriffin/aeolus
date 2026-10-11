package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Roaming (0136): a client that is on a network and moves to another AP of
// it, or another radio of the same AP. The AP it comes to records the
// attempt as it does any (0118), and with it how the client came: as one
// already on the network (a reassociation), by which authentication, with a
// key from before or none, and how long it took to be let on. The manager
// reads those back as roams: how many, of which kind, how long they
// typically took, and those that failed or were slow.

// The kinds of roam, by how much of coming online the client did again, the
// least first: a fast transition (802.11r), which brings its keys; a key
// made from its sign-in at another AP (OKC), or one this AP held from its
// last time here (PMKSA cache), or one of the two where it is not known
// which, each with the four key messages and no sign-in; a passphrase, with
// the four messages, as a network with one has nothing more to do; and
// everything again, the sign-in or WPA3's exchange.
const (
	roamFT     = "ft"
	roamOKC    = "okc"
	roamPMKSA  = "pmksa"
	roamCached = "cached"
	roamKeys   = "keys"
	roamFull   = "full"
)

var roamKinds = []string{roamFT, roamOKC, roamPMKSA, roamCached, roamKeys, roamFull}

const (
	// roamSlowMS is how long a roam may take at the AP it comes to, from
	// the client's first frame there to its being let on, before it is
	// called slow: a call in progress hears a gap of a quarter of a second.
	roamSlowMS = 250
	// roamingMost is how many attempts are read for the roams of a time.
	roamingMost = 5000
	// roamingLatest is how many roams are listed, the latest; and as many
	// of those that failed or were slow.
	roamingLatest = 50
)

// connHow is how a client came, as its AP's record of the attempt has it
// (0136): by a reassociation; its authentication, as hostapd names it (ft
// for a fast transition, sae for WPA3, open); a key from before, from this
// AP's own cache (pmksa) or made from a sign-in at another AP (okc); how
// often the AP asked the client it still held to prove itself (SA Query);
// and how long from its first frame to its being let on.
type connHow struct {
	Re   bool   `json:"re,omitempty"`
	Alg  string `json:"alg,omitempty"`
	PMK  string `json:"pmk,omitempty"`
	SAQ  int    `json:"saq,omitempty"`
	OnMS *int   `json:"on_ms,omitempty"`
}

var howAlgRE = regexp.MustCompile(`^[a-z0-9]{1,12}$`)

func (h *connHow) ok() bool {
	return (h.Alg == "" || howAlgRE.MatchString(h.Alg)) && (h.PMK == "" || h.PMK == "pmksa" || h.PMK == "okc") &&
		h.SAQ >= 0 && h.SAQ <= 64 && (h.OnMS == nil || (*h.OnMS >= 0 && *h.OnMS <= 600000))
}

// roam is one attempt that was a roam: who, when, to which AP and from
// where, where that is known, of which kind, how long it took to be let
// on, and how it went: ok, slow (let on, after more than roamSlowMS),
// failed (not let on), or left (gone before it was let on, with nothing
// refused, as a client that thought better of it).
type roam struct {
	ID        int64            `json:"id"`
	At        time.Time        `json:"at"`
	MAC       string           `json:"mac"`
	Host      string           `json:"host,omitempty"`
	AP        hierarchy.NodeID `json:"ap"`
	APName    string           `json:"ap_name"`
	Network   string           `json:"network"`
	SSID      string           `json:"ssid"`
	Band      string           `json:"band"`
	FromAP    hierarchy.NodeID `json:"from_ap,omitempty"`
	FromName  string           `json:"from_ap_name,omitempty"`
	FromBand  string           `json:"from_band,omitempty"`
	Kind      string           `json:"kind"`
	MS        *int             `json:"ms,omitempty"`
	State     string           `json:"state"`
	Reason    string           `json:"reason,omitempty"`
	SAQueries int              `json:"sa_queries,omitempty"`
}

// roamRecord is what a roam is read from in an attempt's record.
type roamRecord struct {
	BSS    string     `json:"bss"`
	Host   string     `json:"host"`
	How    *connHow   `json:"how"`
	Events []connStep `json:"events"`
}

// roamKind is the kind of roam an attempt was, and whether it was one at
// all: it came as a client already on the network, or by a fast transition,
// which only such a client makes. signs says its network signs clients in
// by 802.1X, where a client that did not sign in again came with a key from
// before, though hostapd, logging less there (0133), does not say which.
func roamKind(rec roamRecord, signs bool) (string, bool) {
	h := rec.How
	if h == nil || !(h.Re || h.Alg == "ft") {
		return "", false
	}
	signedIn := slices.ContainsFunc(rec.Events, func(e connStep) bool { return e.Stage == "signin" })
	switch {
	case h.Alg == "ft":
		return roamFT, true
	case h.PMK == "okc":
		return roamOKC, true
	case h.PMK == "pmksa":
		return roamPMKSA, true
	case signedIn || h.Alg == "sae":
		return roamFull, true
	case signs:
		return roamCached, true
	}
	return roamKeys, true
}

// roamKindsView is the roams of one kind: how many, how many of them
// failed and how many were slow, and of those let on the middle, the least
// and the most of their times in ms.
type roamKindsView struct {
	Kind   string `json:"kind"`
	Roams  int    `json:"roams"`
	Failed int    `json:"failed"`
	Slow   int    `json:"slow"`
	MS     *int   `json:"typical_ms,omitempty"`
	Low    *int   `json:"low_ms,omitempty"`
	High   *int   `json:"high_ms,omitempty"`
}

// roamingView is the roams of the clients on the APs given since a time:
// in all and a kind at a time, the latest, and those that went badly. The
// tests' own clients are left out (0121): what an AP's scan radio does is
// no client's roaming.
type roamingView struct {
	Hours   int             `json:"hours"`
	SlowMS  int             `json:"slow_over_ms"`
	Roams   int             `json:"roams"`
	Failed  int             `json:"failed"`
	Slow    int             `json:"slow"`
	Left    int             `json:"left"`
	Clients int             `json:"clients"`
	MS      *int            `json:"typical_ms,omitempty"`
	Kinds   []roamKindsView `json:"kinds"`
	Latest  []roam          `json:"latest"`
	Bad     []roam          `json:"bad"`
	// More is set where there were more attempts in the time than are
	// read: the counts are then of the latest of them.
	More bool `json:"more,omitempty"`
}

func middle(v []int) (mid, low, high *int) {
	if len(v) == 0 {
		return nil, nil, nil
	}
	sort.Ints(v)
	m, l, h := v[len(v)/2], v[0], v[len(v)-1]
	return &m, &l, &h
}

// signsIn says whether a network on an AP signs each client in by 802.1X,
// worked out once for each.
func (s *Server) signsIn(state *change.State) func(hierarchy.NodeID, string) bool {
	signs := map[string]bool{}
	return func(ap hierarchy.NodeID, network string) bool {
		k := string(ap) + " " + network
		if v, ok := signs[k]; ok {
			return v
		}
		v := false
		if res, err := s.compose(state, ap, s.reveal); err == nil && !res.Unassigned {
			nets, _ := res.Doc["network"].(map[string]any)
			n, _ := nets[network].(map[string]any)
			sec, _ := n["security"].(string)
			v = enterprise[sec]
		}
		signs[k] = v
		return v
	}
}

// roamSaid is what a client's own attempts say of one that came as a
// client already on the network: of which kind, and how long it took to
// be let on. It may have been on this very radio before: only the roams of
// a time (GET /v1/roaming) know where each came from.
type roamSaid struct {
	Kind string `json:"kind"`
	MS   *int   `json:"ms,omitempty"`
}

// roamsOf reads the roams to the APs shown among attempts, which are the
// latest first, as the store gives them, and are those of every AP: where
// a client came from is where its attempt before was let on, another AP or
// another radio of the same, whoever may see that one; and one that asked
// again on the radio it was on is no roam. Where it came from is said only
// of an AP the caller may view.
func (s *Server) roamsOf(c call, list []conditions.Connection, shown map[hierarchy.NodeID]bool) []roam {
	t := c.state.Org.Locations
	signsIn := s.signsIn(c.state)
	type place struct {
		ap        hierarchy.NodeID
		bss, band string
	}
	last := map[string]place{}
	var out []roam
	for i := len(list) - 1; i >= 0; i-- {
		cn := list[i]
		var rec roamRecord
		if json.Unmarshal(cn.Record, &rec) != nil {
			continue
		}
		on := rec.How != nil && rec.How.OnMS != nil
		here := place{cn.AP, rec.BSS, cn.Band}
		before, known := last[cn.MAC]
		if on {
			last[cn.MAC] = here
		}
		if !shown[cn.AP] {
			continue
		}
		kind, is := roamKind(rec, signsIn(cn.AP, cn.Network))
		if !is || (known && before.ap == here.ap && before.bss == here.bss) {
			continue
		}
		r := roam{ID: cn.ID, At: cn.Started, MAC: cn.MAC, Host: rec.Host, AP: cn.AP, Network: cn.Network, SSID: cn.SSID, Band: cn.Band,
			Kind: kind, SAQueries: rec.How.SAQ}
		if n, ok := t.Node(cn.AP); ok {
			r.APName = n.Name
		}
		if n, ok := t.Node(before.ap); known && ok && roleOn(c, change.Locations, t, before.ap) >= access.Viewer {
			r.FromAP, r.FromName, r.FromBand = before.ap, n.Name, before.band
		}
		switch {
		case on:
			r.MS = rec.How.OnMS
			r.State = "ok"
			if *r.MS > roamSlowMS {
				r.State = "slow"
			}
		case cn.Outcome == "failed":
			r.State, r.Reason = "failed", cn.Reason
		default:
			r.State, r.Reason = "left", cn.Reason
		}
		out = append(out, r)
	}
	slices.Reverse(out)
	return out
}

// roaming is the roams of the clients on the APs the caller may view,
// below one Locations node where ?under= names it, over ?hours= (24 unless
// set, at most 720): GET /v1/roaming.
func (s *Server) roaming(w http.ResponseWriter, r *http.Request, c call) error {
	q := r.URL.Query()
	hours := 24
	if v := q.Get("hours"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 720 {
			return badRequest("hours: from 1 to 720")
		}
		hours = n
	}
	shown := map[hierarchy.NodeID]bool{}
	for _, id := range mayView(c, hierarchy.NodeID(q.Get("under"))) {
		shown[id] = true
	}
	var tests []string
	for mac := range testClients(c.state) {
		tests = append(tests, mac)
	}
	// Every AP's, to know where each client was before: what is said is
	// of the APs shown alone.
	var list []conditions.Connection
	if len(shown) > 0 {
		var err error
		list, err = s.conds.ConnectionsSince(time.Now().Add(-time.Duration(hours)*time.Hour), c.state.Org.Locations.APs(), tests, roamingMost+1)
		if err != nil {
			return err
		}
	}
	view := roamingView{Hours: hours, SlowMS: roamSlowMS, Kinds: []roamKindsView{}, Latest: []roam{}, Bad: []roam{}}
	if len(list) > roamingMost {
		list, view.More = list[:roamingMost], true
	}
	roams := s.roamsOf(c, list, shown)
	kinds := map[string]*roamKindsView{}
	times := map[string][]int{}
	var all []int
	who := map[string]bool{}
	for _, m := range roams {
		who[m.MAC] = true
		if m.State == "left" {
			view.Left++
			continue
		}
		k := kinds[m.Kind]
		if k == nil {
			k = &roamKindsView{Kind: m.Kind}
			kinds[m.Kind] = k
		}
		view.Roams++
		k.Roams++
		switch m.State {
		case "failed":
			view.Failed++
			k.Failed++
		case "slow":
			view.Slow++
			k.Slow++
		}
		if m.MS != nil {
			all = append(all, *m.MS)
			times[m.Kind] = append(times[m.Kind], *m.MS)
		}
	}
	view.Clients = len(who)
	view.MS, _, _ = middle(all)
	for _, kind := range roamKinds {
		if k := kinds[kind]; k != nil {
			k.MS, k.Low, k.High = middle(times[kind])
			view.Kinds = append(view.Kinds, *k)
		}
	}
	for _, m := range roams {
		if len(view.Latest) < roamingLatest {
			view.Latest = append(view.Latest, m)
		}
		if m.State != "ok" && len(view.Bad) < roamingLatest {
			view.Bad = append(view.Bad, m)
		}
	}
	writeJSON(w, http.StatusOK, view)
	return nil
}
