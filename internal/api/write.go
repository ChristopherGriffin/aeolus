package api

import (
	"encoding/json"
	"net/http"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

type changeRequest struct {
	Op     change.Op `json:"op"`
	Reason string    `json:"reason"`
}

// prepare readies a submitted change: kinds with their own route are sent
// there, and set values are validated and, for secrets, sealed before they go
// anywhere near the log (0027).
func (s *Server) prepare(op change.Op) (change.Op, error) {
	switch op.Kind {
	case change.CreateOrg:
		return op, badRequest("the Org is created on the manager host with aeolus init")
	case change.IssueToken:
		return op, badRequest("tokens are issued with POST /v1/tokens")
	case change.Set:
		if len(op.Values) == 0 {
			v, err := s.prepareValue(op.Tree, op.Path, op.Value)
			op.Value = v
			return op, err
		}
		values := make(map[hierarchy.Path]json.RawMessage, len(op.Values))
		for p, raw := range op.Values {
			v, err := s.prepareValue(op.Tree, p, raw)
			if err != nil {
				return op, err
			}
			values[p] = v
		}
		op.Values = values
	}
	return op, nil
}

func (s *Server) prepareValue(tree change.TreeName, p hierarchy.Path, raw json.RawMessage) (json.RawMessage, error) {
	if tree == change.Locations && p == hierarchy.ServicesPath {
		return raw, nil // refused by Apply: use assign-services
	}
	return s.schema.Prepare(p, raw, s.box)
}

// checks runs the whole-config check for the given APs and returns the ones
// that fail, with their problems (0029).
func (s *Server) checks(state *change.State, aps []hierarchy.NodeID) map[hierarchy.NodeID][]string {
	out := map[hierarchy.NodeID][]string{}
	for _, ap := range aps {
		if p := s.apProblems(state, ap); len(p) > 0 {
			out[ap] = p
		}
	}
	return out
}

// commit makes one change (0026).
func (s *Server) commit(w http.ResponseWriter, r *http.Request, c call) error {
	var req changeRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	op, err := s.prepare(req.Op)
	if err != nil {
		return err
	}
	e, err := s.log.Commit(string(c.actor), req.Reason, op)
	if err != nil {
		return err
	}
	after := s.log.Snapshot()
	reversioned := s.reversioned(after, e.Seq)
	writeJSON(w, http.StatusOK, map[string]any{
		"change":      viewEntry(e),
		"reversioned": reversioned,
		"checks":      s.checks(after, reversioned),
	})
	return nil
}

// preview runs a change with every check and records nothing (0026).
func (s *Server) preview(w http.ResponseWriter, r *http.Request, c call) error {
	var req changeRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	op, err := s.prepare(req.Op)
	if err != nil {
		return err
	}
	state, eff, changed, err := s.log.Preview(string(c.actor), op)
	if err != nil {
		return err
	}
	if changed == nil {
		changed = []hierarchy.NodeID{}
	}
	eff.Before, eff.After = mask(eff.Before), mask(eff.After)
	eff.Removed = viewOverrides(eff.Removed)
	writeJSON(w, http.StatusOK, map[string]any{
		"effect":      eff,
		"reversioned": changed,
		"checks":      s.checks(state, changed),
	})
	return nil
}

// issueToken issues a token for an account and returns it once. Only its ID
// and hash are logged (0024, 0030).
func (s *Server) issueToken(w http.ResponseWriter, r *http.Request, c call) error {
	var req struct {
		Account access.AccountID `json:"account"`
		Reason  string           `json:"reason"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if req.Account == "" {
		req.Account = c.actor
	}
	plain, id, hash, err := access.NewToken()
	if err != nil {
		return err
	}
	op := change.Op{Kind: change.IssueToken, Account: req.Account, TokenID: id, TokenHash: hash}
	if _, err := s.log.Commit(string(c.actor), req.Reason, op); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": req.Account, "id": id, "token": plain})
	return nil
}

func (s *Server) revokeToken(w http.ResponseWriter, r *http.Request, c call) error {
	op := change.Op{Kind: change.RevokeToken, TokenID: r.PathValue("id")}
	e, err := s.log.Commit(string(c.actor), r.URL.Query().Get("reason"), op)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"change": viewEntry(e)})
	return nil
}
