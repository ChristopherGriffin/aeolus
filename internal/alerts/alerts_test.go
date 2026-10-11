package alerts

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/conditions"
)

var now = time.Date(2026, 10, 10, 1, 0, 0, 0, time.UTC)

func kinds(as []Alert) string {
	var out []string
	for _, a := range as {
		out = append(out, a.Severity+":"+a.Kind)
	}
	return strings.Join(out, " ")
}

func seen(ago time.Duration, running int64) *conditions.Seen {
	at := now.Add(-ago)
	return &conditions.Seen{At: at, Source: "192.168.20.7", Running: &running, RunningAt: &at}
}

func TestAHealthyAPHasNone(t *testing.T) {
	as := For(Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 7,
		Latest: conditions.Latest{Seen: seen(30*time.Second, 7)}})
	if len(as) != 0 {
		t.Fatalf("alerts = %s", kinds(as))
	}
}

func TestOfflineAfterThreePollsAndNeverUnderFiveMinutes(t *testing.T) {
	in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 7}
	in.Latest.Seen = seen(4*time.Minute, 7)
	if as := For(in); len(as) != 0 {
		t.Fatalf("4 minutes: %s", kinds(as))
	}
	in.Latest.Seen = seen(6*time.Minute, 7)
	as := For(in)
	if kinds(as) != "critical:offline" || !strings.Contains(as[0].Message, "6 minutes") {
		t.Fatalf("6 minutes: %v", as)
	}
	// A slow poll waits longer.
	in.Poll = 5 * time.Minute
	if as := For(in); len(as) != 0 {
		t.Fatalf("6 minutes at a 5 minute poll: %s", kinds(as))
	}
}

func TestHeldRefusedAndFailed(t *testing.T) {
	in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 108,
		Problems: []string{"uplink.bond: taking the bond apart needs spanning tree on", "another"}}
	in.Latest.Seen = seen(10*time.Second, 107)
	in.Latest.Check = &conditions.Check{At: now, Version: 108, Result: "refused", Problems: []string{"network bridge br-lan: stp is \"\""}}
	in.Latest.Apply = &conditions.Apply{At: now, Version: 108, OK: false, Error: "lost the manager within 90 seconds of applying; reverted"}
	as := For(in)
	if got := kinds(as); got != "critical:refused critical:apply-failed warning:held" {
		t.Fatalf("alerts = %s", got)
	}
	if !strings.Contains(as[2].Message, "(and 1 more)") {
		t.Fatalf("held = %q", as[2].Message)
	}
	// An earlier version's refusal is history, not an alert.
	in.Problems = nil
	in.Latest.Check.Version, in.Latest.Apply.Version = 107, 107
	in.Latest.Seen = seen(10*time.Second, 108)
	if as := For(in); len(as) != 0 {
		t.Fatalf("old records: %s", kinds(as))
	}
}

func TestBehindOnlyWhenNothingExplainsIt(t *testing.T) {
	// It reports every few minutes, which refreshes when it said what it
	// runs; behind is measured from when the current version was made.
	in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 9, Since: now.Add(-20 * time.Minute)}
	in.Latest.Seen = seen(20*time.Second, 8)
	as := For(in)
	if got := kinds(as); got != "warning:behind" || !as[0].Since.Equal(in.Since) {
		t.Fatalf("alerts = %s", got)
	}
	// A version made a minute ago is still on its way.
	in.Since = now.Add(-time.Minute)
	if got := kinds(For(in)); got != "" {
		t.Fatalf("new version: %s", got)
	}
}

func TestFromTheReport(t *testing.T) {
	report := map[string]any{
		"wireless_missing": true,
		"agent":            map[string]any{"hash": "aaaa", "update": map[string]any{"version": "v0.57.0", "hash": "bbbb", "state": "rolled-back", "why": "it did not confirm itself within 300 s", "ago": 3600}},
		"transports": map[string]any{
			"aeolus-50": map[string]any{"active": "none"},
			"lab":       map[string]any{"active": "fallback"},
		},
		"vxlan": map[string]any{
			"tunnels": []any{
				map[string]any{"vni": 50, "peer": "1.1.1.2", "probe": map[string]any{"verdict": "down"}},
				map[string]any{"vni": 60, "peer": "1.1.1.2", "standby": true, "probe": map[string]any{"verdict": "down"}},
			},
			"loops": []any{map[string]any{"port": "lan3", "vni": 30, "ago": 600}},
		},
		"uplink_vlans": []any{map[string]any{"vlan": 30, "verdict": "silent"}, map[string]any{"vlan": 20, "verdict": "present"}},
		"dhcp":         map[string]any{"guest": map[string]any{"answered": 0, "unanswered": 4}},
		"radius": []any{
			map[string]any{"network": "staff", "server": "192.0.2.10", "verdict": "silent"},
			map[string]any{"network": "lab", "server": "192.0.2.11", "verdict": "up"},
			map[string]any{"network": "guest", "server": "192.0.2.12", "verdict": "unverified"},
		},
		"time": map[string]any{"synced": false},
	}
	raw, _ := json.Marshal(report)
	in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 7, WantsAgent: "bbbb"}
	in.Latest.Seen = seen(10*time.Second, 7)
	in.Latest.State = &conditions.State{At: now.Add(-2 * time.Minute), Version: 7, Report: raw}
	want := "critical:wireless-missing critical:no-transport critical:loop critical:radius-silent warning:agent-update warning:on-fallback warning:tunnel-down warning:vlan-silent warning:dhcp-silent info:clock"
	as := For(in)
	if got := kinds(as); got != want {
		t.Fatalf("alerts =\n%s\nwant\n%s", got, want)
	}
	// Since, from the report's own ago where it has one: the rollback an
	// hour before the report, the loop ten minutes before it.
	for _, a := range as {
		want := in.Latest.State.At
		switch a.Kind {
		case "agent-update":
			want = want.Add(-time.Hour)
		case "loop":
			want = want.Add(-10 * time.Minute)
		}
		if !a.Since.Equal(want) {
			t.Errorf("%s since %v, want %v", a.Kind, a.Since, want)
		}
	}
	// A failed update to a bundle the AP no longer should run is history.
	in.WantsAgent = "aaaa"
	if got := kinds(For(in)); strings.Contains(got, "agent-update") {
		t.Fatalf("an update no longer wanted: %s", got)
	}
	in.WantsAgent = "bbbb"
	// A report too old to speak for the AP now says nothing.
	in.Latest.State.At = now.Add(-time.Hour)
	if got := kinds(For(in)); got != "" {
		t.Fatalf("old report: %s", got)
	}
}

func TestLandingZone(t *testing.T) {
	as := For(Input{AP: "ap-2", Name: "new", Now: now, Unassigned: true, Latest: conditions.Latest{Seen: seen(time.Hour, 0)}})
	if kinds(as) != "info:unassigned" {
		t.Fatalf("alerts = %s", kinds(as))
	}
}

// A network not one of the APs' that broadcasts one of Aeolus's SSIDs is a
// rogue (0105); one of the APs' own BSSIDs, or another SSID, is not.
func TestRogue(t *testing.T) {
	report := map[string]any{"rrm": map[string]any{"others": []any{
		map[string]any{"bssid": "e4:d1:24:0d:e6:11", "ssid": "Aeolus Lab", "band": "2g", "channel": 11, "signal": -61, "ago": 120},
		map[string]any{"bssid": "02:11:22:33:44:55", "ssid": "Aeolus Lab", "band": "5g", "channel": 36, "signal": -50, "ago": 30},
		map[string]any{"bssid": "e4:d1:24:0d:e6:12", "ssid": "Sweet Spot", "band": "2g", "channel": 6, "signal": -70, "ago": 30},
	}}}
	raw, _ := json.Marshal(report)
	in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 7,
		Fleet: &Fleet{BSSIDs: map[string]bool{"02:11:22:33:44:55": true}, SSIDs: map[string]bool{"Aeolus Lab": true}}}
	in.Latest.Seen = seen(10*time.Second, 7)
	in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
	as := For(in)
	if kinds(as) != "warning:rogue" || as[0].Key != "rogue:e4:d1:24:0d:e6:11" || !strings.Contains(as[0].Message, `"Aeolus Lab" on 2.4 GHz channel 11`) {
		t.Fatalf("alerts = %+v", as)
	}
	if !as[0].Since.Equal(in.Latest.State.At.Add(-2 * time.Minute)) {
		t.Fatalf("since = %v", as[0].Since)
	}
}

// An SSID is matched as an AP hears it: a stranger's Café, heard as
// Caf??, is a rogue of Aeolus's Café.
func TestHeard(t *testing.T) {
	for in, want := range map[string]string{"Café": "Caf??", "Aeolus Lab": "Aeolus Lab", "???": "???", "abcdefghijklmnopqrstuvwxyz0123456789": "abcdefghijklmnopqrstuvwxyz012345"} {
		if got := Heard(in); got != want {
			t.Errorf("Heard(%q) = %q, want %q", in, got, want)
		}
	}
}
