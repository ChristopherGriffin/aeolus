package change

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// A folder that holds nothing can be taken away (0125), with what was set
// at it and the roles granted on it; one that holds something, or that
// something leans on, stays.
func TestRemoveFolder(t *testing.T) {
	var s *State
	do := func(op Op) (Effect, error) {
		t.Helper()
		var in *State
		if s != nil {
			in = s.Clone()
		}
		next, eff, err := Apply(in, op)
		if err == nil {
			s = next
		}
		return eff, err
	}
	ok := func(op Op) Effect {
		t.Helper()
		eff, err := do(op)
		if err != nil {
			t.Fatalf("%+v: %v", op, err)
		}
		return eff
	}
	raw := func(v any) json.RawMessage {
		b, _ := json.Marshal(v)
		return b
	}
	ok(Op{Kind: CreateOrg, Node: "symtus", Name: "Symtus", Account: "griff"})
	ok(Op{Kind: AddBuiltins})
	ok(Op{Kind: AddFolder, Tree: Locations, Node: "house", Name: "House", Parent: "symtus"})
	ok(Op{Kind: AddFolder, Tree: Locations, Node: "spare", Name: "New office", Parent: "house"})
	ok(Op{Kind: AddAP, Tree: Locations, Node: "gate-ap", Name: "gate", Parent: "house"})
	ok(Op{Kind: AddFolder, Tree: Services, Node: "lab", Name: "PPSK lab (temporary)", Parent: "symtus"})
	ok(Op{Kind: AddFolder, Tree: Services, Node: "home", Name: "Home", Parent: "symtus"})
	ok(Op{Kind: Set, Tree: Locations, Node: "spare", Path: "radio.5g.width", Value: raw(40)})
	ok(Op{Kind: AddAccount, Account: "tenant", Name: "Tenant"})
	ok(Op{Kind: GrantRole, Account: "tenant", Tree: Locations, Node: "spare", Role: "viewer"})
	ok(Op{Kind: AssignServices, Node: "house", Services: []hierarchy.NodeID{"home"}})

	// What stays: a folder that holds something, the Org, the built-in
	// folders, an AP, a Services folder a Locations node offers.
	for name, c := range map[string]struct {
		op   Op
		want error
	}{
		"a folder with an AP in it":   {Op{Kind: RemoveFolder, Tree: Locations, Node: "house"}, hierarchy.ErrNotEmpty},
		"the Org":                     {Op{Kind: RemoveFolder, Tree: Locations, Node: "symtus"}, hierarchy.ErrNotAFolder},
		"Landing Zone":                {Op{Kind: RemoveFolder, Tree: Locations, Node: LandingZone}, hierarchy.ErrNotAFolder},
		"Sandbox":                     {Op{Kind: RemoveFolder, Tree: Services, Node: Sandbox}, hierarchy.ErrNotAFolder},
		"an AP":                       {Op{Kind: RemoveFolder, Tree: Locations, Node: "gate-ap"}, hierarchy.ErrNotAFolder},
		"a folder there is none of":   {Op{Kind: RemoveFolder, Tree: Locations, Node: "nowhere"}, hierarchy.ErrNotFound},
		"a Services folder on offer":  {Op{Kind: RemoveFolder, Tree: Services, Node: "home"}, ErrInUse},
		"a folder in the wrong tree":  {Op{Kind: RemoveFolder, Tree: Services, Node: "spare"}, hierarchy.ErrNotFound},
		"a folder with no tree named": {Op{Kind: RemoveFolder, Node: "spare"}, ErrUnknownTree},
	} {
		if _, err := do(c.op); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}

	// An empty one goes, with what was set at it and the role granted on it.
	eff := ok(Op{Kind: RemoveFolder, Tree: Locations, Node: "spare"})
	if len(eff.Removed) != 1 || eff.Removed[0].Path != "radio.5g.width" || eff.Before.(map[string]string)["name"] != "New office" {
		t.Fatalf("effect = %+v", eff)
	}
	if _, there := s.Org.Locations.Node("spare"); there {
		t.Fatal("the folder is still there")
	}
	for _, g := range s.Access.Grants() {
		if g.Node == "spare" {
			t.Fatalf("a grant on it is left: %+v", g)
		}
	}
	if got := s.Access.RoleAt("tenant", string(Locations), []hierarchy.NodeID{"symtus", "house"}); got != access.None {
		t.Fatalf("the tenant's role = %v", got)
	}
	ok(Op{Kind: RemoveFolder, Tree: Services, Node: "lab"})
	// Its name can be used again.
	ok(Op{Kind: AddFolder, Tree: Locations, Node: "spare", Name: "Spare", Parent: "house"})
}
