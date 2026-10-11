package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"

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
type clientTestAsk struct {
	Network string   `json:"network"`
	On      string   `json:"on,omitempty"`
	SSID    string   `json:"ssid"`
	BSSIDs  []string `json:"bssids,omitempty"`
	Band    string   `json:"band,omitempty"`
	Speed   string   `json:"speed,omitempty"`
}

// testable are the ways a network is secured that the test can join: with
// the network's own passphrase, or none. One that signs clients in by
// 802.1X needs a user the test does not have.
var testable = map[string]bool{"open": true, "owe": true, "wpa2-psk": true, "wpa3-sae": true, "wpa2-wpa3": true}

// scanRadio says whether an AP said, when it enrolled, that it has a radio
// another service owns (0081): the one a client test borrows.
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

// clientTestFor makes what an AP is told to test from what a person asked:
// one of the AP's own networks, by its ID; on, the AP whose networks to
// join, the testing AP itself among them, or none for the strongest other
// AP; and a band, or none.
func (s *Server) clientTestFor(c call, ap hierarchy.NodeID, network, on, band string) (clientTestAsk, error) {
	var ask clientTestAsk
	t := c.state.Org.Locations
	if !scanRadio(c.state, ap) {
		return ask, badRequest("a client test needs an AP with a scan radio, and this one has not said it has one")
	}
	res, err := s.compose(c.state, ap, s.reveal)
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
	sec, _ := n["security"].(string)
	if rad, _ := n["radius"].(map[string]any); rad["passphrases"] == true {
		return ask, badRequest("network %s gives each device a passphrase of its own from RADIUS (0115), and the test has none", network)
	}
	if !testable[sec] {
		return ask, badRequest("network %s signs clients in by 802.1X, which the test cannot do yet", network)
	}
	if band != "" && !bands[band] {
		return ask, badRequest("band: 2g, 5g or 6g, or none for any")
	}
	ask = clientTestAsk{Network: network, On: on, SSID: ssid, Band: band}
	if on != "" {
		id := hierarchy.NodeID(on)
		target, ok := t.Node(id)
		if !ok || target.Kind != hierarchy.KindAP || roleOn(c, change.Locations, t, id) < access.Viewer {
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
	if r, ok := t.Resolve(ap, hierarchy.Path("system.speed_server")); ok {
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
}

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
	Before int  `json:"before"`
	After  int  `json:"after"`
	Deaf   bool `json:"deaf,omitempty"`
}

var testStages = map[string]bool{"find": true, "auth": true, "assoc": true, "key": true, "dhcp": true, "gateway": true, "dns": true, "speed": true}

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
		(t.Band != "" && !bands[t.Band]) || (t.BSSID != "" && !macRE.MatchString(t.BSSID)) || (t.Client != "" && !macRE.MatchString(t.Client)) ||
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
	if g := t.Gateway; g != nil && (!ip(g.Address) || g.Sent < 0 || g.Sent > 100 || g.Got < 0 || g.Got > g.Sent || !millis(g.Min) || !millis(g.Avg) || !millis(g.Max)) {
		return bad("a gateway is its address, the pings sent and answered, and their times in ms")
	}
	if d := t.DNS; d != nil && (!ip(d.Server) || d.Name == "" || !words(d.Name, 253) || (d.MS != nil && !span(*d.MS)) || (d.Rcode != nil && (*d.Rcode < 0 || *d.Rcode > 15)) ||
		d.Answers < 0 || d.Answers > 65535 || (d.Address != nil && !ip(*d.Address))) {
		return bad("a lookup is the server asked, the name, how long it took, its code and its answer")
	}
	if sp := t.Speed; sp != nil && (!words(sp.Server, 280) || !speed(sp.Down) || !speed(sp.Up) || !words(sp.Error, 120)) {
		return bad("a speed is the server and Mbit/s down and up, or why not")
	}
	if r := t.ScanRadio; r != nil && (r.Before < 0 || r.Before > 100_000 || r.After < 0 || r.After > 100_000) {
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
	out, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("client test: %w", err)
	}
	return out, nil
}
