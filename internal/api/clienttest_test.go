package api

import (
	"strconv"
	"strings"
	"testing"
)

// A client test (0121): a person with operator on an AP that has a scan
// radio asks it to join one of its own networks as a client would, on an AP
// of their choosing. The AP is handed what to test, and what it found is
// checked and kept with the action.
func TestClientTest(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	scanner := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:2a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	plain := strings.Split(f.enrollAt("Plain", "02:00:00:00:00:2b", "office", []any{serving}, map[string]int{"5g": 44}), "|")
	ask := func(ap string, body map[string]any) (int, map[string]any) {
		body["kind"] = "client-test"
		return f.do("POST", "/v1/aps/"+ap+"/actions", "griff", body)
	}
	refused := func(name, ap string, body map[string]any, why string) {
		t.Helper()
		if code, b := ask(ap, body); code != 400 || !strings.Contains(b["error"].(string), why) {
			t.Errorf("%s: %d %v", name, code, b)
		}
	}
	refused("an AP with no scan radio", plain[0], map[string]any{"network": "sweet"}, "scan radio")
	refused("a network the AP does not offer", scanner[0], map[string]any{"network": "guest"}, "one of the networks this AP offers")
	refused("no network", scanner[0], map[string]any{}, "one of the networks this AP offers")
	refused("a band there is none of", scanner[0], map[string]any{"network": "sweet", "band": "60g"}, "band")
	refused("on what is no AP", scanner[0], map[string]any{"network": "sweet", "on": "office"}, "on: an AP")
	refused("on an AP that has not said its BSSIDs", scanner[0], map[string]any{"network": "sweet", "on": plain[0]}, "has not said which BSSIDs")
	if code, b := f.do("POST", "/v1/aps/"+scanner[0]+"/actions", "griff", map[string]any{"kind": "locate", "network": "sweet"}); code != 400 {
		t.Errorf("a network for a locate: %d %v", code, b)
	}

	// The AP to join says its BSSIDs, and the folder has a speed server.
	_, _, cfg := f.apDo("GET", "/v1/ap/config", plain[1], nil, nil)
	if code, _, b := f.apDo("POST", "/v1/ap/state", plain[1], map[string]any{"version": cfg["version"], "uptime": 60,
		"bssids": []string{"02:00:00:00:00:2b", "06:00:00:00:00:2b"}}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, b)
	}
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "house", "path": "system.speed_server", "value": "http://192.168.20.106"}); code != 200 {
		t.Fatalf("speed server: %d %v", code, b)
	}
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "house", "path": "system.speed_server", "value": "https://secure.example"}); code == 200 {
		t.Fatalf("a speed server over TLS was taken: %v", b)
	}
	code, body := ask(scanner[0], map[string]any{"network": "sweet", "on": plain[0], "band": "5g"})
	if code != 200 {
		t.Fatalf("asked: %d %v", code, body)
	}
	a := body["action"].(map[string]any)
	id := strconv.FormatInt(int64(a["id"].(float64)), 10)
	p := a["params"].(map[string]any)
	if a["kind"] != "client-test" || a["state"] != "pending" || p["network"] != "sweet" || p["on"] != plain[0] || p["ssid"] != "Sweet Spot" ||
		p["band"] != "5g" || p["speed"] != "http://192.168.20.106" || len(p["bssids"].([]any)) != 2 {
		t.Fatalf("asked = %v", a)
	}
	// One at a time: asked again while it is open, it is the same one.
	if _, again := ask(scanner[0], map[string]any{"network": "sweet"}); again["action"].(map[string]any)["id"] != a["id"] {
		t.Fatalf("asked again = %v", again)
	}
	// With no AP named and no band, it is the strongest other AP's, on any.
	// (Seen once this one is done, below.)

	// The AP is handed what to test.
	_, _, got := f.apDo("GET", "/v1/ap/actions", scanner[1], nil, nil)
	handed := got["actions"].([]any)[0].(map[string]any)
	if hp := handed["params"].(map[string]any); handed["kind"] != "client-test" || hp["ssid"] != "Sweet Spot" || hp["bssids"].([]any)[1] != "06:00:00:00:00:2b" {
		t.Fatalf("handed = %v", handed)
	}

	// What it found is checked: a result that is not one is refused, and
	// the action stays open for the right one.
	yes := true
	found := func() map[string]any {
		return map[string]any{
			"v": 1, "ssid": "Sweet Spot", "band": "5g", "bssid": "06:00:00:00:00:2b", "channel": 44, "client": "02:00:00:00:00:f0",
			"outcome": "passed", "stage": "done", "reason": "", "summary": "passed: joined in 0.6 s", "waited_ms": 36907, "joined_ms": 636, "took_ms": 15198,
			"heard": []any{map[string]any{"bssid": "06:00:00:00:00:2b", "band": "5g", "channel": 44, "signal": -44.0}},
			"steps": []any{
				map[string]any{"t": 146, "stage": "find", "what": "looks for the network"},
				map[string]any{"t": 636, "stage": "key", "what": "let on to the network", "ok": yes},
				map[string]any{"t": 925, "stage": "dhcp", "what": "given 192.168.20.108 by 192.168.20.254", "ok": yes, "ms": 284},
			},
			"link":          map[string]any{"signal": -45, "tx": map[string]any{"mbps": 54.0, "how": ""}, "rx": nil},
			"dhcp":          map[string]any{"ms": 284, "address": "192.168.20.108", "mask": 24, "router": "192.168.20.1", "dns": []string{"192.168.20.254"}, "server": "192.168.20.254", "lease": 86400},
			"gateway":       map[string]any{"address": "192.168.20.1", "sent": 3, "got": 3, "min": 2.358, "avg": 4.131, "max": 7.61},
			"dns":           map[string]any{"server": "192.168.20.254", "name": "example.com", "ms": 42, "rcode": 0, "answers": 2, "address": "104.20.23.154"},
			"speed":         map[string]any{"server": "http://192.168.20.106", "down": 21.0, "up": 22.1},
			"scan_radio":    map[string]any{"before": 38, "after": 39},
			"something_new": "a later agent's",
		}
	}
	done := func(detail any) (int, map[string]any) {
		code, _, b := f.apDo("POST", "/v1/ap/actions/"+id, scanner[1], map[string]any{"ok": true, "result": "passed: joined in 0.6 s", "detail": detail}, nil)
		return code, b
	}
	for name, spoil := range map[string]func(map[string]any){
		"an outcome there is none of": func(d map[string]any) { d["outcome"] = "maybe" },
		"a stage there is none of":    func(d map[string]any) { d["steps"].([]any)[0].(map[string]any)["stage"] = "lunch" },
		"a step that says nothing":    func(d map[string]any) { d["steps"].([]any)[0].(map[string]any)["what"] = "" },
		"a BSSID that is not one":     func(d map[string]any) { d["bssid"] = "the office AP" },
		"an address that is not one":  func(d map[string]any) { d["dhcp"].(map[string]any)["address"] = "192.168.20" },
		"more pings back than sent":   func(d map[string]any) { d["gateway"].(map[string]any)["got"] = 4 },
		"a reason not in plain text":  func(d map[string]any) { d["reason"] = "café" },
		"a speed below nothing":       func(d map[string]any) { d["speed"].(map[string]any)["down"] = -1 },
	} {
		d := found()
		spoil(d)
		if code, b := done(d); code != 400 {
			t.Errorf("%s: %d %v", name, code, b)
		}
	}
	if code, b := done("not an object"); code != 400 {
		t.Errorf("a detail that is no object: %d %v", code, b)
	}
	if code, b := done(found()); code != 200 {
		t.Fatalf("done: %d %v", code, b)
	}
	_, body = f.do("GET", "/v1/aps/"+scanner[0]+"/actions", "griff", nil)
	kept := body["actions"].([]any)[0].(map[string]any)
	d := kept["detail"].(map[string]any)
	if kept["state"] != "done" || kept["result"] != "passed: joined in 0.6 s" || d["outcome"] != "passed" || len(d["steps"].([]any)) != 3 ||
		d["speed"].(map[string]any)["up"] != 22.1 || d["dhcp"].(map[string]any)["router"] != "192.168.20.1" || d["something_new"] != nil ||
		kept["params"].(map[string]any)["on"] != plain[0] {
		t.Fatalf("kept = %v", kept)
	}
	// Said once: a second saying finds nothing open.
	if code, _ := done(found()); code != 404 {
		t.Errorf("said again: %d", code)
	}

	// With no AP named, it keeps to none, and without a band takes any.
	code, body = ask(scanner[0], map[string]any{"network": "sweet"})
	if p := body["action"].(map[string]any)["params"].(map[string]any); code != 200 || p["bssids"] != nil || p["band"] != nil || p["on"] != nil {
		t.Fatalf("the strongest other AP: %d %v", code, body)
	}
	// A test that failed is kept as failed, with where it stopped.
	next := strconv.FormatInt(int64(body["action"].(map[string]any)["id"].(float64)), 10)
	f.apDo("GET", "/v1/ap/actions", scanner[1], nil, nil)
	failed := map[string]any{"v": 1, "outcome": "failed", "stage": "dhcp", "reason": "no DHCP server gave it an address", "steps": []any{}}
	if code, _, b := f.apDo("POST", "/v1/ap/actions/"+next, scanner[1], map[string]any{"ok": false, "result": "failed at dhcp: no DHCP server gave it an address", "detail": failed}, nil); code != 200 {
		t.Fatalf("failed: %d %v", code, b)
	}
	_, body = f.do("GET", "/v1/aps/"+scanner[0]+"/actions", "griff", nil)
	if a := body["actions"].([]any)[0].(map[string]any); a["state"] != "failed" || a["detail"].(map[string]any)["stage"] != "dhcp" {
		t.Fatalf("failed, kept = %v", a)
	}

	// Only a client test has a detail.
	code, body = f.do("POST", "/v1/aps/"+scanner[0]+"/actions", "griff", map[string]any{"kind": "locate"})
	if code != 200 {
		t.Fatalf("locate: %d %v", code, body)
	}
	loc := strconv.FormatInt(int64(body["action"].(map[string]any)["id"].(float64)), 10)
	f.apDo("GET", "/v1/ap/actions", scanner[1], nil, nil)
	if code, _, b := f.apDo("POST", "/v1/ap/actions/"+loc, scanner[1], map[string]any{"ok": true, "result": "blinking", "detail": found()}, nil); code != 400 {
		t.Errorf("a detail for a locate: %d %v", code, b)
	}
	// Someone who may only view the AP may not ask.
	if code, _ := f.do("POST", "/v1/aps/"+scanner[0]+"/actions", "office", map[string]any{"kind": "client-test", "network": "sweet"}); code == 200 {
		t.Errorf("a viewer asked")
	}
}
