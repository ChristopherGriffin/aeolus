package compose

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/schema"
)

// site: House (office-ap) and Main Gate (gate-ap). The Sweet Spot network's
// primary transport is VXLAN over the tunnel "homelab", which is set only at
// the gate (0055); its fallback is VLAN 20.
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
		set(L, "gate", "concentrators.homelab.address", "1.1.1.2"),
		set(L, "gate", "concentrators.homelab.port", 4789),
		set(L, "gate", "concentrators.homelab.mtu", 1450),
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

// A tunnel set at the Org reaches every AP; an AP can change its far end;
// and one nothing uses, even unfinished, holds no AP back (0055).
func TestTunnelsAreLocationSettings(t *testing.T) {
	s, sch := site(t)
	set := func(node, path string, v any) {
		raw, _ := json.Marshal(v)
		if _, _, err := change.Apply(s, change.Op{Kind: change.Set, Tree: change.Locations, Node: hierarchy.NodeID(node), Path: hierarchy.Path(path), Value: raw}); err != nil {
			t.Fatal(err)
		}
	}
	set("symtus", "concentrators.homelab.address", "1.1.1.9")
	set("symtus", "concentrators.homelab.port", 4789)
	set("symtus", "concentrators.homelab.mtu", 1400)
	set("office-ap", "concentrators.homelab.address", "1.1.1.10")
	set("office-ap", "concentrators.homelab.probe_interval", 10)
	set("symtus", "concentrators.spare.address", "1.1.1.11") // no port or MTU, and unused
	// What a tunnel leaves unset comes as its default (0056, 0059).
	for ap, want := range map[string]map[string]any{
		"office-ap": {"address": "1.1.1.10", "port": float64(4789), "mtu": float64(1400), "probe_interval": float64(10)},
		"gate-ap":   {"address": "1.1.1.2", "port": float64(4789), "mtu": float64(1450), "probe_interval": float64(30)},
	} {
		res, err := AP(s, sch, hierarchy.NodeID(ap), nil)
		must(t, err)
		if len(res.Problems) != 0 {
			t.Fatalf("%s problems: %v", ap, res.Problems)
		}
		concs := res.Doc["concentrators"].(map[string]any)
		if got := concs["homelab"].(map[string]any); fmt.Sprint(got) != fmt.Sprint(want) || len(concs) != 1 {
			t.Fatalf("%s tunnels = %v, want only homelab %v", ap, concs, want)
		}
		if tr := transport(t, res.Doc); tr["primary"].(map[string]any)["type"] != "vxlan" {
			t.Fatalf("%s transport = %v", ap, tr)
		}
	}
	// The folder's page shows the unfinished tunnel.
	if p := Node(s, sch, change.Locations, s.Org.Locations, "symtus", nil); !contains(p, "concentrators.spare") {
		t.Fatalf("node problems = %v", p)
	}
}

// A tunnel port's VNIs are held where the AP could not carry them (0058).
// Above 9999, the AP's MAC on a segment ends in the VNI's last 16 bits, so
// two such VNIs that agree there are held at one AP (0060).
func TestSegmentMACsDoNotClash(t *testing.T) {
	doc := map[string]any{
		"concentrators": map[string]any{"dc": map[string]any{"address": "1.1.1.2", "port": float64(4789), "mtu": float64(1450)}},
		"network": map[string]any{
			"a": map[string]any{"transport": map[string]any{"primary": map[string]any{"type": "vxlan", "concentrator": "dc", "vni": float64(10050)}}},
			"b": map[string]any{"transport": map[string]any{"primary": map[string]any{"type": "vxlan", "concentrator": "dc", "vni": float64(10050 + 65536)}}},
			"c": map[string]any{"transport": map[string]any{"primary": map[string]any{"type": "vxlan", "concentrator": "dc", "vni": float64(50)}}},
		},
	}
	got := strings.Join(tunnelProblems(doc), "\n")
	if !strings.Contains(got, "VNI 10050 and VNI 75586 would give this AP one MAC on both segments") {
		t.Fatalf("problems: %s", got)
	}
	delete(doc["network"].(map[string]any), "b")
	if got := tunnelProblems(doc); len(got) != 0 {
		t.Fatalf("problems without the clash: %v", got)
	}
}

// A network with a fallback has its tunnel moved into and out of its own
// bridge (0061), so no port can carry that VNI as well.
func TestASwitchingNetworksTunnelIsItsOwn(t *testing.T) {
	doc := map[string]any{
		"concentrators": map[string]any{"dc": map[string]any{"address": "1.1.1.2", "port": float64(4789)}},
		"network": map[string]any{
			"lab": map[string]any{"transport": map[string]any{
				"primary":  map[string]any{"type": "vxlan", "concentrator": "dc", "vni": float64(50)},
				"fallback": map[string]any{"type": "vlan", "vlan": float64(20)},
			}},
		},
		"ports": map[string]any{"lan3": map[string]any{"mode": "tunnel", "vxlan": map[string]any{
			"untagged": map[string]any{"tunnel": "dc", "vni": float64(50)},
		}}},
	}
	want := "VNI 50: network lab has a fallback, so the AP moves its tunnel into and out of the network's own bridge (0061), and port lan3 cannot carry it"
	if got := strings.Join(tunnelProblems(doc), "\n"); got != want {
		t.Fatalf("problems: %s", got)
	}
	delete(doc["network"].(map[string]any)["lab"].(map[string]any)["transport"].(map[string]any), "fallback")
	if got := tunnelProblems(doc); len(got) != 0 {
		t.Fatalf("problems without the fallback: %v", got)
	}
}

// Automatic switching needs a fallback (0061). Where only one of a network's
// transports is usable, there is nothing to switch between: the AP gets no
// switching settings, and is not held.
func TestAutomaticSwitchingNeedsAFallback(t *testing.T) {
	s, sch := site(t)
	set := func(path string, v any) {
		raw, _ := json.Marshal(v)
		if _, _, err := change.Apply(s, change.Op{Kind: change.Set, Tree: change.Services, Node: "household", Path: hierarchy.Path(path), Value: raw}); err != nil {
			t.Fatal(err)
		}
	}
	set("network.sweet.transport.switching", "automatic")
	set("network.sweet.transport.holddown", 60)
	res, err := AP(s, sch, "gate-ap", nil)
	must(t, err)
	if tr := transport(t, res.Doc); len(res.Problems) != 0 || tr["switching"] != "automatic" || tr["holddown"] != float64(60) {
		t.Fatalf("gate: %v, problems %v", tr, res.Problems)
	}
	// The office is outside the tunnel's scope, so VLAN 20 is its only transport.
	res, err = AP(s, sch, "office-ap", nil)
	must(t, err)
	if tr := transport(t, res.Doc); len(res.Problems) != 0 || tr["switching"] != nil || tr["holddown"] != nil {
		t.Fatalf("office: %v, problems %v", tr, res.Problems)
	}
	for _, f := range []string{"network.sweet.transport.fallback.type", "network.sweet.transport.fallback.vlan"} {
		if _, _, err := change.Apply(s, change.Op{Kind: change.Unset, Tree: change.Services, Node: "household", Path: hierarchy.Path(f)}); err != nil {
			t.Fatal(err)
		}
	}
	want := "network.sweet.transport.switching: automatic switching needs a fallback (0061)"
	res, err = AP(s, sch, "gate-ap", nil)
	must(t, err)
	if !contains(res.Problems, want) {
		t.Fatalf("gate without a fallback: %v", res.Problems)
	}
	if p := Node(s, sch, change.Services, s.Org.Services, "household", nil); !contains(p, want) {
		t.Fatalf("the folder: %v", p)
	}
}

func TestTunnelPorts(t *testing.T) {
	s, sch := site(t)
	set := func(tree change.TreeName, node, path string, v any) {
		t.Helper()
		raw, _ := json.Marshal(v)
		if _, _, err := change.Apply(s, change.Op{Kind: change.Set, Tree: tree, Node: hierarchy.NodeID(node), Path: hierarchy.Path(path), Value: raw}); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	problems := func(ap string) (string, map[string]any) {
		t.Helper()
		res, err := AP(s, sch, hierarchy.NodeID(ap), nil)
		must(t, err)
		return strings.Join(res.Problems, "\n"), res.Doc
	}
	set(change.Locations, "gate-ap", "ports.lan3.mode", "tunnel")
	if got, _ := problems("gate-ap"); !strings.Contains(got, "ports.lan3: a tunnel port carries no VNIs yet") {
		t.Fatalf("no VNIs: %s", got)
	}
	// Sweet's VNI 20 is not for a port: sweet has a fallback, so the AP
	// moves its tunnel into and out of sweet's own bridge (0061).
	set(change.Locations, "gate-ap", "ports.lan3.vxlan.untagged.tunnel", "homelab")
	set(change.Locations, "gate-ap", "ports.lan3.vxlan.untagged.vni", 20)
	if got, _ := problems("gate-ap"); !strings.Contains(got, "VNI 20: network sweet has a fallback") {
		t.Fatalf("a switching network's VNI on a port: %s", got)
	}
	// Tagged VLAN 50 and untagged both over homelab.
	set(change.Locations, "gate", "ports.lan3.vxlan.50.tunnel", "homelab")
	set(change.Locations, "gate", "ports.lan3.vxlan.50.vni", 50)
	set(change.Locations, "gate-ap", "ports.lan3.vxlan.untagged.vni", 30)
	got, doc := problems("gate-ap")
	if strings.Contains(got, "ports.") || strings.Contains(got, "VNI") {
		t.Fatalf("a good tunnel port: %s", got)
	}
	if doc["concentrators"].(map[string]any)["homelab"] == nil {
		t.Fatalf("the port's tunnel is not in the config: %v", doc["concentrators"])
	}
	// The office has no homelab tunnel.
	set(change.Locations, "office-ap", "ports.lan3.mode", "tunnel")
	set(change.Locations, "office-ap", "ports.lan3.vxlan.50.tunnel", "homelab")
	set(change.Locations, "office-ap", "ports.lan3.vxlan.50.vni", 50)
	if got, _ := problems("office-ap"); !strings.Contains(got, "ports.lan3.vxlan.50: tunnel homelab is not set where this AP is") {
		t.Fatalf("a tunnel not set here: %s", got)
	}
	// One VNI on two VLANs, and a mapping without its VNI.
	set(change.Locations, "gate-ap", "ports.lan3.vxlan.60.tunnel", "homelab")
	set(change.Locations, "gate-ap", "ports.lan3.vxlan.60.vni", 50)
	set(change.Locations, "gate-ap", "ports.lan3.vxlan.70.tunnel", "homelab")
	got, _ = problems("gate-ap")
	for _, want := range []string{"ports.lan3.vxlan.60: VNI 50 is on VLAN 50 of this port already", "ports.lan3.vxlan.70: needs both its tunnel and its VNI"} {
		if !strings.Contains(got, want) {
			t.Fatalf("problems = %s, want %q", got, want)
		}
	}
	set(change.Locations, "gate-ap", "ports.lan3.vxlan.60.vni", 60)
	set(change.Locations, "gate-ap", "ports.lan3.vxlan.70.vni", 70)
	// A VNI that waits as a network's fallback, and one reaching two tunnels.
	set(change.Services, "household", "network.sweet.transport.fallback.type", "vxlan")
	set(change.Services, "household", "network.sweet.transport.fallback.concentrator", "homelab")
	set(change.Services, "household", "network.sweet.transport.fallback.vni", 60)
	set(change.Locations, "gate", "concentrators.other.address", "1.1.1.9")
	set(change.Locations, "gate", "concentrators.other.port", 4789)
	set(change.Locations, "gate-ap", "ports.lan3.vxlan.70.tunnel", "other")
	set(change.Locations, "gate-ap", "ports.lan3.vxlan.70.vni", 20)
	got, _ = problems("gate-ap")
	for _, want := range []string{
		"VNI 60: it is network sweet's fallback here, which waits until switching starts it, so port lan3 cannot carry it",
		"VNI 20: it reaches tunnels homelab and other at this AP",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("problems = %s, want %q", got, want)
		}
	}
}

// A tunnel without an MTU gets the default for its far end's family (0056).
func TestTunnelMTUDefault(t *testing.T) {
	s, sch := site(t)
	op := func(kind change.Kind, path string, v any) {
		raw, _ := json.Marshal(v)
		if _, _, err := change.Apply(s, change.Op{Kind: kind, Tree: change.Locations, Node: "gate", Path: hierarchy.Path(path), Value: raw}); err != nil {
			t.Fatal(err)
		}
	}
	mtu := func() any {
		t.Helper()
		res, err := AP(s, sch, "gate-ap", nil)
		must(t, err)
		if len(res.Problems) != 0 {
			t.Fatalf("problems: %v", res.Problems)
		}
		return res.Doc["concentrators"].(map[string]any)["homelab"].(map[string]any)["mtu"]
	}
	if got := mtu(); got != float64(1450) {
		t.Fatalf("set: %v", got)
	}
	if _, _, err := change.Apply(s, change.Op{Kind: change.Unset, Tree: change.Locations, Node: "gate", Path: "concentrators.homelab.mtu"}); err != nil {
		t.Fatal(err)
	}
	if got := mtu(); got != float64(1450) {
		t.Fatalf("IPv4 default: %v", got)
	}
	op(change.Set, "concentrators.homelab.address", "2001:db8::2")
	if got := mtu(); got != float64(1430) {
		t.Fatalf("IPv6 default: %v", got)
	}
	op(change.Set, "concentrators.homelab.mtu", 9000)
	if got := mtu(); got != float64(9000) {
		t.Fatalf("custom: %v", got)
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

// A key's VLAN is an AP/VLAN interface: on a radio whose driver makes none,
// hostapd fails every network on it, so the config is refused (0082).
func TestKeyVLANsNeedAPVLANInterfaces(t *testing.T) {
	s, sch := site(t)
	set := func(path string, v any) {
		raw, _ := json.Marshal(v)
		if _, _, err := change.Apply(s, change.Op{Kind: change.Set, Tree: change.Services, Node: "household", Path: hierarchy.Path(path), Value: raw}); err != nil {
			t.Fatal(err)
		}
	}
	set("network.sweet.security", "wpa2-psk")
	set("network.sweet.keys.vlans", []int{50})
	keyProblems := func() []string {
		t.Helper()
		res, err := AP(s, sch, "gate-ap", nil)
		must(t, err)
		var out []string
		for _, p := range res.Problems {
			if strings.HasPrefix(p, "network.sweet.keys.vlans:") {
				out = append(out, p)
			}
		}
		return out
	}
	// radio0 has no AP/VLAN interfaces (ath11k); radio1 has; radio2 is the
	// scan radio another service owns (0081); radio3 said nothing.
	s.Facts["gate-ap"] = json.RawMessage(`{"radios":[
		{"radio":"radio0","band":"5g","htmodes":["HE80"],"ap_vlan":false},
		{"radio":"radio1","band":"2g","htmodes":["HE20"],"ap_vlan":true},
		{"radio":"radio2","band":"6g","htmodes":["HE80"],"ap_vlan":false,"reserved":true},
		{"radio":"radio3","band":"6g","htmodes":["HE80"]}]}`)
	want := "network.sweet.keys.vlans: radio0 cannot put clients in VLANs of their own (the driver has no AP/VLAN interfaces); offer the network on other bands, or give its keys no VLANs"
	if got := keyProblems(); len(got) != 1 || got[0] != want {
		t.Fatalf("problems = %v", got)
	}
	// Offered only where every radio can, it is not a problem.
	set("network.sweet.bands", []string{"2g", "6g"})
	if got := keyProblems(); len(got) != 0 {
		t.Fatalf("on 2g and 6g: %v", got)
	}
	set("network.sweet.bands", []string{"5g"})
	if got := keyProblems(); len(got) != 1 {
		t.Fatalf("on 5g: %v", got)
	}
	// An AP that reported nothing about its radios is not judged.
	s.Facts["gate-ap"] = nil
	if got := keyProblems(); len(got) != 0 {
		t.Fatalf("without facts: %v", got)
	}
}

func TestSNMPNeedsAWayIn(t *testing.T) {
	s, sch := site(t)
	set := func(path string, v any) {
		raw, _ := json.Marshal(v)
		if _, _, err := change.Apply(s, change.Op{Kind: change.Set, Tree: change.Locations, Node: "gate-ap", Path: hierarchy.Path(path), Value: raw}); err != nil {
			t.Fatal(err)
		}
	}
	problems := func() string {
		t.Helper()
		res, err := AP(s, sch, "gate-ap", nil)
		must(t, err)
		return strings.Join(res.Problems, "\n")
	}
	set("system.snmp.enabled", true)
	if got := problems(); !strings.Contains(got, "neither a community nor a v3 user") {
		t.Fatalf("problems = %s", got)
	}
	set("system.snmp.v3.user", "monitor")
	if got := problems(); !strings.Contains(got, "needs both an auth and a privacy passphrase") {
		t.Fatalf("problems = %s", got)
	}
	set("system.snmp.v3.auth", "auth-passphrase")
	set("system.snmp.v3.privacy", "privacy-passphrase")
	if got := problems(); strings.Contains(got, "system.snmp") {
		t.Fatalf("a v3 user with both passphrases: %s", got)
	}
}

func TestPortVLANsMustFitTheMode(t *testing.T) {
	s, sch := site(t)
	set := func(node, path string, v any) {
		raw, _ := json.Marshal(v)
		if _, _, err := change.Apply(s, change.Op{Kind: change.Set, Tree: change.Locations, Node: hierarchy.NodeID(node), Path: hierarchy.Path(path), Value: raw}); err != nil {
			t.Fatal(err)
		}
	}
	problems := func() string {
		t.Helper()
		res, err := AP(s, sch, "gate-ap", nil)
		must(t, err)
		return strings.Join(res.Problems, "\n")
	}
	set("gate-ap", "ports.lan2.tagged", []int{10})
	if got := problems(); !strings.Contains(got, "ports.lan2: VLANs are set, but not the mode") {
		t.Fatalf("problems = %s", got)
	}
	// The folder sets the mode, the AP its VLANs.
	set("gate", "ports.lan2.mode", "access")
	got := problems()
	for _, want := range []string{"ports.lan2: an access port needs its untagged VLAN", "ports.lan2: an access port carries no tagged VLANs"} {
		if !strings.Contains(got, want) {
			t.Fatalf("problems = %s, want %q", got, want)
		}
	}
	set("gate-ap", "ports.lan2.mode", "trunk")
	set("gate-ap", "ports.lan2.untagged", 10)
	if got := problems(); !strings.Contains(got, "ports.lan2: VLAN 10 is both untagged and tagged") {
		t.Fatalf("problems = %s", got)
	}
	set("gate-ap", "ports.lan2.untagged", 30)
	set("gate-ap", "ports.lan2.enabled", false)
	if got := problems(); strings.Contains(got, "ports.") {
		t.Fatalf("a trunk with untagged 30 and tagged 10: %s", got)
	}
	set("gate-ap", "ports.lan3.mode", "lacp")
	set("gate-ap", "ports.lan3.bond", "bond0")
	if got := problems(); !strings.Contains(got, "ports.lan3: LACP is not applied yet") {
		t.Fatalf("problems = %s", got)
	}
}

func TestTunnelsAnAPCannotRun(t *testing.T) {
	s, sch := site(t)
	apply := func(op change.Op) {
		t.Helper()
		if _, _, err := change.Apply(s, op); err != nil {
			t.Fatalf("%+v: %v", op, err)
		}
	}
	set := func(path string, v any) {
		raw, _ := json.Marshal(v)
		apply(change.Op{Kind: change.Set, Tree: change.Services, Node: "household", Path: hierarchy.Path(path), Value: raw})
	}
	problems := func() string {
		t.Helper()
		res, err := AP(s, sch, "gate-ap", nil)
		must(t, err)
		return strings.Join(res.Problems, "\n")
	}
	if got := problems(); strings.Contains(got, "VNI") || strings.Contains(got, "address") {
		t.Fatalf("one tunnel: %s", got)
	}
	// A second network on VNI 20 would join the two.
	set("network.guest.ssid", "Guest")
	set("network.guest.security", "open")
	set("network.guest.transport.primary.type", "vxlan")
	set("network.guest.transport.primary.concentrator", "homelab")
	set("network.guest.transport.primary.vni", 20)
	if got := problems(); !strings.Contains(got, "VNI 20: networks guest and sweet would share one tunnel at this AP") {
		t.Fatalf("problems = %s", got)
	}
	// Turned off, it is not rendered, so it shares nothing.
	set("network.guest.enabled", false)
	if got := problems(); strings.Contains(got, "VNI 20") {
		t.Fatalf("guest off: %s", got)
	}
	// A primary and a fallback on one VNI.
	set("network.sweet.transport.fallback.type", "vxlan")
	set("network.sweet.transport.fallback.concentrator", "homelab")
	set("network.sweet.transport.fallback.vni", 20)
	if got := problems(); !strings.Contains(got, "network.sweet.transport: the primary and the fallback both use VNI 20") {
		t.Fatalf("problems = %s", got)
	}
	// An address the schema lets through but that is no IP address.
	set("network.sweet.transport.fallback.type", "vlan")
	tunnel := func(addr string) {
		raw, _ := json.Marshal(addr)
		apply(change.Op{Kind: change.Set, Tree: change.Locations, Node: "gate", Path: "concentrators.homelab.address", Value: raw})
	}
	tunnel("999.1.1.2")
	if got := problems(); !strings.Contains(got, `network.sweet.transport.primary: tunnel homelab's address "999.1.1.2" is not an IP address`) {
		t.Fatalf("problems = %s", got)
	}
	tunnel("2001:db8::2")
	if got := problems(); strings.Contains(got, "is not an IP address") {
		t.Fatalf("an IPv6 address: %s", got)
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

// With DFS avoided, a channel or width that needs DFS channels is held, not
// quietly ignored (0071).
func TestAvoidingDFSRefusesWhatNeedsIt(t *testing.T) {
	s, sch := site(t)
	set := func(path string, v any) {
		raw, _ := json.Marshal(v)
		if _, _, err := change.Apply(s, change.Op{Kind: change.Set, Tree: change.Locations, Node: "gate-ap", Path: hierarchy.Path(path), Value: raw}); err != nil {
			t.Fatal(err)
		}
	}
	problems := func() string {
		t.Helper()
		res, err := AP(s, sch, "gate-ap", nil)
		must(t, err)
		return strings.Join(res.Problems, "\n")
	}
	set("radio.5g.dfs", "avoid")
	set("radio.5g.width", 80)
	set("radio.5g.channel", 48)
	if p := problems(); strings.Contains(p, "dfs") {
		t.Fatalf("48 at 80 MHz is outside DFS: %s", p)
	}
	set("radio.5g.channel", 108)
	if p := problems(); !strings.Contains(p, "radio.5g.channel: channel 108 uses DFS channels, but radio.5g.dfs is avoid") {
		t.Fatalf("108: %s", p)
	}
	set("radio.5g.channel", "auto")
	if p := problems(); strings.Contains(p, "dfs") {
		t.Fatalf("auto at 80 MHz: %s", p)
	}
	// Every 160 MHz block holds DFS channels; the width is the problem,
	// not the channel within it.
	set("radio.5g.width", 160)
	set("radio.5g.channel", 36)
	p := problems()
	if !strings.Contains(p, "radio.5g.width: every 160 MHz channel uses DFS channels, but radio.5g.dfs is avoid") || strings.Contains(p, "radio.5g.channel: channel 36") {
		t.Fatalf("160: %s", p)
	}
	set("radio.5g.dfs", "allow")
	if p := problems(); strings.Contains(p, "dfs") {
		t.Fatalf("allow: %s", p)
	}
}

// A set of channels an automatic channel may be (0075) that leaves the
// radio no whole block at its width, or none outside DFS while it is
// avoided, is held.
func TestChannelSetsNeedAWholeBlock(t *testing.T) {
	s, sch := site(t)
	set := func(path string, v any) {
		raw, _ := json.Marshal(v)
		if _, _, err := change.Apply(s, change.Op{Kind: change.Set, Tree: change.Locations, Node: "gate-ap", Path: hierarchy.Path(path), Value: raw}); err != nil {
			t.Fatal(err)
		}
	}
	problems := func() string {
		t.Helper()
		res, err := AP(s, sch, "gate-ap", nil)
		must(t, err)
		return strings.Join(res.Problems, "\n")
	}
	set("radio.5g.width", 80)
	set("radio.5g.channels", []int{36, 40, 44, 48, 149, 153})
	if p := problems(); strings.Contains(p, "channels") {
		t.Fatalf("36–48 is a whole 80 MHz block: %s", p)
	}
	set("radio.5g.channels", []int{36, 40, 149, 153})
	if p := problems(); !strings.Contains(p, "radio.5g.channels: no 80 MHz block is wholly in the set") {
		t.Fatalf("no whole block: %s", p)
	}
	set("radio.5g.width", 40)
	if p := problems(); strings.Contains(p, "channels") {
		t.Fatalf("at 40 MHz, 36/40 and 149/153 are whole: %s", p)
	}
	set("radio.5g.dfs", "avoid")
	set("radio.5g.channels", []int{52, 56, 60, 64})
	if p := problems(); !strings.Contains(p, "radio.5g.channels: no 40 MHz block outside DFS is wholly in the set, and radio.5g.dfs is avoid") {
		t.Fatalf("only DFS: %s", p)
	}
}

// Power control (0077) works from RRM's neighbours: on without RRM, the
// config is held.
func TestPowerControlNeedsRRM(t *testing.T) {
	s, sch := site(t)
	set := func(path string, v any) {
		raw, _ := json.Marshal(v)
		if _, _, err := change.Apply(s, change.Op{Kind: change.Set, Tree: change.Locations, Node: "gate-ap", Path: hierarchy.Path(path), Value: raw}); err != nil {
			t.Fatal(err)
		}
	}
	problems := func() string {
		t.Helper()
		res, err := AP(s, sch, "gate-ap", nil)
		must(t, err)
		return strings.Join(res.Problems, "\n")
	}
	set("apc.enabled", true)
	if p := problems(); !strings.Contains(p, "apc.enabled: power control needs rrm.enabled") {
		t.Fatalf("without RRM: %s", p)
	}
	set("rrm.enabled", true)
	if p := problems(); strings.Contains(p, "apc") {
		t.Fatalf("with RRM: %s", p)
	}
}

// A VXLAN transport needs a tunnel and a VNI; one without is held, not
// left out quietly. And a VXLAN fallback can't go to the primary's far end,
// by its tunnel or another to the same address (Griff, 2026-10-06).
func TestVXLANTransportsNeedATunnelAndAnotherFarEnd(t *testing.T) {
	s, sch := site(t)
	set := func(tree change.TreeName, node hierarchy.NodeID, path string, v any) {
		raw, _ := json.Marshal(v)
		if _, _, err := change.Apply(s, change.Op{Kind: change.Set, Tree: tree, Node: node, Path: hierarchy.Path(path), Value: raw}); err != nil {
			t.Fatal(err)
		}
	}
	unset := func(path string) {
		if _, _, err := change.Apply(s, change.Op{Kind: change.Unset, Tree: change.Services, Node: "household", Path: hierarchy.Path(path)}); err != nil {
			t.Fatal(err)
		}
	}
	problems := func() string {
		t.Helper()
		res, err := AP(s, sch, "gate-ap", nil)
		must(t, err)
		return strings.Join(res.Problems, "\n")
	}
	unset("network.sweet.transport.fallback.vlan")
	set(change.Services, "household", "network.sweet.transport.fallback.type", "vxlan")
	set(change.Services, "household", "network.sweet.transport.fallback.vni", 77)
	if p := problems(); !strings.Contains(p, "network.sweet.transport.fallback: a VXLAN transport needs a tunnel and a VNI") {
		t.Fatalf("no tunnel: %s", p)
	}
	set(change.Services, "household", "network.sweet.transport.fallback.concentrator", "homelab")
	if p := problems(); !strings.Contains(p, "network.sweet.transport.fallback: it uses tunnel homelab, as the primary does") {
		t.Fatalf("the primary's tunnel: %s", p)
	}
	// Another tunnel, to the same address.
	set(change.Locations, "gate", "concentrators.twin.address", "1.1.1.2")
	set(change.Locations, "gate", "concentrators.twin.port", 4789)
	set(change.Services, "household", "network.sweet.transport.fallback.concentrator", "twin")
	if p := problems(); !strings.Contains(p, "tunnel twin goes to 1.1.1.2, as the primary's tunnel homelab does") {
		t.Fatalf("the primary's address: %s", p)
	}
	set(change.Locations, "gate", "concentrators.twin.address", "1.1.1.3")
	if p := problems(); strings.Contains(p, "far end") || strings.Contains(p, "needs a tunnel") {
		t.Fatalf("another far end: %s", p)
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

// An AP's config carries its name as its hostname (0076); a name from
// before that isn't a hostname is made into one.
func TestTheAPsNameIsItsHostname(t *testing.T) {
	s, sch := site(t)
	res, err := AP(s, sch, "office-ap", nil)
	must(t, err)
	if got := res.Doc["ap"]; !reflect.DeepEqual(got, map[string]any{"hostname": "OfficeOpenWrt"}) {
		t.Fatalf("ap: %v", got)
	}
	for name, want := range map[string]string{
		"OfficeOpenWrt":         "OfficeOpenWrt",
		"Main Gate AP":          "Main-Gate-AP",
		"--café--":              "caf",
		strings.Repeat("x", 70): strings.Repeat("x", 63),
		"!!!":                   "",
	} {
		if got := Hostname(name); got != want {
			t.Errorf("Hostname(%q) = %q, want %q", name, got, want)
		}
	}
}

// 6 GHz takes WPA3 and OWE only (0086): a network named on 6 GHz that is
// WPA2 or open is a problem; one whose bands are unset is simply not
// offered there, and nothing is said.
func TestSixGHzTakesWPA3OrOWE(t *testing.T) {
	s, sch := site(t)
	set := func(path string, v any) {
		raw, _ := json.Marshal(v)
		if _, _, err := change.Apply(s, change.Op{Kind: change.Set, Tree: change.Services, Node: "household", Path: hierarchy.Path(path), Value: raw}); err != nil {
			t.Fatal(err)
		}
	}
	sixProblems := func() []string {
		t.Helper()
		res, err := AP(s, sch, "gate-ap", nil)
		must(t, err)
		var out []string
		for _, p := range res.Problems {
			if strings.Contains(p, "6 GHz") {
				out = append(out, p)
			}
		}
		return out
	}
	set("network.sweet.security", "wpa2-psk")
	if got := sixProblems(); len(got) != 0 {
		t.Fatalf("bands unset: %v", got)
	}
	set("network.sweet.bands", []string{"5g", "6g"})
	want := "network.sweet: 6 GHz takes WPA3 or OWE only, not wpa2-psk; take 6g out of its bands, or make it wpa3-sae, wpa2-wpa3 or owe"
	if got := sixProblems(); len(got) != 1 || got[0] != want {
		t.Fatalf("WPA2 named on 6 GHz: %v", got)
	}
	for _, ok := range []string{"wpa2-wpa3", "wpa3-sae", "owe"} {
		set("network.sweet.security", ok)
		if got := sixProblems(); len(got) != 0 {
			t.Fatalf("%s on 6 GHz: %v", ok, got)
		}
	}
	set("network.sweet.security", "open")
	if got := sixProblems(); len(got) != 1 {
		t.Fatalf("open named on 6 GHz: %v", got)
	}
}

// 6 GHz channels (0087): with preferred scanning channels only, a channel
// set by hand must be one, and the set must leave one in a whole block.
func TestSixGHzChannels(t *testing.T) {
	s, sch := site(t)
	set := func(path string, v any) {
		raw, _ := json.Marshal(v)
		if _, _, err := change.Apply(s, change.Op{Kind: change.Set, Tree: change.Locations, Node: "gate-ap", Path: hierarchy.Path(path), Value: raw}); err != nil {
			t.Fatal(err)
		}
	}
	problems := func() []string {
		t.Helper()
		res, err := AP(s, sch, "gate-ap", nil)
		must(t, err)
		var out []string
		for _, p := range res.Problems {
			if strings.HasPrefix(p, "radio.6g.") {
				out = append(out, p)
			}
		}
		return out
	}
	set("radio.6g.width", 160)
	set("radio.6g.channel", 1)
	if got := problems(); len(got) != 0 {
		t.Fatalf("channel 1 at 160 MHz: %v", got)
	}
	set("radio.6g.psc", true)
	if got := problems(); len(got) != 1 || !strings.Contains(got[0], "1 is not a preferred scanning channel") {
		t.Fatalf("channel 1, psc: %v", got)
	}
	set("radio.6g.channel", "auto")
	set("radio.6g.non_overlapping", true)
	if got := problems(); len(got) != 0 {
		t.Fatalf("automatic, psc, one to a block: %v", got)
	}
	// 1-13 is no whole 160 MHz block.
	set("radio.6g.channels", []int{1, 5, 9, 13})
	if got := problems(); len(got) != 1 || !strings.Contains(got[0], "no preferred scanning channel is in a whole 160 MHz block") {
		t.Fatalf("a set with no whole block: %v", got)
	}
	set("radio.6g.width", 80)
	if got := problems(); len(got) != 0 {
		t.Fatalf("1-13 at 80 MHz: %v", got)
	}
}
