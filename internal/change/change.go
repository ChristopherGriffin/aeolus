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
	"sort"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/keys"
	"github.com/ChristopherGriffin/aeolus/internal/library"
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
	AddBuiltins    Kind = "add-builtins"

	// Access (0024, 0025).
	AddAccount  Kind = "add-account"
	IssueToken  Kind = "issue-token"
	RevokeToken Kind = "revoke-token"
	GrantRole   Kind = "grant"
	RevokeRole  Kind = "revoke"

	// The library (0023).
	SetConcentrator    Kind = "set-concentrator"
	RemoveConcentrator Kind = "remove-concentrator"
	SetVNI             Kind = "set-vni"
	RemoveVNI          Kind = "remove-vni"

	// APs arriving and leaving (0033, 0038).
	Enroll   Kind = "enroll"
	RemoveAP Kind = "remove-ap"

	// Per-user keys (0014, 0070).
	AddKey    Kind = "add-key"
	SetKey    Kind = "set-key"
	RemoveKey Kind = "remove-key"
)

// Built-in folders every Org has (0032), and the manager's own actor name for
// the changes it makes itself (0036).
const (
	LandingZone hierarchy.NodeID = "landing-zone"
	Sandbox     hierarchy.NodeID = "sandbox"
	SystemActor                  = "aeolus"

	// LandingZoneLimit caps how many APs may wait in Landing Zone (0038).
	// Enrollment needs no token, so this bounds what anonymous callers can
	// add to the log until a person adopts or removes some.
	LandingZoneLimit = 250
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
	// Values sets several fields of one node together, in place of Path and
	// Value: all of them or none (0045).
	Values map[hierarchy.Path]json.RawMessage `json:"values,omitempty"`
	// Paths unsets several fields of one node together, in place of Path
	// (0046).
	Paths []hierarchy.Path `json:"paths,omitempty"`

	Account   access.AccountID `json:"account,omitempty"`
	Role      string           `json:"role,omitempty"`
	TokenID   string           `json:"token_id,omitempty"`
	TokenHash []byte           `json:"token_hash,omitempty"`

	Concentrator string `json:"concentrator,omitempty"`
	VNI          int    `json:"vni,omitempty"`

	// Per-user keys (0070): the network, in the Services folder Node, and
	// the key's ID. Value is its definition, its passphrase sealed.
	Network string `json:"network,omitempty"`
	Key     string `json:"key,omitempty"`
}

// Field is one field a set changes.
type Field struct {
	Path  hierarchy.Path
	Value json.RawMessage
}

// Fields lists the fields a set changes: its path and value, or its values
// in path order.
func (op Op) Fields() []Field {
	if len(op.Values) == 0 {
		return []Field{{op.Path, op.Value}}
	}
	out := make([]Field, 0, len(op.Values))
	for p, v := range op.Values {
		out = append(out, Field{p, v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Unsets lists the fields an unset removes: its path, or its paths.
func (op Op) Unsets() []hierarchy.Path {
	if len(op.Paths) == 0 {
		return []hierarchy.Path{op.Path}
	}
	return op.Paths
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
	Org     *hierarchy.Org
	Access  *access.Access
	Library *library.Library
	// Facts holds what each AP reported about itself when it enrolled
	// (0033), for the person deciding whether to adopt it.
	Facts map[hierarchy.NodeID]json.RawMessage
	// Keys are the per-user keys (0070).
	Keys *keys.Store
}

// Clone returns an independent copy, or nil for nil.
func (s *State) Clone() *State {
	if s == nil {
		return nil
	}
	facts := make(map[hierarchy.NodeID]json.RawMessage, len(s.Facts))
	for ap, f := range s.Facts {
		facts[ap] = f // never modified in place
	}
	return &State{Org: s.Org.Clone(), Access: s.Access.Clone(), Library: s.Library.Clone(), Facts: facts, Keys: s.Keys.Clone()}
}

var (
	ErrNoOrg       = errors.New("the Org does not exist yet; the first change must be create-org")
	ErrOrgExists   = errors.New("the Org already exists")
	ErrUnknownKind = errors.New("unknown change kind")
	ErrUnknownTree = errors.New("unknown tree")
	ErrNoValue     = errors.New("set needs a value")
	ErrNoNode      = errors.New("change needs a node")
	ErrNoPath      = errors.New("change needs a field path")
	ErrTwoForms    = errors.New("a set or unset takes one field (path) or several (values, paths), not both")
	ErrNoAccount   = errors.New("change needs an account")
	ErrNoTokenID   = errors.New("change needs a token ID")
	ErrUseAssign   = errors.New("service folders are set with assign-services")
	ErrNotAFolder  = errors.New("roles are granted on folders, not APs")
	ErrBuiltins    = errors.New("the built-in folders already exist")
	ErrReserved    = errors.New("that account name is reserved for the manager itself")
	ErrNoConcID    = errors.New("change needs a concentrator")
	ErrInUse       = errors.New("still in use")
	ErrFull        = fmt.Errorf("Landing Zone holds %d APs, its limit: adopt or remove some before more can enroll", LandingZoneLimit)
)

// InUseError lists what still refers to a concentrator or VNI.
type InUseError struct {
	What string
	By   []string
}

func (e *InUseError) Error() string {
	return fmt.Sprintf("%s is %v by %v", e.What, ErrInUse, e.By)
}

func (e *InUseError) Unwrap() error { return ErrInUse }

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
	case AddBuiltins:
		eff, err = addBuiltins(s.Org)
	case SetConcentrator, RemoveConcentrator, SetVNI, RemoveVNI:
		eff, err = applyLibrary(s, op)
	case AddAccount, IssueToken, RevokeToken, GrantRole, RevokeRole:
		eff, err = applyAccess(s, op)
	case Enroll, RemoveAP:
		eff, err = applyAP(s, op)
	case AddKey, SetKey, RemoveKey:
		eff, err = applyKey(s, op)
	default:
		eff, err = apply(s.Org, op)
		if err == nil {
			err = checkKeys(s)
		}
	}
	return s, eff, err
}

func validate(op Op) error {
	switch op.Kind {
	case AddBuiltins:
		return nil
	case SetConcentrator, RemoveConcentrator, SetVNI, RemoveVNI:
		if op.Concentrator == "" {
			return ErrNoConcID
		}
		return nil
	case CreateOrg:
		if op.Account == "" {
			return ErrNoAccount
		}
	case AddFolder, AddAP, Move, BreakHierarchy, AssignServices:
	case Set:
		if len(op.Values) > 0 && (op.Path != "" || len(op.Value) > 0) {
			return ErrTwoForms
		}
		for _, f := range op.Fields() {
			if f.Path == "" {
				return ErrNoPath
			}
		}
	case Unset:
		if len(op.Paths) > 0 && op.Path != "" {
			return ErrTwoForms
		}
		for _, p := range op.Unsets() {
			if p == "" {
				return ErrNoPath
			}
		}
	case Lock, Unlock:
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
	case Enroll:
		if op.TokenID == "" {
			return ErrNoTokenID
		}
	case RemoveAP:
	case AddKey, SetKey, RemoveKey:
		return validateKey(op)
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
	s := &State{Org: hierarchy.NewOrg(op.Node, op.Name), Access: access.New(), Library: library.New(), Facts: map[hierarchy.NodeID]json.RawMessage{}, Keys: keys.New()}
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

// addBuiltins creates whichever built-in folders are missing: Landing Zone
// (isolated) in Locations, and Sandbox in both trees (0032).
func addBuiltins(o *hierarchy.Org) (Effect, error) {
	var created []string
	add := func(t *hierarchy.Tree, tree TreeName, id hierarchy.NodeID, name string, isolated bool) error {
		if n, ok := t.Node(id); ok {
			if n.Isolated != isolated || n.Parent != t.Root() {
				return fmt.Errorf("%w: %s/%s exists but is not the built-in folder", hierarchy.ErrExists, tree, id)
			}
			return nil
		}
		var err error
		if isolated {
			err = t.AddIsolated(id, name, t.Root())
		} else {
			err = t.AddFolder(id, name, t.Root())
		}
		if err == nil {
			created = append(created, string(tree)+"/"+string(id))
		}
		return err
	}
	if err := add(o.Locations, Locations, LandingZone, "Landing Zone", true); err != nil {
		return Effect{}, err
	}
	if err := add(o.Locations, Locations, Sandbox, "Sandbox", false); err != nil {
		return Effect{}, err
	}
	if err := add(o.Services, Services, Sandbox, "Sandbox", false); err != nil {
		return Effect{}, err
	}
	if len(created) == 0 {
		return Effect{}, ErrBuiltins
	}
	return Effect{After: created}, nil
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
		return set(t, op)
	case Unset:
		return unset(t, op)
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

// set sets one field, or several together (0045). If any of them cannot be
// set, those already set are put back, so none of them change.
func set(t *hierarchy.Tree, op Op) (Effect, error) {
	fields := op.Fields()
	values := make([]hierarchy.Value, len(fields))
	for i, f := range fields {
		if op.Tree == Locations && f.Path == hierarchy.ServicesPath {
			return Effect{}, ErrUseAssign
		}
		v, err := decode(f.Value)
		if err != nil {
			return Effect{}, err
		}
		values[i] = v
	}
	if len(op.Values) == 0 {
		before, _ := t.Own(op.Node, op.Path)
		return Effect{Before: before, After: values[0]}, t.Set(op.Node, op.Path, values[0])
	}
	before, after := map[string]any{}, map[string]any{} // string keys, as read back from the log
	for i, f := range fields {
		if v, ok := t.Own(op.Node, f.Path); ok {
			before[string(f.Path)] = v
		}
		if err := t.Set(op.Node, f.Path, values[i]); err != nil {
			for _, done := range fields[:i] {
				if v, ok := before[string(done.Path)]; ok {
					t.Set(op.Node, done.Path, v)
				} else {
					t.Unset(op.Node, done.Path)
				}
			}
			return Effect{}, err
		}
		after[string(f.Path)] = values[i]
	}
	if len(before) == 0 {
		return Effect{After: after}, nil
	}
	return Effect{Before: before, After: after}, nil
}

// unset unsets one field, or several together (0046). Each must be set at
// the node; if one is not, none is unset.
func unset(t *hierarchy.Tree, op Op) (Effect, error) {
	if len(op.Paths) == 0 {
		before, _ := t.Own(op.Node, op.Path)
		return Effect{Before: before}, t.Unset(op.Node, op.Path)
	}
	before := map[string]any{} // string keys, as read back from the log
	for _, p := range op.Paths {
		v, ok := t.Own(op.Node, p)
		if !ok {
			return Effect{}, fmt.Errorf("%w: %s", hierarchy.ErrNotSetHere, p)
		}
		before[string(p)] = v
	}
	for _, p := range op.Paths {
		t.Unset(op.Node, p) // each is set here, so none fails
	}
	return Effect{Before: before}, nil
}

func applyAccess(s *State, op Op) (Effect, error) {
	a := s.Access
	switch op.Kind {
	case AddAccount:
		if op.Account == SystemActor {
			return Effect{}, ErrReserved
		}
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
