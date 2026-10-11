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
// where it has one; and after a bar the point it stopped at, where it did.
func said(way []wayPoint, stopped string) string {
	var out []string
	for _, p := range way {
		s := p.K + " " + p.State
		if p.MS != nil {
			s += fmt.Sprintf(" %g", *p.MS)
		}
		out = append(out, s)
	}
	line := strings.Join(out, ", ")
	if stopped != "" {
		line += " | " + stopped
	}
	return line
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
			"find ok 2160, auth ok 20, assoc ok 10, signin ok 160, key ok 61, dhcp bad 10039, gateway none, dns none | dhcp"},
		// A look that missed, and the next that found it: it went well, in
		// a test that passed, and stopped nowhere.
		"found at the second look": {`{"band":"5g","outcome":"passed","stage":"done","steps":[
			{"t":150,"stage":"find","what":"looks for the network"},
			{"t":3200,"stage":"find","what":"not heard: looks again","ok":false},
			{"t":6400,"stage":"find","what":"found it","ok":true},
			{"t":6440,"stage":"auth","what":"authenticated","ok":true},
			{"t":6460,"stage":"assoc","what":"associated","ok":true},
			{"t":6600,"stage":"key","what":"let on to the network","ok":true}]}`,
			"find ok 6250, auth ok 40, assoc ok 20, key ok 140, dhcp none, gateway none, dns none"},
		// A gateway that answers ARP and no ping is there, with no time to
		// give: the three seconds its pings took say nothing.
		"a gateway that answers no ping": {`{"band":"5g","outcome":"passed","stage":"done","steps":[
			{"t":300,"stage":"find","what":"found it","ok":true},
			{"t":500,"stage":"key","what":"let on to the network","ok":true},
			{"t":800,"stage":"dhcp","what":"given","ok":true,"ms":290},
			{"t":3900,"stage":"gateway","what":"is there, and answers no ping","ok":true},
			{"t":3940,"stage":"dns","what":"answers","ok":true,"ms":40}]}`,
			"find ok 300, auth none, assoc none, key ok 200, dhcp ok 290, gateway ok, dns ok 40"},
		// It failed at its gateway, and looked a name up all the same: the
		// gateway is where it stopped, and the name went well.
		"no gateway, and a name": {`{"band":"5g","outcome":"failed","stage":"gateway","steps":[
			{"t":300,"stage":"find","what":"found it","ok":true},
			{"t":500,"stage":"key","what":"let on to the network","ok":true},
			{"t":800,"stage":"dhcp","what":"given","ok":true,"ms":290},
			{"t":3900,"stage":"gateway","what":"does not answer","ok":false},
			{"t":3940,"stage":"dns","what":"answers","ok":true,"ms":40}]}`,
			"find ok 300, auth none, assoc none, key ok 200, dhcp ok 290, gateway bad, dns ok 40 | gateway"},
		// It failed where no step says so: the point it stopped at is bad.
		"stopped where nothing was said": {`{"band":"5g","outcome":"failed","stage":"auth","steps":[
			{"t":300,"stage":"find","what":"found it","ok":true}]}`,
			"find ok 300, auth bad, assoc none, key none, dhcp none, gateway none, dns none | auth"},
		// Over the wire the address's time is what DHCP took in all, less
		// the offer's: not when its step came, which is after the test's own
		// 300 ms look for another that uses the address.
		"over the wire": {`{"band":"wire","outcome":"passed","stage":"done","steps":[
			{"t":1,"stage":"dhcp","k":"offer","what":"asks for an address (discover)"},
			{"t":2003,"stage":"dhcp","k":"offer","what":"offered 192.168.20.113","ok":true},
			{"t":2004,"stage":"dhcp","what":"asks to use the address (request)"},
			{"t":2312,"stage":"dhcp","what":"given 192.168.20.113","ok":true,"ms":2010},
			{"t":2315,"stage":"gateway","what":"is there","ok":true,"ms":1.9},
			{"t":2360,"stage":"dns","what":"answers","ok":true,"ms":43},
			{"t":2362,"stage":"dhcp","k":"release","what":"gives 192.168.20.113 back","ok":true}]}`,
			"offer ok 2002, dhcp ok 8, gateway ok 1.9, dns ok 43, release ok"},
		// An agent from before says nothing of the offer: its address is
		// one point, with DHCP's own time; and what it gave back is told
		// from what it was given by where it comes.
		"over the wire, by an agent from before": {`{"band":"wire","outcome":"passed","stage":"done","steps":[
			{"t":1,"stage":"dhcp","what":"asks for an address (discover)"},
			{"t":2003,"stage":"dhcp","what":"offered 192.168.20.113"},
			{"t":2004,"stage":"dhcp","what":"asks to use the address (request)"},
			{"t":2312,"stage":"dhcp","what":"given 192.168.20.113","ok":true,"ms":2010},
			{"t":2315,"stage":"gateway","what":"is there","ok":true,"ms":1.9},
			{"t":2360,"stage":"dns","what":"answers","ok":true,"ms":43},
			{"t":2362,"stage":"dhcp","what":"gives 192.168.20.113 back"}]}`,
			"dhcp ok 2010, gateway ok 1.9, dns ok 43, release ok"},
		"over the wire, and no offer": {`{"band":"wire","outcome":"failed","stage":"dhcp","steps":[
			{"t":5,"stage":"dhcp","k":"offer","what":"asks for an address (discover)"},
			{"t":3005,"stage":"dhcp","k":"offer","what":"asks for an address (discover)"},
			{"t":9005,"stage":"dhcp","k":"offer","what":"no answer to its DHCP requests","ok":false}]}`,
			"offer bad 9000, dhcp none, gateway none, dns none | offer"},
		"over the wire, offered and not given": {`{"band":"wire","outcome":"failed","stage":"dhcp","steps":[
			{"t":5,"stage":"dhcp","k":"offer","what":"asks for an address (discover)"},
			{"t":2010,"stage":"dhcp","k":"offer","what":"offered 192.168.20.113","ok":true},
			{"t":2012,"stage":"dhcp","what":"asks to use the address (request)"},
			{"t":8012,"stage":"dhcp","what":"no answer to its request","ok":false}]}`,
			"offer ok 2005, dhcp bad 6000, gateway none, dns none | dhcp"},
		// One that could not be made at all stopped before any point.
		"over the wire, and not made": {`{"band":"wire","outcome":"failed","stage":"find","steps":[
			{"t":3,"stage":"find","what":"the uplink does not carry that network","ok":false}]}`,
			"dhcp none, gateway none, dns none"},
		// A point this manager does not know is read as its stage.
		"a step of a point not known": {`{"band":"wire","outcome":"passed","stage":"done","steps":[
			{"t":1,"stage":"dhcp","what":"asks for an address (discover)"},
			{"t":2312,"stage":"dhcp","k":"novel","what":"given 192.168.20.113","ok":true,"ms":2010}]}`,
			"dhcp ok 2010, gateway none, dns none"},
		// The offer is taken off a time that is DHCP's own: not off one
		// read from when the steps came, where the step says no time.
		"over the wire, the address with no time of its own": {`{"band":"wire","outcome":"passed","stage":"done","steps":[
			{"t":1,"stage":"dhcp","k":"offer","what":"asks for an address (discover)"},
			{"t":2003,"stage":"dhcp","k":"offer","what":"offered 192.168.20.113","ok":true},
			{"t":2004,"stage":"dhcp","what":"asks to use the address (request)"},
			{"t":2312,"stage":"dhcp","what":"given 192.168.20.113","ok":true}]}`,
			"offer ok 2002, dhcp ok 308, gateway none, dns none"},
		// And never below nothing, where the two disagree.
		"over the wire, an offer longer than all of it": {`{"band":"wire","outcome":"passed","stage":"done","steps":[
			{"t":1,"stage":"dhcp","k":"offer","what":"asks for an address (discover)"},
			{"t":2003,"stage":"dhcp","k":"offer","what":"offered 192.168.20.113","ok":true},
			{"t":2004,"stage":"dhcp","what":"asks to use the address (request)"},
			{"t":2312,"stage":"dhcp","what":"given 192.168.20.113","ok":true,"ms":1500}]}`,
			"offer ok 2002, dhcp ok 0, gateway none, dns none"},
		// What it measured is taken for a point nothing was seen of, and
		// for no other; and only where it found something: a gateway that
		// answered, a name with an address.
		"with no steps, and little found": {`{"band":"5g","outcome":"failed","stage":"gateway","steps":[],
			"dhcp":{"ms":300,"address":"192.168.20.9","mask":24},"gateway":{"address":"192.168.20.1","sent":3,"got":0},
			"dns":{"server":"192.168.20.254","name":"example.com","ms":41,"rcode":0,"answers":0}}`,
			"find none, auth none, assoc none, key none, dhcp ok 300, gateway bad, dns none | gateway"},
		"with no steps, and no such name": {`{"band":"5g","outcome":"passed","stage":"done","steps":[],
			"gateway":{"address":"192.168.20.1","sent":3,"got":0,"mac":"28:e7:1d:ca:29:13"},
			"dns":{"server":"192.168.20.254","name":"example.com","ms":41,"rcode":3,"answers":0}}`,
			"find none, auth none, assoc none, key none, dhcp none, gateway ok, dns none"},
		"an address begun, and one measured": {`{"band":"5g","outcome":"passed","stage":"done","steps":[
			{"t":500,"stage":"dhcp","what":"asks for an address"}],
			"dhcp":{"ms":300,"address":"192.168.20.9","mask":24}}`,
			"find none, auth none, assoc none, key none, dhcp begun, gateway none, dns none"},
		// All that is left of a test whose steps were not kept: what it
		// measured.
		"with no steps": {`{"band":"5g","outcome":"passed","stage":"done","steps":[],
			"dhcp":{"ms":300,"address":"192.168.20.9","mask":24},"gateway":{"address":"192.168.20.1","sent":3,"got":3,"avg":4.2},
			"dns":{"server":"192.168.20.254","name":"example.com","ms":41,"rcode":0,"answers":2}}`,
			"find none, auth none, assoc none, key none, dhcp ok 300, gateway ok 4.2, dns ok 41"},
		// Steps that came out of order take no time away: a point that
		// ends before the one before it has none, and one that ends before
		// its own asking is timed from the point before.
		"steps out of order": {`{"band":"5g","outcome":"passed","stage":"done","steps":[
			{"t":900,"stage":"find","what":"found it","ok":true},
			{"t":400,"stage":"key","what":"let on to the network","ok":true},
			{"t":500,"stage":"dhcp","what":"asks"},
			{"t":450,"stage":"dhcp","what":"given","ok":true}]}`,
			"find ok 900, auth none, assoc none, key ok, dhcp ok 50, gateway none, dns none"},
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
// from its first, which has no time of its own; what it asks for, it asks
// when it chooses, so such a point took from its asking to its answer, and
// never from the point before; a name looked up, and its first connection,
// took what the first answer says.
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
	const joined = `{"t":0,"stage":"auth","what":"authenticated","ok":true},
		{"t":4,"stage":"assoc","what":"associated","ok":true},
		{"t":30,"stage":"key","what":"let on to the network","ok":true}`
	for name, c := range map[string]struct {
		c    conditions.Connection
		want string
	}{
		"online": {attemptOf("online", "internet", online),
			"auth ok, assoc ok 4, key ok 28, dhcp ok 20, gateway ok 2, dns ok 12, internet ok 23"},
		"no address": {attemptOf("failed", "dhcp", `[`+joined+`,
			{"t":200,"stage":"dhcp","what":"asks for an address (discover)"},
			{"t":4200,"stage":"dhcp","what":"asks for an address (discover)"},
			{"t":12200,"stage":"dhcp","what":"asks for an address (discover)"}]`),
			"auth ok, assoc ok 4, key ok 26, dhcp bad 12000, gateway none, dns none, internet none | dhcp"},
		"a wrong passphrase, on a network that signs in": {attemptOf("failed", "signin", `[
			{"t":0,"stage":"auth","what":"authenticated","ok":true},
			{"t":5,"stage":"assoc","what":"associated","ok":true},
			{"t":1100,"stage":"signin","what":"refused","ok":false}]`),
			"auth ok, assoc ok 5, signin bad 1095, key none, dhcp none, gateway none, dns none, internet none | signin"},
		// One that kept its address asks for none: it has one, and that
		// took no time. Its gateway, asked and not yet heard from, is begun.
		"let on, keeping its address": {attemptOf("connected", "key", `[`+joined+`,
			{"t":100,"stage":"dhcp","what":"uses 192.168.20.61, which it had already","ok":true},
			{"t":190,"stage":"gateway","what":"asks who has its gateway"}]`),
			"auth ok, assoc ok 4, key ok 26, dhcp ok, gateway begun, dns none, internet none"},
		// As an agent from before said it: an address begun, and no more.
		"let on, keeping its address, by an agent from before": {attemptOf("connected", "key", `[`+joined+`,
			{"t":100,"stage":"dhcp","what":"uses 192.168.20.61, which it had already"}]`),
			"auth ok, assoc ok 4, key ok 26, dhcp begun, gateway none, dns none, internet none"},
		// One name not found among those looked up: its names went well,
		// in what the first good answer took.
		"a name not found, and one found": {attemptOf("online", "dns", `[`+joined+`,
			{"t":300,"stage":"dns","what":"asks for a.example"},
			{"t":305,"stage":"dns","what":"no such name","ok":false,"ms":5},
			{"t":320,"stage":"dns","what":"asks for b.example"},
			{"t":332,"stage":"dns","what":"answers b.example","ok":true,"ms":12}]`),
			"auth ok, assoc ok 4, key ok 26, dhcp none, gateway none, dns ok 12, internet none"},
		// And the other way about, the good answer first.
		"a name found, and then one not": {attemptOf("online", "dns", `[`+joined+`,
			{"t":300,"stage":"dns","what":"asks for b.example"},
			{"t":312,"stage":"dns","what":"answers b.example","ok":true,"ms":12},
			{"t":320,"stage":"dns","what":"asks for a.example"},
			{"t":325,"stage":"dns","what":"no such name","ok":false,"ms":5}]`),
			"auth ok, assoc ok 4, key ok 26, dhcp none, gateway none, dns ok 12, internet none"},
		// Answers seen with no asking, and no time of their own, have no
		// time: how long after its keys says nothing of the network.
		"answers with no asking seen": {attemptOf("online", "internet", `[`+joined+`,
			{"t":300,"stage":"gateway","what":"its gateway answers","ok":true},
			{"t":400,"stage":"dns","what":"answers","ok":true},
			{"t":500,"stage":"internet","what":"answers","ok":true}]`),
			"auth ok, assoc ok 4, key ok 26, dhcp none, gateway ok, dns ok, internet ok"},
		// A gateway that did not answer, and a client that got on all the
		// same: the gateway went badly, and the client stopped nowhere.
		"no gateway, and online": {attemptOf("online", "dns", `[`+joined+`,
			{"t":300,"stage":"gateway","what":"asks who has its gateway"},
			{"t":3300,"stage":"gateway","what":"its gateway does not answer","ok":false},
			{"t":3400,"stage":"dns","what":"asks for a name"},
			{"t":3409,"stage":"dns","what":"answers","ok":true,"ms":9}]`),
			"auth ok, assoc ok 4, key ok 26, dhcp none, gateway bad 3000, dns ok 9, internet none"},
		"nothing kept of it": {attemptOf("failed", "key", `[]`),
			"auth none, assoc none, key bad, dhcp none, gateway none, dns none, internet none | key"},
		// It left of its own accord: where it had got to is not where it
		// failed.
		"left": {attemptOf("left", "assoc", `[
			{"t":0,"stage":"auth","what":"authenticated","ok":true},
			{"t":4,"stage":"assoc","what":"associated","ok":true}]`),
			"auth ok, assoc ok 4, key none, dhcp none, gateway none, dns none, internet none"},
	} {
		if got := said(connWay(c.c)); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, c.want)
		}
	}
}

// Many ways come to one (0130): at each point how many passed it, how many
// stopped at it and went no further, and of the times the least, the
// middle and the most. A point that went badly in a way that went on is
// neither. A point seen in none of them is left out where it is not every
// way's.
func TestTheWayOfMany(t *testing.T) {
	ms := func(v float64) *float64 { return &v }
	tally := newWayTally(wayClient)
	if got := tally.typical(); got != nil {
		t.Fatalf("of none: %v", got)
	}
	for _, v := range []float64{30, 10, 20} {
		tally.add([]wayPoint{{K: "auth", State: "ok"}, {K: "assoc", State: "ok", MS: ms(4)}, {K: "key", State: "ok", MS: ms(v)},
			{K: "dhcp", State: "ok", MS: ms(v * 10)}, {K: "gateway", State: "none"}}, "")
	}
	tally.add([]wayPoint{{K: "auth", State: "ok"}, {K: "assoc", State: "ok", MS: ms(4)}, {K: "key", State: "ok", MS: ms(40)},
		{K: "dhcp", State: "bad", MS: ms(12000)}}, "dhcp")
	tally.add([]wayPoint{{K: "auth", State: "ok"}, {K: "assoc", State: "begun"}}, "")
	// Its gateway went badly, and it got on: that is no one stopped.
	tally.add([]wayPoint{{K: "auth", State: "ok"}, {K: "gateway", State: "bad", MS: ms(3000)}, {K: "dns", State: "ok", MS: ms(9)}}, "")
	var out []string
	for _, p := range tally.typical() {
		s := fmt.Sprintf("%s %d/%d", p.K, p.Passed, p.Stopped)
		if p.MS != nil {
			s += fmt.Sprintf(" %g %g %g", *p.Low, *p.MS, *p.High)
		}
		out = append(out, s)
	}
	want := "auth 6/0, assoc 4/0 4 4 4, key 4/0 10 30 40, dhcp 3/1 100 200 300, gateway 0/0, dns 1/0 9 9 9, internet 0/0"
	if got := strings.Join(out, ", "); got != want {
		t.Errorf("many:\n got %s\nwant %s", got, want)
	}
}

// What the API says of the way (0130): a test asked of an AP has it among
// what it found, though an AP that sends one is not believed, and a point
// the manager does not know is no reason to refuse a result; a thing tested
// has what its tests come to; a client's attempts each have theirs; and a
// network's joins what its clients' come to, of the APs the caller may view
// and the time asked, but for the tests' own client.
func TestTheWayOnline(t *testing.T) {
	f := newFixture(t)
	change := func(body map[string]any) {
		t.Helper()
		if code, b := f.change("griff", body); code != 200 {
			t.Fatalf("%v: %d %v", body["kind"], code, b)
		}
	}
	change(map[string]any{"kind": "add-builtins"})
	change(map[string]any{"kind": "add-folder", "tree": "locations", "node": "shed", "name": "Shed", "parent": "house"})
	change(map[string]any{"kind": "grant", "account": "tenant", "tree": "locations", "node": "shed", "role": "viewer"})
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	plain := strings.Split(f.enrollAt("Plain", "02:00:00:00:00:5b", "office", []any{serving}, map[string]int{"5g": 44}), "|")
	shed := strings.Split(f.enrollAt("ShedAP", "02:00:00:00:00:5c", "shed", []any{serving}, map[string]int{"5g": 149}), "|")
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
	// A test over the wire: its offer after so long, its acknowledgement 2
	// ms after its request, and its step 300 ms after that, as the agent
	// looks for another that uses the address first. keyed, by an agent
	// that names the steps of the offer and the release.
	found := func(passed bool, offer int, keyed bool) map[string]any {
		k := func(step map[string]any, point string) map[string]any {
			if keyed {
				step["k"] = point
			}
			return step
		}
		offered := map[string]any{"t": 5 + offer, "stage": "dhcp", "what": "offered 192.168.20.113"}
		back := map[string]any{"t": 352 + offer, "stage": "dhcp", "what": "gives 192.168.20.113 back"}
		if keyed {
			offered["ok"], back["ok"] = yes, yes
		}
		d := map[string]any{
			"v": 1, "ssid": "Sweet Spot", "band": "wire", "client": "0e:00:00:5b:00:20", "outcome": "passed", "stage": "done", "reason": "",
			"summary": "passed", "took_ms": offer + 400, "wire": map[string]any{"device": "eth1", "vlan": 20, "tagged": true},
			// An AP that says what its way was is not believed.
			"way": []any{map[string]any{"k": "dhcp", "state": "bad"}},
			"steps": []any{
				k(map[string]any{"t": 5, "stage": "dhcp", "what": "asks for an address (discover)"}, "offer"),
				k(offered, "offer"),
				// A point this manager does not know: kept without it.
				map[string]any{"t": 7 + offer, "stage": "dhcp", "k": "novel", "what": "asks to use the address (request)"},
				map[string]any{"t": 309 + offer, "stage": "dhcp", "what": "given 192.168.20.113", "ok": yes, "ms": 4 + offer},
				map[string]any{"t": 310 + offer, "stage": "gateway", "what": "is there", "ok": yes, "ms": 1.5},
				map[string]any{"t": 350 + offer, "stage": "dns", "what": "answers", "ok": yes, "ms": 40},
				k(back, "release"),
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
	// Four tests on the schedule: an agent from before, with its address in
	// 3004 ms; then offers in 2000 ms and in 1000; and one with none.
	for i, d := range []map[string]any{found(true, 3000, false), found(true, 2000, true), found(true, 1000, true), found(false, 0, true)} {
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
	detail := func(i int) map[string]any { return acts[i].(map[string]any)["detail"].(map[string]any) }
	for i, want := range []string{
		"offer bad 9000, dhcp none, gateway none, dns none",
		"offer ok 1000, dhcp ok 4, gateway ok 1.5, dns ok 40, release ok",
		"offer ok 2000, dhcp ok 4, gateway ok 1.5, dns ok 40, release ok",
		"dhcp ok 3004, gateway ok 1.5, dns ok 40, release ok",
	} {
		if got := line(detail(i)["way"]); got != want {
			t.Errorf("test %d from the latest, its way = %s, want %s", i, got, want)
		}
	}
	// The point not known was left out as the result was kept.
	for _, s := range detail(1)["steps"].([]any) {
		if k := s.(map[string]any)["k"]; k != nil && k != "offer" && k != "release" {
			t.Errorf("a step kept its point %v", k)
		}
	}

	// A client's attempts on the office AP, and on the shed's; the tests'
	// own client's, which is no one's; and one from before the time asked.
	const mac, other, scan = "72:bb:66:12:2a:01", "72:bb:66:12:2a:02", "02:00:00:00:0a:f0"
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
	post := func(tok string, list ...any) {
		t.Helper()
		if code, _, b := f.apDo("POST", "/v1/ap/connections", tok, map[string]any{"connections": list}, nil); code != 200 || b["recorded"] != float64(len(list)) {
			t.Fatalf("connections: %d %v", code, b)
		}
	}
	asks := map[string]any{"t": 200, "stage": "dhcp", "what": "asks for an address (discover)"}
	post(plain[1],
		attempt(mac, "online", "dns", 20, 30*time.Minute, asks,
			map[string]any{"t": 260, "stage": "dhcp", "what": "given 192.168.20.61", "ok": yes},
			map[string]any{"t": 300, "stage": "dns", "what": "asks for a name"},
			map[string]any{"t": 340, "stage": "dns", "what": "answers", "ok": yes, "ms": 40}),
		attempt(mac, "failed", "dhcp", 40, 20*time.Minute, asks,
			map[string]any{"t": 8200, "stage": "dhcp", "what": "asks for an address (discover)"}),
		attempt(scan, "failed", "dhcp", 900, 10*time.Minute,
			map[string]any{"t": 2000, "stage": "dhcp", "what": "asks for an address (discover)"}),
		attempt(mac, "online", "dns", 7000, 30*time.Hour))
	post(shed[1],
		attempt(other, "online", "dns", 100, 50*time.Minute),
		attempt(other, "online", "dns", 300, 40*time.Minute))
	_, body = f.do("GET", "/v1/clients/"+mac+"/connections", "griff", nil)
	conns := body["connections"].([]any)
	if got, want := line(conns[0].(map[string]any)["way"]), "auth ok, assoc ok 4, key ok 40, dhcp bad 8000, gateway none, dns none, internet none"; got != want {
		t.Errorf("the latest attempt's way = %s, want %s", got, want)
	}
	if got, want := line(conns[1].(map[string]any)["way"]), "auth ok, assoc ok 4, key ok 20, dhcp ok 60, gateway none, dns ok 40, internet none"; got != want {
		t.Errorf("the attempt before's way = %s, want %s", got, want)
	}

	// What they come to. The thing tested, over its four tests.
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
	joins := func(as, under string) (string, any) {
		t.Helper()
		_, view := f.do("GET", "/v1/assurance"+under, as, nil)
		list := view["joins"].([]any)
		if len(list) != 1 {
			t.Fatalf("joins as %s = %v", as, list)
		}
		j := list[0].(map[string]any)
		return typical(j["way"]), j["way_of"]
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
	if got, want := typical(ch["way"]), "offer 2/1 1000 2000 2000, dhcp 3/0 4 4 3004, gateway 3/0 1.5 1.5 1.5, dns 3/0 40 40 40, release 3/0"; got != want {
		t.Errorf("the thing's way = %s, want %s", got, want)
	}
	if got := line(ch["last"].(map[string]any)["detail"].(map[string]any)["way"]); !strings.HasPrefix(got, "offer bad 9000") {
		t.Errorf("the thing's latest test's way = %s", got)
	}
	if got := line(view["tests"].([]any)[0].(map[string]any)["detail"].(map[string]any)["way"]); !strings.HasPrefix(got, "offer bad 9000") {
		t.Errorf("the latest of the tests listed, its way = %s", got)
	}
	// The network's joins: over the attempts of the last day on both APs,
	// and not the test client's, nor the one of thirty hours ago.
	if got, of := joins("griff", ""); got != "auth 4/0, assoc 4/0 4 4 4, key 4/0 20 100 300, dhcp 1/1 60 60 60, gateway 0/0, dns 1/0 40 40 40, internet 0/0" || of != float64(4) {
		t.Errorf("the network's way = %s of %v", got, of)
	}
	// One who may view the shed alone has the shed's attempts alone.
	if got, of := joins("tenant", ""); got != "auth 2/0, assoc 2/0 4 4 4, key 2/0 100 300 300, dhcp 0/0, gateway 0/0, dns 0/0, internet 0/0" || of != float64(2) {
		t.Errorf("the network's way, for one who may view the shed = %s of %v", got, of)
	}
	// Where there are more than are read, it is the latest that are, of
	// what the caller may view: the shed's are older than the office's, and
	// its viewer has both of them still.
	most := joinsWayMost
	joinsWayMost = 2
	defer func() { joinsWayMost = most }()
	if got, of := joins("griff", ""); got != "auth 2/0, assoc 2/0 4 4 4, key 2/0 20 40 40, dhcp 1/1 60 60 60, gateway 0/0, dns 1/0 40 40 40, internet 0/0" || of != float64(2) {
		t.Errorf("the network's way, over the latest two = %s of %v", got, of)
	}
	if _, of := joins("tenant", ""); of != float64(2) {
		t.Errorf("the shed's viewer has %v of its two attempts, with others' counted", of)
	}
	if _, of := joins("griff", "?under=shed"); of != float64(2) {
		t.Errorf("below the shed, %v of its two attempts are read", of)
	}
}
