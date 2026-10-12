package api

import (
	"testing"
)

// An AP's own way out (0142): kept with its report as it said it; one not
// well said is left out and the report kept; and the internet's target is
// a setting of a folder, a host name or an address.
func TestPathOut(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	way := func(change func(map[string]any)) map[string]any {
		p := map[string]any{
			"interface": "vlan20", "device": "br-lan.20", "address": "192.168.20.7", "mask": 24,
			"gateway":  map[string]any{"address": "192.168.20.1", "state": "up", "ms": 3.8, "ago": 10},
			"dns":      []any{map[string]any{"server": "192.168.20.254", "state": "up", "ms": 3.7, "ago": 10, "rcode": 0}, map[string]any{"server": "192.168.20.253", "state": "silent", "ms": nil, "ago": nil, "rcode": nil}},
			"name":     "one.one.one.one",
			"internet": map[string]any{"target": "one.one.one.one", "address": "1.1.1.1", "state": "up", "ms": 22.9, "ago": 20},
			"manager":  map[string]any{"ms": 41.5},
		}
		if change != nil {
			change(p)
		}
		return p
	}
	kept := func(p map[string]any) map[string]any {
		t.Helper()
		if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "path": p}, nil); code != 200 {
			t.Fatalf("its report: %d %v", code, b)
		}
		_, cfg := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
		got, _ := cfg["condition"].(map[string]any)["state"].(map[string]any)["report"].(map[string]any)["path"].(map[string]any)
		return got
	}
	got := kept(way(nil))
	if got == nil {
		t.Fatal("a way out well said was not kept")
	}
	gw, net, dns := got["gateway"].(map[string]any), got["internet"].(map[string]any), got["dns"].([]any)
	if got["interface"] != "vlan20" || got["address"] != "192.168.20.7" || got["mask"] != float64(24) || gw["state"] != "up" || gw["ms"] != 3.8 ||
		net["target"] != "one.one.one.one" || net["address"] != "1.1.1.1" || net["ms"] != 22.9 || len(dns) != 2 ||
		dns[1].(map[string]any)["state"] != "silent" || dns[1].(map[string]any)["ms"] != nil || got["manager"].(map[string]any)["ms"] != 41.5 || got["name"] != "one.one.one.one" {
		t.Errorf("as kept: %v", got)
	}
	// With no target set, the internet is not said; a name not yet found.
	if p := kept(way(func(p map[string]any) { delete(p, "internet") })); p == nil || p["internet"] != nil {
		t.Errorf("with no internet target: %v", p)
	}
	if p := kept(way(func(p map[string]any) {
		p["internet"] = map[string]any{"target": "one.one.one.one", "state": "unresolved"}
	})); p == nil || p["internet"].(map[string]any)["state"] != "unresolved" {
		t.Errorf("a target with no address yet: %v", p)
	}
	// Not well said: left out, and the report kept all the same.
	for name, change := range map[string]func(map[string]any){
		"a state of its own":    func(p map[string]any) { p["gateway"].(map[string]any)["state"] = "broken" },
		"a time below nothing":  func(p map[string]any) { p["gateway"].(map[string]any)["ms"] = -1 },
		"a time past a minute":  func(p map[string]any) { p["internet"].(map[string]any)["ms"] = 60001 },
		"an answer yet to come": func(p map[string]any) { p["gateway"].(map[string]any)["ago"] = -5 },
		"five name servers": func(p map[string]any) {
			p["dns"] = []any{p["dns"].([]any)[0], p["dns"].([]any)[0], p["dns"].([]any)[0], p["dns"].([]any)[0], p["dns"].([]any)[0]}
		},
		"a name server unnamed":    func(p map[string]any) { p["dns"].([]any)[0].(map[string]any)["server"] = "" },
		"an answer code past 15":   func(p map[string]any) { p["dns"].([]any)[0].(map[string]any)["rcode"] = 16 },
		"a mask past 32":           func(p map[string]any) { p["mask"] = 33 },
		"an address not printable": func(p map[string]any) { p["address"] = "192.168.20.7\n" },
		"a manager below nothing":  func(p map[string]any) { p["manager"].(map[string]any)["ms"] = -1 },
	} {
		if p := kept(way(change)); p != nil {
			t.Errorf("%s: kept as %v", name, p)
		}
	}

	// The internet's target: a host name or an address, of a Locations folder.
	set := func(value any) int {
		code, _ := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "house", "path": "system.internet", "value": value})
		return code
	}
	for _, ok := range []string{"one.one.one.one", "1.1.1.1", "gateway", "a-b.example", "a12345678901234567890123456789012345678901234567890123456789012.example"} {
		if code := set(ok); code != 200 {
			t.Errorf("target %q: %d", ok, code)
		}
	}
	// Nor a name its name servers could not be asked for: an empty label,
	// or one longer than 63.
	long := "a123456789012345678901234567890123456789012345678901234567890123.example"
	for _, bad := range []any{"http://one.one.one.one", "one one", "-one", "one.", "", 7, "a..b", ".one", "one-.one", long} {
		if code := set(bad); code == 200 {
			t.Errorf("target %v was taken", bad)
		}
	}
}
