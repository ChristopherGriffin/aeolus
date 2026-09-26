package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// apDo sends a request with an AP token ("" for none) and extra headers, and
// returns the status, the response headers and the decoded body, if any.
func (f *fixture) apDo(method, path, token string, body any, header map[string]string) (int, http.Header, map[string]any) {
	f.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		must(f.t, err)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, f.url+path, rd)
	must(f.t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	must(f.t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	must(f.t, err)
	var out map[string]any
	if len(raw) > 0 {
		must(f.t, json.Unmarshal(raw, &out))
	}
	return resp.StatusCode, resp.Header, out
}

var pumphouse = map[string]any{
	"mac":      "BC:FF:4D:85:71:7A",
	"macs":     []string{"bc:ff:4d:85:71:7a", "bc:ff:4d:85:71:7b"},
	"hostname": "PumphouseAP",
	"model":    "Netgear Nighthawk X4S R7800",
	"board":    "netgear,r7800",
	"openwrt":  "25.12.5",
	"radios":   []any{map[string]any{"band": "2g"}, map[string]any{"band": "5g"}},
}

// enrolled adds the built-in folders and enrolls the pumphouse AP, returning
// its ID and token.
func (f *fixture) enrolled() (string, string) {
	f.t.Helper()
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		f.t.Fatalf("add-builtins: %d %v", code, b)
	}
	code, _, body := f.apDo("POST", "/v1/enroll", "", pumphouse, nil)
	if code != 201 {
		f.t.Fatalf("enroll: %d %v", code, body)
	}
	return body["ap"].(string), body["token"].(string)
}

func TestEnrollment(t *testing.T) {
	f := newFixture(t)
	ap, token := f.enrolled()
	if ap != "ap-bcff4d85717a" || !strings.HasPrefix(token, "aeolusap1.") {
		t.Fatalf("enrolled %q with token %q", ap, token)
	}

	// The change is the manager's, with where the AP called from.
	_, log := f.do("GET", "/v1/changes?after=0&limit=1000", "griff", nil)
	entries := log["changes"].([]any)
	last := entries[len(entries)-1].(map[string]any)
	op := last["op"].(map[string]any)
	if last["actor"] != "aeolus" || last["reason"] != "enrolled from 127.0.0.1" || op["kind"] != "enroll" || op["token_hash"] != nil {
		t.Fatalf("enroll entry = %v", last)
	}

	// The person deciding sees what it sent.
	_, page := f.do("GET", "/v1/trees/locations/nodes/"+ap, "griff", nil)
	facts := page["facts"].(map[string]any)
	if page["node"].(map[string]any)["parent"] != "landing-zone" || facts["mac"] != "bc:ff:4d:85:71:7a" || facts["source"] != "127.0.0.1" || facts["model"] != "Netgear Nighthawk X4S R7800" {
		t.Fatalf("AP page = %v", page)
	}

	// Enrolling in the name of a known AP is refused.
	if code, _, body := f.apDo("POST", "/v1/enroll", "", pumphouse, nil); code != 409 || !strings.Contains(body["error"].(string), "already enrolled") {
		t.Fatalf("second enrollment: %d %v", code, body)
	}
}

func TestEnrollmentRefusesBadFacts(t *testing.T) {
	f := newFixture(t)
	f.enrolled()
	with := func(k string, v any) map[string]any {
		out := map[string]any{"mac": "02:00:00:00:00:01"}
		out[k] = v
		return out
	}
	for name, body := range map[string]any{
		"no mac":        map[string]any{"hostname": "x"},
		"not a mac":     with("mac", "hello"),
		"multicast mac": with("mac", "01:00:5e:00:00:01"),
		"zero mac":      with("mac", "00:00:00:00:00:00"),
		"bad macs":      with("macs", []string{"nope"}),
		"long hostname": with("hostname", strings.Repeat("a", 64)),
		"control char":  with("model", "R7800\n"),
		"radios object": with("radios", map[string]any{"band": "5g"}),
		"unknown field": with("serial", "123"),
		"too big":       with("board", strings.Repeat("a", 70<<10)),
	} {
		if code, _, b := f.apDo("POST", "/v1/enroll", "", body, nil); code != 400 {
			t.Errorf("%s: %d %v", name, code, b)
		}
	}
}

func TestAccountsCannotEnroll(t *testing.T) {
	f := newFixture(t)
	f.enrolled()
	code, body := f.change("griff", map[string]any{"kind": "enroll", "node": "ap-x", "token_id": "x"})
	if code != 403 {
		t.Fatalf("enroll through /v1/changes: %d %v", code, body)
	}
}

func TestTokensWorkOnlyOnTheirOwnRoutes(t *testing.T) {
	f := newFixture(t)
	_, token := f.enrolled()
	f.tokens["ap"] = token
	if code, _ := f.do("GET", "/v1/whoami", "ap", nil); code != 401 {
		t.Fatalf("AP token on an account route: %d", code)
	}
	if code, _, _ := f.apDo("GET", "/v1/ap/config", f.tokens["griff"], nil, nil); code != 401 {
		t.Fatalf("account token on an AP route: %d", code)
	}
	if code, _, _ := f.apDo("GET", "/v1/ap/config", "", nil, nil); code != 401 {
		t.Fatalf("no token: %d", code)
	}
}

func TestPoll(t *testing.T) {
	f := newFixture(t)
	ap, token := f.enrolled()
	poll := func(etag string) (int, http.Header, map[string]any) {
		t.Helper()
		h := map[string]string{}
		if etag != "" {
			h["If-None-Match"] = etag
		}
		return f.apDo("GET", "/v1/ap/config", token, nil, h)
	}

	// In Landing Zone it gets nothing, and no version to hold on to.
	code, h, body := poll("")
	if code != 200 || body["state"] != "unassigned" || body["config"] != nil || h.Get("ETag") != "" {
		t.Fatalf("Landing Zone poll: %d %v %v", code, h, body)
	}

	// Adopted, it gets its config, secrets opened.
	if code, b := f.change("griff", map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": "office"}); code != 200 {
		t.Fatalf("adopt: %d %v", code, b)
	}
	code, h, body = poll("")
	if code != 200 || body["state"] != "ready" || h.Get("Cache-Control") != "no-store" {
		t.Fatalf("adopted poll: %d %v %v", code, h, body)
	}
	etag := h.Get("ETag")
	if want := `"` + jsonText(body["version"]) + `"`; etag != want {
		t.Fatalf("ETag %s, want %s", etag, want)
	}
	sweet := body["config"].(map[string]any)["network"].(map[string]any)["sweet"].(map[string]any)
	if sweet["passphrase"] != passphrase || sweet["ssid"] != "Sweet Spot" {
		t.Fatalf("sweet = %v", sweet)
	}

	// Nothing changed: 304.
	if code, h, _ := poll(etag); code != 304 || h.Get("ETag") != etag {
		t.Fatalf("unchanged poll: %d %v", code, h)
	}
	// A change elsewhere: still 304.
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "services", "node": "guest", "path": "network.guest.ssid", "value": "Guests"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	if code, _, _ := poll(etag); code != 304 {
		t.Fatalf("poll after an unrelated change: %d", code)
	}

	// A change that breaks its config: held, with the problems.
	if code, b := f.change("griff", map[string]any{"kind": "unset", "tree": "services", "node": "household", "path": "network.sweet.passphrase"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	code, h, body = poll(etag)
	if code != 200 || body["state"] != "held" || body["config"] != nil || h.Get("ETag") != "" || !strings.Contains(jsonText(body["problems"]), "passphrase") {
		t.Fatalf("held poll: %d %v %v", code, h, body)
	}

	// Fixed: a new version.
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "services", "node": "household", "path": "network.sweet.passphrase", "value": "another-passphrase-9"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	code, h, body = poll(etag)
	if code != 200 || body["state"] != "ready" || h.Get("ETag") == etag {
		t.Fatalf("poll after the fix: %d %v %v", code, h, body)
	}

	// Removed: its token stops working, and it may enroll again.
	if code, b := f.change("griff", map[string]any{"kind": "remove-ap", "node": ap}); code != 200 {
		t.Fatalf("remove-ap: %d %v", code, b)
	}
	if code, _, _ := poll(""); code != 401 {
		t.Fatalf("poll after removal: %d", code)
	}
	if code, _, b := f.apDo("POST", "/v1/enroll", "", pumphouse, nil); code != 201 {
		t.Fatalf("enrolling after removal: %d %v", code, b)
	}
}

func jsonText(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestMatches(t *testing.T) {
	for header, want := range map[string]bool{`"12"`: true, `W/"12"`: true, `"11", "12"`: true, `"1"`: false, ``: false, `*`: false} {
		if got := matches(header, `"12"`); got != want {
			t.Errorf("matches(%q) = %v", header, got)
		}
	}
}
