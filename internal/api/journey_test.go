package api

import "testing"

// GET /v1/clients/{mac} (0103): a client's sessions from the state reports
// of the APs the caller may view.
func TestClientJourney(t *testing.T) {
	f := newFixture(t)
	_, token, version := f.adopted()
	client := map[string]any{"mac": "7e:2a:ea:9b:2b:8f", "network": "sweet", "ssid": "Sweet Spot", "band": "5g", "signal": -61,
		"tx_rate": 400, "rx_rate": 300, "connected": 60, "host": "griffs-phone", "address": "192.168.20.80", "dhcp": "ok"}
	for i := 0; i < 2; i++ {
		if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60 + i, "clients": []any{client}}, nil); code != 200 {
			t.Fatalf("state: %d %v", code, body)
		}
	}
	code, body := f.do("GET", "/v1/clients/7E:2A:EA:9B:2B:8F", "griff", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	sessions, _ := body["sessions"].([]any)
	if len(sessions) != 1 || body["host"] != "griffs-phone" {
		t.Fatalf("journey = %v", body)
	}
	if s := sessions[0].(map[string]any); s["name"] != "PumphouseAP" || s["reports"] != 2.0 || s["signal_avg"] != -61.0 {
		t.Fatalf("session = %v", s)
	}
	// office may view no AP, so it sees no session.
	if _, body := f.do("GET", "/v1/clients/7e:2a:ea:9b:2b:8f", "office", nil); len(body["sessions"].([]any)) != 0 {
		t.Fatalf("office sees %v", body["sessions"])
	}
	if code, _ := f.do("GET", "/v1/clients/not-a-mac", "griff", nil); code != 400 {
		t.Fatalf("a bad MAC: %d", code)
	}
}
