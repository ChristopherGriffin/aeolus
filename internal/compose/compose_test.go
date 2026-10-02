package compose

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/schema"
)

// site: House (office-ap) and Main Gate (gate-ap). The Sweet Spot network's
// primary transport is VXLAN to "homelab", which may only be used at the gate;
// its fallback is VLAN 20.
func site(t *testing.T) (*change.State, *schema.Schema) {
	t.Helper()
	sch, err := schema.V1()
	must(t, err)
	L, S := change.Locations, change.Services
	set := func(tree change.TreeName, node, path string, v any) change.Op {
		raw, _ := json.Marshal(v)
		return change.Op{Kind: change.Set, Tree: tree, Node: hierarchy.NodeID(node), Path: hierarchy.Path(path), Value: raw}
	}
	var s *change.State
	for _, op := range []change.Op{
		{Kind: change.CreateOrg, Node: "symtus", Name: "Symtus", Account: "griff"},
		{Kind: change.AddBuiltins},
		{Kind: change.AddFolder, Tree: L, Node: "house", Name: "House", Parent: "symtus"},
		{Kind: change.AddFolder, Tree: L, Node: "gate", Name: "Main Gate", Parent: "symtus"},
		{Kind: change.AddAP, Tree: L, Node: "office-ap", Name: "OfficeOpenWrt", Parent: "house"},
		{Kind: change.AddAP, Tree: L, Node: "gate-ap", Name: "GateOpenWrt", Parent: "gate"},
		{Kind: change.AddAP, Tree: L, Node: "new-ap", Name: "NewAP", Parent: change.LandingZone},
		{Kind: change.AddFolder, Tree: S, Node: "household", Name: "Household", Parent: "symtus"},
		{Kind: change.SetConcentrator, Concentrator: "homelab", Value: json.RawMessage(`{"name":"Homelab","address":"1.1.1.2","port":4789,"mtu":1450,"scope":["gate"]}`)},
		{Kind: change.SetVNI, Concentrator: "homelab", VNI: 20, Name: "Trusted"},
		set(S, "household", "network.sweet.ssid", "Sweet Spot"),
		set(S, "household", "network.sweet.security", "open"),
		set(S, "household", "network.sweet.transport.primary.type", "vxlan"),
		set(S, "household", "network.sweet.transport.primary.concentrator", "homelab"),
		set(S, "household", "network.sweet.transport.primary.vni", 20),
		set(S, "household", "network.sweet.transport.fallback.type", "vlan"),
		set(S, "household", "network.sweet.transport.fallback.vlan", 20),
		{Kind: change.AssignServices, Node: "symtus", Services: []hierarchy.NodeID{"household"}},
	} {
		if s, _, err = change.Apply(s, op); err != nil {
			t.Fatalf("%+v: %v", op, err)
		}
	}
	return s, sch
}

func transport(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	return doc["network"].(map[string]any)["sweet"].(map[string]any)["transport"].(map[string]any)
}

func TestTransportOutsideItsScopeIsLeftOutAndTheFallbackPromoted(t *testing.T) {
	s, sch := site(t)
	res, err := AP(s, sch, "office-ap", nil)
	must(t, err)
	if len(res.Problems) != 0 {
		t.Fatalf("problems: %v", res.Problems)
	}
	tr := transport(t, res.Doc)
	primary := tr["primary"].(map[string]any)
	if primary["type"] != "vlan" || primary["vlan"] != float64(20) || tr["fallback"] != nil {
		t.Fatalf("office transport = %v", tr)
	}
	if res.Doc["concentrators"] != nil {
		t.Fatalf("office got concentrators: %v", res.Doc["concentrators"])
	}
}

func TestTransportInsideItsScopeIsKeptWithItsConcentrator(t *testing.T) {
	s, sch := site(t)
	res, err := AP(s, sch, "gate-ap", nil)
	must(t, err)
	if len(res.Problems) != 0 {
		t.Fatalf("problems: %v", res.Problems)
	}
	tr := transport(t, res.Doc)
	if tr["primary"].(map[string]any)["type"] != "vxlan" || tr["fallback"].(map[string]any)["type"] != "vlan" {
		t.Fatalf("gate transport = %v", tr)
	}
	homelab := res.Doc["concentrators"].(map[string]any)["homelab"].(map[string]any)
	if homelab["address"] != "1.1.1.2" || homelab["port"] != float64(4789) || homelab["mtu"] != float64(1450) {
		t.Fatalf("homelab = %v", homelab)
	}
}

func TestNoUsableTransportIsAProblem(t *testing.T) {
	s, sch := site(t)
	if _, _, err := change.Apply(s, change.Op{Kind: change.Unset, Tree: change.Services, Node: "household", Path: "network.sweet.transport.fallback.type"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := change.Apply(s, change.Op{Kind: change.Unset, Tree: change.Services, Node: "household", Path: "network.sweet.transport.fallback.vlan"}); err != nil {
		t.Fatal(err)
	}
	res, err := AP(s, sch, "office-ap", nil)
	must(t, err)
	if !contains(res.Problems, "no transport is usable") {
		t.Fatalf("problems = %v", res.Problems)
	}
	if res, _ := AP(s, sch, "gate-ap", nil); len(res.Problems) != 0 {
		t.Fatalf("gate should still work: %v", res.Problems)
	}
}

func TestVNIMissingFromTheConcentratorIsAProblem(t *testing.T) {
	s, sch := site(t)
	if _, _, err := change.Apply(s, change.Op{Kind: change.Set, Tree: change.Services, Node: "household", Path: "network.sweet.transport.primary.vni", Value: json.RawMessage(`99`)}); err != nil {
		t.Fatal(err)
	}
	res, err := AP(s, sch, "gate-ap", nil)
	must(t, err)
	if !contains(res.Problems, "VNI 99 is not defined on concentrator homelab") {
		t.Fatalf("AP problems = %v", res.Problems)
	}
	if p := Node(s, sch, change.Services, s.Org.Services, "household", nil); !contains(p, "VNI 99 is not defined") {
		t.Fatalf("node problems = %v", p)
	}
}

func TestLandingZoneAPGetsNothing(t *testing.T) {
	s, sch := site(t)
	res, err := AP(s, sch, "new-ap", nil)
	must(t, err)
	if !res.Unassigned || res.Doc != nil {
		t.Fatalf("Landing Zone AP = %+v", res)
	}
}

func TestWidthTheRadioCannotDoIsAProblem(t *testing.T) {
	s, sch := site(t)
	s.Facts["gate-ap"] = json.RawMessage(`{"radios":[{"radio":"radio0","band":"5g","htmodes":["HT20","HT40","VHT20","VHT40","VHT80","VHT80+80"]},{"radio":"radio1","band":"2g","htmodes":["HT20","HT40"]}]}`)
	set := func(path string, v any) {
		raw, _ := json.Marshal(v)
		if _, _, err := change.Apply(s, change.Op{Kind: change.Set, Tree: change.Locations, Node: "gate-ap", Path: hierarchy.Path(path), Value: raw}); err != nil {
			t.Fatal(err)
		}
	}
	set("radio.5g.width", 80)
	set("radio.2g.width", 40)
	if res, _ := AP(s, sch, "gate-ap", nil); len(res.Problems) != 0 {
		t.Fatalf("widths it can do: %v", res.Problems)
	}
	set("radio.5g.width", 160)
	res, err := AP(s, sch, "gate-ap", nil)
	must(t, err)
	if !contains(res.Problems, "radio.5g.width: this AP's radio cannot use 160 MHz; it can use 20, 40, 80 MHz") {
		t.Fatalf("problems = %v", res.Problems)
	}
	// An AP that reported nothing about its radios is not judged.
	s.Facts["gate-ap"] = nil
	if res, _ := AP(s, sch, "gate-ap", nil); len(res.Problems) != 0 {
		t.Fatalf("without facts: %v", res.Problems)
	}
}

func TestChannelThatCannotCarryTheWidthIsAProblem(t *testing.T) {
	s, sch := site(t)
	set := func(path string, v any) {
		raw, _ := json.Marshal(v)
		if _, _, err := change.Apply(s, change.Op{Kind: change.Set, Tree: change.Locations, Node: "gate-ap", Path: hierarchy.Path(path), Value: raw}); err != nil {
			t.Fatal(err)
		}
	}
	set("radio.5g.width", 160)
	set("radio.5g.channel", 36)
	if res, _ := AP(s, sch, "gate-ap", nil); contains(res.Problems, "radio.5g.channel") {
		t.Fatalf("36 carries 160 MHz: %v", res.Problems)
	}
	set("radio.5g.channel", 149)
	res, err := AP(s, sch, "gate-ap", nil)
	must(t, err)
	if !contains(res.Problems, "radio.5g.channel: channel 149 cannot use a 160 MHz width") {
		t.Fatalf("problems = %v", res.Problems)
	}
	// A channel the AP picks itself is not known here.
	set("radio.5g.channel", "auto")
	if res, _ := AP(s, sch, "gate-ap", nil); contains(res.Problems, "radio.5g.channel") {
		t.Fatalf("auto: %v", res.Problems)
	}
}

func contains(list []string, sub string) bool {
	return strings.Contains(strings.Join(list, "\n"), sub)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
