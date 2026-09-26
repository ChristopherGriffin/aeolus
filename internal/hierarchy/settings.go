package hierarchy

import (
	"errors"
	"fmt"
	"sort"
)

var ErrNotLocked = errors.New("field is not locked at this node")

// LockedError reports a change refused because a lock above applies.
type LockedError struct {
	Path Path
	By   NodeID
}

func (e *LockedError) Error() string {
	return fmt.Sprintf("%s is locked by %s", e.Path, e.By)
}

// Override is a value set at a node that replaces something it would
// otherwise inherit.
type Override struct {
	Node  NodeID `json:"node"`
	Path  Path   `json:"path"`
	Value Value  `json:"value"`
}

// Own returns the value set at the node itself, if any.
func (t *Tree) Own(id NodeID, p Path) (Value, bool) {
	v, ok := t.set[id][p]
	return v, ok
}

// IsLocked reports whether the node itself locks p.
func (t *Tree) IsLocked(id NodeID, p Path) bool { return t.locks[id][p] }

// Set stores a value at a node. It is refused when a lock above applies.
func (t *Tree) Set(id NodeID, p Path, v Value) error {
	if _, ok := t.nodes[id]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if t.InIsolated(id) {
		return ErrIsolated
	}
	if by, ok := t.lockAbove(id, p); ok {
		return &LockedError{Path: p, By: by}
	}
	if t.set[id] == nil {
		t.set[id] = map[Path]Value{}
	}
	t.set[id][p] = v
	return nil
}

// Unset deletes the value set at a node, so the node inherits again
// ("revert to inherited", 0012). A lock on the value goes with it.
func (t *Tree) Unset(id NodeID, p Path) error {
	if _, ok := t.set[id][p]; !ok {
		return ErrNotSetHere
	}
	delete(t.set[id], p)
	delete(t.locks[id], p)
	return nil
}

// Lock locks the value set at a node for everything below it. Overrides below
// that the lock would hide are removed rather than kept as dead values that
// could resurface after an unlock or a break; they are returned so the change
// can show and record them.
func (t *Tree) Lock(id NodeID, p Path) ([]Override, error) {
	if _, ok := t.set[id][p]; !ok {
		return nil, ErrLockOnValue
	}
	if by, ok := t.lockAbove(id, p); ok {
		return nil, &LockedError{Path: p, By: by}
	}
	if t.locks[id] == nil {
		t.locks[id] = map[Path]bool{}
	}
	t.locks[id][p] = true
	var removed []Override
	for _, d := range t.Descendants(id) {
		if v, ok := t.set[d][p]; ok && t.reaches(id, d) {
			removed = append(removed, Override{Node: d, Path: p, Value: v})
			delete(t.set[d], p)
			delete(t.locks[d], p)
		}
	}
	return removed, nil
}

// Unlock removes a lock.
func (t *Tree) Unlock(id NodeID, p Path) error {
	if !t.locks[id][p] {
		return ErrNotLocked
	}
	delete(t.locks[id], p)
	return nil
}

// Move places a node under a new parent (0005). From then on it inherits the
// new parent's settings and locks; overrides in the moved branch that a lock
// above now hides are removed and returned, as with Lock.
func (t *Tree) Move(id, newParent NodeID) ([]Override, error) {
	if err := t.move(id, newParent); err != nil {
		return nil, err
	}
	var removed []Override
	for _, d := range append([]NodeID{id}, t.Descendants(id)...) {
		paths := make([]Path, 0, len(t.set[d]))
		for p := range t.set[d] {
			paths = append(paths, p)
		}
		sort.Slice(paths, func(i, j int) bool { return paths[i] < paths[j] })
		for _, p := range paths {
			if _, locked := t.lockAbove(d, p); locked {
				removed = append(removed, Override{Node: d, Path: p, Value: t.set[d][p]})
				delete(t.set[d], p)
				delete(t.locks[d], p)
			}
		}
	}
	return removed, nil
}

// BreakHierarchy makes a folder the start of its own branch (0005). Locks
// from above stop applying. The folder keeps a copy of everything it was
// inheriting, so no AP changes at the moment of the break. For fields the
// folder already sets, the copy records what it was overriding, so those
// values still count as overrides and "revert" falls back to the copy.
func (t *Tree) BreakHierarchy(id NodeID) error {
	n, ok := t.nodes[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if id == t.root {
		return ErrBreakRoot
	}
	if n.Broken {
		return ErrBroken
	}
	if t.InIsolated(id) {
		return ErrIsolated
	}
	copied := map[Path]Value{}
	for _, p := range t.EffectivePaths(id) {
		if _, own := t.set[id][p]; own {
			if r, ok := t.Resolve(n.Parent, p); ok {
				copied[p] = r.Value
			}
			continue
		}
		if r, ok := t.Resolve(id, p); ok {
			copied[p] = r.Value
		}
	}
	n.Broken = true
	t.base[id] = copied
	return nil
}

// LocksAbove lists the locks that apply to a node from above: what breaking
// hierarchy there would escape. Permission checks use it (0005).
func (t *Tree) LocksAbove(id NodeID) []Override {
	var out []Override
	for _, a := range t.chain(id)[1:] {
		for p := range t.locks[a] {
			out = append(out, Override{Node: a, Path: p, Value: t.set[a][p]})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// lockAbove reports the topmost lock on p that applies to id from above.
func (t *Tree) lockAbove(id NodeID, p Path) (NodeID, bool) {
	c := t.chain(id)
	for i := len(c) - 1; i >= 1; i-- {
		if t.locks[c[i]][p] {
			return c[i], true
		}
	}
	return "", false
}

// reaches reports whether settings at ancestor still flow down to id, that is
// no node that breaks hierarchy sits between them.
func (t *Tree) reaches(ancestor, id NodeID) bool {
	for _, c := range t.chain(id) {
		if c == ancestor {
			return true
		}
	}
	return false
}
