package api

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/alerts"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// How the clients' own joins go (0127): the attempts each AP records of
// every client coming online (0118), counted by network. A client that
// tried a network in a time got on it, where an attempt of its came online
// or was let on; did not, where one failed and none got on; and is neither
// where it only ever left of its own accord.
const (
	// joinsWindow is the time an alert looks back over.
	joinsWindow = time.Hour
	// joinsFewest is how many clients must have been let on a network at an
	// AP and got no further for it to be an alert, where they are half or
	// more of those let on: fewer is those clients' own trouble.
	joinsFewest = 3
)

// afterLetOn are the stages a client fails at once the network has let it
// on: what it then lacks is the network's to give, an address, a gateway
// that answers, a DNS answer. Before that, a wrong passphrase or a refused
// sign-in is the client's own, however many clients have one.
var afterLetOn = map[string]bool{"dhcp": true, "gateway": true, "dns": true}

// joinsNetwork is how the clients that tried one network went: how many
// tried, got on and did not; their attempts, and how many of those failed;
// how long a client typically took to come online, in ms, the middle one of
// the clients' averages; where the failed attempts stopped, the most
// clients first; and the same by AP.
type joinsNetwork struct {
	Network        string      `json:"network"`
	SSID           string      `json:"ssid"`
	Clients        int         `json:"clients"`
	Online         int         `json:"online"`
	Failed         int         `json:"failed"`
	Attempts       int         `json:"attempts"`
	AttemptsFailed int         `json:"attempts_failed"`
	TypicalMS      int64       `json:"typical_ms,omitempty"`
	Failing        bool        `json:"failing"`
	Stops          []joinsStop `json:"stops"`
	APs            []joinsAP   `json:"aps"`
}

// joinsStop is a stage failed attempts stopped at, and why, as the APs put
// it: how many attempts, of how many clients, and when the latest began.
type joinsStop struct {
	Stage    string    `json:"stage"`
	Reason   string    `json:"reason"`
	Attempts int       `json:"attempts"`
	Clients  int       `json:"clients"`
	Last     time.Time `json:"last"`
}

// joinsAP is one AP's part of a network's joins; failing where it is an
// alert there now.
type joinsAP struct {
	AP             hierarchy.NodeID `json:"ap"`
	Name           string           `json:"name"`
	Clients        int              `json:"clients"`
	Online         int              `json:"online"`
	Failed         int              `json:"failed"`
	Attempts       int              `json:"attempts"`
	AttemptsFailed int              `json:"attempts_failed"`
	Failing        bool             `json:"failing"`
}

// joinsOf is a network as its joins are counted: an Aeolus network by its
// ID, whatever its SSID was at the time; another of the AP's own by its
// SSID.
func joinsOf(network, ssid string) string {
	if network != "" {
		return network
	}
	return "\x00" + ssid
}

// joinTally counts clients: each once, however many APs it tried on.
type joinTally struct {
	got, failed map[string]bool
	tried       map[string]bool
	attempts    int
	bad         int
}

func newTally() *joinTally {
	return &joinTally{got: map[string]bool{}, failed: map[string]bool{}, tried: map[string]bool{}}
}

func (t *joinTally) add(j conditions.Join) {
	t.tried[j.MAC] = true
	t.attempts += j.Attempts
	t.bad += j.Failed
	if j.Online > 0 {
		t.got[j.MAC] = true
	}
	if j.Failed > 0 {
		t.failed[j.MAC] = true
	}
}

// did is how many clients got on, and how many did not: failed, and never
// got on.
func (t *joinTally) did() (online, failed int) {
	for mac := range t.failed {
		if !t.got[mac] {
			failed++
		}
	}
	return len(t.got), failed
}

// mostStopped is where most of the clients that did not get on stopped:
// the stage and reason with the most clients, the most attempts among
// equals, then the latest.
func mostStopped(stops []joinsStop) (joinsStop, bool) {
	if len(stops) == 0 {
		return joinsStop{}, false
	}
	best := stops[0]
	for _, s := range stops[1:] {
		if s.Clients > best.Clients || (s.Clients == best.Clients && (s.Attempts > best.Attempts ||
			(s.Attempts == best.Attempts && s.Last.After(best.Last)))) {
			best = s
		}
	}
	return best, true
}

// numbers are the figures in a reason: how many requests went unanswered,
// an address. Reasons that differ only in those are one reason.
var numbers = regexp.MustCompile(`[0-9]+`)

// stopsOf adds up where failed attempts stopped, by stage and reason, the
// most clients first. A reason is given as its latest attempt had it.
func stopsOf(list []conditions.JoinStop) []joinsStop {
	type key struct{ stage, reason string }
	at := map[key]*joinsStop{}
	who := map[key]map[string]bool{}
	for _, s := range list {
		k := key{s.Stage, numbers.ReplaceAllString(s.Reason, "#")}
		if at[k] == nil {
			at[k] = &joinsStop{Stage: s.Stage}
			who[k] = map[string]bool{}
		}
		at[k].Attempts += s.Attempts
		who[k][s.MAC] = true
		if at[k].Last.IsZero() || s.Last.After(at[k].Last) {
			at[k].Last, at[k].Reason = s.Last, s.Reason
		}
	}
	out := []joinsStop{}
	for k, s := range at {
		s.Clients = len(who[k])
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Clients != b.Clients {
			return a.Clients > b.Clients
		}
		if a.Attempts != b.Attempts {
			return a.Attempts > b.Attempts
		}
		if a.Stage != b.Stage {
			return a.Stage < b.Stage
		}
		return a.Reason < b.Reason
	})
	return out
}

// joined reads how the joins went since a time, but for the APs' own test
// clients (0121), which are no one's devices and whose failures are the
// tests' to say.
func (s *Server) joined(state *change.State, since time.Time) ([]conditions.Join, []conditions.JoinStop, error) {
	joins, stops, err := s.conds.Joins(since)
	if err != nil {
		return nil, nil, err
	}
	own := testClients(state)
	mine := func(mac string) bool { _, ok := own[strings.ToLower(mac)]; return ok }
	joins = slices.DeleteFunc(joins, func(j conditions.Join) bool { return mine(j.MAC) })
	stops = slices.DeleteFunc(stops, func(j conditions.JoinStop) bool { return mine(j.MAC) })
	return joins, stops, nil
}

// failingJoins are the networks clients cannot get on now, by the AP each
// is on, for the alerts (0127): over the last hour, three clients or more
// were let on a network there and got no further, and they are half or more
// of the clients let on, those that got on and those.
func (s *Server) failingJoins(state *change.State, now time.Time) map[hierarchy.NodeID][]alerts.FailingJoins {
	out := map[hierarchy.NodeID][]alerts.FailingJoins{}
	joins, stops, err := s.joined(state, now.Add(-joinsWindow))
	if err != nil || len(joins) == 0 {
		return out
	}
	type key struct {
		ap  hierarchy.NodeID
		net string
	}
	tallies := map[key]*joinTally{}
	ssids := map[key]string{}
	ids := map[key]string{}
	var order []key
	for _, j := range joins {
		k := key{j.AP, joinsOf(j.Network, j.SSID)}
		if tallies[k] == nil {
			tallies[k] = newTally()
			order = append(order, k)
		}
		tallies[k].add(j)
		ssids[k], ids[k] = j.SSID, j.Network
	}
	for _, k := range order {
		t := tallies[k]
		var first time.Time
		mine := []conditions.JoinStop{}
		stuck := map[string]bool{}
		for _, st := range stops {
			if st.AP != k.ap || joinsOf(st.Network, st.SSID) != k.net || t.got[st.MAC] || !afterLetOn[st.Stage] {
				continue
			}
			mine = append(mine, st)
			stuck[st.MAC] = true
			if first.IsZero() || st.First.Before(first) {
				first = st.First
			}
		}
		let := len(t.got) + len(stuck)
		if len(stuck) < joinsFewest || 2*len(stuck) < let {
			continue
		}
		f := alerts.FailingJoins{Network: ids[k], SSID: ssids[k], LetOn: let, Failed: len(stuck)}
		if most, ok := mostStopped(stopsOf(mine)); ok {
			f.Stage, f.Reason = most.Stage, most.Reason
		}
		if !first.IsZero() {
			f.Since = &first
		}
		out[k.ap] = append(out[k.ap], f)
	}
	return out
}

// joinsView is how the clients' own joins went on the APs a caller may
// view, below a node where under names one, since from: a network a line,
// those clients cannot get on now first, then those where the most did not.
func (s *Server) joinsView(c call, under hierarchy.NodeID, from, now time.Time) ([]joinsNetwork, error) {
	t := c.state.Org.Locations
	shown := map[hierarchy.NodeID]bool{}
	for _, id := range t.APs() {
		if roleOn(c, change.Locations, t, id) < access.Viewer {
			continue
		}
		if under != "" && under != id && !slices.Contains(t.Ancestry(id), under) {
			continue
		}
		shown[id] = true
	}
	joins, stops, err := s.joined(c.state, from)
	if err != nil {
		return nil, err
	}
	failing := s.failingJoins(c.state, now)
	alerted := func(ap hierarchy.NodeID, net string) bool {
		for _, f := range failing[ap] {
			if joinsOf(f.Network, f.SSID) == net {
				return true
			}
		}
		return false
	}
	name := func(id hierarchy.NodeID) string {
		if n, ok := t.Node(id); ok {
			return n.Name
		}
		return string(id)
	}
	type apKey struct {
		net string
		ap  hierarchy.NodeID
	}
	nets := map[string]*joinsNetwork{}
	tallies := map[string]*joinTally{}
	perAP := map[apKey]*joinTally{}
	// How long each client took to come online on a network, on average.
	took := map[string]map[string][2]int64{}
	var order []string
	for _, j := range joins {
		if !shown[j.AP] {
			continue
		}
		k := joinsOf(j.Network, j.SSID)
		if nets[k] == nil {
			nets[k] = &joinsNetwork{Network: j.Network, Stops: []joinsStop{}, APs: []joinsAP{}}
			tallies[k] = newTally()
			took[k] = map[string][2]int64{}
			order = append(order, k)
		}
		// The SSID is the one of its latest rows: the rows come in order.
		nets[k].SSID = j.SSID
		tallies[k].add(j)
		ak := apKey{k, j.AP}
		if perAP[ak] == nil {
			perAP[ak] = newTally()
		}
		perAP[ak].add(j)
		if j.Seen > 0 {
			v := took[k][j.MAC]
			took[k][j.MAC] = [2]int64{v[0] + j.SeenMS, v[1] + int64(j.Seen)}
		}
	}
	out := []joinsNetwork{}
	for _, k := range order {
		n, tally := nets[k], tallies[k]
		n.Clients, n.Attempts, n.AttemptsFailed = len(tally.tried), tally.attempts, tally.bad
		n.Online, n.Failed = tally.did()
		var each []int64
		for _, v := range took[k] {
			if v[1] > 0 {
				each = append(each, v[0]/v[1])
			}
		}
		if len(each) > 0 {
			slices.Sort(each)
			n.TypicalMS = each[len(each)/2]
		}
		mine := []conditions.JoinStop{}
		for _, st := range stops {
			if shown[st.AP] && joinsOf(st.Network, st.SSID) == k {
				mine = append(mine, st)
			}
		}
		n.Stops = stopsOf(mine)
		for ak, at := range perAP {
			if ak.net != k {
				continue
			}
			a := joinsAP{AP: ak.ap, Name: name(ak.ap), Clients: len(at.tried), Attempts: at.attempts, AttemptsFailed: at.bad, Failing: alerted(ak.ap, k)}
			a.Online, a.Failed = at.did()
			n.Failing = n.Failing || a.Failing
			n.APs = append(n.APs, a)
		}
		sort.Slice(n.APs, func(i, j int) bool {
			a, b := n.APs[i], n.APs[j]
			if a.Failing != b.Failing {
				return a.Failing
			}
			if a.Failed != b.Failed {
				return a.Failed > b.Failed
			}
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			return a.AP < b.AP
		})
		out = append(out, *n)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Failing != b.Failing {
			return a.Failing
		}
		if a.Failed != b.Failed {
			return a.Failed > b.Failed
		}
		if a.Clients != b.Clients {
			return a.Clients > b.Clients
		}
		return a.SSID < b.SSID
	})
	return out, nil
}
