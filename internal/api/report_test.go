package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	option device 'lan.20'

config device
	option type '8021q'
	option ifname 'lan'
	option vid '20'
	option name 'lan.20'

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
		"ports": []any{map[string]any{"name": "lan1", "up": true, "carrier": false},
			map[string]any{"name": "wan", "up": true, "carrier": true, "speed": "1000F", "uplink": true}},
		"vxlan": map[string]any{"installed": true, "clamp": true, "prober": true, "tunnels": []any{
			// What the prober found (0059); what it does not know yet is null.
			map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789, "mtu": 1450, "up": true, "probe": map[string]any{
				"verdict": "up", "interval": 30, "asks": []string{"192.168.50.1"}, "underlay": true, "underlay_ms": 0.4,
				"from": "192.168.50.1", "rtt_ms": 0.8, "answered_ago": 12,
				"lease": map[string]any{"address": "192.168.50.6", "server": "192.168.50.254", "router": "192.168.50.1", "expires_in": 86000}}},
			// One that starts from VLAN 20, where the AP has an address (0063).
			map[string]any{"vni": 60, "peer": "1.1.1.2", "port": 4789, "mtu": 1450, "from_vlan": 20, "from_address": "192.168.20.74", "up": true, "probe": map[string]any{
				"verdict": "unknown", "interval": 30, "asks": []string{"ff02::1"}, "underlay": nil, "underlay_ms": nil,
				"from": nil, "rtt_ms": nil, "answered_ago": nil}},
			map[string]any{"vni": 10, "peer": "2001:db8::2", "port": 4789, "mtu": 1450, "up": false, "standby": true}},
			"loops": []any{map[string]any{"port": "lan3", "device": "lan3.30", "vni": 30, "came_in": "aeolus_30", "ago": 4}}},
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
		"port":    {"version": 1, "ports": []any{map[string]any{"name": "LAN 1"}}},
		"twice":   {"version": 1, "ports": []any{map[string]any{"name": "lan1"}, map[string]any{"name": "lan1"}}},
		"speed":   {"version": 1, "ports": []any{map[string]any{"name": "lan1", "speed": "fast"}}},
		"vni":     {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 16777216, "peer": "1.1.1.2", "port": 4789}}}},
		"peer":    {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "vtep.example.net", "port": 4789}}}},
		"verdict": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"probe": map[string]any{"verdict": "fine", "interval": 30}}}}},
		"answered by": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"probe": map[string]any{"verdict": "up", "interval": 30, "from": "gateway"}}}}},
		"rtt": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"probe": map[string]any{"verdict": "up", "interval": 30, "rtt_ms": -1}}}}},
		"loop port": {"version": 1, "vxlan": map[string]any{"loops": []any{map[string]any{"port": "LAN 3", "device": "lan3", "ago": 1}}}},
		"lease": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"probe": map[string]any{"verdict": "up", "interval": 30, "lease": map[string]any{"address": "fe80::1", "expires_in": 10}}}}}},
		"loop device": {"version": 1, "vxlan": map[string]any{"loops": []any{map[string]any{"port": "lan3", "device": "lan3; rm", "ago": 1}}}},
		"from vlan":   {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789, "from_vlan": 4095}}}},
		"from address": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"from_vlan": 20, "from_address": "fe80::1"}}}},
		"address, no vlan": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"from_address": "192.168.20.74"}}}},
	} {
		if code, _, body := f.apDo("POST", "/v1/ap/state", token, bad, nil); code != 400 {
			t.Errorf("%s: %d %v", name, code, body)
		}
	}
	_, view := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	cond := view["condition"].(map[string]any)
	state := cond["state"].(map[string]any)["report"].(map[string]any)
	steer, _ := state["steering"].(map[string]any)
	ports, _ := state["ports"].([]any)
	tunnels, _ := state["vxlan"].(map[string]any)["tunnels"].([]any)
	loops, _ := state["vxlan"].(map[string]any)["loops"].([]any)
	if cond["in_sync"] != true || state["openwrt"] != "25.12.5" || len(state["vlans"].([]any)) != 3 || steer["interval"] != 30000.0 || len(steer["bss"].([]any)) != 1 || len(ports) != 2 || len(tunnels) != 3 || len(loops) != 1 {
		t.Fatalf("condition = %v", cond)
	}
	if p, _ := tunnels[0].(map[string]any)["probe"].(map[string]any); p["verdict"] != "up" || p["from"] != "192.168.50.1" || p["rtt_ms"] != 0.8 ||
		p["lease"].(map[string]any)["address"] != "192.168.50.6" {
		t.Fatalf("probe = %v", p)
	}
	if s := tunnels[1].(map[string]any); s["from_vlan"] != 20.0 || s["from_address"] != "192.168.20.74" {
		t.Fatalf("where the tunnel starts = %v", s)
	}
}

// A tunnel the AP's uplink cannot carry now, by what it last reported, is
// held at once: in the preview, and when the AP polls (0056).
func TestTunnelMustFitTheUplink(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	report := func(mtu int) {
		t.Helper()
		if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60,
			"vxlan": map[string]any{"installed": true, "clamp": true, "uplink_mtu": mtu}}, nil); code != 200 {
			t.Fatalf("state: %d %v", code, body)
		}
	}
	set := func(tree, node, path string, v any) map[string]any {
		t.Helper()
		code, body := f.change("griff", map[string]any{"kind": "set", "tree": tree, "node": node, "path": path, "value": v})
		if code != 200 {
			t.Fatalf("%s: %d %v", path, code, body)
		}
		return body
	}
	poll := func() map[string]any {
		t.Helper()
		_, _, body := f.apDo("GET", "/v1/ap/config", token, nil, nil)
		return body
	}
	report(1500)
	set("locations", "office", "concentrators.dc.address", "1.1.1.2")
	set("locations", "office", "concentrators.dc.port", 4789)
	set("services", "household", "network.sweet.transport.primary.concentrator", "dc")
	set("services", "household", "network.sweet.transport.primary.vni", 20)
	set("services", "household", "network.sweet.transport.primary.type", "vxlan")
	// The default MTU, 1450, fits a 1500-byte uplink.
	if body := poll(); body["state"] != "ready" {
		t.Fatalf("default MTU: %v", body)
	}
	// 1500 does not, and the change that sets it says so.
	body := set("locations", "office", "concentrators.dc.mtu", 1500)
	want := "network.sweet.transport.primary: tunnel dc's MTU of 1500 needs 1550 on the AP's uplink, which carries 1500 now"
	if !strings.Contains(fmt.Sprint(body["checks"]), want) {
		t.Fatalf("checks = %v", body["checks"])
	}
	if body := poll(); body["state"] != "held" || !strings.Contains(fmt.Sprint(body["problems"]), want) {
		t.Fatalf("1500 on a 1500 uplink: %v", body)
	}
	// Once the AP reports a jumbo uplink, it goes.
	report(9000)
	if body := poll(); body["state"] != "ready" {
		t.Fatalf("1500 on a 9000 uplink: %v", body)
	}
	_ = ap
}

// What an AP reports it cannot run is held before it is sent (0057): a
// tunnel while netifd has not loaded vxlan, and band steering while hostapd
// lacks 802.11v.
func TestReportedLacksAreHeld(t *testing.T) {
	f := newFixture(t)
	_, token, version := f.adopted()
	report := func(loaded, btm bool) {
		t.Helper()
		if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60,
			"vxlan":    map[string]any{"installed": true, "loaded": loaded, "clamp": true, "uplink_mtu": 1500},
			"steering": map[string]any{"installed": true, "running": true, "interval": 0, "bss_transition": btm}}, nil); code != 200 {
			t.Fatalf("state: %d %v", code, body)
		}
	}
	set := func(tree, node, path string, v any) {
		t.Helper()
		if code, body := f.change("griff", map[string]any{"kind": "set", "tree": tree, "node": node, "path": path, "value": v}); code != 200 {
			t.Fatalf("%s: %d %v", path, code, body)
		}
	}
	poll := func() string {
		t.Helper()
		_, _, body := f.apDo("GET", "/v1/ap/config", token, nil, nil)
		return fmt.Sprint(body["state"], body["problems"])
	}
	report(false, false)
	set("services", "household", "network.sweet.band_steering", true)
	if got := poll(); !strings.HasPrefix(got, "held") || !strings.Contains(got, "network.sweet: band steering and BSS transition need 802.11v, which this AP's hostapd lacks") {
		t.Fatalf("band steering without 802.11v: %s", got)
	}
	set("services", "household", "network.sweet.band_steering", false)
	set("locations", "office", "concentrators.dc.address", "1.1.1.2")
	set("locations", "office", "concentrators.dc.port", 4789)
	set("services", "household", "network.sweet.transport.primary.concentrator", "dc")
	set("services", "household", "network.sweet.transport.primary.vni", 20)
	set("services", "household", "network.sweet.transport.primary.type", "vxlan")
	if got := poll(); !strings.HasPrefix(got, "held") || !strings.Contains(got, "this AP's netifd has not loaded vxlan; restart its network") {
		t.Fatalf("a tunnel before netifd loaded vxlan: %s", got)
	}
	report(true, false)
	if got := poll(); !strings.HasPrefix(got, "ready") {
		t.Fatalf("once loaded: %s", got)
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
