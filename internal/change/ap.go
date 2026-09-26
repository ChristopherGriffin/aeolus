package change

import (
	"bytes"
	"fmt"

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
