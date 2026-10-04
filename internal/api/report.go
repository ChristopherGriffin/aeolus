package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strconv"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
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
	// deviceRE is a Linux device's name, as the loop guard reports one (0059).
	deviceRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@-]{0,14}$`)
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
		res, err := s.compose(state, c.ap, s.reveal)
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
			check.Problems = rendercheck.CheckAP(res.Doc, parsed, string(c.ap))
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
	Transports map[string]transportState `json:"transports,omitempty"`
	Steering   *steeringState            `json:"steering,omitempty"`
	Ports      []portState               `json:"ports,omitempty"`
	VXLAN      *vxlanState               `json:"vxlan,omitempty"`
	VLANProbes []vlanProbe               `json:"vlan_probes,omitempty"`
	// The VLANs the AP watches on its uplink, and the switch it is on (0064).
	UplinkVLANs    []uplinkVLAN    `json:"uplink_vlans,omitempty"`
	UplinkNeighbor *uplinkNeighbor `json:"uplink_neighbor,omitempty"`
}

// uplinkVLAN is a VLAN the AP carries on its uplink for its intent, and
// whether it reaches the AP from the switch (0064): present, when a frame
// came in on it within three minutes; silent, when none has for that long,
// though nudged; unknown, before it has been watched that long.
type uplinkVLAN struct {
	VLAN     int    `json:"vlan"`
	Tagged   bool   `json:"tagged"`
	Verdict  string `json:"verdict"`
	HeardAgo *int64 `json:"heard_ago"`
}

// uplinkNeighbor is what the switch's LLDP says of itself and of the port
// the AP's uplink is on (0064): its chassis ID and name, the port's ID and
// description, the port's native VLAN, the VLANs it names, and seconds
// since it last said so.
type uplinkNeighbor struct {
	Chassis         string `json:"chassis,omitempty"`
	System          string `json:"system,omitempty"`
	Port            string `json:"port,omitempty"`
	PortDescription string `json:"port_description,omitempty"`
	NativeVLAN      int    `json:"native_vlan,omitempty"`
	VLANs           []int  `json:"vlans,omitempty"`
	Ago             int64  `json:"ago"`
}

// vlanProbe is what the prober found on a VLAN transport of a network with a
// fallback (0061), probed on the uplink, tagged or not as the uplink carries
// the VLAN.
type vlanProbe struct {
	VLAN   int         `json:"vlan"`
	Tagged bool        `json:"tagged"`
	Probe  *probeState `json:"probe"`
}

// vxlanState is what the AP's VXLAN tunnels are doing (0054): whether the
// packages they need are installed (vxlan, and kmod-nft-bridge for the
// clamp), each tunnel Aeolus made, and whether the prober can run and which
// tunnel ports its loop guard took off their tunnels (0059).
type vxlanState struct {
	Installed bool          `json:"installed"`
	Loaded    *bool         `json:"loaded,omitempty"` // whether netifd has loaded vxlan (0057)
	Clamp     bool          `json:"clamp"`
	Prober    *bool         `json:"prober,omitempty"`     // whether ucode-mod-socket, which the prober needs, is installed (0059)
	UplinkMTU int           `json:"uplink_mtu,omitempty"` // what the AP's uplink carries now (0056)
	Tunnels   []tunnelState `json:"tunnels,omitempty"`
	Loops     []loopState   `json:"loops,omitempty"`
}

// tunnelState is one tunnel: its VNI, the concentrator's address and port,
// its MTU, the VLAN it starts from and the AP's address there (0063),
// whether it is up, or standing by as a fallback, and what the prober found.
type tunnelState struct {
	VNI         int         `json:"vni"`
	Peer        string      `json:"peer"`
	Port        int         `json:"port"`
	MTU         int         `json:"mtu"`
	FromVLAN    int         `json:"from_vlan,omitempty"`
	FromAddress string      `json:"from_address,omitempty"`
	Up          bool        `json:"up"`
	Standby     bool        `json:"standby,omitempty"`
	Probe       *probeState `json:"probe,omitempty"`
}

// probeState is what the prober found for a tunnel (0059): its verdict, how
// often it probes and what it asks, whether the concentrator answers a ping
// over the underlay and how fast, and what on the segment last answered,
// how fast, and how many seconds ago. What is not known yet is null.
type probeState struct {
	Verdict     string   `json:"verdict"`
	Interval    int      `json:"interval"`
	Asks        []string `json:"asks,omitempty"`
	Underlay    *bool    `json:"underlay"`
	UnderlayMS  *float64 `json:"underlay_ms"`
	From        string   `json:"from,omitempty"`
	RTTMS       *float64 `json:"rtt_ms"`
	AnsweredAgo *int64   `json:"answered_ago"`
	Lease       *lease   `json:"lease,omitempty"`
}

// lease is the address the AP holds on a tunnel's segment, for its probes
// (0060): who gave it, the gateway it names, and seconds until it ends.
type lease struct {
	Address   string `json:"address"`
	Server    string `json:"server,omitempty"`
	Router    string `json:"router,omitempty"`
	ExpiresIn int64  `json:"expires_in"`
}

// loopState is a tunnel port the loop guard took off its tunnels (0059):
// the device whose frame came back, the VNI it carries, the device the frame
// came back in on, and how many seconds ago.
type loopState struct {
	Port   string `json:"port"`
	Device string `json:"device"`
	VNI    int    `json:"vni,omitempty"`
	CameIn string `json:"came_in,omitempty"`
	Ago    int64  `json:"ago"`
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
	// BSSTransition says whether hostapd has 802.11v, which band steering
	// and BSS transition need (0057); absent when not known.
	BSSTransition *bool `json:"bss_transition,omitempty"`
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
// how each is doing (0020, 0022), by the prober's verdicts (0061). A
// transport not started, a VXLAN fallback waiting, is off. With automatic
// switching, the last switch, and why a switch that is due cannot be made.
type transportState struct {
	Active       string      `json:"active"`
	Primary      string      `json:"primary,omitempty"`
	Fallback     string      `json:"fallback,omitempty"`
	LastSwitch   *lastSwitch `json:"last_switch,omitempty"`
	CannotSwitch string      `json:"cannot_switch,omitempty"`
}

// lastSwitch is a network's last move between its transports (0061): from
// which to which, why, and how many seconds ago.
type lastSwitch struct {
	From string `json:"from"`
	To   string `json:"to"`
	Why  string `json:"why"`
	Ago  int64  `json:"ago"`
}

var (
	verdicts = map[string]bool{"up": true, "down": true, "unverified": true, "unknown": true, "off": true}
	bands    = map[string]bool{"2g": true, "5g": true, "6g": true}
	actives  = map[string]bool{"primary": true, "fallback": true, "none": true}
	// What the prober says of a VLAN watched on the uplink (0064).
	watchVerdicts = map[string]bool{"present": true, "silent": true, "unknown": true}
	sides         = map[string]bool{"primary": true, "fallback": true}
	healths       = map[string]bool{"": true, "up": true, "down": true, "unknown": true, "unverified": true, "off": true}
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
	if x := st.VXLAN; x != nil {
		if len(x.Tunnels) > 64 || x.UplinkMTU < 0 || x.UplinkMTU > 65535 {
			return badRequest("vxlan: at most 64 tunnels, and an uplink MTU from 0 to 65535")
		}
		for _, t := range x.Tunnels {
			if t.VNI < 1 || t.VNI > 16777215 || t.Port < 1 || t.Port > 65535 || t.MTU < 0 || t.MTU > 9000 || net.ParseIP(t.Peer) == nil {
				return badRequest("vxlan: each tunnel has a VNI from 1 to 16777215, its peer's IP address, a port and an MTU of at most 9000")
			}
			if t.FromVLAN < 0 || t.FromVLAN > 4094 || (t.FromAddress != "" && (t.FromVLAN == 0 || net.ParseIP(t.FromAddress).To4() == nil)) {
				return badRequest("vxlan: a tunnel starts from a VLAN from 1 to 4094, and the AP's address there is an IPv4 address")
			}
			if err := t.Probe.check(); err != nil {
				return err
			}
		}
		if len(x.Loops) > 32 {
			return badRequest("vxlan: at most 32 loops")
		}
		for _, l := range x.Loops {
			if !portNameRE.MatchString(l.Port) || !deviceRE.MatchString(l.Device) || (l.CameIn != "" && !deviceRE.MatchString(l.CameIn)) ||
				l.VNI < 0 || l.VNI > 16777215 || l.Ago < 0 {
				return badRequest("vxlan: each loop has its port's name, the devices the frame went out and came in on, a VNI and seconds ago")
			}
		}
	}
	for id, t := range st.Transports {
		if !networkIDRE.MatchString(id) {
			return badRequest("%q is not a network ID", id)
		}
		if !actives[t.Active] || !healths[t.Primary] || !healths[t.Fallback] {
			return badRequest("network %s: active is primary, fallback or none; health is up, down, unverified, unknown or off", id)
		}
		if l := t.LastSwitch; l != nil && (!sides[l.From] || !sides[l.To] || l.From == l.To || l.Why == "" || len(l.Why) > 200 || l.Ago < 0) {
			return badRequest("network %s: the last switch is from one transport to the other (primary or fallback), with why, in at most 200 characters, and seconds ago", id)
		}
		if len(t.CannotSwitch) > 200 {
			return badRequest("network %s: why it cannot switch is at most 200 characters", id)
		}
	}
	if len(st.VLANProbes) > 64 {
		return badRequest("at most 64 VLAN probes")
	}
	for _, v := range st.VLANProbes {
		if v.VLAN < 1 || v.VLAN > 4094 || v.Probe == nil {
			return badRequest("vlan_probes: each has a VLAN from 1 to 4094 and what its probe found")
		}
		if err := v.Probe.check(); err != nil {
			return err
		}
	}
	if len(st.UplinkVLANs) > 64 {
		return badRequest("at most 64 VLANs on the uplink")
	}
	seen := map[int]bool{}
	for _, v := range st.UplinkVLANs {
		if v.VLAN < 1 || v.VLAN > 4094 || seen[v.VLAN] || !watchVerdicts[v.Verdict] || (v.HeardAgo != nil && *v.HeardAgo < 0) {
			return badRequest("uplink_vlans: each is a distinct VLAN from 1 to 4094, with its verdict (present, silent or unknown) and seconds since it was heard, not negative")
		}
		seen[v.VLAN] = true
	}
	if n := st.UplinkNeighbor; n != nil {
		named := map[int]bool{}
		for _, v := range n.VLANs {
			if v < 1 || v > 4094 || named[v] {
				return badRequest("uplink_neighbor: the VLANs it names are distinct, from 1 to 4094")
			}
			named[v] = true
		}
		for _, t := range []string{n.Chassis, n.System, n.Port, n.PortDescription} {
			if len(t) > 255 || !printable(t) {
				return badRequest("uplink_neighbor: its names are printable, and at most 255 characters")
			}
		}
		if n.NativeVLAN < 0 || n.NativeVLAN > 4094 || n.Ago < 0 {
			return badRequest("uplink_neighbor: a native VLAN from 1 to 4094, and seconds since it was heard, not negative")
		}
	}
	return nil
}

// printable says whether s is printable ASCII, as the prober keeps the
// switch's names (0064).
func printable(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < ' ' || s[i] > '~' {
			return false
		}
	}
	return true
}

func (p *probeState) check() error {
	if p == nil {
		return nil
	}
	bad := func(f *float64) bool { return f != nil && (*f < 0 || *f > 1e6) }
	if !verdicts[p.Verdict] || p.Interval < 0 || p.Interval > 300 || len(p.Asks) > 8 || bad(p.UnderlayMS) || bad(p.RTTMS) ||
		(p.AnsweredAgo != nil && *p.AnsweredAgo < 0) || (p.From != "" && net.ParseIP(p.From) == nil) {
		return badRequest("vxlan: a tunnel's probe has a verdict (up, down, unverified, unknown or off), an interval of at most 300 seconds, at most 8 addresses it asks, times that are not negative, and the IP address that answered")
	}
	for _, a := range p.Asks {
		if net.ParseIP(a) == nil {
			return badRequest("vxlan: a probe asks IP addresses")
		}
	}
	if l := p.Lease; l != nil {
		ip := func(s string, need bool) bool { return (s == "" && !need) || net.ParseIP(s).To4() != nil }
		if !ip(l.Address, true) || !ip(l.Server, false) || !ip(l.Router, false) || l.ExpiresIn < 0 {
			return badRequest("vxlan: a probe's lease is an IPv4 address, from a server and with a gateway that are IPv4 addresses, and seconds left that are not negative")
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
		res, err := s.compose(c.state, id, s.reveal)
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
