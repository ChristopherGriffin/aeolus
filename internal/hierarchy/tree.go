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
	"sort"
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
	// Isolated marks a folder outside inheritance, such as Landing Zone
	// (0032): nothing inherits into it, nothing can be set in it, and it
	// holds only APs, which resolve to no config.
	Isolated bool
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
	ErrIsolated    = errors.New("inside an isolated folder: nothing can be set there, and it holds only APs")
	ErrNotAnAP     = errors.New("node is not an AP")
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

// Root returns the Org root's ID.
func (t *Tree) Root() NodeID { return t.root }

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

// AddIsolated adds a folder outside inheritance, such as Landing Zone (0032).
func (t *Tree) AddIsolated(id NodeID, name string, parent NodeID) error {
	if err := t.add(id, name, KindFolder, parent); err != nil {
		return err
	}
	t.nodes[id].Isolated = true
	return nil
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
	if kind != KindAP && t.InIsolated(parent) {
		return ErrIsolated
	}
	t.nodes[id] = &Node{ID: id, Name: name, Kind: kind, Parent: parent}
	t.children[parent] = append(t.children[parent], id)
	return nil
}

// RemoveAP takes an AP out of the tree, with whatever was set on it, and
// returns those values. Folders are never removed: the log keeps what they
// held, and an empty folder costs nothing.
func (t *Tree) RemoveAP(id NodeID) ([]Override, error) {
	n, ok := t.nodes[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if n.Kind != KindAP {
		return nil, fmt.Errorf("%w: %s", ErrNotAnAP, id)
	}
	var removed []Override
	for p, v := range t.set[id] {
		removed = append(removed, Override{Node: id, Path: p, Value: v})
	}
	sort.Slice(removed, func(i, j int) bool { return removed[i].Path < removed[j].Path })
	siblings := t.children[n.Parent]
	for i, c := range siblings {
		if c == id {
			t.children[n.Parent] = append(siblings[:i:i], siblings[i+1:]...)
			break
		}
	}
	delete(t.nodes, id)
	delete(t.set, id)
	delete(t.base, id)
	delete(t.locks, id)
	return removed, nil
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
	if n.Isolated || (n.Kind != KindAP && t.InIsolated(newParent)) {
		return ErrIsolated
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

// InIsolated reports whether a node is an isolated folder or sits inside one.
func (t *Tree) InIsolated(id NodeID) bool {
	for c := id; c != ""; {
		n, ok := t.nodes[c]
		if !ok {
			return false
		}
		if n.Isolated {
			return true
		}
		c = n.Parent
	}
	return false
}

// Ancestry returns the chain from the Org root down to id, ignoring breaks:
// the path roles flow along (0025). It is empty for an unknown node.
func (t *Tree) Ancestry(id NodeID) []NodeID {
	var out []NodeID
	for c := id; c != ""; {
		n, ok := t.nodes[c]
		if !ok {
			return nil
		}
		out = append([]NodeID{c}, out...)
		c = n.Parent
	}
	return out
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

// APs returns every AP in the tree, sorted.
func (t *Tree) APs() []NodeID {
	var out []NodeID
	for id, n := range t.nodes {
		if n.Kind == KindAP {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Clone returns an independent copy of the tree. Values are shared, which is
// safe because stored values are immutable.
func (t *Tree) Clone() *Tree {
	c := &Tree{
		root:     t.root,
		allowAPs: t.allowAPs,
		nodes:    make(map[NodeID]*Node, len(t.nodes)),
		children: make(map[NodeID][]NodeID, len(t.children)),
		set:      cloneValues(t.set),
		base:     cloneValues(t.base),
		locks:    make(map[NodeID]map[Path]bool, len(t.locks)),
	}
	for id, n := range t.nodes {
		cp := *n
		c.nodes[id] = &cp
	}
	for id, kids := range t.children {
		c.children[id] = append([]NodeID(nil), kids...)
	}
	for id, ls := range t.locks {
		c.locks[id] = make(map[Path]bool, len(ls))
		for p, v := range ls {
			c.locks[id][p] = v
		}
	}
	return c
}

func cloneValues(m map[NodeID]map[Path]Value) map[NodeID]map[Path]Value {
	out := make(map[NodeID]map[Path]Value, len(m))
	for id, vals := range m {
		out[id] = make(map[Path]Value, len(vals))
		for p, v := range vals {
			out[id][p] = v
		}
	}
	return out
}
