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
	in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 9}
	s := seen(20*time.Second, 8)
	old := now.Add(-20 * time.Minute)
	s.RunningAt = &old
	in.Latest.Seen = s
	if got := kinds(For(in)); got != "warning:behind" {
		t.Fatalf("alerts = %s", got)
	}
}

func TestFromTheReport(t *testing.T) {
	report := map[string]any{
		"wireless_missing": true,
		"agent":            map[string]any{"update": map[string]any{"version": "v0.57.0", "state": "rolled-back", "why": "it did not confirm itself within 300 s"}},
		"transports": map[string]any{
			"aeolus-50": map[string]any{"active": "none"},
			"lab":       map[string]any{"active": "fallback"},
		},
		"vxlan": map[string]any{
			"tunnels": []any{
				map[string]any{"vni": 50, "peer": "1.1.1.2", "probe": map[string]any{"verdict": "down"}},
				map[string]any{"vni": 60, "peer": "1.1.1.2", "standby": true, "probe": map[string]any{"verdict": "down"}},
			},
			"loops": []any{map[string]any{"port": "lan3", "vni": 30}},
		},
		"uplink_vlans": []any{map[string]any{"vlan": 30, "verdict": "silent"}, map[string]any{"vlan": 20, "verdict": "present"}},
		"dhcp":         map[string]any{"guest": map[string]any{"answered": 0, "unanswered": 4}},
		"time":         map[string]any{"synced": false},
	}
	raw, _ := json.Marshal(report)
	in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 7}
	in.Latest.Seen = seen(10*time.Second, 7)
	in.Latest.State = &conditions.State{At: now.Add(-2 * time.Minute), Version: 7, Report: raw}
	want := "critical:wireless-missing critical:no-transport critical:loop warning:agent-update warning:on-fallback warning:tunnel-down warning:vlan-silent warning:dhcp-silent info:clock"
	if got := kinds(For(in)); got != want {
		t.Fatalf("alerts =\n%s\nwant\n%s", got, want)
	}
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
