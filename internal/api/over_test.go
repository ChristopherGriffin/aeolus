package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Which tests (0131): a folder says whether its APs are asked for tests
// through the air, over the wire, or both, on a schedule; and a round asked
// by hand says the same for itself. The setting before, over the wire too,
// says both where which is not said.
func TestWhichTests(t *testing.T) {
	f := newFixture(t)
	change := func(op map[string]any) (int, map[string]any) {
		t.Helper()
		return f.change("griff", op)
	}
	must := func(op map[string]any) {
		t.Helper()
		if code, b := change(op); code != 200 {
			t.Fatalf("%v: %d %v", op, code, b)
		}
	}
	must(map[string]any{"kind": "add-builtins"})
	must(map[string]any{"kind": "add-folder", "tree": "locations", "node": "annex", "name": "Annex", "parent": "office"})
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	// Three APs: one with a scan radio whose agent can test over the wire;
	// one with no scan radio, and the wire, in a folder of its own below;
	// and one with a scan radio whose agent cannot test over the wire.
	scanner := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:7a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	plain := strings.Split(f.enrollAt("Plain", "02:00:00:00:00:7b", "annex", []any{serving}, map[string]int{"5g": 44}), "|")
	deaf := strings.Split(f.enrollAt("Deaf", "02:00:00:00:00:7c", "office", []any{serving, spare}, map[string]int{"5g": 149}), "|")
	names := map[string]string{scanner[0]: "Scanner", plain[0]: "Plain", deaf[0]: "Deaf"}
	tokens := map[string]string{scanner[0]: scanner[1], plain[0]: plain[1], deaf[0]: deaf[1]}
	say := func(tok string, more map[string]any) {
		t.Helper()
		_, _, cfg := f.apDo("GET", "/v1/ap/config", tok, nil, nil)
		body := map[string]any{"version": cfg["version"], "uptime": 60}
		for k, v := range more {
			body[k] = v
		}
		if code, _, b := f.apDo("POST", "/v1/ap/state", tok, body, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
	}
	// The scanner hears the plain AP well: it tests the network on itself
	// and on it, through the air. The other with a scan radio, on itself.
	hears := map[string]any{"ap": plain[0], "address": "192.0.2.9", "state": "up", "chosen": true, "hello_ago": 3,
		"bands": []any{map[string]any{"band": "5g", "signal": -50, "their_signal": -50, "channel": 44, "width": 40}}}
	report := func() {
		t.Helper()
		say(plain[1], map[string]any{"wire_tests": true, "bssids": []string{"02:00:00:00:00:7b"}})
		say(deaf[1], map[string]any{"bssids": []string{"02:00:00:00:00:7c"}})
		say(scanner[1], map[string]any{"wire_tests": true, "bssids": []string{"02:00:00:00:00:7a"}, "rrm": map[string]any{"address": "192.0.2.1", "advertised": []string{"5g"},
			"neighbours": []any{hears}, "ratings": []any{}, "moves": []any{}}})
	}
	report()
	must(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.every", "value": 15})

	// What each AP is asked for on the schedule, as the tab says it: which
	// tests, whether it is asked on a schedule at all, and the bands of the
	// things it is asked for.
	listed := func() string {
		t.Helper()
		_, view := f.do("GET", "/v1/assurance", "griff", nil)
		var out []string
		for _, x := range view["testers"].([]any) {
			tr := x.(map[string]any)
			var bands []string
			for _, c := range view["checks"].([]any) {
				if ch := c.(map[string]any); ch["tester"] == tr["ap"] && ch["scheduled"] == true {
					bands = append(bands, ch["band"].(string))
				}
			}
			out = append(out, strings.TrimSpace(strings.Join([]string{tr["name"].(string), tr["over"].(string),
				map[bool]string{true: "scheduled", false: "idle"}[tr["every"] != float64(0)], strings.Join(bands, "+")}, " ")))
		}
		sort.Strings(out)
		return strings.Join(out, "; ")
	}
	// turn is one turn of the manager's asking: what it asks, each taken up
	// and done so the next can be asked. It returns what was asked, an AP
	// and a band each, those of a round asked by hand marked so.
	turn := func() []string {
		t.Helper()
		*f.skew += 16 * time.Minute
		report()
		var out []string
		for _, a := range f.api.assure(f.now()) {
			var p clientTestAsk
			json.Unmarshal(a.Params, &p)
			said := names[string(a.AP)] + " " + p.Band
			if p.Round {
				said += " (round)"
			}
			out = append(out, said)
			tok := tokens[string(a.AP)]
			f.apDo("GET", "/v1/ap/actions", tok, nil, nil)
			d := map[string]any{"v": 1, "ssid": "Sweet Spot", "band": p.Band, "client": "02:00:00:00:0a:f0", "outcome": "passed", "stage": "done", "reason": "", "steps": []any{}}
			if code, _, b := f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%d", a.ID), tok, map[string]any{"ok": true, "result": "passed", "detail": d}, nil); code != 200 {
				t.Fatalf("done: %d %v", code, b)
			}
		}
		return out
	}
	// over is what a few turns ask, each thing said once.
	over := func(turns int) string {
		t.Helper()
		seen := map[string]bool{}
		for i := 0; i < turns; i++ {
			for _, s := range turn() {
				seen[s] = true
			}
		}
		var out []string
		for s := range seen {
			out = append(out, s)
		}
		sort.Strings(out)
		return strings.Join(out, ", ")
	}
	set := func(path string, v any) {
		t.Helper()
		must(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": path, "value": v})
		report()
	}
	unset := func(path string) {
		t.Helper()
		must(map[string]any{"kind": "unset", "tree": "locations", "node": "office", "path": path})
		report()
	}
	const (
		airListed  = "Deaf air scheduled 5g; Plain air idle; Scanner air scheduled 5g+5g"
		airAsked   = "Deaf 5g, Scanner 5g"
		bothListed = "Deaf both scheduled 5g; Plain both scheduled wire; Scanner both scheduled 5g+5g+wire"
		bothAsked  = "Deaf 5g, Plain wire, Scanner 5g, Scanner wire"
	)
	for _, s := range []struct {
		name          string
		do            func()
		listed, asked string
	}{
		{"nothing said: through the air, by the APs with a scan radio", func() {}, airListed, airAsked},
		{"the setting before, over the wire too: both", func() { set("assurance.tests.wire", true) }, bothListed, bothAsked},
		{"through the air said, which the setting before does not undo", func() { set("assurance.tests.over", "air") }, airListed, airAsked},
		// Over the wire the scan radios are left alone; and an AP with one
		// whose agent cannot test over the wire is asked for nothing, though
		// it is listed still, as one that can test when asked.
		{"over the wire", func() { set("assurance.tests.over", "wire") },
			"Deaf wire idle; Plain wire scheduled wire; Scanner wire scheduled wire", "Plain wire, Scanner wire"},
		{"both", func() { set("assurance.tests.over", "both") }, bothListed, bothAsked},
		{"the setting before taken away, and both still said", func() { unset("assurance.tests.wire") }, bothListed, bothAsked},
	} {
		s.do()
		if got := listed(); got != s.listed {
			t.Fatalf("%s, as the tab lists it:\n got %s\nwant %s", s.name, got, s.listed)
		}
		if got := over(4); got != s.asked {
			t.Fatalf("%s, as the manager asks:\n got %s\nwant %s", s.name, got, s.asked)
		}
	}
	if code, b := change(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.over", "value": "radio"}); code == 200 {
		t.Fatalf("tests of no kind were set: %v", b)
	}

	// A round says which tests for itself, whatever the folder asks for on
	// its schedule: here, nothing, so that what is asked is the round's.
	set("assurance.tests.every", 0)
	round := func(under, over string) (int, map[string]any) {
		body := map[string]any{"under": under}
		if over != "" {
			body["over"] = over
		}
		return f.do("POST", "/v1/assurance/round", "griff", body)
	}
	fresh := func() { f.api.rounds.at = nil }
	for over, things := range map[string]float64{"": 5, "both": 5, "air": 3, "wire": 2} {
		fresh()
		code, b := round("office", over)
		want := over
		if want == "" {
			want = "both"
		}
		if code != 200 || b["things"] != things || b["round"].(map[string]any)["over"] != want {
			t.Fatalf("a round over %q: %d %v, want %v things", over, code, b, things)
		}
	}
	fresh()
	if code, b := round("office", "radio"); code != 400 || !strings.Contains(b["error"].(string), "air, wire or both") {
		t.Fatalf("a round of no kind: %d %v", code, b)
	}
	if code, b := round(plain[0], "air"); code != 400 || !strings.Contains(b["error"].(string), "through the air") {
		t.Fatalf("a round through the air of an AP with no scan radio: %d %v", code, b)
	}
	said := func(under string) string {
		t.Helper()
		_, view := f.do("GET", "/v1/assurance?under="+under, "griff", nil)
		r, ok := view["round"].(map[string]any)
		if !ok {
			return "none"
		}
		return fmt.Sprintf("%v at %v: %v of %v", r["over"], r["node"], r["done"], r["of"])
	}

	// A round through the air: the APs with a scan radio are asked for
	// their tests through the air, and the one without for nothing. Seen
	// from that one alone, there is no round to speak of.
	if code, b := round("office", "air"); code != 200 {
		t.Fatalf("a round through the air: %d %v", code, b)
	}
	if got, want := said(plain[0]), "none"; got != want {
		t.Errorf("a round through the air, at an AP with no scan radio = %s, want %s", got, want)
	}
	if got, want := over(3), "Deaf 5g (round), Scanner 5g (round)"; got != want {
		t.Errorf("a round through the air asked %s, want %s", got, want)
	}
	if got, want := said("office"), "air at office: 3 of 3"; got != want {
		t.Errorf("the round through the air, as the tab says it = %s, want %s", got, want)
	}

	// A round over the wire, and then one through the air at the same
	// folder while it is worked at: the first goes on. Each kind is worked
	// at from when it was asked for, and the round is of both.
	fresh()
	round("office", "wire")
	if code, b := round("office", "air"); code != 200 || b["things"] != float64(3) || b["round"].(map[string]any)["over"] != "both" {
		t.Fatalf("a round through the air, after one over the wire: %d %v", code, b)
	}
	if got, want := over(4), "Deaf 5g (round), Plain wire (round), Scanner 5g (round), Scanner wire (round)"; got != want {
		t.Errorf("the two rounds asked %s, want %s", got, want)
	}
	if got, want := said("office"), "both at office: 5 of 5"; got != want {
		t.Errorf("the two rounds, as the tab says them = %s, want %s", got, want)
	}

	// Each kind from its own time: a round over the wire, done; and then
	// one through the air, which asks for nothing over the wire again.
	fresh()
	round("office", "wire")
	if got, want := over(2), "Plain wire (round), Scanner wire (round)"; got != want {
		t.Errorf("a round over the wire asked %s, want %s", got, want)
	}
	round("office", "air")
	if got, want := said("office"), "both at office: 2 of 5"; got != want {
		t.Errorf("a round through the air, after one over the wire was done = %s, want %s", got, want)
	}
	if got, want := over(3), "Deaf 5g (round), Scanner 5g (round)"; got != want {
		t.Errorf("a round through the air, after one over the wire was done, asked %s, want %s", got, want)
	}
	// And the other way about: the air first, and then the wire.
	fresh()
	round("office", "air")
	if got, want := over(3), "Deaf 5g (round), Scanner 5g (round)"; got != want {
		t.Errorf("a round through the air asked %s, want %s", got, want)
	}
	if code, b := round("office", "wire"); code != 200 || b["round"].(map[string]any)["over"] != "both" {
		t.Fatalf("a round over the wire, after one through the air: %d %v", code, b)
	}
	if got, want := over(2), "Plain wire (round), Scanner wire (round)"; got != want {
		t.Errorf("a round over the wire, after one through the air was done, asked %s, want %s", got, want)
	}
	if got, want := said("office"), "both at office: 5 of 5"; got != want {
		t.Errorf("the two rounds, one after the other = %s, want %s", got, want)
	}

	// What is said of a round at a folder is of what it asks of the APs
	// there, and of where that was asked: below a round through the air, a
	// folder whose AP has no scan radio, asked for one over the wire, is in
	// a round over the wire, asked at itself, whichever was asked last.
	fresh()
	round("office", "air")
	round("annex", "wire")
	if got, want := said("annex"), "wire at annex: 0 of 1"; got != want {
		t.Errorf("a round over the wire, below one through the air = %s, want %s", got, want)
	}
	fresh()
	round("annex", "wire")
	round("office", "air")
	if got, want := said("annex"), "wire at annex: 0 of 1"; got != want {
		t.Errorf("a round over the wire, below a later one through the air = %s, want %s", got, want)
	}

	// And the same where the second is asked at an AP below: that AP is in
	// both, and the other below the folder still in the first.
	fresh()
	round("office", "wire")
	if code, b := round(scanner[0], "air"); code != 200 || b["things"] != float64(2) {
		t.Fatalf("a round through the air at an AP, below one over the wire: %d %v", code, b)
	}
	if got, want := over(4), "Plain wire (round), Scanner 5g (round), Scanner wire (round)"; got != want {
		t.Errorf("a round at the folder and one at an AP below asked %s, want %s", got, want)
	}
}

// A round is worked at for an hour, each kind from when it was asked for
// (0129, 0131): after that the kind is let go, and the other goes on; and a
// kind let go is not taken up again by a later asking for the other.
func TestRoundsAreLetGo(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	ap := strings.Split(f.enrollAt("Plain", "02:00:00:00:00:7b", "office", []any{serving}, map[string]int{"5g": 44}), "|")[0]
	tree := f.log.Snapshot().Org.Locations
	var r rounds
	t0 := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	in := func(after time.Duration) string {
		air, wire, ok := r.of(tree, hierarchy.NodeID(ap), t0.Add(after))
		if !ok {
			return "none"
		}
		return roundsOver(air, wire)
	}
	r.ask("office", "griff", t0, overWire)
	r.ask("office", "griff", t0.Add(50*time.Minute), overAir)
	for after, want := range map[time.Duration]string{
		51 * time.Minute:  overBoth,
		60 * time.Minute:  overBoth,
		61 * time.Minute:  overAir, // the wire's hour is up
		110 * time.Minute: overAir,
		111 * time.Minute: "none",
	} {
		if got := in(after); got != want {
			t.Errorf("%v after the first: in %s, want %s", after, got, want)
		}
	}
	// The wire, let go at 60 minutes, is not taken up again by the air
	// asked for once more at 70.
	if one := r.ask("office", "griff", t0.Add(70*time.Minute), overAir); one.Over != overAir {
		t.Errorf("a round through the air asked again, the wire's hour up: of %s", one.Over)
	}
	if got := in(71 * time.Minute); got != overAir {
		t.Errorf("after that: in %s", got)
	}
}

// What a thing's last test came to is said for as long as its alert is
// (0123): three rounds of its AP's schedule and an hour, or an hour where
// the AP is asked for none.
func TestVerdictStands(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	for name, c := range map[string]struct {
		ago, round time.Duration
		want       bool
	}{
		"just tested":                         {time.Minute, time.Hour, true},
		"three rounds and an hour ago":        {4 * time.Hour, time.Hour, true},
		"longer than that":                    {4*time.Hour + time.Minute, time.Hour, false},
		"asked for none, within the hour":     {59 * time.Minute, 0, true},
		"asked for none, an hour and more":    {61 * time.Minute, 0, false},
		"a test by a clock ahead of this one": {-time.Minute, 0, true},
	} {
		if got := verdictStands(now, now.Add(-c.ago), c.round); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
}

// A thing that was failing, and is tested no longer, is listed still, and
// is said to be failing only for as long as its alert is (0123, 0131): an
// hour after its last test, where its AP is asked for nothing now.
func TestFailingIsSaidAsLongAsItsAlert(t *testing.T) {
	f := newFixture(t)
	change := func(op map[string]any) {
		t.Helper()
		if code, b := f.change("griff", op); code != 200 {
			t.Fatalf("%v: %d %v", op, code, b)
		}
	}
	change(map[string]any{"kind": "add-builtins"})
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	scanner := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:7a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	report := func() {
		t.Helper()
		_, _, cfg := f.apDo("GET", "/v1/ap/config", scanner[1], nil, nil)
		if code, _, b := f.apDo("POST", "/v1/ap/state", scanner[1], map[string]any{"version": cfg["version"], "uptime": 60, "bssids": []string{"02:00:00:00:00:7a"}}, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
	}
	change(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.every", "value": 15})
	// Its one thing, tested an hour and a half ago by the manager's clock,
	// fails, is tried again within minutes, and fails again.
	*f.skew = -90 * time.Minute
	for i := 0; i < 2; i++ {
		report()
		got := f.api.assure(f.now())
		if len(got) != 1 {
			t.Fatalf("test %d asked: %v", i, got)
		}
		f.apDo("GET", "/v1/ap/actions", scanner[1], nil, nil)
		d := map[string]any{"v": 1, "ssid": "Sweet Spot", "band": "5g", "bssid": "02:00:00:00:00:7a", "client": "02:00:00:00:0a:f0",
			"outcome": "failed", "stage": "dhcp", "reason": "no DHCP server gave it an address", "steps": []any{}}
		if code, _, b := f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%d", got[0].ID), scanner[1], map[string]any{"ok": false, "result": "failed at dhcp", "detail": d}, nil); code != 200 {
			t.Fatalf("done: %d %v", code, b)
		}
		*f.skew += assureRetry
	}
	*f.skew = 0
	said := func() string {
		t.Helper()
		_, view := f.do("GET", "/v1/assurance", "griff", nil)
		checks := view["checks"].([]any)
		if len(checks) != 1 {
			t.Fatalf("checks = %v", checks)
		}
		ch := checks[0].(map[string]any)
		return fmt.Sprintf("failing %v, scheduled %v, failed %v", ch["failing"], ch["scheduled"], ch["failed"])
	}
	// On its schedule still, a quarter of an hour a thing: its verdict
	// stands for three of those and an hour.
	if got, want := said(), "failing true, scheduled true, failed 2"; got != want {
		t.Errorf("an hour and a half after, tested still: %s, want %s", got, want)
	}
	// Asked for the wire alone, which its agent cannot make, it is tested
	// no longer: its verdict stands for an hour, and that is up.
	change(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.over", "value": "wire"})
	if got, want := said(), "failing false, scheduled false, failed 2"; got != want {
		t.Errorf("an hour and a half after, tested no longer: %s, want %s", got, want)
	}
}
