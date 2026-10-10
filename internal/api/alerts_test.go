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

// Where an AP's alerts go (0101): the folders' notify.* fields, nearest
// first, the URLs sealed in the log and opened only to send, and never in
// an AP's config.
func TestNotifyTarget(t *testing.T) {
	f := newFixture(t)
	for _, op := range []map[string]any{
		{"kind": "set", "tree": "locations", "node": "symtus", "path": "notify.ntfy", "value": "https://ntfy.example/symtus-alerts"},
		{"kind": "set", "tree": "locations", "node": "house", "path": "notify.severity", "value": "warning"},
		{"kind": "set", "tree": "locations", "node": "house", "path": "notify.resolved", "value": false},
	} {
		if code, body := f.change("griff", op); code != 200 {
			t.Fatalf("%v: %d %v", op, code, body)
		}
	}
	got := f.api.notifyTarget(f.log.Snapshot(), "office-ap")
	if got.Ntfy != "https://ntfy.example/symtus-alerts" || got.Webhook != "" || got.Least != "warning" || got.Resolved {
		t.Fatalf("target = %+v", got)
	}
	// Sealed as it is kept, and shown so.
	_, page := f.do("GET", "/v1/trees/locations/nodes/symtus", "griff", nil)
	if v, _ := page["fields"].(map[string]any)["notify.ntfy"].(map[string]any)["value"].(map[string]any); v["sealed"] != true {
		t.Fatalf("notify.ntfy shows %v", page["fields"].(map[string]any)["notify.ntfy"])
	}
	// Not in the AP's config.
	_, cfg := f.do("GET", "/v1/aps/office-ap/config", "griff", nil)
	if doc, _ := cfg["document"].(map[string]any); doc["notify"] != nil {
		t.Fatalf("the AP's document has notify: %v", doc["notify"])
	}
}
