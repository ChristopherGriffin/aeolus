package api

import (
	"encoding/json"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/oui"
)

// Clients and their connections (0118): each AP reports every attempt a
// client makes to come online, step by step, and the manager keeps them,
// and every client it has seen.

const (
	maxConnections = 512 << 10 // a report of them: 64, each with its steps
	connStepsMax   = 64
)

// The stages of coming online, in order, and what an attempt came to.
var (
	connStages   = []string{"auth", "assoc", "signin", "key", "dhcp", "gateway", "dns", "internet"}
	connOutcomes = map[string]bool{"online": true, "connected": true, "failed": true, "left": true}
)

// connStep is one step of an attempt: how long after its start, in ms; its
// stage, or end where the client left, or note for a line of hostapd's
// that is kept as it reads; what happened; whether it went well; and, where
// they apply, the server that answered, the gateway and DNS servers DHCP
// gave, who holds an address, and how long an answer took.
type connStep struct {
	T      int    `json:"t"`
	Stage  string `json:"stage"`
	What   string `json:"what"`
	OK     *bool  `json:"ok,omitempty"`
	Server string `json:"server,omitempty"`
	Router string `json:"router,omitempty"`
	DNS    string `json:"dns,omitempty"`
	Holder string `json:"holder,omitempty"`
	MS     *int   `json:"ms,omitempty"`
}

// connProbe is where a client was heard asking for networks before it
// joined: on this AP, where AP is empty, or on another by its address; the
// band, where it is this AP's; and how strongly.
type connProbe struct {
	AP     string `json:"ap"`
	Band   string `json:"band"`
	Signal int    `json:"signal"`
}

// connRecord is an attempt as an AP reports it (0118).
type connRecord struct {
	MAC     string      `json:"mac"`
	BSS     string      `json:"bss"`
	Network string      `json:"network"`
	SSID    string      `json:"ssid"`
	Band    string      `json:"band"`
	Started int64       `json:"started"` // ms since 1970, by the AP's clock
	TookMS  int64       `json:"took_ms"`
	Outcome string      `json:"outcome"`
	Stage   string      `json:"stage"`
	Reason  string      `json:"reason"`
	Events  []connStep  `json:"events"`
	More    int         `json:"more,omitempty"`
	Address string      `json:"address,omitempty"`
	Host    string      `json:"host,omitempty"`
	Signal  *int        `json:"signal,omitempty"`
	Probes  []connProbe `json:"probes,omitempty"`
}

func shortText(s string, n int) bool { return len(s) <= n && printable(s) }

// check holds a record to what aeolus-journey writes.
func (c connRecord) check() error {
	bad := func(what string) error { return badRequest("connections: %s", what) }
	if !macRE.MatchString(c.MAC) {
		return bad("each names its client by MAC, in lower case")
	}
	if c.BSS == "" || !shortText(c.BSS, 32) || (c.Network != "" && !networkIDRE.MatchString(c.Network)) || len(c.SSID) > 32 || !printable(c.SSID) {
		return bad("each has the interface it was on, a network ID or none, and an SSID of at most 32 printable characters")
	}
	if c.Band != "" && c.Band != "2g" && c.Band != "5g" && c.Band != "6g" {
		return bad("a band is 2g, 5g or 6g")
	}
	if c.Started <= 0 || c.TookMS < 0 || c.TookMS > 600000 || c.More < 0 || c.More > 100000 {
		return bad("each has when it started and how long it took, at most 10 minutes")
	}
	if !connOutcomes[c.Outcome] || !slices.Contains(connStages, c.Stage) || !shortText(c.Reason, 200) {
		return bad("each has an outcome (online, connected, failed or left), the stage it reached or stopped at, and a reason of at most 200 printable characters")
	}
	if len(c.Events) > connStepsMax || len(c.Probes) > 8 {
		return bad("at most 64 steps and 8 places it was heard")
	}
	for _, e := range c.Events {
		if e.T < 0 || e.T > 600000 || (e.Stage != "end" && e.Stage != "note" && !slices.Contains(connStages, e.Stage)) || e.What == "" || !shortText(e.What, 200) ||
			!shortText(e.Server, 45) || !shortText(e.Router, 45) || !shortText(e.DNS, 140) || (e.Holder != "" && !macRE.MatchString(e.Holder)) ||
			(e.MS != nil && (*e.MS < 0 || *e.MS > 600000)) {
			return bad("each step has its time, its stage, and what happened in at most 200 printable characters")
		}
	}
	for _, p := range c.Probes {
		if (p.AP != "" && net.ParseIP(p.AP) == nil) || (p.Band != "" && p.Band != "2g" && p.Band != "5g" && p.Band != "6g") || p.Signal < -127 || p.Signal > 0 {
			return bad("where it was heard is this AP or another's address, a band or none, and a signal from -127 to 0 dBm")
		}
	}
	if (c.Address != "" && net.ParseIP(c.Address) == nil) || !shortText(c.Host, 64) || (c.Signal != nil && (*c.Signal < -127 || *c.Signal > 0)) {
		return bad("an address is an IP address, a host name at most 64 printable characters, a signal from -127 to 0 dBm")
	}
	return nil
}

// connections takes the attempts an AP reports: POST /v1/ap/connections,
// {connections: [...]}, at most 64. Each is checked by itself, and one that
// is not well formed is passed over, so it does not hold the others back;
// the answer says how many were new and how many were refused, with the
// first's reason. An attempt reported before is not kept twice. One whose
// start is not near now, as from an AP whose clock is unset, is given the
// time it came in.
func (s *Server) connections(w http.ResponseWriter, r *http.Request, c apCall) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxConnections)
	var req struct {
		Connections []connRecord `json:"connections"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if len(req.Connections) > 64 {
		return badRequest("connections: at most 64 at a time")
	}
	if err := unadopted(s.log.Snapshot(), c.ap); err != nil {
		return err
	}
	now := time.Now()
	var list []conditions.Connection
	refused, why := 0, ""
	for i, rec := range req.Connections {
		if err := rec.check(); err != nil {
			if refused++; why == "" {
				why = err.Error()
			}
			continue
		}
		started := time.UnixMilli(rec.Started)
		if started.After(now.Add(10*time.Minute)) || started.Before(now.Add(-7*24*time.Hour)) {
			// Apart by a millisecond each, so two of one report stay two.
			started = now.Add(time.Duration(i) * time.Millisecond)
			rec.Started = started.UnixMilli()
		}
		raw, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		list = append(list, conditions.Connection{
			MAC: rec.MAC, Started: started, Network: rec.Network, SSID: rec.SSID, Band: rec.Band,
			Outcome: rec.Outcome, Stage: rec.Stage, Reason: rec.Reason, TookMS: rec.TookMS, Record: raw,
			Host: rec.Host, Address: rec.Address,
		})
	}
	added, err := s.conds.RecordConnections(c.ap, list)
	if err != nil {
		return err
	}
	out := map[string]any{"recorded": added, "refused": refused}
	if why != "" {
		out["why"] = why
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// mayView is the APs the caller may view, below one Locations node where
// under names it.
func mayView(c call, under hierarchy.NodeID) []hierarchy.NodeID {
	t := c.state.Org.Locations
	var ids []hierarchy.NodeID
	for _, id := range t.APs() {
		if roleOn(c, change.Locations, t, id) < access.Viewer {
			continue
		}
		if under != "" && under != id && !slices.Contains(t.Ancestry(id), under) {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// clientList is every client ever seen whose last AP the caller may view
// (0118), the latest first: GET /v1/clients. ?under= keeps it to a
// Locations node's APs; ?q= to those whose MAC, host name, user or address
// has that in it; ?limit= (100 unless set, at most 500) and ?offset= page
// through. Each says where it was last seen, what it says of itself, how
// many times it tried to come online and how many failed.
func (s *Server) clientList(w http.ResponseWriter, r *http.Request, c call) error {
	q := r.URL.Query()
	limit, offset := 100, 0
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			return badRequest("limit: from 1 to 500")
		}
		limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return badRequest("offset: 0 or more")
		}
		offset = n
	}
	find := strings.ToLower(strings.TrimSpace(q.Get("q")))
	if len(find) > 64 {
		return badRequest("q: at most 64 characters")
	}
	t := c.state.Org.Locations
	list, total, err := s.conds.Clients(mayView(c, hierarchy.NodeID(q.Get("under"))), find, limit, offset)
	if err != nil {
		return err
	}
	type entry struct {
		conditions.Client
		APName  string `json:"ap_name"`
		Maker   string `json:"maker,omitempty"`
		Private bool   `json:"private,omitempty"`
	}
	out := []entry{}
	for _, cl := range list {
		e := entry{Client: cl}
		if n, ok := t.Node(cl.AP); ok {
			e.APName = n.Name
		}
		e.Maker, e.Private = oui.Lookup(cl.MAC)
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"clients": out, "total": total})
	return nil
}

// clientConnections is one client's attempts to come online, on the APs the
// caller may view (0118), the latest first: GET
// /v1/clients/{mac}/connections. ?limit= is 50 unless set, at most 200;
// ?before= an attempt's start, as it is given here, pages back. Each has
// its outcome, the stage it reached or stopped at and why, and its record:
// every step, with its time.
func (s *Server) clientConnections(w http.ResponseWriter, r *http.Request, c call) error {
	mac := strings.ToLower(r.PathValue("mac"))
	if !macRE.MatchString(mac) {
		return badRequest("a client is named by its MAC, such as aa:bb:cc:00:11:22")
	}
	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			return badRequest("limit: from 1 to 200")
		}
		limit = n
	}
	var before time.Time
	if v := q.Get("before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return badRequest("before: a time, as an attempt's started")
		}
		before = t
	}
	t := c.state.Org.Locations
	aps := mayView(c, "")
	list, err := s.conds.Connections(mac, aps, limit, before)
	if err != nil {
		return err
	}
	type entry struct {
		conditions.Connection
		APName string `json:"ap_name"`
	}
	out := []entry{}
	for _, cn := range list {
		e := entry{Connection: cn}
		if n, ok := t.Node(cn.AP); ok {
			e.APName = n.Name
		}
		out = append(out, e)
	}
	res := map[string]any{"mac": mac, "connections": out}
	// The client itself, where its last AP is one the caller may view.
	if cl, ok, err := s.conds.ClientOf(mac); err != nil {
		return err
	} else if ok && slices.Contains(aps, cl.AP) {
		maker, private := oui.Lookup(mac)
		name := ""
		if n, ok := t.Node(cl.AP); ok {
			name = n.Name
		}
		res["client"] = map[string]any{
			"mac": cl.MAC, "first_seen": cl.FirstSeen, "last_seen": cl.LastSeen, "ap": cl.AP, "ap_name": name,
			"network": cl.Network, "ssid": cl.SSID, "host": cl.Host, "user": cl.User, "address": cl.Address,
			"attempts": cl.Attempts, "failed": cl.Failed, "last_outcome": cl.LastOutcome, "maker": maker, "private": private,
		}
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}
