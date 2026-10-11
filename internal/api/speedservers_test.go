package api

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/schema"
)

// The remote speed server (0134) is a public one by its name, or one's own
// by its URL. The names the schema offers are those the manager knows, and
// each is a server an agent will take: a plain host and plain paths.
func TestTheRemoteSpeedServers(t *testing.T) {
	sch, err := schema.V1()
	if err != nil {
		t.Fatal(err)
	}
	f := sch.Describe().Fields["system.speed_server_remote"]
	choices, _ := f["x-aeolus-choices"].([]any)
	var offered []string
	for _, c := range choices {
		pair, _ := c.([]any)
		if len(pair) != 2 || pair[1] == "" {
			t.Fatalf("a choice is a name and how it is said: %v", c)
		}
		offered = append(offered, pair[0].(string))
	}
	var known []string
	for id := range publicSpeed {
		known = append(known, id)
	}
	slices.Sort(offered)
	slices.Sort(known)
	if !slices.Equal(offered, known) {
		t.Errorf("the schema offers %v, and the manager knows %v", offered, known)
	}
	pattern := regexp.MustCompile(f["pattern"].(string))
	// What the agent takes of a host and of a path (clienttest.uc).
	host, path := regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}$`), regexp.MustCompile(`^/[A-Za-z0-9._~/?&=%+-]{0,200}$`)
	for _, id := range known {
		if !pattern.MatchString(id) {
			t.Errorf("%s is not a value the setting takes", id)
		}
		s := remoteSpeed(id)
		if s == nil || s.Name == "" || !host.MatchString(s.Host) || s.Port != 80 || !path.MatchString(s.Down) || !path.MatchString(s.Up) || s.Body != publicBody {
			t.Errorf("%s is %+v", id, s)
		}
	}
	// One's own is an OpenSpeedTest server, by its URL.
	for v, want := range map[string]string{
		"http://speed.example.net":       "speed.example.net speed.example.net:80 /downloading /upload 0",
		"http://speed.example.net:3000/": "speed.example.net:3000 speed.example.net:3000 /downloading /upload 0",
		"http://203.0.113.9:8080":        "203.0.113.9:8080 203.0.113.9:8080 /downloading /upload 0",
	} {
		s := remoteSpeed(v)
		if s == nil {
			t.Errorf("%s is no server", v)
			continue
		}
		if got := fmt.Sprintf("%s %s:%d %s %s %d", s.Name, s.Host, s.Port, s.Down, s.Up, s.Body); got != want {
			t.Errorf("%s is %s, want %s", v, got, want)
		}
		if !pattern.MatchString(v) {
			t.Errorf("%s is not a value the setting takes", v)
		}
	}
	// What is neither is none: as a name a later manager knows.
	for _, v := range []string{"", "ookla", "librespeed-mars", "https://speed.example.net", "http://speed.example.net/path", "http://speed.example.net:0", "http://speed.example.net:65536"} {
		if s := remoteSpeed(v); s != nil {
			t.Errorf("%q is %+v", v, s)
		}
	}
}

// A test asked by hand measures against both speed servers, where they are
// set (0134); one on a schedule against the local one where the folder says
// so, and against the remote one where it says that too. What the AP found
// of the remote one is kept beside the local, and is a point of its way.
func TestATestMeasuresBothSpeeds(t *testing.T) {
	f := newFixture(t)
	change := func(op map[string]any) (int, map[string]any) { return f.change("griff", op) }
	set := func(path string, v any) (int, map[string]any) {
		return change(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": path, "value": v})
	}
	must := func(path string, v any) {
		t.Helper()
		if code, b := set(path, v); code != 200 {
			t.Fatalf("%s = %v: %d %v", path, v, code, b)
		}
	}
	if code, b := change(map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	scanner := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:6a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	say := func() {
		t.Helper()
		_, _, cfg := f.apDo("GET", "/v1/ap/config", scanner[1], nil, nil)
		if code, _, b := f.apDo("POST", "/v1/ap/state", scanner[1], map[string]any{"version": cfg["version"], "uptime": 60, "bssids": []string{"02:00:00:00:00:6a"}}, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
	}

	// The setting takes a public server's name, or a URL of one's own, and
	// nothing else.
	for v, good := range map[string]bool{"cloudflare": true, "librespeed-atlanta": true, "tele2": true, "http://speed.example.net:3000": true,
		"ookla": false, "librespeed-mars": false, "https://speed.example.net": false, "Cloudflare": false, "http://speed.example.net:99999": false} {
		if code, b := set("system.speed_server_remote", v); (code == 200) != good {
			t.Errorf("a remote speed server of %q: %d %v", v, code, b)
		}
	}
	must("system.speed_server", "http://192.168.20.106")
	must("system.speed_server_remote", "cloudflare")
	say()

	// Asked by hand: both.
	type params struct {
		Speed       string       `json:"speed"`
		SpeedRemote *speedTarget `json:"speed_remote"`
	}
	said := func(p params) string {
		if p.SpeedRemote == nil {
			return p.Speed + ", no remote"
		}
		r := p.SpeedRemote
		return fmt.Sprintf("%s, %s at %s:%d%s and %s by %d", p.Speed, r.Name, r.Host, r.Port, r.Down, r.Up, r.Body)
	}
	code, body := f.do("POST", "/v1/aps/"+scanner[0]+"/actions", "griff", map[string]any{"kind": "client-test", "network": "sweet", "on": scanner[0], "band": "5g"})
	if code != 200 {
		t.Fatalf("asked: %d %v", code, body)
	}
	a := body["action"].(map[string]any)
	var p params
	raw, _ := json.Marshal(a["params"])
	json.Unmarshal(raw, &p)
	if got, want := said(p), "http://192.168.20.106, Cloudflare at speed.cloudflare.com:80/__down?bytes=50000000 and /__up by 4194304"; got != want {
		t.Errorf("asked by hand: %s, want %s", got, want)
	}

	// What the AP found: the remote speed beside the local, its steps
	// naming their point; and one a later agent names, which is left out.
	yes := true
	id := strconv.FormatInt(int64(a["id"].(float64)), 10)
	f.apDo("GET", "/v1/ap/actions", scanner[1], nil, nil)
	found := map[string]any{
		"v": 1, "ssid": "Sweet Spot", "band": "5g", "bssid": "02:00:00:00:00:6a", "channel": 36, "client": "02:00:00:00:00:f0",
		"outcome": "passed", "stage": "done", "reason": "", "joined_ms": 636, "took_ms": 27000,
		"steps": []any{
			map[string]any{"t": 146, "stage": "find", "what": "looks for the network"},
			map[string]any{"t": 636, "stage": "key", "what": "let on to the network", "ok": yes},
			map[string]any{"t": 925, "stage": "dhcp", "what": "given 192.168.20.108 by 192.168.20.254", "ok": yes, "ms": 284},
			map[string]any{"t": 9107, "stage": "speed", "what": "down 412 Mbit/s", "ok": yes},
			map[string]any{"t": 15193, "stage": "speed", "what": "up 380 Mbit/s", "ok": yes},
			map[string]any{"t": 15240, "stage": "speed", "what": "192.168.20.254 answers speed.cloudflare.com: 172.66.0.218", "ok": yes, "ms": 41, "k": "remote"},
			map[string]any{"t": 21300, "stage": "speed", "what": "down 480 Mbit/s from Cloudflare", "ok": yes, "k": "remote"},
			map[string]any{"t": 27400, "stage": "speed", "what": "up 39 Mbit/s to Cloudflare", "ok": yes, "k": "remote"},
			map[string]any{"t": 27401, "stage": "speed", "what": "something a later agent measures", "ok": yes, "k": "farther"},
		},
		"speed":        map[string]any{"server": "http://192.168.20.106", "down": 412.0, "up": 380.0},
		"speed_remote": map[string]any{"server": "Cloudflare", "down": 480.0, "up": 39.0},
	}
	if code, _, b := f.apDo("POST", "/v1/ap/actions/"+id, scanner[1], map[string]any{"ok": true, "result": "passed", "detail": found}, nil); code != 200 {
		t.Fatalf("done: %d %v", code, b)
	}
	_, body = f.do("GET", "/v1/aps/"+scanner[0]+"/actions", "griff", nil)
	d := body["actions"].([]any)[0].(map[string]any)["detail"].(map[string]any)
	if r, _ := d["speed_remote"].(map[string]any); r == nil || r["server"] != "Cloudflare" || r["down"] != 480.0 || r["up"] != 39.0 {
		t.Errorf("the remote speed kept = %v", d["speed_remote"])
	}
	var named []string
	for _, s := range d["steps"].([]any) {
		k, _ := s.(map[string]any)["k"].(string)
		named = append(named, k)
	}
	if got, want := strings.Join(named, ","), ",,,,,remote,remote,remote,"; got != want {
		t.Errorf("the steps' points = %q, want %q", got, want)
	}
	var way []string
	for _, w := range d["way"].([]any) {
		w := w.(map[string]any)
		way = append(way, fmt.Sprintf("%v %v", w["k"], w["state"]))
	}
	if got := strings.Join(way, ", "); !strings.HasSuffix(got, "speed ok, remote ok") {
		t.Errorf("its way = %s", got)
	}
	// A remote speed that is no speed is refused, as the local is.
	bad := map[string]any{"v": 1, "outcome": "passed", "stage": "done", "reason": "", "steps": []any{}, "speed_remote": map[string]any{"server": "Cloudflare", "down": -1.0}}
	code, body = f.do("POST", "/v1/aps/"+scanner[0]+"/actions", "griff", map[string]any{"kind": "client-test", "network": "sweet", "on": scanner[0], "band": "5g"})
	if code != 200 {
		t.Fatalf("asked again: %d %v", code, body)
	}
	next := strconv.FormatInt(int64(body["action"].(map[string]any)["id"].(float64)), 10)
	f.apDo("GET", "/v1/ap/actions", scanner[1], nil, nil)
	if code, _, b := f.apDo("POST", "/v1/ap/actions/"+next, scanner[1], map[string]any{"ok": true, "result": "passed", "detail": bad}, nil); code != 400 {
		t.Errorf("a remote speed below nothing: %d %v", code, b)
	}
	// It could not be measured: the test passed all the same, and says why.
	bad["speed_remote"] = map[string]any{"server": "Cloudflare", "error": "its name was not found"}
	if code, _, b := f.apDo("POST", "/v1/ap/actions/"+next, scanner[1], map[string]any{"ok": true, "result": "passed", "detail": bad}, nil); code != 200 {
		t.Errorf("a remote speed not measured: %d %v", code, b)
	}

	// On a schedule: no speed unless the folder says; then the local one;
	// and the remote one only where it says that too.
	asked := func() string {
		t.Helper()
		say()
		got := f.api.assure(f.now())
		if len(got) != 1 {
			t.Fatalf("asked %v", got)
		}
		var p params
		json.Unmarshal(got[0].Params, &p)
		// Done, so the next can be asked.
		f.apDo("GET", "/v1/ap/actions", scanner[1], nil, nil)
		ok := map[string]any{"v": 1, "ssid": "Sweet Spot", "band": "5g", "bssid": "02:00:00:00:00:6a", "outcome": "passed", "stage": "done", "reason": "", "steps": []any{}}
		if code, _, b := f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%d", got[0].ID), scanner[1], map[string]any{"ok": true, "result": "passed", "detail": ok}, nil); code != 200 {
			t.Fatalf("done: %d %v", code, b)
		}
		*f.skew += assureRetry + 16*60*1e9
		if p.Speed == "" && p.SpeedRemote == nil {
			return "no speed"
		}
		return said(p)
	}
	must("assurance.tests.every", 15)
	if got := asked(); got != "no speed" {
		t.Errorf("on a schedule, with no speed asked for: %s", got)
	}
	must("assurance.tests.speed", true)
	if got, want := asked(), "http://192.168.20.106, no remote"; got != want {
		t.Errorf("on a schedule, with its local speed: %s, want %s", got, want)
	}
	must("assurance.tests.speed_remote", true)
	if got := asked(); !strings.HasPrefix(got, "http://192.168.20.106, Cloudflare at speed.cloudflare.com:80") {
		t.Errorf("on a schedule, with both speeds: %s", got)
	}
	must("assurance.tests.speed", false)
	if got := asked(); !strings.HasPrefix(got, ", Cloudflare at speed.cloudflare.com:80") {
		t.Errorf("on a schedule, with its remote speed alone: %s", got)
	}
	// The tab says which speeds an AP's tests measure.
	_, view := f.do("GET", "/v1/assurance", "griff", nil)
	tester := view["testers"].([]any)[0].(map[string]any)
	if tester["speed"] != false || tester["speed_remote"] != true {
		t.Errorf("the tester = %v", tester)
	}
	// And a thing's tests in brief have the remote speed measured.
	var remote []string
	for _, c := range view["checks"].([]any) {
		for _, r := range c.(map[string]any)["runs"].([]any) {
			if v, ok := r.(map[string]any)["remote_down"]; ok {
				remote = append(remote, fmt.Sprint(v, "/", r.(map[string]any)["remote_up"]))
			}
		}
	}
	if got, want := strings.Join(remote, " "), "480/39"; got != want {
		t.Errorf("the remote speeds in brief = %q, want %q", got, want)
	}
}
