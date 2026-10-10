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
	"github.com/ChristopherGriffin/aeolus/internal/library"
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
	hierarchy.OriginTemplate:  "template",
}

// templateView says which AP template an AP takes (0085) and whether it
// follows it: the template's fields that something set closer to the AP,
// or locked, replaces, each with where. An AP with no template has none.
// It also says the AP's board and model, which its kind of AP's own
// settings are by (0092).
func templateView(state *change.State, ap hierarchy.NodeID, use *hierarchy.TemplateUse) map[string]any {
	out := map[string]any{"board": state.Board(ap), "model": state.Model(ap), "id": nil}
	if use == nil {
		return out
	}
	out["id"], out["name"], out["at"] = use.ID, use.Name, use.At
	out["replaced"] = viewOverrides(use.Replaced)
	out["follows"] = len(use.Replaced) == 0
	return out
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
	// An AP's page shows what its template gives it, where the template
	// wins (0085).
	if n.Kind == hierarchy.KindAP && name == change.Locations {
		if cfg, err := c.state.ResolveAP(id); err == nil && cfg.Template != nil {
			for p, res := range cfg.Location {
				if res.Origin == hierarchy.OriginTemplate {
					fields[p] = viewResolved(res)
				}
			}
		}
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
	if name == change.Locations && !n.Isolated {
		hw, err := s.hardware(c.state, id)
		if err != nil {
			return err
		}
		page["hardware"] = hw // what its radios can be set to (0044)
	}
	writeJSON(w, http.StatusOK, page)
	return nil
}

// apProblems composes an AP's config and returns the rules it breaks (0029).
func (s *Server) apProblems(state *change.State, ap hierarchy.NodeID) []string {
	res, err := s.compose(state, ap, s.reveal)
	if err != nil {
		return []string{err.Error()}
	}
	return res.Problems
}

// compose composes an AP's config, and holds what the AP said it cannot
// run, by its last state report: a tunnel its uplink cannot carry now
// (0056), a tunnel while netifd has not loaded vxlan, and band steering or
// BSS transition while hostapd lacks 802.11v (0057). What the AP has not
// reported is left to the render check and to the apply.
func (s *Server) compose(state *change.State, ap hierarchy.NodeID, reveal compose.Reveal) (compose.Result, error) {
	res, err := compose.AP(state, s.schema, ap, reveal)
	if err != nil || res.Unassigned {
		return res, err
	}
	st, err := s.conds.LatestState(ap)
	if err != nil || st == nil {
		return res, err
	}
	var report struct {
		VXLAN *struct {
			UplinkMTU int   `json:"uplink_mtu"`
			Loaded    *bool `json:"loaded"`
		} `json:"vxlan"`
		Steering *struct {
			BSSTransition *bool `json:"bss_transition"`
		} `json:"steering"`
	}
	if json.Unmarshal(st.Report, &report) != nil {
		return res, nil
	}
	var r compose.Reported
	if v := report.VXLAN; v != nil {
		r.UplinkMTU, r.VXLANLoaded = v.UplinkMTU, v.Loaded
	}
	if g := report.Steering; g != nil {
		r.BSSTransition = g.BSSTransition
	}
	res.Problems = append(res.Problems, compose.ReportedProblems(res.Doc, r)...)
	return res, nil
}

func (s *Server) apConfig(w http.ResponseWriter, r *http.Request, c call) error {
	id := hierarchy.NodeID(r.PathValue("ap"))
	t := c.state.Org.Locations
	if n, ok := t.Node(id); !ok || n.Kind != hierarchy.KindAP || roleOn(c, change.Locations, t, id) < access.Viewer {
		return errNotFound
	}
	cfg, err := c.state.ResolveAP(id)
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
	checked, err := s.compose(c.state, id, s.reveal)
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
	cond, err := s.condition(id, version)
	if err != nil {
		return err
	}
	// The per-user keys it should have (0070), to compare with the version
	// its state report says it has.
	refs, keyVersion := keyRefs(c.state, id, time.Now())
	keyCount := 0
	for _, n := range refs {
		keyCount += len(n.Keys)
	}
	var report json.RawMessage
	if l, err := s.conds.Latest(id); err == nil && l.State != nil {
		report = l.State.Report
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"agent":      s.agentView(c.state, id, report),
		"keys":       map[string]any{"version": keyVersion, "count": keyCount},
		"ap":         id,
		"version":    version,
		"services":   services,
		"template":   templateView(c.state, id, cfg.Template),
		"location":   location,
		"networks":   networks,
		"unassigned": checked.Unassigned,
		"document":   mask(anyMap(shown.Doc)),
		"check":      map[string]any{"ok": len(problems) == 0 && !checked.Unassigned, "problems": problems},
		"condition":  cond,
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
		op.Value = maskRaw(op.Value)
	}
	if len(op.Values) > 0 {
		values := make(map[hierarchy.Path]json.RawMessage, len(op.Values))
		for p, raw := range op.Values {
			values[p] = maskRaw(raw)
		}
		op.Values = values
	}
	eff := e.Effect
	eff.Before, eff.After = mask(eff.Before), mask(eff.After)
	eff.Removed = viewOverrides(eff.Removed)
	return entryView{Seq: e.Seq, At: e.At, Actor: e.Actor, Reason: e.Reason, Op: op, Effect: eff}
}

// maskRaw masks secrets in a JSON value. A value that is not JSON is kept.
func maskRaw(raw json.RawMessage) json.RawMessage {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return raw
	}
	out, _ := json.Marshal(mask(v))
	return out
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
// library lists the AP templates (0085). With ?at=, those offered at that
// Locations node, made there or above it, nearest first; without, those made
// where the caller may view. Each has the APs the caller may view that take
// it, and whether each follows it, or has fields of it replaced; and
// whether the caller may change it.
func (s *Server) library(w http.ResponseWriter, r *http.Request, c call) error {
	st, t := c.state, c.state.Org.Locations
	var list []library.Template
	if at := hierarchy.NodeID(r.URL.Query().Get("at")); at != "" {
		if _, ok := t.Node(at); !ok || roleOn(c, change.Locations, t, at) < access.Viewer {
			return errNotFound
		}
		anc := t.Ancestry(at)
		level := map[hierarchy.NodeID]int{}
		for i, n := range anc {
			level[n] = len(anc) - i // the node itself first
		}
		for _, tm := range st.Library.Templates() {
			if level[tm.At] > 0 {
				list = append(list, tm)
			}
		}
		sort.SliceStable(list, func(i, j int) bool { return level[list[i].At] < level[list[j].At] })
	} else {
		for _, tm := range st.Library.Templates() {
			if roleOn(c, change.Locations, t, tm.At) >= access.Viewer {
				list = append(list, tm)
			}
		}
	}
	takes := map[string][]map[string]any{}
	for _, ap := range t.APs() {
		if roleOn(c, change.Locations, t, ap) < access.Viewer {
			continue
		}
		cfg, err := st.ResolveAP(ap)
		if err != nil || cfg.Template == nil {
			continue
		}
		takes[cfg.Template.ID] = append(takes[cfg.Template.ID], map[string]any{"ap": ap, "follows": len(cfg.Template.Replaced) == 0})
	}
	out := make([]map[string]any, 0, len(list))
	for _, tm := range list {
		values := map[hierarchy.Path]any{}
		for p, v := range tm.Values {
			values[p] = mask(v)
		}
		aps := takes[tm.ID]
		if aps == nil {
			aps = []map[string]any{}
		}
		out = append(out, map[string]any{"id": tm.ID, "name": tm.Name, "at": tm.At, "boards": tm.Boards, "values": values, "aps": aps,
			"can_edit": roleOn(c, change.Locations, t, tm.At) >= access.Operator})
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": out})
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

// describe tells a client the fields it can set, so it can offer them for
// editing (0048). Anyone signed in may read it: it is the same for everyone.
func (s *Server) describe(w http.ResponseWriter, r *http.Request, c call) error {
	writeJSON(w, http.StatusOK, s.schema.Describe())
	return nil
}
