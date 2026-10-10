package change

import (
	"slices"
	"sort"
	"strings"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Settings for one kind of AP (0092): a Locations field under
// boards.<board>., such as boards.arista,c360.ports.eth1.mode, is the plain
// field (ports.eth1.mode) for the APs of that board alone, so a folder can
// set the ports of each kind of AP it holds without one kind's settings
// reaching another's ports of the same name. It inherits like any field.
// An AP of the board takes it in place of the plain one set on the same
// node or above; a plain one set closer to the AP, or locked above, stays.
// Ports only, for now.

// BoardsPrefix starts the Locations fields for one kind of AP.
const BoardsPrefix = "boards."

// BoardPath is p for the APs of one board.
func BoardPath(board string, p hierarchy.Path) hierarchy.Path {
	return hierarchy.Path(BoardsPrefix + board + "." + string(p))
}

// foldBoards puts the board's fields in place of the plain ones they
// replace, and takes every board's out of the AP's settings: none is sent
// to an AP as it is. chain is the AP and the nodes it inherits from,
// nearest first.
func foldBoards(cfg *hierarchy.APConfig, board string, chain []hierarchy.NodeID) {
	prefix := BoardsPrefix + board + "."
	var mine []hierarchy.Path
	for p := range cfg.Location {
		if !strings.HasPrefix(string(p), BoardsPrefix) {
			continue
		}
		if board != "" && strings.HasPrefix(string(p), prefix) {
			mine = append(mine, p)
			continue
		}
		delete(cfg.Location, p)
	}
	sort.Slice(mine, func(i, j int) bool { return mine[i] < mine[j] })
	for _, p := range mine {
		r := cfg.Location[p]
		delete(cfg.Location, p)
		plain := hierarchy.Path(strings.TrimPrefix(string(p), prefix))
		if cur, set := cfg.Location[plain]; set {
			if cur.Origin == hierarchy.OriginLocked && cur.From != r.From {
				continue // a lock above holds
			}
			if i := slices.Index(chain, cur.From); i >= 0 && i < slices.Index(chain, r.From) {
				continue // set closer to the AP
			}
		}
		cfg.Location[plain] = r
	}
}
