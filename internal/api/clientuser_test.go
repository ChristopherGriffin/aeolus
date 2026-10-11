package api

import (
	"strings"
	"testing"
)

// A client's user (0113): who it signed in as on WPA Enterprise, kept with
// the report, and held to 64 printable characters.
func TestClientUser(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	client := func(user string) map[string]any {
		return map[string]any{"mac": "92:8e:1b:2e:aa:2c", "network": "sweet", "ssid": "Sweet Spot", "band": "5g", "signal": -60, "signal_avg": -60,
			"tx_rate": 400, "rx_rate": 300, "connected": 60, "inactive_ms": 10, "rx_bytes": 1, "tx_bytes": 1, "rx_packets": 1, "tx_packets": 1, "user": user}
	}
	post := func(user string) int {
		code, _, _ := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "clients": []any{client(user)}}, nil)
		return code
	}
	if code := post("aeolus-test"); code != 200 {
		t.Fatalf("a user: %d", code)
	}
	_, cfg := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	c := cfg["condition"].(map[string]any)["state"].(map[string]any)["report"].(map[string]any)["clients"].([]any)[0].(map[string]any)
	if c["user"] != "aeolus-test" {
		t.Fatalf("client = %v", c)
	}
	if code := post(strings.Repeat("u", 65)); code != 400 {
		t.Fatalf("65 characters: %d", code)
	}
	if code := post("café"); code != 400 {
		t.Fatalf("not printable ASCII: %d", code)
	}
}
