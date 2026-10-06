// Package api is the manager's one door (0003, 0026): JSON over HTTPS. The
// UI, the CLI and the MCP adapter all use it; none has a private path into
// the database.
//
// Every request except /healthz and /v1/enroll carries "Authorization:
// Bearer <token>": an account's token, or on the /v1/ap/ routes an AP's
// (0038).
// Every write is one change, logged with who made it and when, and with a
// note if the caller gives one (0062); it is authorized and checked at commit
// time (0029, 0030). Secrets are never returned in plain text (0027), and a
// node the caller cannot view answers 404, not 403, so the API does not
// reveal what exists.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/bundle"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/changelog"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/dhcpwatch"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/keys"
	"github.com/ChristopherGriffin/aeolus/internal/library"
	"github.com/ChristopherGriffin/aeolus/internal/schema"
	"github.com/ChristopherGriffin/aeolus/internal/secret"
)

// Server serves the API.
type Server struct {
	log    *changelog.Log
	schema *schema.Schema
	box    *secret.Box
	conds  *conditions.Store
	watch  *dhcpwatch.Book // what the manager's DHCP listeners hear (0068); nil if none
	psks   pskCache        // per-user keys' PSKs, worked out (0070)
	agents *bundle.Store   // the agent bundles it offers its APs (0079); nil if none
	agent  bundle.Bundle   // its own release's bundle
}

// New returns a Server. The log should be opened with Check(sch) as its
// commit check. conds records what APs say and do (0039).
func New(log *changelog.Log, sch *schema.Schema, box *secret.Box, conds *conditions.Store) *Server {
	return &Server{log: log, schema: sch, box: box, conds: conds}
}

// Check is the commit check every change passes: permissions against the
// live state (0030), then the field schema (0027).
func Check(sch *schema.Schema) func(*change.State, string, change.Op) error {
	return func(state *change.State, actor string, op change.Op) error {
		if err := change.Authorize(state, actor, op); err != nil {
			return err
		}
		if err := change.Guard(op); err != nil {
			return err
		}
		return sch.CheckOp(op)
	}
}

// Handler returns the API's routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", root)
	mux.HandleFunc("GET /healthz", s.health)
	mux.Handle("GET /v1/whoami", s.auth(s.whoami))
	mux.Handle("GET /v1/trees/{tree}", s.auth(s.tree))
	mux.Handle("GET /v1/trees/{tree}/nodes/{node}", s.auth(s.node))
	mux.Handle("GET /v1/aps", s.auth(s.aps))
	mux.Handle("GET /v1/aps/{ap}/config", s.auth(s.apConfig))
	mux.Handle("GET /v1/aps/{ap}/history", s.auth(s.apHistory))
	mux.Handle("GET /v1/changes", s.auth(s.changes))
	mux.Handle("GET /v1/library", s.auth(s.library))
	mux.Handle("GET /v1/schema", s.auth(s.describe))
	mux.Handle("GET /v1/dhcp/relayed", s.auth(s.relayed))
	mux.Handle("GET /v1/detected", s.auth(s.detected))
	mux.Handle("GET /v1/keys", s.auth(s.keyList))
	mux.Handle("POST /v1/changes", s.auth(s.commit))
	mux.Handle("POST /v1/preview", s.auth(s.preview))
	mux.Handle("POST /v1/tokens", s.auth(s.issueToken))
	mux.Handle("DELETE /v1/tokens/{id}", s.auth(s.revokeToken))

	// The routes APs use (0033, 0038).
	mux.HandleFunc("POST /v1/enroll", s.enroll)
	mux.Handle("GET /v1/ap/config", s.apAuth(s.apPoll))
	mux.Handle("GET /v1/ap/agent", s.apAuth(s.agentManifest))
	mux.Handle("GET /v1/ap/agent/files/{sha256}", s.apAuth(s.agentFile))
	mux.Handle("GET /v1/agent/versions", s.auth(s.agentVersions))
	mux.Handle("POST /v1/ap/render", s.apAuth(s.render))
	mux.Handle("POST /v1/ap/applied", s.apAuth(s.applied))
	mux.Handle("POST /v1/ap/state", s.apAuth(s.state))
	mux.Handle("GET /v1/ap/keys", s.apAuth(s.apKeys))
	return mux
}

// call is one authenticated request: who is asking, and the state they see.
type call struct {
	actor access.AccountID
	state *change.State
}

type handler func(w http.ResponseWriter, r *http.Request, c call) error

func (s *Server) auth(h handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		state := s.log.Snapshot()
		if !ok || state == nil {
			writeError(w, http.StatusUnauthorized, "missing or invalid token")
			return
		}
		who, err := state.Access.Authenticate(strings.TrimSpace(token))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "missing or invalid token")
			return
		}
		if err := h(w, r, call{actor: who, state: state}); err != nil {
			s.fail(w, r, string(who), err)
		}
	})
}

// fail answers with the error's status. An unexpected error is logged, and
// the caller told only that something went wrong.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, who string, err error) {
	code := status(err)
	if code == http.StatusInternalServerError {
		slog.Error("api", "method", r.Method, "path", r.URL.Path, "caller", who, "err", err)
		writeError(w, code, "internal error")
		return
	}
	writeError(w, code, err.Error())
}

// apiError is an error with its own status.
type apiError struct {
	code int
	msg  string
}

func (e *apiError) Error() string { return e.msg }

var errNotFound = &apiError{http.StatusNotFound, "not found"}

func badRequest(format string, args ...any) error {
	return &apiError{http.StatusBadRequest, fmt.Sprintf(format, args...)}
}

func status(err error) int {
	var ae *apiError
	var le *hierarchy.LockedError
	var be *changelog.APBreakError
	var fe *schema.FieldError
	var ce *hierarchy.NetworkConflictError
	switch {
	case errors.As(err, &ae):
		return ae.code
	case errors.Is(err, change.ErrForbidden), errors.Is(err, change.ErrUnknownActor):
		return http.StatusForbidden
	case errors.Is(err, change.ErrFull):
		return http.StatusServiceUnavailable
	case errors.Is(err, hierarchy.ErrNotFound), errors.Is(err, access.ErrNoAccount),
		errors.Is(err, access.ErrNoToken), errors.Is(err, access.ErrNoGrant):
		return http.StatusNotFound
	case errors.Is(err, library.ErrNoConcentrator), errors.Is(err, library.ErrNoVNI), errors.Is(err, keys.ErrNoKey):
		return http.StatusNotFound
	case errors.Is(err, change.ErrBadKey):
		return http.StatusBadRequest
	case errors.Is(err, keys.ErrKeyExists), errors.Is(err, keys.ErrTooMany):
		return http.StatusConflict
	case errors.As(err, &le), errors.As(err, &be), errors.As(err, &ce), errors.Is(err, change.ErrInUse),
		errors.Is(err, hierarchy.ErrExists), errors.Is(err, access.ErrExists), errors.Is(err, access.ErrRevoked):
		return http.StatusConflict
	case errors.As(err, &fe),
		errors.Is(err, change.ErrUnknownKind), errors.Is(err, change.ErrUnknownTree),
		errors.Is(err, change.ErrNoValue), errors.Is(err, change.ErrNoNode), errors.Is(err, change.ErrNoPath), errors.Is(err, change.ErrTwoForms),
		errors.Is(err, change.ErrNoAccount), errors.Is(err, change.ErrNoTokenID), errors.Is(err, change.ErrUseAssign),
		errors.Is(err, change.ErrNotAFolder), errors.Is(err, access.ErrBadRole), errors.Is(err, change.ErrNoConcID),
		errors.Is(err, library.ErrBadVNI), errors.Is(err, library.ErrNoLabel), errors.Is(err, change.ErrBuiltins),
		errors.Is(err, change.ErrReserved), errors.Is(err, hierarchy.ErrIsolated),
		errors.Is(err, hierarchy.ErrBadParent), errors.Is(err, hierarchy.ErrAPsNotHere), errors.Is(err, hierarchy.ErrMoveRoot),
		errors.Is(err, hierarchy.ErrCycle), errors.Is(err, hierarchy.ErrBreakRoot), errors.Is(err, hierarchy.ErrBroken),
		errors.Is(err, hierarchy.ErrNotSetHere), errors.Is(err, hierarchy.ErrLockOnValue), errors.Is(err, hierarchy.ErrNotLocked),
		errors.Is(err, hierarchy.ErrNotAnAP), errors.Is(err, access.ErrBadTokenArg):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("api: encoding response", "err", err)
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// readJSON decodes a request body strictly: unknown fields are errors.
func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return badRequest("request body: %v", err)
	}
	return nil
}

// root points a visitor, or a browser, at what is here.
func root(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "aeolus", "api": "/v1", "health": "/healthz", "mcp": "/mcp",
		"docs": "https://github.com/ChristopherGriffin/aeolus/blob/main/docs/api.md",
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "seq": s.log.Seq()})
}

// mask replaces sealed secrets, wherever they sit in a value, with a marker.
func mask(v any) any {
	if secret.IsSealed(v) {
		return map[string]any{"sealed": true}
	}
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = mask(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = mask(e)
		}
		return out
	}
	return v
}

// reveal opens a sealed value with the manager's key, for checks only.
func (s *Server) reveal(path string, v any) (any, error) {
	if s.box == nil {
		return v, nil
	}
	return s.box.Open(path, v)
}

// roleOn is the caller's role on a node, None if it does not exist.
func roleOn(c call, tree change.TreeName, t *hierarchy.Tree, node hierarchy.NodeID) access.Role {
	chain := t.Ancestry(node)
	if chain == nil {
		return access.None
	}
	return c.state.Access.RoleAt(c.actor, string(tree), chain)
}

func treeOf(state *change.State, name string) (change.TreeName, *hierarchy.Tree, error) {
	switch change.TreeName(name) {
	case change.Locations:
		return change.Locations, state.Org.Locations, nil
	case change.Services:
		return change.Services, state.Org.Services, nil
	}
	return "", nil, errNotFound
}
