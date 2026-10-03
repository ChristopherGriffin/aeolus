package api

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// goodUCI carries the config an AP adopted into House › Office gets from the
// fixture: Sweet Spot on every radio, over VLAN 20, and 40 MHz on 5 GHz.
const goodUCI = `package wireless

config wifi-device 'radio0'
	option band '2g'

config wifi-device 'radio1'
	option band '5g'
	option htmode 'VHT40'

config wifi-iface 'aeolus_sweet_radio0'
	option device 'radio0'
	option mode 'ap'
	option ssid 'Sweet Spot'
	option encryption 'psk2'
	option key '` + passphrase + `'
	option network 'aeolus_sweet'

config wifi-iface 'aeolus_sweet_radio1'
	option device 'radio1'
	option mode 'ap'
	option ssid 'Sweet Spot'
	option encryption 'psk2'
	option key '` + passphrase + `'
	option network 'aeolus_sweet'

package network

config interface 'aeolus_sweet'
	option proto 'none'

config device
	option type '8021q'
	option ifname 'lan'
	option vid '20'

package aeolus

config agent 'agent'
	option poll '60'
`

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// adopted enrolls the pumphouse AP, moves it into House › Office and polls
// once, returning its ID, token and version.
func (f *fixture) adopted() (string, string, float64) {
	f.t.Helper()
	ap, token := f.enrolled()
	if code, b := f.change("griff", map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": "office"}); code != 200 {
		f.t.Fatalf("adopt: %d %v", code, b)
	}
	code, _, body := f.apDo("GET", "/v1/ap/config", token, nil, nil)
	if code != 200 || body["state"] != "ready" {
		f.t.Fatalf("poll: %d %v", code, body)
	}
	return ap, token, body["version"].(float64)
}

func (f *fixture) render(token string, version float64, text string) (int, map[string]any) {
	f.t.Helper()
	code, _, body := f.apDo("POST", "/v1/ap/render", token, map[string]any{"version": version, "uci": text}, nil)
	return code, body
}

func TestLandingZoneAPsCanOnlyPoll(t *testing.T) {
	f := newFixture(t)
	ap, token := f.enrolled()
	if code, body := f.render(token, 1, goodUCI); code != 409 {
		t.Fatalf("render from Landing Zone: %d %v", code, body)
	}
	if code, _, body := f.apDo("POST", "/v1/ap/applied", token, map[string]any{"version": 1, "hash": sha(goodUCI), "ok": true}, nil); code != 409 {
		t.Fatalf("applied from Landing Zone: %d %v", code, body)
	}
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": 0, "uptime": 5}, nil); code != 409 {
		t.Fatalf("state from Landing Zone: %d %v", code, body)
	}
	_, hist := f.do("GET", "/v1/aps/"+ap+"/history", "griff", nil)
	if len(hist["checks"].([]any))+len(hist["applies"].([]any))+len(hist["states"].([]any)) != 0 {
		t.Fatalf("Landing Zone AP left records: %v", hist)
	}
	// It was seen, though.
	_, view := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	cond := view["condition"].(map[string]any)
	if seen, _ := cond["seen"].(map[string]any); seen == nil || seen["source"] != "127.0.0.1" || cond["in_sync"] != nil {
		t.Fatalf("condition = %v", cond)
	}
}

func TestRenderCheck(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()

	if code, body := f.render(token, version-1, goodUCI); code != 200 || body["result"] != "stale" || body["version"] != version {
		t.Fatalf("old version: %d %v", code, body)
	}
	code, body := f.render(token, version, strings.Replace(goodUCI, "option ssid 'Sweet Spot'", "option ssid 'Sweet Spot 2'", 1))
	if code != 200 || body["result"] != "refused" || !strings.Contains(jsonText(body["problems"]), "Sweet Spot 2") {
		t.Fatalf("wrong SSID: %d %v", code, body)
	}
	if _, body := f.render(token, version, "package wireless\nconfig wifi-iface 'x"); body["result"] != "refused" || !strings.Contains(jsonText(body["problems"]), "does not parse") {
		t.Fatalf("broken UCI: %v", body)
	}
	code, body = f.render(token, version, goodUCI)
	if code != 200 || body["result"] != "ok" || body["hash"] != sha(goodUCI) || len(body["problems"].([]any)) != 0 {
		t.Fatalf("good UCI: %d %v", code, body)
	}

	// Applying what passed is checked; applying anything else is not.
	if code, _, body := f.apDo("POST", "/v1/ap/applied", token, map[string]any{"version": version, "hash": sha(goodUCI), "ok": true}, nil); code != 200 || body["checked"] != true {
		t.Fatalf("applied: %d %v", code, body)
	}
	if code, _, body := f.apDo("POST", "/v1/ap/applied", token, map[string]any{"version": version, "hash": sha("other"), "ok": false, "error": "wifi failed"}, nil); code != 200 || body["checked"] != false {
		t.Fatalf("unchecked apply: %d %v", code, body)
	}
	if code, _, _ := f.apDo("POST", "/v1/ap/applied", token, map[string]any{"version": version, "hash": "nope", "ok": true}, nil); code != 400 {
		t.Fatalf("bad hash: %d", code)
	}

	_, view := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	cond := view["condition"].(map[string]any)
	if cond["in_sync"] != true || cond["check"].(map[string]any)["result"] != "ok" || cond["apply"].(map[string]any)["error"] != "wifi failed" {
		t.Fatalf("condition = %v", cond)
	}
	_, hist := f.do("GET", "/v1/aps/"+ap+"/history", "griff", nil)
	checks := hist["checks"].([]any)
	if n := len(checks); n != 4 {
		t.Fatalf("%d checks recorded, want 4: %v", n, hist)
	}
	if strings.Contains(jsonText(hist), passphrase) {
		t.Fatal("the history holds a passphrase")
	}
	// The kept UCI is what was sent, secrets blanked (0041); a stale one is
	// not kept.
	kept, _ := checks[0].(map[string]any)["uci"].(string)
	if kept != strings.ReplaceAll(goodUCI, passphrase, "<secret>") {
		t.Fatalf("kept UCI:\n%s", kept)
	}
	if stale := checks[3].(map[string]any); stale["result"] != "stale" || stale["uci"] != nil {
		t.Fatalf("stale check = %v", stale)
	}
	if strings.Contains(jsonText(cond["check"]), "package wireless") {
		t.Fatal("the summary carries the whole UCI")
	}

	// A change that breaks the config: the check refuses, and the AP is out
	// of sync until it runs the new version.
	if code, b := f.change("griff", map[string]any{"kind": "unset", "tree": "services", "node": "household", "path": "network.sweet.passphrase"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	_, view = f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	next := view["version"].(float64)
	if view["condition"].(map[string]any)["in_sync"] != false {
		t.Fatalf("in_sync after a change = %v", view["condition"])
	}
	if _, body := f.render(token, next, goodUCI); body["result"] != "refused" || !strings.Contains(jsonText(body["problems"]), "held") {
		t.Fatalf("held config: %v", body)
	}

	// Others cannot read an AP's history.
	if code, _ := f.do("GET", "/v1/aps/"+ap+"/history", "office", nil); code != 404 {
		t.Fatalf("office reading history: %d", code)
	}
}

func TestStateReports(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	report := map[string]any{
		"version": version, "uptime": 3600, "openwrt": "25.12.5",
		"radios":     []any{map[string]any{"radio": "radio1", "band": "5g", "channel": 36, "width": 40, "clients": 3}},
		"vlans":      []int{1, 20, 30},
		"transports": map[string]any{"sweet": map[string]any{"active": "primary", "primary": "up"}},
		"steering": map[string]any{"installed": true, "running": true, "interval": 30000, "ssids": []string{"Sweet Spot"},
			"bss": []any{map[string]any{"ssid": "Sweet Spot", "band": "2g", "clients": 1, "steered_away": 4, "steered_in": 0}}},
	}
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, report, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	for name, bad := range map[string]map[string]any{
		"band":     {"version": 1, "radios": []any{map[string]any{"radio": "r", "band": "60g"}}},
		"vlan":     {"version": 1, "vlans": []int{5000}},
		"repeat":   {"version": 1, "vlans": []int{20, 20}},
		"active":   {"version": 1, "transports": map[string]any{"sweet": map[string]any{"active": "both"}}},
		"network":  {"version": 1, "transports": map[string]any{"Sweet Spot": map[string]any{"active": "none"}}},
		"unknown":  {"version": 1, "temperature": 40},
		"negative": {"version": -1},
		"steering": {"version": 1, "steering": map[string]any{"installed": true, "running": true, "interval": 30000,
			"bss": []any{map[string]any{"ssid": "Sweet Spot", "band": "60g", "clients": 1}}}},
		"steered": {"version": 1, "steering": map[string]any{"installed": true, "bss": []any{map[string]any{"ssid": "x", "band": "5g", "steered_in": -1}}}},
	} {
		if code, _, body := f.apDo("POST", "/v1/ap/state", token, bad, nil); code != 400 {
			t.Errorf("%s: %d %v", name, code, body)
		}
	}
	_, view := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	cond := view["condition"].(map[string]any)
	state := cond["state"].(map[string]any)["report"].(map[string]any)
	steer, _ := state["steering"].(map[string]any)
	if cond["in_sync"] != true || state["openwrt"] != "25.12.5" || len(state["vlans"].([]any)) != 3 || steer["interval"] != 30000.0 || len(steer["bss"].([]any)) != 1 {
		t.Fatalf("condition = %v", cond)
	}
}

func TestFleetView(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	code, body := f.do("GET", "/v1/aps", "griff", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	byID := map[string]map[string]any{}
	for _, a := range body["aps"].([]any) {
		m := a.(map[string]any)
		byID[m["id"].(string)] = m
	}
	got := byID[ap]
	if got == nil || got["config"] != "ready" || got["in_sync"] != true || got["name"] != "PumphouseAP" || got["seen"] == nil {
		t.Fatalf("adopted AP = %v", got)
	}
	if office := byID["office-ap"]; office == nil || office["seen"] != nil || office["in_sync"] != nil {
		t.Fatalf("office-ap = %v", office)
	}
	// office has no role in Locations, so it sees no APs.
	if _, body := f.do("GET", "/v1/aps", "office", nil); len(body["aps"].([]any)) != 0 {
		t.Fatalf("office sees %v", body["aps"])
	}
}
