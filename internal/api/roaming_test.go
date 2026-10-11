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
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	enroll := func(name, mac string, radios []any, macs []string) (string, string) {
		t.Helper()
		code, _, body := f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": mac, "hostname": name, "radios": radios, "macs": macs}, nil)
		if code != 201 {
			t.Fatalf("enroll %s: %d %v", name, code, body)
		}
		ap, token := body["ap"].(string), body["token"].(string)
		change(map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": "office"})
		return ap, token
	}
	const scan = "02:00:00:00:0b:f0"
	north, northToken := enroll("North", "02:00:00:00:00:3a", []any{serving, spare}, []string{"02:00:00:00:00:3a", scan})
	_, southToken := enroll("South", "02:00:00:00:00:3b", []any{serving}, nil)

	now := time.Now()
	// One attempt, at an AP: on a radio of it, so long ago, how it went,
	// and how the client came.
	attempt := func(token, mac, bss, band string, ago time.Duration, outcome, stage, reason string, how map[string]any, steps ...any) {
		t.Helper()
		if steps == nil {
			steps = []any{}
		}
		rec := map[string]any{"mac": mac, "bss": bss, "network": "sweet", "ssid": "Sweet Spot", "band": band,
			"started": now.Add(-ago).UnixMilli(), "took_ms": 300, "outcome": outcome, "stage": stage, "reason": reason, "events": steps}
		if how != nil {
			rec["how"] = how
		}
		if code, _, body := f.apDo("POST", "/v1/ap/connections", token, map[string]any{"connections": []any{rec}}, nil); code != 200 || body["recorded"] != float64(1) {
			t.Fatalf("an attempt of %s: %d %v", mac, code, body)
		}
	}
	const phone, laptop, tablet = "72:bb:66:12:2a:01", "72:bb:66:12:2a:02", "72:bb:66:12:2a:03"
	// A phone joins North afresh; goes to South and back by fast
	// transitions; to South once more with the four messages, slowly;
	// fails to get back on North; gets back on it; and asks there again as
	// one already on, on the radio it is on: that is no roam.
	attempt(northToken, phone, "phy0-ap0", "5g", 600*time.Second, "online", "internet", "", map[string]any{"alg": "open", "on_ms": 40})
	attempt(southToken, phone, "phy0-ap0", "5g", 500*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "ft", "on_ms": 17})
	attempt(northToken, phone, "phy0-ap0", "5g", 400*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "ft", "on_ms": 21})
	attempt(southToken, phone, "phy0-ap0", "5g", 300*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 280})
	attempt(northToken, phone, "phy0-ap0", "5g", 200*time.Second, "failed", "key", "no answer the AP could use to the first message of the key handshake, sent 4 times", map[string]any{"re": true})
	attempt(northToken, phone, "phy0-ap0", "5g", 100*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 35})
	attempt(northToken, phone, "phy0-ap0", "5g", 50*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 30})
	// A laptop comes back to North, which still held it: a second of SA
	// Queries, and its key from before. Then to South's other radio with a
	// key made from its sign-in.
	attempt(northToken, laptop, "phy1-ap0", "2g", 90*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "open", "pmk": "pmksa", "saq": 5, "on_ms": 1198})
	attempt(southToken, laptop, "phy1-ap0", "2g", 80*time.Second, "online", "dns", "", map[string]any{"re": true, "alg": "open", "pmk": "okc", "on_ms": 30})
	// A tablet asks South as one already on, and goes before it is let on.
	attempt(southToken, tablet, "phy0-ap0", "5g", 70*time.Second, "left", "assoc", "before it was let on: deauthenticated", map[string]any{"re": true})
	// North's own scan radio, testing: no client's roaming.
	attempt(southToken, scan, "phy0-ap0", "5g", 60*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "ft", "on_ms": 15})
	// How it came is held to what an agent writes.
	for what, how := range map[string]map[string]any{"a key from nowhere known": {"pmk": "magic"}, "an authentication no name": {"alg": "Open System"}, "a time past ten minutes": {"on_ms": 700000}} {
		rec := map[string]any{"mac": phone, "bss": "phy0-ap0", "network": "sweet", "ssid": "Sweet Spot", "band": "5g",
			"started": now.UnixMilli(), "took_ms": 1, "outcome": "connected", "stage": "key", "reason": "", "events": []any{}, "how": how}
		if code, _, body := f.apDo("POST", "/v1/ap/connections", northToken, map[string]any{"connections": []any{rec}}, nil); code != 200 || body["refused"] != float64(1) {
			t.Errorf("%s: %d %v", what, code, body)
		}
	}

	code, v := f.do("GET", "/v1/roaming", "griff", nil)
	if code != 200 {
		t.Fatalf("roaming: %d %v", code, v)
	}
	if got, want := fmt.Sprintf("%v roams, %v failed, %v slow, %v left, %v clients, typically %v ms, slow past %v", v["roams"], v["failed"], v["slow"], v["left"], v["clients"], v["typical_ms"], v["slow_over_ms"]),
		"7 roams, 1 failed, 2 slow, 1 left, 3 clients, typically 35 ms, slow past 250"; got != want {
		t.Errorf("in all: %s, want %s", got, want)
	}
	var kinds []string
	for _, k := range v["kinds"].([]any) {
		k := k.(map[string]any)
		kinds = append(kinds, fmt.Sprintf("%v: %v, %v failed, %v slow, %v ms (%v to %v)", k["kind"], k["roams"], k["failed"], k["slow"], k["typical_ms"], k["low_ms"], k["high_ms"]))
	}
	if got, want := strings.Join(kinds, "; "), "ft: 2, 0 failed, 0 slow, 21 ms (17 to 21); okc: 1, 0 failed, 0 slow, 30 ms (30 to 30); pmksa: 1, 0 failed, 1 slow, 1198 ms (1198 to 1198); keys: 3, 1 failed, 1 slow, 280 ms (35 to 280)"; got != want {
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
			if r["sa_queries"] != nil {
				s += fmt.Sprintf(" after %v SA Queries", r["sa_queries"])
			}
			out = append(out, s)
		}
		return out
	}
	// The latest first. The phone's last ask, on the radio it was on, is
	// not among them; nor is the scan radio's.
	if got, want := strings.Join(said(v["latest"]), "\n"), strings.Join([]string{
		"03 to South 5g from somewhere not known, keys, left",
		"02 to South 2g from North 2g, okc, ok in 30 ms",
		"02 to North 2g from somewhere not known, pmksa, slow in 1198 ms after 5 SA Queries",
		"01 to North 5g from South 5g, keys, ok in 35 ms",
		"01 to North 5g from South 5g, keys, failed",
		"01 to South 5g from North 5g, keys, slow in 280 ms",
		"01 to North 5g from South 5g, ft, ok in 21 ms",
		"01 to South 5g from North 5g, ft, ok in 17 ms",
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
	bad := v["bad"].([]any)[2].(map[string]any)
	if !strings.Contains(fmt.Sprint(bad["reason"]), "first message of the key handshake") {
		t.Errorf("the roam that failed says why: %v", bad)
	}
	// Below one AP, its own alone; and one with no role there sees none.
	_, one := f.do("GET", "/v1/roaming?under="+north+"&hours=1", "griff", nil)
	if one["roams"] != float64(4) || one["hours"] != float64(1) {
		t.Errorf("below North: %v roams over %v hours, want 4 over 1", one["roams"], one["hours"])
	}
	if _, none := f.do("GET", "/v1/roaming", "tenant", nil); none["roams"] != float64(0) || len(none["latest"].([]any)) != 0 {
		t.Errorf("one with no role on the APs: %v", none)
	}
	if code, b := f.do("GET", "/v1/roaming?hours=0", "griff", nil); code != 400 {
		t.Errorf("no hours: %d %v", code, b)
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
	} {
		kind, is := roamKind(roamRecord{How: c.how, Events: c.steps}, c.signs)
		if kind != c.kind || is != c.is {
			t.Errorf("%s: %q, a roam %v; want %q, %v", what, kind, is, c.kind, c.is)
		}
	}
}
