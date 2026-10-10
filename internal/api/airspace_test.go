package api

import "testing"

// GET /v1/airspace (0106): the networks the APs hear, once each by BSSID,
// rogues first; a BSSID the folders' rogues.known name is known, raises no
// alert, and re-versions no AP.
func TestAirspace(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	report := map[string]any{"version": version, "uptime": 60, "bssids": []any{"02:00:00:00:00:01"},
		"rrm": map[string]any{"address": "192.168.1.38", "advertised": []any{}, "neighbours": []any{}, "ratings": []any{}, "moves": []any{},
			"others": []any{
				map[string]any{"bssid": "02:00:00:00:00:01", "ssid": "Sweet Spot", "band": "5g", "channel": 36, "signal": -50, "ago": 10},
				map[string]any{"bssid": "e4:d1:24:0d:e6:11", "ssid": "Sweet Spot", "band": "2g", "channel": 11, "signal": -61, "ago": 20},
				map[string]any{"bssid": "e4:d1:24:0d:e6:12", "ssid": "Sweet Spot", "band": "2g", "channel": 6, "signal": -70, "ago": 20},
				map[string]any{"bssid": "5c:83:6c:77:10:c0", "ssid": "Neighbour", "band": "2g", "channel": 6, "signal": -79, "ago": 20},
			}}}
	post := func() {
		t.Helper()
		if code, _, body := f.apDo("POST", "/v1/ap/state", token, report, nil); code != 200 {
			t.Fatalf("state: %d %v", code, body)
		}
	}
	post()
	heard := func(who string) (string, map[string]any) {
		t.Helper()
		code, body := f.do("GET", "/v1/airspace?under=office", who, nil)
		if code != 200 {
			t.Fatalf("%d %v", code, body)
		}
		kinds := ""
		for _, n := range body["networks"].([]any) {
			m := n.(map[string]any)
			kinds += m["bssid"].(string)[12:] + "=" + m["kind"].(string) + " "
		}
		return kinds, body
	}
	rogues := func() int {
		t.Helper()
		_, body := f.do("GET", "/v1/alerts", "griff", nil)
		n := 0
		for _, a := range body["alerts"].([]any) {
			if a.(map[string]any)["kind"] == "rogue" {
				n++
			}
		}
		return n
	}
	if kinds, body := heard("griff"); kinds != "e6:11=rogue e6:12=rogue 00:01=aeolus 10:c0=other " {
		t.Fatalf("kinds = %s", kinds)
	} else if first := body["networks"].([]any)[0].(map[string]any)["heard_by"].([]any)[0].(map[string]any); first["ap"] != ap || first["signal"] != -61.0 {
		t.Fatalf("heard by = %v", first)
	} else if c := body["counts"].(map[string]any); c["rogue"] != 2.0 || c["known"] != 0.0 || c["aeolus"] != 1.0 || c["other"] != 1.0 {
		t.Fatalf("counts = %v", c)
	}
	if n := rogues(); n != 2 {
		t.Fatalf("%d rogue alerts", n)
	}
	// Known on its folder, in capitals: known, and no alert.
	if code, body := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "rogues.known", "value": []any{"E4:D1:24:0D:E6:12"}}); code != 200 {
		t.Fatalf("known: %d %v", code, body)
	}
	if kinds, _ := heard("griff"); kinds != "e6:11=rogue e6:12=known 00:01=aeolus 10:c0=other " {
		t.Fatalf("kinds = %s", kinds)
	}
	if n := rogues(); n != 1 {
		t.Fatalf("%d rogue alerts", n)
	}
	// The manager's own: not in the AP's config, and its version stands.
	_, cfg := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	if doc, _ := cfg["document"].(map[string]any); doc["rogues"] != nil || cfg["version"] != version {
		t.Fatalf("config = version %v, rogues %v", cfg["version"], doc["rogues"])
	}
	// office has no role in Locations, so it hears nothing.
	if kinds, _ := heard("office"); kinds != "" {
		t.Fatalf("office hears %s", kinds)
	}
}
