package hierarchy

import (
	"errors"
	"reflect"
	"testing"
)

// symtus builds the scenario from the UI mockup.
func symtus(t *testing.T) *Org {
	t.Helper()
	o := NewOrg("symtus", "Symtus")
	L, S := o.Locations, o.Services

	must(t, L.AddFolder("house", "House", "symtus"))
	must(t, L.AddFolder("office", "Office", "house"))
	must(t, L.AddAP("office-ap", "OfficeOpenWrt", "office"))
	must(t, L.AddFolder("pump", "Pumphouse", "symtus"))
	must(t, L.AddAP("pump-ap", "PumphouseAP", "pump"))
	must(t, L.AddFolder("gate", "Main Gate", "symtus"))
	must(t, L.AddAP("gate-ap", "GateOpenWrt", "gate"))

	must(t, S.AddFolder("household", "Household", "symtus"))
	must(t, S.AddFolder("household-gate", "Gate", "household"))
	must(t, S.AddFolder("iot", "IoT", "symtus"))
	must(t, S.AddFolder("iot-gate", "Gate", "iot"))
	must(t, S.AddFolder("guest", "Guest", "symtus"))

	setAll(t, L, "symtus", map[Path]Value{
		"radio.2g.width":   "20",
		"radio.5g.channel": "auto",
		"radio.5g.width":   "80",
		"system.poll":      "60",
		"system.tz":        "America/Chicago",
	})
	if _, err := L.Lock("symtus", "system.poll"); err != nil {
		t.Fatal(err)
	}
	must(t, o.AssignServices("symtus", []NodeID{"household", "iot"}))
	must(t, L.Set("house", "radio.5g.width", "40"))
	must(t, L.Set("office-ap", "radio.5g.channel", "48"))
	must(t, o.AssignServices("pump", []NodeID{"iot"}))

	setAll(t, S, "household", map[Path]Value{
		"network.sweet.ssid":    "Sweet Spot",
		"network.sweet.segment": "vlan 20",
		"network.sweet.enabled": true,
	})
	must(t, S.Set("household-gate", "network.sweet.segment", "vxlan vx20"))
	setAll(t, S, "iot", map[Path]Value{
		"network.iot.ssid":    "Sweet_Spot_IoT",
		"network.iot.segment": "vlan 10",
		"network.iot.enabled": true,
	})
	must(t, S.Set("iot-gate", "network.iot.segment", "vxlan vx10"))
	setAll(t, S, "guest", map[Path]Value{
		"network.guest.ssid":      "Guest",
		"network.guest.isolation": true,
		"network.guest.enabled":   false,
	})
	if _, err := S.Lock("guest", "network.guest.isolation"); err != nil {
		t.Fatal(err)
	}
	return o
}

func TestInheritancePerField(t *testing.T) {
	L := symtus(t).Locations
	want(t, L, "office-ap", "radio.5g.channel", Resolved{"48", "office-ap", OriginSelf})
	want(t, L, "office-ap", "radio.5g.width", Resolved{"40", "house", OriginInherited})
	want(t, L, "office-ap", "radio.2g.width", Resolved{"20", "symtus", OriginInherited})
}

func TestChangesAboveFlowPastFieldOverrides(t *testing.T) {
	L := symtus(t).Locations
	must(t, L.Set("symtus", "radio.2g.width", "40"))
	want(t, L, "office-ap", "radio.2g.width", Resolved{"40", "symtus", OriginInherited})
	want(t, L, "office-ap", "radio.5g.channel", Resolved{"48", "office-ap", OriginSelf})
}

func TestLockRefusesOverrideBelow(t *testing.T) {
	L := symtus(t).Locations
	var le *LockedError
	if err := L.Set("house", "system.poll", "30"); !errors.As(err, &le) || le.By != "symtus" {
		t.Fatalf("Set below lock: got %v, want LockedError by symtus", err)
	}
	want(t, L, "office-ap", "system.poll", Resolved{"60", "symtus", OriginLocked})
	want(t, L, "symtus", "system.poll", Resolved{"60", "symtus", OriginLocked})
}

func TestLockRemovesOverridesItWouldHide(t *testing.T) {
	L := symtus(t).Locations
	must(t, L.Set("house", "system.tz", "UTC"))
	removed, err := L.Lock("symtus", "system.tz")
	must(t, err)
	if !reflect.DeepEqual(removed, []Override{{"house", "system.tz", "UTC"}}) {
		t.Fatalf("removed = %v", removed)
	}
	must(t, L.Unlock("symtus", "system.tz"))
	want(t, L, "office-ap", "system.tz", Resolved{"America/Chicago", "symtus", OriginInherited})
}

func TestLockNeedsValueAtSameNode(t *testing.T) {
	L := symtus(t).Locations
	if _, err := L.Lock("house", "system.tz"); !errors.Is(err, ErrLockOnValue) {
		t.Fatalf("got %v, want ErrLockOnValue", err)
	}
}

func TestRevertToInherited(t *testing.T) {
	L := symtus(t).Locations
	must(t, L.Unset("office-ap", "radio.5g.channel"))
	want(t, L, "office-ap", "radio.5g.channel", Resolved{"auto", "symtus", OriginInherited})
	if err := L.Unset("office-ap", "radio.5g.channel"); !errors.Is(err, ErrNotSetHere) {
		t.Fatalf("second Unset: got %v, want ErrNotSetHere", err)
	}
}

func TestBreakHierarchyIsZeroImpact(t *testing.T) {
	L := symtus(t).Locations
	before := values(L.ResolveAll("gate-ap"))
	must(t, L.BreakHierarchy("gate"))
	if after := values(L.ResolveAll("gate-ap")); !reflect.DeepEqual(before, after) {
		t.Fatalf("break changed values:\nbefore %v\nafter  %v", before, after)
	}
	want(t, L, "gate-ap", "radio.5g.width", Resolved{"80", "gate", OriginBaseline})
}

func TestBreakHierarchyEscapesLocks(t *testing.T) {
	L := symtus(t).Locations
	if err := L.Set("gate", "system.poll", "300"); err == nil {
		t.Fatal("Set under the Org lock succeeded before the break")
	}
	must(t, L.BreakHierarchy("gate"))
	must(t, L.Set("gate", "system.poll", "300"))
	want(t, L, "gate-ap", "system.poll", Resolved{"300", "gate", OriginInherited})
	want(t, L, "office-ap", "system.poll", Resolved{"60", "symtus", OriginLocked})
	if got := L.LocksAbove("gate"); len(got) != 0 {
		t.Fatalf("LocksAbove(gate) after break = %v", got)
	}
}

func TestBreakHierarchyStopsInheritance(t *testing.T) {
	L := symtus(t).Locations
	must(t, L.BreakHierarchy("gate"))
	must(t, L.Set("symtus", "radio.2g.width", "40"))
	want(t, L, "office-ap", "radio.2g.width", Resolved{"40", "symtus", OriginInherited})
	want(t, L, "gate-ap", "radio.2g.width", Resolved{"20", "gate", OriginBaseline})
}

func TestBreakKeepsExistingOverridesAsOverrides(t *testing.T) {
	L := symtus(t).Locations
	must(t, L.Set("gate", "radio.5g.width", "20"))
	must(t, L.BreakHierarchy("gate"))
	if got := L.OverridesInEffect("gate"); !reflect.DeepEqual(got, []Override{{"gate", "radio.5g.width", "20"}}) {
		t.Fatalf("OverridesInEffect(gate) = %v", got)
	}
	must(t, L.Unset("gate", "radio.5g.width"))
	want(t, L, "gate", "radio.5g.width", Resolved{"80", "gate", OriginBaseline})
}

func TestBreakHierarchyRules(t *testing.T) {
	L := symtus(t).Locations
	if err := L.BreakHierarchy("symtus"); !errors.Is(err, ErrBreakRoot) {
		t.Fatalf("break root: got %v", err)
	}
	must(t, L.BreakHierarchy("gate"))
	if err := L.BreakHierarchy("gate"); !errors.Is(err, ErrBroken) {
		t.Fatalf("break twice: got %v", err)
	}
}

func TestDisableInheritedNetwork(t *testing.T) {
	S := symtus(t).Services
	must(t, S.Set("household-gate", "network.sweet.enabled", false))
	want(t, S, "household-gate", "network.sweet.enabled", Resolved{false, "household-gate", OriginSelf})
	want(t, S, "household", "network.sweet.enabled", Resolved{true, "household", OriginSelf})
}

func TestOverridesMenu(t *testing.T) {
	o := symtus(t)
	L, S := o.Locations, o.Services

	gotIn := L.OverridesInEffect("office-ap")
	wantIn := []Override{{"office-ap", "radio.5g.channel", "48"}, {"house", "radio.5g.width", "40"}}
	if !reflect.DeepEqual(gotIn, wantIn) {
		t.Fatalf("OverridesInEffect(office-ap) =\n%v\nwant\n%v", gotIn, wantIn)
	}

	gotBelow := L.OverridesBelow("symtus")
	wantBelow := []Override{
		{"house", "radio.5g.width", "40"},
		{"office-ap", "radio.5g.channel", "48"},
		{"pump", ServicesPath, []NodeID{"iot"}},
	}
	if !reflect.DeepEqual(gotBelow, wantBelow) {
		t.Fatalf("OverridesBelow(symtus) =\n%v\nwant\n%v", gotBelow, wantBelow)
	}

	// Networks defined in a folder are definitions, not overrides.
	gotSvc := S.OverridesBelow("symtus")
	wantSvc := []Override{
		{"household-gate", "network.sweet.segment", "vxlan vx20"},
		{"iot-gate", "network.iot.segment", "vxlan vx10"},
	}
	if !reflect.DeepEqual(gotSvc, wantSvc) {
		t.Fatalf("Services OverridesBelow(symtus) =\n%v\nwant\n%v", gotSvc, wantSvc)
	}
}

func TestResolveAPNetworks(t *testing.T) {
	o := symtus(t)

	office, err := o.ResolveAP("office-ap")
	must(t, err)
	if !reflect.DeepEqual(office.Services, []NodeID{"household", "iot"}) {
		t.Fatalf("office services = %v", office.Services)
	}
	if !reflect.DeepEqual(office.NetworkIDs(), []string{"iot", "sweet"}) {
		t.Fatalf("office networks = %v", office.NetworkIDs())
	}
	if _, ok := office.Location[ServicesPath]; ok {
		t.Fatal("the services assignment leaked into the Location fields")
	}
	if got := office.Networks["sweet"].Fields["segment"]; got != (Resolved{"vlan 20", "household", OriginSelf}) {
		t.Fatalf("office sweet segment = %v", got)
	}

	pump, err := o.ResolveAP("pump-ap")
	must(t, err)
	if !reflect.DeepEqual(pump.NetworkIDs(), []string{"iot"}) {
		t.Fatalf("pump networks = %v", pump.NetworkIDs())
	}

	must(t, o.Locations.BreakHierarchy("gate"))
	must(t, o.AssignServices("gate", []NodeID{"household-gate", "iot-gate"}))
	gate, err := o.ResolveAP("gate-ap")
	must(t, err)
	sweet := gate.Networks["sweet"]
	if sweet.From != "household-gate" {
		t.Fatalf("gate sweet from %s", sweet.From)
	}
	if got := sweet.Fields["segment"]; got != (Resolved{"vxlan vx20", "household-gate", OriginSelf}) {
		t.Fatalf("gate sweet segment = %v", got)
	}
	if got := sweet.Fields["ssid"]; got != (Resolved{"Sweet Spot", "household", OriginInherited}) {
		t.Fatalf("gate sweet ssid = %v", got)
	}
}

func TestNetworkConflict(t *testing.T) {
	o := symtus(t)
	must(t, o.AssignServices("pump", []NodeID{"household", "household-gate"}))
	var ce *NetworkConflictError
	if _, err := o.ResolveAP("pump-ap"); !errors.As(err, &ce) || ce.Network != "sweet" {
		t.Fatalf("got %v, want NetworkConflictError for sweet", err)
	}
}

func TestAssignUnknownServiceFolder(t *testing.T) {
	o := symtus(t)
	if err := o.AssignServices("pump", []NodeID{"nope"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestMoveInheritsNewParent(t *testing.T) {
	o := symtus(t)
	removed, err := o.Locations.Move("office", "pump")
	must(t, err)
	if len(removed) != 0 {
		t.Fatalf("removed = %v", removed)
	}
	want(t, o.Locations, "office-ap", "radio.5g.width", Resolved{"80", "symtus", OriginInherited})
	cfg, err := o.ResolveAP("office-ap")
	must(t, err)
	if !reflect.DeepEqual(cfg.Services, []NodeID{"iot"}) {
		t.Fatalf("services after move = %v", cfg.Services)
	}
}

func TestMoveRemovesOverridesUnderNewLocks(t *testing.T) {
	L := symtus(t).Locations
	must(t, L.Set("office-ap", "system.tz", "UTC"))
	must(t, L.Set("pump", "system.tz", "America/Denver"))
	if _, err := L.Lock("pump", "system.tz"); err != nil {
		t.Fatal(err)
	}
	removed, err := L.Move("office", "pump")
	must(t, err)
	if !reflect.DeepEqual(removed, []Override{{"office-ap", "system.tz", "UTC"}}) {
		t.Fatalf("removed = %v", removed)
	}
	want(t, L, "office-ap", "system.tz", Resolved{"America/Denver", "pump", OriginLocked})
}

func TestMoveRules(t *testing.T) {
	L := symtus(t).Locations
	cases := []struct {
		id, parent NodeID
		err        error
	}{
		{"house", "office", ErrCycle},
		{"symtus", "house", ErrMoveRoot},
		{"office", "office-ap", ErrBadParent},
		{"office", "nope", ErrNotFound},
	}
	for _, c := range cases {
		if _, err := L.Move(c.id, c.parent); !errors.Is(err, c.err) {
			t.Errorf("Move(%s, %s) = %v, want %v", c.id, c.parent, err, c.err)
		}
	}
}

func TestServicesTreeHoldsNoAPs(t *testing.T) {
	S := symtus(t).Services
	if err := S.AddAP("x", "X", "household"); !errors.Is(err, ErrAPsNotHere) {
		t.Fatalf("got %v, want ErrAPsNotHere", err)
	}
}

func TestLocksAbove(t *testing.T) {
	L := symtus(t).Locations
	if got := L.LocksAbove("house"); !reflect.DeepEqual(got, []Override{{"symtus", "system.poll", "60"}}) {
		t.Fatalf("LocksAbove(house) = %v", got)
	}
}

func TestCloneIsIndependent(t *testing.T) {
	o := symtus(t)
	c := o.Clone()
	must(t, c.Locations.Set("house", "radio.5g.width", "20"))
	must(t, c.Locations.BreakHierarchy("gate"))
	must(t, c.Locations.AddFolder("barn", "Barn", "symtus"))
	want(t, o.Locations, "office-ap", "radio.5g.width", Resolved{"40", "house", OriginInherited})
	if n, _ := o.Locations.Node("gate"); n.Broken {
		t.Fatal("break on the clone reached the original")
	}
	if _, ok := o.Locations.Node("barn"); ok {
		t.Fatal("folder added to the clone reached the original")
	}
}

func TestAPsOwnAndIsLocked(t *testing.T) {
	L := symtus(t).Locations
	if got := L.APs(); !reflect.DeepEqual(got, []NodeID{"gate-ap", "office-ap", "pump-ap"}) {
		t.Fatalf("APs = %v", got)
	}
	if v, ok := L.Own("house", "radio.5g.width"); !ok || v != "40" {
		t.Fatalf("Own(house) = %v, %v", v, ok)
	}
	if _, ok := L.Own("office", "radio.5g.width"); ok {
		t.Fatal("Own reported an inherited value")
	}
	if !L.IsLocked("symtus", "system.poll") || L.IsLocked("house", "system.poll") {
		t.Fatal("IsLocked wrong")
	}
}

func TestAncestryIgnoresBreaks(t *testing.T) {
	L := symtus(t).Locations
	must(t, L.BreakHierarchy("gate"))
	if got := L.Ancestry("gate-ap"); !reflect.DeepEqual(got, []NodeID{"symtus", "gate", "gate-ap"}) {
		t.Fatalf("Ancestry(gate-ap) = %v", got)
	}
	if got := L.Ancestry("nope"); got != nil {
		t.Fatalf("Ancestry(nope) = %v", got)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func setAll(t *testing.T, tr *Tree, id NodeID, vals map[Path]Value) {
	t.Helper()
	for p, v := range vals {
		must(t, tr.Set(id, p, v))
	}
}

func want(t *testing.T, tr *Tree, id NodeID, p Path, w Resolved) {
	t.Helper()
	got, ok := tr.Resolve(id, p)
	if !ok {
		t.Fatalf("%s %s: not resolved", id, p)
	}
	if !reflect.DeepEqual(got, w) {
		t.Fatalf("%s %s = %+v, want %+v", id, p, got, w)
	}
}

func values(m map[Path]Resolved) map[Path]Value {
	out := map[Path]Value{}
	for p, r := range m {
		out[p] = r.Value
	}
	return out
}
