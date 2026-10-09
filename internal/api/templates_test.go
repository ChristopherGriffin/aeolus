package api

import (
	"strings"
	"testing"
)

// AP templates (0085), through the API:
//   - adopting the first AP of a kind makes its template, empty, at the Org,
//     picked there, in the manager's name; an AP in Landing Zone makes none;
//   - a template's values reach its APs where nothing set closer to the AP
//     replaces them, and the AP's config says which it follows and which not;
//   - a folder's own template is offered there and below, and picked there;
//   - a pick of a template not offered, or in use, or for another board, is
//     refused, and so is a template field a template may not set.
func TestTemplatesThroughTheAPI(t *testing.T) {
	f := newFixture(t)
	ap, _ := f.enrolled()
	library := func(as, at string) []any {
		t.Helper()
		path := "/v1/library"
		if at != "" {
			path += "?at=" + at
		}
		code, body := f.do("GET", path, as, nil)
		if code != 200 {
			t.Fatalf("library at %q: %d %v", at, code, body)
		}
		return body["templates"].([]any)
	}
	if got := library("griff", ""); len(got) != 0 {
		t.Fatalf("an AP in Landing Zone made a template: %v", got)
	}

	// Adopting it makes its kind's template.
	if code, body := f.change("claude", map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": "office"}); code != 200 {
		t.Fatalf("adopt: %d %v", code, body)
	}
	got := library("griff", "")
	if len(got) != 1 {
		t.Fatalf("templates after adopting: %v", got)
	}
	tm := got[0].(map[string]any)
	if tm["id"] != "netgear-r7800" || tm["name"] != "Netgear Nighthawk X4S R7800" || tm["at"] != "symtus" ||
		len(tm["values"].(map[string]any)) != 0 || len(tm["aps"].([]any)) != 1 {
		t.Fatalf("the new kind's template = %v", tm)
	}
	_, log := f.do("GET", "/v1/changes?after=0&limit=1000", "griff", nil)
	entries := log["changes"].([]any)
	last := entries[len(entries)-1].(map[string]any)
	if op := last["op"].(map[string]any); last["actor"] != "aeolus" || op["kind"] != "add-template" || op["default"] != true {
		t.Fatalf("made by %v", last)
	}
	_, node := f.do("GET", "/v1/trees/locations/nodes/symtus", "griff", nil)
	if pick := node["fields"].(map[string]any)["templates.netgear,r7800"].(map[string]any); pick["value"] != "netgear-r7800" {
		t.Fatalf("picked at the Org: %v", pick)
	}
	// A second AP of the kind makes no other.
	if code, body := f.change("griff", map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": "house"}); code != 200 {
		t.Fatalf("move: %d %v", code, body)
	}
	if got := library("griff", ""); len(got) != 1 {
		t.Fatalf("templates after a move: %v", got)
	}
	if code, body := f.change("griff", map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": "office"}); code != 200 {
		t.Fatalf("move back: %d %v", code, body)
	}

	// Its values: radio.2g.channel reaches the AP; radio.5g.width beats the
	// Org's own 80, where it is picked, but not House's 40, below.
	set := map[string]any{"kind": "set-template", "template": "netgear-r7800", "values": map[string]any{"radio.2g.channel": 11, "radio.5g.width": 20}}
	code, body := f.change("claude", set)
	if code != 200 {
		t.Fatalf("set-template: %d %v", code, body)
	}
	if rv := body["reversioned"].([]any); len(rv) != 1 || rv[0] != ap {
		t.Fatalf("reversioned %v", rv)
	}
	_, cfg := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	loc := cfg["location"].(map[string]any)
	if ch := loc["radio.2g.channel"].(map[string]any); ch["value"] != float64(11) || ch["origin"] != "template" || ch["from"] != "symtus" {
		t.Fatalf("radio.2g.channel = %v", ch)
	}
	if w := loc["radio.5g.width"].(map[string]any); w["value"] != float64(40) || w["from"] != "house" {
		t.Fatalf("radio.5g.width = %v", w)
	}
	if _, sent := loc["templates.netgear,r7800"]; sent {
		t.Fatalf("the pick is in the AP's config: %v", loc)
	}
	use := cfg["template"].(map[string]any)
	replaced := use["replaced"].([]any)
	if use["id"] != "netgear-r7800" || use["board"] != "netgear,r7800" || use["follows"] != false || len(replaced) != 1 ||
		replaced[0].(map[string]any)["node"] != "house" || replaced[0].(map[string]any)["path"] != "radio.5g.width" {
		t.Fatalf("template = %v", use)
	}
	doc := cfg["document"].(map[string]any)
	if _, sent := doc["templates"]; sent {
		t.Fatalf("templates in the document: %v", doc)
	}
	_, aps := f.do("GET", "/v1/aps", "griff", nil)
	for _, a := range aps["aps"].([]any) {
		if a := a.(map[string]any); a["id"] == ap && a["template"].(map[string]any)["follows"] != false {
			t.Fatalf("fleet view: %v", a)
		}
	}

	// House's own template, offered at House and below, picked there.
	add := map[string]any{"kind": "add-template", "template": "r7800-quiet", "name": "R7800, quiet", "parent": "house", "boards": []string{"netgear,r7800"}}
	if code, body := f.change("office", add); code != 403 {
		t.Fatalf("no Locations role: %d %v", code, body)
	}
	if code, body := f.change("claude", add); code != 200 {
		t.Fatalf("add-template: %d %v", code, body)
	}
	if code, body := f.change("claude", map[string]any{"kind": "set-template", "template": "r7800-quiet", "path": "radio.5g.width", "value": 20}); code != 200 {
		t.Fatalf("set-template: %d %v", code, body)
	}
	pick := func(node, id string) (int, map[string]any) {
		return f.change("claude", map[string]any{"kind": "set", "tree": "locations", "node": node, "path": "templates.netgear,r7800", "value": id})
	}
	if code, body := pick("symtus", "r7800-quiet"); code != 400 || !strings.Contains(body["error"].(string), "not this node or above it") {
		t.Fatalf("a template made below: %d %v", code, body)
	}
	if code, body := pick("house", "r7800-quiet"); code != 200 {
		t.Fatalf("pick at House: %d %v", code, body)
	}
	if ids := []string{library("claude", "office")[0].(map[string]any)["id"].(string), library("claude", "office")[1].(map[string]any)["id"].(string)}; ids[0] != "r7800-quiet" || ids[1] != "netgear-r7800" {
		t.Fatalf("offered at Office, nearest first: %v", ids)
	}
	if got := library("claude", "symtus"); len(got) != 1 {
		t.Fatalf("offered at the Org: %v", got)
	}
	_, cfg = f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	loc = cfg["location"].(map[string]any)
	if w := loc["radio.5g.width"].(map[string]any); w["value"] != float64(20) || w["origin"] != "template" || w["from"] != "house" {
		t.Fatalf("House's template beats House's own: %v", w)
	}
	if use := cfg["template"].(map[string]any); use["id"] != "r7800-quiet" || use["follows"] != true {
		t.Fatalf("template = %v", use)
	}
	if _, set := loc["radio.2g.channel"]; set {
		t.Fatalf("one template at a time: %v", loc["radio.2g.channel"])
	}

	// What is refused.
	if code, body := f.change("claude", map[string]any{"kind": "remove-template", "template": "r7800-quiet"}); code != 409 || !strings.Contains(body["error"].(string), "in use") {
		t.Fatalf("removing a template in use: %d %v", code, body)
	}
	if code, body := f.change("claude", map[string]any{"kind": "edit-template", "template": "r7800-quiet", "name": "R7800, quiet", "boards": []string{"arista,c360"}}); code != 400 || !strings.Contains(body["error"].(string), "not for that board") {
		t.Fatalf("taking a picked board away: %d %v", code, body)
	}
	if code, body := f.change("claude", map[string]any{"kind": "set-template", "template": "r7800-quiet", "path": "concentrators.lab.address", "value": "1.1.1.2"}); code != 400 {
		t.Fatalf("a tunnel in a template: %d %v", code, body)
	}
	if code, body := f.change("claude", map[string]any{"kind": "add-template", "template": "x", "name": "X", "parent": ap, "boards": []string{"netgear,r7800"}}); code != 400 {
		t.Fatalf("a template at an AP: %d %v", code, body)
	}
	if code, body := f.change("claude", map[string]any{"kind": "add-template", "template": "y", "name": "Y", "parent": "house", "boards": []string{"Bad.Board"}}); code != 400 {
		t.Fatalf("a bad board: %d %v", code, body)
	}
	if code, body := pick("house", ""); code != 400 {
		t.Fatalf("an empty pick: %d %v", code, body)
	}
	// Unpicked, it can go.
	if code, body := f.change("claude", map[string]any{"kind": "unset", "tree": "locations", "node": "house", "path": "templates.netgear,r7800"}); code != 200 {
		t.Fatalf("unpick: %d %v", code, body)
	}
	if code, body := f.change("claude", map[string]any{"kind": "remove-template", "template": "r7800-quiet"}); code != 200 {
		t.Fatalf("remove-template: %d %v", code, body)
	}
}
