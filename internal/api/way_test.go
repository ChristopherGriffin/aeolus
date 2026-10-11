package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/conditions"
)

// said is a way as a line: each point, how it went, and its time in ms
// where it has one.
func said(way []wayPoint) string {
	var out []string
	for _, p := range way {
		s := p.K + " " + p.State
		if p.MS != nil {
			s += fmt.Sprintf(" %g", *p.MS)
		}
		out = append(out, s)
	}
	return strings.Join(out, ", ")
}

func testOf(t *testing.T, raw string) *clientTest {
	t.Helper()
	var d clientTest
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatal(err)
	}
	return &d
}

// The way online (0130), read from a test's steps: through the air, each
// point from the one before, but where it began by asking, or says its own
// answer's time; over the wire, the offer and the address apart, where the
// agent says which step is which.
func TestTheWayOfATest(t *testing.T) {
	for name, c := range map[string]struct{ raw, want string }{
		"through the air": {`{"band":"5g","outcome":"passed","stage":"done","steps":[
			{"t":146,"stage":"find","what":"looks for the network"},
			{"t":378,"stage":"find","what":"found it","ok":true},
			{"t":417,"stage":"auth","what":"authenticated","ok":true},
			{"t":439,"stage":"assoc","what":"associated","ok":true},
			{"t":634,"stage":"key","what":"key handshake completed","ok":true},
			{"t":636,"stage":"key","what":"let on to the network","ok":true},
			{"t":925,"stage":"dhcp","what":"given 192.168.20.108","ok":true,"ms":284},
			{"t":2944,"stage":"gateway","what":"answers 3 of 3 pings","ok":true,"ms":4.131},
			{"t":2987,"stage":"dns","what":"answers","ok":true,"ms":42},
			{"t":9107,"stage":"speed","what":"down 21 Mbit/s","ok":true},
			{"t":15193,"stage":"speed","what":"up 22.1 Mbit/s","ok":true}]}`,
			"find ok 232, auth ok 39, assoc ok 22, key ok 197, dhcp ok 284, gateway ok 4.131, dns ok 42, speed ok"},
		// A network that signs its clients in has that point; one that
		// failed says how long it was at it; what it never reached is none.
		"signed in, and no address": {`{"band":"2g","outcome":"failed","stage":"dhcp","steps":[
			{"t":150,"stage":"find","what":"looks for the network"},
			{"t":2310,"stage":"find","what":"found it","ok":true},
			{"t":2330,"stage":"auth","what":"authenticated","ok":true},
			{"t":2340,"stage":"assoc","what":"associated","ok":true},
			{"t":2500,"stage":"signin","what":"signed in","ok":true},
			{"t":2561,"stage":"key","what":"let on to the network","ok":true},
			{"t":12600,"stage":"dhcp","what":"no answer to its DHCP requests","ok":false}]}`,
			"find ok 2160, auth ok 20, assoc ok 10, signin ok 160, key ok 61, dhcp bad 10039, gateway none, dns none"},
		// A gateway that answers ARP and no ping is there, with no time to
		// give: the three seconds its pings took say nothing.
		"a gateway that answers no ping": {`{"band":"5g","outcome":"passed","stage":"done","steps":[
			{"t":300,"stage":"find","what":"found it","ok":true},
			{"t":500,"stage":"key","what":"let on to the network","ok":true},
			{"t":800,"stage":"dhcp","what":"given","ok":true,"ms":290},
			{"t":3900,"stage":"gateway","what":"is there, and answers no ping","ok":true},
			{"t":3940,"stage":"dns","what":"answers","ok":true,"ms":40}]}`,
			"find ok 300, auth none, assoc none, key ok 200, dhcp ok 290, gateway ok, dns ok 40"},
		// It failed where no step says so: the point it stopped at is bad.
		"stopped where nothing was said": {`{"band":"5g","outcome":"failed","stage":"auth","steps":[
			{"t":300,"stage":"find","what":"found it","ok":true}]}`,
			"find ok 300, auth bad, assoc none, key none, dhcp none, gateway none, dns none"},
		"over the wire": {`{"band":"wire","outcome":"passed","stage":"done","steps":[
			{"t":5,"stage":"dhcp","k":"offer","what":"asks for an address (discover)"},
			{"t":2010,"stage":"dhcp","k":"offer","what":"offered 192.168.20.113","ok":true},
			{"t":2012,"stage":"dhcp","what":"asks to use the address (request)"},
			{"t":2335,"stage":"dhcp","what":"given 192.168.20.113","ok":true,"ms":2330},
			{"t":2340,"stage":"gateway","what":"is there","ok":true,"ms":1.9},
			{"t":2385,"stage":"dns","what":"answers","ok":true,"ms":43},
			{"t":2390,"stage":"dhcp","k":"release","what":"gives 192.168.20.113 back","ok":true}]}`,
			"offer ok 2005, dhcp ok 323, gateway ok 1.9, dns ok 43, release ok"},
		// An agent from before says nothing of the offer: its address is
		// one point, from its first asking; and what it gave back is told
		// from what it was given by where it comes.
		"over the wire, by an agent from before": {`{"band":"wire","outcome":"passed","stage":"done","steps":[
			{"t":5,"stage":"dhcp","what":"asks for an address (discover)"},
			{"t":2010,"stage":"dhcp","what":"offered 192.168.20.113"},
			{"t":2012,"stage":"dhcp","what":"asks to use the address (request)"},
			{"t":2335,"stage":"dhcp","what":"given 192.168.20.113","ok":true,"ms":2330},
			{"t":2340,"stage":"gateway","what":"is there","ok":true,"ms":1.9},
			{"t":2385,"stage":"dns","what":"answers","ok":true,"ms":43},
			{"t":2390,"stage":"dhcp","what":"gives 192.168.20.113 back"}]}`,
			"dhcp ok 2330, gateway ok 1.9, dns ok 43, release ok"},
		"over the wire, and no offer": {`{"band":"wire","outcome":"failed","stage":"dhcp","steps":[
			{"t":5,"stage":"dhcp","k":"offer","what":"asks for an address (discover)"},
			{"t":3005,"stage":"dhcp","k":"offer","what":"asks for an address (discover)"},
			{"t":9005,"stage":"dhcp","k":"offer","what":"no answer to its DHCP requests","ok":false}]}`,
			"offer bad 9000, dhcp none, gateway none, dns none"},
		"over the wire, offered and not given": {`{"band":"wire","outcome":"failed","stage":"dhcp","steps":[
			{"t":5,"stage":"dhcp","k":"offer","what":"asks for an address (discover)"},
			{"t":2010,"stage":"dhcp","k":"offer","what":"offered 192.168.20.113","ok":true},
			{"t":2012,"stage":"dhcp","what":"asks to use the address (request)"},
			{"t":8012,"stage":"dhcp","what":"no answer to its request","ok":false}]}`,
			"offer ok 2005, dhcp bad 6000, gateway none, dns none"},
		// All a test from before its steps were kept has: what it measured.
		"with no steps": {`{"band":"5g","outcome":"passed","stage":"done","steps":[],
			"dhcp":{"ms":300,"address":"192.168.20.9","mask":24},"gateway":{"address":"192.168.20.1","sent":3,"got":3,"avg":4.2},
			"dns":{"server":"192.168.20.254","name":"example.com","ms":41}}`,
			"find none, auth none, assoc none, key none, dhcp none, gateway none, dns none"},
	} {
		if got := said(testWay(testOf(t, c.raw))); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, c.want)
		}
	}
}

func attemptOf(outcome, stage, events string) conditions.Connection {
	return conditions.Connection{Outcome: outcome, Stage: stage, Record: json.RawMessage(`{"events":` + events + `}`)}
}

// A client's own attempt is read the same way (0130): its steps are timed
// from its first, which has no time of its own; a point that began by
// asking took from then to its answer; a name looked up, and its first
// connection, took what the first answer says.
func TestTheWayOfAClient(t *testing.T) {
	const online = `[
		{"t":0,"stage":"auth","what":"authenticated","ok":true},
		{"t":4,"stage":"assoc","what":"associated","ok":true},
		{"t":31,"stage":"key","what":"let on to the network","ok":true},
		{"t":32,"stage":"key","what":"key handshake completed","ok":true},
		{"t":210,"stage":"dhcp","what":"asks for an address (discover)"},
		{"t":215,"stage":"dhcp","what":"offered 192.168.20.61"},
		{"t":228,"stage":"dhcp","what":"asks to use the address (request)"},
		{"t":230,"stage":"dhcp","what":"given 192.168.20.61","ok":true},
		{"t":300,"stage":"gateway","what":"asks who has 192.168.20.1, its gateway"},
		{"t":302,"stage":"gateway","what":"192.168.20.1, its gateway, answers","ok":true},
		{"t":350,"stage":"dns","what":"asks for captive.apple.com"},
		{"t":362,"stage":"dns","what":"answers captive.apple.com","ok":true,"ms":12},
		{"t":380,"stage":"dns","what":"asks for www.apple.com"},
		{"t":460,"stage":"dns","what":"answers www.apple.com","ok":true,"ms":80},
		{"t":400,"stage":"internet","what":"opens a connection"},
		{"t":423,"stage":"internet","what":"answers","ok":true,"ms":23},
		{"t":5000,"stage":"note","what":"hostapd says something"},
		{"t":9000,"stage":"end","what":"left"}]`
	for name, c := range map[string]struct {
		c    conditions.Connection
		want string
	}{
		"online": {attemptOf("online", "internet", online),
			"auth ok, assoc ok 4, key ok 28, dhcp ok 20, gateway ok 2, dns ok 12, internet ok 23"},
		"no address": {attemptOf("failed", "dhcp", `[
			{"t":0,"stage":"auth","what":"authenticated","ok":true},
			{"t":4,"stage":"assoc","what":"associated","ok":true},
			{"t":31,"stage":"key","what":"let on to the network","ok":true},
			{"t":200,"stage":"dhcp","what":"asks for an address (discover)"},
			{"t":4200,"stage":"dhcp","what":"asks for an address (discover)"},
			{"t":12200,"stage":"dhcp","what":"asks for an address (discover)"}]`),
			"auth ok, assoc ok 4, key ok 27, dhcp bad 12000, gateway none, dns none, internet none"},
		"a wrong passphrase, on a network that signs in": {attemptOf("failed", "signin", `[
			{"t":0,"stage":"auth","what":"authenticated","ok":true},
			{"t":5,"stage":"assoc","what":"associated","ok":true},
			{"t":1100,"stage":"signin","what":"refused","ok":false}]`),
			"auth ok, assoc ok 5, signin bad 1095, key none, dhcp none, gateway none, dns none, internet none"},
		// One that kept its address asks for none, and is seen at its
		// gateway: DHCP is not seen, which is not a fault.
		"let on, and asking no address": {attemptOf("connected", "key", `[
			{"t":0,"stage":"auth","what":"authenticated","ok":true},
			{"t":4,"stage":"assoc","what":"associated","ok":true},
			{"t":30,"stage":"key","what":"let on to the network","ok":true},
			{"t":90,"stage":"gateway","what":"asks who has its gateway"}]`),
			"auth ok, assoc ok 4, key ok 26, dhcp none, gateway begun, dns none, internet none"},
		"nothing kept of it": {attemptOf("failed", "key", `[]`),
			"auth none, assoc none, key bad, dhcp none, gateway none, dns none, internet none"},
	} {
		if got := said(connWay(c.c)); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, c.want)
		}
	}
}

// Many ways come to one (0130): at each point how many passed it, how many
// stopped at it, and of the times the least, the middle and the most. A
// point seen in none of them is left out where it is not every way's.
func TestTheWayOfMany(t *testing.T) {
	ms := func(v float64) *float64 { return &v }
	tally := newWayTally(wayClient)
	if got := tally.typical(); got != nil {
		t.Fatalf("of none: %v", got)
	}
	for _, v := range []float64{30, 10, 20} {
		tally.add([]wayPoint{{K: "auth", State: "ok"}, {K: "assoc", State: "ok", MS: ms(4)}, {K: "key", State: "ok", MS: ms(v)},
			{K: "dhcp", State: "ok", MS: ms(v * 10)}, {K: "gateway", State: "none"}})
	}
	tally.add([]wayPoint{{K: "auth", State: "ok"}, {K: "assoc", State: "ok", MS: ms(4)}, {K: "key", State: "ok", MS: ms(40)},
		{K: "dhcp", State: "bad", MS: ms(12000)}})
	tally.add([]wayPoint{{K: "auth", State: "ok"}, {K: "assoc", State: "begun"}})
	var out []string
	for _, p := range tally.typical() {
		s := fmt.Sprintf("%s %d/%d", p.K, p.Passed, p.Stopped)
		if p.MS != nil {
			s += fmt.Sprintf(" %g %g %g", *p.Low, *p.MS, *p.High)
		}
		out = append(out, s)
	}
	want := "auth 5/0, assoc 4/0 4 4 4, key 4/0 10 30 40, dhcp 3/1 100 200 300, gateway 0/0, dns 0/0, internet 0/0"
	if got := strings.Join(out, ", "); got != want {
		t.Errorf("many:\n got %s\nwant %s", got, want)
	}
}

// What the API says of the way (0130): a test asked of an AP has it among
// what it found, though an AP that sends one is not believed; a thing
// tested has what its tests come to; a client's attempts each have theirs;
// and a network's joins what its clients' come to, but for the tests' own
// client.
func TestTheWayOnline(t *testing.T) {
	f := newFixture(t)
	change := func(body map[string]any) {
		t.Helper()
		if code, b := f.change("griff", body); code != 200 {
			t.Fatalf("%v: %d %v", body["kind"], code, b)
		}
	}
	change(map[string]any{"kind": "add-builtins"})
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	plain := strings.Split(f.enrollAt("Plain", "02:00:00:00:00:5b", "office", []any{serving}, map[string]int{"5g": 44}), "|")
	say := func() {
		t.Helper()
		_, _, cfg := f.apDo("GET", "/v1/ap/config", plain[1], nil, nil)
		if code, _, b := f.apDo("POST", "/v1/ap/state", plain[1], map[string]any{"version": cfg["version"], "uptime": 60, "wire_tests": true}, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
	}
	say()
	change(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.every", "value": 15})
	change(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.wire", "value": true})

	yes, no := true, false
	found := func(passed bool, offer int) map[string]any {
		d := map[string]any{
			"v": 1, "ssid": "Sweet Spot", "band": "wire", "client": "0e:00:00:5b:00:20", "outcome": "passed", "stage": "done", "reason": "",
			"summary": "passed", "took_ms": offer + 400, "wire": map[string]any{"device": "eth1", "vlan": 20, "tagged": true},
			// An AP that says what its way was is not believed.
			"way": []any{map[string]any{"k": "dhcp", "state": "bad"}},
			"steps": []any{
				map[string]any{"t": 5, "stage": "dhcp", "k": "offer", "what": "asks for an address (discover)"},
				map[string]any{"t": 5 + offer, "stage": "dhcp", "k": "offer", "what": "offered 192.168.20.113", "ok": yes},
				map[string]any{"t": 7 + offer, "stage": "dhcp", "what": "asks to use the address (request)"},
				map[string]any{"t": 307 + offer, "stage": "dhcp", "what": "given 192.168.20.113", "ok": yes, "ms": 302 + offer},
				map[string]any{"t": 310 + offer, "stage": "gateway", "what": "is there", "ok": yes, "ms": 1.5},
				map[string]any{"t": 350 + offer, "stage": "dns", "what": "answers", "ok": yes, "ms": 40},
				map[string]any{"t": 352 + offer, "stage": "dhcp", "k": "release", "what": "gives 192.168.20.113 back", "ok": yes},
			},
		}
		if !passed {
			d["outcome"], d["stage"], d["reason"], d["summary"] = "failed", "dhcp", "no DHCP server gave it an address", "failed at dhcp"
			d["steps"] = []any{
				map[string]any{"t": 5, "stage": "dhcp", "k": "offer", "what": "asks for an address (discover)"},
				map[string]any{"t": 9005, "stage": "dhcp", "k": "offer", "what": "no answer to its DHCP requests", "ok": no},
			}
		}
		return d
	}
	// Three tests on the schedule: an offer in 2000 ms, one in 1000, and
	// one with none.
	for i, d := range []map[string]any{found(true, 2000), found(true, 1000), found(false, 0)} {
		*f.skew += 16 * time.Minute
		say()
		if got := f.api.assure(f.now()); len(got) != 1 {
			t.Fatalf("test %d asked: %v", i, got)
		}
		_, _, got := f.apDo("GET", "/v1/ap/actions", plain[1], nil, nil)
		id := got["actions"].([]any)[0].(map[string]any)["id"]
		if code, _, b := f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%v", id), plain[1], map[string]any{"ok": d["outcome"] == "passed", "result": d["summary"], "detail": d}, nil); code != 200 {
			t.Fatalf("test %d done: %d %v", i, code, b)
		}
	}
	line := func(way any) string {
		var out []string
		list, _ := way.([]any)
		for _, x := range list {
			p := x.(map[string]any)
			s := fmt.Sprintf("%v %v", p["k"], p["state"])
			if p["ms"] != nil {
				s += fmt.Sprintf(" %v", p["ms"])
			}
			out = append(out, s)
		}
		return strings.Join(out, ", ")
	}
	_, body := f.do("GET", "/v1/aps/"+plain[0]+"/actions", "griff", nil)
	acts := body["actions"].([]any)
	if got, want := line(acts[0].(map[string]any)["detail"].(map[string]any)["way"]), "offer bad 9000, dhcp none, gateway none, dns none"; got != want {
		t.Errorf("the latest test's way = %s, want %s", got, want)
	}
	if got, want := line(acts[1].(map[string]any)["detail"].(map[string]any)["way"]), "offer ok 1000, dhcp ok 300, gateway ok 1.5, dns ok 40, release ok"; got != want {
		t.Errorf("the test before's way = %s, want %s", got, want)
	}

	// A client's attempts, and the tests' own client's, which is no one's.
	const mac, scan = "72:bb:66:12:2a:01", "02:00:00:00:0a:f0"
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	if code, _, b := f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": "02:00:00:00:00:2a", "hostname": "Scanner", "radios": []any{serving, spare},
		"macs": []string{"02:00:00:00:00:2a", scan}}, nil); code != 201 {
		t.Fatalf("enroll: %d %v", code, b)
	} else {
		change(map[string]any{"kind": "move", "tree": "locations", "node": b["ap"], "parent": "office"})
	}
	now := time.Now()
	attempt := func(who, outcome, stage string, key int, ago time.Duration, more ...any) map[string]any {
		events := []any{
			map[string]any{"t": 0, "stage": "auth", "what": "authenticated", "ok": yes},
			map[string]any{"t": 4, "stage": "assoc", "what": "associated", "ok": yes},
			map[string]any{"t": 4 + key, "stage": "key", "what": "let on to the network", "ok": yes},
		}
		return map[string]any{"mac": who, "bss": "phy0-ap0", "network": "sweet", "ssid": "Sweet Spot", "band": "5g",
			"started": now.Add(-ago).UnixMilli(), "took_ms": 500, "outcome": outcome, "stage": stage, "reason": "", "events": append(events, more...)}
	}
	list := []any{
		attempt(mac, "online", "dns", 20, 30*time.Minute,
			map[string]any{"t": 200, "stage": "dhcp", "what": "asks for an address (discover)"},
			map[string]any{"t": 260, "stage": "dhcp", "what": "given 192.168.20.61", "ok": yes},
			map[string]any{"t": 300, "stage": "dns", "what": "asks for a name"},
			map[string]any{"t": 340, "stage": "dns", "what": "answers", "ok": yes, "ms": 40}),
		attempt(mac, "failed", "dhcp", 40, 20*time.Minute,
			map[string]any{"t": 200, "stage": "dhcp", "what": "asks for an address (discover)"},
			map[string]any{"t": 8200, "stage": "dhcp", "what": "asks for an address (discover)"}),
		attempt(scan, "failed", "dhcp", 900, 10*time.Minute,
			map[string]any{"t": 2000, "stage": "dhcp", "what": "asks for an address (discover)"}),
	}
	if code, _, b := f.apDo("POST", "/v1/ap/connections", plain[1], map[string]any{"connections": list}, nil); code != 200 || b["recorded"] != float64(3) {
		t.Fatalf("connections: %d %v", code, b)
	}
	_, body = f.do("GET", "/v1/clients/"+mac+"/connections", "griff", nil)
	conns := body["connections"].([]any)
	if got, want := line(conns[0].(map[string]any)["way"]), "auth ok, assoc ok 4, key ok 40, dhcp bad 8000, gateway none, dns none, internet none"; got != want {
		t.Errorf("the latest attempt's way = %s, want %s", got, want)
	}
	if got, want := line(conns[1].(map[string]any)["way"]), "auth ok, assoc ok 4, key ok 20, dhcp ok 60, gateway none, dns ok 40, internet none"; got != want {
		t.Errorf("the attempt before's way = %s, want %s", got, want)
	}

	// What they come to: the thing tested, over its three tests; and the
	// network's joins, over the client's two attempts and not the test
	// client's.
	typical := func(way any) string {
		var out []string
		list, _ := way.([]any)
		for _, x := range list {
			p := x.(map[string]any)
			s := fmt.Sprintf("%v %v/%v", p["k"], p["passed"], p["stopped"])
			if p["ms"] != nil {
				s += fmt.Sprintf(" %v %v %v", p["low"], p["ms"], p["high"])
			}
			out = append(out, s)
		}
		return strings.Join(out, ", ")
	}
	_, view := f.do("GET", "/v1/assurance", "griff", nil)
	var ch map[string]any
	for _, x := range view["checks"].([]any) {
		if m := x.(map[string]any); m["tester"] == plain[0] {
			ch = m
		}
	}
	if ch == nil {
		t.Fatalf("checks = %v", view["checks"])
	}
	if got, want := typical(ch["way"]), "offer 2/1 1000 2000 2000, dhcp 2/0 300 300 300, gateway 2/0 1.5 1.5 1.5, dns 2/0 40 40 40, release 2/0"; got != want {
		t.Errorf("the thing's way = %s, want %s", got, want)
	}
	if got := line(ch["last"].(map[string]any)["detail"].(map[string]any)["way"]); !strings.HasPrefix(got, "offer bad 9000") {
		t.Errorf("the thing's latest test's way = %s", got)
	}
	if got := line(view["tests"].([]any)[0].(map[string]any)["detail"].(map[string]any)["way"]); !strings.HasPrefix(got, "offer bad 9000") {
		t.Errorf("the latest of the tests listed, its way = %s", got)
	}
	joins := view["joins"].([]any)
	if len(joins) != 1 {
		t.Fatalf("joins = %v", joins)
	}
	j := joins[0].(map[string]any)
	if got, want := typical(j["way"]), "auth 2/0, assoc 2/0 4 4 4, key 2/0 20 40 40, dhcp 1/1 60 60 60, gateway 0/0, dns 1/0 40 40 40, internet 0/0"; got != want || j["way_of"] != float64(2) {
		t.Errorf("the network's way = %s of %v, want %s of 2", got, j["way_of"], want)
	}
}
