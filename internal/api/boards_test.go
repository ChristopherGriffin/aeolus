package api

import (
	"strings"
	"testing"
)

// A kind of AP's own port settings (0092), through the API: checked against
// the schema as any port setting is, and folded into its APs' own, so an
// AP's config says the port's mode, from the folder that set it.
func TestBoardPortsThroughTheAPI(t *testing.T) {
	f := newFixture(t)
	ap, _ := f.enrolled()
	if code, body := f.change("claude", map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": "office"}); code != 200 {
		t.Fatalf("adopt: %d %v", code, body)
	}
	bad := map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "boards.netgear,r7800.ports.lan2.mode", "value": "sideways"}
	if code, body := f.change("griff", bad); code != 400 || !strings.Contains(body["error"].(string), "boards.netgear,r7800.ports.lan2.mode") {
		t.Fatalf("a mode the schema refuses: %d %v", code, body)
	}
	op := map[string]any{"kind": "set", "tree": "locations", "node": "office", "values": map[string]any{
		"boards.netgear,r7800.ports.lan2.mode": "access", "boards.netgear,r7800.ports.lan2.untagged": 30, "ports.lan2.mode": "trunk"}}
	if code, body := f.change("griff", op); code != 200 {
		t.Fatalf("set: %d %v", code, body)
	}
	_, cfg := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	loc := cfg["location"].(map[string]any)
	mode, _ := loc["ports.lan2.mode"].(map[string]any)
	if mode["value"] != "access" || mode["from"] != "office" {
		t.Fatalf("the R7800's lan2 = %v", mode)
	}
	for p := range loc {
		if strings.HasPrefix(p, "boards.") {
			t.Errorf("%s is in the AP's config as it is", p)
		}
	}
	doc := cfg["document"].(map[string]any)
	if _, has := doc["boards"]; has {
		t.Errorf("the document sent to the AP has boards: %v", doc["boards"])
	}
}
