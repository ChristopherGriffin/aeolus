package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// ActionsHeader tells an AP, on every config poll, how many actions wait
// for it (0104), so it asks for them only when there are some.
const ActionsHeader = "Aeolus-Actions"

// actionKinds are what a person may ask an AP to do once (0104).
var actionKinds = map[string]bool{"locate": true, "restart-wifi": true, "reboot": true}

// addAction asks an AP to do something once (0104): POST /v1/aps/{ap}/actions
// {kind}. It needs operator on the AP, as changing it does (0030).
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
		Kind string `json:"kind"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if !actionKinds[body.Kind] {
		return badRequest("an action is locate, restart-wifi or reboot")
	}
	a, err := s.conds.AddAction(id, body.Kind, string(c.actor))
	if err != nil {
		return err
	}
	slog.Info("action asked", "ap", id, "kind", a.Kind, "by", c.actor, "id", a.ID)
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

// apActions is what the AP has to do: GET /v1/ap/actions.
func (s *Server) apActions(w http.ResponseWriter, _ *http.Request, c apCall) error {
	list, err := s.conds.PendingActions(c.ap)
	if err != nil {
		return err
	}
	out := []map[string]any{}
	for _, a := range list {
		out = append(out, map[string]any{"id": a.ID, "kind": a.Kind})
	}
	writeJSON(w, http.StatusOK, map[string]any{"actions": out})
	return nil
}

// apActionDone is what came of one: POST /v1/ap/actions/{id} {ok, result}.
func (s *Server) apActionDone(w http.ResponseWriter, r *http.Request, c apCall) error {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return errNotFound
	}
	var body struct {
		OK     bool   `json:"ok"`
		Result string `json:"result"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if len(body.Result) > 300 || !printable(body.Result) {
		return badRequest("result: at most 300 printable characters")
	}
	a, err := s.conds.FinishAction(c.ap, id, body.OK, body.Result)
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
