package api

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/ChristopherGriffin/aeolus/internal/bundle"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Fleet updates (0079): which agent bundle each AP should run, and the
// bundles themselves.

// AgentPath is the Locations field that says which agent a folder's APs run:
// "current", the manager's own release, unless set, or a release it keeps.
const AgentPath hierarchy.Path = "system.agent"

// AgentHeader names, on every answer to an AP's poll, the bundle it should
// run.
const AgentHeader = "Aeolus-Agent"

// WithAgents gives the server the bundles it offers its APs: the store it
// keeps them in, and its own release's.
func (s *Server) WithAgents(store *bundle.Store, own bundle.Bundle) *Server {
	s.agents, s.agent = store, own
	return s
}

// agentPin is the release an AP's folder pins, or "current".
func agentPin(st *change.State, ap hierarchy.NodeID) string {
	if r, ok := st.Org.Locations.Resolve(ap, AgentPath); ok {
		var pin string
		switch v := r.Value.(type) {
		case string:
			pin = v
		case json.RawMessage:
			_ = json.Unmarshal(v, &pin)
		}
		if pin != "" {
			return pin
		}
	}
	return "current"
}

// agentFor is the bundle an AP should run, and the pin that names it. ok is
// false where the pin names a release the manager no longer keeps: the AP
// is told nothing, and keeps what it runs.
func (s *Server) agentFor(st *change.State, ap hierarchy.NodeID) (b bundle.Bundle, pin string, ok bool) {
	if s.agents == nil {
		return bundle.Bundle{}, "", false
	}
	pin = agentPin(st, ap)
	if pin == "current" {
		return s.agent, pin, true
	}
	b, ok = s.agents.Find(pin)
	return b, pin, ok
}

// setAgentHeader names the bundle an AP should run on an answer to it.
func (s *Server) setAgentHeader(w http.ResponseWriter, ap hierarchy.NodeID) {
	st := s.log.Snapshot()
	if st == nil {
		return
	}
	if b, _, ok := s.agentFor(st, ap); ok {
		w.Header().Set(AgentHeader, b.Hash)
	}
}

// agentManifest answers an AP with the manifest of the bundle it should run.
func (s *Server) agentManifest(w http.ResponseWriter, _ *http.Request, c apCall) error {
	b, _, ok := s.agentFor(s.log.Snapshot(), c.ap)
	if !ok {
		return errNotFound
	}
	writeJSON(w, http.StatusOK, b)
	return nil
}

// agentFile answers an AP with one of its bundle's files, by SHA-256.
func (s *Server) agentFile(w http.ResponseWriter, r *http.Request, c apCall) error {
	b, _, ok := s.agentFor(s.log.Snapshot(), c.ap)
	sha := r.PathValue("sha256")
	if !ok || !bundleHas(b, sha) {
		return errNotFound
	}
	data, err := s.agents.File(sha)
	if errors.Is(err, fs.ErrNotExist) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	_, err = w.Write(data)
	return err
}

func bundleHas(b bundle.Bundle, sha string) bool {
	for _, f := range b.Files {
		if f.SHA256 == sha {
			return true
		}
	}
	return false
}

// agentVersions lists the releases the manager keeps, newest first, for the
// Agent version setting: current is the manager's own.
func (s *Server) agentVersions(w http.ResponseWriter, _ *http.Request, _ call) error {
	out := []map[string]any{}
	if s.agents != nil {
		all, err := s.agents.Versions()
		if err != nil {
			return err
		}
		for _, b := range all {
			out = append(out, map[string]any{"version": b.Version, "hash": b.Hash, "stored": b.Stored, "current": b.Hash == s.agent.Hash})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"current": s.agent.Version, "versions": out})
	return nil
}

// checkAgentPin refuses a pin to a release the manager doesn't keep, when it
// is set; replaying old changes never checks, so a release since pruned
// stays a valid past change.
func (s *Server) checkAgentPin(raw json.RawMessage) error {
	var pin string
	if err := json.Unmarshal(raw, &pin); err != nil || pin == "current" || s.agents == nil {
		return nil // the schema says what form it takes
	}
	if _, ok := s.agents.Find(pin); ok {
		return nil
	}
	held := []string{"current"}
	if all, err := s.agents.Versions(); err == nil {
		for _, b := range all {
			held = append(held, b.Version)
		}
	}
	return badRequest("the manager keeps no agent release %s; it has %s", pin, strings.Join(held, ", "))
}

// agentState is the agent an AP says it runs (0079): the release and bundle
// it last confirmed, and its last update that is under way, failed or was
// rolled back, with why.
type agentState struct {
	Version string       `json:"version"`
	Hash    string       `json:"hash"`
	Update  *agentUpdate `json:"update,omitempty"`
}

type agentUpdate struct {
	Version string `json:"version"`
	Hash    string `json:"hash"`
	State   string `json:"state"`
	Why     string `json:"why,omitempty"`
	Ago     int64  `json:"ago"`
}

var (
	bundleHashRE   = regexp.MustCompile(`^([0-9a-f]{64})?$`)
	releaseRE      = regexp.MustCompile(`^[A-Za-z0-9._-]{0,64}$`)
	updateStates   = map[string]bool{"updating": true, "failed": true, "rolled-back": true}
	errAgentReport = badRequest("agent: a release, a bundle hash of 64 hex digits, and an update that is updating, failed or rolled-back, with why in at most 200 characters")
)

func (a *agentState) check() error {
	if a == nil {
		return nil
	}
	if !releaseRE.MatchString(a.Version) || !bundleHashRE.MatchString(a.Hash) {
		return errAgentReport
	}
	if u := a.Update; u != nil {
		if !releaseRE.MatchString(u.Version) || !bundleHashRE.MatchString(u.Hash) || !updateStates[u.State] || u.Ago < 0 {
			return errAgentReport
		}
		if err := plainText("agent.update.why", u.Why, 200); err != nil {
			return err
		}
	}
	return nil
}

// agentView is an AP's agent for the fleet and config views: what it says it
// runs, the bundle it should, and where its folder pins a release the
// manager no longer keeps, that pin.
func (s *Server) agentView(st *change.State, ap hierarchy.NodeID, report json.RawMessage) map[string]any {
	if s.agents == nil {
		return nil
	}
	out := map[string]any{}
	var r struct {
		Agent *agentState `json:"agent"`
	}
	if len(report) > 0 {
		if err := json.Unmarshal(report, &r); err != nil {
			slog.Error("reading an AP's state report", "ap", ap, "err", err)
		}
	}
	out["runs"] = r.Agent
	b, pin, ok := s.agentFor(st, ap)
	if ok {
		out["wants"] = map[string]any{"version": b.Version, "hash": b.Hash}
	} else {
		out["missing_pin"] = pin
	}
	out["pin"] = pin
	return out
}
