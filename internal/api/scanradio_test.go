package api

import (
	"strings"
	"testing"
)

// How an AP's scan radio is doing (0126): kept with the report, shown with
// its health, and an alert where it hears nothing and nothing is bringing
// it back.
func TestScanRadioState(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	post := func(scan map[string]any) int {
		code, _, _ := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "scan_radio": scan}, nil)
		return code
	}
	alerted := func() string {
		kinds := alertKinds(t, f)
		if !strings.Contains(kinds, "scan-radio") {
			return ""
		}
		_, body := f.do("GET", "/v1/alerts", "griff", nil)
		for _, a := range body["alerts"].([]any) {
			if m := a.(map[string]any); m["kind"] == "scan-radio" {
				return m["severity"].(string) + ": " + m["message"].(string)
			}
		}
		return "?"
	}

	// Hearing, and brought back once: no alert, and it is with the health.
	if code := post(map[string]any{"state": "clients", "networks": 41, "looked_ago": 120, "heals": 1, "heal_failed": 0, "healed_ago": 3600}); code != 200 {
		t.Fatalf("a scan radio that hears: %d", code)
	}
	if got := alerted(); got != "" {
		t.Fatalf("a scan radio that hears: %s", got)
	}
	_, hl := f.do("GET", "/v1/aps/"+ap+"/health", "griff", nil)
	if now := hl["now"].(map[string]any)["scan_radio"].(map[string]any); now["networks"] != float64(41) || now["heals"] != float64(1) || now["state"] != "clients" {
		t.Fatalf("health now = %v", hl["now"])
	}

	// Deaf, with an airscan that brings it back: it sees to it, no alert;
	// nor while it is at it.
	for name, scan := range map[string]map[string]any{
		"deaf, to be brought back":    {"state": "bss", "networks": 0, "looked_ago": 5, "heals": 1, "heal_failed": 0},
		"being brought back":          {"state": "healing", "networks": 0, "looked_ago": 5, "heals": 2, "heal_failed": 1},
		"lent to a test":              {"state": "lent", "networks": 38, "looked_ago": 20, "heals": 0, "heal_failed": 0},
		"a look that heard none, old": {"state": "clients", "networks": 0, "looked_ago": 4000},
	} {
		if code := post(scan); code != 200 {
			t.Fatalf("%s: %d", name, code)
		}
		if got := alerted(); got != "" {
			t.Errorf("%s: %s", name, got)
		}
	}

	// Deaf, and not brought back; deaf, with an airscan that brings none
	// back; airscan gone; no radio for it.
	for name, c := range map[string]struct {
		scan map[string]any
		want string
	}{
		"binding did not help": {map[string]any{"state": "clients", "networks": 0, "looked_ago": 30, "heals": 3, "heal_failed": 1}, "binding its driver again did not bring it back"},
		"an older airscan":     {map[string]any{"state": "clients", "networks": 0, "looked_ago": 30}, "does not bring a deaf radio back"},
		"airscan gone":         {map[string]any{"state": "missing"}, "airscan, which runs its scan radio, does not answer"},
		"no radio":             {map[string]any{"state": "waiting"}, "is not there for airscan to use"},
	} {
		if code := post(c.scan); code != 200 {
			t.Fatalf("%s: %d", name, code)
		}
		if got := alerted(); !strings.HasPrefix(got, "warning: ") || !strings.Contains(got, c.want) {
			t.Errorf("%s: %q", name, got)
		}
	}

	// What it may not say.
	for name, scan := range map[string]map[string]any{
		"a state of its own":   {"state": "napping"},
		"no state":             {"networks": 3},
		"a negative count":     {"state": "clients", "networks": -1},
		"a negative time":      {"state": "clients", "looked_ago": -5},
		"heals out of measure": {"state": "clients", "heals": 5000000},
	} {
		if code := post(scan); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
}
