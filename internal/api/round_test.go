package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// A round of tests asked by hand (0129): every AP below the node tests each
// thing it can, once, at once, whatever its schedule or none; over the wire
// too, whether a folder asks for that on a schedule or not.
func TestAssuranceRound(t *testing.T) {
	f := newFixture(t)
	change := func(body map[string]any) {
		t.Helper()
		if code, b := f.change("griff", body); code != 200 {
			t.Fatalf("%v: %d %v", body["kind"], code, b)
		}
	}
	change(map[string]any{"kind": "add-builtins"})
	change(map[string]any{"kind": "add-folder", "tree": "locations", "node": "shed", "name": "Shed", "parent": "house"})
	change(map[string]any{"kind": "grant", "account": "tenant", "tree": "locations", "node": "office", "role": "viewer"})
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	plain := strings.Split(f.enrollAt("Plain", "02:00:00:00:00:6b", "office", []any{serving}, map[string]int{"5g": 44}), "|")
	old := strings.Split(f.enrollAt("Old", "02:00:00:00:00:6c", "office", []any{serving}, map[string]int{"5g": 149}), "|")
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
	// Plain's agent can test over the wire; Old's cannot.
	say(plain[1], map[string]any{"wire_tests": true})
	say(old[1], nil)
	round := func(as, under string) (int, map[string]any) {
		return f.do("POST", "/v1/assurance/round", as, map[string]any{"under": under})
	}

	// Who may ask, and where.
	if code, b := round("office", "office"); code != 404 {
		t.Fatalf("one with no right to the folder: %d %v", code, b)
	}
	if code, b := round("tenant", "office"); code != 403 {
		t.Fatalf("a viewer: %d %v", code, b)
	}
	if code, b := round("griff", "no-such-folder"); code != 404 {
		t.Fatalf("no such folder: %d %v", code, b)
	}
	if code, b := round("griff", "shed"); code != 400 || !strings.Contains(b["error"].(string), "no AP here can test") {
		t.Fatalf("a folder with no AP that can test: %d %v", code, b)
	}
	if code, b := round("griff", old[0]); code != 400 {
		t.Fatalf("an AP that cannot test: %d %v", code, b)
	}
	// Nothing is on a schedule, and nothing is asked until a round is.
	if got := f.api.assure(f.now()); len(got) != 0 {
		t.Fatalf("asked with no schedule and no round: %v", got)
	}
	code, body := round("griff", "house")
	if code != 200 || body["things"] != float64(1) || body["round"].(map[string]any)["by"] != "griff" || body["round"].(map[string]any)["node"] != "house" {
		t.Fatalf("a round: %d %v", code, body)
	}
	progress := func() string {
		t.Helper()
		_, view := f.do("GET", "/v1/assurance?under=office", "griff", nil)
		r, ok := view["round"].(map[string]any)
		if !ok {
			return "none"
		}
		return fmt.Sprintf("%v by %v: %v asked, %v done of %v", r["name"], r["by"], r["asked"], r["done"], r["of"])
	}
	if got := progress(); got != "House by griff: 0 asked, 0 done of 1" {
		t.Fatalf("a round just asked: %s", got)
	}
	// One who may view the office and not the house sees the round, and is
	// not told the name of the folder it was asked at.
	_, part := f.do("GET", "/v1/assurance?under=office", "tenant", nil)
	if r, ok := part["round"].(map[string]any); !ok || r["node"] != "house" || r["name"] != nil || r["of"] != float64(1) {
		t.Fatalf("a round, to one who may not view where it was asked: %v", part["round"])
	}

	// The AP that can is asked, over the wire, though no folder asks for
	// that on a schedule; and no second while the first is open.
	got := f.api.assure(f.now())
	if len(got) != 1 || got[0].AP != plain[0] || got[0].Actor != "aeolus" {
		t.Fatalf("asked %v", got)
	}
	var p clientTestAsk
	json.Unmarshal(got[0].Params, &p)
	if p.Network != "sweet" || p.Band != "wire" || p.On != plain[0] {
		t.Fatalf("asked for %s", got[0].Params)
	}
	if again := f.api.assure(f.now()); len(again) != 0 {
		t.Fatalf("asked again while one is open: %v", again)
	}
	if got := progress(); got != "House by griff: 1 asked, 0 done of 1" {
		t.Fatalf("a round under way: %s", got)
	}
	f.apDo("GET", "/v1/ap/actions", plain[1], nil, nil)
	d := map[string]any{"v": 1, "ssid": "Sweet Spot", "band": "wire", "client": "0e:00:00:6b:00:20", "outcome": "passed", "stage": "done", "reason": "",
		"wire": map[string]any{"device": "eth0", "vlan": 20, "tagged": true}, "steps": []any{}}
	if code, _, b := f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%d", got[0].ID), plain[1], map[string]any{"ok": true, "result": "passed", "detail": d}, nil); code != 200 {
		t.Fatalf("done: %d %v", code, b)
	}
	// Each thing once: the round is done, and nothing more is asked.
	if again := f.api.assure(f.now()); len(again) != 0 {
		t.Fatalf("asked again after the round: %v", again)
	}
	if got := progress(); got != "House by griff: 1 asked, 1 done of 1" {
		t.Fatalf("a round done: %s", got)
	}
	_, view := f.do("GET", "/v1/assurance?under=office", "griff", nil)
	if ch := view["checks"].([]any); len(ch) != 1 || ch[0].(map[string]any)["band"] != "wire" || ch[0].(map[string]any)["passed"] != float64(1) {
		t.Fatalf("what the round tested = %v", view["checks"])
	}

	// Another round, of the AP alone, asks it again; one left for more than
	// an hour, as of an AP that did not call in, is let go.
	if code, b := round("griff", plain[0]); code != 200 {
		t.Fatalf("a round of one AP: %d %v", code, b)
	}
	say(plain[1], map[string]any{"wire_tests": true})
	if late := f.api.assure(f.now().Add(roundLife + time.Minute)); len(late) != 0 {
		t.Fatalf("asked for a round an hour old: %v", late)
	}
	if again := f.api.assure(f.now()); len(again) != 1 || again[0].AP != plain[0] {
		t.Fatalf("a second round: %v", again)
	}
}

// A test on a schedule is asked when its wait is up to within a few
// seconds: the manager looks once a minute, and a look a moment short of
// the wait would else put it off a minute every time.
func TestAssuranceAsksOnTime(t *testing.T) {
	f := newFixture(t)
	change := func(body map[string]any) {
		t.Helper()
		if code, b := f.change("griff", body); code != 200 {
			t.Fatalf("%v: %d %v", body["kind"], code, b)
		}
	}
	change(map[string]any{"kind": "add-builtins"})
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	plain := strings.Split(f.enrollAt("Plain", "02:00:00:00:00:7b", "office", []any{serving}, map[string]int{"5g": 44}), "|")
	change(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.every", "value": 15})
	change(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.wire", "value": true})
	_, _, cfg := f.apDo("GET", "/v1/ap/config", plain[1], nil, nil)
	if code, _, b := f.apDo("POST", "/v1/ap/state", plain[1], map[string]any{"version": cfg["version"], "uptime": 60, "wire_tests": true}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, b)
	}
	start := f.now()
	got := f.api.assure(start)
	if len(got) != 1 {
		t.Fatalf("asked %v", got)
	}
	f.apDo("GET", "/v1/ap/actions", plain[1], nil, nil)
	d := map[string]any{"v": 1, "ssid": "Sweet Spot", "band": "wire", "client": "0e:00:00:7b:00:20", "outcome": "passed", "stage": "done", "reason": "", "steps": []any{}}
	if code, _, b := f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%d", got[0].ID), plain[1], map[string]any{"ok": true, "result": "passed", "detail": d}, nil); code != 200 {
		t.Fatalf("done: %d %v", code, b)
	}
	// The AP calls in again a quarter of an hour on, less a little.
	at := func(short time.Duration) int {
		t.Helper()
		*f.skew = 15*time.Minute - short
		if code, _, b := f.apDo("POST", "/v1/ap/state", plain[1], map[string]any{"version": cfg["version"], "uptime": 900, "wire_tests": true}, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
		return len(f.api.assure(start.Add(*f.skew)))
	}
	if early := at(30 * time.Second); early != 0 {
		t.Fatalf("asked half a minute early: %d", early)
	}
	if due := at(2 * time.Second); due != 1 {
		t.Fatalf("not asked a moment short of its wait: %d", due)
	}
}

// A round beside a schedule (0129): a thing of the round that cannot be
// asked does not hold the schedule up; and the round's count of what is
// done is of its own tests, whatever the schedule asks after.
func TestAssuranceRoundBesideASchedule(t *testing.T) {
	f := newFixture(t)
	change := func(body map[string]any) {
		t.Helper()
		if code, b := f.change("griff", body); code != 200 {
			t.Fatalf("%v: %d %v", body["kind"], code, b)
		}
	}
	change(map[string]any{"kind": "add-builtins"})
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	scanner := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:8a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	plain := strings.Split(f.enrollAt("Plain", "02:00:00:00:00:8b", "office", []any{serving}, map[string]int{"5g": 44}), "|")
	// The scanner hears Plain well; Plain has not said which BSSIDs are its
	// own, so a test cannot be kept to it.
	say := func() {
		t.Helper()
		for _, tok := range []string{plain[1], scanner[1]} {
			_, _, cfg := f.apDo("GET", "/v1/ap/config", tok, nil, nil)
			body := map[string]any{"version": cfg["version"], "uptime": 60}
			if tok == scanner[1] {
				body["bssids"] = []string{"02:00:00:00:00:8a"}
				body["rrm"] = map[string]any{"address": "192.0.2.1", "advertised": []string{"5g"}, "ratings": []any{}, "moves": []any{},
					"neighbours": []any{map[string]any{"ap": plain[0], "address": "192.0.2.9", "state": "up", "chosen": true, "hello_ago": 3,
						"bands": []any{map[string]any{"band": "5g", "signal": -50, "their_signal": -50, "channel": 44, "width": 40}}}}}
			}
			if code, _, b := f.apDo("POST", "/v1/ap/state", tok, body, nil); code != 200 {
				t.Fatalf("state: %d %v", code, b)
			}
		}
	}
	say()
	change(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.every", "value": 15})
	say()
	pass := func(id int64) {
		t.Helper()
		f.apDo("GET", "/v1/ap/actions", scanner[1], nil, nil)
		d := map[string]any{"v": 1, "ssid": "Sweet Spot", "band": "5g", "bssid": "02:00:00:00:00:8a", "channel": 36, "client": "02:00:00:00:00:fe",
			"outcome": "passed", "stage": "done", "reason": "", "steps": []any{}}
		if code, _, b := f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%d", id), scanner[1], map[string]any{"ok": true, "result": "passed", "detail": d}, nil); code != 200 {
			t.Fatalf("done: %d %v", code, b)
		}
	}
	progress := func(as string) string {
		t.Helper()
		_, view := f.do("GET", "/v1/assurance?under=office", as, nil)
		r, ok := view["round"].(map[string]any)
		if !ok {
			return "none"
		}
		return fmt.Sprintf("by %v: %v asked, %v done of %v", r["by"], r["asked"], r["done"], r["of"])
	}

	// A round of two things: on itself, which is asked, and on Plain, which
	// cannot be.
	if code, b := f.do("POST", "/v1/assurance/round", "griff", map[string]any{"under": "house"}); code != 200 || b["things"] != float64(2) {
		t.Fatalf("a round: %d %v", code, b)
	}
	got := f.api.assure(f.now())
	if len(got) != 1 || got[0].AP != scanner[0] {
		t.Fatalf("asked %v", got)
	}
	pass(got[0].ID)
	if got := progress("griff"); got != "by griff: 1 asked, 1 done of 2" {
		t.Fatalf("the round, one thing done: %s", got)
	}
	// What is left of the round cannot be asked, and the schedule's own
	// wait is not up: nothing.
	if again := f.api.assure(f.now()); len(again) != 0 {
		t.Fatalf("asked what cannot be: %v", again)
	}
	// A quarter of an hour on the schedule has its turn, round or no round.
	*f.skew = 16 * time.Minute
	say()
	turn := f.api.assure(f.now())
	if len(turn) != 1 || turn[0].AP != scanner[0] {
		t.Fatalf("the schedule, held up by a round: %v", turn)
	}
	// That test is open, and is the schedule's: the round has what it had.
	if got := progress("griff"); got != "by griff: 1 asked, 1 done of 2" {
		t.Fatalf("the round, with the schedule's test open: %s", got)
	}
	// Who asked a round is said to one who may view where it was asked,
	// and to no other.
	change(map[string]any{"kind": "grant", "account": "tenant", "tree": "locations", "node": "office", "role": "viewer"})
	if got := progress("tenant"); got != "by <nil>: 1 asked, 1 done of 2" {
		t.Fatalf("the round, to one who may view only a folder below: %s", got)
	}
}
