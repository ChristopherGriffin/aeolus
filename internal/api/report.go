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
	"github.com/ChristopherGriffin/aeolus/internal/identify"
	"github.com/ChristopherGriffin/aeolus/internal/oui"
	"github.com/ChristopherGriffin/aeolus/internal/rendercheck"
	"github.com/ChristopherGriffin/aeolus/internal/uci"
)

// The render check and the reports (0039).

const (
	maxUCI    = 256 << 10
	maxReport = 512 << 10 // room for 256 clients (0066, 0067)
	maxError  = 2048
)

var (
	hashRE      = regexp.MustCompile(`^[0-9a-f]{64}$`)
	networkIDRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	portNameRE  = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`) // as the schema names ports
	speedRE     = regexp.MustCompile(`^([0-9]{1,6}[FH])?$`)
	// deviceRE is a Linux device's name, as the loop guard reports one (0059).
	deviceRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@-]{0,14}$`)
	// ifaceNameRE is a netifd interface's name, a UCI section's.
	ifaceNameRE = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
	// macRE is a MAC as Linux writes one; hexRE, bytes as the prober writes
	// what it doesn't know of the switch's LLDP, at most 64 of them (0064).
	macRE = regexp.MustCompile(`^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)
	hexRE = regexp.MustCompile(`^([0-9a-f]{2}){0,64}$`)
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
	UplinkPort     *uplinkPort     `json:"uplink_port,omitempty"`
	// What the AP saw of its Wi-Fi clients' DHCP, by network (0065).
	DHCP map[string]dhcpState `json:"dhcp,omitempty"`
	// Every Wi-Fi client on the AP (0066).
	Clients []wifiClient `json:"clients,omitempty"`
	// Its clock (0069).
	Time *timeState `json:"time,omitempty"`
	// The per-user keys it has (0070): their version, and how many.
	Keys *keysState `json:"keys,omitempty"`
	// What its radio resource management found (0073).
	RRM *rrmState `json:"rrm,omitempty"`
	// The agent it runs, and its last update (0079).
	Agent *agentState `json:"agent,omitempty"`
	// netifd's network.wireless object is gone: the radios run, but the
	// agent, its prober and RRM see none of them, nor their clients, until
	// the network restarts on the AP (2026-10-06).
	WirelessMissing bool `json:"wireless_missing,omitempty"`
}

// rrmState is what the AP's radio resource management found (0073): its
// management address, where its neighbours reach it; the bands its beacons
// say it is an Aeolus AP on; the other Aeolus APs it hears in the air or
// keeps neighbours with over the wire; how it rates each channel; its last
// moves; and the radios its power control holds (0077).
type rrmState struct {
	Address    string         `json:"address"`
	Advertised []string       `json:"advertised"`
	Neighbours []rrmNeighbour `json:"neighbours"`
	Ratings    []rrmRating    `json:"ratings"`
	Moves      []rrmMove      `json:"moves"`
	APC        []rrmPower     `json:"apc,omitempty"`
}

// rrmPower is a radio whose power the AP's power control holds (0077): the
// power it holds it at and the most it may, in dBm, none where it couldn't;
// how many neighbours it looks for, and its target; how many of them hear
// it, and the weakest of them, as of its last look; its last step, in dB;
// why: new (just taken over), looking (fewer neighbours hear it than it
// looks for), below (the weakest hears it below the target), ceiling (it
// would go up, but is at its most), target (they hear it at the target),
// above (all hear it well above), floor (it would go down, but is at its
// least) or failed (its power couldn't be read or set); and seconds since
// its power last moved.
type rrmPower struct {
	Radio   string `json:"radio"`
	Band    string `json:"band"`
	Power   *int   `json:"power"`
	Ceiling *int   `json:"ceiling"`
	Wanted  int    `json:"wanted"`
	Target  int    `json:"target"`
	Count   *int   `json:"count"`
	Weakest *int   `json:"weakest"`
	Step    int    `json:"step"`
	Why     string `json:"why"`
	Ago     *int64 `json:"ago"`
}

// rrmMove is one of the AP's moves (0073): a radio's band, the channel it
// was on and the one it went for; why: start, shared, better or
// interference; what came of it: announced (claimed in its hellos, not yet
// made), moved, yielded (to AP's claim), withdrawn or failed; and when it
// came to that, in Unix seconds.
type rrmMove struct {
	Band  string `json:"band"`
	From  int    `json:"from"`
	To    int    `json:"to"`
	Why   string `json:"why"`
	State string `json:"state"`
	At    int64  `json:"at"`
	AP    string `json:"ap,omitempty"`
}

// rrmRating is one channel as the AP rates it (0073), lower being better:
// its lasting rating, earned over many visits, and its rating now; how busy
// others kept it on the last visit, in percent; its noise floor, where the
// driver says; how many other networks were heard there; how many visits it
// has had, and seconds since the last; whether one of the AP's radios is
// on it; the neighbours using it, which blot it out; and whether it is the
// AP's best on its band: the best rated no neighbour uses, or where
// neighbours use them all, the one whose nearest user is furthest away.
type rrmRating struct {
	Band      string   `json:"band"`
	Channel   int      `json:"channel"`
	Cost      int      `json:"cost"`
	Now       int      `json:"now"`
	Busy      int      `json:"busy"`
	Noise     *int     `json:"noise"`
	Networks  int      `json:"networks"`
	Visits    int      `json:"visits"`
	Ago       int64    `json:"ago"`
	Own       bool     `json:"own"`
	BlottedBy []string `json:"blotted_by"`
	Best      bool     `json:"best"`
}

// rrmNeighbour is one other Aeolus AP: its ID and management address; up
// when hellos go both ways, one-way when its hellos come but don't list this
// AP, heard when it is only heard in the air, down when its hellos stopped;
// whether it is among the three this AP hears most strongly on a band;
// seconds since its last hello; and on each band, how strongly each hears
// the other, and its channel and width there, as its hellos say.
type rrmNeighbour struct {
	AP       string    `json:"ap"`
	Address  string    `json:"address"`
	State    string    `json:"state"`
	Chosen   bool      `json:"chosen"`
	HelloAgo *int64    `json:"hello_ago"`
	Bands    []rrmBand `json:"bands"`
}

type rrmBand struct {
	Band        string `json:"band"`
	Signal      *int   `json:"signal"`
	TheirSignal *int   `json:"their_signal"`
	Channel     *int   `json:"channel"`
	Width       *int   `json:"width"`
}

var (
	apIDRE    = regexp.MustCompile(`^ap-[0-9a-f]{12}$`)
	rrmStates = map[string]bool{"up": true, "one-way": true, "heard": true, "down": true}
	rrmWhys   = map[string]bool{"start": true, "shared": true, "better": true, "interference": true}
	rrmEnds   = map[string]bool{"announced": true, "moved": true, "yielded": true, "withdrawn": true, "failed": true}
	rrmWidths = map[int]bool{0: true, 20: true, 40: true, 80: true, 160: true, 320: true}
	apcWhys   = map[string]bool{"new": true, "looking": true, "below": true, "ceiling": true, "target": true, "above": true, "floor": true, "failed": true}
	radioRE   = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,32}$`)
)

func (r *rrmState) check() error {
	if r == nil {
		return nil
	}
	bad := badRequest("rrm: an address, at most 3 bands, and at most 64 neighbours, each an AP ID with its address, state, and on each band a signal from -127 to 0 dBm, a channel and a width")
	if (r.Address != "" && net.ParseIP(r.Address) == nil) || len(r.Advertised) > 3 || len(r.Neighbours) > 64 || len(r.Ratings) > 64 {
		return bad
	}
	for _, rt := range r.Ratings {
		if !bands[rt.Band] || rt.Channel < 1 || rt.Channel > 233 || rt.Cost < 0 || rt.Cost > 100000 || rt.Now < 0 || rt.Now > 100000 ||
			rt.Busy < 0 || rt.Busy > 100 || !dbm(rt.Noise) || rt.Networks < 0 || rt.Networks > 1000 || rt.Visits < 0 || rt.Ago < 0 ||
			len(rt.BlottedBy) > 8 {
			return badRequest("rrm: a rating is a band, a channel from 1 to 233, costs from 0, busy from 0 to 100, a noise floor in dBm, counts from 0, and at most 8 APs blotting it out")
		}
		for _, ap := range rt.BlottedBy {
			if !apIDRE.MatchString(ap) {
				return bad
			}
		}
	}
	if len(r.Moves) > 16 {
		return bad
	}
	for _, m := range r.Moves {
		if !bands[m.Band] || m.From < 0 || m.From > 233 || m.To < 1 || m.To > 233 || !rrmWhys[m.Why] || !rrmEnds[m.State] || m.At < 0 ||
			(m.AP != "" && !apIDRE.MatchString(m.AP)) {
			return badRequest("rrm: at most 16 moves, each a band, the channels from and to, why (start, shared, better or interference), what came of it (announced, moved, yielded, withdrawn or failed), when, and the AP it yielded to")
		}
	}
	for _, b := range r.Advertised {
		if !bands[b] {
			return bad
		}
	}
	if len(r.APC) > 4 {
		return bad
	}
	for _, p := range r.APC {
		if !radioRE.MatchString(p.Radio) || !bands[p.Band] || !power(p.Power) || !power(p.Ceiling) || p.Wanted < 1 || p.Wanted > 6 ||
			p.Target < -85 || p.Target > -50 || (p.Count != nil && (*p.Count < 0 || *p.Count > 6)) || !dbm(p.Weakest) ||
			p.Step < -3 || p.Step > 3 || !apcWhys[p.Why] || (p.Ago != nil && *p.Ago < 0) {
			return badRequest("rrm: at most 4 radios under power control, each a radio and band, its power and ceiling from 0 to 40 dBm, 1 to 6 neighbours looked for, a target from -85 to -50 dBm, how many hear it, the weakest in dBm, a step from -3 to 3 dB, and why")
		}
	}
	for _, n := range r.Neighbours {
		if !apIDRE.MatchString(n.AP) || (n.Address != "" && net.ParseIP(n.Address) == nil) || !rrmStates[n.State] ||
			(n.HelloAgo != nil && *n.HelloAgo < 0) || len(n.Bands) > 3 {
			return bad
		}
		for _, b := range n.Bands {
			if !bands[b.Band] || !dbm(b.Signal) || !dbm(b.TheirSignal) ||
				(b.Channel != nil && (*b.Channel < 0 || *b.Channel > 233)) || (b.Width != nil && !rrmWidths[*b.Width]) {
				return bad
			}
		}
	}
	return nil
}

func dbm(v *int) bool {
	return v == nil || (*v >= -127 && *v <= 0)
}

func power(v *int) bool {
	return v == nil || (*v >= 0 && *v <= 40)
}

type keysState struct {
	Version string `json:"version"`
	Count   int    `json:"count"`
}

var keysVersionRE = regexp.MustCompile(`^[0-9a-f]{0,64}$`)

func (k *keysState) check() error {
	if k != nil && (!keysVersionRE.MatchString(k.Version) || k.Count < 0 || k.Count > 1<<20) {
		return badRequest("keys: version is hex, count 0 or more")
	}
	return nil
}

// timeState is whether the AP's clock is synchronized, as ntpd last said
// (0069): null until it has said; its stratum and offset in seconds; seconds
// since it said so; and the time servers ntpd was started with.
type timeState struct {
	Synced  *bool    `json:"synced"`
	Stratum *int     `json:"stratum"`
	Offset  *float64 `json:"offset"`
	Ago     *int64   `json:"ago"`
	Servers []string `json:"servers"`
}

func (t *timeState) check() error {
	if t == nil {
		return nil
	}
	if (t.Stratum != nil && (*t.Stratum < 0 || *t.Stratum > 16)) || (t.Ago != nil && *t.Ago < 0) || len(t.Servers) > 8 {
		return badRequest("time: stratum is 0 to 16, ago cannot be negative, at most 8 servers")
	}
	for _, sv := range t.Servers {
		if err := plainText("time server", sv, 253); err != nil {
			return err
		}
	}
	return nil
}

// wifiClient is one Wi-Fi client on an AP (0066), as nl80211 and the DHCP
// watch saw it: its MAC; its network (Aeolus's, or none for the AP's own),
// SSID and band; its signal and average; its rates each way, in Mbit/s, with
// MCS and streams; its data, packets, retries and failures; how long it has
// been connected, and since it was last heard; and on Aeolus's networks the
// address it uses, the host name it gave, and how it does with DHCP.
type wifiClient struct {
	MAC        string   `json:"mac"`
	Network    string   `json:"network"`
	SSID       string   `json:"ssid"`
	Band       string   `json:"band"`
	Signal     *int     `json:"signal"`
	SignalAvg  *int     `json:"signal_avg"`
	RxRate     *float64 `json:"rx_rate"`
	RxMCS      *int     `json:"rx_mcs"`
	RxNSS      *int     `json:"rx_nss"`
	TxRate     *float64 `json:"tx_rate"`
	TxMCS      *int     `json:"tx_mcs"`
	TxNSS      *int     `json:"tx_nss"`
	RxBytes    int64    `json:"rx_bytes"`
	TxBytes    int64    `json:"tx_bytes"`
	RxPackets  int64    `json:"rx_packets"`
	TxPackets  int64    `json:"tx_packets"`
	TxRetries  int64    `json:"tx_retries"`
	TxFailed   int64    `json:"tx_failed"`
	Connected  int64    `json:"connected"`
	InactiveMS int64    `json:"inactive_ms"`
	Address    string   `json:"address"`
	Host       string   `json:"host"`
	DHCP       string   `json:"dhcp"`
	// What it said of itself in DHCP, and the 802.11 features its
	// association showed (0067).
	// The VLAN a per-user key put it in, if any (0070).
	VLAN        *int   `json:"vlan"`
	VendorClass string `json:"vendor_class"`
	Params      string `json:"params"`
	Gen         string `json:"gen"`
	K           *bool  `json:"k"`
	V           *bool  `json:"v"`
	W           *bool  `json:"w"`
	MBO         *bool  `json:"mbo"`
	WMM         *bool  `json:"wmm"`
	// Added by the manager when the report arrives (0067): the maker by
	// OUI, whether the MAC is private, and a guess at what the device is.
	Maker   string `json:"maker,omitempty"`
	Private bool   `json:"private,omitempty"`
	Kind    string `json:"kind,omitempty"`
	OS      string `json:"os,omitempty"`
	Basis   string `json:"basis,omitempty"`
}

// gens are the 802.11 generations a client can show: n, ac, ax and be, or
// none, for a/b/g (0067).
var gens = map[string]bool{"": true, "n": true, "ac": true, "ax": true, "be": true}

// paramsRE is a DHCP parameter request list, as the prober writes it.
var paramsRE = regexp.MustCompile(`^([0-9]{1,3}(,[0-9]{1,3}){0,63})?$`)

// identify adds to each client what the manager knows of it (0067): its
// maker, by OUI; whether its MAC is private; and a guess at its kind and
// operating system, with what the guess went on.
func (st *stateReport) identify() {
	for i := range st.Clients {
		c := &st.Clients[i]
		c.Maker, c.Private = oui.Lookup(c.MAC)
		g := identify.Of(identify.Evidence{Host: c.Host, VendorClass: c.VendorClass, Params: c.Params, Maker: c.Maker, Private: c.Private})
		c.Kind, c.OS, c.Basis = g.Kind, g.OS, g.Basis
	}
}

// The verdicts of the DHCP watch on a client (0065); "" off Aeolus's networks.
var clientDHCP = map[string]bool{"": true, "ok": true, "static": true, "none": true, "unknown": true}

// check holds a client to what the prober writes.
func (c wifiClient) check() error {
	bad := badRequest("clients: each has a MAC, a network ID or none, an SSID of at most 32 bytes, a band (2g, 5g or 6g), a signal from -150 to 50 dBm, rates up to 100000 Mbit/s, MCS up to 31 and up to 16 streams, counts not negative, an IPv4 address or none, a printable host name of at most 64 characters, and a DHCP verdict (ok, static, none or unknown)")
	inRange := func(p *int, lo, hi int) bool { return p == nil || (*p >= lo && *p <= hi) }
	rate := func(p *float64) bool { return p == nil || (*p >= 0 && *p <= 100000) }
	ip := net.ParseIP(c.Address)
	if !macRE.MatchString(c.MAC) || (c.Network != "" && !networkIDRE.MatchString(c.Network)) || len(c.SSID) > 32 ||
		(c.Band != "" && !bands[c.Band]) || !inRange(c.Signal, -150, 50) || !inRange(c.SignalAvg, -150, 50) ||
		!rate(c.RxRate) || !rate(c.TxRate) || !inRange(c.RxMCS, 0, 31) || !inRange(c.TxMCS, 0, 31) || !inRange(c.RxNSS, 0, 16) || !inRange(c.TxNSS, 0, 16) ||
		c.RxBytes < 0 || c.TxBytes < 0 || c.RxPackets < 0 || c.TxPackets < 0 || c.TxRetries < 0 || c.TxFailed < 0 || c.Connected < 0 || c.InactiveMS < 0 ||
		(c.Address != "" && (ip == nil || ip.To4() == nil)) || len(c.Host) > 64 || !printable(c.Host) || !clientDHCP[c.DHCP] ||
		len(c.VendorClass) > 64 || !printable(c.VendorClass) || !paramsRE.MatchString(c.Params) || !gens[c.Gen] ||
		(c.VLAN != nil && (*c.VLAN < 1 || *c.VLAN > 4094)) {
		return bad
	}
	return nil
}

// dhcpState is what the AP saw of one network's DHCP on its Wi-Fi
// interfaces (0065): the servers that answered, and when last; how many of
// the clients' requests in the last 10 minutes were answered, and how many
// not, and which clients made those; requests more than one server answered;
// and the clients that don't use DHCP, with the address each shows.
type dhcpState struct {
	Servers           []dhcpServer    `json:"servers"`
	Answered          int             `json:"answered"`
	Unanswered        int             `json:"unanswered"`
	UnansweredClients []string        `json:"unanswered_clients"`
	Duplicates        []dhcpDuplicate `json:"duplicates"`
	Without           []dhcpClient    `json:"without"`
}

type dhcpServer struct {
	ID      string `json:"id"`
	MAC     string `json:"mac"`
	Answers int    `json:"answers"`
	Ago     int64  `json:"ago"`
}

type dhcpDuplicate struct {
	Servers []string `json:"servers"`
	Client  string   `json:"client"`
	Ago     int64    `json:"ago"`
}

type dhcpClient struct {
	MAC       string  `json:"mac"`
	JoinedAgo int64   `json:"joined_ago"`
	Address   *string `json:"address"`
}

// check holds a network's DHCP findings to what the prober writes: IPv4
// addresses, MACs, counts that are not negative, and short lists.
func (d dhcpState) check(id string) error {
	bad := badRequest("dhcp.%s: servers by IPv4 address and MAC, counts not negative, clients by MAC, at most 8 servers, 16 unanswered clients, 8 duplicates and 32 clients without DHCP", id)
	ipv4 := func(s string) bool {
		ip := net.ParseIP(s)
		return ip != nil && ip.To4() != nil
	}
	if !networkIDRE.MatchString(id) || len(d.Servers) > 8 || len(d.UnansweredClients) > 16 || len(d.Duplicates) > 8 || len(d.Without) > 32 ||
		d.Answered < 0 || d.Unanswered < 0 {
		return bad
	}
	for _, s := range d.Servers {
		if !ipv4(s.ID) || !macRE.MatchString(s.MAC) || s.Answers < 0 || s.Ago < 0 {
			return bad
		}
	}
	for _, m := range d.UnansweredClients {
		if !macRE.MatchString(m) {
			return bad
		}
	}
	for _, x := range d.Duplicates {
		if !macRE.MatchString(x.Client) || x.Ago < 0 || len(x.Servers) < 2 || len(x.Servers) > 8 {
			return bad
		}
		for _, s := range x.Servers {
			if !ipv4(s) {
				return bad
			}
		}
	}
	for _, c := range d.Without {
		if !macRE.MatchString(c.MAC) || c.JoinedAgo < 0 || (c.Address != nil && !ipv4(*c.Address)) {
			return bad
		}
	}
	return nil
}

// uplinkPort is what the AP knows of its uplink itself (0064): its name, MAC
// and MTU; how often its link has come and gone; the VLANs it carries, as
// its network config has them, and which is the management VLAN; and its
// counters.
type uplinkPort struct {
	Name           string        `json:"name"`
	MAC            string        `json:"mac"`
	MTU            int           `json:"mtu"`
	CarrierChanges int64         `json:"carrier_changes"`
	VLANs          []carriedVLAN `json:"vlans,omitempty"`
	ManagementVLAN int           `json:"management_vlan,omitempty"`
	RxBytes        int64         `json:"rx_bytes"`
	TxBytes        int64         `json:"tx_bytes"`
	RxPackets      int64         `json:"rx_packets"`
	TxPackets      int64         `json:"tx_packets"`
	RxErrors       int64         `json:"rx_errors"`
	TxErrors       int64         `json:"tx_errors"`
	RxDropped      int64         `json:"rx_dropped"`
	TxDropped      int64         `json:"tx_dropped"`
}

// carriedVLAN is a VLAN on the uplink, tagged or not.
type carriedVLAN struct {
	VLAN   int  `json:"vlan"`
	Tagged bool `json:"tagged"`
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

// uplinkNeighbor is all the switch's LLDP says of itself and of the port the
// AP's uplink is on (0064), as the prober reads it, and seconds since it last
// said so. What the prober doesn't know is in Other, as hex.
type uplinkNeighbor struct {
	Chassis             string            `json:"chassis,omitempty"`
	ChassisKind         string            `json:"chassis_kind,omitempty"`
	System              string            `json:"system,omitempty"`
	SystemDescription   string            `json:"system_description,omitempty"`
	Port                string            `json:"port,omitempty"`
	PortKind            string            `json:"port_kind,omitempty"`
	PortDescription     string            `json:"port_description,omitempty"`
	TTL                 *int              `json:"ttl,omitempty"`
	Capabilities        []string          `json:"capabilities,omitempty"`
	EnabledCapabilities []string          `json:"enabled_capabilities,omitempty"`
	Management          []lldpManagement  `json:"management,omitempty"`
	NativeVLAN          int               `json:"native_vlan,omitempty"`
	VLANs               []int             `json:"vlans,omitempty"`
	VLANNames           map[string]string `json:"vlan_names,omitempty"`
	ProtocolVLANs       []int             `json:"protocol_vlans,omitempty"`
	Protocols           []string          `json:"protocols,omitempty"`
	Aggregation         *lldpAggregation  `json:"aggregation,omitempty"`
	MaxFrame            int               `json:"max_frame,omitempty"`
	MACPHY              *lldpMACPHY       `json:"mac_phy,omitempty"`
	Power               *lldpPower        `json:"power,omitempty"`
	MED                 *lldpMED          `json:"med,omitempty"`
	Other               []lldpOther       `json:"other,omitempty"`
	Ago                 int64             `json:"ago"`
}

type lldpManagement struct {
	Address       string `json:"address"`
	Interface     int64  `json:"interface"`
	InterfaceKind string `json:"interface_kind"`
}

type lldpAggregation struct {
	Capable bool  `json:"capable"`
	Enabled bool  `json:"enabled"`
	Port    int64 `json:"port"`
}

type lldpMACPHY struct {
	AutonegSupported bool   `json:"autoneg_supported"`
	AutonegEnabled   bool   `json:"autoneg_enabled"`
	Advertised       string `json:"advertised"`
	MAU              int    `json:"mau"`
	MAUName          string `json:"mau_name,omitempty"`
}

type lldpPower struct {
	PSE        bool     `json:"pse"`
	Supported  bool     `json:"supported"`
	Enabled    bool     `json:"enabled"`
	Pair       string   `json:"pair,omitempty"`
	Class      *int     `json:"class,omitempty"`
	RequestedW *float64 `json:"requested_w,omitempty"`
	AllocatedW *float64 `json:"allocated_w,omitempty"`
}

type lldpMED struct {
	Capabilities string            `json:"capabilities,omitempty"`
	Class        int               `json:"class,omitempty"`
	Policies     []lldpPolicy      `json:"policies,omitempty"`
	Inventory    map[string]string `json:"inventory,omitempty"`
	Location     string            `json:"location,omitempty"`
	PowerW       *float64          `json:"power_w,omitempty"`
}

type lldpPolicy struct {
	Application string `json:"application"`
	Unknown     bool   `json:"unknown"`
	Tagged      bool   `json:"tagged"`
	VLAN        int    `json:"vlan"`
	Priority    int    `json:"priority"`
	DSCP        int    `json:"dscp"`
}

type lldpOther struct {
	Type    int    `json:"type"`
	OUI     string `json:"oui,omitempty"`
	Subtype int    `json:"subtype,omitempty"`
	Data    string `json:"data"`
}

// check holds the switch's account to what the prober can write: printable
// names of at most 255 characters, short lists, VLANs from 1 to 4094, and hex
// for what it doesn't know.
func (n *uplinkNeighbor) check() error {
	bad := func(what string) error {
		return badRequest("uplink_neighbor: %s", what)
	}
	texts := []string{n.Chassis, n.ChassisKind, n.System, n.SystemDescription, n.Port, n.PortKind, n.PortDescription}
	texts = append(texts, n.Capabilities...)
	texts = append(texts, n.EnabledCapabilities...)
	for _, m := range n.Management {
		texts = append(texts, m.Address, m.InterfaceKind)
	}
	if p := n.Power; p != nil {
		texts = append(texts, p.Pair)
	}
	if m := n.MED; m != nil {
		texts = append(texts, m.Capabilities)
		for k, v := range m.Inventory {
			texts = append(texts, k, v)
		}
		for _, p := range m.Policies {
			texts = append(texts, p.Application)
			if p.VLAN < 0 || p.VLAN > 4095 || p.Priority < 0 || p.Priority > 7 || p.DSCP < 0 || p.DSCP > 63 {
				return bad("a network policy's VLAN, priority or DSCP is out of range")
			}
		}
		if len(m.Policies) > 8 || len(m.Inventory) > 8 || !hexRE.MatchString(m.Location) {
			return bad("at most 8 network policies and 8 inventory items, and the location in hex")
		}
	}
	for _, t := range texts {
		if len(t) > 255 || !printable(t) {
			return bad("its names are printable, and at most 255 characters")
		}
	}
	if len(n.Capabilities) > 16 || len(n.EnabledCapabilities) > 16 || len(n.Management) > 4 || len(n.Protocols) > 16 ||
		len(n.ProtocolVLANs) > 64 || len(n.Other) > 16 || len(n.VLANs) > 4094 || len(n.VLANNames) > 4094 {
		return bad("too many capabilities, addresses, protocols, VLANs or unknown TLVs")
	}
	named := map[int]bool{}
	for _, v := range n.VLANs {
		if v < 1 || v > 4094 || named[v] {
			return bad("the VLANs it names are distinct, from 1 to 4094")
		}
		named[v] = true
	}
	for k, name := range n.VLANNames {
		if v, err := strconv.Atoi(k); err != nil || !named[v] || len(name) > 32 || !printable(name) {
			return bad("each VLAN name is for a VLAN it names, printable, and at most 32 characters")
		}
	}
	for _, v := range n.ProtocolVLANs {
		if v < 0 || v > 4094 {
			return bad("protocol VLANs are from 0 to 4094")
		}
	}
	for _, p := range n.Protocols {
		if !hexRE.MatchString(p) {
			return bad("protocols are in hex")
		}
	}
	for _, o := range n.Other {
		if o.Type < 0 || o.Type > 127 || o.Subtype < 0 || o.Subtype > 255 || !hexRE.MatchString(o.OUI) || !hexRE.MatchString(o.Data) {
			return bad("an unknown TLV has a type, and its data in hex")
		}
	}
	if (n.TTL != nil && (*n.TTL < 0 || *n.TTL > 65535)) || n.NativeVLAN < 0 || n.NativeVLAN > 4094 || n.MaxFrame < 0 || n.MaxFrame > 65535 || n.Ago < 0 {
		return bad("a TTL and maximum frame size from 0 to 65535, a native VLAN from 1 to 4094, and seconds since it was heard, not negative")
	}
	return nil
}

// check holds the uplink's own account (0064).
func (u *uplinkPort) check() error {
	if !portNameRE.MatchString(u.Name) || !macRE.MatchString(u.MAC) || u.MTU < 0 || u.MTU > 65535 || u.CarrierChanges < 0 ||
		u.ManagementVLAN < 0 || u.ManagementVLAN > 4094 || len(u.VLANs) > 4094 {
		return badRequest("uplink_port: a port's name, its MAC, an MTU up to 65535, a management VLAN up to 4094, and counts that are not negative")
	}
	for _, c := range []int64{u.RxBytes, u.TxBytes, u.RxPackets, u.TxPackets, u.RxErrors, u.TxErrors, u.RxDropped, u.TxDropped} {
		if c < 0 {
			return badRequest("uplink_port: counters are not negative")
		}
	}
	seen := map[int]bool{}
	for _, v := range u.VLANs {
		if v.VLAN < 1 || v.VLAN > 4094 || seen[v.VLAN] {
			return badRequest("uplink_port: its VLANs are distinct, from 1 to 4094")
		}
		seen[v.VLAN] = true
	}
	return nil
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
// its MTU, the VLAN it starts from and the AP's address there (0063), the
// gateway there and whether it answers, by the AP's neighbour table; the
// AP's other interfaces holding that address on that VLAN, and seconds
// since the prober last put back routes the start lost
// (2026-10-06); whether it is up, or standing by as a fallback, and what
// the prober found.
type tunnelState struct {
	VNI                int         `json:"vni"`
	Peer               string      `json:"peer"`
	Port               int         `json:"port"`
	MTU                int         `json:"mtu"`
	FromVLAN           int         `json:"from_vlan,omitempty"`
	FromAddress        string      `json:"from_address,omitempty"`
	FromGateway        string      `json:"from_gateway,omitempty"`
	FromGatewayAnswers *bool       `json:"from_gateway_answers,omitempty"`
	FromShared         []string    `json:"from_shared,omitempty"`
	FromPutBackAgo     *int64      `json:"from_put_back_ago,omitempty"`
	Up                 bool        `json:"up"`
	Standby            bool        `json:"standby,omitempty"`
	Probe              *probeState `json:"probe,omitempty"`
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
	// Whether it is up, and the power it sends at in dBm, as it runs them,
	// set by Aeolus or not. An older agent leaves them out.
	Up      *bool `json:"up,omitempty"`
	TxPower *int  `json:"txpower,omitempty"`
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
	if err := st.Time.check(); err != nil {
		return err
	}
	if err := st.Keys.check(); err != nil {
		return err
	}
	if err := st.RRM.check(); err != nil {
		return err
	}
	if err := st.Agent.check(); err != nil {
		return err
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
		if rd.TxPower != nil && (*rd.TxPower < 0 || *rd.TxPower > 40) {
			return badRequest("a radio's power is 0 to 40 dBm")
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
			if t.FromVLAN < 0 || t.FromVLAN > 4094 || (t.FromAddress != "" && (t.FromVLAN == 0 || net.ParseIP(t.FromAddress).To4() == nil)) ||
				(t.FromGateway != "" && (t.FromAddress == "" || net.ParseIP(t.FromGateway).To4() == nil)) || (t.FromGatewayAnswers != nil && t.FromGateway == "") {
				return badRequest("vxlan: a tunnel starts from a VLAN from 1 to 4094, and the AP's address and gateway there are IPv4 addresses")
			}
			if len(t.FromShared) > 8 || ((len(t.FromShared) > 0 || t.FromPutBackAgo != nil) && t.FromVLAN == 0) || (t.FromPutBackAgo != nil && *t.FromPutBackAgo < 0) {
				return badRequest("vxlan: a tunnel that starts from a VLAN names at most 8 other interfaces with an address there, and seconds since its routes were put back")
			}
			for _, name := range t.FromShared {
				if !ifaceNameRE.MatchString(name) {
					return badRequest("vxlan: %q is not an interface name", name)
				}
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
		if err := n.check(); err != nil {
			return err
		}
	}
	if u := st.UplinkPort; u != nil {
		if err := u.check(); err != nil {
			return err
		}
	}
	if len(st.DHCP) > 64 {
		return badRequest("dhcp: at most 64 networks")
	}
	for id, d := range st.DHCP {
		if err := d.check(id); err != nil {
			return err
		}
	}
	if len(st.Clients) > 256 {
		return badRequest("clients: at most 256")
	}
	for _, c := range st.Clients {
		if err := c.check(); err != nil {
			return err
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
	req.identify()
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
		var report json.RawMessage
		if l.State != nil {
			report = l.State.Report
		}
		ap := map[string]any{
			"id": id, "name": n.Name, "ancestry": t.Ancestry(id), "version": version,
			"config": config, "problems": len(res.Problems), "seen": l.Seen, "in_sync": inSync,
			"agent": s.agentView(c.state, id, report),
		}
		if wirelessMissing(report) {
			ap["wireless_missing"] = true
		}
		out = append(out, ap)
	}
	writeJSON(w, http.StatusOK, map[string]any{"aps": out})
	return nil
}

// wirelessMissing says whether an AP's last report says netifd lost its
// network.wireless object.
func wirelessMissing(report json.RawMessage) bool {
	var r struct {
		WirelessMissing bool `json:"wireless_missing"`
	}
	return len(report) > 0 && json.Unmarshal(report, &r) == nil && r.WirelessMissing
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
