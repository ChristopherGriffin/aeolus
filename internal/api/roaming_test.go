package api

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Roaming (0136): the attempts of clients that came as ones already on the
// network are read back as roams: how many, of which kind, how long they
// took to be let on, where each came from, and those that failed or were
// slow.
func TestRoaming(t *testing.T) {
	f := newFixture(t)
	change := func(op map[string]any) {
		t.Helper()
		if code, b := f.change("griff", op); code != 200 {
			t.Fatalf("%v: %d %v", op, code, b)
		}
	}
	change(map[string]any{"kind": "add-builtins"})
	change(map[string]any{"kind": "add-folder", "tree": "locations", "node": "annex", "name": "Annex", "parent": "house"})
	change(map[string]any{"kind": "grant", "account": "tenant", "tree": "locations", "node": "annex", "role": "viewer"})
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	enroll := func(name, mac, folder string, radios []any, macs []string) (string, string) {
		t.Helper()
		code, _, body := f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": mac, "hostname": name, "radios": radios, "macs": macs}, nil)
		if code != 201 {
			t.Fatalf("enroll %s: %d %v", name, code, body)
		}
		ap, token := body["ap"].(string), body["token"].(string)
		change(map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": folder})
		return ap, token
	}
	const scan = "02:00:00:00:0b:f0"
	north, northToken := enroll("North", "02:00:00:00:00:3a", "office", []any{serving, spare}, []string{"02:00:00:00:00:3a", scan})
	_, southToken := enroll("South", "02:00:00:00:00:3b", "office", []any{serving}, nil)
	_, eastToken := enroll("East", "02:00:00:00:00:3c", "annex", []any{serving}, nil)

	now := time.Now()
	// One attempt, at an AP: on a radio of it, so long ago, how it went,
	// and how the client came; none, as an agent before this says nothing.
	attempt := func(token, mac, bss, band string, ago time.Duration, outcome, stage, reason string, how map[string]any) {
		t.Helper()
		rec := map[string]any{"mac": mac, "bss": bss, "network": "sweet", "ssid": "Sweet Spot", "band": band,
			"started": now.Add(-ago).UnixMilli(), "took_ms": 300, "outcome": outcome, "stage": stage, "reason": reason, "events": []any{}}
		if how != nil {
			rec["how"] = how
		}
		if code, _, body := f.apDo("POST", "/v1/ap/connections", token, map[string]any{"connections": []any{rec}}, nil); code != 200 || body["recorded"] != float64(1) || body["refused"] != float64(0) {
			t.Fatalf("an attempt of %s: %d %v", mac, code, body)
		}
	}
	client := func(n int) string { return fmt.Sprintf("72:bb:66:12:2a:%02x", n) }
	phone, laptop, tablet, watch, gadget, oldie, mixed, odd, relet := client(1), client(2), client(3), client(4), client(5), client(6), client(7), client(8), client(9)
	// A phone joins North afresh; goes to South and back by fast
	// transitions; to South once more with the four messages, slowly;
	// fails to get back on North; gets back on it; and asks there again as
	// one already on, on the radio it is on: that is no roam.
	attempt(northToken, phone, "phy0-ap0", "5g", 600*time.Second, "online", "internet", "", map[string]any{"alg": "open", "on_ms": 40})
	attempt(southToken, phone, "phy0-ap0", "5g", 500*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "ft", "on_ms": 17})
	attempt(northToken, phone, "phy0-ap0", "5g", 400*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "ft", "on_ms": 21})
	attempt(southToken, phone, "phy0-ap0", "5g", 300*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 280})
	attempt(northToken, phone, "phy0-ap0", "5g", 200*time.Second, "failed", "key", "no answer the AP could use to the first message of the key handshake, sent 4 times", map[string]any{"re": true, "alg": "open"})
	attempt(northToken, phone, "phy0-ap0", "5g", 100*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 35})
	attempt(northToken, phone, "phy0-ap0", "5g", 50*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 30})
	// A laptop comes back to North, which still held it: a second of SA
	// Queries, and its key from before. Then to South's 2.4 GHz radio with
	// a key made from its sign-in, and from that to South's other radio.
	attempt(northToken, laptop, "phy1-ap0", "2g", 90*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "open", "pmk": "pmksa", "saq": 5, "on_ms": 1198})
	attempt(southToken, laptop, "phy1-ap0", "2g", 80*time.Second, "online", "dns", "", map[string]any{"re": true, "alg": "open", "pmk": "okc", "on_ms": 30})
	attempt(southToken, laptop, "phy0-ap0", "5g", 75*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 45})
	// A tablet asks South as one already on, and goes before it is let on.
	attempt(southToken, tablet, "phy0-ap0", "5g", 70*time.Second, "left", "assoc", "before it was let on: deauthenticated", map[string]any{"re": true})
	// A watch comes to South by a fast transition of which South's log
	// has the association and no more: let on, and not timed.
	attempt(southToken, watch, "phy0-ap0", "5g", 65*time.Second, "connected", "key", "", map[string]any{"alg": "ft", "on_ms": 1, "began": "assoc"})
	// A gadget goes from North to East, in another folder.
	attempt(northToken, gadget, "phy0-ap0", "5g", 260*time.Second, "online", "dns", "", map[string]any{"alg": "open", "on_ms": 30})
	attempt(eastToken, gadget, "phy0-ap0", "5g", 250*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "ft", "on_ms": 19})
	// One that joined North a day and more ago, and has asked there since
	// as one already on, on the radio it is on: no roam, however long ago
	// it came.
	attempt(northToken, oldie, "phy0-ap0", "5g", 30*time.Hour, "online", "dns", "", map[string]any{"alg": "open", "on_ms": 30})
	attempt(northToken, oldie, "phy0-ap0", "5g", 2*time.Hour, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 20})
	attempt(northToken, oldie, "phy0-ap0", "5g", 1*time.Hour, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 20})
	// One that was on South in between, of which South's agent, an older
	// one, said only that it came online.
	attempt(northToken, mixed, "phy0-ap0", "5g", 900*time.Second, "online", "dns", "", map[string]any{"alg": "open", "on_ms": 30})
	attempt(southToken, mixed, "phy0-ap0", "5g", 850*time.Second, "online", "dns", "", nil)
	attempt(northToken, mixed, "phy0-ap0", "5g", 800*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 33})
	// One comes to South as one already on, and is let on, by a record
	// that has no time for it, as an agent of the first release with
	// roaming wrote of a client that authenticated again once it was on:
	// it went well, and is not timed.
	attempt(northToken, relet, "phy0-ap0", "5g", 950*time.Second, "online", "dns", "", map[string]any{"alg": "open", "on_ms": 30})
	attempt(southToken, relet, "phy0-ap0", "5g", 940*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "sae"})
	// North's own scan radio, testing: no client's roaming.
	attempt(southToken, scan, "phy0-ap0", "5g", 60*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "ft", "on_ms": 15})
	// How it came costs no record. What this manager cannot take of it is
	// left out and the attempt kept; a word of a later agent's is passed
	// over, and the rest taken.
	for i, how := range []map[string]any{{"re": true, "pmk": "magic"}, {"re": true, "alg": "Open System"}, {"re": true, "on_ms": 700000}, {"re": true, "saq": 65}, {"re": true, "alg": 7},
		{"alg": "open", "on_ms": 9, "hops": 3}} {
		attempt(southToken, odd, "phy0-ap0", "5g", time.Duration(40-i)*time.Second, "connected", "key", "", how)
	}
	_, kept := f.do("GET", "/v1/clients/"+odd+"/connections", "griff", nil)
	var hows []string
	for _, c := range kept["connections"].([]any) {
		c := c.(map[string]any)
		hows = append(hows, fmt.Sprintf("%v roam %v", c["record"].(map[string]any)["how"], c["roam"]))
	}
	if got, want := strings.Join(hows, "; "), "map[alg:open on_ms:9] roam <nil>; <nil> roam <nil>; <nil> roam <nil>; <nil> roam <nil>; <nil> roam <nil>; <nil> roam <nil>"; got != want {
		t.Errorf("how it came, where it cannot all be taken:\n got %s\nwant %s", got, want)
	}

	code, v := f.do("GET", "/v1/roaming", "griff", nil)
	if code != 200 {
		t.Fatalf("roaming: %d %v", code, v)
	}
	if got, want := fmt.Sprintf("%v roams, %v failed, %v slow, %v left, %v not timed, %v clients, typically %v ms, slow past %v, more %v", v["roams"], v["failed"], v["slow"], v["left"], v["untimed"], v["clients"], v["typical_ms"], v["slow_over_ms"], v["more"]),
		"12 roams, 1 failed, 2 slow, 1 left, 2 not timed, 7 clients, typically 33 ms, slow past 250, more <nil>"; got != want {
		t.Errorf("in all: %s, want %s", got, want)
	}
	var kinds []string
	for _, k := range v["kinds"].([]any) {
		k := k.(map[string]any)
		kinds = append(kinds, fmt.Sprintf("%v: %v, %v failed, %v slow, %v ms (%v to %v)", k["kind"], k["roams"], k["failed"], k["slow"], k["typical_ms"], k["low_ms"], k["high_ms"]))
	}
	if got, want := strings.Join(kinds, "; "), "ft: 4, 0 failed, 0 slow, 19 ms (17 to 21); okc: 1, 0 failed, 0 slow, 30 ms (30 to 30); pmksa: 1, 0 failed, 1 slow, 1198 ms (1198 to 1198); keys: 5, 1 failed, 1 slow, 45 ms (33 to 280); full: 1, 0 failed, 0 slow, <nil> ms (<nil> to <nil>)"; got != want {
		t.Errorf("a kind at a time:\n got %s\nwant %s", got, want)
	}
	said := func(list any) []string {
		var out []string
		for _, r := range list.([]any) {
			r := r.(map[string]any)
			from := "somewhere not known"
			if r["from_ap_name"] != nil {
				from = fmt.Sprintf("%v %v", r["from_ap_name"], r["from_band"])
			}
			s := fmt.Sprintf("%v to %v %v from %v, %v, %v", r["mac"].(string)[15:], r["ap_name"], r["band"], from, r["kind"], r["state"])
			if r["ms"] != nil {
				s += fmt.Sprintf(" in %v ms", r["ms"])
			}
			if r["untimed"] == true {
				s += ", not timed"
			}
			if r["sa_queries"] != nil {
				s += fmt.Sprintf(" after %v SA Queries", r["sa_queries"])
			}
			out = append(out, s)
		}
		return out
	}
	// The latest first. The phone's last ask, on the radio it was on, is
	// not among them; nor the scan radio's; nor any of the one that joined
	// North long ago and stayed.
	if got, want := strings.Join(said(v["latest"]), "\n"), strings.Join([]string{
		"04 to South 5g from somewhere not known, ft, ok, not timed",
		"03 to South 5g from somewhere not known, keys, left",
		"02 to South 5g from South 2g, keys, ok in 45 ms",
		"02 to South 2g from North 2g, okc, ok in 30 ms",
		"02 to North 2g from somewhere not known, pmksa, slow in 1198 ms after 5 SA Queries",
		"01 to North 5g from South 5g, keys, ok in 35 ms",
		"01 to North 5g from South 5g, keys, failed",
		"05 to East 5g from North 5g, ft, ok in 19 ms",
		"01 to South 5g from North 5g, keys, slow in 280 ms",
		"01 to North 5g from South 5g, ft, ok in 21 ms",
		"01 to South 5g from North 5g, ft, ok in 17 ms",
		"07 to North 5g from South 5g, keys, ok in 33 ms",
		"09 to South 5g from North 5g, full, ok, not timed",
	}, "\n"); got != want {
		t.Errorf("the latest:\n%s\nwant:\n%s", got, want)
	}
	if got, want := strings.Join(said(v["bad"]), "\n"), strings.Join([]string{
		"03 to South 5g from somewhere not known, keys, left",
		"02 to North 2g from somewhere not known, pmksa, slow in 1198 ms after 5 SA Queries",
		"01 to North 5g from South 5g, keys, failed",
		"01 to South 5g from North 5g, keys, slow in 280 ms",
	}, "\n"); got != want {
		t.Errorf("those that went badly:\n%s\nwant:\n%s", got, want)
	}
	if bad := v["bad"].([]any)[2].(map[string]any); !strings.Contains(fmt.Sprint(bad["reason"]), "first message of the key handshake") {
		t.Errorf("the roam that failed says why: %v", bad)
	}
	// A client's own attempts say which were roams, and not the one on
	// the radio it was on.
	_, own := f.do("GET", "/v1/clients/"+phone+"/connections", "griff", nil)
	var marks []string
	for _, c := range own["connections"].([]any) {
		if m, _ := c.(map[string]any)["roam"].(map[string]any); m != nil {
			marks = append(marks, fmt.Sprintf("%v %v", m["kind"], m["ms"]))
		} else {
			marks = append(marks, "-")
		}
	}
	if got, want := strings.Join(marks, ", "), "-, keys 35, keys <nil>, keys 280, ft 21, ft 17, -"; got != want {
		t.Errorf("the phone's attempts, the latest first = %s, want %s", got, want)
	}
	// Below one AP, its own alone, with where each came from all the same.
	_, one := f.do("GET", "/v1/roaming?under="+north+"&hours=1", "griff", nil)
	if one["roams"] != float64(5) || one["hours"] != float64(1) {
		t.Errorf("below North: %v roams over %v hours, want 5 over 1", one["roams"], one["hours"])
	}
	if got := said(one["latest"]); len(got) != 5 || got[1] != "01 to North 5g from South 5g, keys, ok in 35 ms" {
		t.Errorf("below North, the latest: %q", got)
	}
	// One who may view East alone sees the roam to it, and not from where:
	// North is not theirs to see. One with no role on any AP sees none.
	_, part := f.do("GET", "/v1/roaming", "tenant", nil)
	if got, want := strings.Join(said(part["latest"]), "; "), "05 to East 5g from somewhere not known, ft, ok in 19 ms"; got != want || part["roams"] != float64(1) {
		t.Errorf("one who may view East alone: %v roams, %s", part["roams"], got)
	}
	if _, none := f.do("GET", "/v1/roaming", "office", nil); none["roams"] != float64(0) || len(none["latest"].([]any)) != 0 {
		t.Errorf("one with no role on the APs: %v", none)
	}
	if code, b := f.do("GET", "/v1/roaming?hours=0", "griff", nil); code != 400 {
		t.Errorf("no hours: %d %v", code, b)
	}
	// So many attempts are read, of the APs asked about and no others: a
	// folder with few is whole, whatever the next one has.
	most := roamingMost
	roamingMost = 2
	defer func() { roamingMost = most }()
	if _, few := f.do("GET", "/v1/roaming?under=annex", "griff", nil); few["roams"] != float64(1) || few["more"] != nil {
		t.Errorf("below Annex, with two attempts read at the most: %v roams, more %v", few["roams"], few["more"])
	}
	if _, many := f.do("GET", "/v1/roaming?under=office", "griff", nil); many["more"] != true {
		t.Errorf("below Office, with two attempts read at the most: more %v", many["more"])
	}
}

// The kind of a roam, from how the client came.
func TestRoamKind(t *testing.T) {
	signin := []connStep{{Stage: "signin", What: "signed in with PEAP"}}
	for what, c := range map[string]struct {
		how   *connHow
		steps []connStep
		signs bool
		kind  string
		is    bool
	}{
		"nothing said of how it came":                    {nil, nil, false, "", false},
		"a join afresh":                                  {&connHow{Alg: "open"}, nil, false, "", false},
		"a fast transition":                              {&connHow{Re: true, Alg: "ft"}, nil, false, roamFT, true},
		"a fast transition, with no debug lines":         {&connHow{Alg: "ft"}, nil, true, roamFT, true},
		"a key made from a sign-in elsewhere":            {&connHow{Re: true, Alg: "open", PMK: "okc"}, nil, false, roamOKC, true},
		"a key this AP held":                             {&connHow{Re: true, Alg: "open", PMK: "pmksa"}, nil, false, roamPMKSA, true},
		"a passphrase":                                   {&connHow{Re: true, Alg: "open"}, nil, false, roamKeys, true},
		"WPA3's exchange again":                          {&connHow{Re: true, Alg: "sae"}, nil, false, roamFull, true},
		"a sign-in again":                                {&connHow{Re: true, Alg: "open"}, signin, true, roamFull, true},
		"no sign-in, on a network that signs clients in": {&connHow{Re: true, Alg: "open"}, nil, true, roamCached, true},
		"a sign-in afresh, on a network that signs in":   {&connHow{Alg: "open"}, signin, true, "", false},
		"how it came that cannot be taken":               {&connHow{Re: true, PMK: "magic"}, nil, false, "", false},
	} {
		kind, is := roamKind(roamRecord{How: c.how, Events: c.steps}, func() bool { return c.signs })
		if kind != c.kind || is != c.is {
			t.Errorf("%s: %q, a roam %v; want %q, %v", what, kind, is, c.kind, c.is)
		}
	}
}
