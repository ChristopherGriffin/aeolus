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
	case Set, Unset:
		return need(access.Operator, op.Tree, op.Node)
	case Move:
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
	case AddAccount:
		if !s.Access.AdminAnywhere(who) {
			return &ForbiddenError{Actor: actor, Need: access.Admin}
		}
		return nil
	case GrantRole, RevokeRole:
		return need(access.Admin, op.Tree, op.Node)
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
