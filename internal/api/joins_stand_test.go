package api

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// A client is counted by what it found last (0127): clients that were on
// and fail since, as when a DHCP server stops answering, are an alert; it
// is so since the third of them failed; a network says the SSID it has
// now; and an AP no one adopted hides no client by calling it its own.
func TestClientJoinsAsTheyStand(t *testing.T) {
	f := newFixture(t)
	change := func(body map[string]any) {
		t.Helper()
		if code, b := f.change("griff", body); code != 200 {
			t.Fatalf("%v: %d %v", body["kind"], code, b)
		}
	}
	change(map[string]any{"kind": "add-builtins"})
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	code, _, body := f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": "02:00:00:00:00:2b", "hostname": "Hall", "radios": []any{serving}}, nil)
	if code != 201 {
		t.Fatalf("enroll: %d %v", code, body)
	}
	hall, token := body["ap"].(string), body["token"].(string)
	change(map[string]any{"kind": "move", "tree": "locations", "node": hall, "parent": "office"})

	now := time.Now()
	seq := 0
	attempt := func(mac, ssid, outcome, stage, reason string, ago time.Duration) map[string]any {
		seq++
		return map[string]any{"mac": mac, "bss": "phy0-ap0", "network": "sweet", "ssid": ssid, "band": "5g",
			"started": now.Add(-ago).UnixMilli() + int64(seq), "took_ms": 400, "outcome": outcome, "stage": stage, "reason": reason, "events": []any{}}
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
	alert := func() map[string]any {
		t.Helper()
		_, body := f.do("GET", "/v1/alerts", "griff", nil)
		for _, a := range body["alerts"].([]any) {
			if m := a.(map[string]any); m["kind"] == "joins-failing" {
				return m
			}
		}
		return nil
	}
	network := func() map[string]any {
		t.Helper()
		_, body := f.do("GET", "/v1/assurance?hours=2", "griff", nil)
		return body["joins"].([]any)[0].(map[string]any)
	}

	// Six clients came online, under the name the network had then.
	for i := 1; i <= 6; i++ {
		post(attempt(client(i), "Zulu", "online", "dns", "", 50*time.Minute))
	}
	if got := network(); got["online"] != float64(6) || got["failed"] != float64(0) || got["ssid"] != "Zulu" || alert() != nil {
		t.Fatalf("all on = %v, %v", got, alert())
	}
	// Then DHCP stops, and they fail one after another, under its new name.
	// Two are no alert; with the third it is one, since that third.
	post(attempt(client(1), "Alpha", "failed", "dhcp", noDHCP, 25*time.Minute), attempt(client(2), "Alpha", "failed", "dhcp", noDHCP, 20*time.Minute))
	if a := alert(); a != nil {
		t.Fatalf("two that fail since: %v", a)
	}
	third := now.Add(-10 * time.Minute)
	post(attempt(client(3), "Alpha", "failed", "dhcp", noDHCP, 10*time.Minute), attempt(client(3), "Alpha", "failed", "dhcp", noDHCP, 5*time.Minute))
	a := alert()
	if a == nil || !strings.Contains(a["message"].(string), "3 of the 6 clients let on to Alpha here") {
		t.Fatalf("three that were on and fail since: %v", a)
	}
	since, err := time.Parse(time.RFC3339Nano, a["since"].(string))
	if err != nil || since.Before(third.Add(-time.Second)) || since.After(third.Add(time.Second)) {
		t.Fatalf("since %v, and the third failed at %v", a["since"], third)
	}
	if got := network(); got["online"] != float64(3) || got["failed"] != float64(3) || got["ssid"] != "Alpha" || got["failing"] != true {
		t.Fatalf("as it stands = %v", got)
	}

	// An AP no one adopted says the clients' MACs are its own scan radio's:
	// they are clients still.
	code, _, body = f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": "02:00:00:00:66:66", "hostname": "Stranger",
		"radios": []any{serving, spare}, "macs": []string{client(1), client(2), client(3)}}, nil)
	if code != 201 {
		t.Fatalf("enroll a stranger: %d %v", code, body)
	}
	if got := network(); alert() == nil || got["failed"] != float64(3) {
		t.Fatalf("with a stranger claiming them = %v, %v", got, alert())
	}
	_, all := f.do("GET", "/v1/clients", "griff", nil)
	for _, c := range all["clients"].([]any) {
		if m := c.(map[string]any); m["tester"] != nil {
			t.Fatalf("a stranger's claim made %v a test client", m)
		}
	}

	// One of them back on, and it is two of six: no alert.
	post(attempt(client(3), "Alpha", "online", "dns", "", time.Second))
	if a := alert(); a != nil {
		t.Fatalf("after one got on again: %v", a)
	}
}
