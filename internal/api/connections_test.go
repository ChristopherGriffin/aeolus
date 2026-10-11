package api

import (
	"strings"
	"testing"
	"time"
)

// A client's attempts to come online (0118): an AP reports them, the
// manager keeps each once and every client for good, and a person reads a
// client's history back.
func TestClientConnections(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	now := time.Now().UnixMilli()
	const mac = "72:bb:66:12:2a:94"
	yes, ms := true, 12
	online := map[string]any{
		"mac": mac, "bss": "phy0-ap0", "network": "sweet", "ssid": "Sweet Spot", "band": "5g",
		"started": now - 60000, "took_ms": 302, "outcome": "online", "stage": "internet", "reason": "",
		"address": "192.168.20.61", "host": "Griffs-iPhone", "signal": -52,
		"probes": []any{map[string]any{"ap": "", "band": "5g", "signal": -40}, map[string]any{"ap": "192.168.1.38", "band": "", "signal": -85}},
		"events": []any{
			map[string]any{"t": 0, "stage": "auth", "what": "authenticated", "ok": yes},
			map[string]any{"t": 4, "stage": "assoc", "what": "associated", "ok": yes},
			map[string]any{"t": 31, "stage": "key", "what": "let on to the network", "ok": yes},
			map[string]any{"t": 230, "stage": "dhcp", "what": "given 192.168.20.61", "ok": yes, "server": "192.168.20.1", "router": "192.168.20.1", "dns": "192.168.20.1 9.9.9.9"},
			map[string]any{"t": 302, "stage": "gateway", "what": "192.168.20.1, its gateway, answers", "ok": yes, "holder": "d4:01:c3:00:00:01"},
			map[string]any{"t": 362, "stage": "dns", "what": "192.168.20.1 answers captive.apple.com: 2 records", "ok": yes, "ms": ms},
		},
	}
	failed := map[string]any{
		"mac": mac, "bss": "phy0-ap0", "network": "sweet", "ssid": "Sweet Spot", "band": "5g",
		"started": now - 30000, "took_ms": 4200, "outcome": "failed", "stage": "key", "reason": "wrong passphrase",
		"events": []any{
			map[string]any{"t": 0, "stage": "auth", "what": "authenticated", "ok": yes},
			map[string]any{"t": 1100, "stage": "key", "what": "wrong passphrase", "ok": false},
			map[string]any{"t": 4200, "stage": "end", "what": "deauthenticated due to local deauth request"},
		},
	}
	bad := map[string]any{"mac": "72:BB:66:12:2A:94", "bss": "phy0-ap0", "started": now, "outcome": "online", "stage": "dns", "events": []any{}}
	post := func(list ...any) (int, map[string]any) {
		code, _, body := f.apDo("POST", "/v1/ap/connections", token, map[string]any{"connections": list}, nil)
		return code, body
	}
	code, body := post(online, bad, failed)
	if code != 200 || body["recorded"] != float64(2) || body["refused"] != float64(1) || !strings.Contains(body["why"].(string), "MAC") {
		t.Fatalf("two good and one bad: %d %v", code, body)
	}
	// Sent again, as after an agent restarts, nothing is kept twice.
	if code, body := post(online, failed); code != 200 || body["recorded"] != float64(0) {
		t.Fatalf("sent again: %d %v", code, body)
	}

	_, got := f.do("GET", "/v1/clients/"+mac+"/connections", "griff", nil)
	list := got["connections"].([]any)
	if len(list) != 2 {
		t.Fatalf("connections = %v", got)
	}
	first, second := list[0].(map[string]any), list[1].(map[string]any)
	if first["outcome"] != "failed" || first["stage"] != "key" || first["reason"] != "wrong passphrase" || first["ap"] != ap ||
		second["outcome"] != "online" || second["took_ms"] != float64(302) {
		t.Fatalf("the latest first: %v", list)
	}
	rec := second["record"].(map[string]any)
	if len(rec["events"].([]any)) != 6 || rec["signal"] != float64(-52) || len(rec["probes"].([]any)) != 2 {
		t.Fatalf("record = %v", rec)
	}
	cl := got["client"].(map[string]any)
	if cl["attempts"] != float64(2) || cl["failed"] != float64(1) || cl["host"] != "Griffs-iPhone" || cl["address"] != "192.168.20.61" ||
		cl["last_outcome"] != "failed" || cl["ap"] != ap {
		t.Fatalf("client = %v", cl)
	}
	// Paging back from the latest's start gives the one before it.
	_, older := f.do("GET", "/v1/clients/"+mac+"/connections?limit=1&before="+first["started"].(string), "griff", nil)
	if l := older["connections"].([]any); len(l) != 1 || l[0].(map[string]any)["outcome"] != "online" {
		t.Fatalf("before the latest: %v", older)
	}

	// A state report says who it signed in as, and sees it again; one seen
	// only there, never trying to come online, is a client too.
	client := func(m, user string) map[string]any {
		return map[string]any{"mac": m, "network": "sweet", "ssid": "Sweet Spot", "band": "5g", "signal": -60, "signal_avg": -60,
			"tx_rate": 400, "rx_rate": 300, "connected": 60, "inactive_ms": 10, "rx_bytes": 1, "tx_bytes": 1, "rx_packets": 1, "tx_packets": 1, "user": user}
	}
	if code, _, _ := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60,
		"clients": []any{client(mac, "aeolus-test"), client("02:00:00:00:00:07", "")}}, nil); code != 200 {
		t.Fatalf("state: %d", code)
	}
	_, all := f.do("GET", "/v1/clients", "griff", nil)
	if all["total"] != float64(2) || len(all["clients"].([]any)) != 2 {
		t.Fatalf("clients = %v", all)
	}
	_, found := f.do("GET", "/v1/clients?q=AEOLUS-test", "griff", nil)
	hit := found["clients"].([]any)
	if len(hit) != 1 || hit[0].(map[string]any)["mac"] != mac || hit[0].(map[string]any)["user"] != "aeolus-test" ||
		hit[0].(map[string]any)["attempts"] != float64(2) || hit[0].(map[string]any)["ap_name"] == "" {
		t.Fatalf("found by user = %v", found)
	}
	// Seen again by a report, how its latest attempt went is still known.
	if hit[0].(map[string]any)["last_outcome"] != "failed" {
		t.Fatalf("its latest attempt, after a report saw it = %v", hit[0])
	}
	if _, none := f.do("GET", "/v1/clients?q=nobody", "griff", nil); none["total"] != float64(0) {
		t.Fatalf("found nobody = %v", none)
	}
	if _, under := f.do("GET", "/v1/clients?under=no-such-folder", "griff", nil); under["total"] != float64(0) {
		t.Fatalf("under another folder = %v", under)
	}

	// An AP whose clock is unset: its attempt is given the time it came in.
	stale := map[string]any{"mac": "02:00:00:00:00:09", "bss": "phy0-ap0", "started": 86400000, "took_ms": 10, "outcome": "left", "stage": "auth", "reason": "", "events": []any{}}
	if code, body := post(stale); code != 200 || body["recorded"] != float64(1) {
		t.Fatalf("an unset clock: %d %v", code, body)
	}
	_, got = f.do("GET", "/v1/clients/02:00:00:00:00:09/connections", "griff", nil)
	at, err := time.Parse(time.RFC3339Nano, got["connections"].([]any)[0].(map[string]any)["started"].(string))
	if err != nil || time.Since(at) > time.Minute {
		t.Fatalf("started = %v %v", at, err)
	}
	// Sent again, as when the answer to the first was lost, it is still the
	// one attempt: told apart by the start its AP gave, not the time it came.
	if code, body := post(stale); code != 200 || body["recorded"] != float64(0) {
		t.Fatalf("an unset clock, sent again: %d %v", code, body)
	}

	// One record with a field of another kind, and one with a field this
	// manager does not know, are passed over; the good one beside them is kept.
	wrong := map[string]any{"mac": "02:00:00:00:00:0b", "bss": "phy0-ap0", "started": "yesterday", "took_ms": 10, "outcome": "left", "stage": "auth", "reason": "", "events": []any{}}
	newer := map[string]any{"mac": "02:00:00:00:00:0c", "bss": "phy0-ap0", "started": now, "took_ms": 10, "outcome": "left", "stage": "auth", "reason": "", "events": []any{}, "roamed_from": "ap-1"}
	good := map[string]any{"mac": "02:00:00:00:00:0d", "bss": "phy0-ap0", "started": now, "took_ms": 10, "outcome": "left", "stage": "auth", "reason": "", "events": []any{}}
	if code, body := post(wrong, newer, good); code != 200 || body["recorded"] != float64(1) || body["refused"] != float64(2) {
		t.Fatalf("two that cannot be read and one that can: %d %v", code, body)
	}

	for name, change := range map[string]func(map[string]any){
		"an outcome of its own": func(e map[string]any) { e["outcome"] = "won" },
		"a stage of its own":    func(e map[string]any) { e["stage"] = "probe" },
		"an hour long":          func(e map[string]any) { e["took_ms"] = 3600000 },
		"a step with no words":  func(e map[string]any) { e["events"] = []any{map[string]any{"t": 0, "stage": "auth", "what": ""}} },
		"a step's own stage":    func(e map[string]any) { e["events"] = []any{map[string]any{"t": 0, "stage": "x", "what": "y"}} },
		"an address that isn't": func(e map[string]any) { e["address"] = "next door" },
		"a signal above 0":      func(e map[string]any) { e["signal"] = 5 },
	} {
		e := map[string]any{"mac": "02:00:00:00:00:0a", "bss": "phy0-ap0", "started": now, "took_ms": 10, "outcome": "left", "stage": "auth", "reason": "", "events": []any{}}
		change(e)
		if code, body := post(e); code != 200 || body["refused"] != float64(1) || body["recorded"] != float64(0) {
			t.Errorf("%s: %d %v", name, code, body)
		}
	}
	many := make([]any, 65)
	for i := range many {
		many[i] = failed
	}
	if code, _ := post(many...); code != 400 {
		t.Fatalf("65 at once: %d", code)
	}
	if code, _ := f.do("GET", "/v1/clients/not-a-mac/connections", "griff", nil); code != 400 {
		t.Fatalf("not a MAC: %d", code)
	}
}
