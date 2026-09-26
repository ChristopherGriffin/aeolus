// Package change defines the changes that can be made to an Org and applies
// them. Every change the manager accepts is one Op, recorded in the change
// log (0009) and replayed from it; Apply is deterministic, so replaying the
// log rebuilds exactly the same state.
package change

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Kind names an operation.
type Kind string

const (
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
}

// Effect records what a change did, for the change log: the state before and
// after, and any overrides it removed (0017).
type Effect struct {
	Before  any                  `json:"before,omitempty"`
	After   any                  `json:"after,omitempty"`
	Removed []hierarchy.Override `json:"removed,omitempty"`
}

var (
	ErrNoOrg       = errors.New("the Org does not exist yet; the first change must be create-org")
	ErrOrgExists   = errors.New("the Org already exists")
	ErrUnknownKind = errors.New("unknown change kind")
	ErrUnknownTree = errors.New("unknown tree")
	ErrNoValue     = errors.New("set needs a value")
	ErrNoNode      = errors.New("change needs a node")
	ErrNoPath      = errors.New("change needs a field path")
	ErrUseAssign   = errors.New("service folders are set with assign-services")
)

// Apply applies op to o and reports its effect. It returns the Org to use from
// then on: a new one for create-org, otherwise o itself. On error, o is
// unchanged.
func Apply(o *hierarchy.Org, op Op) (*hierarchy.Org, Effect, error) {
	if op.Node == "" {
		return o, Effect{}, ErrNoNode
	}
	switch op.Kind {
	case Set, Unset, Lock, Unlock:
		if op.Path == "" {
			return o, Effect{}, ErrNoPath
		}
	}
	if op.Kind == CreateOrg {
		if o != nil {
			return o, Effect{}, ErrOrgExists
		}
		return hierarchy.NewOrg(op.Node, op.Name), Effect{After: map[string]string{"node": string(op.Node), "name": op.Name}}, nil
	}
	if o == nil {
		return nil, Effect{}, ErrNoOrg
	}
	eff, err := apply(o, op)
	return o, eff, err
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
