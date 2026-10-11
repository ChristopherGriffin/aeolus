package api

import (
	"bytes"
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
	// the first line its AP has of it to its being let on, before it is
	// called slow: a call in progress hears a gap of a quarter of a second.
	roamSlowMS = 250
	// roamingLatest is how many roams are listed, the latest; and as many
	// of those that failed or were slow.
	roamingLatest = 50
	// roamingBack is how far before the first roam looked at a client's
	// attempts are read together, to know where each was before; one that
	// was nowhere in that time is looked for by itself, however far back.
	roamingBack = time.Hour
)

// roamingMost is how many attempts are read for the roams of a time, of
// the APs asked about; and roamingPast how many of those clients' attempts
// anywhere, to know where each was before.
var roamingMost, roamingPast = 5000, 20000

// connHow is how a client came, as its AP's record of the attempt has it
// (0136): by a reassociation; its authentication, as hostapd names it (ft
// for a fast transition, sae for WPA3, open); a key from before, from this
// AP's own cache (pmksa) or made from a sign-in at another AP (okc); how
// often the AP asked the client it still held to prove itself (SA Query);
// how long from the first line its AP has of it to its being let on; and,
// where that first line was its association, as where hostapd says nothing
// of an authentication (began: assoc), that the time is short of what it
// took.
//
// It is read by itself, whatever else it holds: a later agent's word here
// that this manager does not know costs that word, and a value it cannot
// take costs how the client came, and neither the attempt's record.
type connHow struct {
	Re    bool   `json:"re,omitempty"`
	Alg   string `json:"alg,omitempty"`
	PMK   string `json:"pmk,omitempty"`
	SAQ   int    `json:"saq,omitempty"`
	OnMS  *int   `json:"on_ms,omitempty"`
	Began string `json:"began,omitempty"`
	bad   bool
}

var howAlgRE = regexp.MustCompile(`^[a-z0-9]{1,12}$`)

func (h *connHow) UnmarshalJSON(b []byte) error {
	type plain connHow
	var p plain
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&p); err != nil {
		h.bad = true
		return nil
	}
	*h = connHow(p)
	return nil
}

func (h *connHow) ok() bool {
	return !h.bad && (h.Alg == "" || howAlgRE.MatchString(h.Alg)) && (h.PMK == "" || h.PMK == "pmksa" || h.PMK == "okc") &&
		h.SAQ >= 0 && h.SAQ <= 64 && (h.OnMS == nil || (*h.OnMS >= 0 && *h.OnMS <= 600000)) && (h.Began == "" || h.Began == "assoc")
}

// roam is one attempt that was a roam: who, when, to which AP and from
// where, where that is known, of which kind, how long it took to be let
// on, and how it went: ok, slow (let on, after more than roamSlowMS),
// failed (not let on), or left (gone before it was let on, with nothing
// refused, as a client that thought better of it). One whose AP has no
// line of its authentication is let on with no time: what there is, from
// its association, is not what it took.
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
	Untimed   bool             `json:"untimed,omitempty"`
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
// which only such a client makes. signs says whether its network signs
// clients in by 802.1X, where a client that did not sign in again came
// with a key from before, though hostapd, logging less there (0133), does
// not say which; it is asked only where that decides.
func roamKind(rec roamRecord, signs func() bool) (string, bool) {
	h := rec.How
	if h == nil || !h.ok() || !(h.Re || h.Alg == "ft") {
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
	case signs():
		return roamCached, true
	}
	return roamKeys, true
}

// letOn says whether an attempt was let on to its network: its record says
// when, or, of an agent that did not say, it came online, was connected,
// or failed at something that comes after.
func letOn(cn conditions.Connection, rec roamRecord) bool {
	if rec.How != nil && rec.How.ok() && rec.How.OnMS != nil {
		return true
	}
	return cn.Outcome == "online" || cn.Outcome == "connected" ||
		(cn.Outcome == "failed" && slices.Contains([]string{"dhcp", "gateway", "dns", "internet"}, cn.Stage))
}

// roamKindsView is the roams of one kind: how many, how many of them
// failed and how many were slow, and of those let on and timed the middle,
// the least and the most of their times in ms.
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
	Untimed int             `json:"untimed"`
	Clients int             `json:"clients"`
	MS      *int            `json:"typical_ms,omitempty"`
	Kinds   []roamKindsView `json:"kinds"`
	Latest  []roam          `json:"latest"`
	Bad     []roam          `json:"bad"`
	// More is set where the APs asked about had more attempts in the time
	// than are read: the counts are then of the latest of them.
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

// roamSaid is what a client's own attempts say of one that was a roam: of
// which kind, and how long it took to be let on, where that is known.
type roamSaid struct {
	Kind string `json:"kind"`
	MS   *int   `json:"ms,omitempty"`
}

// roamsAmong reads the roams among attempts, which are the caller's to
// see, and gives them the latest first. An attempt is a roam where the
// client came as one already on the network (roamKind) to an AP or a radio
// other than the one it was last let on at; one that asked again on the
// radio it was on is none. Where a client was before is read from its own
// attempts on every AP, as the attempts given may leave out the APs it
// came from: together back to an hour before the first of the roams, and
// one by one further back for a client not seen in that time. It is named
// only where it is an AP the caller may view.
func (s *Server) roamsAmong(c call, list []conditions.Connection) ([]roam, error) {
	t := c.state.Org.Locations
	signsIn := s.signsIn(c.state)
	type cand struct {
		cn   conditions.Connection
		rec  roamRecord
		kind string
	}
	cands := map[int64]cand{}
	macs := map[string]bool{}
	var first time.Time
	for _, cn := range list {
		var rec roamRecord
		if json.Unmarshal(cn.Record, &rec) != nil {
			continue
		}
		kind, is := roamKind(rec, func() bool { return signsIn(cn.AP, cn.Network) })
		if !is {
			continue
		}
		cands[cn.ID] = cand{cn, rec, kind}
		macs[cn.MAC] = true
		if first.IsZero() || cn.Started.Before(first) {
			first = cn.Started
		}
	}
	if len(cands) == 0 {
		return nil, nil
	}
	who := make([]string, 0, len(macs))
	for mac := range macs {
		who = append(who, mac)
	}
	past, err := s.conds.ConnectionsOfClients(who, first.Add(-roamingBack), roamingPast)
	if err != nil {
		return nil, err
	}
	type place struct {
		ap        hierarchy.NodeID
		bss, band string
	}
	last := map[string]place{}
	var out []roam
	seen := map[int64]bool{}
	emit := func(k cand, before place, known bool) {
		cn, rec := k.cn, k.rec
		r := roam{ID: cn.ID, At: cn.Started, MAC: cn.MAC, Host: rec.Host, AP: cn.AP, Network: cn.Network, SSID: cn.SSID, Band: cn.Band,
			Kind: k.kind, SAQueries: rec.How.SAQ}
		if n, ok := t.Node(cn.AP); ok {
			r.APName = n.Name
		}
		if n, ok := t.Node(before.ap); known && ok && roleOn(c, change.Locations, t, before.ap) >= access.Viewer {
			r.FromAP, r.FromName, r.FromBand = before.ap, n.Name, before.band
		}
		switch {
		case rec.How.OnMS != nil && rec.How.Began == "assoc":
			r.State, r.Untimed = "ok", true
		case rec.How.OnMS != nil:
			r.MS = rec.How.OnMS
			r.State = "ok"
			if *r.MS > roamSlowMS {
				r.State = "slow"
			}
		case letOn(cn, rec):
			// Let on, as its outcome says, by a record that has no time
			// for it: it went well, and is not timed.
			r.State, r.Untimed = "ok", true
		case cn.Outcome == "failed":
			r.State, r.Reason = "failed", cn.Reason
		default:
			r.State, r.Reason = "left", cn.Reason
		}
		out = append(out, r)
	}
	// earlier is where a client was let on before an attempt, however long
	// before.
	var failed error
	earlier := func(cn conditions.Connection) (place, bool) {
		prev, ok, err := s.conds.LastLetOn(cn.MAC, cn.Started)
		if err != nil {
			failed = err
		}
		var rec roamRecord
		if !ok || json.Unmarshal(prev.Record, &rec) != nil {
			return place{}, false
		}
		return place{prev.AP, rec.BSS, prev.Band}, true
	}
	// The oldest first: each client's place as it goes.
	for i := len(past) - 1; i >= 0; i-- {
		cn := past[i]
		var rec roamRecord
		if json.Unmarshal(cn.Record, &rec) != nil {
			continue
		}
		here := place{cn.AP, rec.BSS, cn.Band}
		before, known := last[cn.MAC]
		if letOn(cn, rec) {
			last[cn.MAC] = here
		}
		k, is := cands[cn.ID]
		if !is {
			continue
		}
		seen[cn.ID] = true
		if !known {
			before, known = earlier(cn)
		}
		if known && before.ap == here.ap && before.bss == here.bss {
			continue
		}
		emit(k, before, known)
	}
	// One its client's attempts did not reach back to, as where there were
	// more of them than are read.
	for id, k := range cands {
		if seen[id] {
			continue
		}
		before, known := earlier(k.cn)
		if known && before.ap == k.cn.AP && before.bss == k.rec.BSS {
			continue
		}
		emit(k, before, known)
	}
	if failed != nil {
		return nil, failed
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.After(out[j].At)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
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
	var tests []string
	for mac := range testClients(c.state) {
		tests = append(tests, mac)
	}
	view := roamingView{Hours: hours, SlowMS: roamSlowMS, Kinds: []roamKindsView{}, Latest: []roam{}, Bad: []roam{}}
	list, err := s.conds.ConnectionsSince(time.Now().Add(-time.Duration(hours)*time.Hour), mayView(c, hierarchy.NodeID(q.Get("under"))), tests, roamingMost+1)
	if err != nil {
		return err
	}
	if len(list) > roamingMost {
		list, view.More = list[:roamingMost], true
	}
	roams, err := s.roamsAmong(c, list)
	if err != nil {
		return err
	}
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
		if m.Untimed {
			view.Untimed++
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
