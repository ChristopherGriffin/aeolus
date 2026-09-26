package hierarchy

import "sort"

// Origin says where a resolved value came from.
type Origin int

const (
	OriginSelf      Origin = iota // set at the node itself
	OriginInherited               // set at a node above
	OriginLocked                  // enforced by a lock at From (which may be the node itself)
	OriginBaseline                // copied when From broke hierarchy
)

// Resolved is a field's value at a node and where it came from.
type Resolved struct {
	Value  Value
	From   NodeID
	Origin Origin
}

// chain returns id and the nodes above it that it inherits from: upward,
// stopping at the first node that breaks hierarchy (0005).
func (t *Tree) chain(id NodeID) []NodeID {
	var out []NodeID
	for c := id; c != ""; c = t.nodes[c].Parent {
		out = append(out, c)
		if t.nodes[c].Broken {
			break
		}
	}
	return out
}

// Resolve returns a field's value at a node (0012): a lock above wins,
// otherwise the closest node that sets the field, otherwise a copy taken at a
// break.
func (t *Tree) Resolve(id NodeID, p Path) (Resolved, bool) {
	c := t.chain(id)
	for i := len(c) - 1; i >= 0; i-- {
		if t.locks[c[i]][p] {
			return Resolved{Value: t.set[c[i]][p], From: c[i], Origin: OriginLocked}, true
		}
	}
	for _, n := range c {
		if v, ok := t.set[n][p]; ok {
			o := OriginInherited
			if n == id {
				o = OriginSelf
			}
			return Resolved{Value: v, From: n, Origin: o}, true
		}
		if v, ok := t.base[n][p]; ok {
			return Resolved{Value: v, From: n, Origin: OriginBaseline}, true
		}
	}
	return Resolved{}, false
}

// EffectivePaths lists every field that resolves at a node, sorted.
func (t *Tree) EffectivePaths(id NodeID) []Path {
	seen := map[Path]bool{}
	for _, n := range t.chain(id) {
		for p := range t.set[n] {
			seen[p] = true
		}
		for p := range t.base[n] {
			seen[p] = true
		}
	}
	out := make([]Path, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ResolveAll resolves every effective field at a node.
func (t *Tree) ResolveAll(id NodeID) map[Path]Resolved {
	out := map[Path]Resolved{}
	for _, p := range t.EffectivePaths(id) {
		if r, ok := t.Resolve(id, p); ok {
			out[p] = r
		}
	}
	return out
}

// isOverride reports whether the value set at n replaces something n would
// otherwise have: a value above it, or its own copy from a break. A value set
// for the first time is a definition, not an override.
func (t *Tree) isOverride(n NodeID, p Path) bool {
	if _, ok := t.base[n][p]; ok {
		return true
	}
	for _, a := range t.chain(n)[1:] {
		if _, ok := t.set[a][p]; ok {
			return true
		}
		if _, ok := t.base[a][p]; ok {
			return true
		}
	}
	return false
}

// OverridesInEffect lists the overrides that shape a node's values, made at
// the node or above it. This feeds the overrides menu (0012).
func (t *Tree) OverridesInEffect(id NodeID) []Override {
	var out []Override
	for _, p := range t.EffectivePaths(id) {
		r, ok := t.Resolve(id, p)
		if !ok || (r.Origin != OriginSelf && r.Origin != OriginInherited) {
			continue
		}
		if t.isOverride(r.From, p) {
			out = append(out, Override{Node: r.From, Path: p, Value: r.Value})
		}
	}
	return out
}

// OverridesBelow lists the overrides made anywhere below a node, including
// ones that no longer take effect because a lock shadows them.
func (t *Tree) OverridesBelow(id NodeID) []Override {
	var out []Override
	for _, d := range t.Descendants(id) {
		paths := make([]Path, 0, len(t.set[d]))
		for p := range t.set[d] {
			paths = append(paths, p)
		}
		sort.Slice(paths, func(i, j int) bool { return paths[i] < paths[j] })
		for _, p := range paths {
			if t.isOverride(d, p) {
				out = append(out, Override{Node: d, Path: p, Value: t.set[d][p]})
			}
		}
	}
	return out
}
