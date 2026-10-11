package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// ActionsHeader tells an AP, on every config poll, how many actions wait
// for it (0104), so it asks for them only when there are some.
const ActionsHeader = "Aeolus-Actions"

// actionKinds are what a person may ask an AP to do once (0104), and
// whether each is of a client, by MAC, its target (0107).
var actionKinds = map[string]bool{"locate": false, "restart-wifi": false, "reboot": false, "disconnect": true, "client-test": false}

// addAction asks an AP to do something once (0104): POST /v1/aps/{ap}/actions
// {kind, target?}, target being the client's MAC for disconnect (0107). A
// client-test (0121) says what to test instead: {kind, network, on?,
// band?}, one of the AP's networks, the AP whose networks to join, and a
// band. It needs operator on the AP, as changing it does (0030).
func (s *Server) addAction(w http.ResponseWriter, r *http.Request, c call) error {
	id := hierarchy.NodeID(r.PathValue("ap"))
	t := c.state.Org.Locations
	if n, ok := t.Node(id); !ok || n.Kind != hierarchy.KindAP || roleOn(c, change.Locations, t, id) < access.Viewer {
		return errNotFound
	}
	if roleOn(c, change.Locations, t, id) < access.Operator {
		return &apiError{http.StatusForbidden, "asking an AP to act needs operator on it"}
	}
	var body struct {
		Kind    string `json:"kind"`
		Target  string `json:"target"`
		Network string `json:"network"`
		On      string `json:"on"`
		Band    string `json:"band"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	ofClient, ok := actionKinds[body.Kind]
	if !ok {
		return badRequest("an action is locate, restart-wifi, reboot, disconnect or client-test")
	}
	body.Target = strings.ToLower(body.Target)
	if ofClient != macRE.MatchString(body.Target) {
		return badRequest("disconnect is of a client, its MAC the target; the others take none")
	}
	var params json.RawMessage
	if body.Kind == "client-test" {
		ask, err := s.clientTestFor(c, id, body.Network, body.On, body.Band)
		if err != nil {
			return err
		}
		if params, err = json.Marshal(ask); err != nil {
			return err
		}
	} else if body.Network != "" || body.On != "" || body.Band != "" {
		return badRequest("network, on and band are a client-test's")
	}
	a, err := s.conds.AddActionWith(id, body.Kind, body.Target, string(c.actor), params)
	if err != nil {
		return err
	}
	slog.Info("action asked", "ap", id, "kind", a.Kind, "target", a.Target, "by", c.actor, "id", a.ID)
	writeJSON(w, http.StatusOK, map[string]any{"action": a})
	return nil
}

// actionList is an AP's latest actions, newest first: GET
// /v1/aps/{ap}/actions.
func (s *Server) actionList(w http.ResponseWriter, r *http.Request, c call) error {
	id := hierarchy.NodeID(r.PathValue("ap"))
	t := c.state.Org.Locations
	if n, ok := t.Node(id); !ok || n.Kind != hierarchy.KindAP || roleOn(c, change.Locations, t, id) < access.Viewer {
		return errNotFound
	}
	list, err := s.conds.Actions(id, 20)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"actions": list})
	return nil
}

// apActions is what the AP has to do: GET /v1/ap/actions. Each is handed
// it once: it is running from here, so a lost report of it never has the
// AP do it again.
func (s *Server) apActions(w http.ResponseWriter, _ *http.Request, c apCall) error {
	list, err := s.conds.ClaimActions(c.ap)
	if err != nil {
		return err
	}
	out := []map[string]any{}
	for _, a := range list {
		x := map[string]any{"id": a.ID, "kind": a.Kind}
		if a.Target != "" {
			x["target"] = a.Target
		}
		if len(a.Params) > 0 {
			x["params"] = a.Params
		}
		out = append(out, x)
	}
	writeJSON(w, http.StatusOK, map[string]any{"actions": out})
	return nil
}

// apActionDone is what came of one: POST /v1/ap/actions/{id} {ok, result,
// detail?}, detail being what a client-test found (0121).
func (s *Server) apActionDone(w http.ResponseWriter, r *http.Request, c apCall) error {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return errNotFound
	}
	var body struct {
		OK     bool            `json:"ok"`
		Result string          `json:"result"`
		Detail json.RawMessage `json:"detail"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if len(body.Result) > 300 || !printable(body.Result) {
		return badRequest("result: at most 300 printable characters")
	}
	var detail json.RawMessage
	if len(body.Detail) > 0 && string(body.Detail) != "null" {
		open, err := s.conds.OpenAction(c.ap, id)
		if errors.Is(err, conditions.ErrNoAction) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if open.Kind != "client-test" {
			return badRequest("detail: only a client-test has any")
		}
		if detail, err = clientTestDetail(body.Detail); err != nil {
			return err
		}
	}
	a, err := s.conds.FinishActionWith(c.ap, id, body.OK, body.Result, detail)
	if errors.Is(err, conditions.ErrNoAction) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	slog.Info("action done", "ap", c.ap, "kind", a.Kind, "state", a.State, "result", a.Result)
	writeJSON(w, http.StatusOK, map[string]any{"action": a})
	return nil
}

// setActionsHeader says, on a config poll, how many actions wait.
func (s *Server) setActionsHeader(w http.ResponseWriter, ap hierarchy.NodeID) {
	if list, err := s.conds.PendingActions(ap); err == nil && len(list) > 0 {
		w.Header().Set(ActionsHeader, strconv.Itoa(len(list)))
	}
}
