package change

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

func TestFirstChangeCreatesTheOrg(t *testing.T) {
	if _, _, err := Apply(nil, Op{Kind: AddFolder, Tree: Locations, Node: "house", Name: "House", Parent: "symtus"}); !errors.Is(err, ErrNoOrg) {
		t.Fatalf("got %v, want ErrNoOrg", err)
	}
	o, _, err := Apply(nil, Op{Kind: CreateOrg, Node: "symtus", Name: "Symtus", Account: "griff"})
	if err != nil || o == nil {
		t.Fatalf("create-org: %v", err)
	}
	if _, _, err := Apply(o, Op{Kind: CreateOrg, Node: "other", Name: "Other", Account: "griff"}); !errors.Is(err, ErrOrgExists) {
		t.Fatalf("second create-org: got %v", err)
	}
}

func TestSetDecodesJSONAndRecordsBeforeAfter(t *testing.T) {
	o := org(t)
	_, eff, err := Apply(o, Op{Kind: Set, Tree: Locations, Node: "symtus", Path: "radio.5g.width", Value: json.RawMessage(`80`)})
	if err != nil {
		t.Fatal(err)
	}
	if eff.Before != nil || eff.After != float64(80) {
		t.Fatalf("effect = %+v", eff)
	}
	_, eff, err = Apply(o, Op{Kind: Set, Tree: Locations, Node: "symtus", Path: "radio.5g.width", Value: json.RawMessage(`40`)})
	if err != nil || eff.Before != float64(80) || eff.After != float64(40) {
		t.Fatalf("second set: effect %+v, err %v", eff, err)
	}
}

func TestSetValuesAllOrNone(t *testing.T) {
	o := org(t)
	mustApply(t, o, Op{Kind: Set, Tree: Locations, Node: "house", Path: "radio.5g.width", Value: json.RawMessage(`40`)})
	_, eff, err := Apply(o, Op{Kind: Set, Tree: Locations, Node: "house", Values: map[hierarchy.Path]json.RawMessage{
		"radio.5g.width": json.RawMessage(`160`), "radio.5g.channel": json.RawMessage(`36`)}})
	if err != nil {
		t.Fatal(err)
	}
	want := Effect{Before: map[string]any{"radio.5g.width": float64(40)}, After: map[string]any{"radio.5g.width": float64(160), "radio.5g.channel": float64(36)}}
	if !reflect.DeepEqual(eff, want) {
		t.Fatalf("effect = %+v", eff)
	}

	// With the width locked above, neither field changes: the channel,
	// set first in path order, is put back.
	mustApply(t, o, Op{Kind: Set, Tree: Locations, Node: "symtus", Path: "radio.5g.width", Value: json.RawMessage(`80`)})
	mustApply(t, o, Op{Kind: Lock, Tree: Locations, Node: "symtus", Path: "radio.5g.width"})
	_, _, err = Apply(o, Op{Kind: Set, Tree: Locations, Node: "house", Values: map[hierarchy.Path]json.RawMessage{
		"radio.5g.width": json.RawMessage(`40`), "radio.5g.channel": json.RawMessage(`44`)}})
	var locked *hierarchy.LockedError
	if !errors.As(err, &locked) {
		t.Fatalf("got %v, want a lock error", err)
	}
	if v, _ := o.Org.Locations.Own("house", "radio.5g.channel"); v != float64(36) {
		t.Fatalf("channel = %v, want 36 put back", v)
	}
	if _, _, err := Apply(o, Op{Kind: Set, Tree: Locations, Node: "house", Path: "radio.5g.width", Value: json.RawMessage(`40`),
		Values: map[hierarchy.Path]json.RawMessage{"radio.5g.channel": json.RawMessage(`44`)}}); !errors.Is(err, ErrTwoForms) {
		t.Fatalf("both forms: %v", err)
	}
}

func TestUnsetPathsAllOrNone(t *testing.T) {
	o := org(t)
	mustApply(t, o, Op{Kind: Set, Tree: Locations, Node: "house", Values: map[hierarchy.Path]json.RawMessage{
		"radio.5g.width": json.RawMessage(`160`), "radio.5g.channel": json.RawMessage(`"auto"`)}})

	// One path not set here: nothing is unset.
	_, _, err := Apply(o, Op{Kind: Unset, Tree: Locations, Node: "house", Paths: []hierarchy.Path{"radio.5g.width", "radio.2g.width"}})
	if !errors.Is(err, hierarchy.ErrNotSetHere) {
		t.Fatalf("got %v, want ErrNotSetHere", err)
	}
	if _, ok := o.Org.Locations.Own("house", "radio.5g.width"); !ok {
		t.Fatal("width was unset although the change failed")
	}

	_, eff, err := Apply(o, Op{Kind: Unset, Tree: Locations, Node: "house", Paths: []hierarchy.Path{"radio.5g.width", "radio.5g.channel"}})
	if err != nil {
		t.Fatal(err)
	}
	if want := (Effect{Before: map[string]any{"radio.5g.width": float64(160), "radio.5g.channel": "auto"}}); !reflect.DeepEqual(eff, want) {
		t.Fatalf("effect = %+v", eff)
	}
	if _, ok := o.Org.Locations.Own("house", "radio.5g.channel"); ok {
		t.Fatal("channel still set")
	}
	if _, _, err := Apply(o, Op{Kind: Unset, Tree: Locations, Node: "house", Path: "radio.5g.width", Paths: []hierarchy.Path{"radio.5g.channel"}}); !errors.Is(err, ErrTwoForms) {
		t.Fatalf("both forms: %v", err)
	}
}

func TestLockReportsRemovedOverrides(t *testing.T) {
	o := org(t)
	mustApply(t, o, Op{Kind: Set, Tree: Locations, Node: "symtus", Path: "system.tz", Value: json.RawMessage(`"America/Chicago"`)})
	mustApply(t, o, Op{Kind: Set, Tree: Locations, Node: "house", Path: "system.tz", Value: json.RawMessage(`"UTC"`)})
	_, eff, err := Apply(o, Op{Kind: Lock, Tree: Locations, Node: "symtus", Path: "system.tz"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(eff.Removed, []hierarchy.Override{{Node: "house", Path: "system.tz", Value: "UTC"}}) {
		t.Fatalf("removed = %v", eff.Removed)
	}
}

func TestAssignServicesIsTheOnlyWayToSetServices(t *testing.T) {
	o := org(t)
	_, _, err := Apply(o, Op{Kind: Set, Tree: Locations, Node: "house", Path: hierarchy.ServicesPath, Value: json.RawMessage(`["household"]`)})
	if !errors.Is(err, ErrUseAssign) {
		t.Fatalf("got %v, want ErrUseAssign", err)
	}
	_, eff, err := Apply(o, Op{Kind: AssignServices, Node: "house", Services: []hierarchy.NodeID{"household"}})
	if err != nil || !reflect.DeepEqual(eff.After, []hierarchy.NodeID{"household"}) {
		t.Fatalf("assign: effect %+v, err %v", eff, err)
	}
}

func TestBadOps(t *testing.T) {
	o := org(t)
	cases := []struct {
		op   Op
		want error
	}{
		{Op{Kind: "rename", Tree: Locations, Node: "house"}, ErrUnknownKind},
		{Op{Kind: AddFolder, Tree: "sites", Node: "x", Name: "X", Parent: "symtus"}, ErrUnknownTree},
		{Op{Kind: AddFolder, Tree: Locations, Name: "X", Parent: "symtus"}, ErrNoNode},
		{Op{Kind: Set, Tree: Locations, Node: "house", Value: json.RawMessage(`1`)}, ErrNoPath},
		{Op{Kind: Set, Tree: Locations, Node: "house", Path: "a"}, ErrNoValue},
		{Op{Kind: Move, Tree: Locations, Node: "nope", Parent: "symtus"}, hierarchy.ErrNotFound},
	}
	for _, c := range cases {
		if _, _, err := Apply(o, c.op); !errors.Is(err, c.want) {
			t.Errorf("%+v: got %v, want %v", c.op, err, c.want)
		}
	}
}

func org(t *testing.T) *State {
	t.Helper()
	o, _, err := Apply(nil, Op{Kind: CreateOrg, Node: "symtus", Name: "Symtus", Account: "griff"})
	if err != nil {
		t.Fatal(err)
	}
	mustApply(t, o, Op{Kind: AddFolder, Tree: Locations, Node: "house", Name: "House", Parent: "symtus"})
	mustApply(t, o, Op{Kind: AddFolder, Tree: Services, Node: "household", Name: "Household", Parent: "symtus"})
	return o
}

func mustApply(t *testing.T, o *State, op Op) {
	t.Helper()
	if _, _, err := Apply(o, op); err != nil {
		t.Fatalf("%+v: %v", op, err)
	}
}
