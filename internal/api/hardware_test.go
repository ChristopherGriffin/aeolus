package api

import (
	"fmt"
	"strings"
	"testing"
)

// enrollAt enrolls an AP with the radios it reports, adopts it into a
// folder, and has it report its channels.
func (f *fixture) enrollAt(name, mac, folder string, radios []any, channels map[string]int) string {
	f.t.Helper()
	code, _, body := f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": mac, "hostname": name, "radios": radios}, nil)
	if code != 201 {
		f.t.Fatalf("enroll %s: %d %v", name, code, body)
	}
	ap, token := body["ap"].(string), body["token"].(string)
	if code, b := f.change("griff", map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": folder}); code != 200 {
		f.t.Fatalf("adopt %s: %d %v", name, code, b)
	}
	f.report(token, channels)
	return ap + "|" + token
}

func (f *fixture) report(token string, channels map[string]int) {
	f.t.Helper()
	var radios []any
	for band, ch := range channels {
		radios = append(radios, map[string]any{"radio": "r" + band, "band": band, "channel": ch, "clients": 0})
	}
	_, _, cfg := f.apDo("GET", "/v1/ap/config", token, nil, nil)
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": cfg["version"], "radios": radios}, nil); code != 200 {
		f.t.Fatalf("state: %d %v", code, b)
	}
}

// hardwareOf returns band -> width -> why ("" when it can be used).
func (f *fixture) hardwareOf(node string) (map[string]map[float64]string, map[string]any) {
	f.t.Helper()
	code, page := f.do("GET", "/v1/trees/locations/nodes/"+node, "griff", nil)
	if code != 200 {
		f.t.Fatalf("%s: %d %v", node, code, page)
	}
	hw := page["hardware"].(map[string]any)
	out := map[string]map[float64]string{}
	for _, b := range hw["bands"].([]any) {
		bv := b.(map[string]any)
		ws := map[float64]string{}
		for _, w := range bv["widths"].([]any) {
			wv := w.(map[string]any)
			why, _ := wv["why"].(string)
			if wv["ok"] == true {
				why = ""
			}
			ws[wv["width"].(float64)] = why
		}
		out[bv["band"].(string)] = ws
	}
	return out, hw
}

// widthOf finds one width's offer on a band.
func widthOf(t *testing.T, hw map[string]any, band string, width float64) map[string]any {
	t.Helper()
	for _, b := range hw["bands"].([]any) {
		bv := b.(map[string]any)
		if bv["band"] != band {
			continue
		}
		for _, w := range bv["widths"].([]any) {
			if wv := w.(map[string]any); wv["width"] == width {
				return wv
			}
		}
	}
	t.Fatalf("no %v MHz on %s in %v", width, band, hw)
	return nil
}

func TestFolderOffersWhatEveryAPCanDo(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	wide := []any{
		map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HT20", "HT40", "VHT20", "VHT40", "VHT80", "VHT160"}},
		map[string]any{"radio": "radio1", "band": "2g", "htmodes": []string{"HT20", "HT40"}},
	}
	narrow := []any{
		map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HT20", "HT40", "VHT80"}},
		map[string]any{"radio": "radio1", "band": "2g", "htmodes": []string{"HT20"}},
	}
	a := strings.Split(f.enrollAt("WideAP", "02:00:00:00:00:0a", "house", wide, map[string]int{"5g": 36, "2g": 6}), "|")
	f.enrollAt("NarrowAP", "02:00:00:00:00:0b", "house", narrow, map[string]int{"5g": 149, "2g": 1})

	// The folder offers only what both can do.
	house, hw := f.hardwareOf("house")
	if house["5g"][80] != "" || house["5g"][160] != "NarrowAP cannot do it" || house["2g"][40] != "NarrowAP cannot do it" {
		t.Fatalf("house = %v", house)
	}
	if len(hw["aps"].([]any)) != 3 || len(hw["unknown"].([]any)) != 1 { // office-ap never reported its radios
		t.Fatalf("aps %v, unknown %v", hw["aps"], hw["unknown"])
	}

	// One AP alone offers what it can do, as its own setting.
	wideAP, _ := f.hardwareOf(a[0])
	if wideAP["5g"][160] != "" || wideAP["2g"][40] != "" {
		t.Fatalf("WideAP = %v", wideAP)
	}
	// On channel 149, which cannot carry 160 MHz, it is still offered, with
	// the channel to move to (0045).
	f.report(a[1], map[string]int{"5g": 149})
	_, hw = f.hardwareOf(a[0])
	if w := widthOf(t, hw, "5g", 160); w["ok"] != true || w["channel"] != 36.0 || len(w["moves"].([]any)) != 1 {
		t.Fatalf("WideAP 160 MHz on 149 = %v", w)
	}
	if w := widthOf(t, hw, "5g", 80); w["ok"] != true || w["channel"] != nil || w["radar"] != nil {
		t.Fatalf("WideAP 80 MHz on 149 = %v", w)
	}

	// A folder's setting does not reach past a break, so those APs do not
	// count.
	if code, b := f.change("griff", map[string]any{"kind": "break-hierarchy", "tree": "locations", "node": "office"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	if _, hw := f.hardwareOf("house"); len(hw["unknown"].([]any)) != 0 {
		t.Fatalf("office-ap still counted past the break: %v", hw["unknown"])
	}
}

func TestWidthMovesTheChannel(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	wide := []any{map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HT20", "HT40", "VHT80", "VHT160"}}}
	a := strings.Split(f.enrollAt("LowAP", "02:00:00:00:00:0a", "house", wide, map[string]int{"5g": 36}), "|")[0]
	b := strings.Split(f.enrollAt("HighAP", "02:00:00:00:00:0b", "house", wide, map[string]int{"5g": 149}), "|")[0]

	// House offers 160 MHz, moving the band to 36. Only HighAP moves, and
	// 36–64 includes radar channels. 80 MHz fits both where they are.
	_, hw := f.hardwareOf("house")
	w := widthOf(t, hw, "5g", 160)
	moves, _ := w["moves"].([]any)
	if w["ok"] != true || w["channel"] != 36.0 || w["radar"] != true || len(moves) != 1 {
		t.Fatalf("160 MHz = %v", w)
	}
	if m := moves[0].(map[string]any); m["id"] != b || m["name"] != "HighAP" || m["from"] != 149.0 {
		t.Fatalf("moves = %v", moves)
	}
	if w := widthOf(t, hw, "5g", 80); w["ok"] != true || w["channel"] != nil || w["moves"] != nil || w["radar"] != nil {
		t.Fatalf("80 MHz = %v", w)
	}

	// Width and channel go in one change: one log entry, one new version
	// for each AP, and no problems.
	code, res := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "house",
		"values": map[string]any{"radio.5g.width": 160, "radio.5g.channel": 36}})
	if code != 200 || len(res["reversioned"].([]any)) != 3 || len(res["checks"].(map[string]any)) != 0 {
		t.Fatalf("%d %v", code, res)
	}
	eff := res["change"].(map[string]any)["effect"].(map[string]any)
	if before := eff["before"].(map[string]any); before["radio.5g.width"] != 40.0 || len(before) != 1 {
		t.Fatalf("effect = %v", eff)
	}
	if after := eff["after"].(map[string]any); after["radio.5g.width"] != 160.0 || after["radio.5g.channel"] != 36.0 {
		t.Fatalf("effect = %v", eff)
	}
	for _, ap := range []string{a, b} {
		_, page := f.do("GET", "/v1/trees/locations/nodes/"+ap, "griff", nil)
		ch := page["fields"].(map[string]any)["radio.5g.channel"].(map[string]any)
		if ch["value"] != 36.0 || ch["from"] != "house" {
			t.Fatalf("%s channel = %v", ap, ch)
		}
	}

	// If one field cannot be set, neither is.
	if code, res := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "house",
		"values": map[string]any{"radio.5g.width": 80, "radio.5g.channel": 50}}); code != 400 {
		t.Fatalf("channel 50: %d %v", code, res)
	}
	if code, res := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "house",
		"path": "radio.5g.width", "value": 80, "values": map[string]any{"radio.5g.channel": 36}}); code != 400 {
		t.Fatalf("both forms: %d %v", code, res)
	}

	// A channel HighAP sets for itself that cannot carry the width is held,
	// and House no longer offers 160 MHz: its channel would not reach.
	code, res = f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": b, "path": "radio.5g.channel", "value": 149})
	if code != 200 || !strings.Contains(fmt.Sprint(res["checks"]), "channel 149 cannot use a 160 MHz width") {
		t.Fatalf("%d %v", code, res)
	}
	if house, _ := f.hardwareOf("house"); house["5g"][160] != "HighAP sets its own channel 149" || house["5g"][80] != "" {
		t.Fatalf("house = %v", house)
	}

	// So does a channel a folder between sets for it.
	for _, op := range []map[string]any{
		{"kind": "unset", "tree": "locations", "node": b, "path": "radio.5g.channel"},
		{"kind": "add-folder", "tree": "locations", "node": "attic", "name": "Attic", "parent": "house"},
		{"kind": "move", "tree": "locations", "node": b, "parent": "attic"},
		{"kind": "set", "tree": "locations", "node": "attic", "path": "radio.5g.channel", "value": 149},
	} {
		if code, res := f.change("griff", op); code != 200 {
			t.Fatalf("%v: %d %v", op, code, res)
		}
	}
	if house, _ := f.hardwareOf("house"); house["5g"][160] != "Attic sets channel 149 for HighAP" {
		t.Fatalf("house = %v", house)
	}

	// A lock above holds the channel, so it cannot move.
	for _, op := range []map[string]any{
		{"kind": "set", "tree": "locations", "node": "symtus", "path": "radio.5g.channel", "value": 149},
		{"kind": "lock", "tree": "locations", "node": "symtus", "path": "radio.5g.channel"},
		{"kind": "unset", "tree": "locations", "node": "house", "path": "radio.5g.width"},
	} {
		if code, res := f.change("griff", op); code != 200 {
			t.Fatalf("%v: %d %v", op, code, res)
		}
	}
	if house, _ := f.hardwareOf("house"); house["5g"][160] != "the channel is locked at Symtus" || house["5g"][80] != "" {
		t.Fatalf("house = %v", house)
	}
}
