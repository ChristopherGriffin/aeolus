package api

import (
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
	// Its channel limits it too: 160 MHz does not fit channel 149.
	f.report(a[1], map[string]int{"5g": 149})
	if wideAP, _ := f.hardwareOf(a[0]); wideAP["5g"][160] != "not on channel 149" || wideAP["5g"][80] != "" {
		t.Fatalf("WideAP on 149 = %v", wideAP)
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
