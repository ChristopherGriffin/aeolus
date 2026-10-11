package change

import (
	"bytes"
	"fmt"
	"slices"
	"sort"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// applyAP handles an AP arriving and leaving (0033, 0038).
//
// enroll places a new AP in Landing Zone with its token and the facts it sent.
// The ID must be new: an AP already known, wherever it is, cannot enroll
// again until a person removes it, so nobody can take over an enrolled AP's
// identity by enrolling in its name.
//
// remove-ap takes an AP out of the Locations tree, with whatever was set on
// it, and revokes its tokens.
//
// facts replaces what an AP said of itself with what it says now (0122).
// Nothing else of it changes: its place, its name and its token stay.
func applyAP(s *State, op Op) (Effect, error) {
	t := s.Org.Locations
	switch op.Kind {
	case Enroll:
		if err := t.AddAP(op.Node, op.Name, LandingZone); err != nil {
			return Effect{}, err
		}
		if err := s.Access.AddAPToken(op.TokenID, op.Node, op.TokenHash); err != nil {
			t.RemoveAP(op.Node) // leave the state as it was
			return Effect{}, err
		}
		if len(bytes.TrimSpace(op.Value)) > 0 {
			s.Facts[op.Node] = append([]byte(nil), op.Value...)
		}
		return Effect{After: map[string]string{"name": op.Name, "parent": string(LandingZone), "token": op.TokenID}}, nil
	case Facts:
		if n, ok := t.Node(op.Node); !ok || n.Kind != hierarchy.KindAP {
			return Effect{}, fmt.Errorf("%w: %s", hierarchy.ErrNotFound, op.Node)
		}
		s.Facts[op.Node] = append([]byte(nil), op.Value...)
		return Effect{}, nil
	case RemoveAP:
		n, ok := t.Node(op.Node)
		if !ok {
			return Effect{}, fmt.Errorf("%w: %s", hierarchy.ErrNotFound, op.Node)
		}
		removed, err := t.RemoveAP(op.Node)
		if err != nil {
			return Effect{}, err
		}
		s.Access.RevokeAPTokens(op.Node)
		delete(s.Facts, op.Node)
		return Effect{Before: map[string]string{"name": n.Name, "parent": string(n.Parent)}, Removed: removed}, nil
	}
	return Effect{}, fmt.Errorf("%w: %q", ErrUnknownKind, op.Kind)
}

// removeFolder takes away a folder that holds nothing (0125): no folder and
// no AP below it, which the tree sees to, and nothing that still leans on
// it. A Services folder some Locations node offers stays, and one with
// per-user keys (0070); a Locations folder AP templates are made at stays
// (0085); and the built-in folders stay (0032). What was set at it goes with
// it, and the roles granted on it.
func removeFolder(s *State, op Op) (Effect, error) {
	t, err := tree(s.Org, op.Tree)
	if err != nil {
		return Effect{}, err
	}
	n, ok := t.Node(op.Node)
	if !ok {
		return Effect{}, fmt.Errorf("%w: %s", hierarchy.ErrNotFound, op.Node)
	}
	if op.Node == Sandbox || op.Node == LandingZone {
		return Effect{}, fmt.Errorf("%w: %s is a built-in folder", hierarchy.ErrNotAFolder, op.Node)
	}
	var by []string
	if op.Tree == Services {
		offers := func(at hierarchy.NodeID, p hierarchy.Path, v hierarchy.Value) {
			if list, ok := v.([]hierarchy.NodeID); ok && p == hierarchy.ServicesPath && slices.Contains(list, op.Node) && !slices.Contains(by, string(at)) {
				by = append(by, string(at))
			}
		}
		// Offered where it is set, or kept from before a break (0005).
		s.Org.Locations.EachSet(offers)
		s.Org.Locations.EachBase(offers)
		for _, k := range s.Keys.All() {
			if k.Folder == op.Node {
				by = append(by, "per-user key "+k.Name)
			}
		}
	} else {
		for _, tm := range s.Library.Templates() {
			if tm.At == op.Node {
				by = append(by, "template "+tm.Name)
			}
		}
	}
	if len(by) > 0 {
		sort.Strings(by)
		return Effect{}, &InUseError{What: "folder " + n.Name, By: by}
	}
	removed, err := t.RemoveFolder(op.Node)
	if err != nil {
		return Effect{}, err
	}
	for _, g := range s.Access.Grants() {
		if g.Tree == string(op.Tree) && g.Node == op.Node {
			s.Access.Revoke(g)
		}
	}
	return Effect{Before: map[string]string{"name": n.Name, "parent": string(n.Parent)}, Removed: removed}, nil
}
