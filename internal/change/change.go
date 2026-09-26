// Package change defines the changes that can be made to the manager's state
// and applies them. Every change the manager accepts is one Op, recorded in
// the change log (0009) and replayed from it; Apply is deterministic, so
// replaying the log rebuilds exactly the same state.
//
// The state is the Org's two trees plus its accounts, tokens and roles
// (0024): access changes go through the log like everything else.
package change

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Kind names an operation.
type Kind string

const (
	// The trees.
	CreateOrg      Kind = "create-org"
	AddFolder      Kind = "add-folder"
	AddAP          Kind = "add-ap"
	Move           Kind = "move"
	Set            Kind = "set"
	Unset          Kind = "unset"
	Lock           Kind = "lock"
	Unlock         Kind = "unlock"
	BreakHierarchy Kind = "break-hierarchy"
	AssignServices Kind = "assign-services"

	// Access (0024, 0025).
	AddAccount  Kind = "add-account"
	IssueToken  Kind = "issue-token"
	RevokeToken Kind = "revoke-token"
	GrantRole   Kind = "grant"
	RevokeRole  Kind = "revoke"
)

// TreeName picks one of the Org's two trees (0013).
type TreeName string

const (
	Locations TreeName = "locations"
	Services  TreeName = "services"
)

// Op is one change. Only the fields its Kind uses are set.
type Op struct {
	Kind     Kind               `json:"kind"`
	Tree     TreeName           `json:"tree,omitempty"`
	Node     hierarchy.NodeID   `json:"node,omitempty"`
	Parent   hierarchy.NodeID   `json:"parent,omitempty"`
	Name     string             `json:"name,omitempty"`
	Path     hierarchy.Path     `json:"path,omitempty"`
	Value    json.RawMessage    `json:"value,omitempty"`
	Services []hierarchy.NodeID `json:"services,omitempty"`

	Account   access.AccountID `json:"account,omitempty"`
	Role      string           `json:"role,omitempty"`
	TokenID   string           `json:"token_id,omitempty"`
	TokenHash []byte           `json:"token_hash,omitempty"`
}

// Effect records what a change did, for the change log: the state before and
// after, and any overrides it removed (0017).
type Effect struct {
	Before  any                  `json:"before,omitempty"`
	After   any                  `json:"after,omitempty"`
	Removed []hierarchy.Override `json:"removed,omitempty"`
}

// State is everything the change log rebuilds.
type State struct {
	Org    *hierarchy.Org
	Access *access.Access
}

// Clone returns an independent copy, or nil for nil.
func (s *State) Clone() *State {
	if s == nil {
		return nil
	}
	return &State{Org: s.Org.Clone(), Access: s.Access.Clone()}
}

var (
	ErrNoOrg       = errors.New("the Org does not exist yet; the first change must be create-org")
	ErrOrgExists   = errors.New("the Org already exists")
	ErrUnknownKind = errors.New("unknown change kind")
	ErrUnknownTree = errors.New("unknown tree")
	ErrNoValue     = errors.New("set needs a value")
	ErrNoNode      = errors.New("change needs a node")
	ErrNoPath      = errors.New("change needs a field path")
	ErrNoAccount   = errors.New("change needs an account")
	ErrNoTokenID   = errors.New("change needs a token ID")
	ErrUseAssign   = errors.New("service folders are set with assign-services")
	ErrNotAFolder  = errors.New("roles are granted on folders, not APs")
)

// Apply applies op to s and reports its effect. It returns the state to use
// from then on: a new one for create-org, otherwise s itself. On error, s is
// unchanged.
func Apply(s *State, op Op) (*State, Effect, error) {
	if err := validate(op); err != nil {
		return s, Effect{}, err
	}
	if op.Kind == CreateOrg {
		if s != nil {
			return s, Effect{}, ErrOrgExists
		}
		return createOrg(op)
	}
	if s == nil {
		return nil, Effect{}, ErrNoOrg
	}
	var eff Effect
	var err error
	switch op.Kind {
	case AddAccount, IssueToken, RevokeToken, GrantRole, RevokeRole:
		eff, err = applyAccess(s, op)
	default:
		eff, err = apply(s.Org, op)
	}
	return s, eff, err
}

func validate(op Op) error {
	switch op.Kind {
	case CreateOrg:
		if op.Account == "" {
			return ErrNoAccount
		}
	case AddFolder, AddAP, Move, BreakHierarchy, AssignServices:
	case Set, Unset, Lock, Unlock:
		if op.Path == "" {
			return ErrNoPath
		}
	case AddAccount, IssueToken:
		if op.Account == "" {
			return ErrNoAccount
		}
		if op.Kind == IssueToken && op.TokenID == "" {
			return ErrNoTokenID
		}
		return nil
	case RevokeToken:
		if op.TokenID == "" {
			return ErrNoTokenID
		}
		return nil
	case GrantRole, RevokeRole:
		if op.Account == "" {
			return ErrNoAccount
		}
	default:
		return fmt.Errorf("%w: %q", ErrUnknownKind, op.Kind)
	}
	if op.Node == "" {
		return ErrNoNode
	}
	return nil
}

// createOrg starts the state. The Org's first account is its admin at the
// root of both trees, so there is never a moment without one.
func createOrg(op Op) (*State, Effect, error) {
	s := &State{Org: hierarchy.NewOrg(op.Node, op.Name), Access: access.New()}
	if err := s.Access.AddAccount(op.Account, string(op.Account)); err != nil {
		return nil, Effect{}, err
	}
	for _, t := range []TreeName{Locations, Services} {
		if err := s.Access.Grant(access.Grant{Account: op.Account, Tree: string(t), Node: op.Node, Role: access.Admin}); err != nil {
			return nil, Effect{}, err
		}
	}
	return s, Effect{After: map[string]string{"node": string(op.Node), "name": op.Name, "admin": string(op.Account)}}, nil
}

func apply(o *hierarchy.Org, op Op) (Effect, error) {
	if op.Kind == AssignServices {
		before, _ := o.Locations.Own(op.Node, hierarchy.ServicesPath)
		if err := o.AssignServices(op.Node, op.Services); err != nil {
			return Effect{}, err
		}
		return Effect{Before: before, After: op.Services}, nil
	}

	t, err := tree(o, op.Tree)
	if err != nil {
		return Effect{}, err
	}
	switch op.Kind {
	case AddFolder:
		return Effect{After: placement(op)}, t.AddFolder(op.Node, op.Name, op.Parent)
	case AddAP:
		return Effect{After: placement(op)}, t.AddAP(op.Node, op.Name, op.Parent)
	case Move:
		n, ok := t.Node(op.Node)
		if !ok {
			return Effect{}, fmt.Errorf("%w: %s", hierarchy.ErrNotFound, op.Node)
		}
		removed, err := t.Move(op.Node, op.Parent)
		return Effect{Before: n.Parent, After: op.Parent, Removed: removed}, err
	case Set:
		if op.Tree == Locations && op.Path == hierarchy.ServicesPath {
			return Effect{}, ErrUseAssign
		}
		v, err := decode(op.Value)
		if err != nil {
			return Effect{}, err
		}
		before, _ := t.Own(op.Node, op.Path)
		return Effect{Before: before, After: v}, t.Set(op.Node, op.Path, v)
	case Unset:
		before, _ := t.Own(op.Node, op.Path)
		return Effect{Before: before}, t.Unset(op.Node, op.Path)
	case Lock:
		removed, err := t.Lock(op.Node, op.Path)
		return Effect{Before: false, After: true, Removed: removed}, err
	case Unlock:
		return Effect{Before: true, After: false}, t.Unlock(op.Node, op.Path)
	case BreakHierarchy:
		return Effect{Before: false, After: true}, t.BreakHierarchy(op.Node)
	}
	return Effect{}, fmt.Errorf("%w: %q", ErrUnknownKind, op.Kind)
}

func applyAccess(s *State, op Op) (Effect, error) {
	a := s.Access
	switch op.Kind {
	case AddAccount:
		return Effect{After: map[string]string{"account": string(op.Account), "name": op.Name}}, a.AddAccount(op.Account, op.Name)
	case IssueToken:
		return Effect{After: map[string]string{"account": string(op.Account), "token": op.TokenID}}, a.AddToken(op.TokenID, op.Account, op.TokenHash)
	case RevokeToken:
		tok, ok := a.Token(op.TokenID)
		if !ok {
			return Effect{}, fmt.Errorf("%w: %s", access.ErrNoToken, op.TokenID)
		}
		return Effect{Before: map[string]string{"account": string(tok.Account), "token": tok.ID}}, a.RevokeToken(op.TokenID)
	}

	// Grant or revoke a role on a folder.
	t, err := tree(s.Org, op.Tree)
	if err != nil {
		return Effect{}, err
	}
	n, ok := t.Node(op.Node)
	if !ok {
		return Effect{}, fmt.Errorf("%w: %s", hierarchy.ErrNotFound, op.Node)
	}
	if n.Kind == hierarchy.KindAP {
		return Effect{}, ErrNotAFolder
	}
	role, err := access.ParseRole(op.Role)
	if err != nil {
		return Effect{}, err
	}
	g := access.Grant{Account: op.Account, Tree: string(op.Tree), Node: op.Node, Role: role}
	view := map[string]string{"account": string(g.Account), "tree": g.Tree, "node": string(g.Node), "role": g.Role.String()}
	if op.Kind == GrantRole {
		return Effect{After: view}, a.Grant(g)
	}
	return Effect{Before: view}, a.Revoke(g)
}

func tree(o *hierarchy.Org, name TreeName) (*hierarchy.Tree, error) {
	switch name {
	case Locations:
		return o.Locations, nil
	case Services:
		return o.Services, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownTree, name)
}

func placement(op Op) map[string]string {
	return map[string]string{"name": op.Name, "parent": string(op.Parent)}
}

// decode turns a JSON value into the plain Go value stored in the tree. Values
// always pass through JSON, live and on replay, so both see the same types.
func decode(raw json.RawMessage) (hierarchy.Value, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, ErrNoValue
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("value: %w", err)
	}
	return v, nil
}
