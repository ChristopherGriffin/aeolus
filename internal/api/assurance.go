package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/alerts"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/radio"
)

// Service assurance (0123): the service tried from a client's side, on a
// schedule. Where a folder says how often (assurance.tests.every), the
// manager asks each AP there that has a scan radio for a client test (0121)
// that often: each time one network, on one AP, on one band, turn and turn
// about. What a test finds is kept as any client test's is. A network that
// fails twice running where it is tested is an alert (0099) on the AP it
// failed on.

const (
	// assureFloor is the weakest an AP may be heard, in dBm, to be tested
	// from here: weaker, a test fails for the distance and says nothing
	// of the service.
	assureFloor = -75
	// assureRetry is how soon a test that failed for the first time is
	// made again, to tell a network that does not work from one bad try.
	assureRetry = 2 * time.Minute
	// assureWindow is how far back the tests count for what is failing.
	assureWindow = 24 * time.Hour
	// assureKeep is how long a client test is kept.
	assureKeep = 30 * 24 * time.Hour
)

// assureCombo is one thing tested: a network, the AP it is joined on, the
// tester itself among them, and a band.
type assureCombo struct {
	Network string `json:"network"`
	On      string `json:"on"`
	Band    string `json:"band"`
}

// testRecord is one client test, asked of an AP: what was asked and, once
// it is done, what it found.
type testRecord struct {
	conditions.Action
	ask    clientTestAsk
	detail *clientTest
}

func readTest(a conditions.Action) testRecord {
	r := testRecord{Action: a}
	json.Unmarshal(a.Params, &r.ask)
	if len(a.Detail) > 0 {
		var d clientTest
		if json.Unmarshal(a.Detail, &d) == nil {
			r.detail = &d
		}
	}
	return r
}

func (r testRecord) combo() assureCombo {
	return assureCombo{Network: r.ask.Network, On: r.ask.On, Band: r.ask.Band}
}

// ran says whether the test was made: the radio taken, and the network
// looked for. One that never had the radio, or that its AP never took up,
// says nothing of the network.
func (r testRecord) ran() bool { return r.detail != nil && r.detail.Client != "" }

func (r testRecord) failed() bool { return r.ran() && r.detail.Outcome != "passed" }

func (r testRecord) open() bool { return r.State == "pending" || r.State == "running" }

// assureSettings are an AP's, from its folders: how often it is asked for
// a test, none for never, and whether a test on a schedule measures speed.
func assureSettings(state *change.State, ap hierarchy.NodeID) (time.Duration, bool) {
	cfg, err := state.Org.ResolveAP(ap)
	if err != nil || cfg.Unassigned {
		return 0, false
	}
	var every time.Duration
	if r, ok := cfg.Location["assurance.tests.every"]; ok {
		switch v := r.Value.(type) {
		case float64:
			every = time.Duration(v) * time.Minute
		case int:
			every = time.Duration(v) * time.Minute
		case json.Number:
			if n, err := v.Int64(); err == nil {
				every = time.Duration(n) * time.Minute
			}
		}
	}
	speed := false
	if r, ok := cfg.Location["assurance.tests.speed"]; ok {
		speed, _ = r.Value.(bool)
	}
	return every, speed
}

// assureCombos are what an AP with a scan radio tests, in the order it
// takes them: each network it offers that a test can join, on itself and
// on each AP it hears well enough that offers it too, on each band the
// network is on there.
func (s *Server) assureCombos(state *change.State, ap hierarchy.NodeID, docs map[hierarchy.NodeID]map[string]any) []assureCombo {
	doc := func(id hierarchy.NodeID) map[string]any {
		if d, ok := docs[id]; ok {
			return d
		}
		var nets map[string]any
		if res, err := s.compose(state, id, s.reveal); err == nil && !res.Unassigned {
			nets, _ = res.Doc["network"].(map[string]any)
		}
		docs[id] = nets
		return nets
	}
	// offered says whether a network, as an AP's config has it, is on a band there.
	offered := func(n map[string]any, band string) bool {
		sec, _ := n["security"].(string)
		if _, six := radio.SixGHzEncryption(sec); band == "6g" && !six {
			return false
		}
		list, ok := n["bands"].([]any)
		return !ok || slices.Contains(list, any(band))
	}
	type place struct {
		ap    hierarchy.NodeID
		bands []string
	}
	places := []place{{ap: ap, bands: servingBands(state, ap)}}
	if st, err := s.conds.LatestState(ap); err == nil && st != nil {
		var rep struct {
			RRM struct {
				Neighbours []rrmNeighbour `json:"neighbours"`
			} `json:"rrm"`
		}
		json.Unmarshal(st.Report, &rep)
		for _, n := range rep.RRM.Neighbours {
			id := hierarchy.NodeID(n.AP)
			if node, ok := state.Org.Locations.Node(id); !ok || node.Kind != hierarchy.KindAP || n.State == "down" || id == ap {
				continue
			}
			p := place{ap: id}
			for _, b := range n.Bands {
				if b.Signal != nil && *b.Signal >= assureFloor {
					p.bands = append(p.bands, b.Band)
				}
			}
			if len(p.bands) > 0 {
				places = append(places, p)
			}
		}
	}
	sort.SliceStable(places[1:], func(i, j int) bool { return places[1+i].ap < places[1+j].ap })
	mine := doc(ap)
	ids := make([]string, 0, len(mine))
	for id := range mine {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []assureCombo
	for _, id := range ids {
		n, _ := mine[id].(map[string]any)
		sec, _ := n["security"].(string)
		if rad, _ := n["radius"].(map[string]any); !testable[sec] || rad["passphrases"] == true {
			continue
		}
		for _, p := range places {
			theirs, _ := doc(p.ap)[id].(map[string]any)
			if theirs == nil || theirs["ssid"] != n["ssid"] {
				continue
			}
			for _, band := range []string{"2g", "5g", "6g"} {
				if slices.Contains(p.bands, band) && offered(theirs, band) {
					out = append(out, assureCombo{Network: id, On: string(p.ap), Band: band})
				}
			}
		}
	}
	return out
}

// pollOf is how often an AP calls in, by its folders: a minute unless set.
func pollOf(state *change.State, ap hierarchy.NodeID) time.Duration {
	if r, ok := state.Org.Locations.Resolve(ap, hierarchy.Path("system.poll")); ok {
		if v, ok := r.Value.(float64); ok && v > 0 {
			return time.Duration(v) * time.Second
		}
	}
	return time.Minute
}

// servingBands are the bands an AP says it has a radio on that serves
// clients: not one another service owns (0081).
func servingBands(state *change.State, ap hierarchy.NodeID) []string {
	var f struct {
		Radios []struct {
			Band     string `json:"band"`
			Reserved bool   `json:"reserved"`
		} `json:"radios"`
	}
	json.Unmarshal(state.Facts[ap], &f)
	var out []string
	for _, r := range f.Radios {
		if !r.Reserved && bands[r.Band] && !slices.Contains(out, r.Band) {
			out = append(out, r.Band)
		}
	}
	return out
}

// assureVerdict is what the tests of one thing, by one AP, come to.
type assureVerdict struct {
	last    *testRecord // the latest that was made
	failing bool        // the latest two made both failed
	since   time.Time   // when the first of the failures running was asked
	passed  int
	failed  int
}

// assureKey is one thing tested by one AP.
type assureKey struct {
	tester hierarchy.NodeID
	combo  assureCombo
}

// assureVerdicts reads the tests of a time, newest first, into what each
// thing tested by each AP comes to. One failure is not yet a network that
// does not work; two running are.
func assureVerdicts(list []testRecord) map[assureKey]*assureVerdict {
	out := map[assureKey]*assureVerdict{}
	run := map[assureKey]int{}    // the failures running, from the latest back
	ended := map[assureKey]bool{} // a test that passed has ended them
	for i := range list {
		r := list[i]
		if !r.ran() {
			continue
		}
		k := assureKey{tester: hierarchy.NodeID(r.AP), combo: r.combo()}
		v := out[k]
		if v == nil {
			v = &assureVerdict{last: &list[i]}
			out[k] = v
		}
		if !r.failed() {
			v.passed++
			ended[k] = true
			continue
		}
		v.failed++
		if !ended[k] {
			run[k]++
			v.since = r.At
		}
	}
	for k, v := range out {
		if v.failing = run[k] >= 2; !v.failing {
			v.since = time.Time{}
		}
	}
	return out
}

// tests are the client tests of a time, newest first, read.
func (s *Server) tests(since time.Time) ([]testRecord, error) {
	list, err := s.conds.ClientTests(since)
	if err != nil {
		return nil, err
	}
	out := make([]testRecord, len(list))
	for i, a := range list {
		out[i] = readTest(a)
	}
	return out, nil
}

// assure asks each AP for the client test it is due, and returns what it
// asked. An AP is due when its folders say how often, it has a scan radio,
// it has called in lately and runs its config, and no test of it is open.
// What it tests next is the thing it tested longest ago; but a thing that
// failed for the first time is tested again within minutes, and one that
// is failing every other time, so its mending is seen soon.
func (s *Server) assure(now time.Time) []conditions.Action {
	state := s.log.Snapshot()
	if state == nil {
		return nil
	}
	list, err := s.tests(now.Add(-assureWindow))
	if err != nil {
		slog.Warn("assurance: cannot read the tests", "err", err)
		return nil
	}
	verdicts := assureVerdicts(list)
	docs := map[hierarchy.NodeID]map[string]any{}
	var asked []conditions.Action
	for _, ap := range state.Org.Locations.APs() {
		every, speed := assureSettings(state, ap)
		if every <= 0 || !scanRadio(state, ap) {
			continue
		}
		l, err := s.conds.Latest(ap)
		version, _ := s.log.Version(ap)
		if err != nil || l.Seen == nil || now.Sub(l.Seen.At) > alerts.Offline(pollOf(state, ap)) || l.Seen.Running == nil || *l.Seen.Running != version {
			continue
		}
		var last *testRecord // the latest the manager asked itself
		open := false
		for i := range list {
			r := &list[i]
			if hierarchy.NodeID(r.AP) != ap {
				continue
			}
			open = open || r.open()
			if last == nil && r.Actor == change.SystemActor {
				last = r
			}
		}
		// Not while one is open, nor sooner than any wait could end.
		if open || (last != nil && now.Sub(last.At) < min(every, assureRetry)) {
			continue
		}
		combos := s.assureCombos(state, ap, docs)
		if len(combos) == 0 {
			continue
		}
		// When each thing was last asked for by the manager: the one asked
		// longest ago comes first, and one never asked before any.
		at := map[assureCombo]time.Time{}
		for i := range list {
			r := &list[i]
			if hierarchy.NodeID(r.AP) == ap && r.Actor == change.SystemActor {
				if _, ok := at[r.combo()]; !ok {
					at[r.combo()] = r.At
				}
			}
		}
		oldest := func(of []assureCombo) []assureCombo {
			sort.SliceStable(of, func(i, j int) bool { return at[of[i]].Before(at[of[j]]) })
			return of
		}
		var failing, rest []assureCombo
		for _, c := range combos {
			if v := verdicts[assureKey{tester: ap, combo: c}]; v != nil && v.failing {
				failing = append(failing, c)
			} else {
				rest = append(rest, c)
			}
		}
		// What to ask, the likeliest first. A thing that failed for the
		// first time is asked again, soon; else a failing thing every other
		// time, and the rest in their turn.
		var order []assureCombo
		wait := every
		if last != nil {
			v := verdicts[assureKey{tester: ap, combo: last.combo()}]
			if last.failed() && v != nil && !v.failing && slices.Contains(combos, last.combo()) {
				wait = min(every, assureRetry)
				order = []assureCombo{last.combo()}
			}
			if now.Sub(last.At) < wait {
				continue
			}
		}
		if wasFailing := last != nil && slices.Contains(failing, last.combo()); len(failing) > 0 && !wasFailing {
			order = append(append(order, oldest(failing)...), oldest(rest)...)
		} else {
			order = append(append(order, oldest(rest)...), oldest(failing)...)
		}
		// The first of them that can be asked: one that cannot, as its AP
		// has not said its BSSIDs yet, does not hold up the others.
		var ask clientTestAsk
		var next *assureCombo
		for i := range order {
			c := order[i]
			if ask, err = s.clientTestOf(state, ap, c.Network, c.On, c.Band, speed, func(hierarchy.NodeID) bool { return true }); err == nil {
				next = &c
				break
			}
			slog.Debug("assurance: a test cannot be asked", "ap", ap, "network", c.Network, "on", c.On, "band", c.Band, "err", err)
		}
		if next == nil {
			continue
		}
		params, err := json.Marshal(ask)
		if err != nil {
			continue
		}
		a, err := s.conds.AddActionWith(ap, "client-test", "", change.SystemActor, params)
		if err != nil {
			slog.Warn("assurance: a test was not asked", "ap", ap, "err", err)
			continue
		}
		slog.Info("assurance: test asked", "ap", ap, "network", next.Network, "on", next.On, "band", next.Band, "id", a.ID)
		asked = append(asked, a)
	}
	return asked
}

// AssureAt asks what is due at a time, once, and says how many tests it
// asked: for whoever drives the schedule by hand.
func (s *Server) AssureAt(now time.Time) int { return len(s.assure(now)) }

// Assure asks the client tests the folders want on a schedule (0123),
// looking every interval until ctx ends, and forgets tests older than a
// month.
func (s *Server) Assure(ctx context.Context, every time.Duration) {
	pruned := time.Time{}
	look := func() {
		now := time.Now()
		s.assure(now)
		if now.Sub(pruned) > time.Hour {
			pruned = now
			if n, err := s.conds.PruneClientTests(now.Add(-assureKeep)); err != nil {
				slog.Warn("assurance: old tests not forgotten", "err", err)
			} else if n > 0 {
				slog.Info("assurance: old tests forgotten", "tests", n)
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

// failingTests are the things failing now, by the AP each fails on, for
// its alerts (0099): the network, where, and what the latest test said. A
// verdict stands while its AP is still asked for tests, or for an hour
// after the last of them.
func (s *Server) failingTests(state *change.State, now time.Time) map[hierarchy.NodeID][]alerts.FailingTest {
	out := map[hierarchy.NodeID][]alerts.FailingTest{}
	list, err := s.tests(now.Add(-assureWindow))
	if err != nil {
		return out
	}
	t := state.Org.Locations
	for k, v := range assureVerdicts(list) {
		if !v.failing {
			continue
		}
		tester, ok := t.Node(k.tester)
		if !ok {
			continue
		}
		if every, _ := assureSettings(state, k.tester); every <= 0 && now.Sub(v.last.At) > time.Hour {
			continue
		} else if every > 0 && now.Sub(v.last.At) > 3*every+time.Hour {
			continue
		}
		on := hierarchy.NodeID(k.combo.On)
		if _, ok := t.Node(on); !ok {
			on = k.tester // the strongest AP it heard, whichever that was
		}
		since := v.since
		out[on] = append(out[on], alerts.FailingTest{
			Network: k.combo.Network, SSID: v.last.ask.SSID, Band: k.combo.Band, Tester: tester.Name, Itself: on == k.tester,
			Stage: v.last.detail.Stage, Reason: v.last.detail.Reason, Since: &since,
		})
	}
	for _, list := range out {
		sort.Slice(list, func(i, j int) bool {
			if list[i].Network != list[j].Network {
				return list[i].Network < list[j].Network
			}
			return list[i].Band < list[j].Band
		})
	}
	return out
}

// assuranceView is one AP that tests, for a person.
type assuranceTester struct {
	AP     hierarchy.NodeID `json:"ap"`
	Name   string           `json:"name"`
	Every  int              `json:"every"` // minutes; 0 where it is not asked on a schedule
	Speed  bool             `json:"speed"`
	Things int              `json:"things"` // how many things it tests in turn
}

// assuranceCheck is one thing tested by one AP, and what its tests of the
// time come to.
type assuranceCheck struct {
	Tester     hierarchy.NodeID   `json:"tester"`
	TesterName string             `json:"tester_name"`
	Network    string             `json:"network"`
	SSID       string             `json:"ssid,omitempty"`
	On         string             `json:"on"`
	OnName     string             `json:"on_name"`
	Band       string             `json:"band,omitempty"`
	Scheduled  bool               `json:"scheduled"` // among what the AP is asked for in turn now
	Failing    bool               `json:"failing"`
	Since      *time.Time         `json:"since,omitempty"`
	Passed     int                `json:"passed"`
	Failed     int                `json:"failed"`
	Last       *conditions.Action `json:"last,omitempty"`
}

// assurance is the client tests of the APs the caller may view, below one
// Locations node where ?under= names it, over ?hours= (24 unless set, at
// most 720): GET /v1/assurance. testers are the APs with a scan radio;
// checks, each thing one of them is asked for in turn, or was, or that is
// failing, with what its tests come to, the failing first; tests, the
// latest 50 of them, by hand or on the schedule, newest first.
func (s *Server) assurance(w http.ResponseWriter, r *http.Request, c call) error {
	t := c.state.Org.Locations
	under := hierarchy.NodeID(r.URL.Query().Get("under"))
	hours := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		n, err := strconv.Atoi(h)
		if err != nil || n < 1 || n > 720 {
			return badRequest("hours: 1 to 720")
		}
		hours = n
	}
	now := time.Now()
	list, err := s.tests(now.Add(-time.Duration(hours) * time.Hour))
	if err != nil {
		return err
	}
	view := map[hierarchy.NodeID]bool{}
	testers := []assuranceTester{}
	checks := []assuranceCheck{}
	docs := map[hierarchy.NodeID]map[string]any{}
	name := func(id hierarchy.NodeID) string {
		if n, ok := t.Node(id); ok {
			return n.Name
		}
		return string(id)
	}
	seen := map[assureKey]bool{}
	verdicts := assureVerdicts(list)
	for _, id := range t.APs() {
		if roleOn(c, change.Locations, t, id) < access.Viewer || !scanRadio(c.state, id) {
			continue
		}
		if under != "" && under != id && !slices.Contains(t.Ancestry(id), under) {
			continue
		}
		view[id] = true
		every, speed := assureSettings(c.state, id)
		combos := s.assureCombos(c.state, id, docs)
		testers = append(testers, assuranceTester{AP: id, Name: name(id), Every: int(every / time.Minute), Speed: speed, Things: len(combos)})
		ssid := func(network string) string {
			n, _ := docs[id][network].(map[string]any)
			v, _ := n["ssid"].(string)
			return v
		}
		add := func(k assureKey, scheduled bool) {
			if seen[k] {
				return
			}
			seen[k] = true
			ch := assuranceCheck{Tester: id, TesterName: name(id), Network: k.combo.Network, SSID: ssid(k.combo.Network), On: k.combo.On,
				OnName: name(hierarchy.NodeID(k.combo.On)), Band: k.combo.Band, Scheduled: scheduled}
			if k.combo.On == "" {
				ch.OnName = ""
			}
			if v := verdicts[k]; v != nil {
				ch.Failing, ch.Passed, ch.Failed = v.failing, v.passed, v.failed
				if v.failing {
					since := v.since
					ch.Since = &since
				}
				last := v.last.Action
				ch.Last = &last
				if ch.SSID == "" {
					ch.SSID = v.last.ask.SSID
				}
			}
			checks = append(checks, ch)
		}
		// What it is asked for in turn; and what it was, or that fails,
		// though it is asked for no longer. A thing a person tried once by
		// hand is among the tests, not here.
		if every > 0 {
			for _, combo := range combos {
				add(assureKey{tester: id, combo: combo}, true)
			}
		}
		for k, v := range verdicts {
			if k.tester == id && (v.failing || v.last.Actor == change.SystemActor) {
				add(k, false)
			}
		}
	}
	sort.SliceStable(checks, func(i, j int) bool {
		a, b := checks[i], checks[j]
		if a.Failing != b.Failing {
			return a.Failing
		}
		if a.TesterName != b.TesterName {
			return a.TesterName < b.TesterName
		}
		if a.SSID != b.SSID {
			return a.SSID < b.SSID
		}
		if a.OnName != b.OnName {
			return a.OnName < b.OnName
		}
		return a.Band < b.Band
	})
	tests := []conditions.Action{}
	for _, r := range list {
		if view[hierarchy.NodeID(r.AP)] && len(tests) < 50 {
			tests = append(tests, r.Action)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"testers": testers, "checks": checks, "tests": tests, "hours": hours})
	return nil
}
