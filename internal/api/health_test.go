package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/conditions"
)

// An AP's health and what happened to it (0119): kept with each state
// report, read back as a series and a history, and an alert where its
// memory runs low.
func TestAPHealthAndHistory(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	radios := func(channel, width int) []any {
		return []any{map[string]any{"radio": "radio0", "band": "5g", "channel": channel, "width": width, "clients": 0, "up": true}}
	}
	health := func(cpu int, free int64) map[string]any {
		return map[string]any{"cpu": cpu, "cores": 4, "load": []float64{0.3, 0.2, 0.1}, "mem_total": 1000000, "mem_available": free,
			"storage_total": 300000, "storage_free": 290000, "temp": 52.1, "procs": 135, "conntrack": 18, "conntrack_max": 65536}
	}
	post := func(uptime int, r []any, h map[string]any) int {
		code, _, _ := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": uptime, "radios": r, "health": h}, nil)
		return code
	}
	if code := post(5000, radios(36, 80), health(12, 700000)); code != 200 {
		t.Fatalf("first report: %d", code)
	}
	if code := post(5001, radios(149, 80), health(40, 600000)); code != 200 {
		t.Fatalf("moved channel: %d", code)
	}
	if code := post(30, radios(149, 80), health(95, 60000)); code != 200 {
		t.Fatalf("restarted: %d", code)
	}
	for name, h := range map[string]map[string]any{
		"a processor 101% busy":        health(101, 1),
		"more memory free than it has": health(1, 2000000),
		"two loads":                    {"load": []float64{1, 2}},
		"a temperature of 500":         {"temp": 500},
	} {
		if code := post(40, radios(149, 80), h); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}

	_, got := f.do("GET", "/v1/aps/"+ap+"/health", "griff", nil)
	pts := got["points"].([]any)
	if len(pts) != 3 {
		t.Fatalf("points = %v", got)
	}
	last := pts[2].(map[string]any)
	if last["cpu"] != float64(95) || last["mem"] != float64(94) || last["temp"] != 52.1 || last["load"] != 0.3 {
		t.Fatalf("the last point = %v", last)
	}
	now := got["now"].(map[string]any)
	if now["uptime"] != float64(30) || now["health"].(map[string]any)["cores"] != float64(4) || now["health"].(map[string]any)["procs"] != float64(135) {
		t.Fatalf("now = %v", now)
	}

	_, tl := f.do("GET", "/v1/aps/"+ap+"/timeline", "griff", nil)
	var said []string
	for _, e := range tl["events"].([]any) {
		m := e.(map[string]any)
		said = append(said, m["kind"].(string)+": "+m["text"].(string))
	}
	all := strings.Join(said, "\n")
	for _, want := range []string{"first: first report: up 83 min", "channel: 5 GHz moved from channel 36 to 149", "restart: restarted: it had been up 82 min"} {
		if !strings.Contains(all, want) {
			t.Errorf("the history lacks %q:\n%s", want, all)
		}
	}

	// 6% of its memory free, and a processor busy throughout.
	kinds := alertKinds(t, f)
	if !strings.Contains(kinds, "warning:memory-low") || !strings.Contains(kinds, "warning:cpu-busy") {
		t.Fatalf("alerts = %s", kinds)
	}
	if code, _ := f.do("GET", "/v1/aps/no-such-ap/health", "griff", nil); code != 404 {
		t.Fatalf("no such AP: %d", code)
	}
	if code, _ := f.do("GET", "/v1/aps/"+ap+"/timeline?hours=0", "griff", nil); code != 400 {
		t.Fatalf("0 hours: %d", code)
	}
}

// What changed between two reports (0119).
func TestChangesBetweenReports(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	yes, no := true, false
	base := func() stateReport {
		return stateReport{
			Uptime: 86400, OpenWrt: "25.12-SNAPSHOT r32295", Agent: &agentState{Version: "v0.68.0"},
			Radios:         []radioState{{Radio: "radio0", Band: "5g", Channel: 36, Width: 80, Up: &yes}, {Radio: "radio1", Band: "2g", Channel: 6, Width: 20, Up: &yes}},
			UplinkNeighbor: &uplinkNeighbor{System: "homelab", Port: "Ethernet13"},
		}
	}
	prev := func(ago time.Duration, r stateReport) *conditions.State {
		raw, _ := json.Marshal(r)
		return &conditions.State{At: now.Add(-ago), Report: raw}
	}
	say := func(p *conditions.State, cur stateReport) string {
		var out []string
		for _, e := range changes(p, now, &cur) {
			out = append(out, e.Kind+": "+e.Text)
		}
		return strings.Join(out, "; ")
	}
	next := func(change func(*stateReport)) stateReport {
		r := base()
		r.Uptime += 300
		change(&r)
		return r
	}
	for name, c := range map[string]struct {
		ago  time.Duration
		cur  stateReport
		want string
	}{
		"nothing":             {5 * time.Minute, next(func(r *stateReport) {}), ""},
		"a restart":           {5 * time.Minute, next(func(r *stateReport) { r.Uptime = 45 }), "restart: restarted: it had been up 24 h"},
		"a long restart":      {2 * time.Hour, next(func(r *stateReport) { r.Uptime = 600 }), "restart: restarted: it had been up 25 h, and was not heard from for 2 h"},
		"a silence":           {40 * time.Minute, next(func(r *stateReport) { r.Uptime = 86400 + 2400 }), "silence: reports again, after 40 min without one; it did not restart"},
		"a new agent":         {5 * time.Minute, next(func(r *stateReport) { r.Agent = &agentState{Version: "v0.69.0"} }), "agent: agent updated from v0.68.0 to v0.69.0"},
		"new firmware":        {5 * time.Minute, next(func(r *stateReport) { r.OpenWrt = "25.12-SNAPSHOT r33000" }), "firmware: firmware changed from 25.12-SNAPSHOT r32295 to 25.12-SNAPSHOT r33000"},
		"a channel":           {5 * time.Minute, next(func(r *stateReport) { r.Radios[0].Channel = 149 }), "channel: 5 GHz moved from channel 36 to 149"},
		"a channel and width": {5 * time.Minute, next(func(r *stateReport) { r.Radios[0].Channel, r.Radios[0].Width = 149, 40 }), "channel: 5 GHz moved from channel 36 to 149, now 40 MHz"},
		"a width":             {5 * time.Minute, next(func(r *stateReport) { r.Radios[1].Width = 40 }), "channel: 2.4 GHz went from 20 to 40 MHz on channel 6"},
		"a radio down":        {5 * time.Minute, next(func(r *stateReport) { r.Radios[1].Up = &no }), "radio: 2.4 GHz radio went down"},
		"another port":        {5 * time.Minute, next(func(r *stateReport) { r.UplinkNeighbor = &uplinkNeighbor{System: "homelab", Port: "Ethernet14"} }), "uplink: uplink moved from homelab Ethernet13 to homelab Ethernet14"},
		"no neighbour heard":  {5 * time.Minute, next(func(r *stateReport) { r.UplinkNeighbor = nil }), ""},
	} {
		if got := say(prev(c.ago, base()), c.cur); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
	if got := say(nil, base()); got != "first: first report: up 24 h" {
		t.Errorf("a first report: %q", got)
	}
}
