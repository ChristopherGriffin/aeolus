package api

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// How the clients' own joins go (0127): counted by network from the
// attempts the APs record (0118), shown with the assurance of a folder, and
// an alert where several clients cannot get on a network at an AP.
func TestClientJoins(t *testing.T) {
	f := newFixture(t)
	change := func(body map[string]any) {
		t.Helper()
		if code, b := f.change("griff", body); code != 200 {
			t.Fatalf("%v: %d %v", body["kind"], code, b)
		}
	}
	change(map[string]any{"kind": "add-builtins"})
	change(map[string]any{"kind": "add-folder", "tree": "locations", "node": "shed", "name": "Shed", "parent": "house"})
	change(map[string]any{"kind": "grant", "account": "tenant", "tree": "locations", "node": "shed", "role": "viewer"})
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	enroll := func(name, mac, folder string, radios []any, macs []string) (string, string) {
		t.Helper()
		code, _, body := f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": mac, "hostname": name, "radios": radios, "macs": macs}, nil)
		if code != 201 {
			t.Fatalf("enroll %s: %d %v", name, code, body)
		}
		ap, token := body["ap"].(string), body["token"].(string)
		change(map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": folder})
		return ap, token
	}
	const scan = "02:00:00:00:0a:f0"
	enroll("Scanner", "02:00:00:00:00:2a", "shed", []any{serving, spare}, []string{"02:00:00:00:00:2a", scan})
	hall, token := enroll("Hall", "02:00:00:00:00:2b", "office", []any{serving}, nil)

	now := time.Now()
	seq := 0
	attempt := func(mac, outcome, stage, reason string, took int, ago time.Duration) map[string]any {
		seq++
		return map[string]any{"mac": mac, "bss": "phy0-ap0", "network": "sweet", "ssid": "Sweet Spot", "band": "5g",
			"started": now.Add(-ago).UnixMilli() + int64(seq), "took_ms": took, "outcome": outcome, "stage": stage, "reason": reason, "events": []any{}}
	}
	post := func(list ...any) {
		t.Helper()
		code, _, body := f.apDo("POST", "/v1/ap/connections", token, map[string]any{"connections": list}, nil)
		if code != 200 || body["recorded"] != float64(len(list)) {
			t.Fatalf("connections: %d %v", code, body)
		}
	}
	client := func(n int) string { return fmt.Sprintf("72:bb:66:12:2a:%02x", n) }
	const noDHCP = "no answer to its 4 DHCP requests"
	post(
		// Two that cannot get an address, one of them twice.
		// Reasons that differ only in a figure are one reason, said as the
		// latest had it.
		attempt(client(1), "failed", "dhcp", "no answer to its 3 DHCP requests", 8000, 50*time.Minute),
		attempt(client(1), "failed", "dhcp", noDHCP, 8000, 40*time.Minute),
		attempt(client(2), "failed", "dhcp", noDHCP, 8000, 30*time.Minute),
		// One that failed, and then got on; one that got on at once.
		attempt(client(3), "failed", "dhcp", noDHCP, 8000, 30*time.Minute),
		attempt(client(3), "online", "internet", "", 400, 29*time.Minute),
		attempt(client(4), "online", "dns", "", 600, 20*time.Minute),
		// One that left of its own accord: it tried, and failed nothing.
		attempt(client(5), "left", "assoc", "before it was let on: left", 100, 10*time.Minute),
		// An AP's own scan radio, failing a test (0121): no client's.
		attempt(scan, "failed", "dhcp", noDHCP, 8000, 9*time.Minute),
		attempt(scan, "failed", "dhcp", noDHCP, 8000, 8*time.Minute),
		// And one from before the time an alert looks over.
		attempt(client(9), "failed", "key", "wrong passphrase", 4000, 3*time.Hour),
	)

	joins := func(as, query string) []any {
		t.Helper()
		code, body := f.do("GET", "/v1/assurance"+query, as, nil)
		if code != 200 {
			t.Fatalf("assurance as %s: %d %v", as, code, body)
		}
		return body["joins"].([]any)
	}
	got := joins("griff", "?hours=2")
	if len(got) != 1 {
		t.Fatalf("joins = %v", got)
	}
	n := got[0].(map[string]any)
	if n["network"] != "sweet" || n["ssid"] != "Sweet Spot" || n["clients"] != float64(5) || n["online"] != float64(2) || n["failed"] != float64(2) ||
		n["attempts"] != float64(7) || n["attempts_failed"] != float64(4) || n["typical_ms"] != float64(600) || n["failing"] != false {
		t.Fatalf("the network = %v", n)
	}
	stops := n["stops"].([]any)
	if s := stops[0].(map[string]any); len(stops) != 1 || s["stage"] != "dhcp" || s["reason"] != noDHCP || s["attempts"] != float64(4) || s["clients"] != float64(3) {
		t.Fatalf("where they stopped = %v", stops)
	}
	aps := n["aps"].([]any)
	if a := aps[0].(map[string]any); len(aps) != 1 || a["ap"] != hall || a["name"] != "Hall" || a["clients"] != float64(5) || a["failed"] != float64(2) || a["failing"] != false {
		t.Fatalf("by AP = %v", aps)
	}
	// Over a longer time the older failure counts too; below another folder,
	// and for one who may not view the AP, there is nothing.
	if got := joins("griff", ""); got[0].(map[string]any)["clients"] != float64(6) || got[0].(map[string]any)["failed"] != float64(3) {
		t.Fatalf("over a day = %v", got)
	}
	if got := joins("griff", "?under=shed"); len(got) != 0 {
		t.Fatalf("below another folder = %v", got)
	}
	if got := joins("tenant", ""); len(got) != 0 {
		t.Fatalf("to one who may not view the AP = %v", got)
	}

	alerted := func() string {
		t.Helper()
		_, body := f.do("GET", "/v1/alerts", "griff", nil)
		for _, a := range body["alerts"].([]any) {
			if m := a.(map[string]any); m["kind"] == "joins-failing" {
				return m["severity"].(string) + " " + m["ap"].(string) + " " + m["key"].(string) + ": " + m["message"].(string)
			}
		}
		return ""
	}
	// Two let on that got no further are those clients' own trouble.
	if got := alerted(); got != "" {
		t.Fatalf("two clients: %s", got)
	}
	// Clients with a wrong passphrase were never let on: theirs is their
	// own, however many they are.
	post(attempt(client(6), "failed", "key", "wrong passphrase", 4000, 2*time.Minute),
		attempt(client(10), "failed", "key", "wrong passphrase", 4000, 2*time.Minute),
		attempt(client(11), "failed", "key", "wrong passphrase", 4000, 2*time.Minute),
		attempt(client(12), "failed", "signin", "the server refused the sign-in", 900, 2*time.Minute))
	if got := alerted(); got != "" {
		t.Fatalf("clients never let on: %s", got)
	}
	if got := joins("griff", "?hours=2")[0].(map[string]any); got["failing"] != false || got["failed"] != float64(6) {
		t.Fatalf("with those, the assurance = %v", got)
	}
	// A third let on that got no further: three of the five let on.
	post(attempt(client(7), "failed", "gateway", "its gateway never answered", 4000, 2*time.Minute))
	want := "warning " + hall + " joins-failing:sweet: 3 of the 5 clients let on to Sweet Spot here in the last hour got no further; most stopped at DHCP: " + noDHCP
	if got := alerted(); got != want {
		t.Fatalf("three of five:\n got %s\nwant %s", got, want)
	}
	if got := joins("griff", "?hours=2")[0].(map[string]any); got["failing"] != true || got["aps"].([]any)[0].(map[string]any)["failing"] != true {
		t.Fatalf("failing, with the assurance = %v", got)
	}
	// More clients getting on makes them fewer than half, and it ends.
	post(attempt(client(8), "online", "dns", "", 300, time.Minute), attempt(client(13), "online", "dns", "", 300, time.Minute))
	if got := alerted(); got != "" {
		t.Fatalf("three of seven: %s", got)
	}
	// A fourth makes them half again; and two of them getting on ends it.
	post(attempt(client(14), "failed", "dns", "no answer to its 2 DNS lookups", 4000, time.Minute))
	if got := alerted(); !strings.Contains(got, "4 of the 8 clients let on") {
		t.Fatalf("four of eight: %s", got)
	}
	post(attempt(client(1), "online", "dns", "", 500, time.Second), attempt(client(2), "connected", "key", "", 0, time.Second))
	if got := alerted(); got != "" {
		t.Fatalf("after two of them got on: %s", got)
	}
}
