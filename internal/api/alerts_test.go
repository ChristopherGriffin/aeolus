package api

import "testing"

// GET /v1/alerts (0099): what needs attention on the APs the caller may
// view, most urgent first, with counts.
func TestAlerts(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	alertsOf := func(who, query string) []map[string]any {
		t.Helper()
		code, body := f.do("GET", "/v1/alerts"+query, who, nil)
		if code != 200 {
			t.Fatalf("%d %v", code, body)
		}
		var out []map[string]any
		for _, a := range body["alerts"].([]any) {
			if m := a.(map[string]any); m["ap"] == ap {
				out = append(out, m)
			}
		}
		if body["counts"] == nil {
			t.Fatalf("no counts: %v", body)
		}
		return out
	}
	if got := alertsOf("griff", ""); len(got) != 0 {
		t.Fatalf("a healthy AP: %v", got)
	}
	// Its netifd lost network.wireless: critical, as the AP says so.
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 90, "wireless_missing": true}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	got := alertsOf("griff", "")
	if len(got) != 1 || got[0]["kind"] != "wireless-missing" || got[0]["severity"] != "critical" || got[0]["name"] != "PumphouseAP" {
		t.Fatalf("alerts = %v", got)
	}
	// Under a folder it isn't in, none of its alerts.
	if got := alertsOf("griff", "?under=no-such-folder"); len(got) != 0 {
		t.Fatalf("under another folder: %v", got)
	}
	// office has no role in Locations, so it sees no AP's alerts.
	if got := alertsOf("office", ""); len(got) != 0 {
		t.Fatalf("office sees %v", got)
	}
}
