package api

import (
	"strings"
	"testing"
)

// An AP says what it is again, and the manager records it where it has
// changed (0122).
func TestFactsAgain(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}}
	ids := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:3a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	ap, token := ids[0], ids[1]
	changes := func() []any {
		_, log := f.do("GET", "/v1/changes?after=0&limit=1000", "griff", nil)
		return log["changes"].([]any)
	}
	before := len(changes())
	test := func() (int, map[string]any) {
		return f.do("POST", "/v1/aps/"+ap+"/actions", "griff", map[string]any{"kind": "client-test", "network": "sweet"})
	}

	// What it enrolled with, said again, changes nothing.
	said := map[string]any{"mac": "02:00:00:00:00:3A", "hostname": "Scanner", "radios": []any{serving, spare}}
	if code, _, b := f.apDo("POST", "/v1/ap/facts", token, said, nil); code != 200 || b["changed"] != false || len(changes()) != before {
		t.Fatalf("the same facts: %d %v, %d changes for %d", code, b, len(changes()), before)
	}
	if code, b := test(); code != 400 || !strings.Contains(b["error"].(string), "scan radio") {
		t.Fatalf("a client test with no scan radio: %d %v", code, b)
	}
	// Its second 5 GHz radio does 20 MHz alone, so the band can do no more
	// while the manager takes it for a serving one.
	if office, _ := f.hardwareOf("office"); office["5g"][80] == "" {
		t.Fatalf("office, with two serving 5 GHz radios = %v", office)
	}

	// It gains a scan radio, and a newer OpenWrt: the manager records it, in
	// its own name, and where it enrolled from stays.
	spare["reserved"] = true
	said["openwrt"] = "25.12.3"
	if code, _, b := f.apDo("POST", "/v1/ap/facts", token, said, nil); code != 200 || b["changed"] != true {
		t.Fatalf("new facts: %d %v", code, b)
	}
	log := changes()
	last := log[len(log)-1].(map[string]any)
	op := last["op"].(map[string]any)
	if len(log) != before+1 || last["actor"] != "aeolus" || op["kind"] != "facts" || op["node"] != ap {
		t.Fatalf("the change: %d for %d, %v", len(log), before, last)
	}
	_, page := f.do("GET", "/v1/trees/locations/nodes/"+ap, "griff", nil)
	facts := page["facts"].(map[string]any)
	radios := facts["radios"].([]any)
	if facts["openwrt"] != "25.12.3" || facts["source"] != "127.0.0.1" || radios[1].(map[string]any)["reserved"] != true ||
		page["node"].(map[string]any)["parent"] != "office" {
		t.Fatalf("AP page = %v", page)
	}
	// A scan radio serves no one, so what it can do is not the band's.
	if office, _ := f.hardwareOf("office"); office["5g"][80] != "" {
		t.Fatalf("office, with a scan radio = %v", office)
	}
	// So it can be asked for a client test (0121).
	if code, b := test(); code != 200 {
		t.Fatalf("a client test with a scan radio: %d %v", code, b)
	}

	// Said again, it is the same; changed again so soon, it waits.
	if code, _, b := f.apDo("POST", "/v1/ap/facts", token, said, nil); code != 200 || b["changed"] != false || len(changes()) != before+1 {
		t.Fatalf("the new facts again: %d %v", code, b)
	}
	said["openwrt"] = "25.12.4"
	code, head, b := f.apDo("POST", "/v1/ap/facts", token, said, nil)
	if code != 429 || head.Get("Retry-After") == "" || len(changes()) != before+1 {
		t.Fatalf("facts changed again at once: %d %v %v", code, head, b)
	}

	// Only as itself: not under another AP's MAC, and not by a person.
	other := map[string]any{"mac": "02:00:00:00:00:3b", "radios": []any{serving}}
	if code, _, b := f.apDo("POST", "/v1/ap/facts", token, other, nil); code != 400 {
		t.Fatalf("another AP's facts: %d %v", code, b)
	}
	if code, _, b := f.apDo("POST", "/v1/ap/facts", "", said, nil); code != 401 {
		t.Fatalf("facts with no token: %d %v", code, b)
	}
	if code, b := f.change("griff", map[string]any{"kind": "facts", "node": ap, "value": said}); code != 403 {
		t.Fatalf("a person's facts: %d %v", code, b)
	}
}
