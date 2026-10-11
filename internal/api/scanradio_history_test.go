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
	// It hears, and has looked since it last bound the driver.
	hears := func(heals, failed int) map[string]any {
		return map[string]any{"state": "clients", "networks": 40, "looked_ago": 20, "heals": heals, "heal_failed": failed, "healed_ago": 200}
	}
	// It heard nothing at its latest look.
	deaf := func(heals, failed int) map[string]any {
		return map[string]any{"state": "clients", "networks": 0, "looked_ago": 5, "heals": heals, "heal_failed": failed, "healed_ago": 40}
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
		// airscan counts a bind as it makes it, and says how it went only
		// after its next look: a report between the two says no more than
		// it knows, and the next says how it went.
		{map[string]any{"state": "healing", "networks": 40, "looked_ago": 30, "heals": 6, "heal_failed": 0, "healed_ago": 5}, "its scan radio went deaf, and airscan bound its driver again; it has not looked since"},
		{deaf(6, 1), "binding its scan radio's driver again did not bring it back"},
		// Still deaf: each try that does not help is no news.
		{deaf(7, 2), ""},
		{deaf(8, 3), ""},
		{hears(9, 0), "its scan radio hears again"},
		// Deaf, bound and not back, all between two reports.
		{deaf(10, 1), "its scan radio went deaf, and binding its driver again has not brought it back"},
		// Another airscan counts from none: nothing is known to have
		// happened to the radio, deaf as it still is, or hearing.
		{deaf(0, 0), ""},
		{hears(0, 0), ""},
		// airscan gone is said once, however long it stays gone.
		{map[string]any{"state": "missing"}, "airscan, which runs its scan radio, stopped answering"},
		{map[string]any{"state": "missing"}, ""},
		{map[string]any{"state": "missing"}, ""},
		{hears(0, 0), "airscan answers again"},
		{map[string]any{"state": "waiting"}, "its scan radio is not there for airscan to use"},
		{map[string]any{"state": "waiting"}, ""},
		{map[string]any{"state": "missing"}, "airscan, which runs its scan radio, stopped answering"},
		{map[string]any{"state": "waiting"}, "airscan answers again, and has no scan radio to use"},
		{hears(0, 0), "its scan radio is there again"},
		// Lent to a test, and back: that is its work, and nothing happened;
		// nor is a radio a test has, with airscan stopped, there again.
		{map[string]any{"state": "lent", "networks": 40, "looked_ago": 20, "heals": 0, "heal_failed": 0}, ""},
		{hears(0, 0), ""},
		{map[string]any{"state": "waiting"}, "its scan radio is not there for airscan to use"},
		{map[string]any{"state": "lent"}, ""},
		{hears(0, 0), ""},
		// The driver bound, and the radio gone after: both are said.
		{map[string]any{"state": "waiting", "heals": 1, "heal_failed": 0, "healed_ago": 30},
			"its scan radio is not there for airscan to use\nits scan radio went deaf, and airscan bound its driver again; it has not looked since"},
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
