package change

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// enroll returns an enroll change for ap and the plain token it records.
func enroll(t *testing.T, ap hierarchy.NodeID) (Op, string) {
	t.Helper()
	plain, id, hash, err := access.NewAPToken()
	must(t, err)
	return Op{Kind: Enroll, Node: ap, Name: string(ap), TokenID: id, TokenHash: hash, Value: json.RawMessage(`{"mac":"02:00:00:00:00:01"}`)}, plain
}

func TestEnrollLandsInLandingZoneWithAToken(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	op, plain := enroll(t, "ap-020000000001")
	mustApply(t, s, op)

	if n, _ := s.Org.Locations.Node("ap-020000000001"); n.Parent != LandingZone || n.Kind != hierarchy.KindAP {
		t.Fatalf("enrolled AP = %+v", n)
	}
	if ap, err := s.Access.AuthenticateAP(plain); err != nil || ap != "ap-020000000001" {
		t.Fatalf("AuthenticateAP = %q, %v", ap, err)
	}
	if string(s.Facts["ap-020000000001"]) != `{"mac":"02:00:00:00:00:01"}` {
		t.Fatalf("facts = %s", s.Facts["ap-020000000001"])
	}
	if _, err := s.Access.Authenticate(plain); err == nil {
		t.Fatal("an AP token worked as an account token")
	}
}

func TestAKnownAPCannotEnrollAgain(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	first, plain := enroll(t, "ap-1")
	mustApply(t, s, first)
	again, _ := enroll(t, "ap-1")
	if _, _, err := Apply(s, again); !errors.Is(err, hierarchy.ErrExists) {
		t.Fatalf("second enrollment: %v", err)
	}
	if _, err := s.Access.AuthenticateAP(plain); err != nil {
		t.Fatalf("the first token stopped working: %v", err)
	}
	// Nor can an AP enroll in the name of one that was adopted.
	mustApply(t, s, Op{Kind: Move, Tree: Locations, Node: "ap-1", Parent: "house"})
	if _, _, err := Apply(s, again); !errors.Is(err, hierarchy.ErrExists) {
		t.Fatalf("enrolling over an adopted AP: %v", err)
	}
}

func TestAFailedEnrollmentLeavesNoAP(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	first, _ := enroll(t, "ap-1")
	mustApply(t, s, first)
	clash, _ := enroll(t, "ap-2")
	clash.TokenID = first.TokenID
	if _, _, err := Apply(s, clash); !errors.Is(err, access.ErrExists) {
		t.Fatalf("token ID clash: %v", err)
	}
	if _, ok := s.Org.Locations.Node("ap-2"); ok {
		t.Fatal("the AP stayed after its token was refused")
	}
}

func TestRemoveAPRevokesItsToken(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	op, plain := enroll(t, "ap-1")
	mustApply(t, s, op)
	mustApply(t, s, Op{Kind: Move, Tree: Locations, Node: "ap-1", Parent: "house"})
	mustApply(t, s, Op{Kind: Set, Tree: Locations, Node: "ap-1", Path: "radio.5g.channel", Value: json.RawMessage(`"36"`)})

	_, eff, err := Apply(s, Op{Kind: RemoveAP, Node: "ap-1"})
	must(t, err)
	if len(eff.Removed) != 1 || eff.Removed[0].Path != "radio.5g.channel" {
		t.Fatalf("effect = %+v", eff)
	}
	if _, ok := s.Org.Locations.Node("ap-1"); ok {
		t.Fatal("AP still in the tree")
	}
	if _, err := s.Access.AuthenticateAP(plain); !errors.Is(err, access.ErrBadToken) {
		t.Fatalf("token after removal: %v", err)
	}
	if _, ok := s.Facts["ap-1"]; ok {
		t.Fatal("facts kept after removal")
	}
	// Removed, it may enroll again, into Landing Zone.
	again, _ := enroll(t, "ap-1")
	mustApply(t, s, again)
	if _, _, err := Apply(s, Op{Kind: RemoveAP, Node: "house"}); !errors.Is(err, hierarchy.ErrNotAnAP) {
		t.Fatalf("removing a folder: %v", err)
	}
}

func TestWhoMayEnrollAndRemove(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	op, _ := enroll(t, "ap-1")
	if err := Authorize(s, "griff", op); !errors.Is(err, ErrForbidden) {
		t.Fatalf("an account enrolling an AP: %v", err)
	}
	remove := Op{Kind: RemoveAP, Node: "gate-ap"}
	// gatekeeper is admin on Main Gate, where the AP is; office has no role
	// in Locations.
	for actor, ok := range map[string]bool{"gatekeeper": true, "office": false, "claude": true, "griff": true} {
		if err := Authorize(s, actor, remove); (err == nil) != ok {
			t.Errorf("%s removing gate-ap: %v, want ok=%v", actor, err, ok)
		}
	}
}

func TestLandingZoneHasALimit(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	for i := 0; i < LandingZoneLimit; i++ {
		mustApply(t, s, Op{Kind: AddAP, Tree: Locations, Node: hierarchy.NodeID(fmt.Sprintf("ap-%d", i)), Parent: LandingZone})
	}
	op, _ := enroll(t, "ap-one-more")
	if err := Authorize(s, SystemActor, op); !errors.Is(err, ErrFull) {
		t.Fatalf("enrolling into a full Landing Zone: %v", err)
	}
	mustApply(t, s, Op{Kind: RemoveAP, Node: "ap-0"})
	if err := Authorize(s, SystemActor, op); err != nil {
		t.Fatalf("enrolling after one was removed: %v", err)
	}
}
