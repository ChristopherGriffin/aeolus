package api

import (
	"strings"
	"testing"
)

// How an AP's RADIUS servers are doing (0114): kept with the report, held to
// its shape, and a silent one is a critical alert on that AP.
func TestRadiusState(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	entry := func(change func(map[string]any)) map[string]any {
		e := map[string]any{"network": "sweet", "server": "192.0.2.10", "port": 1812, "verdict": "up", "rtt_ms": 16.7, "answered_ago": 5,
			"requests": 20, "accepts": 2, "rejects": 1, "timeouts": 0, "bad": 0}
		if change != nil {
			change(e)
		}
		return e
	}
	post := func(radius ...any) int {
		code, _, _ := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "radius": radius}, nil)
		return code
	}
	if code := post(entry(nil)); code != 200 {
		t.Fatalf("a server that answers: %d", code)
	}
	_, cfg := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	got := cfg["condition"].(map[string]any)["state"].(map[string]any)["report"].(map[string]any)["radius"].([]any)[0].(map[string]any)
	if got["server"] != "192.0.2.10" || got["verdict"] != "up" || got["accepts"] != float64(2) || got["rtt_ms"] != 16.7 {
		t.Fatalf("radius = %v", got)
	}
	if kinds := alertKinds(t, f); strings.Contains(kinds, "radius-silent") {
		t.Fatalf("a server that answers: %s", kinds)
	}

	// One that never answered has no round trip and no last answer.
	silent := entry(func(e map[string]any) { e["verdict"], e["rtt_ms"], e["answered_ago"] = "silent", nil, nil })
	if code := post(silent, entry(func(e map[string]any) { e["server"] = "radius.example.com" })); code != 200 {
		t.Fatalf("a silent server, and one by name: %d", code)
	}
	if kinds := alertKinds(t, f); !strings.Contains(kinds, "critical:radius-silent") {
		t.Fatalf("a silent server: %s", kinds)
	}

	for name, change := range map[string]func(map[string]any){
		"a verdict of its own":  func(e map[string]any) { e["verdict"] = "down" },
		"no network":            func(e map[string]any) { e["network"] = "" },
		"no server":             func(e map[string]any) { e["server"] = "" },
		"a server with a path":  func(e map[string]any) { e["server"] = "192.0.2.10/24" },
		"port 0":                func(e map[string]any) { e["port"] = 0 },
		"port 65536":            func(e map[string]any) { e["port"] = 65536 },
		"a negative count":      func(e map[string]any) { e["timeouts"] = -1 },
		"a negative answer":     func(e map[string]any) { e["answered_ago"] = -1 },
		"a round trip too long": func(e map[string]any) { e["rtt_ms"] = 60001 },
	} {
		if code := post(entry(change)); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
	many := make([]any, 65)
	for i := range many {
		many[i] = entry(nil)
	}
	if code := post(many...); code != 400 {
		t.Fatalf("65 entries: %d", code)
	}
}

// alertKinds is the fleet's alerts as "severity:kind" words.
func alertKinds(t *testing.T, f *fixture) string {
	t.Helper()
	code, body := f.do("GET", "/v1/alerts", "griff", nil)
	if code != 200 {
		t.Fatalf("alerts: %d", code)
	}
	var out []string
	for _, a := range body["alerts"].([]any) {
		m := a.(map[string]any)
		out = append(out, m["severity"].(string)+":"+m["kind"].(string))
	}
	return strings.Join(out, " ")
}
