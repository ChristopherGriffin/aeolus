package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// A client test over the wire (0128): any AP whose agent says it can is
// asked for one of its networks, from itself, with no radio in it; what it
// found is checked and kept; and where a folder asks for them, an AP with
// no scan radio is asked on the schedule, and a network that fails twice
// running is an alert that says it was the wire.
func TestClientTestOverTheWire(t *testing.T) {
	f := newFixture(t)
	change := func(body map[string]any) {
		t.Helper()
		if code, b := f.change("griff", body); code != 200 {
			t.Fatalf("%v: %d %v", body["kind"], code, b)
		}
	}
	change(map[string]any{"kind": "add-builtins"})
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	plain := strings.Split(f.enrollAt("Plain", "02:00:00:00:00:5b", "office", []any{serving}, map[string]int{"5g": 44}), "|")
	other := strings.Split(f.enrollAt("Other", "02:00:00:00:00:5c", "office", []any{serving}, map[string]int{"5g": 149}), "|")
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
	ask := func(ap string, body map[string]any) (int, map[string]any) {
		body["kind"] = "client-test"
		return f.do("POST", "/v1/aps/"+ap+"/actions", "griff", body)
	}
	refused := func(name string, body map[string]any, why string) {
		t.Helper()
		if code, b := ask(plain[0], body); code != 400 || !strings.Contains(b["error"].(string), why) {
			t.Errorf("%s: %d %v", name, code, b)
		}
	}

	// An agent that has not said it can is not asked; nor is an AP with no
	// scan radio asked for a band.
	refused("an agent that has not said so", map[string]any{"network": "sweet", "band": "wire"}, "has not said it can test over the wire")
	refused("through the air, with no scan radio", map[string]any{"network": "sweet", "band": "5g"}, "scan radio")
	say(plain[1], map[string]any{"wire_tests": true})
	refused("a network the AP does not offer", map[string]any{"network": "guest", "band": "wire"}, "one of the networks this AP offers")
	refused("on another AP", map[string]any{"network": "sweet", "band": "wire", "on": other[0]}, "from the AP itself")

	code, body := ask(plain[0], map[string]any{"network": "sweet", "band": "wire"})
	if code != 200 {
		t.Fatalf("asked: %d %v", code, body)
	}
	a := body["action"].(map[string]any)
	p := a["params"].(map[string]any)
	if p["network"] != "sweet" || p["ssid"] != "Sweet Spot" || p["band"] != "wire" || p["on"] != plain[0] || p["ap"] != plain[0] ||
		p["bssids"] != nil || p["eap"] != nil || p["speed"] != nil {
		t.Fatalf("asked = %v", a)
	}
	_, _, got := f.apDo("GET", "/v1/ap/actions", plain[1], nil, nil)
	if hp := got["actions"].([]any)[0].(map[string]any)["params"].(map[string]any); hp["band"] != "wire" || hp["ap"] != plain[0] {
		t.Fatalf("handed = %v", got)
	}

	// What it found: where it was made, the address, the gateway by ARP
	// and how long that took, and the name looked up.
	yes := true
	found := func(outcome string) map[string]any {
		d := map[string]any{
			"v": 1, "ssid": "Sweet Spot", "band": "wire", "client": "0e:00:00:5b:00:20", "outcome": outcome, "stage": "done", "reason": "",
			"summary": "passed: over the wire, address in 2.0 s, gateway answers ARP in 1.6 ms, DNS 35 ms", "took_ms": 2052,
			"wire": map[string]any{"device": "eth1", "vlan": 20, "tagged": true},
			"steps": []any{
				map[string]any{"t": 1, "stage": "dhcp", "what": "asks for an address (discover)"},
				map[string]any{"t": 2012, "stage": "dhcp", "what": "given 192.168.20.113 by 192.168.20.254", "ok": yes, "ms": 2012},
				map[string]any{"t": 2014, "stage": "gateway", "what": "192.168.20.1, its gateway, is there, at 28:e7:1d:ca:29:13", "ok": yes, "ms": 1.6},
			},
			"dhcp":    map[string]any{"ms": 2012, "address": "192.168.20.113", "mask": 24, "router": "192.168.20.1", "dns": []string{"192.168.20.254"}, "server": "192.168.20.254", "lease": 86400},
			"gateway": map[string]any{"address": "192.168.20.1", "sent": 0, "got": 0, "min": nil, "avg": nil, "max": nil, "arp": true, "mac": "28:e7:1d:ca:29:13", "arp_ms": 1.6},
			"dns":     map[string]any{"server": "192.168.20.254", "name": "example.com", "ms": 35, "rcode": 0, "answers": 1, "address": "64.226.122.113"},
		}
		if outcome != "passed" {
			d["stage"], d["reason"], d["summary"] = "dhcp", "no DHCP server gave it an address", "failed at dhcp: no DHCP server gave it an address"
			delete(d, "dhcp")
			delete(d, "gateway")
			delete(d, "dns")
		}
		return d
	}
	done := func(id any, tok string, d map[string]any) (int, map[string]any) {
		code, _, b := f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%v", id), tok, map[string]any{"ok": d["outcome"] == "passed", "result": d["summary"], "detail": d}, nil)
		return code, b
	}
	for name, spoil := range map[string]func(map[string]any){
		"a wire with no device":     func(d map[string]any) { d["wire"].(map[string]any)["device"] = "" },
		"a VLAN there is none of":   func(d map[string]any) { d["wire"].(map[string]any)["vlan"] = 5000 },
		"an ARP answer before time": func(d map[string]any) { d["gateway"].(map[string]any)["arp_ms"] = -1 },
	} {
		d := found("passed")
		spoil(d)
		if code, b := done(a["id"], plain[1], d); code != 400 {
			t.Errorf("%s: %d %v", name, code, b)
		}
	}
	if code, b := done(a["id"], plain[1], found("passed")); code != 200 {
		t.Fatalf("done: %d %v", code, b)
	}
	_, body = f.do("GET", "/v1/aps/"+plain[0]+"/actions", "griff", nil)
	kept := body["actions"].([]any)[0].(map[string]any)["detail"].(map[string]any)
	if w := kept["wire"].(map[string]any); kept["band"] != "wire" || w["device"] != "eth1" || w["vlan"] != float64(20) || w["tagged"] != true ||
		kept["gateway"].(map[string]any)["arp_ms"] != 1.6 || kept["client"] != "0e:00:00:5b:00:20" {
		t.Fatalf("kept = %v", kept)
	}

	// On a schedule: an AP with no scan radio tests nothing until a folder
	// asks for the wire; then each of its networks, over the wire. One
	// whose agent cannot is not asked.
	change(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.every", "value": 15})
	say(plain[1], map[string]any{"wire_tests": true})
	say(other[1], nil)
	if got := f.api.assure(f.now()); len(got) != 0 {
		t.Fatalf("asked with no scan radio and no wire: %v", got)
	}
	// It is shown as one that can test, and is asked for nothing in turn.
	_, view := f.do("GET", "/v1/assurance", "griff", nil)
	if tr := view["testers"].([]any); len(tr) != 1 || tr[0].(map[string]any)["ap"] != plain[0] || tr[0].(map[string]any)["every"] != float64(0) ||
		tr[0].(map[string]any)["wire"] != false || tr[0].(map[string]any)["can_wire"] != true || tr[0].(map[string]any)["things"] != float64(1) {
		t.Fatalf("testers with no wire = %v", view["testers"])
	}
	change(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.wire", "value": true})
	say(plain[1], map[string]any{"wire_tests": true})
	say(other[1], nil)
	_, _, cfg := f.apDo("GET", "/v1/ap/config", plain[1], nil, nil)
	if _, ok := cfg["config"].(map[string]any)["assurance"]; ok {
		t.Fatalf("the schedule is in the AP's config: %v", cfg["config"])
	}
	_, view = f.do("GET", "/v1/assurance", "griff", nil)
	testers := view["testers"].([]any)
	if tr := testers[0].(map[string]any); len(testers) != 1 || tr["ap"] != plain[0] || tr["things"] != float64(1) || tr["wire"] != true || tr["air"] != false || tr["every"] != float64(15) {
		t.Fatalf("testers = %v", testers)
	}
	fail := func() {
		t.Helper()
		got := f.api.assure(f.now())
		if len(got) != 1 || got[0].AP != plain[0] || got[0].Actor != "aeolus" {
			t.Fatalf("asked %v", got)
		}
		var p clientTestAsk
		json.Unmarshal(got[0].Params, &p)
		if p.Network != "sweet" || p.Band != "wire" || p.On != plain[0] || p.AP != plain[0] {
			t.Fatalf("asked for %s", got[0].Params)
		}
		f.apDo("GET", "/v1/ap/actions", plain[1], nil, nil)
		if code, b := done(got[0].ID, plain[1], found("failed")); code != 200 {
			t.Fatalf("done: %d %v", code, b)
		}
	}
	// It fails, is tried again within minutes, and fails again: an alert.
	fail()
	*f.skew += assureRetry
	say(plain[1], map[string]any{"wire_tests": true})
	fail()
	_, al := f.do("GET", "/v1/alerts", "griff", nil)
	said := ""
	for _, x := range al["alerts"].([]any) {
		if m := x.(map[string]any); m["kind"] == "client-test" {
			said = m["ap"].(string) + ": " + m["message"].(string)
		}
	}
	if want := plain[0] + ": Sweet Spot does not work here over the wire, tried from this AP's uplink as a client would: it failed at dhcp: no DHCP server gave it an address"; said != want {
		t.Fatalf("the alert:\n got %s\nwant %s", said, want)
	}
	_, view = f.do("GET", "/v1/assurance", "griff", nil)
	if ch := view["checks"].([]any)[0].(map[string]any); ch["band"] != "wire" || ch["on"] != plain[0] || ch["failing"] != true {
		t.Fatalf("checks = %v", view["checks"])
	}
}
