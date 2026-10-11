package api

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// What the schedule leaves out, how long a verdict stands, whose alert a
// test of "the strongest AP" is, and what a person with a narrower view
// sees.
func TestAssuranceEdges(t *testing.T) {
	f := newFixture(t)
	change := func(op map[string]any) {
		t.Helper()
		if code, b := f.change("griff", op); code != 200 {
			t.Fatalf("%v: %d %v", op, code, b)
		}
	}
	change(map[string]any{"kind": "add-builtins"})
	// Two more places: an annex the tenant may not view, and a shed where
	// the network is turned off.
	change(map[string]any{"kind": "add-folder", "tree": "locations", "node": "annex", "name": "Annex", "parent": "house"})
	change(map[string]any{"kind": "add-folder", "tree": "locations", "node": "shed", "name": "Shed", "parent": "house"})
	change(map[string]any{"kind": "add-folder", "tree": "services", "node": "quiet", "name": "Quiet", "parent": "household"})
	change(map[string]any{"kind": "set", "tree": "services", "node": "quiet", "path": "network.sweet.enabled", "value": false})
	change(map[string]any{"kind": "assign-services", "node": "shed", "services": []string{"quiet"}})
	change(map[string]any{"kind": "grant", "account": "tenant", "tree": "locations", "node": "office", "role": "viewer"})
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	scanner := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:6a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	annex := strings.Split(f.enrollAt("AnnexAP", "02:00:00:00:00:6b", "annex", []any{serving}, map[string]int{"5g": 44}), "|")
	shed := strings.Split(f.enrollAt("ShedAP", "02:00:00:00:00:6c", "shed", []any{serving}, map[string]int{"5g": 149}), "|")
	change(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.every", "value": 15})
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
	hears := func(ap string) map[string]any {
		return map[string]any{"ap": ap, "address": "192.0.2.9", "state": "up", "chosen": true, "hello_ago": 3,
			"bands": []any{map[string]any{"band": "5g", "signal": -50, "their_signal": -50, "channel": 44, "width": 40}}}
	}
	say(annex[1], map[string]any{"bssids": []string{"02:00:00:00:00:6b"}})
	say(shed[1], map[string]any{"bssids": []string{"02:00:00:00:00:6c"}})
	say(scanner[1], map[string]any{"bssids": []string{"02:00:00:00:00:6a"}, "rrm": map[string]any{"address": "192.0.2.1", "advertised": []string{"5g"},
		"neighbours": []any{hears(annex[0]), hears(shed[0])}, "ratings": []any{}, "moves": []any{}}})

	// The shed's AP is heard as well as the annex's, but the network is
	// turned off there: two things to test, not three.
	_, view := f.do("GET", "/v1/assurance", "griff", nil)
	if ts := view["testers"].([]any); len(ts) != 1 || ts[0].(map[string]any)["things"] != float64(2) {
		t.Fatalf("testers = %v", ts)
	}
	for _, c := range view["checks"].([]any) {
		if c.(map[string]any)["on"] == shed[0] {
			t.Fatalf("the shed's AP is tested: %v", c)
		}
	}

	// A test, taken up by the scanner and answered.
	answer := func(id float64, outcome, bssid string) {
		t.Helper()
		if _, _, b := f.apDo("GET", "/v1/ap/actions", scanner[1], nil, nil); len(b["actions"].([]any)) != 1 {
			t.Fatalf("the AP was handed %v", b)
		}
		d := map[string]any{"v": 1, "ssid": "Sweet Spot", "band": "5g", "bssid": bssid, "channel": 44, "client": "02:00:00:00:00:fe",
			"outcome": outcome, "stage": "done", "reason": "", "steps": []any{}}
		if outcome != "passed" {
			d["stage"], d["reason"] = "dhcp", "no DHCP server gave it an address"
		}
		if code, _, b := f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%d", int64(id)), scanner[1], map[string]any{"ok": outcome == "passed", "result": outcome, "detail": d}, nil); code != 200 {
			t.Fatalf("done: %d %v", code, b)
		}
	}
	byHand := func(body map[string]any, outcome, bssid string) {
		t.Helper()
		body["kind"] = "client-test"
		code, b := f.do("POST", "/v1/aps/"+scanner[0]+"/actions", "griff", body)
		if code != 200 {
			t.Fatalf("asked: %d %v", code, b)
		}
		answer(b["action"].(map[string]any)["id"].(float64), outcome, bssid)
	}
	alerts := func() []map[string]any {
		_, body := f.do("GET", "/v1/alerts", "griff", nil)
		var out []map[string]any
		for _, a := range body["alerts"].([]any) {
			if a.(map[string]any)["kind"] == "client-test" {
				out = append(out, a.(map[string]any))
			}
		}
		return out
	}

	// By hand, on the strongest AP it hears, twice failing: the alert is the
	// AP's whose BSSID it joined, the annex's. One that joined a BSSID no AP
	// here has is nobody's.
	byHand(map[string]any{"network": "sweet", "band": "5g"}, "failed", "02:00:00:00:00:6b")
	byHand(map[string]any{"network": "sweet", "band": "5g"}, "failed", "02:00:00:00:00:6b")
	if got := alerts(); len(got) != 1 || got[0]["ap"] != annex[0] || !strings.Contains(got[0]["message"].(string), "from Scanner") {
		t.Fatalf("alerts for the strongest AP = %v", got)
	}
	byHand(map[string]any{"network": "sweet", "band": "5g"}, "passed", "02:00:00:00:00:6b")
	byHand(map[string]any{"network": "sweet"}, "failed", "02:00:00:00:00:99")
	byHand(map[string]any{"network": "sweet"}, "failed", "02:00:00:00:00:99")
	if got := alerts(); len(got) != 0 {
		t.Fatalf("alerts for a BSSID that is nobody's = %v", got)
	}

	// On the annex's AP by name, twice failing. The tenant, who may view
	// the scanner and not the annex, sees neither the check nor the tests,
	// and in the scanner's own list not where they joined.
	byHand(map[string]any{"network": "sweet", "on": annex[0], "band": "5g"}, "failed", "02:00:00:00:00:6b")
	byHand(map[string]any{"network": "sweet", "on": annex[0], "band": "5g"}, "failed", "02:00:00:00:00:6b")
	if got := alerts(); len(got) != 1 || got[0]["ap"] != annex[0] {
		t.Fatalf("alerts = %v", got)
	}
	_, mine := f.do("GET", "/v1/assurance", "griff", nil)
	_, theirs := f.do("GET", "/v1/assurance", "tenant", nil)
	on := func(v map[string]any, key string) int {
		n := 0
		for _, x := range v[key].([]any) {
			m := x.(map[string]any)
			if p, ok := m["params"].(map[string]any); ok {
				m = p
			}
			if m["on"] == annex[0] {
				n++
			}
		}
		return n
	}
	if on(mine, "checks") != 1 || on(mine, "tests") != 2 || len(theirs["testers"].([]any)) != 1 || on(theirs, "checks") != 0 || on(theirs, "tests") != 0 {
		t.Fatalf("griff sees %d checks and %d tests on the annex; the tenant %v", on(mine, "checks"), on(mine, "tests"), theirs)
	}
	_, list := f.do("GET", "/v1/aps/"+scanner[0]+"/actions", "tenant", nil)
	for _, a := range list["actions"].([]any) {
		p := a.(map[string]any)["params"].(map[string]any)
		if p["on"] != nil || p["bssids"] != nil || p["network"] != "sweet" {
			t.Fatalf("the tenant reads %v", p)
		}
	}

	// The verdict stands for three rounds of the schedule and an hour: two
	// things every 15 minutes are a round of 30, so 150 minutes, though no
	// test follows. Not for three intervals.
	failing := func(after time.Duration) int {
		return len(f.api.failingTests(f.log.Snapshot(), time.Now().Add(after))[hierarchy.NodeID(annex[0])])
	}
	if failing(140*time.Minute) != 1 || failing(160*time.Minute) != 0 {
		t.Fatalf("failing after 140 minutes: %d, after 160: %d", failing(140*time.Minute), failing(160*time.Minute))
	}
}
