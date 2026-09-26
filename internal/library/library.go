// Package library holds the Org's library of reusable definitions (0015):
// concentrators, each with its labeled VNIs, and the Location folders each may
// be used at (0023). It is part of the state the change log rebuilds.
package library

import (
	"errors"
	"fmt"
	"sort"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Concentrator is where VXLAN transports go (0018). VNIs maps each VNI number
// to its label. An empty Scope means it can be used everywhere.
type Concentrator struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	Address string             `json:"address"`
	Port    int                `json:"port"`
	MTU     int                `json:"mtu"`
	Scope   []hierarchy.NodeID `json:"scope,omitempty"`
	VNIs    map[int]string     `json:"vnis,omitempty"`
}

var (
	ErrNoConcentrator = errors.New("no such concentrator in the library")
	ErrNoVNI          = errors.New("no such VNI on that concentrator")
	ErrBadVNI         = errors.New("a VNI is 1 to 16777215")
	ErrNoLabel        = errors.New("a VNI needs a label")
)

// Library is the set of concentrators.
type Library struct {
	concentrators map[string]*Concentrator
}

// New returns an empty library.
func New() *Library { return &Library{concentrators: map[string]*Concentrator{}} }

// Clone returns an independent copy.
func (l *Library) Clone() *Library {
	c := New()
	for id, k := range l.concentrators {
		c.concentrators[id] = copyOf(k)
	}
	return c
}

func copyOf(k *Concentrator) *Concentrator {
	cp := *k
	cp.Scope = append([]hierarchy.NodeID(nil), k.Scope...)
	cp.VNIs = make(map[int]string, len(k.VNIs))
	for v, label := range k.VNIs {
		cp.VNIs[v] = label
	}
	return &cp
}

// Get returns a copy of a concentrator.
func (l *Library) Get(id string) (Concentrator, bool) {
	k, ok := l.concentrators[id]
	if !ok {
		return Concentrator{}, false
	}
	return *copyOf(k), true
}

// All returns copies of every concentrator, sorted by ID.
func (l *Library) All() []Concentrator {
	out := make([]Concentrator, 0, len(l.concentrators))
	for _, k := range l.concentrators {
		out = append(out, *copyOf(k))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Set creates a concentrator or replaces its definition. Its VNIs are kept.
// It returns the previous definition, if any.
func (l *Library) Set(k Concentrator) (*Concentrator, error) {
	if k.ID == "" {
		return nil, ErrNoConcentrator
	}
	prev, existed := l.concentrators[k.ID]
	next := copyOf(&k)
	next.VNIs = map[int]string{}
	if existed {
		for v, label := range prev.VNIs {
			next.VNIs[v] = label
		}
		prev = copyOf(prev)
	}
	l.concentrators[k.ID] = next
	return prev, nil
}

// Remove deletes a concentrator. The caller checks nothing refers to it.
func (l *Library) Remove(id string) error {
	if _, ok := l.concentrators[id]; !ok {
		return fmt.Errorf("%w: %s", ErrNoConcentrator, id)
	}
	delete(l.concentrators, id)
	return nil
}

// SetVNI adds a VNI to a concentrator, or relabels it. It returns the previous
// label, or "".
func (l *Library) SetVNI(id string, vni int, label string) (string, error) {
	k, ok := l.concentrators[id]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrNoConcentrator, id)
	}
	if vni < 1 || vni > 16777215 {
		return "", ErrBadVNI
	}
	if label == "" {
		return "", ErrNoLabel
	}
	prev := k.VNIs[vni]
	k.VNIs[vni] = label
	return prev, nil
}

// RemoveVNI deletes a VNI. The caller checks nothing refers to it.
func (l *Library) RemoveVNI(id string, vni int) error {
	k, ok := l.concentrators[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoConcentrator, id)
	}
	if _, ok := k.VNIs[vni]; !ok {
		return fmt.Errorf("%w: %d on %s", ErrNoVNI, vni, id)
	}
	delete(k.VNIs, vni)
	return nil
}

// AvailableAt reports whether a concentrator may be used at a location, given
// the location's ancestry in the Locations tree (0023).
func (k Concentrator) AvailableAt(ancestry []hierarchy.NodeID) bool {
	if len(k.Scope) == 0 {
		return true
	}
	for _, s := range k.Scope {
		for _, a := range ancestry {
			if s == a {
				return true
			}
		}
	}
	return false
}
