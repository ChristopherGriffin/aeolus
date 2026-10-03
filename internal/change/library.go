package change

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/library"
)

// concentratorDef is the body of a set-concentrator change. VNIs are managed
// with set-vni and remove-vni.
type concentratorDef struct {
	Name    string             `json:"name"`
	Address string             `json:"address"`
	Port    int                `json:"port"`
	MTU     int                `json:"mtu"`
	Scope   []hierarchy.NodeID `json:"scope,omitempty"`
}

// TransportField splits a network transport path,
// "network.<id>.transport.<primary|fallback>.<field>".
func TransportField(p hierarchy.Path) (network, slot, field string, ok bool) {
	parts := strings.Split(string(p), ".")
	if len(parts) != 5 || parts[0] != "network" || parts[2] != "transport" {
		return "", "", "", false
	}
	if parts[3] != "primary" && parts[3] != "fallback" {
		return "", "", "", false
	}
	return parts[1], parts[3], parts[4], true
}

func applyLibrary(s *State, op Op) (Effect, error) {
	lib := s.Library
	switch op.Kind {
	case SetConcentrator:
		var def concentratorDef
		dec := json.NewDecoder(bytes.NewReader(op.Value))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&def); err != nil {
			return Effect{}, fmt.Errorf("concentrator: %w", err)
		}
		for _, f := range def.Scope {
			if n, ok := s.Org.Locations.Node(f); !ok || n.Kind == hierarchy.KindAP {
				return Effect{}, fmt.Errorf("%w: scope folder %s", hierarchy.ErrNotFound, f)
			}
		}
		prev, err := lib.Set(library.Concentrator{ID: op.Concentrator, Name: def.Name, Address: def.Address, Port: def.Port, MTU: def.MTU, Scope: def.Scope})
		if err != nil {
			return Effect{}, err
		}
		eff := Effect{After: def}
		if prev != nil {
			eff.Before = concentratorDef{Name: prev.Name, Address: prev.Address, Port: prev.Port, MTU: prev.MTU, Scope: prev.Scope}
		}
		return eff, nil
	case RemoveConcentrator:
		k, ok := lib.Get(op.Concentrator)
		if !ok {
			return Effect{}, fmt.Errorf("%w: %s", library.ErrNoConcentrator, op.Concentrator)
		}
		if by := concentratorUsers(s, op.Concentrator); len(by) > 0 {
			return Effect{}, &InUseError{What: "concentrator " + op.Concentrator, By: by}
		}
		return Effect{Before: k}, lib.Remove(op.Concentrator)
	case SetVNI:
		prev, err := lib.SetVNI(op.Concentrator, op.VNI, op.Name)
		if err != nil {
			return Effect{}, err
		}
		eff := Effect{After: op.Name}
		if prev != "" {
			eff.Before = prev
		}
		return eff, nil
	case RemoveVNI:
		k, ok := lib.Get(op.Concentrator)
		if !ok {
			return Effect{}, fmt.Errorf("%w: %s", library.ErrNoConcentrator, op.Concentrator)
		}
		if by := vniUsers(s, op.Concentrator, op.VNI); len(by) > 0 {
			return Effect{}, &InUseError{What: fmt.Sprintf("VNI %d on %s", op.VNI, op.Concentrator), By: by}
		}
		return Effect{Before: k.VNIs[op.VNI]}, lib.RemoveVNI(op.Concentrator, op.VNI)
	}
	return Effect{}, fmt.Errorf("%w: %q", ErrUnknownKind, op.Kind)
}

// concentratorUsers lists the Services nodes whose networks name a concentrator.
func concentratorUsers(s *State, id string) []string {
	var by []string
	s.Org.Services.EachSet(func(n hierarchy.NodeID, p hierarchy.Path, v hierarchy.Value) {
		if _, _, field, ok := TransportField(p); ok && field == "concentrator" && v == id {
			by = append(by, string(n)+" "+string(p))
		}
	})
	sort.Strings(by)
	return by
}

// vniUsers lists the Services nodes that set a VNI which, where it is set,
// goes to the given concentrator.
func vniUsers(s *State, id string, vni int) []string {
	var by []string
	t := s.Org.Services
	t.EachSet(func(n hierarchy.NodeID, p hierarchy.Path, v hierarchy.Value) {
		network, slot, field, ok := TransportField(p)
		if !ok || field != "vni" || v != float64(vni) {
			return
		}
		conc, ok := t.Resolve(n, hierarchy.Path("network."+network+".transport."+slot+".concentrator"))
		if ok && conc.Value == id {
			by = append(by, string(n)+" "+string(p))
		}
	})
	sort.Strings(by)
	return by
}
