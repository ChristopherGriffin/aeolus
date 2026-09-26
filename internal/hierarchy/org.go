package hierarchy

import (
	"fmt"
	"sort"
	"strings"
)

// ServicesPath is the Locations field that names the service folders an AP
// offers (0013). It inherits like any other field.
const ServicesPath Path = "services"

// networkPrefix starts every network field in the Services tree:
// "network.<id>.<field>".
const networkPrefix = "network."

// Org holds the two trees (0013).
type Org struct {
	Locations *Tree
	Services  *Tree
}

// NewOrg returns an Org with empty Locations and Services trees.
func NewOrg(id NodeID, name string) *Org {
	return &Org{
		Locations: NewTree(id, name, true),
		Services:  NewTree(id, name, false),
	}
}

// Clone returns an independent copy of the Org.
func (o *Org) Clone() *Org {
	return &Org{Locations: o.Locations.Clone(), Services: o.Services.Clone()}
}

// AssignServices sets which service folders apply at a Locations node.
func (o *Org) AssignServices(at NodeID, folders []NodeID) error {
	for _, f := range folders {
		if _, ok := o.Services.Node(f); !ok {
			return fmt.Errorf("%w: service folder %s", ErrNotFound, f)
		}
	}
	return o.Locations.Set(at, ServicesPath, append([]NodeID(nil), folders...))
}

// NetworkConflictError reports two assigned service folders that both provide
// the same network to one AP.
type NetworkConflictError struct {
	Network string
	A, B    NodeID
}

func (e *NetworkConflictError) Error() string {
	return fmt.Sprintf("network %s is provided by both %s and %s", e.Network, e.A, e.B)
}

// APConfig is everything an AP resolves to, with where each value came from.
// The manager keeps the origins for the UI; an AP only receives the values.
type APConfig struct {
	AP NodeID
	// Unassigned: the AP sits in an isolated folder such as Landing Zone and
	// gets no config (0032).
	Unassigned bool
	Location   map[Path]Resolved
	Services   []NodeID
	Networks   map[string]Network
}

// Network is one network's fields as resolved at the service folder that
// provides it.
type Network struct {
	From   NodeID
	Fields map[string]Resolved
}

// ResolveAP resolves an AP's Location fields, then the networks of every
// service folder assigned to it.
func (o *Org) ResolveAP(ap NodeID) (APConfig, error) {
	n, ok := o.Locations.Node(ap)
	if !ok || n.Kind != KindAP {
		return APConfig{}, fmt.Errorf("%w: AP %s", ErrNotFound, ap)
	}
	cfg := APConfig{AP: ap, Location: o.Locations.ResolveAll(ap), Networks: map[string]Network{}}
	if o.Locations.InIsolated(ap) {
		cfg.Unassigned = true
		return cfg, nil
	}
	delete(cfg.Location, ServicesPath)

	if r, ok := o.Locations.Resolve(ap, ServicesPath); ok {
		folders, ok := r.Value.([]NodeID)
		if !ok {
			return APConfig{}, fmt.Errorf("%s at %s is not a list of service folders", ServicesPath, r.From)
		}
		cfg.Services = folders
	}
	for _, f := range cfg.Services {
		if _, ok := o.Services.Node(f); !ok {
			return APConfig{}, fmt.Errorf("%w: service folder %s", ErrNotFound, f)
		}
		for p, r := range o.Services.ResolveAll(f) {
			id, field, ok := splitNetworkPath(p)
			if !ok {
				continue
			}
			net, seen := cfg.Networks[id]
			if seen && net.From != f {
				return APConfig{}, &NetworkConflictError{Network: id, A: net.From, B: f}
			}
			if !seen {
				net = Network{From: f, Fields: map[string]Resolved{}}
			}
			net.Fields[field] = r
			cfg.Networks[id] = net
		}
	}
	return cfg, nil
}

// NetworkIDs returns the networks in an APConfig, sorted.
func (c APConfig) NetworkIDs() []string {
	ids := make([]string, 0, len(c.Networks))
	for id := range c.Networks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func splitNetworkPath(p Path) (id, field string, ok bool) {
	rest, ok := strings.CutPrefix(string(p), networkPrefix)
	if !ok {
		return "", "", false
	}
	id, field, ok = strings.Cut(rest, ".")
	return id, field, ok && id != "" && field != ""
}
