package change

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// people builds an Org with a few accounts and roles:
//   - griff:  admin at both roots (from create-org)
//   - claude: operator at both roots
//   - office: operator on Services › Household, viewer at the Services root
//   - gatekeeper: admin on Locations › Main Gate only
func people(t *testing.T) *State {
	t.Helper()
	s, _, err := Apply(nil, Op{Kind: CreateOrg, Node: "symtus", Name: "Symtus", Account: "griff"})
	must(t, err)
	L, S := Locations, Services
	for _, op := range []Op{
		{Kind: AddFolder, Tree: L, Node: "house", Name: "House", Parent: "symtus"},
		{Kind: AddFolder, Tree: L, Node: "gate", Name: "Main Gate", Parent: "symtus"},
		{Kind: AddAP, Tree: L, Node: "gate-ap", Name: "GateOpenWrt", Parent: "gate"},
		{Kind: AddFolder, Tree: S, Node: "household", Name: "Household", Parent: "symtus"},
		{Kind: AddFolder, Tree: S, Node: "guest", Name: "Guest", Parent: "symtus"},
		{Kind: Set, Tree: L, Node: "symtus", Path: "system.poll", Value: json.RawMessage(`60`)},
		{Kind: Lock, Tree: L, Node: "symtus", Path: "system.poll"},
		{Kind: AddAccount, Account: "claude", Name: "Claude"},
		{Kind: AddAccount, Account: "office", Name: "Leasing office"},
		{Kind: AddAccount, Account: "gatekeeper", Name: "Gate admin"},
		{Kind: GrantRole, Account: "claude", Tree: L, Node: "symtus", Role: "operator"},
		{Kind: GrantRole, Account: "claude", Tree: S, Node: "symtus", Role: "operator"},
		{Kind: GrantRole, Account: "office", Tree: S, Node: "household", Role: "operator"},
		{Kind: GrantRole, Account: "office", Tree: S, Node: "symtus", Role: "viewer"},
		{Kind: GrantRole, Account: "gatekeeper", Tree: L, Node: "gate", Role: "admin"},
	} {
		mustApply(t, s, op)
	}
	return s
}

func TestAuthorize(t *testing.T) {
	s := people(t)
	L, S := Locations, Services
	set := func(tree TreeName, node hierarchy.NodeID, path hierarchy.Path) Op {
		return Op{Kind: Set, Tree: tree, Node: node, Path: path, Value: json.RawMessage(`1`)}
	}
	cases := []struct {
		name  string
		actor string
		op    Op
		ok    bool
	}{
		{"operator sets below its grant", "claude", set(L, "house", "radio.2g.width"), true},
		{"no role in this tree", "office", set(L, "house", "radio.2g.width"), false},
		{"operator on its folder", "office", set(S, "household", "network.sweet.ssid"), true},
		{"viewer cannot set", "office", set(S, "guest", "network.guest.ssid"), false},
		{"operator cannot lock", "claude", Op{Kind: Lock, Tree: L, Node: "house", Path: "system.tz"}, false},
		{"admin locks", "griff", Op{Kind: Lock, Tree: L, Node: "house", Path: "system.tz"}, true},
		{"break needs admin where the escaped lock is", "gatekeeper", Op{Kind: BreakHierarchy, Tree: L, Node: "gate"}, false},
		{"org admin breaks", "griff", Op{Kind: BreakHierarchy, Tree: L, Node: "gate"}, true},
		{"add under a folder needs operator on it", "office", Op{Kind: AddFolder, Tree: S, Node: "b", Name: "B", Parent: "household"}, true},
		{"move needs operator on the new parent too", "office", Op{Kind: Move, Tree: S, Node: "household", Parent: "guest"}, false},
		{"assign services needs Locations operator", "office", Op{Kind: AssignServices, Node: "house", Services: []hierarchy.NodeID{"household"}}, false},
		{"assign services with both roles", "claude", Op{Kind: AssignServices, Node: "house", Services: []hierarchy.NodeID{"household"}}, true},
		{"add account needs admin somewhere", "claude", Op{Kind: AddAccount, Account: "x", Name: "X"}, false},
		{"a folder admin may add accounts", "gatekeeper", Op{Kind: AddAccount, Account: "x", Name: "X"}, true},
		{"grant inside own admin folder", "gatekeeper", Op{Kind: GrantRole, Account: "office", Tree: L, Node: "gate", Role: "viewer"}, true},
		{"grant outside own admin folder", "gatekeeper", Op{Kind: GrantRole, Account: "office", Tree: L, Node: "house", Role: "viewer"}, false},
		{"own token", "claude", Op{Kind: IssueToken, Account: "claude", TokenID: "t1"}, true},
		{"token for someone else", "claude", Op{Kind: IssueToken, Account: "office", TokenID: "t2"}, false},
		{"org admin issues for someone else", "griff", Op{Kind: IssueToken, Account: "office", TokenID: "t3"}, true},
		{"unknown actor", "mallory", set(L, "house", "radio.2g.width"), false},
		{"create-org must name its caller", "claude", Op{Kind: CreateOrg, Node: "x", Name: "X", Account: "griff"}, false},
	}
	for _, c := range cases {
		err := Authorize(s, c.actor, c.op)
		if (err == nil) != c.ok {
			t.Errorf("%s: Authorize(%s, %s) = %v, want ok=%v", c.name, c.actor, c.op.Kind, err, c.ok)
		}
		if err != nil && !errors.Is(err, ErrForbidden) && !errors.Is(err, ErrUnknownActor) {
			t.Errorf("%s: unexpected error kind %v", c.name, err)
		}
	}
}

func TestBreakWithoutLocksAboveNeedsOnlyTheFolder(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: Unlock, Tree: Locations, Node: "symtus", Path: "system.poll"})
	if err := Authorize(s, "gatekeeper", Op{Kind: BreakHierarchy, Tree: Locations, Node: "gate"}); err != nil {
		t.Fatalf("break with no locks above: %v", err)
	}
}

func TestRevokingTokens(t *testing.T) {
	s := people(t)
	_, id, hash, err := access.NewToken()
	must(t, err)
	mustApply(t, s, Op{Kind: IssueToken, Account: "office", TokenID: id, TokenHash: hash})
	if err := Authorize(s, "office", Op{Kind: RevokeToken, TokenID: id}); err != nil {
		t.Fatalf("revoking own token: %v", err)
	}
	if err := Authorize(s, "claude", Op{Kind: RevokeToken, TokenID: id}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("revoking someone else's token: %v", err)
	}
	mustApply(t, s, Op{Kind: RevokeToken, TokenID: id})
	if _, _, err := Apply(s, Op{Kind: RevokeToken, TokenID: id}); !errors.Is(err, access.ErrRevoked) {
		t.Fatalf("second revoke: %v", err)
	}
}

func TestAccessOpsApply(t *testing.T) {
	s := people(t)
	if _, _, err := Apply(s, Op{Kind: GrantRole, Account: "office", Tree: Locations, Node: "gate-ap", Role: "viewer"}); !errors.Is(err, ErrNotAFolder) {
		t.Fatalf("grant on an AP: %v", err)
	}
	if _, _, err := Apply(s, Op{Kind: GrantRole, Account: "office", Tree: Locations, Node: "gate", Role: "owner"}); !errors.Is(err, access.ErrBadRole) {
		t.Fatalf("unknown role: %v", err)
	}
	if _, _, err := Apply(s, Op{Kind: AddAccount, Account: "claude", Name: "again"}); !errors.Is(err, access.ErrExists) {
		t.Fatalf("duplicate account: %v", err)
	}
	if _, _, err := Apply(s, Op{Kind: IssueToken, Account: "ghost", TokenID: "t", TokenHash: make([]byte, 32)}); !errors.Is(err, access.ErrNoAccount) {
		t.Fatalf("token for unknown account: %v", err)
	}
	mustApply(t, s, Op{Kind: RevokeRole, Account: "office", Tree: Services, Node: "household", Role: "operator"})
	if err := Authorize(s, "office", Op{Kind: Set, Tree: Services, Node: "household", Path: "network.sweet.ssid", Value: json.RawMessage(`"x"`)}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("set after revoke: %v", err)
	}
}

func TestCreateOrgMakesItsCallerAdminOfBothTrees(t *testing.T) {
	s, _, err := Apply(nil, Op{Kind: CreateOrg, Node: "symtus", Name: "Symtus", Account: "griff"})
	must(t, err)
	for _, tree := range []string{"locations", "services"} {
		if r := s.Access.RoleAt("griff", tree, []hierarchy.NodeID{"symtus"}); r != access.Admin {
			t.Errorf("griff in %s = %v, want admin", tree, r)
		}
	}
	if _, _, err := Apply(nil, Op{Kind: CreateOrg, Node: "symtus", Name: "Symtus"}); !errors.Is(err, ErrNoAccount) {
		t.Fatalf("create-org without an admin: %v", err)
	}
}

func TestBuiltins(t *testing.T) {
	s := people(t)
	_, eff, err := Apply(s, Op{Kind: AddBuiltins})
	must(t, err)
	if !reflect.DeepEqual(eff.After, []string{"locations/landing-zone", "locations/sandbox", "services/sandbox"}) {
		t.Fatalf("created %v", eff.After)
	}
	if n, _ := s.Org.Locations.Node(LandingZone); !n.Isolated {
		t.Fatal("Landing Zone is not isolated")
	}
	if n, _ := s.Org.Locations.Node(Sandbox); n.Isolated {
		t.Fatal("Sandbox is isolated")
	}
	if _, _, err := Apply(s, Op{Kind: AddBuiltins}); !errors.Is(err, ErrBuiltins) {
		t.Fatalf("second add-builtins: %v", err)
	}
}

func TestBuiltinsRefuseALookalike(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddFolder, Tree: Services, Node: Sandbox, Name: "My sandbox", Parent: "household"})
	if _, _, err := Apply(s, Op{Kind: AddBuiltins}); !errors.Is(err, hierarchy.ErrExists) {
		t.Fatalf("add-builtins over a lookalike: %v", err)
	}
}

func TestTheManagerMayDoOnlyTwoThings(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	allowed := []Op{
		{Kind: AddBuiltins},
		{Kind: Enroll, Node: "new-ap", Name: "NewAP", TokenID: "t1", TokenHash: make([]byte, 32)},
	}
	for _, op := range allowed {
		if err := Authorize(s, SystemActor, op); err != nil {
			t.Errorf("%s: %v", op.Kind, err)
		}
	}
	refused := []Op{
		{Kind: AddAP, Tree: Locations, Node: "x", Name: "X", Parent: LandingZone},
		{Kind: AddAP, Tree: Locations, Node: "x", Name: "X", Parent: "house"},
		{Kind: RemoveAP, Node: "gate-ap"},
		{Kind: Set, Tree: Locations, Node: "house", Path: "system.tz", Value: json.RawMessage(`"UTC"`)},
		{Kind: AddAccount, Account: "x", Name: "X"},
		{Kind: GrantRole, Account: "claude", Tree: Locations, Node: "symtus", Role: "admin"},
	}
	for _, op := range refused {
		if err := Authorize(s, SystemActor, op); !errors.Is(err, ErrForbidden) {
			t.Errorf("%s: got %v, want ErrForbidden", op.Kind, err)
		}
	}
	if _, _, err := Apply(s, Op{Kind: AddAccount, Account: SystemActor, Name: "impostor"}); !errors.Is(err, ErrReserved) {
		t.Fatalf("an account named aeolus: %v", err)
	}
	if err := Authorize(s, "claude", Op{Kind: AddBuiltins}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("operator adding built-ins: %v", err)
	}
}

func TestAdoptionIsDecidedByTheDestination(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	mustApply(t, s, Op{Kind: AddAP, Tree: Locations, Node: "new-ap", Name: "NewAP", Parent: LandingZone})
	for _, op := range []Op{
		{Kind: AddAccount, Account: "house-op", Name: "House operator"},
		{Kind: GrantRole, Account: "house-op", Tree: Locations, Node: "house", Role: "operator"},
		{Kind: AddAccount, Account: "house-adopter", Name: "House adopter"},
		{Kind: GrantRole, Account: "house-adopter", Tree: Locations, Node: "house", Role: "operator"},
		{Kind: GrantRole, Account: "house-adopter", Tree: Locations, Node: LandingZone, Role: "viewer"},
	} {
		mustApply(t, s, op)
	}
	adopt := func(dest hierarchy.NodeID) Op {
		return Op{Kind: Move, Tree: Locations, Node: "new-ap", Parent: dest}
	}
	cases := []struct {
		actor string
		op    Op
		ok    bool
	}{
		{"house-op", adopt("house"), false},     // cannot see Landing Zone
		{"house-adopter", adopt("house"), true}, // sees it, operates the destination
		{"house-adopter", adopt("gate"), false}, // no role on the destination
		{"claude", adopt("gate"), true},         // operator at the root
		{"gatekeeper", adopt("gate"), false},    // admin on gate, but cannot see Landing Zone
	}
	for _, c := range cases {
		err := Authorize(s, c.actor, c.op)
		if (err == nil) != c.ok {
			t.Errorf("%s adopting into %s: %v, want ok=%v", c.actor, c.op.Parent, err, c.ok)
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
