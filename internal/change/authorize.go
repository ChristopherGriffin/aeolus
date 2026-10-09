package change

import (
	"errors"
	"fmt"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

var (
	ErrUnknownActor = errors.New("the actor has no account")
	ErrForbidden    = errors.New("not permitted")
)

// ForbiddenError says which role the actor lacked, and where.
type ForbiddenError struct {
	Actor string
	Need  access.Role
	Tree  TreeName
	Node  hierarchy.NodeID
}

func (e *ForbiddenError) Error() string {
	if e.Node == "" {
		return fmt.Sprintf("%s: %v: needs %s", e.Actor, ErrForbidden, e.Need)
	}
	return fmt.Sprintf("%s: %v: needs %s on %s in %s", e.Actor, ErrForbidden, e.Need, e.Node, e.Tree)
}

func (e *ForbiddenError) Unwrap() error { return ErrForbidden }

// Authorize decides whether actor may make op, given the current state (0025,
// 0030). The change log calls it under its lock, just before applying, so the
// decision and the change see the same state. A target that does not exist is
// left for Apply to report.
func Authorize(s *State, actor string, op Op) error {
	if op.Kind == CreateOrg {
		if string(op.Account) != actor {
			return fmt.Errorf("%w: create-org must name its caller as the first admin", ErrForbidden)
		}
		return nil
	}
	if s == nil {
		return ErrNoOrg
	}
	if actor == SystemActor {
		return authorizeSystem(s, op)
	}
	who := access.AccountID(actor)
	if _, ok := s.Access.Account(who); !ok {
		return fmt.Errorf("%w: %s", ErrUnknownActor, actor)
	}
	need := func(role access.Role, tn TreeName, node hierarchy.NodeID) error {
		t, err := tree(s.Org, tn)
		if err != nil {
			return nil // Apply reports the unknown tree
		}
		chain := t.Ancestry(node)
		if chain == nil {
			return nil // Apply reports the unknown node
		}
		if s.Access.RoleAt(who, string(tn), chain) < role {
			return &ForbiddenError{Actor: actor, Need: role, Tree: tn, Node: node}
		}
		return nil
	}
	// orgAdmin: admin at the Org root of either tree.
	orgAdmin := func() error {
		root := s.Org.Locations.Root()
		if need(access.Admin, Locations, root) == nil || need(access.Admin, Services, root) == nil {
			return nil
		}
		return &ForbiddenError{Actor: actor, Need: access.Admin, Tree: Locations, Node: root}
	}

	switch op.Kind {
	case AddFolder, AddAP:
		return need(access.Operator, op.Tree, op.Parent)
	case Set, Unset, Rename:
		return need(access.Operator, op.Tree, op.Node)
	case Move:
		if t, err := tree(s.Org, op.Tree); err == nil && op.Tree == Locations {
			if n, ok := t.Node(op.Node); ok && n.Kind == hierarchy.KindAP && t.InIsolated(n.Parent) && !t.InIsolated(op.Parent) {
				// Adoption out of Landing Zone (0032): the destination decides.
				if err := need(access.Viewer, op.Tree, n.Parent); err != nil {
					return err
				}
				return need(access.Operator, op.Tree, op.Parent)
			}
		}
		if err := need(access.Operator, op.Tree, op.Node); err != nil {
			return err
		}
		return need(access.Operator, op.Tree, op.Parent)
	case AssignServices:
		if err := need(access.Operator, Locations, op.Node); err != nil {
			return err
		}
		for _, f := range op.Services {
			if err := need(access.Viewer, Services, f); err != nil {
				return err
			}
		}
		return nil
	case Lock, Unlock:
		return need(access.Admin, op.Tree, op.Node)
	case BreakHierarchy:
		if err := need(access.Admin, op.Tree, op.Node); err != nil {
			return err
		}
		t, err := tree(s.Org, op.Tree)
		if err != nil {
			return nil
		}
		for _, l := range t.LocksAbove(op.Node) {
			if err := need(access.Admin, op.Tree, l.Node); err != nil {
				return err
			}
		}
		return nil
	case AddBuiltins:
		root := s.Org.Locations.Root()
		if err := need(access.Admin, Locations, root); err != nil {
			return err
		}
		return need(access.Admin, Services, root)
	case AddKey, SetKey, RemoveKey:
		// A network's keys are its folder's operators' (0070): a leasing
		// office, say.
		return need(access.Operator, Services, op.Node)
	case SetConcentrator, RemoveConcentrator, SetVNI, RemoveVNI:
		// The library serves every network (0037).
		return need(access.Admin, Services, s.Org.Services.Root())
	case AddTemplate:
		// A template is its level's, as a setting there is (0085).
		return need(access.Operator, Locations, op.Parent)
	case EditTemplate, SetTemplate, UnsetTemplate, RemoveTemplate:
		if tm, ok := s.Library.Template(op.Template); ok {
			return need(access.Operator, Locations, tm.At)
		}
		return nil // Apply reports the unknown template
	case AddAccount:
		if !s.Access.AdminAnywhere(who) {
			return &ForbiddenError{Actor: actor, Need: access.Admin}
		}
		return nil
	case GrantRole, RevokeRole:
		return need(access.Admin, op.Tree, op.Node)
	case Enroll:
		return fmt.Errorf("%w: APs enroll themselves, through POST /v1/enroll", ErrForbidden)
	case RemoveAP:
		return need(access.Operator, Locations, op.Node)
	case IssueToken:
		if op.Account == who {
			return nil
		}
		return orgAdmin()
	case RevokeToken:
		if tok, ok := s.Access.Token(op.TokenID); ok && tok.Account == who {
			return nil
		}
		return orgAdmin()
	}
	return fmt.Errorf("%w: %q", ErrUnknownKind, op.Kind)
}

// authorizeSystem limits what the manager may do in its own name (0036):
// create the built-in folders, enroll APs into Landing Zone while it has
// room (0038), and make the template of a kind of AP no template is for, at
// the Org, picked there, with nothing in it (0085).
func authorizeSystem(s *State, op Op) error {
	switch op.Kind {
	case AddBuiltins:
		return nil
	case AddTemplate:
		if op.Parent != s.Org.Locations.Root() || !op.Default || len(op.Values) > 0 {
			return ErrDefaultTemplate
		}
		for _, tm := range s.Library.Templates() {
			for _, b := range op.Boards {
				if tm.ForBoard(b) {
					return ErrDefaultTemplate
				}
			}
		}
		return nil
	case Enroll:
		if len(s.Org.Locations.Descendants(LandingZone)) >= LandingZoneLimit {
			return ErrFull
		}
		return nil
	}
	return fmt.Errorf("%w: the manager itself may only add built-in folders, enroll APs into Landing Zone, and make a new kind of AP's template", ErrForbidden)
}
