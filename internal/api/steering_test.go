package api

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// Steering's history (0139): the askings an AP says in its state reports
// are kept by the manager, each once however often said, and read back as
// what steering came to over a time: for each network and way, and each AP
// of it, how many were asked and what came of them; the clients asked
// most; and the latest askings. Of the APs the caller may view, and not of
// the tests' own clients.
func TestSteering(t *testing.T) {
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
	east, eastToken := enroll("East", "02:00:00:00:00:3c", "annex", []any{serving}, nil)

	now := time.Now()
	asking := func(mac, network, method, asked, outcome string, ago time.Duration, more map[string]any) map[string]any {
		e := map[string]any{"at": now.Add(-ago).Unix(), "mac": mac, "network": network, "method": method, "from": "2g", "asked": asked, "outcome": outcome, "took": 2.5}
		for k, v := range more {
			e[k] = v
		}
		return e
	}
	report := func(token string, uptime int, clients []any, events ...any) {
		t.Helper()
		code, _, cfg := f.apDo("GET", "/v1/ap/config", token, nil, nil)
		if code != 200 {
			t.Fatalf("its config: %d %v", code, cfg)
		}
		body := map[string]any{"version": cfg["version"], "uptime": uptime, "clients": clients, "steer": map[string]any{"running": true, "since": now.Add(-time.Hour).Unix(), "events": events}}
		if code, _, b := f.apDo("POST", "/v1/ap/state", token, body, nil); code != 200 {
			t.Fatalf("its report: %d %v", code, b)
		}
	}
	const a, b, c = "02:00:00:00:00:0a", "02:00:00:00:00:0b", "02:00:00:00:00:0c"
	hour := 3600
	northSaid := []any{
		asking(a, "sweet", "suggest", "btm", "moved", 50*time.Minute, map[string]any{"to": []string{"6g", "5g"}, "answer": "accepted", "code": 0, "signal": -48}),
		asking(a, "sweet", "suggest", "none", "back", 49*time.Minute, nil),
		asking(b, "sweet", "suggest", "btm", "refused", 40*time.Minute, map[string]any{"answer": "refused", "code": 1}),
		asking(b, "sweet", "hold", "held", "joined", 30*time.Minute, nil),
		asking(c, "guest", "sticky", "kick", "left", 20*time.Minute, map[string]any{"from": "5g", "signal": -80}),
		// The scan radio's own, joining for a test: of no client.
		asking(scan, "sweet", "suggest", "btm", "moved", 10*time.Minute, nil),
		// Said to be of before the AP started, a day up: its clock was not
		// yet set, and the asking is of this run. Taken to be of now.
		asking(c, "sweet", "suggest", "btm", "stayed", 40*24*time.Hour, nil),
	}
	client := func(mac, host string) map[string]any {
		return map[string]any{"mac": mac, "network": "sweet", "ssid": "Sweet Spot", "band": "5g", "host": host}
	}
	report(northToken, 24*hour, []any{client(a, "phone")}, northSaid...)
	// Said again in the next report, with one more: only that one is new.
	report(northToken, 24*hour+60, nil, append(northSaid, asking(b, "sweet", "suggest", "btm", "moved", time.Minute, nil))...)
	report(eastToken, 60*24*hour, nil,
		asking(a, "sweet", "suggest", "btm", "moved", 5*time.Minute, nil),
		// First heard of more than a week after: one already kept, or let
		// go since. Not kept anew. The AP has been up long enough to have
		// made it.
		asking(a, "sweet", "suggest", "btm", "moved", 8*24*time.Hour, nil),
		// Two days old, and its AP up since before: kept with its time.
		asking(b, "sweet", "suggest", "btm", "refused", 48*time.Hour, nil))

	counts := func(v any) string {
		var out []string
		for method, c := range v.(map[string]any) {
			for what, n := range c.(map[string]any) {
				out = append(out, fmt.Sprintf("%s %s %v", method, what, n))
			}
		}
		slices.Sort(out)
		return strings.Join(out, ", ")
	}
	get := func(as, query string) map[string]any {
		t.Helper()
		code, v := f.do("GET", "/v1/steering"+query, as, nil)
		if code != 200 {
			t.Fatalf("steering%s as %s: %d %v", query, as, code, v)
		}
		return v
	}
	v := get("griff", "")
	if v["hours"] != float64(24) {
		t.Errorf("hours: %v", v["hours"])
	}
	nets := v["networks"].([]any)
	if len(nets) != 2 {
		t.Fatalf("networks: %v", nets)
	}
	guest, sweet := nets[0].(map[string]any), nets[1].(map[string]any)
	if guest["network"] != "guest" || counts(guest["counts"]) != "sticky asked 1, sticky left 1" {
		t.Errorf("guest: %v", guest)
	}
	if got, want := counts(sweet["counts"]), "hold asked 1, hold joined 1, suggest asked 5, suggest back 1, suggest moved 3, suggest refused 1, suggest stayed 1"; sweet["network"] != "sweet" || got != want {
		t.Errorf("sweet:\n got %s\nwant %s", got, want)
	}
	aps := sweet["aps"].([]any)
	if len(aps) != 2 {
		t.Fatalf("sweet's APs: %v", aps)
	}
	if e := aps[0].(map[string]any); e["ap"] != east || e["name"] != "East" || counts(e["counts"]) != "suggest asked 1, suggest moved 1" {
		t.Errorf("East: %v", e)
	}
	if n := aps[1].(map[string]any); n["ap"] != north || n["name"] != "North" || counts(n["counts"]) != "hold asked 1, hold joined 1, suggest asked 4, suggest back 1, suggest moved 2, suggest refused 1, suggest stayed 1" {
		t.Errorf("North: %v", n)
	}
	var who []string
	for _, cl := range v["clients"].([]any) {
		cl := cl.(map[string]any)
		who = append(who, fmt.Sprintf("%s %v asked %v moved %v refused %v back %v", cl["mac"].(string)[15:], cl["host"], cl["asked"], cl["moved"], cl["refused"], cl["back"]))
	}
	if got, want := strings.Join(who, "; "), "0b <nil> asked 3 moved 1 refused 1 back 0; 0c <nil> asked 2 moved 0 refused 0 back 0; 0a phone asked 2 moved 2 refused 0 back 1"; got != want {
		t.Errorf("clients:\n got %s\nwant %s", got, want)
	}
	latest := v["latest"].([]any)
	var ends []string
	for _, e := range latest {
		e := e.(map[string]any)
		ends = append(ends, fmt.Sprintf("%s %s %s %s", e["ap_name"], e["mac"].(string)[15:], e["method"], e["outcome"]))
	}
	// The one of a clock not set is of when it came in, the first report:
	// before the one made a minute ago, which the second brought.
	if got, want := strings.Join(ends, ", "), "North 0c suggest stayed, North 0b suggest moved, East 0a suggest moved, North 0c sticky left, North 0b hold joined, North 0b suggest refused, North 0a suggest back, North 0a suggest moved"; got != want {
		t.Errorf("latest:\n got %s\nwant %s", got, want)
	}
	last := latest[len(latest)-1].(map[string]any)
	if last["ap"] != north || last["host"] != "phone" || last["network"] != "sweet" || last["from"] != "2g" || fmt.Sprint(last["to"]) != "[6g 5g]" || last["asked"] != "btm" ||
		last["answer"] != "accepted" || last["code"] != float64(0) || last["signal"] != float64(-48) || last["took"] != 2.5 {
		t.Errorf("an asking, as said: %v", last)
	}
	if at, err := time.Parse(time.RFC3339Nano, last["at"].(string)); err != nil || at.Unix() != now.Add(-50*time.Minute).Unix() {
		t.Errorf("when it was: %v %v", last["at"], err)
	}

	// Over a longer time, the one of two days ago is there too; under a
	// folder, only its APs'.
	if got, want := counts(get("griff", "?hours=72")["networks"].([]any)[1].(map[string]any)["counts"]), "hold asked 1, hold joined 1, suggest asked 6, suggest back 1, suggest moved 3, suggest refused 2, suggest stayed 1"; got != want {
		t.Errorf("sweet over three days:\n got %s\nwant %s", got, want)
	}
	under := get("griff", "?under=annex")
	if nets := under["networks"].([]any); len(nets) != 1 || counts(nets[0].(map[string]any)["counts"]) != "suggest asked 1, suggest moved 1" || len(under["latest"].([]any)) != 1 || len(under["clients"].([]any)) != 1 {
		t.Errorf("under the annex: %v", under)
	}
	// One who may view the annex alone sees its askings alone, and no name
	// a client gave elsewhere.
	mine := get("tenant", "")
	if nets := mine["networks"].([]any); len(nets) != 1 || len(nets[0].(map[string]any)["aps"].([]any)) != 1 || len(mine["latest"].([]any)) != 1 {
		t.Errorf("the tenant's: %v", mine)
	}
	if cl := mine["clients"].([]any)[0].(map[string]any); cl["mac"] != a || cl["host"] != nil || cl["asked"] != float64(1) {
		t.Errorf("the tenant's client: %v", cl)
	}
	if nothing := get("tenant", "?under=office"); len(nothing["networks"].([]any)) != 0 || len(nothing["latest"].([]any)) != 0 || len(nothing["clients"].([]any)) != 0 {
		t.Errorf("the tenant, of the office: %v", nothing)
	}
	// North starts again, its clock once more not yet set, and says an
	// asking of the same time as one of its run before: another asking,
	// kept as well, and once however often it says it.
	again := asking(c, "sweet", "suggest", "btm", "stayed", 40*24*time.Hour, nil)
	report(northToken, 300, nil, again)
	report(northToken, 330, nil, again)
	if got, want := counts(get("griff", "?under=office")["networks"].([]any)[1].(map[string]any)["counts"]), "hold asked 1, hold joined 1, suggest asked 5, suggest back 1, suggest moved 2, suggest refused 1, suggest stayed 2"; got != want {
		t.Errorf("sweet at the office, North started again:\n got %s\nwant %s", got, want)
	}
	for _, q := range []string{"?hours=0", "?hours=721", "?hours=many"} {
		if code, b := f.do("GET", "/v1/steering"+q, "griff", nil); code != 400 {
			t.Errorf("%s: %d %v", q, code, b)
		}
	}
	if code, b := f.do("GET", "/v1/steering", "", nil); code != 401 {
		t.Errorf("no token: %d %v", code, b)
	}
}
