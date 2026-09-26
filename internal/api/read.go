package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/changelog"
	"github.com/ChristopherGriffin/aeolus/internal/compose"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

type grantView struct {
	Tree string           `json:"tree"`
	Node hierarchy.NodeID `json:"node"`
	Role string           `json:"role"`
}

func (s *Server) whoami(w http.ResponseWriter, _ *http.Request, c call) error {
	acc, _ := c.state.Access.Account(c.actor)
	grants := []grantView{}
	for _, g := range c.state.Access.Grants() {
		if g.Account == c.actor {
			grants = append(grants, grantView{Tree: g.Tree, Node: g.Node, Role: g.Role.String()})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": acc.ID, "name": acc.Name, "grants": grants})
	return nil
}

type nodeView struct {
	ID       hierarchy.NodeID `json:"id"`
	Name     string           `json:"name"`
	Kind     string           `json:"kind"`
	Parent   hierarchy.NodeID `json:"parent,omitempty"`
	Broken   bool             `json:"broken,omitempty"`
	Isolated bool             `json:"isolated,omitempty"`
}

var kindNames = map[hierarchy.Kind]string{hierarchy.KindOrg: "org", hierarchy.KindFolder: "folder", hierarchy.KindAP: "ap"}

func viewNode(n hierarchy.Node) nodeView {
	return nodeView{ID: n.ID, Name: n.Name, Kind: kindNames[n.Kind], Parent: n.Parent, Broken: n.Broken, Isolated: n.Isolated}
}

// tree lists the nodes of a tree the caller can view, root first, depth first.
func (s *Server) tree(w http.ResponseWriter, r *http.Request, c call) error {
	name, t, err := treeOf(c.state, r.PathValue("tree"))
	if err != nil {
		return err
	}
	nodes := []nodeView{}
	for _, id := range append([]hierarchy.NodeID{t.Root()}, t.Descendants(t.Root())...) {
		if roleOn(c, name, t, id) >= access.Viewer {
			n, _ := t.Node(id)
			nodes = append(nodes, viewNode(n))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tree": name, "nodes": nodes})
	return nil
}

type resolvedView struct {
	Value  any              `json:"value"`
	From   hierarchy.NodeID `json:"from"`
	Origin string           `json:"origin"`
}

var originNames = map[hierarchy.Origin]string{
	hierarchy.OriginSelf:      "self",
	hierarchy.OriginInherited: "inherited",
	hierarchy.OriginLocked:    "locked",
	hierarchy.OriginBaseline:  "baseline",
}

func viewResolved(r hierarchy.Resolved) resolvedView {
	return resolvedView{Value: mask(r.Value), From: r.From, Origin: originNames[r.Origin]}
}

func viewOverrides(ovs []hierarchy.Override) []hierarchy.Override {
	out := make([]hierarchy.Override, 0, len(ovs))
	for _, o := range ovs {
		o.Value = mask(o.Value)
		out = append(out, o)
	}
	return out
}

// node is one node's page: every value with where it comes from, the
// overrides menu (0012), the locks a break would escape, and the rules its
// config breaks, so the UI can keep an admin on the page (0029).
func (s *Server) node(w http.ResponseWriter, r *http.Request, c call) error {
	name, t, err := treeOf(c.state, r.PathValue("tree"))
	if err != nil {
		return err
	}
	id := hierarchy.NodeID(r.PathValue("node"))
	role := roleOn(c, name, t, id)
	if role < access.Viewer {
		return errNotFound
	}
	n, _ := t.Node(id)

	resolved := t.ResolveAll(id)
	fields := make(map[hierarchy.Path]resolvedView, len(resolved))
	for p, res := range resolved {
		fields[p] = viewResolved(res)
	}
	problems := compose.Node(c.state, s.schema, name, t, id, s.reveal)
	page := map[string]any{
		"tree":        name,
		"node":        viewNode(n),
		"ancestry":    t.Ancestry(id),
		"role":        role.String(),
		"fields":      fields,
		"overrides":   map[string]any{"in_effect": viewOverrides(t.OverridesInEffect(id)), "below": viewOverrides(t.OverridesBelow(id))},
		"locks_above": viewOverrides(t.LocksAbove(id)),
		"problems":    problems,
	}
	if f, ok := c.state.Facts[id]; ok && name == change.Locations {
		page["facts"] = f // what it sent when it enrolled (0033)
	}
	writeJSON(w, http.StatusOK, page)
	return nil
}

// apProblems composes an AP's config and returns the rules it breaks (0029).
func (s *Server) apProblems(state *change.State, ap hierarchy.NodeID) []string {
	res, err := compose.AP(state, s.schema, ap, s.reveal)
	if err != nil {
		return []string{err.Error()}
	}
	return res.Problems
}

func (s *Server) apConfig(w http.ResponseWriter, r *http.Request, c call) error {
	id := hierarchy.NodeID(r.PathValue("ap"))
	t := c.state.Org.Locations
	if n, ok := t.Node(id); !ok || n.Kind != hierarchy.KindAP || roleOn(c, change.Locations, t, id) < access.Viewer {
		return errNotFound
	}
	cfg, err := c.state.Org.ResolveAP(id)
	if err != nil {
		return err
	}
	location := map[hierarchy.Path]resolvedView{}
	for p, res := range cfg.Location {
		location[p] = viewResolved(res)
	}
	networks := map[string]any{}
	for nid, net := range cfg.Networks {
		fields := map[string]resolvedView{}
		for f, res := range net.Fields {
			fields[f] = viewResolved(res)
		}
		networks[nid] = map[string]any{"from": net.From, "fields": fields}
	}
	version, _ := s.log.Version(id)
	checked, err := compose.AP(c.state, s.schema, id, s.reveal)
	if err != nil {
		return err
	}
	shown, err := compose.AP(c.state, s.schema, id, nil)
	if err != nil {
		return err
	}
	problems := checked.Problems
	services := cfg.Services
	if services == nil {
		services = []hierarchy.NodeID{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ap":         id,
		"version":    version,
		"services":   services,
		"location":   location,
		"networks":   networks,
		"unassigned": checked.Unassigned,
		"document":   mask(anyMap(shown.Doc)),
		"check":      map[string]any{"ok": len(problems) == 0 && !checked.Unassigned, "problems": problems},
	})
	return nil
}

type entryView struct {
	Seq    int64         `json:"seq"`
	At     time.Time     `json:"at"`
	Actor  string        `json:"actor"`
	Reason string        `json:"reason"`
	Op     change.Op     `json:"op"`
	Effect change.Effect `json:"effect"`
}

// viewEntry masks secrets in an entry and drops token hashes.
func viewEntry(e changelog.Entry) entryView {
	op := e.Op
	op.TokenHash = nil
	if len(op.Value) > 0 {
		var v any
		if json.Unmarshal(op.Value, &v) == nil {
			op.Value, _ = json.Marshal(mask(v))
		}
	}
	eff := e.Effect
	eff.Before, eff.After = mask(eff.Before), mask(eff.After)
	eff.Removed = viewOverrides(eff.Removed)
	return entryView{Seq: e.Seq, At: e.At, Actor: e.Actor, Reason: e.Reason, Op: op, Effect: eff}
}

// changes reads the change log. It needs at least viewer at the Org root of
// either tree: the log spans the whole Org.
func (s *Server) changes(w http.ResponseWriter, r *http.Request, c call) error {
	root := c.state.Org.Locations.Root()
	if roleOn(c, change.Locations, c.state.Org.Locations, root) < access.Viewer &&
		roleOn(c, change.Services, c.state.Org.Services, root) < access.Viewer {
		return &apiError{http.StatusForbidden, "reading the change log needs viewer at the Org root"}
	}
	after, limit := int64(0), 100
	if v := r.URL.Query().Get("after"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return badRequest("after must be a sequence number")
		}
		after = n
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			return badRequest("limit must be 1 to 1000")
		}
		limit = n
	}
	entries, err := s.log.Entries(after, limit)
	if err != nil {
		return err
	}
	out := make([]entryView, 0, len(entries))
	for _, e := range entries {
		out = append(out, viewEntry(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"changes": out})
	return nil
}

// library lists the concentrators and their VNIs. Any account may read it: it
// is what the transport pull-downs offer (0023).
func (s *Server) library(w http.ResponseWriter, _ *http.Request, c call) error {
	writeJSON(w, http.StatusOK, map[string]any{"concentrators": c.state.Library.All()})
	return nil
}

// anyMap turns a nil document into an empty one for display.
func anyMap(m map[string]any) any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// reversioned lists the APs whose version is seq, sorted.
func (s *Server) reversioned(state *change.State, seq int64) []hierarchy.NodeID {
	out := []hierarchy.NodeID{}
	for _, ap := range state.Org.Locations.APs() {
		if v, _ := s.log.Version(ap); v == seq {
			out = append(out, ap)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
