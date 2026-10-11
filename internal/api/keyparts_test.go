package api

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// keysSaid is the keys of a way as a line: how the point went, and in
// brackets each message of its handshake, with what is said of it in
// quotes; nothing in brackets where the way has no parts.
func keysSaid(way []wayPoint) string {
	for _, p := range way {
		if p.K != "key" {
			continue
		}
		s := "key " + p.State
		if p.MS != nil {
			s += fmt.Sprintf(" %g", *p.MS)
		}
		if p.Parts == nil {
			return s
		}
		var parts []string
		for _, m := range p.Parts {
			one := m.K + " " + m.State
			if m.MS != nil {
				one += fmt.Sprintf(" %g", *m.MS)
			}
			if m.Said != "" {
				one += fmt.Sprintf(" %q", m.Said)
			}
			parts = append(parts, one)
		}
		return s + " [" + strings.Join(parts, ", ") + "]"
	}
	return "no keys"
}

// The key handshake is read a message at a time (0133), from the steps an
// AP records where hostapd says them: as aeolus-journey wrote them of the
// C-360's log on 2026-10-11, agent/test/journey.out.
func TestTheKeysAMessageAtATime(t *testing.T) {
	const joined = `{"t":3,"stage":"auth","what":"authenticated","ok":true},
		{"t":16,"stage":"assoc","what":"associated","ok":true},
		{"t":18,"stage":"key","what":"message 1 of 4 sent to it","k":"m1"}`
	const through = `{"t":32,"stage":"key","what":"message 2 of 4 came from it","k":"m2"},
		{"t":32,"stage":"key","what":"message 3 of 4 sent to it","k":"m3"},
		{"t":36,"stage":"key","what":"message 4 of 4 came from it","k":"m4"},
		{"t":99,"stage":"key","what":"let on to the network","ok":true},
		{"t":100,"stage":"key","what":"key handshake completed","ok":true}`
	wrong := func(t int) string {
		return fmt.Sprintf(`{"t":%d,"stage":"key","what":"message 2 of 4 came from it","k":"m2"},
		{"t":%d,"stage":"key","what":"wrong passphrase","ok":false,"k":"m2"}`, t, t)
	}
	again := func(t, n, try int) string {
		return fmt.Sprintf(`{"t":%d,"stage":"key","what":"message %d of 4 sent to it, try %d","k":"m%d"}`, t, n, try, n)
	}
	for name, c := range map[string]struct {
		outcome, stage, events string
		want                   string
	}{
		// The right passphrase: each message in what it took after the one
		// before it, the AP's first after the association.
		"the four messages": {"connected", "key", `[` + joined + `,` + through + `]`,
			"key ok 82 [m1 ok 2, m2 ok 14, m3 ok 0, m4 ok 4]"},
		// A wrong one: the client's answer came each time, in 15 ms, and
		// did not check out. Nothing after it was come to.
		"a wrong passphrase": {"failed", "key", `[` + joined + `,` + wrong(33) + `,` + again(1018, 1, 2) + `,` + wrong(1033) + `,` +
			again(2018, 1, 3) + `,` + wrong(2033) + `,` + again(3018, 1, 4) + `,` + wrong(3033) + `,
			{"t":4018,"stage":"key","what":"gave up: message 1 of 4 was sent 4 times"},
			{"t":9018,"stage":"end","what":"deauthenticated due to local deauth request"}]`,
			`key bad 4000 [m1 ok 2, m2 bad 15 "wrong passphrase", m3 none, m4 none]`},
		// One that answered nothing, and was sent off: it failed nothing
		// that is known, and its answer is what was waited for.
		"no answer to the first": {"left", "key", `[` + joined + `,` + again(1018, 1, 2) + `,` + again(2018, 1, 3) + `,` + again(3018, 1, 4) + `,
			{"t":4018,"stage":"key","what":"gave up: message 1 of 4 was sent 4 times"},
			{"t":9018,"stage":"end","what":"deauthenticated due to local deauth request"}]`,
			`key begun [m1 ok 2, m2 begun "no answer", m3 none, m4 none]`},
		// The same, of an attempt that failed there.
		"no answer to the first, and never let on": {"failed", "key", `[` + joined + `,` + again(1018, 1, 2) + `]`,
			`key bad 1000 [m1 ok 2, m2 bad "no answer", m3 none, m4 none]`},
		// Its answer was right, and its last never came.
		"no answer to the third": {"failed", "key", `[` + joined + `,
			{"t":32,"stage":"key","what":"message 2 of 4 came from it","k":"m2"},
			{"t":32,"stage":"key","what":"message 3 of 4 sent to it","k":"m3"},` + again(1032, 3, 2) + `,` + again(2032, 3, 3) + `,
			{"t":4032,"stage":"key","what":"gave up: message 3 of 4 was sent 4 times"}]`,
			`key bad 4014 [m1 ok 2, m2 ok 14, m3 ok 0, m4 bad "no answer"]`},
		// A wrong answer, and then a right one on the next try: it went
		// well, in what the last answer took.
		"wrong, and then right": {"online", "key", `[` + joined + `,` + wrong(33) + `,` + again(1018, 1, 2) + `,
			{"t":1030,"stage":"key","what":"message 2 of 4 came from it","k":"m2"},
			{"t":1031,"stage":"key","what":"message 3 of 4 sent to it","k":"m3"},
			{"t":1036,"stage":"key","what":"message 4 of 4 came from it","k":"m4"},
			{"t":1100,"stage":"key","what":"key handshake completed","ok":true}]`,
			"key ok 1082 [m1 ok 2, m2 ok 12, m3 ok 1, m4 ok 5]"},
		// Seen through, with a line lost on the way: what was not said
		// went well all the same, in no time that is known.
		"a message not said": {"connected", "key", `[` + joined + `,
			{"t":32,"stage":"key","what":"message 3 of 4 sent to it","k":"m3"},
			{"t":100,"stage":"key","what":"key handshake completed","ok":true}]`,
			"key ok 82 [m1 ok 2, m2 ok, m3 ok, m4 ok]"},
		// Where hostapd does not say the messages, its word for a wrong
		// passphrase is still of the second.
		"a wrong passphrase, and no message said": {"failed", "key", `[
			{"t":0,"stage":"auth","what":"authenticated","ok":true},
			{"t":5,"stage":"assoc","what":"associated","ok":true},
			{"t":1100,"stage":"key","what":"wrong passphrase","ok":false,"k":"m2"}]`,
			`key bad 1095 [m1 ok, m2 bad "wrong passphrase", m3 none, m4 none]`},
		// And one from an agent before this has its keys whole.
		"an agent from before": {"connected", "key", `[
			{"t":0,"stage":"auth","what":"authenticated","ok":true},
			{"t":4,"stage":"assoc","what":"associated","ok":true},
			{"t":31,"stage":"key","what":"let on to the network","ok":true}]`,
			"key ok 27"},
		// The handshake begun, and the client gone before its answer was
		// due: the AP's first went, and no more is known.
		"left during it": {"left", "key", `[` + joined + `,
			{"t":300,"stage":"end","what":"disassociated"}]`,
			`key begun [m1 ok 2, m2 begun "no answer", m3 none, m4 none]`},
	} {
		way, _ := connWay(attemptOf(c.outcome, c.stage, c.events))
		if got := keysSaid(way); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, c.want)
		}
	}
}

// What many handshakes come to (0133): at each message how many passed it,
// how long it typically took, and how many stopped there, over the ways
// that said them; one that did not is counted at the keys alone.
func TestTheKeysOfMany(t *testing.T) {
	ms := func(v float64) *float64 { return &v }
	through := func(key, m2 float64) []wayPoint {
		return []wayPoint{{K: "assoc", State: "ok", MS: ms(4)}, {K: "key", State: "ok", MS: ms(key), Parts: []wayPoint{
			{K: "m1", State: "ok", MS: ms(2)}, {K: "m2", State: "ok", MS: ms(m2)}, {K: "m3", State: "ok", MS: ms(0)}, {K: "m4", State: "ok", MS: ms(4)}}}}
	}
	tally := newWayTally(wayClient)
	tally.add(through(80, 14), "")
	tally.add(through(90, 30), "")
	tally.add(through(70, 10), "")
	// A wrong passphrase stopped at the second message.
	tally.add([]wayPoint{{K: "assoc", State: "ok", MS: ms(4)}, {K: "key", State: "bad", MS: ms(4000), Parts: []wayPoint{
		{K: "m1", State: "ok", MS: ms(2)}, {K: "m2", State: "bad", MS: ms(15), Said: saidWrong}, {K: "m3", State: "none"}, {K: "m4", State: "none"}}}}, "key")
	// One that went off during it stopped nowhere: it failed nothing.
	tally.add([]wayPoint{{K: "assoc", State: "ok", MS: ms(4)}, {K: "key", State: "begun", Parts: []wayPoint{
		{K: "m1", State: "ok", MS: ms(3)}, {K: "m2", State: "begun", Said: saidNoAnswer}, {K: "m3", State: "none"}, {K: "m4", State: "none"}}}}, "")
	// And one whose AP said no message, let on.
	tally.add([]wayPoint{{K: "assoc", State: "ok", MS: ms(4)}, {K: "key", State: "ok", MS: ms(60)}}, "")
	said := func(list []wayTypical) string {
		var out []string
		for _, p := range list {
			s := fmt.Sprintf("%s %d/%d", p.K, p.Passed, p.Stopped)
			if p.MS != nil {
				s += fmt.Sprintf(" %g %g %g", *p.Low, *p.MS, *p.High)
			}
			out = append(out, s)
		}
		return strings.Join(out, ", ")
	}
	for _, p := range tally.typical() {
		switch p.K {
		case "key":
			if got, want := said([]wayTypical{p}), "key 4/1 60 80 90"; got != want {
				t.Errorf("the keys of many:\n got %s\nwant %s", got, want)
			}
			if got, want := said(p.Parts), "m1 5/0 2 2 3, m2 3/1 10 14 30, m3 3/0 0 0 0, m4 3/0 4 4 4"; got != want {
				t.Errorf("the messages of many:\n got %s\nwant %s", got, want)
			}
		default:
			if p.Parts != nil {
				t.Errorf("%s has parts: %v", p.K, p.Parts)
			}
		}
	}
	// With no way that said them, the keys have no parts.
	plain := newWayTally(wayClient)
	plain.add([]wayPoint{{K: "key", State: "ok", MS: ms(60)}}, "")
	for _, p := range plain.typical() {
		if p.Parts != nil {
			t.Errorf("%s has parts with none said: %v", p.K, p.Parts)
		}
	}
}

// An AP's record names the message a step is of, and the manager keeps it
// (0133): a client's attempt read back has its keys in parts. A name the
// manager does not know, or one on a step that is not of the keys, is left
// out, and the step kept.
func TestAMessageNamedIsKept(t *testing.T) {
	f := newFixture(t)
	_, token, _ := f.adopted()
	const mac = "72:bb:66:12:2a:94"
	yes := true
	rec := map[string]any{
		"mac": mac, "bss": "phy0-ap0", "network": "sweet", "ssid": "Sweet Spot", "band": "5g",
		"started": time.Now().UnixMilli() - 60000, "took_ms": 4018, "outcome": "failed", "stage": "key", "reason": "wrong passphrase",
		"events": []any{
			map[string]any{"t": 3, "stage": "auth", "what": "authenticated", "ok": yes, "k": "m1"},
			map[string]any{"t": 16, "stage": "assoc", "what": "associated", "ok": yes},
			map[string]any{"t": 18, "stage": "key", "what": "message 1 of 4 sent to it", "k": "m1"},
			map[string]any{"t": 33, "stage": "key", "what": "message 2 of 4 came from it", "k": "m2"},
			map[string]any{"t": 33, "stage": "key", "what": "wrong passphrase", "ok": false, "k": "m2"},
			map[string]any{"t": 4018, "stage": "key", "what": "something a later agent says", "k": "m9"},
		},
	}
	if code, _, body := f.apDo("POST", "/v1/ap/connections", token, map[string]any{"connections": []any{rec}}, nil); code != 200 || body["recorded"] != float64(1) {
		t.Fatalf("reported: %d %v", code, body)
	}
	_, got := f.do("GET", "/v1/clients/"+mac+"/connections", "griff", nil)
	list := got["connections"].([]any)
	if len(list) != 1 {
		t.Fatalf("connections = %v", got)
	}
	one := list[0].(map[string]any)
	var named []string
	for _, e := range one["record"].(map[string]any)["events"].([]any) {
		k, _ := e.(map[string]any)["k"].(string)
		named = append(named, k)
	}
	if got, want := strings.Join(named, ","), ",,m1,m2,m2,"; got != want {
		t.Errorf("the steps' messages = %q, want %q", got, want)
	}
	var parts []string
	for _, p := range one["way"].([]any) {
		p := p.(map[string]any)
		if p["k"] != "key" {
			if p["parts"] != nil {
				t.Errorf("%v has parts", p["k"])
			}
			continue
		}
		for _, m := range p["parts"].([]any) {
			m := m.(map[string]any)
			s := fmt.Sprintf("%v %v", m["k"], m["state"])
			if m["said"] != nil {
				s += fmt.Sprintf(" %q", m["said"])
			}
			parts = append(parts, s)
		}
	}
	if got, want := strings.Join(parts, ", "), `m1 ok, m2 bad "wrong passphrase", m3 none, m4 none`; got != want {
		t.Errorf("its keys = %s, want %s", got, want)
	}
}
