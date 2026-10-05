package change

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/keys"
)

const sealedPass = `{"$sealed":"bm90IHJlYWxseSBzZWFsZWQ="}`

// keyed is people() with a WPA2-PSK network at Household offering VLANs 101
// and 102 to its keys.
func keyed(t *testing.T) *State {
	t.Helper()
	s := people(t)
	for _, op := range []Op{
		{Kind: Set, Tree: Services, Node: "household", Path: "network.sweet.ssid", Value: json.RawMessage(`"Sweet Spot"`)},
		{Kind: Set, Tree: Services, Node: "household", Path: "network.sweet.security", Value: json.RawMessage(`"wpa2-psk"`)},
		{Kind: Set, Tree: Services, Node: "household", Path: "network.sweet.keys.vlans", Value: json.RawMessage(`[101, 102]`)},
		{Kind: Set, Tree: Services, Node: "guest", Path: "network.guest.ssid", Value: json.RawMessage(`"Guest"`)},
		{Kind: Set, Tree: Services, Node: "guest", Path: "network.guest.security", Value: json.RawMessage(`"open"`)},
	} {
		mustApply(t, s, op)
	}
	return s
}

func keyOp(kind Kind, node, network, id, def string) Op {
	return Op{Kind: kind, Node: hierarchy.NodeID(node), Network: network, Key: id, Value: json.RawMessage(def)}
}

func TestKeys(t *testing.T) {
	s := keyed(t)
	before := s.Org.Clone()
	mustApply(t, s, keyOp(AddKey, "household", "sweet", "unit-101", `{"name":"Unit 101","passphrase":`+sealedPass+`,"vlan":101}`))
	mustApply(t, s, keyOp(AddKey, "household", "sweet", "griff-phone", `{"name":"Griff's phone","passphrase":`+sealedPass+`,"macs":["7e:2a:ea:9b:2b:8f"]}`))
	if s.Keys.Len() != 2 || len(s.Keys.Of("household", "sweet")) != 2 {
		t.Fatalf("keys = %+v", s.Keys.All())
	}
	// A key re-versions nothing: the trees are as they were.
	if !reflect.DeepEqual(before.Services.ResolveAll("household"), s.Org.Services.ResolveAll("household")) {
		t.Fatal("a key changed the trees")
	}

	// Rotating without a passphrase keeps the old one; the VLAN changes.
	mustApply(t, s, keyOp(SetKey, "household", "sweet", "unit-101", `{"name":"Unit 101","vlan":102}`))
	k, _ := s.Keys.Get("unit-101")
	if k.VLAN != 102 || string(k.Passphrase) != sealedPass {
		t.Fatalf("after set-key: %+v", k)
	}
	_, eff, err := Apply(s, keyOp(RemoveKey, "household", "sweet", "griff-phone", ``))
	if err != nil || eff.Before == nil {
		t.Fatalf("remove-key: %v %+v", err, eff)
	}
	if strings.Contains(mustJSON(t, eff), "sealed") {
		t.Fatal("a key's effect shows its passphrase")
	}

	cases := []struct {
		name string
		op   Op
		want error
	}{
		{"a plain passphrase", keyOp(AddKey, "household", "sweet", "x", `{"name":"X","passphrase":"plain-text-pass"}`), ErrPlainSecret},
		{"no passphrase", keyOp(AddKey, "household", "sweet", "x", `{"name":"X"}`), nil},
		{"a VLAN the network doesn't offer", keyOp(AddKey, "household", "sweet", "x", `{"name":"X","passphrase":`+sealedPass+`,"vlan":300}`), nil},
		{"an open network", keyOp(AddKey, "guest", "guest", "x", `{"name":"X","passphrase":`+sealedPass+`}`), ErrNotPSK},
		{"a network the folder doesn't offer", keyOp(AddKey, "guest", "sweet", "x", `{"name":"X","passphrase":`+sealedPass+`}`), nil},
		{"an ID in use", keyOp(AddKey, "household", "sweet", "unit-101", `{"name":"X","passphrase":`+sealedPass+`}`), keys.ErrKeyExists},
		{"a key that isn't there", keyOp(SetKey, "household", "sweet", "nobody", `{"name":"X"}`), keys.ErrNoKey},
		{"a bad MAC", keyOp(AddKey, "household", "sweet", "x", `{"name":"X","passphrase":`+sealedPass+`,"macs":["7E:2A:EA:9B:2B:8F"]}`), nil},
		{"a bad ID", keyOp(AddKey, "household", "sweet", "Unit 1", `{"name":"X","passphrase":`+sealedPass+`}`), nil},
		{"an unknown field", keyOp(AddKey, "household", "sweet", "x", `{"name":"X","passphrase":`+sealedPass+`,"psk":"x"}`), nil},
	}
	for _, c := range cases {
		_, _, err := Apply(s, c.op)
		if err == nil || (c.want != nil && !errors.Is(err, c.want)) {
			t.Errorf("%s: %v", c.name, err)
		}
	}

	// The trees can't strand a key: its VLAN, its network's security, or
	// its network.
	for name, op := range map[string]Op{
		"its VLAN dropped": {Kind: Set, Tree: Services, Node: "household", Path: "network.sweet.keys.vlans", Value: json.RawMessage(`[101]`)},
		"security changed": {Kind: Set, Tree: Services, Node: "household", Path: "network.sweet.security", Value: json.RawMessage(`"wpa3-sae"`)},
		"the network gone": {Kind: Unset, Tree: Services, Node: "household", Paths: []hierarchy.Path{"network.sweet.ssid", "network.sweet.security", "network.sweet.keys.vlans"}},
	} {
		c := s.Clone()
		if _, _, err := Apply(c, op); !errors.Is(err, ErrInUse) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A VLAN no key uses can go.
	mustApply(t, s, Op{Kind: Set, Tree: Services, Node: "household", Path: "network.sweet.keys.vlans", Value: json.RawMessage(`[102]`)})
}

func TestWhoMayChangeKeys(t *testing.T) {
	s := keyed(t)
	add := keyOp(AddKey, "household", "sweet", "unit-101", `{"name":"Unit 101","passphrase":`+sealedPass+`}`)
	for actor, ok := range map[string]bool{"office": true, "claude": true, "griff": true, "gatekeeper": false} {
		if err := Authorize(s, actor, add); (err == nil) != ok {
			t.Errorf("%s: %v", actor, err)
		}
	}
	if err := Authorize(s, "office", keyOp(AddKey, "guest", "guest", "x", `{}`)); err == nil {
		t.Error("office, a viewer at Guest, may add a key there")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
