package api

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// How long the network took to answer (0141): what a client test timed of
// its network, and how long a network's RADIUS server took an AP, are
// counted by the hour, and said a segment at a time, of the APs the caller
// may view.
func TestNetwork(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	id := hierarchy.NodeID(ap)
	raw := func(v any) json.RawMessage {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	yes, no := true, false
	// A test through the air that passed, one over the wire that got no
	// address, and one of a network on no VLAN.
	passed := map[string]any{"v": 1, "ssid": "Sweet Spot", "band": "5g", "client": "02:00:00:00:00:f0", "outcome": "passed", "stage": "done", "reason": "",
		"steps": []any{
			map[string]any{"t": 146, "stage": "find", "what": "looks for the network"},
			map[string]any{"t": 636, "stage": "key", "what": "let on to the network", "ok": yes},
			map[string]any{"t": 925, "stage": "dhcp", "what": "given 192.168.20.108 by 192.168.20.254", "ok": yes, "ms": 284},
			map[string]any{"t": 1100, "stage": "gateway", "what": "the gateway answers", "ok": yes, "ms": 4},
			map[string]any{"t": 1200, "stage": "dns", "what": "a name looked up", "ok": yes, "ms": 42},
		},
		"dhcp":    map[string]any{"ms": 284, "address": "192.168.20.108", "mask": 24, "router": "192.168.20.1", "dns": []string{"192.168.20.254"}, "server": "192.168.20.254", "lease": 86400},
		"gateway": map[string]any{"address": "192.168.20.1", "sent": 3, "got": 3, "min": 2, "avg": 4, "max": 7},
		"dns":     map[string]any{"server": "192.168.20.254", "name": "example.com", "ms": 42, "rcode": 0, "answers": 2, "address": "104.20.23.154"}}
	stuck := map[string]any{"v": 1, "band": wireBand, "client": "02:00:00:00:00:f1", "outcome": "failed", "stage": "dhcp", "reason": "no DHCP server gave it an address",
		"steps": []any{map[string]any{"t": 9000, "stage": "dhcp", "what": "no DHCP server gave it an address", "ok": no}}}
	action := func(network string, detail map[string]any) conditions.Action {
		done := f.now()
		return conditions.Action{AP: ap, Kind: "client-test", State: "done", DoneAt: &done, Params: raw(map[string]any{"network": network, "ssid": "x", "band": detail["band"]}), Detail: raw(detail)}
	}
	said := func(steps []conditions.StepTime) string {
		var out []string
		for _, s := range steps {
			v := "none"
			if s.MS != nil {
				v = fmt.Sprint(*s.MS)
			}
			out = append(out, s.Network+" "+s.Step+" "+v)
		}
		slices.Sort(out)
		return strings.Join(out, ", ")
	}
	if got, want := said(testSteps(action("sweet", passed))), "sweet dhcp 284, sweet dns 42, sweet gateway 4"; got != want {
		t.Fatalf("what a passed test timed:\n got %s\nwant %s", got, want)
	}
	if got, want := said(testSteps(action("sweet", stuck))), "sweet dhcp none"; got != want {
		t.Fatalf("what a test with no address timed:\n got %s\nwant %s", got, want)
	}
	if got := testSteps(conditions.Action{Kind: "reboot"}); got != nil {
		t.Errorf("an action that is no test: %v", got)
	}
	f.api.keepTestSteps(action("sweet", passed))
	f.api.keepTestSteps(action("sweet", stuck))
	f.api.keepTestSteps(action("guest", passed))

	// The RADIUS server of a network: its round trip where it answers, and
	// no answer where it has gone silent. One not yet asked says nothing.
	radius := func(network, verdict string, rtt, ago any) map[string]any {
		return map[string]any{"network": network, "server": "192.0.2.10", "port": 1812, "verdict": verdict, "rtt_ms": rtt, "answered_ago": ago,
			"requests": 1, "accepts": 1, "rejects": 0, "timeouts": 0, "bad": 0}
	}
	// Each report here comes an hour after the one before. The third's
	// answer is from before the report before it: counted then.
	*f.skew = -time.Hour
	for _, rs := range [][]any{{radius("sweet", "up", 16.5, 12)}, {radius("sweet", "up", 20.5, 40), radius("guest", "unknown", nil, nil)},
		{radius("sweet", "up", 20.5, 4000)}, {radius("sweet", "silent", nil, 9000)}} {
		if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "radius": rs}, nil); code != 200 {
			t.Fatalf("its report: %d %v", code, b)
		}
	}
	*f.skew = 0
	// Two reports seconds apart say the same answer: it is counted once.
	secs := func(n int64) *int64 { return &n }
	rtt := 9.5
	same := []radiusState{{Network: "sweet", Verdict: "up", RTT: &rtt, AnsweredAgo: secs(20)}}
	if a, b := radiusSteps(same, 5*time.Minute), radiusSteps(same, 8*time.Second); len(a) != 1 || len(b) != 0 {
		t.Errorf("an answer 20 s old, the report before 5 min ago and 8 s ago: %v, %v", a, b)
	}
	gone := &pathState{Gateway: &pathHop{State: "up", MS: &rtt, Ago: secs(20)}, DNS: []pathName{{Server: "192.0.2.1", State: "silent"}}}
	if a, b := pathSteps(gone, nil, 5*time.Minute), pathSteps(gone, nil, 8*time.Second); len(a) != 2 || len(b) != 1 || b[0].Step != "dns" || b[0].MS != nil {
		t.Errorf("a way out answered 20 s ago, the report before 5 min ago and 8 s ago: %v, %v", a, b)
	}
	if d := sinceBefore(nil, time.Now()); d != stateEvery {
		t.Errorf("with no report before: %v", d)
	}

	get := func(as, query string) map[string]any {
		t.Helper()
		code, v := f.do("GET", "/v1/network"+query, as, nil)
		if code != 200 {
			t.Fatalf("network%s as %s: %d %v", query, as, code, v)
		}
		return v
	}
	steps := func(v any) string {
		var out []string
		for k, s := range v.(map[string]any) {
			s := s.(map[string]any)
			out = append(out, fmt.Sprintf("%s n %v ms %v most %v failed %v", k, s["n"], s["ms"], s["most"], s["failed"]))
		}
		slices.Sort(out)
		return strings.Join(out, "; ")
	}
	v := get("griff", "")
	if v["hours"] != float64(24) || v["bucket"] != float64(3600) || len(v["series"].([]any)) != 24 || fmt.Sprint(v["steps"]) != "[dhcp gateway dns aaa signin internet]" {
		t.Errorf("a day: hours %v, %d buckets of %v s, steps %v", v["hours"], len(v["series"].([]any)), v["bucket"], v["steps"])
	}
	if got, want := steps(v["all"]), "aaa n 2 ms 18.5 most 20.5 failed 1; dhcp n 2 ms 284 most 284 failed 1; dns n 2 ms 42 most 42 failed 0; gateway n 2 ms 4 most 4 failed 0"; got != want {
		t.Errorf("all together:\n got %s\nwant %s", got, want)
	}
	// Sweet is on VLAN 20; guest is on none here, and is said by itself.
	segs := v["segments"].([]any)
	if len(segs) != 2 {
		t.Fatalf("segments: %v", segs)
	}
	sweet, guest := segs[0].(map[string]any), segs[1].(map[string]any)
	if sweet["id"] != "vlan:20" || sweet["vlan"] != float64(20) || fmt.Sprint(sweet["networks"]) != "[map[network:sweet ssid:Sweet Spot]]" ||
		steps(sweet["steps"]) != "aaa n 2 ms 18.5 most 20.5 failed 1; dhcp n 1 ms 284 most 284 failed 1; dns n 1 ms 42 most 42 failed 0; gateway n 1 ms 4 most 4 failed 0" {
		t.Errorf("VLAN 20: %v", sweet)
	}
	if guest["id"] != "net:guest" || guest["vlan"] != nil || fmt.Sprint(guest["networks"]) != "[map[network:guest ssid:Guest]]" || steps(guest["steps"]) != "dhcp n 1 ms 284 most 284 failed 0; dns n 1 ms 42 most 42 failed 0; gateway n 1 ms 4 most 4 failed 0" {
		t.Errorf("guest: %v", guest)
	}
	// The series: each step's average in its hour, of all, or of one segment.
	last := func(v map[string]any) map[string]any {
		pts := v["series"].([]any)
		return pts[len(pts)-1].(map[string]any)["steps"].(map[string]any)
	}
	if p := last(v); p["dhcp"] != 284.0 || p["aaa"] != 18.5 || p["gateway"] != 4.0 {
		t.Errorf("the latest hour of all: %v", p)
	}
	one := get("griff", "?segment=net:guest")
	if p := last(one); one["segment"] != "net:guest" || p["dhcp"] != 284.0 || p["aaa"] != nil || len(one["segments"].([]any)) != 2 {
		t.Errorf("the latest hour of guest: %v", p)
	}
	if p := last(get("griff", "?segment=vlan:999")); len(p) != 0 {
		t.Errorf("a segment there is none of: %v", p)
	}
	// Over a year, whole hours and no more than 48 buckets; under a folder
	// with none of its APs, nothing.
	y := get("griff", "?hours=8760")
	if int(y["bucket"].(float64))%3600 != 0 || len(y["series"].([]any)) > 48 || steps(y["all"]) != steps(v["all"]) {
		t.Errorf("a year: %d buckets of %v s, %s", len(y["series"].([]any)), y["bucket"], steps(y["all"]))
	}
	if under := get("griff", "?under=annex"); len(under["segments"].([]any)) != 0 || len(under["all"].(map[string]any)) != 0 {
		t.Errorf("under a folder it is not in: %v", under)
	}
	// One with no role in Locations sees none of it.
	if o := get("office", ""); len(o["segments"].([]any)) != 0 || len(o["all"].(map[string]any)) != 0 || len(last(o)) != 0 {
		t.Errorf("the office's: %v", o)
	}
	for _, q := range []string{"?hours=0", "?hours=8761", "?hours=many"} {
		if code, b := f.do("GET", "/v1/network"+q, "griff", nil); code != 400 {
			t.Errorf("%s: %d %v", q, code, b)
		}
	}
	if code, b := f.do("GET", "/v1/network", "", nil); code != 401 {
		t.Errorf("no token: %d %v", code, b)
	}

	// The tests kept from before this manager are counted in, once ever:
	// not one done since it started, which was counted as it came.
	if _, err := f.conds.TrimTrends(-time.Hour); err != nil {
		t.Fatal(err)
	}
	// What is timed is gone; the AP's networks are listed still, with
	// nothing beside them, as one just made is.
	gone2 := get("griff", "")
	if len(gone2["all"].(map[string]any)) != 0 {
		t.Fatalf("after all was let go: %v", gone2["all"])
	}
	for _, sg := range gone2["segments"].([]any) {
		if sg := sg.(map[string]any); len(sg["steps"].(map[string]any)) != 0 || len(sg["networks"].([]any)) == 0 {
			t.Errorf("a segment with nothing timed: %v", sg)
		}
	}
	if len(gone2["segments"].([]any)) == 0 {
		t.Error("the AP's own networks are not listed where nothing was timed")
	}
	finished := func(network string) {
		t.Helper()
		a, err := f.conds.AddActionWith(id, "client-test", network, "griff", raw(map[string]any{"network": network, "ssid": "x", "band": "5g"}))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.conds.ClaimActions(id); err != nil {
			t.Fatal(err)
		}
		if _, err := f.conds.FinishActionWith(id, a.ID, true, "passed", raw(passed)); err != nil {
			t.Fatal(err)
		}
	}
	*f.skew = -2 * time.Hour
	finished("sweet")
	*f.skew = 0
	finished("guest")
	f.api.countKeptTests()
	f.api.countKeptTests()
	after := get("griff", "")
	if got, want := steps(after["all"]), "dhcp n 1 ms 284 most 284 failed 0; dns n 1 ms 42 most 42 failed 0; gateway n 1 ms 4 most 4 failed 0"; got != want || len(after["segments"].([]any)) != 1 {
		t.Errorf("counted from the tests kept:\n got %s\nwant %s\n%v", got, want, after["segments"])
	}
	// And never again, whatever comes to be kept.
	*f.skew = -3 * time.Hour
	finished("sweet")
	*f.skew = 0
	f.api.countKeptTests()
	if got := steps(get("griff", "")["all"]); !strings.HasPrefix(got, "dhcp n 1 ") {
		t.Errorf("counted a second time: %s", got)
	}

	// The AP's own way out (0142) is counted under its management VLAN: its
	// gateway, each of its name servers and the internet. An answer from
	// before the report before is not counted again, nor one not yet known;
	// silence is no answer. And the networks it has of its own, not
	// Aeolus's (0137), are said on their VLANs, timed or not.
	port := map[string]any{"name": "eth0", "mac": "02:00:00:00:00:3a", "mtu": 1500, "carrier_changes": 1, "management_vlan": 30,
		"rx_bytes": 1, "tx_bytes": 1, "rx_packets": 1, "tx_packets": 1, "rx_errors": 0, "tx_errors": 0, "rx_dropped": 0, "tx_dropped": 0}
	way := map[string]any{"interface": "lan", "device": "br-lan.30", "address": "192.168.30.7", "mask": 24,
		"gateway": map[string]any{"address": "192.168.30.1", "state": "up", "ms": 2.0, "ago": 10},
		"dns": []any{map[string]any{"server": "192.168.30.254", "state": "up", "ms": 6.0, "ago": 10, "rcode": 0},
			map[string]any{"server": "192.168.30.253", "state": "silent", "ms": nil, "ago": nil, "rcode": nil},
			map[string]any{"server": "192.168.30.252", "state": "up", "ms": 9.0, "ago": 4000, "rcode": 0}},
		"internet": map[string]any{"target": "one.one.one.one", "state": "unresolved"}}
	others := []any{
		map[string]any{"section": "iot2", "radio": "radio1", "band": "2g", "ssid": "House IoT", "security": "psk2", "vlan": 10, "clients": 3},
		map[string]any{"section": "iot5", "radio": "radio0", "band": "5g", "ssid": "House IoT", "security": "psk2", "vlan": 10, "clients": 0},
		map[string]any{"section": "house", "radio": "radio0", "band": "5g", "ssid": "House", "security": "psk2", "vlan": 20, "clients": 1},
		map[string]any{"section": "plain", "radio": "radio0", "band": "5g", "ssid": "No VLAN", "security": "psk2", "clients": 0},
	}
	// This report, too, an hour after the one before, and an hour before
	// the next.
	*f.skew = -time.Hour
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "uplink_port": port, "path": way, "others": others}, nil); code != 200 {
		t.Fatalf("its report: %d %v", code, b)
	}
	*f.skew = 0
	last30 := get("griff", "")
	var said30 []string
	for _, sg := range last30["segments"].([]any) {
		sg := sg.(map[string]any)
		var names []string
		for _, n := range sg["networks"].([]any) {
			n := n.(map[string]any)
			names = append(names, fmt.Sprintf("%v/%v/%v", n["network"], n["ssid"], n["kind"]))
		}
		said30 = append(said30, fmt.Sprintf("%v [%s] %s", sg["id"], strings.Join(names, " "), steps(sg["steps"])))
	}
	want30 := []string{
		"vlan:10 [<nil>/House IoT/own] ",
		"vlan:20 [sweet/Sweet Spot/<nil> <nil>/House/own] dhcp n 1 ms 284 most 284 failed 0; dns n 1 ms 42 most 42 failed 0; gateway n 1 ms 4 most 4 failed 0",
		"vlan:30 [mgmt:30/<nil>/management] dns n 1 ms 6 most 6 failed 1; gateway n 1 ms 2 most 2 failed 0",
	}
	if got, want := strings.Join(said30, "\n"), strings.Join(want30, "\n"); got != want {
		t.Errorf("with its own way out and its own networks:\n got\n%s\nwant\n%s", got, want)
	}
	// The internet, once its target answers.
	way["internet"] = map[string]any{"target": "one.one.one.one", "address": "1.1.1.1", "state": "up", "ms": 22.0, "ago": 5}
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 120, "uplink_port": port, "path": way}, nil); code != 200 {
		t.Fatalf("its report: %d %v", code, b)
	}
	for _, sg := range get("griff", "")["segments"].([]any) {
		if sg := sg.(map[string]any); sg["id"] == "vlan:30" {
			if got, want := steps(sg["steps"]), "dns n 2 ms 6 most 6 failed 2; gateway n 2 ms 2 most 2 failed 0; internet n 1 ms 22 most 22 failed 0"; got != want {
				t.Errorf("with the internet answering:\n got %s\nwant %s", got, want)
			}
		}
	}
}
