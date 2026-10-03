package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/compose"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/rendercheck"
	"github.com/ChristopherGriffin/aeolus/internal/uci"
)

// The render check and the reports (0039).

const (
	maxUCI    = 256 << 10
	maxReport = 64 << 10
	maxError  = 2048
)

var (
	hashRE      = regexp.MustCompile(`^[0-9a-f]{64}$`)
	networkIDRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	portNameRE  = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`) // as the schema names ports
	speedRE     = regexp.MustCompile(`^([0-9]{1,6}[FH])?$`)
)

// secretOptions are the UCI options that hold keys and passwords (0041).
var secretOptions = map[string]bool{
	"key": true, "sae_password": true, "password": true,
	"auth_secret": true, "private_key": true, "preshared_key": true,
	"community": true, "auth_pass": true, "privacy_pass": true,
}

// secretsIn lists the secret values in a composed config (0027), so a kept
// copy of rendered UCI can blank them wherever they appear (0041).
func (s *Server) secretsIn(doc map[string]any) []string {
	var out []string
	var walk func(v any, path string)
	walk = func(v any, path string) {
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				if path == "" {
					walk(e, k)
				} else {
					walk(e, path+"."+k)
				}
			}
		case string:
			if f, err := s.schema.Field(hierarchy.Path(path)); err == nil && f.Secret {
				out = append(out, x)
			}
		}
	}
	walk(doc, "")
	return out
}

// unadopted refuses a check or report from an AP in Landing Zone: it has no
// config, and nobody has vouched for it yet (0039). Nothing is recorded.
func unadopted(state *change.State, ap hierarchy.NodeID) error {
	if state.Org.Locations.InIsolated(ap) {
		return &apiError{http.StatusConflict, "this AP is in Landing Zone: it has nothing to check or report until a person adopts it"}
	}
	return nil
}

// render checks the UCI an AP rendered before it applies it (0008, 0039).
// The UCI itself is not kept, since it carries passphrases in plain text;
// its hash is.
func (s *Server) render(w http.ResponseWriter, r *http.Request, c apCall) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxUCI+4096)
	var req struct {
		Version int64  `json:"version"`
		UCI     string `json:"uci"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(req.UCI))
	hash := hex.EncodeToString(sum[:])

	// The version is read on both sides of the snapshot: if a change landed
	// in between, the state may not be the one the AP rendered.
	current, _ := s.log.Version(c.ap)
	state := s.log.Snapshot()
	after, _ := s.log.Version(c.ap)
	if err := unadopted(state, c.ap); err != nil {
		return err
	}
	check := conditions.Check{Version: req.Version, Hash: hash}
	switch {
	case req.Version != after || after != current:
		check.Result = conditions.Stale
		check.Problems = []string{fmt.Sprintf("version %d is not current (%d): poll again", req.Version, after)}
	default:
		res, err := compose.AP(state, s.schema, c.ap, s.reveal)
		if err != nil {
			return err
		}
		// Kept for the record, secrets blanked (0041). A stale render is
		// not kept: it will never run.
		check.UCI = uci.Redact(req.UCI, secretOptions, s.secretsIn(res.Doc))
		switch parsed, err := uci.Parse(req.UCI); {
		case len(res.Problems) > 0:
			check.Result, check.Problems = conditions.Refused, append([]string{"the config itself is held (0029)"}, res.Problems...)
		case err != nil:
			check.Result, check.Problems = conditions.Refused, []string{"the UCI does not parse: " + err.Error()}
		default:
			check.Problems = rendercheck.Check(res.Doc, parsed)
			check.Result = conditions.OK
			if len(check.Problems) > 0 {
				check.Result = conditions.Refused
			}
		}
	}
	if check.Problems == nil {
		check.Problems = []string{}
	}
	if err := s.conds.RecordCheck(c.ap, check); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": check.Result, "problems": check.Problems, "version": after, "hash": hash})
	return nil
}

// applied records an apply attempt, and whether an ok check covered it.
func (s *Server) applied(w http.ResponseWriter, r *http.Request, c apCall) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxReport)
	var req struct {
		Version int64  `json:"version"`
		Hash    string `json:"hash"`
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if !hashRE.MatchString(req.Hash) {
		return badRequest("hash must be the SHA-256 of the UCI, as 64 lowercase hex digits")
	}
	if len(req.Error) > maxError {
		return badRequest("error is longer than %d characters", maxError)
	}
	if err := unadopted(s.log.Snapshot(), c.ap); err != nil {
		return err
	}
	a, err := s.conds.RecordApply(c.ap, conditions.Apply{Version: req.Version, Hash: req.Hash, OK: req.OK, Error: req.Error})
	if err != nil {
		return err
	}
	if !a.Checked {
		slog.Warn("AP applied UCI that no ok check covers", "ap", c.ap, "version", req.Version, "hash", req.Hash)
	}
	if req.OK {
		if err := s.conds.Running(c.ap, req.Version); err != nil {
			return err
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"recorded": true, "checked": a.Checked})
	return nil
}

// stateReport is what an AP reports about itself periodically (0039).
type stateReport struct {
	Version    int64                     `json:"version"`
	Uptime     int64                     `json:"uptime"`
	OpenWrt    string                    `json:"openwrt,omitempty"`
	Radios     []radioState              `json:"radios,omitempty"`
	VLANs      []int                     `json:"vlans,omitempty"`
	Transports map[string]transportState `json:"transports,omitempty"`
	Steering   *steeringState            `json:"steering,omitempty"`
	Ports      []portState               `json:"ports,omitempty"`
}

// portState is one Ethernet port in the bridge the AP's uplink is in
// (0053): whether it is up, whether it has a link and at what speed, in
// Mbit/s and duplex ("1000F"), and whether it is the uplink.
type portState struct {
	Name    string `json:"name"`
	Up      bool   `json:"up"`
	Carrier bool   `json:"carrier"`
	Speed   string `json:"speed,omitempty"`
	Uplink  bool   `json:"uplink,omitempty"`
}

// steeringState is what usteer is doing on the AP (0050, 0051), so a person
// can see whether band steering works.
type steeringState struct {
	Installed bool       `json:"installed"`
	Running   bool       `json:"running"`
	Interval  int        `json:"interval"`        // band_steering_interval, ms; 0 is off
	SSIDs     []string   `json:"ssids,omitempty"` // the SSIDs it steers
	BSS       []bssState `json:"bss,omitempty"`
}

// bssState is one SSID on one band: its clients now, and the clients usteer
// has moved off it and onto it since usteer started.
type bssState struct {
	SSID        string `json:"ssid"`
	Band        string `json:"band"`
	Clients     int    `json:"clients"`
	SteeredAway int    `json:"steered_away"`
	SteeredIn   int    `json:"steered_in"`
}

type radioState struct {
	Radio   string `json:"radio"`
	Band    string `json:"band"`
	Channel int    `json:"channel,omitempty"`
	Width   int    `json:"width,omitempty"`
	Clients int    `json:"clients"`
}

// transportState is one network's transports: which is carrying traffic, and
// how each is doing (0020, 0022).
type transportState struct {
	Active   string `json:"active"`
	Primary  string `json:"primary,omitempty"`
	Fallback string `json:"fallback,omitempty"`
}

var (
	bands   = map[string]bool{"2g": true, "5g": true, "6g": true}
	actives = map[string]bool{"primary": true, "fallback": true, "none": true}
	healths = map[string]bool{"": true, "up": true, "down": true, "unknown": true}
)

func (st *stateReport) check() error {
	if st.Version < 0 || st.Uptime < 0 {
		return badRequest("version and uptime cannot be negative")
	}
	if err := plainText("openwrt", st.OpenWrt, maxText); err != nil {
		return err
	}
	if len(st.Radios) > 8 {
		return badRequest("at most 8 radios")
	}
	for _, rd := range st.Radios {
		if !bands[rd.Band] {
			return badRequest("radio band %q: want 2g, 5g or 6g", rd.Band)
		}
		if err := plainText("radio", rd.Radio, 32); err != nil {
			return err
		}
		if rd.Channel < 0 || rd.Width < 0 || rd.Clients < 0 {
			return badRequest("radio numbers cannot be negative")
		}
	}
	seen := map[int]bool{}
	for _, v := range st.VLANs {
		if v < 1 || v > 4094 || seen[v] {
			return badRequest("vlans must be distinct IDs from 1 to 4094")
		}
		seen[v] = true
	}
	if len(st.Transports) > 64 {
		return badRequest("at most 64 networks")
	}
	if g := st.Steering; g != nil {
		if g.Interval < 0 || len(g.SSIDs) > 64 || len(g.BSS) > 64 {
			return badRequest("steering: interval cannot be negative; at most 64 SSIDs and 64 BSSes")
		}
		for _, ssid := range g.SSIDs {
			if len(ssid) > 32 {
				return badRequest("steering: an SSID is at most 32 bytes")
			}
		}
		for _, b := range g.BSS {
			if !bands[b.Band] || len(b.SSID) > 32 || b.Clients < 0 || b.SteeredAway < 0 || b.SteeredIn < 0 {
				return badRequest("steering: each BSS has a band (2g, 5g or 6g), an SSID of at most 32 bytes, and counts that are not negative")
			}
		}
	}
	if len(st.Ports) > 32 {
		return badRequest("at most 32 ports")
	}
	names := map[string]bool{}
	for _, p := range st.Ports {
		if !portNameRE.MatchString(p.Name) || names[p.Name] || !speedRE.MatchString(p.Speed) {
			return badRequest("ports: each has its own name (such as lan1) and a speed such as 1000F, or none")
		}
		names[p.Name] = true
	}
	for id, t := range st.Transports {
		if !networkIDRE.MatchString(id) {
			return badRequest("%q is not a network ID", id)
		}
		if !actives[t.Active] || !healths[t.Primary] || !healths[t.Fallback] {
			return badRequest("network %s: active is primary, fallback or none; health is up, down or unknown", id)
		}
	}
	return nil
}

// state records a state report.
func (s *Server) state(w http.ResponseWriter, r *http.Request, c apCall) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxReport)
	var req stateReport
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if err := req.check(); err != nil {
		return err
	}
	if err := unadopted(s.log.Snapshot(), c.ap); err != nil {
		return err
	}
	report, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := s.conds.RecordState(c.ap, req.Version, report); err != nil {
		return err
	}
	if err := s.conds.Running(c.ap, req.Version); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"recorded": true})
	return nil
}

// apHistory lists an AP's recent checks, applies and state reports, newest
// first.
func (s *Server) apHistory(w http.ResponseWriter, r *http.Request, c call) error {
	id := hierarchy.NodeID(r.PathValue("ap"))
	t := c.state.Org.Locations
	if n, ok := t.Node(id); !ok || n.Kind != hierarchy.KindAP || roleOn(c, change.Locations, t, id) < access.Viewer {
		return errNotFound
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			return badRequest("limit must be 1 to 200")
		}
		limit = n
	}
	h, err := s.conds.History(id, limit)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"ap": id, "checks": h.Checks, "applies": h.Applies, "states": h.States})
	return nil
}

// aps lists every AP the caller can view, with what the fleet view needs
// (0042): where it is, its version, whether its config is ready, held or
// unassigned, and when it was last seen and what it runs.
func (s *Server) aps(w http.ResponseWriter, _ *http.Request, c call) error {
	t := c.state.Org.Locations
	out := []map[string]any{}
	for _, id := range t.APs() {
		if roleOn(c, change.Locations, t, id) < access.Viewer {
			continue
		}
		n, _ := t.Node(id)
		version, _ := s.log.Version(id)
		res, err := compose.AP(c.state, s.schema, id, s.reveal)
		if err != nil {
			return err
		}
		config := "ready"
		switch {
		case res.Unassigned:
			config = "unassigned"
		case len(res.Problems) > 0:
			config = "held"
		}
		l, err := s.conds.Latest(id)
		if err != nil {
			return err
		}
		var inSync any
		if l.Seen != nil && l.Seen.Running != nil {
			inSync = *l.Seen.Running == version
		}
		out = append(out, map[string]any{
			"id": id, "name": n.Name, "ancestry": t.Ancestry(id), "version": version,
			"config": config, "problems": len(res.Problems), "seen": l.Seen, "in_sync": inSync,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"aps": out})
	return nil
}

// condition is what the manager knows of an AP's own account of itself, for
// the AP's config view: last seen, what it runs, and whether that is its
// current version.
func (s *Server) condition(ap hierarchy.NodeID, version int64) (map[string]any, error) {
	l, err := s.conds.Latest(ap)
	if err != nil {
		return nil, err
	}
	if l.Check != nil {
		l.Check.UCI = "" // in the history; too long for a summary
	}
	out := map[string]any{"seen": l.Seen, "check": l.Check, "apply": l.Apply, "state": l.State, "in_sync": nil}
	if l.Seen != nil && l.Seen.Running != nil {
		out["in_sync"] = *l.Seen.Running == version
	}
	return out, nil
}
