package api

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// A test through the air has, with what it found, the same attempt as the
// AP it joined recorded it (0132): that AP's attempt by the test's client,
// begun while the test was made. Not one from before; and not for one who
// may not view that AP.
func TestATestAsItsAPSawIt(t *testing.T) {
	f := newFixture(t)
	change := func(op map[string]any) {
		t.Helper()
		if code, b := f.change("griff", op); code != 200 {
			t.Fatalf("%v: %d %v", op, code, b)
		}
	}
	change(map[string]any{"kind": "add-builtins"})
	change(map[string]any{"kind": "add-folder", "tree": "locations", "node": "shed", "name": "Shed", "parent": "house"})
	change(map[string]any{"kind": "grant", "account": "tenant", "tree": "locations", "node": "office", "role": "viewer"})
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	scanner := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:2a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	// The AP it joins is in a folder the tenant may not view.
	plain := strings.Split(f.enrollAt("Plain", "02:00:00:00:00:2b", "shed", []any{serving}, map[string]int{"5g": 44}), "|")
	_, _, cfg := f.apDo("GET", "/v1/ap/config", plain[1], nil, nil)
	if code, _, b := f.apDo("POST", "/v1/ap/state", plain[1], map[string]any{"version": cfg["version"], "uptime": 60,
		"bssids": []string{"02:00:00:00:00:2b", "06:00:00:00:00:2b"}}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, b)
	}

	const client = "02:00:00:00:00:f0"
	yes, no := true, false
	// What Plain recorded of the client: let on, an address, and its
	// gateway asked for three times with no answer.
	attempt := func(started time.Time) map[string]any {
		return map[string]any{"mac": client, "bss": "phy0-ap0", "network": "sweet", "ssid": "Sweet Spot", "band": "5g",
			"started": started.UnixMilli(), "took_ms": 6678, "outcome": "failed", "stage": "gateway", "reason": "192.168.20.1, its gateway, did not answer, and it left",
			"events": []any{
				map[string]any{"t": 0, "stage": "auth", "what": "authenticated", "ok": yes},
				map[string]any{"t": 12, "stage": "assoc", "what": "associated", "ok": yes},
				map[string]any{"t": 94, "stage": "key", "what": "let on to the network", "ok": yes},
				map[string]any{"t": 384, "stage": "dhcp", "what": "asks for an address (discover)"},
				map[string]any{"t": 468, "stage": "dhcp", "what": "given 192.168.20.108", "ok": yes},
				map[string]any{"t": 622, "stage": "gateway", "what": "asks who has 192.168.20.1, its gateway"},
				map[string]any{"t": 1658, "stage": "gateway", "what": "asks who has 192.168.20.1, its gateway"},
				map[string]any{"t": 2698, "stage": "gateway", "what": "asks who has 192.168.20.1, its gateway"},
				map[string]any{"t": 6678, "stage": "end", "what": "disconnected"},
			}}
	}
	record := func(started time.Time) {
		t.Helper()
		if code, _, b := f.apDo("POST", "/v1/ap/connections", plain[1], map[string]any{"connections": []any{attempt(started)}}, nil); code != 200 || b["recorded"] != float64(1) {
			t.Fatalf("connections: %d %v", code, b)
		}
	}
	// An attempt of ten minutes ago is not the test's.
	record(time.Now().Add(-10 * time.Minute))

	code, body := f.do("POST", "/v1/aps/"+scanner[0]+"/actions", "griff", map[string]any{"kind": "client-test", "network": "sweet", "on": plain[0], "band": "5g"})
	if code != 200 {
		t.Fatalf("asked: %d %v", code, body)
	}
	id := body["action"].(map[string]any)["id"]
	f.apDo("GET", "/v1/ap/actions", scanner[1], nil, nil)
	d := map[string]any{
		"v": 1, "ssid": "Sweet Spot", "band": "5g", "bssid": "06:00:00:00:00:2b", "channel": 44, "client": client,
		"outcome": "failed", "stage": "gateway", "reason": "its gateway, 192.168.20.1, does not answer", "summary": "failed at gateway", "joined_ms": 495,
		"steps": []any{
			map[string]any{"t": 293, "stage": "find", "what": "found it", "ok": yes},
			map[string]any{"t": 495, "stage": "key", "what": "let on to the network", "ok": yes},
			map[string]any{"t": 842, "stage": "dhcp", "what": "given 192.168.20.108", "ok": yes, "ms": 342},
			map[string]any{"t": 3859, "stage": "gateway", "what": "192.168.20.1, its gateway, does not answer", "ok": no},
		},
		// An AP that says what the other saw is not believed.
		"seen": map[string]any{"ap": "ap-000000000000", "name": "Nobody", "outcome": "online", "stage": "internet", "way": []any{}},
	}
	if code, _, b := f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%v", id), scanner[1], map[string]any{"ok": false, "result": "failed at gateway", "detail": d}, nil); code != 200 {
		t.Fatalf("done: %d %v", code, b)
	}
	seen := func(as string) map[string]any {
		t.Helper()
		_, body := f.do("GET", "/v1/aps/"+scanner[0]+"/actions", as, nil)
		detail := body["actions"].([]any)[0].(map[string]any)["detail"].(map[string]any)
		s, _ := detail["seen"].(map[string]any)
		return s
	}
	if s := seen("griff"); s != nil {
		t.Fatalf("an attempt from before the test was taken for it: %v", s)
	}

	// Plain's record of the test's own attempt comes in.
	record(time.Now())
	s := seen("griff")
	if s == nil || s["ap"] != plain[0] || s["name"] != "Plain" || s["outcome"] != "failed" || s["stage"] != "gateway" || !strings.Contains(s["reason"].(string), "did not answer") {
		t.Fatalf("seen = %v", s)
	}
	var way []string
	for _, x := range s["way"].([]any) {
		p := x.(map[string]any)
		line := fmt.Sprintf("%v %v", p["k"], p["state"])
		if p["ms"] != nil {
			line += fmt.Sprintf(" %v", p["ms"])
		}
		way = append(way, line)
	}
	if got, want := strings.Join(way, ", "), "auth ok, assoc ok 12, key ok 82, dhcp ok 84, gateway bad 2076, dns none, internet none"; got != want {
		t.Errorf("the way as the AP saw it = %s, want %s", got, want)
	}
	// And with the thing tested, and the tests listed, on the tab.
	_, view := f.do("GET", "/v1/assurance", "griff", nil)
	if s, _ := view["tests"].([]any)[0].(map[string]any)["detail"].(map[string]any)["seen"].(map[string]any); s == nil || s["ap"] != plain[0] {
		t.Errorf("the test listed on the tab, as its AP saw it = %v", s)
	}
	// One who may view the scanner and not the AP it joined is not told.
	if s := seen("tenant"); s != nil {
		t.Errorf("one who may not view the AP was told what it saw: %v", s)
	}
}
