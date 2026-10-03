package api

import (
	"fmt"
	"strings"
	"testing"
)

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

func TestSchemaDescribesItsFields(t *testing.T) {
	f := newFixture(t)
	if code, _ := f.do("GET", "/v1/schema", "", nil); code != 401 {
		t.Fatalf("without a token: %d", code)
	}
	code, d := f.do("GET", "/v1/schema", "tenant", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, d)
	}
	sec := d["fields"].(map[string]any)["network.*.security"].(map[string]any)
	if len(sec["enum"].([]any)) != 5 || sec["x-aeolus-tree"] != "services" {
		t.Fatalf("security = %v", sec)
	}
	if d["names"].(map[string]any)["network"] == nil {
		t.Fatalf("names = %v", d["names"])
	}
}

// A secret set with other fields in one change is sealed before it is
// logged, like one set alone (0027, 0045).
func TestValuesSealSecrets(t *testing.T) {
	f := newFixture(t)
	code, res := f.change("griff", map[string]any{"kind": "set", "tree": "services", "node": "household", "values": map[string]any{
		"network.guest2.ssid": "Guest 2", "network.guest2.security": "wpa2-psk", "network.guest2.passphrase": "plain-text-passphrase",
		"network.guest2.transport.primary.type": "vlan", "network.guest2.transport.primary.vlan": 30}})
	if code != 200 {
		t.Fatalf("%d %v", code, res)
	}
	_, log := f.do("GET", "/v1/changes", "griff", nil)
	raw := fmt.Sprint(log)
	if strings.Contains(raw, "plain-text-passphrase") {
		t.Fatalf("the passphrase reached the log view: %s", raw)
	}
	changes := log["changes"].([]any)
	op := changes[len(changes)-1].(map[string]any)["op"].(map[string]any)
	if pass := op["values"].(map[string]any)["network.guest2.passphrase"]; fmt.Sprint(pass) != "map[sealed:true]" {
		t.Fatalf("passphrase logged as %v", pass)
	}
}
