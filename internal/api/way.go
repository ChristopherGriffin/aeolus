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
// time is their answer's own, as the step says it; those with no time but
// that one, where the time between two steps says nothing, as of a
// gateway pinged three times a second apart; those done at their first
// success, as a first name looked up is, where the others are done at
// their last step; and those left out where nothing was seen of them.
type wayKind struct {
	order    []string
	own      map[string]bool
	ownOnly  map[string]bool
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
		ownOnly:   set("gateway", "dns", "speed"),
		optional:  set("signin", "speed"),
		fromStart: true,
	}
	// A test over the wire (0128): an address offered, then given; the
	// gateway; a name; and the address given back. An agent from before
	// 0130 says nothing of the offer, and its address is one point.
	wayWire = wayKind{
		order:     []string{"offer", "dhcp", "gateway", "dns", "release"},
		own:       set("gateway", "dns"),
		ownOnly:   set("gateway", "dns", "release"),
		optional:  set("offer", "release"),
		fromStart: true,
	}
	// A client's own attempt (0118), as its AP saw it.
	wayClient = wayKind{
		order:    connStages,
		own:      set("gateway", "dns", "internet"),
		first:    set("gateway", "dns", "internet"),
		optional: set("signin"),
	}
)

// wayOf reads the way from its steps, in the order they came. failed is
// the point the whole stopped at, where it failed: that point is bad though
// no step of it says so, as of an address asked for and never answered.
//
// How long a point took is, the first of these that applies: its answer's
// own time, where its kind takes that and the step says it; from its first
// step to its end, where it began with a step that only asks; from the
// point before it to its end.
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
		if k == failed {
			p.State = "bad"
		}
		if len(mine) == 0 {
			if !kind.optional[k] || p.State == "bad" {
				out = append(out, p)
			}
			continue
		}
		// Where it was done: at its first success, or at its last step.
		end, won := mine[len(mine)-1], false
		for _, s := range mine {
			if s.OK == nil {
				continue
			}
			if !*s.OK {
				p.State = "bad"
			} else if !won {
				won = true
				if kind.first[k] {
					end = s
				}
			}
		}
		switch {
		case p.State == "bad":
		case won:
			p.State = "ok"
		default:
			p.State = "begun"
		}
		// A point begun and not seen through has no time: it took none yet.
		var ms float64 = -1
		switch {
		case p.State == "begun":
		case kind.own[k] && p.State == "ok" && end.MS != nil:
			ms = *end.MS
		case kind.ownOnly[k]:
		case mine[0].OK == nil && len(mine) > 1 && end.T >= mine[0].T:
			ms = float64(end.T - mine[0].T)
		case begun && end.T >= before:
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

// testWay is a client test's way. A step belongs to the point it names,
// or to its stage.
func testWay(d *clientTest) []wayPoint {
	wire := d.Band == wireBand
	steps := make([]wayStep, len(d.Steps))
	past := false
	for i, s := range d.Steps {
		k := s.K
		if k == "" {
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
	failed := ""
	if d.Outcome == "failed" {
		failed = d.Stage
	}
	kind := wayAir
	if wire {
		kind = wayWire
		// It stopped at DHCP: before an offer, where it was asking for
		// one and none came.
		if failed == "dhcp" && slices.ContainsFunc(steps, func(s wayStep) bool { return s.K == "offer" && s.OK != nil && !*s.OK }) {
			failed = "offer"
		}
	}
	way := wayOf(steps, kind, failed)
	for i := range way {
		p := &way[i]
		if p.State != "ok" || p.MS != nil {
			continue
		}
		// What the test measured, where its steps do not say: all a
		// manager kept of a test from before its steps were.
		switch {
		case p.K == "dhcp" && d.DHCP != nil && kind.own["dhcp"]:
			ms := float64(d.DHCP.MS)
			p.MS = &ms
		case p.K == "gateway" && d.Gateway != nil && d.Gateway.Avg != nil:
			p.MS = d.Gateway.Avg
		case p.K == "gateway" && d.Gateway != nil:
			p.MS = d.Gateway.ARPMS
		case p.K == "dns" && d.DNS != nil && d.DNS.MS != nil:
			ms := float64(*d.DNS.MS)
			p.MS = &ms
		}
	}
	return way
}

// connWay is the way of a client's own attempt, from its AP's record of it.
func connWay(c conditions.Connection) []wayPoint {
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
	failed := ""
	if c.Outcome == "failed" {
		failed = c.Stage
	}
	return wayOf(steps, wayClient, failed)
}

// wayTypical is one point over many ways: how many passed it, and of those
// with a time the least, the middle and the most, in ms; and how many
// stopped at it.
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

func (t *wayTally) add(way []wayPoint) {
	t.n++
	for _, p := range way {
		switch p.State {
		case "ok":
			t.passed[p.K]++
			if p.MS != nil {
				t.times[p.K] = append(t.times[p.K], *p.MS)
			}
		case "bad":
			t.stopped[p.K]++
		}
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
	d.Way = testWay(&d)
	if raw, err := json.Marshal(d); err == nil {
		a.Detail = raw
	}
	return a
}
