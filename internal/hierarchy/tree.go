// Package hierarchy resolves settings through a folder tree.
//
// It implements decisions 0004 (Org, then folders to any depth), 0005 (locks
// and Break Hierarchy), 0012 (inheritance per field) and 0013 (the Locations
// and Services trees). It knows nothing about what a field means: fields are
// addressed by Path and their values are opaque.
//
// A Tree is not safe for concurrent use. The manager serializes changes
// through the change log (0009).
package hierarchy

import (
	"errors"
	"fmt"
)

// NodeID identifies a node. IDs are stable; names can change.
type NodeID string

// Path addresses one field, for example "radio.5g.channel" (0012).
type Path string

// Value is a JSON-compatible value. Stored values are treated as immutable.
type Value = any

// Kind is what a node is.
type Kind int

const (
	KindOrg Kind = iota
	KindFolder
	KindAP
)

// Node is one position in a tree.
type Node struct {
	ID     NodeID
	Name   string
	Kind   Kind
	Parent NodeID // empty for the Org root
	Broken bool   // Break Hierarchy (0005)
}

var (
	ErrNotFound    = errors.New("node not found")
	ErrExists      = errors.New("node already exists")
	ErrBadParent   = errors.New("parent cannot hold children")
	ErrAPsNotHere  = errors.New("this tree does not hold APs")
	ErrMoveRoot    = errors.New("the Org root cannot move")
	ErrCycle       = errors.New("a node cannot move below itself")
	ErrBreakRoot   = errors.New("the Org root cannot break hierarchy")
	ErrBroken      = errors.New("node already breaks hierarchy")
	ErrNotSetHere  = errors.New("field is not set at this node")
	ErrLockOnValue = errors.New("a lock needs a value set at the same node")
)

// Tree is one hierarchy under the Org: Locations or Services (0013).
type Tree struct {
	root     NodeID
	allowAPs bool
	nodes    map[NodeID]*Node
	children map[NodeID][]NodeID
	set      map[NodeID]map[Path]Value // values set at a node
	base     map[NodeID]map[Path]Value // copies taken when a node breaks hierarchy
	locks    map[NodeID]map[Path]bool
}

// NewTree returns a tree holding only its Org root.
func NewTree(root NodeID, name string, allowAPs bool) *Tree {
	t := &Tree{
		root:     root,
		allowAPs: allowAPs,
		nodes:    map[NodeID]*Node{},
		children: map[NodeID][]NodeID{},
		set:      map[NodeID]map[Path]Value{},
		base:     map[NodeID]map[Path]Value{},
		locks:    map[NodeID]map[Path]bool{},
	}
	t.nodes[root] = &Node{ID: root, Name: name, Kind: KindOrg}
	return t
}

// Node returns a copy of a node.
func (t *Tree) Node(id NodeID) (Node, bool) {
	n, ok := t.nodes[id]
	if !ok {
		return Node{}, false
	}
	return *n, true
}

// AddFolder adds a folder under parent.
func (t *Tree) AddFolder(id NodeID, name string, parent NodeID) error {
	return t.add(id, name, KindFolder, parent)
}

// AddAP places an AP in a folder (or directly under the Org).
func (t *Tree) AddAP(id NodeID, name string, parent NodeID) error {
	if !t.allowAPs {
		return ErrAPsNotHere
	}
	return t.add(id, name, KindAP, parent)
}

func (t *Tree) add(id NodeID, name string, kind Kind, parent NodeID) error {
	if _, ok := t.nodes[id]; ok {
		return fmt.Errorf("%w: %s", ErrExists, id)
	}
	p, ok := t.nodes[parent]
	if !ok {
		return fmt.Errorf("%w: parent %s", ErrNotFound, parent)
	}
	if p.Kind == KindAP {
		return ErrBadParent
	}
	t.nodes[id] = &Node{ID: id, Name: name, Kind: kind, Parent: parent}
	t.children[parent] = append(t.children[parent], id)
	return nil
}

// move re-parents a node. Move (settings.go) wraps it with lock handling.
func (t *Tree) move(id, newParent NodeID) error {
	n, ok := t.nodes[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if id == t.root {
		return ErrMoveRoot
	}
	p, ok := t.nodes[newParent]
	if !ok {
		return fmt.Errorf("%w: parent %s", ErrNotFound, newParent)
	}
	if p.Kind == KindAP {
		return ErrBadParent
	}
	for c := newParent; c != ""; c = t.nodes[c].Parent {
		if c == id {
			return ErrCycle
		}
	}
	siblings := t.children[n.Parent]
	for i, c := range siblings {
		if c == id {
			t.children[n.Parent] = append(siblings[:i:i], siblings[i+1:]...)
			break
		}
	}
	n.Parent = newParent
	t.children[newParent] = append(t.children[newParent], id)
	return nil
}

// Descendants returns every node below id, depth first.
func (t *Tree) Descendants(id NodeID) []NodeID {
	var out []NodeID
	for _, c := range t.children[id] {
		out = append(out, c)
		out = append(out, t.Descendants(c)...)
	}
	return out
}
