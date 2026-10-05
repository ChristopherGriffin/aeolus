package api

import (
	"fmt"
	"testing"
)

// The key the APs sign their hellos with comes beside the config while
// radio resource management is on, the same at every poll, and not
// otherwise (0073).
func TestRRMKeyComesWithTheConfig(t *testing.T) {
	f := newFixture(t)
	ap, token, _ := f.adopted()
	poll := func() map[string]any {
		t.Helper()
		code, _, body := f.apDo("GET", "/v1/ap/config", token, nil, nil)
		if code != 200 || body["state"] != "ready" {
			t.Fatalf("poll: %d %v", code, body)
		}
		return body
	}
	if body := poll(); body["rrm"] != nil {
		t.Fatalf("off: %v", body["rrm"])
	}
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": ap, "path": "rrm.enabled", "value": true}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	body := poll()
	key, _ := body["rrm"].(map[string]any)["key"].(string)
	if len(key) != 64 {
		t.Fatalf("key = %q", key)
	}
	if again, _ := poll()["rrm"].(map[string]any)["key"].(string); again != key {
		t.Fatal("the key changed between polls")
	}
	if cfg := body["config"].(map[string]any); fmt.Sprint(cfg["rrm"]) != "map[enabled:true]" {
		t.Fatalf("config rrm = %v", cfg["rrm"])
	}
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": ap, "path": "rrm.enabled", "value": false}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	if body := poll(); body["rrm"] != nil {
		t.Fatalf("turned off: %v", body["rrm"])
	}
}
