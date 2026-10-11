package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"
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
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	scanner := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:7a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	plain := strings.Split(f.enrollAt("Plain", "02:00:00:00:00:7b", "office", []any{serving}, map[string]int{"5g": 44}), "|")
	say := func(tok string, more map[string]any) {
		t.Helper()
		_, _, cfg := f.apDo("GET", "/v1/ap/config", tok, nil, nil)
		body := map[string]any{"version": cfg["version"], "uptime": 60, "wire_tests": true}
		for k, v := range more {
			body[k] = v
		}
		if code, _, b := f.apDo("POST", "/v1/ap/state", tok, body, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
	}
	// The scanner hears the plain AP well: it tests the network on itself
	// and on it, through the air. Both can test over the wire.
	hears := map[string]any{"ap": plain[0], "address": "192.0.2.9", "state": "up", "chosen": true, "hello_ago": 3,
		"bands": []any{map[string]any{"band": "5g", "signal": -50, "their_signal": -50, "channel": 44, "width": 40}}}
	report := func() {
		t.Helper()
		say(plain[1], map[string]any{"bssids": []string{"02:00:00:00:00:7b"}})
		say(scanner[1], map[string]any{"bssids": []string{"02:00:00:00:00:7a"}, "rrm": map[string]any{"address": "192.0.2.1", "advertised": []string{"5g"},
			"neighbours": []any{hears}, "ratings": []any{}, "moves": []any{}}})
	}
	report()
	must(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.every", "value": 15})

	// What each AP is asked for on the schedule, as the tab says it: which
	// tests, how many things, and the bands of those things.
	asked := func() string {
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
	steps := []struct {
		name string
		do   func()
		want string
	}{
		{"nothing said: through the air, by the AP with a scan radio", func() {},
			"Plain air idle; Scanner air scheduled 5g+5g"},
		{"the setting before, over the wire too: both", func() { set("assurance.tests.wire", true) },
			"Plain both scheduled wire; Scanner both scheduled 5g+5g+wire"},
		{"through the air said, which the setting before does not undo", func() { set("assurance.tests.over", "air") },
			"Plain air idle; Scanner air scheduled 5g+5g"},
		{"over the wire: the scan radio is left alone", func() { set("assurance.tests.over", "wire") },
			"Plain wire scheduled wire; Scanner wire scheduled wire"},
		{"both", func() { set("assurance.tests.over", "both") },
			"Plain both scheduled wire; Scanner both scheduled 5g+5g+wire"},
		{"the setting before taken away, and both still said", func() { unset("assurance.tests.wire") },
			"Plain both scheduled wire; Scanner both scheduled 5g+5g+wire"},
	}
	for _, s := range steps {
		s.do()
		if got := asked(); got != s.want {
			t.Fatalf("%s:\n got %s\nwant %s", s.name, got, s.want)
		}
	}
	if code, b := change(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.over", "value": "radio"}); code == 200 {
		t.Fatalf("tests of no kind were set: %v", b)
	}

	// A round says which tests for itself, whatever the folder asks for on
	// its schedule: here, over the wire alone.
	set("assurance.tests.over", "wire")
	round := func(under, over string) (int, map[string]any) {
		body := map[string]any{"under": under}
		if over != "" {
			body["over"] = over
		}
		return f.do("POST", "/v1/assurance/round", "griff", body)
	}
	for over, things := range map[string]float64{"": 4, "both": 4, "air": 2, "wire": 2} {
		code, b := round("office", over)
		want := over
		if want == "" {
			want = "both"
		}
		if code != 200 || b["things"] != things || b["round"].(map[string]any)["over"] != want {
			t.Fatalf("a round over %q: %d %v, want %v things", over, code, b, things)
		}
	}
	if code, b := round("office", "radio"); code != 400 || !strings.Contains(b["error"].(string), "air, wire or both") {
		t.Fatalf("a round of no kind: %d %v", code, b)
	}
	if code, b := round(plain[0], "air"); code != 400 || !strings.Contains(b["error"].(string), "through the air") {
		t.Fatalf("a round through the air of an AP with no scan radio: %d %v", code, b)
	}

	// A round through the air, where the schedule is over the wire: the
	// scanner is asked for its tests through the air, the plain AP for
	// none of the round; and the tab says which tests the round is of, and
	// of how many things.
	if code, b := round("office", "air"); code != 200 {
		t.Fatalf("a round through the air: %d %v", code, b)
	}
	for i := 0; i < 2; i++ {
		*f.skew += time.Minute
		report()
		got := f.api.assure(f.now())
		var mine []string
		for _, a := range got {
			var p clientTestAsk
			json.Unmarshal(a.Params, &p)
			if p.Round {
				mine = append(mine, string(a.AP)+" "+p.Band)
			}
			// Each is taken up and done, so the next can be asked.
			tok := scanner[1]
			if string(a.AP) == plain[0] {
				tok = plain[1]
			}
			f.apDo("GET", "/v1/ap/actions", tok, nil, nil)
			d := map[string]any{"v": 1, "ssid": "Sweet Spot", "band": p.Band, "client": "02:00:00:00:0a:f0", "outcome": "passed", "stage": "done", "reason": "", "steps": []any{}}
			if code, _, b := f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%d", a.ID), tok, map[string]any{"ok": true, "result": "passed", "detail": d}, nil); code != 200 {
				t.Fatalf("done: %d %v", code, b)
			}
		}
		if want := scanner[0] + " 5g"; len(mine) != 1 || mine[0] != want {
			t.Fatalf("turn %d of the round through the air asked %v, want %s", i, mine, want)
		}
	}
	_, view := f.do("GET", "/v1/assurance", "griff", nil)
	if r, _ := view["round"].(map[string]any); r["over"] != "air" || r["of"] != float64(2) || r["done"] != float64(2) {
		t.Fatalf("the round, as the tab says it = %v", view["round"])
	}
}
