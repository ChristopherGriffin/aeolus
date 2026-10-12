package api

import (
	"strconv"
	"strings"
	"testing"
)

// An AP's own Wi-Fi networks, that are not Aeolus's (0137): its report says
// them, and one that is not well said is left out and the report kept. A
// person with operator on the AP asks it to take one out, by its SSID and
// its sections, which must be what the AP last said it has; and the AP is
// handed just that.
func TestOtherNetworks(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	own := func(section, radio, band, ssid string, more map[string]any) map[string]any {
		o := map[string]any{"section": section, "radio": radio, "band": band, "ssid": ssid, "security": "psk2", "clients": 0}
		for k, v := range more {
			o[k] = v
		}
		return o
	}
	// An AP that has said nothing of its own networks has none to take out.
	if code, body := f.do("POST", "/v1/aps/"+ap+"/actions", "griff", map[string]any{"kind": "remove-network", "ssid": "Sweet Spot", "sections": []string{"wifinet2"}}); code != 409 {
		t.Errorf("an AP that has said none of its own: %d %v", code, body)
	}
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "others": []any{
		own("default_radio1", "radio1", "5g", "Sweet Spot", map[string]any{"vlan": 20, "ft": true, "clients": 1}),
		own("wifinet2", "radio0", "2g", "Sweet Spot", map[string]any{"vlan": 20, "ft": true}),
		own("default_radio0", "radio0", "2g", "Sweet_Spot_IoT", map[string]any{"vlan": 10, "clients": 13}),
		// Not well said: a section no config has, an SSID too long, a VLAN
		// there is none of. Left out, and the report kept.
		own("not a section", "radio0", "2g", "Odd", nil),
		own("long", "radio0", "2g", strings.Repeat("x", 33), nil),
		own("far", "radio0", "2g", "Far", map[string]any{"vlan": 5000}),
	}}, nil); code != 200 {
		t.Fatalf("its report: %d %v", code, b)
	}
	code, cfg := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	if code != 200 {
		t.Fatalf("its config: %d %v", code, cfg)
	}
	report := cfg["condition"].(map[string]any)["state"].(map[string]any)["report"].(map[string]any)
	var said []string
	for _, o := range report["others"].([]any) {
		o := o.(map[string]any)
		said = append(said, o["section"].(string)+" "+o["ssid"].(string))
	}
	if got, want := strings.Join(said, "; "), "default_radio1 Sweet Spot; wifinet2 Sweet Spot; default_radio0 Sweet_Spot_IoT"; got != want {
		t.Errorf("its own networks, as kept: %s, want %s", got, want)
	}
	if first := report["others"].([]any)[0].(map[string]any); first["vlan"] != float64(20) || first["ft"] != true || first["clients"] != float64(1) || first["security"] != "psk2" {
		t.Errorf("the first, as kept: %v", first)
	}

	// Asked to take one out: only what it last said it has, by section and
	// SSID together, and with a section at least.
	for what, b := range map[string]map[string]any{
		"no sections":                     {"kind": "remove-network", "ssid": "Sweet Spot"},
		"a section of another network":    {"kind": "remove-network", "ssid": "Sweet Spot", "sections": []string{"default_radio0"}},
		"a section it has not":            {"kind": "remove-network", "ssid": "Sweet Spot", "sections": []string{"aeolus_lab_radio0"}},
		"one it has and one it has not":   {"kind": "remove-network", "ssid": "Sweet Spot", "sections": []string{"wifinet2", "nosuch"}},
		"sections, of another kind":       {"kind": "locate", "sections": []string{"wifinet2"}},
		"an SSID, of another kind":        {"kind": "restart-wifi", "ssid": "Sweet Spot"},
		"a client, which it is not one's": {"kind": "remove-network", "ssid": "Sweet Spot", "sections": []string{"wifinet2"}, "target": "7e:2a:ea:9b:2b:8f"},
	} {
		if code, body := f.do("POST", "/v1/aps/"+ap+"/actions", "griff", b); code != 400 && code != 409 {
			t.Errorf("%s: %d %v", what, code, body)
		}
	}
	code, body := f.do("POST", "/v1/aps/"+ap+"/actions", "griff", map[string]any{"kind": "remove-network", "ssid": "Sweet Spot", "sections": []string{"default_radio1", "wifinet2", "wifinet2"}})
	if code != 200 {
		t.Fatalf("asked to take Sweet Spot out: %d %v", code, body)
	}
	_, _, got := f.apDo("GET", "/v1/ap/actions", token, nil, nil)
	list, _ := got["actions"].([]any)
	if len(list) != 1 {
		t.Fatalf("what the AP is handed: %v", got)
	}
	a := list[0].(map[string]any)
	p, _ := a["params"].(map[string]any)
	secs, _ := p["sections"].([]any)
	if a["kind"] != "remove-network" || p["ssid"] != "Sweet Spot" || len(secs) != 2 || secs[0] != "default_radio1" || secs[1] != "wifinet2" {
		t.Errorf("what the AP is handed: %v", a)
	}
	// Another network asked of the same AP while one waits is another
	// action, and the same network asked again is the one that waits:
	// each answered with what was asked.
	first := func(b map[string]any) (float64, string) {
		t.Helper()
		code, body := f.do("POST", "/v1/aps/"+ap+"/actions", "griff", b)
		if code != 200 {
			t.Fatalf("%v: %d %v", b, code, body)
		}
		a := body["action"].(map[string]any)
		return a["id"].(float64), a["params"].(map[string]any)["ssid"].(string)
	}
	iot, was := first(map[string]any{"kind": "remove-network", "ssid": "Sweet_Spot_IoT", "sections": []string{"default_radio0"}})
	if was != "Sweet_Spot_IoT" {
		t.Errorf("asked for the IoT network, answered with %q", was)
	}
	spot, was := first(map[string]any{"kind": "remove-network", "ssid": "Sweet Spot", "sections": []string{"wifinet2"}})
	if was != "Sweet Spot" || spot == iot {
		t.Errorf("asked for Sweet Spot on one radio while the IoT network waits: action %v (the IoT one is %v), answered with %q", spot, iot, was)
	}
	if again, _ := first(map[string]any{"kind": "remove-network", "ssid": "Sweet Spot", "sections": []string{"wifinet2"}}); again != spot {
		t.Errorf("the same asked again: action %v, want the one that waits, %v", again, spot)
	}
	_, _, got = f.apDo("GET", "/v1/ap/actions", token, nil, nil)
	if list, _ := got["actions"].([]any); len(list) != 2 {
		t.Errorf("the AP is handed %d actions, want the two that wait: %v", len(list), got)
	}
	// What an AP says came of it is plain words, whatever the SSID: the
	// manager takes no other.
	if code, _, body := f.apDo("POST", "/v1/ap/actions/"+strconv.FormatInt(int64(spot), 10), token, map[string]any{"ok": true, "result": "took it out of one radio; kept as it was in /etc/aeolus/removed"}, nil); code != 200 {
		t.Errorf("done: %d %v", code, body)
	}
	// One who may not see the AP is not told it is there.
	if code, _ := f.do("POST", "/v1/aps/"+ap+"/actions", "office", map[string]any{"kind": "remove-network", "ssid": "Sweet Spot", "sections": []string{"wifinet2"}}); code != 404 {
		t.Errorf("one with no role on the AP: %d", code)
	}
}
