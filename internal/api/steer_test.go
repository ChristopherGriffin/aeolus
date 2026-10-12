package api

import (
	"strings"
	"testing"
)

// What an AP says of its steering of clients (0138): the networks it
// steers and how, the count of what it asked and what came of it, and the
// last of those askings. What is not well said is left out, and the report
// kept, as with its own networks (0137).
func TestSteerReport(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	asking := func(mac, method, outcome string, more map[string]any) map[string]any {
		e := map[string]any{"at": 1791770010, "mac": mac, "network": "sweet", "method": method, "from": "2g", "asked": "btm", "outcome": outcome, "took": 2.5}
		for k, v := range more {
			e[k] = v
		}
		return e
	}
	steer := map[string]any{
		"running": true, "since": 1791770000, "known": 12,
		"networks": []any{
			map[string]any{"network": "sweet", "suggest": true, "hold": true, "sticky": true, "weak": -75, "bands": []string{"2g", "5g", "6g"},
				"counts": map[string]any{"suggest": map[string]any{"asked": 3, "moved": 2, "refused": 1}, "hold": map[string]any{"asked": 1, "joined": 1}}},
			// Not well said: no network of that kind of name, a band there is
			// none of, a way there is none of, an end there is none of.
			map[string]any{"network": "Not An ID", "suggest": true, "hold": false, "sticky": false},
			map[string]any{"network": "odd", "suggest": true, "hold": false, "sticky": false, "bands": []string{"60g"}},
			map[string]any{"network": "odder", "suggest": true, "hold": false, "sticky": false, "counts": map[string]any{"nudge": map[string]any{"asked": 1}}},
			map[string]any{"network": "oddest", "suggest": true, "hold": false, "sticky": false, "counts": map[string]any{"suggest": map[string]any{"vanished": 1}}},
		},
		"events": []any{
			asking("02:00:00:00:00:0a", "suggest", "moved", map[string]any{"to": []string{"6g", "5g"}, "answer": "accepted", "code": 0, "signal": -50}),
			asking("02:00:00:00:00:0b", "hold", "joined", map[string]any{"asked": "held", "to": []string{"5g"}, "signal": -52}),
			asking("02:00:00:00:00:0c", "sticky", "back", map[string]any{"asked": "kick", "from": "5g", "answer": "refused", "code": 1, "signal": -80}),
			asking("02:00:00:00:00:0d", "suggest", "back", map[string]any{"asked": "none"}),
			// Not well said.
			asking("not a mac", "suggest", "moved", nil),
			asking("02:00:00:00:00:0e", "nudge", "moved", nil),
			asking("02:00:00:00:00:0e", "suggest", "vanished", nil),
			asking("02:00:00:00:00:0e", "suggest", "moved", map[string]any{"from": "60g"}),
			asking("02:00:00:00:00:0e", "suggest", "moved", map[string]any{"asked": "begged"}),
			asking("02:00:00:00:00:0e", "suggest", "moved", map[string]any{"signal": 12}),
			asking("02:00:00:00:00:0e", "suggest", "moved", map[string]any{"took": -1}),
		},
	}
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "steer": steer}, nil); code != 200 {
		t.Fatalf("its report: %d %v", code, b)
	}
	said := func() map[string]any {
		t.Helper()
		code, cfg := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
		if code != 200 {
			t.Fatalf("its config: %d %v", code, cfg)
		}
		s, _ := cfg["condition"].(map[string]any)["state"].(map[string]any)["report"].(map[string]any)["steer"].(map[string]any)
		return s
	}
	s := said()
	if s["running"] != true || s["since"] != float64(1791770000) || s["known"] != float64(12) {
		t.Errorf("its steering, as kept: %v", s)
	}
	nets, _ := s["networks"].([]any)
	if len(nets) != 1 {
		t.Fatalf("networks kept: %v", nets)
	}
	n := nets[0].(map[string]any)
	counts, _ := n["counts"].(map[string]any)
	if n["network"] != "sweet" || n["weak"] != float64(-75) || len(n["bands"].([]any)) != 3 || counts["suggest"].(map[string]any)["moved"] != float64(2) || counts["hold"].(map[string]any)["joined"] != float64(1) {
		t.Errorf("the network, as kept: %v", n)
	}
	var ends []string
	for _, e := range s["events"].([]any) {
		e := e.(map[string]any)
		ends = append(ends, e["method"].(string)+" "+e["asked"].(string)+" "+e["outcome"].(string))
	}
	if got, want := strings.Join(ends, "; "), "suggest btm moved; hold held joined; sticky kick back; suggest none back"; got != want {
		t.Errorf("askings kept: %s, want %s", got, want)
	}
	if first := s["events"].([]any)[0].(map[string]any); first["answer"] != "accepted" || first["code"] != float64(0) || first["signal"] != float64(-50) || first["took"] != 2.5 {
		t.Errorf("the first, as kept: %v", first)
	}

	// One whose daemon is not running says only that; one that steers
	// nothing says nothing of it.
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 120, "steer": map[string]any{"running": false}}, nil); code != 200 {
		t.Fatalf("a report that its steering is not running: %d %v", code, b)
	}
	if s := said(); s == nil || s["running"] != false || s["networks"] != nil || s["events"] != nil {
		t.Errorf("not running, as kept: %v", s)
	}
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 180}, nil); code != 200 {
		t.Fatalf("a report with no steering: %d %v", code, b)
	}
	if s := said(); s != nil {
		t.Errorf("an AP that steers nothing says: %v", s)
	}
}
