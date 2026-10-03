package api

import "testing"

// An AP that set its own radio settings follows its folder again in one
// change, and the preview says what each field becomes (0046).
func TestFollowTheFolderAgain(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "office-ap",
		"values": map[string]any{"radio.5g.width": 160, "radio.5g.channel": "auto"}}); code != 200 {
		t.Fatalf("custom: %d %v", code, b)
	}

	op := map[string]any{"kind": "unset", "tree": "locations", "node": "office-ap", "paths": []string{"radio.5g.width", "radio.5g.channel"}}
	code, p := f.do("POST", "/v1/preview", "griff", map[string]any{"op": op})
	if code != 200 {
		t.Fatalf("preview: %d %v", code, p)
	}
	res := p["resolved"].(map[string]any)
	width, _ := res["radio.5g.width"].(map[string]any)
	if width["value"] != 40.0 || width["from"] != "house" || width["origin"] != "inherited" {
		t.Fatalf("width becomes %v", res["radio.5g.width"])
	}
	if ch, ok := res["radio.5g.channel"]; !ok || ch != nil {
		t.Fatalf("channel becomes %v, want null: nothing above sets it", ch)
	}
	if before := p["effect"].(map[string]any)["before"].(map[string]any); before["radio.5g.width"] != 160.0 || before["radio.5g.channel"] != "auto" {
		t.Fatalf("effect = %v", p["effect"])
	}
	if got := p["reversioned"].([]any); len(got) != 1 || got[0] != "office-ap" {
		t.Fatalf("reversioned = %v", got)
	}

	if code, b := f.change("griff", op); code != 200 {
		t.Fatalf("follow: %d %v", code, b)
	}
	_, page := f.do("GET", "/v1/trees/locations/nodes/office-ap", "griff", nil)
	fields := page["fields"].(map[string]any)
	if w := fields["radio.5g.width"].(map[string]any); w["from"] != "house" {
		t.Fatalf("width = %v", w)
	}
	if _, ok := fields["radio.5g.channel"]; ok {
		t.Fatalf("channel still set: %v", fields["radio.5g.channel"])
	}

	// Nothing is left to unset: refused, and nothing changes.
	if code, b := f.change("griff", op); code != 400 {
		t.Fatalf("again: %d %v", code, b)
	}
}
