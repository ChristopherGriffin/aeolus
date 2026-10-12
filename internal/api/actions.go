package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
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
var actionKinds = map[string]bool{"locate": false, "restart-wifi": false, "reboot": false, "disconnect": true, "client-test": false, "remove-network": false, "import-network": false}

// removeNetworkAsk is what an AP is asked to take out of its config (0137):
// one of its own Wi-Fi networks, not Aeolus's, by its SSID and its
// sections, one for each radio it is on.
type removeNetworkAsk struct {
	SSID     string   `json:"ssid"`
	Sections []string `json:"sections"`
}

// removeNetworkFor is the asking for an AP to take out one of its own
// networks: every section named must be one the AP last said it has, with
// that SSID. Aeolus's own networks are never among those.
func (s *Server) removeNetworkFor(ap hierarchy.NodeID, ssid string, sections []string) (removeNetworkAsk, error) {
	if len(sections) < 1 || len(sections) > 32 {
		return removeNetworkAsk{}, badRequest("remove-network: sections, the AP's own name for the network on each radio it is on, 1 to 32 of them, and its ssid")
	}
	st, err := s.conds.LatestState(ap)
	if err != nil {
		return removeNetworkAsk{}, err
	}
	var rep struct {
		Others []otherNetwork `json:"others"`
	}
	if st != nil {
		_ = json.Unmarshal(st.Report, &rep)
	}
	ask := removeNetworkAsk{SSID: ssid}
	for _, sec := range sections {
		if !slices.ContainsFunc(rep.Others, func(o otherNetwork) bool { return o.Section == sec && o.SSID == ssid }) {
			return removeNetworkAsk{}, &apiError{http.StatusConflict, "the AP has no network of its own with that SSID and section, as it last reported: look again"}
		}
		if !slices.Contains(ask.Sections, sec) {
			ask.Sections = append(ask.Sections, sec)
		}
	}
	// In one order however they were asked for: they are what tells this
	// asking from another.
	slices.Sort(ask.Sections)
	return ask, nil
}

// addAction asks an AP to do something once (0104): POST /v1/aps/{ap}/actions
// {kind, target?}, target being the client's MAC for disconnect (0107). A
// client-test (0121) says what to test instead: {kind, network, on?,
// band?, remote?}, one of the AP's networks, the AP whose networks to join,
// a band, and, false, that the speed to the remote server is not measured
// (0134), as on a link where bytes cost. A remove-network (0137) says which
// of the AP's own networks to take out: {kind, ssid, sections}. An
// import-network (0137) says which of them to make Aeolus's own, and
// where: {kind, ssid, sections, folder, network, bands?, vlan}, the
// Services folder it goes in and its ID there. It needs operator on the
// AP, as changing it does (0030); an import, the leave to change that
// folder too.
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
		Remote  *bool  `json:"remote"`
		// Of a remove-network and an import-network (0137).
		SSID     string   `json:"ssid"`
		Sections []string `json:"sections"`
		// Of an import-network: where the network goes, and as what.
		Folder string   `json:"folder"`
		Bands  []string `json:"bands"`
		VLAN   int      `json:"vlan"`
		// The security the asker was shown, which the import must be of.
		Security string `json:"security"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	ofClient, ok := actionKinds[body.Kind]
	if !ok {
		return badRequest("an action is locate, restart-wifi, reboot, disconnect, client-test, remove-network or import-network")
	}
	body.Target = strings.ToLower(body.Target)
	if ofClient != macRE.MatchString(body.Target) {
		return badRequest("disconnect is of a client, its MAC the target; the others take none")
	}
	var params json.RawMessage
	if body.Kind == "client-test" {
		ask, err := s.clientTestFor(c, id, body.Network, body.On, body.Band, body.Remote == nil || *body.Remote)
		if err != nil {
			return err
		}
		if params, err = json.Marshal(ask); err != nil {
			return err
		}
	} else if body.Kind != "import-network" && (body.Network != "" || body.On != "" || body.Band != "" || body.Remote != nil) {
		return badRequest("network, on, band and remote are a client-test's")
	}
	// The sections of an AP's own network, by which one asking for it is
	// told from another (0137).
	var sections []string
	switch body.Kind {
	case "remove-network":
		if body.Folder != "" || len(body.Bands) > 0 || body.VLAN != 0 || body.Security != "" {
			return badRequest("folder, bands, vlan and security are an import-network's")
		}
		ask, err := s.removeNetworkFor(id, body.SSID, body.Sections)
		if err != nil {
			return err
		}
		if params, err = json.Marshal(ask); err != nil {
			return err
		}
		sections = ask.Sections
	case "import-network":
		if body.On != "" || body.Band != "" || body.Remote != nil {
			return badRequest("on, band and remote are a client-test's")
		}
		ask, err := s.importFor(c, id, body.SSID, body.Sections, body.Folder, body.Network, body.Bands, body.VLAN, body.Security)
		if err != nil {
			return err
		}
		if params, err = json.Marshal(ask); err != nil {
			return err
		}
		sections = ask.Sections
	default:
		if body.SSID != "" || len(body.Sections) > 0 || body.Folder != "" || len(body.Bands) > 0 || body.VLAN != 0 || body.Security != "" {
			return badRequest("ssid and sections are a remove-network's or an import-network's; folder, bands, vlan and security an import-network's")
		}
	}
	// One open action of a kind for a target stands for every asking of
	// it (0104). The taking out of a network is told from that of another
	// by its sections: without them, a second network asked of an AP while
	// the first waited was dropped, and its asker told it was asked.
	target := body.Target
	if sections != nil {
		target = strings.Join(sections, ",")
	}
	a, err := s.conds.AddActionWith(id, body.Kind, target, string(c.actor), params)
	if err != nil {
		return err
	}
	// An import of a network that waits stands for no other: one asked
	// again into another folder, under another ID, or on another VLAN is
	// told that one waits, and not that it was asked.
	if body.Kind == "import-network" && string(a.Params) != string(params) {
		return &apiError{http.StatusConflict, "an import of this network is asked of this AP already, and waits for it: let it be made, or wait for it to expire, before asking another way"}
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
	// A client test (0121) joined on an AP the caller may not view does not
	// say which, nor its BSSIDs.
	for i, a := range list {
		if a.Kind != "client-test" {
			continue
		}
		// What it found, with its way online a point at a time (0130).
		list[i] = withWay(a)
		var ask clientTestAsk
		if json.Unmarshal(a.Params, &ask) != nil || ask.On == "" || roleOn(c, change.Locations, t, hierarchy.NodeID(ask.On)) >= access.Viewer {
			continue
		}
		ask.On, ask.BSSIDs = "", nil
		list[i].Params, _ = json.Marshal(ask)
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
		// A test of a network that signs clients in takes its user's
		// password with it (0124): opened here, for the AP alone.
		if a.Kind == "client-test" {
			if p, ok := s.withTestPassword(c.ap, a.Params); ok {
				x["params"] = p
			}
		}
		out = append(out, x)
	}
	writeJSON(w, http.StatusOK, map[string]any{"actions": out})
	return nil
}

// withTestPassword puts the password of a test's user in what the AP is
// handed (0124), where the test has a user: read from the network as the
// AP's folders have it now, and opened. A password that is gone, or does
// not open, leaves the test without one, and the AP says it cannot sign in.
func (s *Server) withTestPassword(ap hierarchy.NodeID, params json.RawMessage) (json.RawMessage, bool) {
	var ask clientTestAsk
	if json.Unmarshal(params, &ask) != nil || ask.EAP == nil {
		return nil, false
	}
	state := s.log.Snapshot()
	if state == nil {
		return nil, false
	}
	user, sealed := testUser(state, ap, ask.Network)
	if user == nil {
		return nil, false
	}
	v, err := s.reveal("network."+ask.Network+"."+change.TestPassword, sealed)
	pass, _ := v.(string)
	if err != nil || pass == "" {
		slog.Warn("client test: its user's password does not open", "ap", ap, "network", ask.Network, "err", err)
		return nil, false
	}
	ask.EAP.Password = pass
	out, err := json.Marshal(ask)
	return out, err == nil
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
		switch open.Kind {
		case "client-test":
			if detail, err = clientTestDetail(body.Detail); err != nil {
				return err
			}
		case "import-network":
			// What the AP said of its own network (0137): the network is
			// made of it now, and what came of that is the action's
			// result. What it said, its key in it, is not kept.
			if body.OK {
				body.OK, body.Result = s.importNetwork(c.ap, open, body.Detail)
			}
		default:
			return badRequest("detail: only a client-test or an import-network has any")
		}
	} else if body.OK {
		// An import whose AP said nothing of the network made nothing.
		if open, err := s.conds.OpenAction(c.ap, id); err == nil && open.Kind == "import-network" {
			body.OK, body.Result = false, "the AP said nothing of the network: nothing was made"
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
