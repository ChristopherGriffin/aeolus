package api

import (
	"encoding/json"
	"slices"

	"github.com/ChristopherGriffin/aeolus/internal/conditions"
)

// The way online (0130): the points a client passes on its way online, in
// order, each with how it went and how long it took. A client test (0121,
// 0128) and a real client's own attempt (0118) are read the same way, from
// the steps their AP recorded, so one is drawn as the other is, and their
// times can be set side by side.

// wayPoint is one point on the way: which; how it went, ok, bad, begun
// where it was started and not seen through, or none where nothing was seen
// of it; and how long it took, in ms, where that is known.
type wayPoint struct {
	K     string   `json:"k"`
	State string   `json:"state"`
	MS    *float64 `json:"ms,omitempty"`
}

// wayStep is a step as the way reads it, a test's or an attempt's: when it
// came, in ms from the start; the point it belongs to; whether it went
// well, where it says; and how long its answer took, where it says.
type wayStep struct {
	T  int64
	K  string
	OK *bool
	MS *float64
}

// wayKind is how one kind of way is read: its points in order; those whose
// time is their answer's own, as the step says it; those never timed from
// the point before, where that time says nothing, as of a gateway pinged
// three times a second apart, or of an address a client asked for when it
// chose to; those done at their first success, as a first name looked up
// is, where the others are done at their last step; and those left out
// where nothing was seen of them.
type wayKind struct {
	order    []string
	own      map[string]bool
	noGap    map[string]bool
	first    map[string]bool
	optional map[string]bool
	// fromStart says the steps' times are from the start of the way, as a
	// test's are, so its first point's time is from then. A client's are
	// from its first step, which says nothing of how long that took.
	fromStart bool
}

func set(keys ...string) map[string]bool {
	out := map[string]bool{}
	for _, k := range keys {
		out[k] = true
	}
	return out
}

var (
	// A test through the air (0121): the scan radio finds the network,
	// joins it, and asks for what a client asks.
	wayAir = wayKind{
		order:     []string{"find", "auth", "assoc", "signin", "key", "dhcp", "gateway", "dns", "speed"},
		own:       set("dhcp", "gateway", "dns"),
		noGap:     set("gateway", "dns", "speed"),
		optional:  set("signin", "speed"),
		fromStart: true,
	}
	// A test over the wire (0128): an address offered, then given; the
	// gateway; a name; and the address given back. An agent from before
	// 0130 says nothing of the offer, and its address is one point.
	wayWire = wayKind{
		order:     []string{"offer", "dhcp", "gateway", "dns", "release"},
		own:       set("dhcp", "gateway", "dns"),
		noGap:     set("gateway", "dns", "release"),
		optional:  set("offer", "release"),
		fromStart: true,
	}
	// A client's own attempt (0118), as its AP saw it. What a client asks
	// for, it asks for when it chooses: how long after its keys says
	// nothing of the network.
	wayClient = wayKind{
		order:    connStages,
		own:      set("gateway", "dns", "internet"),
		noGap:    set("dhcp", "gateway", "dns", "internet"),
		first:    set("gateway", "dns", "internet"),
		optional: set("signin"),
	}
)

// wayOf reads the way from its steps, in the order they came. failed is
// the point the whole stopped at, where it failed: that point is bad though
// no step of it says so, as of an address asked for and never answered.
//
// How a point went is what its last step that says said: a look that
// missed and then found the network went well. One done at its first
// success went well where any step did.
//
// How long a point took is, the first of these that applies: its answer's
// own time, where its kind takes that and the step says it; from its first
// step to its end, where it began with a step that only asks; from the
// point before it to its end, where its kind is timed so. One only begun
// has no time.
func wayOf(steps []wayStep, kind wayKind, failed string) []wayPoint {
	by := map[string][]wayStep{}
	for _, s := range steps {
		by[s.K] = append(by[s.K], s)
	}
	out := make([]wayPoint, 0, len(kind.order))
	var before int64
	begun := kind.fromStart
	for _, k := range kind.order {
		mine := by[k]
		p := wayPoint{K: k, State: "none"}
		if len(mine) == 0 {
			if k == failed {
				p.State = "bad"
			}
			if !kind.optional[k] || p.State == "bad" {
				out = append(out, p)
			}
			continue
		}
		// Where it was done: at its first success, or at its last step.
		end := mine[len(mine)-1]
		var said *bool
		won := false
		for _, s := range mine {
			if s.OK == nil {
				continue
			}
			said = s.OK
			if *s.OK && !won {
				won = true
				if kind.first[k] {
					end = s
				}
			}
		}
		switch {
		case k == failed:
			p.State = "bad"
		case kind.first[k] && won, said != nil && *said:
			p.State = "ok"
		case said != nil:
			p.State = "bad"
		default:
			p.State = "begun"
		}
		var ms float64 = -1
		switch {
		case p.State == "begun":
		case kind.own[k] && p.State == "ok" && end.MS != nil:
			ms = *end.MS
		case mine[0].OK == nil && len(mine) > 1 && end.T >= mine[0].T:
			ms = float64(end.T - mine[0].T)
		case !kind.noGap[k] && begun && end.T >= before:
			ms = float64(end.T - before)
		}
		if ms >= 0 {
			p.MS = &ms
		}
		before, begun = mine[len(mine)-1].T, true
		out = append(out, p)
	}
	return out
}

// testWay is a client test's way, and the point it stopped at, where it
// failed at one of them. A step belongs to the point it names, or to its
// stage.
func testWay(d *clientTest) (way []wayPoint, stopped string) {
	wire := d.Band == wireBand
	kind := wayAir
	if wire {
		kind = wayWire
	}
	steps := make([]wayStep, len(d.Steps))
	past := false
	for i, s := range d.Steps {
		k := s.K
		// A point that is none of this kind's is read as its stage.
		if k == "" || !slices.Contains(kind.order, k) {
			k = s.Stage
			// Over the wire the address is given back at the end, which an
			// agent from before 0130 says as one more step of DHCP.
			if wire && k == "dhcp" && past {
				k = "release"
			}
		}
		past = past || s.Stage == "gateway" || s.Stage == "dns"
		steps[i] = wayStep{T: s.T, K: k, OK: s.OK, MS: s.MS}
		// It was sent, and nothing answers it: that is it done.
		if k == "release" && s.OK == nil {
			yes := true
			steps[i].OK = &yes
		}
	}
	if d.Outcome == "failed" {
		stopped = d.Stage
		// Over the wire it stopped at DHCP: before an offer, where it was
		// asking for one and none came.
		if wire && stopped == "dhcp" && slices.ContainsFunc(steps, func(s wayStep) bool { return s.K == "offer" && s.OK != nil && !*s.OK }) {
			stopped = "offer"
		}
	}
	way = wayOf(steps, kind, stopped)
	if !slices.Contains(kind.order, stopped) {
		// It stopped before any of them, as one that could not be made.
		stopped = ""
	}
	at := func(k string) *wayPoint {
		for i := range way {
			if way[i].K == k {
				return &way[i]
			}
		}
		return nil
	}
	// What the test measured, where its steps do not say: all that is left
	// of a test whose steps were cut short, or kept by a manager from
	// before they were.
	measured := func(k string, ok bool, ms *float64) {
		if p := at(k); p != nil && p.State == "none" && ok {
			p.State, p.MS = "ok", ms
		}
	}
	if d.DHCP != nil {
		ms := float64(d.DHCP.MS)
		measured("dhcp", true, &ms)
	}
	if g := d.Gateway; g != nil {
		ms := g.Avg
		if ms == nil {
			ms = g.ARPMS
		}
		measured("gateway", g.Got > 0 || g.ARPMS != nil || g.MAC != "", ms)
	}
	if n := d.DNS; n != nil && n.Rcode != nil && *n.Rcode == 0 {
		var ms *float64
		if n.MS != nil {
			v := float64(*n.MS)
			ms = &v
		}
		measured("dns", true, ms)
	}
	// Over the wire, the address's own time is all of DHCP's, from the
	// first asking: with the offer a point of its own, what is left after
	// it. The step itself comes later than the acknowledgement did, by the
	// test's own look for another that uses the address.
	if offer, given := at("offer"), at("dhcp"); wire && offer != nil && given != nil && offer.MS != nil && given.MS != nil && given.State == "ok" {
		left := max(0, *given.MS-*offer.MS)
		given.MS = &left
	}
	return way, stopped
}

// connWay is the way of a client's own attempt, from its AP's record of
// it, and the point it stopped at, where it failed.
func connWay(c conditions.Connection) (way []wayPoint, stopped string) {
	var rec struct {
		Events []connStep `json:"events"`
	}
	json.Unmarshal(c.Record, &rec)
	steps := make([]wayStep, 0, len(rec.Events))
	for _, e := range rec.Events {
		s := wayStep{T: int64(e.T), K: e.Stage, OK: e.OK}
		if e.MS != nil {
			ms := float64(*e.MS)
			s.MS = &ms
		}
		steps = append(steps, s)
	}
	if c.Outcome == "failed" && slices.Contains(wayClient.order, c.Stage) {
		stopped = c.Stage
	}
	return wayOf(steps, wayClient, stopped), stopped
}

// wayTypical is one point over many ways: how many passed it, and of those
// with a time the least, the middle and the most, in ms; and how many
// stopped at it, going no further.
type wayTypical struct {
	K       string   `json:"k"`
	Passed  int      `json:"passed"`
	Stopped int      `json:"stopped"`
	MS      *float64 `json:"ms,omitempty"`
	Low     *float64 `json:"low,omitempty"`
	High    *float64 `json:"high,omitempty"`
}

// wayTally adds ways up, a point at a time.
type wayTally struct {
	kind    wayKind
	times   map[string][]float64
	passed  map[string]int
	stopped map[string]int
	n       int
}

func newWayTally(kind wayKind) *wayTally {
	return &wayTally{kind: kind, times: map[string][]float64{}, passed: map[string]int{}, stopped: map[string]int{}}
}

// add adds one way, and the point it stopped at, where it failed. A point
// that went badly in a way that went on, as a gateway that answers nothing
// before a name that is looked up all the same, is neither passed nor
// where it stopped.
func (t *wayTally) add(way []wayPoint, stopped string) {
	t.n++
	for _, p := range way {
		if p.State == "ok" && p.K != stopped {
			t.passed[p.K]++
			if p.MS != nil {
				t.times[p.K] = append(t.times[p.K], *p.MS)
			}
		}
	}
	if stopped != "" {
		t.stopped[stopped]++
	}
}

// typical is what the ways added come to, in the kind's order: every point
// but those left out where nothing was seen of them in any; nothing where
// no way was added.
func (t *wayTally) typical() []wayTypical {
	if t.n == 0 {
		return nil
	}
	out := []wayTypical{}
	for _, k := range t.kind.order {
		p := wayTypical{K: k, Passed: t.passed[k], Stopped: t.stopped[k]}
		if p.Passed == 0 && p.Stopped == 0 && t.kind.optional[k] {
			continue
		}
		if v := t.times[k]; len(v) > 0 {
			slices.Sort(v)
			low, mid, high := v[0], v[len(v)/2], v[len(v)-1]
			p.Low, p.MS, p.High = &low, &mid, &high
		}
		out = append(out, p)
	}
	return out
}

// withWay gives a client test's action back with its way among what it
// found. The way is worked out as it is read, from what was kept, so a test
// from before this has one too; any other action, or a test with nothing
// found yet, comes back as it is.
func withWay(a conditions.Action) conditions.Action {
	if a.Kind != "client-test" || len(a.Detail) == 0 {
		return a
	}
	var d clientTest
	if json.Unmarshal(a.Detail, &d) != nil {
		return a
	}
	d.Way, _ = testWay(&d)
	if raw, err := json.Marshal(d); err == nil {
		a.Detail = raw
	}
	return a
}
