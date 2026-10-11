package api

import (
	"strings"
	"testing"
)

// What an AP's scan radio went through is in the AP's history (0126): told
// from one report to the next, by airscan's count of the times it bound the
// driver again and by what it is at.
func TestScanRadioHistory(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	post := func(scan map[string]any) {
		t.Helper()
		body := map[string]any{"version": version, "uptime": 60}
		if scan != nil {
			body["scan_radio"] = scan
		}
		if code, _, b := f.apDo("POST", "/v1/ap/state", token, body, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
	}
	history := func() string {
		t.Helper()
		_, tl := f.do("GET", "/v1/aps/"+ap+"/timeline", "griff", nil)
		var said []string
		for _, e := range tl["events"].([]any) {
			if m := e.(map[string]any); m["kind"] == "scan-radio" {
				said = append(said, m["text"].(string))
			}
		}
		return strings.Join(said, "\n")
	}
	hears := func(heals, failed int) map[string]any {
		return map[string]any{"state": "clients", "networks": 40, "looked_ago": 20, "heals": heals, "heal_failed": failed}
	}

	// A first report, one with no scan radio said, and one the same as the
	// last are nothing that happened.
	post(hears(1, 0))
	post(nil)
	post(hears(1, 0))
	post(hears(1, 0))
	if got := history(); got != "" {
		t.Fatalf("nothing happened: %s", got)
	}
	steps := []struct {
		scan map[string]any
		want string
	}{
		{hears(2, 0), "its scan radio went deaf, and airscan brought it back by binding its driver again"},
		{hears(5, 0), "its scan radio went deaf 3 times, and airscan brought it back each time by binding its driver again"},
		{map[string]any{"state": "clients", "networks": 0, "looked_ago": 5, "heals": 6, "heal_failed": 1}, "its scan radio went deaf, and binding its driver again has not brought it back"},
		{hears(7, 0), "its scan radio went deaf, and airscan brought it back by binding its driver again"},
		{map[string]any{"state": "clients", "networks": 0, "looked_ago": 5, "heals": 8, "heal_failed": 2}, "its scan radio went deaf, and binding its driver again has not brought it back"},
		{hears(8, 0), "its scan radio hears again"},
		// Another airscan counts from none: nothing happened to the radio.
		{hears(0, 0), ""},
		{map[string]any{"state": "missing"}, "airscan, which runs its scan radio, stopped answering"},
		{hears(0, 0), "airscan answers again"},
		{map[string]any{"state": "waiting"}, "its scan radio is not there for airscan to use"},
		{map[string]any{"state": "missing"}, "airscan, which runs its scan radio, stopped answering"},
		{map[string]any{"state": "waiting"}, "airscan answers again, and has no scan radio to use"},
		{hears(0, 0), "its scan radio is there again"},
		// Lent to a test, and back: that is its work, and nothing happened.
		{map[string]any{"state": "lent", "networks": 40, "looked_ago": 20, "heals": 0, "heal_failed": 0}, ""},
		{hears(0, 0), ""},
	}
	for i, s := range steps {
		before := history()
		post(s.scan)
		got := history()
		// The newest is first.
		latest := strings.TrimSuffix(strings.TrimSuffix(got, before), "\n")
		if latest != s.want {
			t.Fatalf("step %d: %q, want %q", i, latest, s.want)
		}
	}
}
