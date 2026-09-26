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
	o, _, err := Apply(nil, Op{Kind: CreateOrg, Node: "symtus", Name: "Symtus"})
	if err != nil || o == nil {
		t.Fatalf("create-org: %v", err)
	}
	if _, _, err := Apply(o, Op{Kind: CreateOrg, Node: "other", Name: "Other"}); !errors.Is(err, ErrOrgExists) {
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

func org(t *testing.T) *hierarchy.Org {
	t.Helper()
	o, _, err := Apply(nil, Op{Kind: CreateOrg, Node: "symtus", Name: "Symtus"})
	if err != nil {
		t.Fatal(err)
	}
	mustApply(t, o, Op{Kind: AddFolder, Tree: Locations, Node: "house", Name: "House", Parent: "symtus"})
	mustApply(t, o, Op{Kind: AddFolder, Tree: Services, Node: "household", Name: "Household", Parent: "symtus"})
	return o
}

func mustApply(t *testing.T, o *hierarchy.Org, op Op) {
	t.Helper()
	if _, _, err := Apply(o, op); err != nil {
		t.Fatalf("%+v: %v", op, err)
	}
}
