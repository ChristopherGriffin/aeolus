package api

import (
	"testing"
)

// An AP's scan radio, joining a network as a client for a test (0121), is
// among the clients of the AP it joins. It is said to be the AP's, wherever
// clients are listed: it is no device of anyone's.
func TestAnAPsOwnTestClientIsSaidSo(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	if code, b := f.change("griff", map[string]any{"kind": "add-folder", "tree": "locations", "node": "shed", "name": "Shed", "parent": "house"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	if code, b := f.change("griff", map[string]any{"kind": "grant", "account": "tenant", "tree": "locations", "node": "office", "role": "viewer"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	enroll := func(name, mac, folder string, radios []any, macs []string) (string, string) {
		t.Helper()
		code, _, body := f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": mac, "hostname": name, "radios": radios, "macs": macs}, nil)
		if code != 201 {
			t.Fatalf("enroll %s: %d %v", name, code, body)
		}
		ap, token := body["ap"].(string), body["token"].(string)
		if code, b := f.change("griff", map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": folder}); code != 200 {
			t.Fatalf("adopt %s: %d %v", name, code, b)
		}
		return ap, token
	}
	// The scan radio's MAC is one of its AP's, said in another case.
	const scan, link, phone = "02:00:00:00:0a:f0", "02:00:00:00:0b:f0", "02:00:00:00:00:77"
	enroll("Scanner", "02:00:00:00:00:2a", "shed", []any{serving, spare}, []string{"02:00:00:00:00:2a", "02:00:00:00:0A:F0"})
	// An AP with no scan radio makes no tests: its MAC among the clients is
	// something else.
	enroll("Repeater", "02:00:00:00:00:2c", "shed", []any{serving}, []string{"02:00:00:00:00:2c", link})
	plain, token := enroll("Plain", "02:00:00:00:00:2b", "office", []any{serving}, nil)

	_, _, cfg := f.apDo("GET", "/v1/ap/config", token, nil, nil)
	client := func(m string) map[string]any {
		return map[string]any{"mac": m, "network": "sweet", "ssid": "Sweet Spot", "band": "5g", "signal": -85, "signal_avg": -85,
			"tx_rate": 6, "rx_rate": 6, "connected": 60, "inactive_ms": 10, "rx_bytes": 5000000, "tx_bytes": 5000000, "rx_packets": 1, "tx_packets": 1}
	}
	said := client(phone)
	said["test_client"] = true
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": cfg["version"], "uptime": 60,
		"clients": []any{client(scan), client(link), said}}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, b)
	}

	// In the AP's own report, kept as it came, it is marked and not guessed
	// at; an AP cannot mark a client so itself.
	_, kept := f.do("GET", "/v1/aps/"+plain+"/config", "griff", nil)
	for _, c := range kept["condition"].(map[string]any)["state"].(map[string]any)["report"].(map[string]any)["clients"].([]any) {
		m := c.(map[string]any)
		if _, kind := m["kind"]; (m["test_client"] == true) != (m["mac"] == scan) || (m["mac"] == scan && kind) {
			t.Fatalf("on now = %v", m)
		}
	}

	testers := func(as string) map[string]any {
		t.Helper()
		code, body := f.do("GET", "/v1/clients", as, nil)
		if code != 200 {
			t.Fatalf("clients as %s: %d %v", as, code, body)
		}
		out := map[string]any{}
		for _, c := range body["clients"].([]any) {
			m := c.(map[string]any)
			out[m["mac"].(string)] = m["tester"]
		}
		return out
	}
	if got := testers("griff"); len(got) != 3 || got[scan] != "Scanner" || got[link] != nil || got[phone] != nil {
		t.Fatalf("everyone = %v", got)
	}
	// One who may not view the AP it is of is told it is an AP's, not which.
	if got := testers("tenant"); len(got) != 3 || got[scan] != "an AP" || got[link] != nil || got[phone] != nil {
		t.Fatalf("everyone, to one who may not view its AP = %v", got)
	}

	// Its journey says whose it is and holds nothing to look into; a phone's
	// with the same weak signal does.
	_, own := f.do("GET", "/v1/clients/"+scan, "griff", nil)
	if own["tester"] != "Scanner" || len(own["issues"].([]any)) != 0 || len(own["sessions"].([]any)) != 1 {
		t.Fatalf("its journey = %v", own)
	}
	_, other := f.do("GET", "/v1/clients/"+phone, "griff", nil)
	if _, said := other["tester"]; said || len(other["issues"].([]any)) == 0 {
		t.Fatalf("a phone's journey = %v", other)
	}

	// Whose it is is said only to one who sees something of it: not to one
	// with no right to any AP here, nor of an AP's MAC that was never a
	// client, which would say which MACs are an AP's.
	if _, none := f.do("GET", "/v1/clients/"+scan, "office", nil); none["tester"] != nil {
		t.Fatalf("to one who sees none of it = %v", none)
	}
	if _, never := f.do("GET", "/v1/clients/02:00:00:00:00:2a", "tenant", nil); never["tester"] != nil {
		t.Fatalf("of an AP's MAC that was no client = %v", never)
	}
	if _, part := f.do("GET", "/v1/clients/"+scan, "tenant", nil); part["tester"] != "an AP" {
		t.Fatalf("to one who sees it and not its AP = %v", part)
	}

	// And among those that used the most.
	more := func(m string) map[string]any {
		c := client(m)
		c["connected"], c["rx_bytes"], c["tx_bytes"] = 120, 90000000, 90000000
		return c
	}
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": cfg["version"], "uptime": 120,
		"clients": []any{more(scan), more(link), more(phone)}}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, b)
	}
	_, usage := f.do("GET", "/v1/usage", "griff", nil)
	top := map[string]any{}
	for _, c := range usage["clients"].([]any) {
		m := c.(map[string]any)
		top[m["mac"].(string)] = m["tester"]
	}
	if len(top) != 3 || top[scan] != "Scanner" || top[phone] != nil || top[link] != nil {
		t.Fatalf("top clients = %v", usage["clients"])
	}
}
