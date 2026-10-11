package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// A network that signs clients in by 802.1X is tested as its test user
// (0124): refused until it has one, and the user's password reaches the AP
// that makes the test and nobody else.
func TestClientTestSignsIn(t *testing.T) {
	f := newFixture(t)
	change := func(tree, node, path string, v any) {
		t.Helper()
		if code, b := f.change("griff", map[string]any{"kind": "set", "tree": tree, "node": node, "path": path, "value": v}); code != 200 {
			t.Fatalf("%s: %d %v", path, code, b)
		}
	}
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	scanner := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:7a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	change("services", "household", "network.sweet.security", "wpa2-enterprise")
	change("services", "household", "network.sweet.radius.auth_server", "192.0.2.10")
	change("services", "household", "network.sweet.radius.auth_secret", "radius-secret")
	ask := func() (int, map[string]any) {
		return f.do("POST", "/v1/aps/"+scanner[0]+"/actions", "griff", map[string]any{"kind": "client-test", "network": "sweet"})
	}

	// No test user: refused, and so with a name and no password.
	if code, b := ask(); code != 400 || !strings.Contains(b["error"].(string), "has no test user") {
		t.Fatalf("with no test user: %d %v", code, b)
	}
	change("services", "household", "network.sweet.radius.test.user", "aeolus-test")
	if code, b := ask(); code != 400 || !strings.Contains(b["error"].(string), "has no test user") {
		t.Fatalf("with a name and no password: %d %v", code, b)
	}
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "services", "node": "household", "path": "network.sweet.radius.test.method", "value": "tls"}); code == 200 {
		t.Fatalf("a method there is none of was taken: %v", b)
	}
	const secret = "pa55 \"word\""
	change("services", "household", "network.sweet.radius.test.password", secret)

	// Asked: the action says who signs in and how, and not the password,
	// to the person who asked or anyone who lists the AP's actions.
	code, b := ask()
	if code != 200 {
		t.Fatalf("asked: %d %v", code, b)
	}
	raw, _ := json.Marshal(b)
	_, list := f.do("GET", "/v1/aps/"+scanner[0]+"/actions", "griff", nil)
	listed, _ := json.Marshal(list)
	if eap := b["action"].(map[string]any)["params"].(map[string]any)["eap"].(map[string]any); eap["method"] != "peap" || eap["identity"] != "aeolus-test" ||
		strings.Contains(string(raw), "pa55") || strings.Contains(string(listed), "pa55") {
		t.Fatalf("the action = %s; listed %s", raw, listed)
	}
	// Nor is it in the AP's config, where the user's name is.
	_, _, cfg := f.apDo("GET", "/v1/ap/config", scanner[1], nil, nil)
	doc, _ := json.Marshal(cfg)
	test := cfg["config"].(map[string]any)["network"].(map[string]any)["sweet"].(map[string]any)["radius"].(map[string]any)["test"].(map[string]any)
	if test["user"] != "aeolus-test" || test["password"] != nil || strings.Contains(string(doc), "pa55") {
		t.Fatalf("the AP's config has %v", test)
	}

	// The AP that makes the test is handed it.
	_, _, got := f.apDo("GET", "/v1/ap/actions", scanner[1], nil, nil)
	handed := got["actions"].([]any)[0].(map[string]any)
	if eap := handed["params"].(map[string]any)["eap"].(map[string]any); eap["password"] != secret || eap["identity"] != "aeolus-test" || eap["method"] != "peap" {
		t.Fatalf("the AP was handed %v", handed)
	}
	// And what it found may stop at the sign-in.
	d := map[string]any{"v": 1, "ssid": "Sweet Spot", "client": "02:00:00:00:00:fe", "outcome": "failed", "stage": "signin", "reason": "the sign-in was refused",
		"steps": []any{map[string]any{"t": 900, "stage": "signin", "what": "the sign-in was refused", "ok": false}}}
	if code, _, b := f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%d", int64(handed["id"].(float64))), scanner[1], map[string]any{"ok": false, "result": "failed at signin", "detail": d}, nil); code != 200 {
		t.Fatalf("done: %d %v", code, b)
	}
	// The record of it still has no password.
	_, list = f.do("GET", "/v1/aps/"+scanner[0]+"/actions", "griff", nil)
	if listed, _ = json.Marshal(list); strings.Contains(string(listed), "pa55") || !strings.Contains(string(listed), `"stage":"signin"`) {
		t.Fatalf("listed after = %s", listed)
	}

	// By TTLS, where the network says so.
	change("services", "household", "network.sweet.radius.test.method", "ttls")
	if code, b := ask(); code != 200 || b["action"].(map[string]any)["params"].(map[string]any)["eap"].(map[string]any)["method"] != "ttls" {
		t.Fatalf("by TTLS: %d %v", code, b)
	}
}
