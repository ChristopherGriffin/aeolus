package api

import (
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// A round of tests, asked now (0129): every AP below a Locations node
// tests each thing it can, once, as soon as it can, whatever its schedule
// or none. It is what someone asks who has just built a site, or changed
// it. The manager asks the tests as it asks those on a schedule (0123),
// one of an AP at a time; what a round has not had asked since comes
// before the schedule's turn.

// roundLife is how long a round is worked at. What is left of it then, as
// of an AP that did not call in, is let go: an AP that comes back a day
// later is not asked for a round nobody is waiting for.
const roundLife = time.Hour

// round is one asked: at which node, when, by the manager's clock, by
// whom, and for which tests (0131): through the air, over the wire, or
// both.
type round struct {
	Node hierarchy.NodeID `json:"node"`
	At   time.Time        `json:"at"`
	By   string           `json:"by,omitempty"`
	Over string           `json:"over"`
}

// rounds are those being worked at, the latest asked at each node. They
// are kept in memory: a manager that restarts forgets them, and whoever
// asked asks again.
type rounds struct {
	mu sync.Mutex
	at map[hierarchy.NodeID]round
}

// ask notes a round asked at a node now, for the tests over says, in place
// of one asked there before.
func (r *rounds) ask(node hierarchy.NodeID, by string, now time.Time, over string) round {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.at == nil {
		r.at = map[hierarchy.NodeID]round{}
	}
	for id, old := range r.at {
		if now.Sub(old.At) > roundLife {
			delete(r.at, id)
		}
	}
	one := round{Node: node, At: now, By: by, Over: over}
	r.at[node] = one
	return one
}

// of is the round an AP is in: the latest still worked at that was asked at
// the AP or at a folder above it.
func (r *rounds) of(t *hierarchy.Tree, ap hierarchy.NodeID, now time.Time) (round, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.at) == 0 {
		return round{}, false
	}
	var best round
	found := false
	for _, id := range append(t.Ancestry(ap), ap) {
		if one, ok := r.at[id]; ok && now.Sub(one.At) <= roundLife && (!found || one.At.After(best.At)) {
			best, found = one, true
		}
	}
	return best, found
}

// roundView is a round as a person sees it: of how many things, how many
// have been asked for since, and how many of those are done. Its folder's
// name, and who asked it, are for one who may view that folder.
type roundView struct {
	round
	Name  string `json:"name,omitempty"` // its node's, for one who may view that node
	Of    int    `json:"of"`
	Asked int    `json:"asked"`
	Done  int    `json:"done"`
}

// askRound asks every AP below a Locations node for a round of tests, now
// (0129): POST /v1/assurance/round, {under, over?}. over says which tests
// (0131): air, wire, or both, which it is unless said. It needs operator
// on the node. It is refused where no AP there can make a test of those.
func (s *Server) askRound(w http.ResponseWriter, r *http.Request, c call) error {
	var body struct {
		Under string `json:"under"`
		Over  string `json:"over"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if body.Over == "" {
		body.Over = overBoth
	}
	if body.Over != overAir && body.Over != overWire && body.Over != overBoth {
		return badRequest("over: air, wire or both")
	}
	t := c.state.Org.Locations
	node := hierarchy.NodeID(body.Under)
	if _, ok := t.Node(node); !ok || roleOn(c, change.Locations, t, node) < access.Viewer {
		return errNotFound
	}
	if roleOn(c, change.Locations, t, node) < access.Operator {
		return &apiError{http.StatusForbidden, "asking for a round of tests needs operator here"}
	}
	docs := map[hierarchy.NodeID]map[string]any{}
	things := 0
	for _, ap := range t.APs() {
		if ap == node || slices.Contains(t.Ancestry(ap), node) {
			things += len(s.roundCombos(c.state, ap, docs, body.Over))
		}
	}
	if things == 0 {
		switch body.Over {
		case overAir:
			return badRequest("no AP here can test anything through the air: one needs a scan radio, and a network a test can join")
		case overWire:
			return badRequest("no AP here can test anything over the wire: one needs an agent that tests over the wire, and a network to test")
		}
		return badRequest("no AP here can test anything: one needs a scan radio, or an agent that tests over the wire, and a network to test")
	}
	// By the clock the tests are stamped with: what a round has had asked
	// is told by comparing the two.
	one := s.rounds.ask(node, string(c.actor), s.conds.Now(), body.Over)
	slog.Info("assurance: a round asked", "under", node, "by", c.actor, "over", body.Over, "things", things)
	writeJSON(w, http.StatusOK, map[string]any{"round": one, "things": things})
	return nil
}
