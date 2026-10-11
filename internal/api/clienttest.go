package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// The client test (0121): an AP with a scan radio joins one of its own
// networks as a client would, on the AP asked for, and says how each step
// of the way online went. It is an action (0104) that carries what to test
// and brings back what it found.

// clientTestAsk is what the AP is told to test: the network's SSID; the
// BSSIDs it may join, one AP's own, or none for the strongest other AP it
// hears; a band, or none for any; and an OpenSpeedTest server to measure
// against, where one is set (system.speed_server). Network and On say what
// was asked, for whoever reads it back; the AP does not read them.
//
// A test over the wire (0128) has wire for its band: it joins nothing, so
// it has no BSSIDs, no user and no speed, and On is the AP itself. AP is
// that AP's ID, which the MAC it tests from is made of.
type clientTestAsk struct {
	Network string   `json:"network"`
	On      string   `json:"on,omitempty"`
	SSID    string   `json:"ssid"`
	BSSIDs  []string `json:"bssids,omitempty"`
	Band    string   `json:"band,omitempty"`
	Speed   string   `json:"speed,omitempty"`
	EAP     *testEAP `json:"eap,omitempty"`
	AP      string   `json:"ap,omitempty"`
	// Round says the manager asked it for a round asked by hand (0129),
	// not in its turn on a schedule: for whoever reads it back.
	Round bool `json:"round,omitempty"`
}

// wireBand is what a test over the wire has for a band (0128): not through
// the air at all, but from the AP's own uplink, on the network's VLAN or
// into its tunnel.
const wireBand = "wire"

// wireTests says whether an AP's agent can test over the wire, as its
// latest report says (0128).
func (s *Server) wireTests(ap hierarchy.NodeID) bool {
	st, err := s.conds.LatestState(ap)
	if err != nil || st == nil {
		return false
	}
	var rep struct {
		WireTests bool `json:"wire_tests"`
	}
	return json.Unmarshal(st.Report, &rep) == nil && rep.WireTests
}

// wireTestOf makes what an AP is told to test over the wire: one of the
// networks it offers, from itself. Any AP whose agent can, a scan radio or
// none; and any network, however it is secured, as the test joins nothing.
func (s *Server) wireTestOf(state *change.State, ap hierarchy.NodeID, network, on string) (clientTestAsk, error) {
	var ask clientTestAsk
	if on != "" && on != string(ap) {
		return ask, badRequest("on: a test over the wire is made from the AP itself, and joins no other")
	}
	if !s.wireTests(ap) {
		return ask, badRequest("this AP's agent has not said it can test over the wire: it is older than that, or has not reported yet")
	}
	res, err := s.compose(state, ap, s.reveal)
	if err != nil {
		return ask, err
	}
	if res.Unassigned {
		return ask, badRequest("this AP is in no folder yet, and offers no network to test")
	}
	nets, _ := res.Doc["network"].(map[string]any)
	n, ok := nets[network].(map[string]any)
	if !ok {
		return ask, badRequest("network: one of the networks this AP offers")
	}
	ssid, _ := n["ssid"].(string)
	return clientTestAsk{Network: network, On: string(ap), SSID: ssid, Band: wireBand, AP: string(ap)}, nil
}

// testEAP is the user a test signs in as, on a network that signs clients
// in by 802.1X (0124): how, and its name. Its password is put in only on
// the way to the AP that makes the test: it is not kept with the action,
// and nobody reads it back.
type testEAP struct {
	Method   string `json:"method"`
	Identity string `json:"identity"`
	Password string `json:"password,omitempty"`
}

// enterprise are the ways a network signs each client in as itself, by
// 802.1X: a test joins one as the network's test user, where it has one.
var enterprise = map[string]bool{"wpa2-enterprise": true, "wpa3-enterprise": true, "wpa2-wpa3-enterprise": true}

// testUser is the user a network's tests sign in as on an AP (0124), as its
// folders have it: nil where the network has none, or no password for it.
// sealed is that password, still sealed.
func testUser(state *change.State, ap hierarchy.NodeID, network string) (user *testEAP, sealed any) {
	cfg, err := state.Org.ResolveAP(ap)
	if err != nil {
		return nil, nil
	}
	f := cfg.Networks[network].Fields
	name, _ := f["radius.test.user"].Value.(string)
	pass, ok := f[change.TestPassword]
	if name == "" || !ok {
		return nil, nil
	}
	method, _ := f["radius.test.method"].Value.(string)
	if method == "" {
		method = "peap"
	}
	return &testEAP{Method: method, Identity: name}, pass.Value
}

// cannotTest says why a test cannot join a network as an AP's config has
// it, or nothing where it can: one that gives each device a passphrase of
// its own from RADIUS (0115), or signs clients in by 802.1X and has no test
// user (0124).
func cannotTest(state *change.State, ap hierarchy.NodeID, network string, n map[string]any) string {
	sec, _ := n["security"].(string)
	if rad, _ := n["radius"].(map[string]any); rad["passphrases"] == true {
		return "gives each device a passphrase of its own from RADIUS (0115), and the test has none"
	}
	if enterprise[sec] {
		if u, _ := testUser(state, ap, network); u == nil {
			return "signs clients in by 802.1X, and has no test user to sign in as (radius.test.user and radius.test.password, 0124)"
		}
		return ""
	}
	if !testable[sec] {
		return "is secured in a way the test does not know"
	}
	return ""
}

// testable are the ways a network is secured that the test can join: with
// the network's own passphrase, or none. One that signs clients in by
// 802.1X needs a user the test does not have.
var testable = map[string]bool{"open": true, "owe": true, "wpa2-psk": true, "wpa3-sae": true, "wpa2-wpa3": true}

// scanRadio says whether an AP says it has a radio another service owns
// (0081): the one a client test borrows. It says so when it enrolls, and
// again when it gains one (0122).
func scanRadio(state *change.State, ap hierarchy.NodeID) bool {
	var f struct {
		Radios []struct {
			Reserved bool `json:"reserved"`
		} `json:"radios"`
	}
	if json.Unmarshal(state.Facts[ap], &f) != nil {
		return false
	}
	for _, r := range f.Radios {
		if r.Reserved {
			return true
		}
	}
	return false
}

// testClients are the Wi-Fi clients that are the APs' own: an AP's scan
// radio, joining a network as a client for a test (0121), is a client to
// the AP it joins, under one of its own AP's MACs. By MAC, in lower case,
// the AP whose it is, of the adopted APs that have a scan radio. A list of
// clients says so of each, as it is no device of anyone's.
func testClients(state *change.State) map[string]hierarchy.NodeID {
	out := map[string]hierarchy.NodeID{}
	t := state.Org.Locations
	aps := make([]hierarchy.NodeID, 0, len(state.Facts))
	for ap := range state.Facts {
		aps = append(aps, ap)
	}
	// In the same order each time: a MAC two APs both say is theirs is the
	// first's, whoever asks and whenever.
	slices.Sort(aps)
	for _, ap := range aps {
		var f struct {
			MAC  string   `json:"mac"`
			MACs []string `json:"macs"`
		}
		// Not an AP no one has adopted: anything may enroll, and say any
		// MAC is its own, a client's among them, which would then be
		// counted as no client. And only an AP with a scan radio makes
		// tests: another AP's MAC among the clients is something else, a
		// link between APs say.
		if n, ok := t.Node(ap); !ok || n.Kind != hierarchy.KindAP || t.InIsolated(ap) {
			continue
		}
		if !scanRadio(state, ap) || json.Unmarshal(state.Facts[ap], &f) != nil {
			continue
		}
		for _, m := range append(f.MACs, f.MAC) {
			m = strings.ToLower(m)
			if _, taken := out[m]; m != "" && !taken {
				out[m] = ap
			}
		}
	}
	return out
}

// testerOf is the name of the AP a client is the scan radio of, where it is
// one: for a caller who may not view that AP, that it is an AP's and no more.
func testerOf(c call, own map[string]hierarchy.NodeID, mac string) string {
	ap, ok := own[strings.ToLower(mac)]
	if !ok {
		return ""
	}
	t := c.state.Org.Locations
	if n, ok := t.Node(ap); ok && roleOn(c, change.Locations, t, ap) >= access.Viewer {
		return n.Name
	}
	return "an AP"
}

// clientTestFor makes what an AP is told to test from what a person asked:
// one of the AP's own networks, by its ID; on, the AP whose networks to
// join, the testing AP itself among them, or none for the strongest other
// AP; and a band, or none.
func (s *Server) clientTestFor(c call, ap hierarchy.NodeID, network, on, band string) (clientTestAsk, error) {
	t := c.state.Org.Locations
	return s.clientTestOf(c.state, ap, network, on, band, true, func(id hierarchy.NodeID) bool {
		return roleOn(c, change.Locations, t, id) >= access.Viewer
	})
}

// clientTestOf is clientTestFor for whoever asks: a person, who may join
// only an AP they may view, or the manager, on a schedule (0123), which
// may leave the speed out.
func (s *Server) clientTestOf(state *change.State, ap hierarchy.NodeID, network, on, band string, speed bool, may func(hierarchy.NodeID) bool) (clientTestAsk, error) {
	var ask clientTestAsk
	t := state.Org.Locations
	if band == wireBand {
		return s.wireTestOf(state, ap, network, on)
	}
	if !scanRadio(state, ap) {
		return ask, badRequest("a client test needs an AP with a scan radio, and this one has not said it has one")
	}
	res, err := s.compose(state, ap, s.reveal)
	if err != nil {
		return ask, err
	}
	if res.Unassigned {
		return ask, badRequest("this AP is in no folder yet, and offers no network to test")
	}
	nets, _ := res.Doc["network"].(map[string]any)
	n, ok := nets[network].(map[string]any)
	if !ok {
		return ask, badRequest("network: one of the networks this AP offers")
	}
	ssid, _ := n["ssid"].(string)
	if why := cannotTest(state, ap, network, n); why != "" {
		return ask, badRequest("network %s %s", network, why)
	}
	if band != "" && !bands[band] {
		return ask, badRequest("band: 2g, 5g or 6g, none for any, or wire for a test over the wire")
	}
	ask = clientTestAsk{Network: network, On: on, SSID: ssid, Band: band}
	ask.EAP, _ = testUser(state, ap, network)
	if sec, _ := n["security"].(string); !enterprise[sec] {
		ask.EAP = nil
	}
	if on != "" {
		id := hierarchy.NodeID(on)
		target, ok := t.Node(id)
		if !ok || target.Kind != hierarchy.KindAP || !may(id) {
			return ask, badRequest("on: an AP, whose networks the test joins")
		}
		st, err := s.conds.LatestState(id)
		if err != nil {
			return ask, err
		}
		var rep struct {
			BSSIDs []string `json:"bssids"`
		}
		if st != nil {
			json.Unmarshal(st.Report, &rep)
		}
		if len(rep.BSSIDs) == 0 {
			return ask, badRequest("%s has not said which BSSIDs are its own yet, so the test cannot keep to it", target.Name)
		}
		ask.BSSIDs = rep.BSSIDs
		if len(ask.BSSIDs) > 32 {
			ask.BSSIDs = ask.BSSIDs[:32]
		}
	}
	if r, ok := t.Resolve(ap, hierarchy.Path("system.speed_server")); ok && speed {
		ask.Speed, _ = r.Value.(string)
	}
	return ask, nil
}

// clientTest is what a client test found (0121), as the AP says it: the
// network and where it joined it; how it went, and where and why it
// stopped if it did; how long it waited for the scan radio, took to be let
// on, and took in all, in ms; the networks the scan radio had heard for it;
// each step with the ms it came at; the link's signal and rates; the
// address it was given; its gateway's pings; a name looked up; how fast it
// moved bytes each way, in Mbit/s; and how many networks the scan radio
// heard before and after, with deaf where it hears none any more.
type clientTest struct {
	V         int           `json:"v"`
	SSID      string        `json:"ssid,omitempty"`
	Band      string        `json:"band,omitempty"`
	BSSID     string        `json:"bssid,omitempty"`
	Channel   int           `json:"channel,omitempty"`
	Client    string        `json:"client,omitempty"`
	Outcome   string        `json:"outcome"`
	Stage     string        `json:"stage"`
	Reason    string        `json:"reason"`
	Summary   string        `json:"summary,omitempty"`
	WaitedMS  int64         `json:"waited_ms,omitempty"`
	JoinedMS  int64         `json:"joined_ms,omitempty"`
	TookMS    int64         `json:"took_ms,omitempty"`
	Heard     []testHeard   `json:"heard,omitempty"`
	Steps     []testStep    `json:"steps"`
	Link      *testLink     `json:"link,omitempty"`
	DHCP      *testDHCP     `json:"dhcp,omitempty"`
	Gateway   *testPing     `json:"gateway,omitempty"`
	DNS       *testDNS      `json:"dns,omitempty"`
	Speed     *testSpeed    `json:"speed,omitempty"`
	ScanRadio *testScanning `json:"scan_radio,omitempty"`
	Wire      *testWire     `json:"wire,omitempty"`
	// Way is its way online, a point at a time (0130): worked out by the
	// manager as a test is read, and never kept, nor taken from an AP.
	Way []wayPoint `json:"way,omitempty"`
	// Seen is the same attempt as the AP it joined recorded it (0132): read
	// with the test, for one who may view that AP, and never kept either.
	Seen *testSeen `json:"seen,omitempty"`
}

// testWire is where a test over the wire was made (0128): the device, the
// uplink's port or a tunnel's own; the VLAN, and whether the port carries
// it tagged; or the tunnel's VNI.
type testWire struct {
	Device string `json:"device"`
	VLAN   int    `json:"vlan,omitempty"`
	Tagged *bool  `json:"tagged,omitempty"`
	VNI    *int   `json:"vni,omitempty"`
}

type testHeard struct {
	BSSID   string  `json:"bssid"`
	Band    string  `json:"band"`
	Channel int     `json:"channel"`
	Signal  float64 `json:"signal"`
}

type testStep struct {
	T     int64    `json:"t"`
	Stage string   `json:"stage"`
	What  string   `json:"what"`
	OK    *bool    `json:"ok,omitempty"`
	MS    *float64 `json:"ms,omitempty"`
	// K names the point of the way the step belongs to, where that is not
	// its stage (0130): over the wire, the offer of an address, and the
	// address given back, which are steps of DHCP both.
	K string `json:"k,omitempty"`
}

// The points a step may name. One this manager does not know is left out
// as the result is kept, and the step is read as its stage: a result is not
// refused for it, as an AP whose result is refused holds its later tests
// up.
var testPoints = map[string]bool{"": true, "offer": true, "release": true}

type testRate struct {
	Mbps float64 `json:"mbps"`
	How  string  `json:"how,omitempty"`
}

type testLink struct {
	Signal *float64  `json:"signal"`
	TX     *testRate `json:"tx"`
	RX     *testRate `json:"rx"`
}

type testDHCP struct {
	MS      int64    `json:"ms"`
	Address string   `json:"address"`
	Mask    int      `json:"mask"`
	Router  *string  `json:"router"`
	DNS     []string `json:"dns"`
	Server  *string  `json:"server"`
	Lease   int64    `json:"lease"`
}

type testPing struct {
	Address string   `json:"address"`
	Sent    int      `json:"sent"`
	Got     int      `json:"got"`
	Min     *float64 `json:"min"`
	Avg     *float64 `json:"avg"`
	Max     *float64 `json:"max"`
	ARP     *bool    `json:"arp,omitempty"`    // whether it answered ARP; absent where the test did not look
	MAC     string   `json:"mac,omitempty"`    // its MAC, where it did: it is there, though it answer no ping
	ARPMS   *float64 `json:"arp_ms,omitempty"` // how long its ARP answer took, where the test timed it (0128)
}

type testDNS struct {
	Server  string  `json:"server"`
	Name    string  `json:"name"`
	MS      *int64  `json:"ms,omitempty"`
	Rcode   *int    `json:"rcode,omitempty"`
	Answers int     `json:"answers,omitempty"`
	Address *string `json:"address,omitempty"`
}

type testSpeed struct {
	Server string   `json:"server"`
	Down   *float64 `json:"down,omitempty"`
	Up     *float64 `json:"up,omitempty"`
	Error  string   `json:"error,omitempty"`
}

type testScanning struct {
	Before   int  `json:"before"`
	After    int  `json:"after"`
	WasDeaf  bool `json:"was_deaf,omitempty"`
	AfterDFS bool `json:"after_dfs,omitempty"`
	Deaf     bool `json:"deaf,omitempty"`
	Unseen   bool `json:"unseen,omitempty"`
	Rebound  bool `json:"rebound,omitempty"`
	Again    *int `json:"again,omitempty"`
}

var testStages = map[string]bool{"find": true, "auth": true, "assoc": true, "signin": true, "key": true, "dhcp": true, "gateway": true, "dns": true, "speed": true}

func (t *clientTest) check() error {
	bad := func(format string, args ...any) error {
		return badRequest("detail: "+format, args...)
	}
	words := func(s string, most int) bool { return len(s) <= most && printable(s) }
	ip := func(s string) bool { return net.ParseIP(s) != nil }
	span := func(ms int64) bool { return ms >= 0 && ms <= 3600_000 }
	rate := func(r *testRate) bool { return r == nil || (r.Mbps >= 0 && r.Mbps <= 100_000 && words(r.How, 80)) }
	speed := func(v *float64) bool { return v == nil || (*v >= 0 && *v <= 100_000) }
	millis := func(v *float64) bool { return v == nil || (*v >= 0 && *v <= 60_000) }
	if (t.Outcome != "passed" && t.Outcome != "failed") || !(testStages[t.Stage] || (t.Stage == "done" && t.Outcome == "passed")) {
		return bad("an outcome, passed or failed, and the stage it stopped at")
	}
	if !words(t.Reason, 200) || !words(t.Summary, 300) || len(t.SSID) > 32 || !printable(t.SSID) ||
		(t.Band != "" && t.Band != wireBand && !bands[t.Band]) || (t.BSSID != "" && !macRE.MatchString(t.BSSID)) || (t.Client != "" && !macRE.MatchString(t.Client)) ||
		t.Channel < 0 || t.Channel > 233 || !span(t.WaitedMS) || !span(t.JoinedMS) || !span(t.TookMS) {
		return bad("its network, where it joined it, its reason and its times")
	}
	if len(t.Heard) > 8 || len(t.Steps) > 40 {
		return bad("at most 8 networks heard and 40 steps")
	}
	for _, h := range t.Heard {
		if !macRE.MatchString(h.BSSID) || !bands[h.Band] || h.Channel < 1 || h.Channel > 233 || h.Signal < -127 || h.Signal > 20 {
			return bad("a network heard is a BSSID, a band, a channel and a signal")
		}
	}
	for _, s := range t.Steps {
		if !span(s.T) || !testStages[s.Stage] || s.What == "" || !words(s.What, 160) || !millis(s.MS) {
			return bad("a step is when, in ms, a stage and what happened, printable and at most 160 characters")
		}
	}
	if l := t.Link; l != nil && ((l.Signal != nil && (*l.Signal < -127 || *l.Signal > 20)) || !rate(l.TX) || !rate(l.RX)) {
		return bad("a link is a signal in dBm and the rates each way")
	}
	if d := t.DHCP; d != nil {
		if !span(d.MS) || !ip(d.Address) || d.Mask < 0 || d.Mask > 32 || (d.Router != nil && !ip(*d.Router)) || (d.Server != nil && !ip(*d.Server)) || len(d.DNS) > 3 || d.Lease < 0 {
			return bad("an address given is the address, its prefix length, the router, the DNS servers and the server that gave it")
		}
		for _, x := range d.DNS {
			if !ip(x) {
				return bad("a DNS server is an address")
			}
		}
	}
	if g := t.Gateway; g != nil && (!ip(g.Address) || g.Sent < 0 || g.Sent > 100 || g.Got < 0 || g.Got > g.Sent || !millis(g.Min) || !millis(g.Avg) || !millis(g.Max) || !millis(g.ARPMS) ||
		(g.MAC != "" && !macRE.MatchString(g.MAC))) {
		return bad("a gateway is its address, the pings sent and answered, their times in ms, and its MAC where it answered ARP")
	}
	if d := t.DNS; d != nil && (!ip(d.Server) || d.Name == "" || !words(d.Name, 253) || (d.MS != nil && !span(*d.MS)) || (d.Rcode != nil && (*d.Rcode < 0 || *d.Rcode > 15)) ||
		d.Answers < 0 || d.Answers > 65535 || (d.Address != nil && !ip(*d.Address))) {
		return bad("a lookup is the server asked, the name, how long it took, its code and its answer")
	}
	if sp := t.Speed; sp != nil && (!words(sp.Server, 280) || !speed(sp.Down) || !speed(sp.Up) || !words(sp.Error, 120)) {
		return bad("a speed is the server and Mbit/s down and up, or why not")
	}
	if w := t.Wire; w != nil && (w.Device == "" || !words(w.Device, 15) || w.VLAN < 0 || w.VLAN > 4094 || (w.VNI != nil && (*w.VNI < 0 || *w.VNI > 16777215))) {
		return bad("a wire is its device, and the VLAN or the VNI it carries")
	}
	if r := t.ScanRadio; r != nil && (r.Before < 0 || r.Before > 100_000 || r.After < 0 || r.After > 100_000 ||
		(r.Again != nil && (*r.Again < 0 || *r.Again > 100_000))) {
		return bad("the scan radio's networks heard, before and after")
	}
	return nil
}

// clientTestDetail reads what an AP says a client test found, checks it,
// and returns it as it is kept: what this manager knows of it, and no more.
func clientTestDetail(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > 16<<10 {
		return nil, &apiError{http.StatusRequestEntityTooLarge, "detail: at most 16 kB"}
	}
	var t clientTest
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, badRequest("detail: %v", err)
	}
	if err := t.check(); err != nil {
		return nil, err
	}
	if t.Steps == nil {
		t.Steps = []testStep{}
	}
	for i := range t.Steps {
		if !testPoints[t.Steps[i].K] {
			t.Steps[i].K = ""
		}
	}
	t.Way, t.Seen = nil, nil
	out, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("client test: %w", err)
	}
	return out, nil
}
