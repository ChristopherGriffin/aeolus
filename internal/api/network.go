package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// How long the network takes to answer (0141). Every client test (0121,
// 0128) times what a network's own services took: an address from its DHCP
// server, its gateway, a name looked up, a sign-in; and every state report
// says how long a network's RADIUS server took to answer the AP (0114). The
// manager counts each into its hour as it comes, so what a VLAN's services
// typically take, and how that moved over a day or a year, can be said.

// The steps kept, in the order they are shown: how long an address took,
// the gateway, a name, the RADIUS server's answer, a whole sign-in, and the
// internet.
var networkSteps = []string{"dhcp", "gateway", "dns", "aaa", "signin", "internet"}

// mgmtPrefix begins what an AP's own way out is counted under (0142): no
// network of Aeolus's, but the AP's management, on the VLAN that follows
// it. A network's ID can hold no colon, so it is never one.
const mgmtPrefix = "mgmt:"

// The points of a test's way (0130) that are a network's own answers.
var testTimed = map[string]bool{"dhcp": true, "gateway": true, "dns": true, "signin": true}

// testSteps is what a finished client test timed of its network: each of
// its way's points that is a network's own answer, with how long it took,
// or with no time where the test stopped there.
func testSteps(a conditions.Action) []conditions.StepTime {
	if a.Kind != "client-test" || len(a.Detail) == 0 {
		return nil
	}
	var ask clientTestAsk
	var d clientTest
	if json.Unmarshal(a.Params, &ask) != nil || ask.Network == "" || json.Unmarshal(a.Detail, &d) != nil {
		return nil
	}
	way, _ := testWay(&d)
	var out []conditions.StepTime
	for _, p := range way {
		if !testTimed[p.K] {
			continue
		}
		switch {
		case p.State == "ok" && p.MS != nil:
			ms := *p.MS
			out = append(out, conditions.StepTime{Network: ask.Network, Step: p.K, MS: &ms})
		case p.State == "bad":
			out = append(out, conditions.StepTime{Network: ask.Network, Step: p.K})
		}
	}
	return out
}

// keepTestSteps counts what a finished client test timed into its hour. A
// test is kept whether or not this is, so a failure here is only said.
func (s *Server) keepTestSteps(a conditions.Action) {
	steps := testSteps(a)
	if len(steps) == 0 {
		return
	}
	at := time.Now()
	if a.DoneAt != nil {
		at = *a.DoneAt
	}
	if err := s.conds.AddSteps(hierarchy.NodeID(a.AP), at, steps); err != nil {
		slog.Error("keeping what a client test timed", "ap", a.AP, "err", err)
	}
}

// radiusSteps is how long each network's RADIUS server took to answer an
// AP, as a state report says it (0114): its last round trip where it
// answers, and no time where it has gone silent. One not yet asked, or not
// known to be asked, says nothing; nor does an answer from before the
// report before, which that report counted.
func radiusSteps(rs []radiusState) []conditions.StepTime {
	var out []conditions.StepTime
	for _, r := range rs {
		switch {
		case r.Verdict == "up" && r.RTT != nil && r.AnsweredAgo != nil && time.Duration(*r.AnsweredAgo)*time.Second <= stateEvery:
			ms := *r.RTT
			out = append(out, conditions.StepTime{Network: r.Network, Step: "aaa", MS: &ms})
		case r.Verdict == "silent":
			out = append(out, conditions.StepTime{Network: r.Network, Step: "aaa"})
		}
	}
	return out
}

// testsCounted marks that the client tests kept from before these times
// were kept have been counted in.
const testsCounted = "client-tests-counted"

// pathSteps is what an AP's own way out timed (0142), as a state report
// says it: its gateway, each of its name servers, and the internet where
// it has a target, counted under its management VLAN, so that the VLAN the
// APs themselves live on is said beside the VLANs their clients do. An
// answer counts where it came since the report before; silence counts as
// no answer; what is not yet known says nothing.
func pathSteps(p *pathState, port *uplinkPort) []conditions.StepTime {
	if p == nil {
		return nil
	}
	network := mgmtPrefix
	if port != nil && port.ManagementVLAN > 0 {
		network += strconv.Itoa(port.ManagementVLAN)
	}
	var out []conditions.StepTime
	add := func(step, state string, ms *float64, ago *int64) {
		switch {
		case state == "up" && ms != nil && ago != nil && time.Duration(*ago)*time.Second <= stateEvery:
			v := *ms
			out = append(out, conditions.StepTime{Network: network, Step: step, MS: &v})
		case state == "silent":
			out = append(out, conditions.StepTime{Network: network, Step: step})
		}
	}
	if g := p.Gateway; g != nil {
		add("gateway", g.State, g.MS, g.Ago)
	}
	for _, d := range p.DNS {
		add("dns", d.State, d.MS, d.Ago)
	}
	if i := p.Internet; i != nil {
		add("internet", i.State, i.MS, i.Ago)
	}
	return out
}

// countKeptTests counts the client tests still kept into the hours they
// were made in, once ever: so that the history begins with the tests kept,
// and not with this manager. Those done since this manager started are
// left out: each was counted as its result came. It is done whole or not
// at all, and marked so in the same transaction (AddStepsOnce): a manager
// stopped part way does it again from the start, and one that has done it
// never does it twice, whatever was counted in between.
func (s *Server) countKeptTests() {
	if done, err := s.conds.Marked(testsCounted); err != nil || done {
		return
	}
	tests, err := s.conds.ClientTests(s.started.Add(-assureKeep))
	if err != nil {
		slog.Error("counting the client tests kept", "err", err)
		return
	}
	var all []conditions.StepsAt
	for _, a := range tests {
		if a.DoneAt == nil || !a.DoneAt.Before(s.started) {
			continue
		}
		if steps := testSteps(a); len(steps) > 0 {
			all = append(all, conditions.StepsAt{AP: hierarchy.NodeID(a.AP), At: *a.DoneAt, Steps: steps})
		}
	}
	done, err := s.conds.AddStepsOnce(testsCounted, all)
	if err != nil {
		slog.Error("counting the client tests kept", "err", err)
		return
	}
	if done && len(all) > 0 {
		slog.Info("counted the client tests kept into the network's hours", "tests", len(all))
	}
}

// networkStep is one step over a time: how many times it was timed, how
// long it took on average and at its slowest, in ms, and how many times no
// answer came.
type networkStep struct {
	N      int      `json:"n"`
	MS     *float64 `json:"ms"`
	Most   *float64 `json:"most"`
	Failed int      `json:"failed"`
}

// networkName is what is on a segment: a network of Aeolus's, by its ID and
// the SSID it is set to somewhere; the APs' own management (kind
// management), whose way out is timed there (0142); or a network of an
// AP's own, not Aeolus's (kind own, 0137), by its SSID, which nothing
// times.
type networkName struct {
	Network string `json:"network,omitempty"`
	SSID    string `json:"ssid,omitempty"`
	Kind    string `json:"kind,omitempty"`
}

// networkSegment is the networks that share a segment, a VLAN or a tunnel,
// and what was timed of them together: its DHCP server, gateway and names
// are the segment's own, whichever network a test joined it by.
type networkSegment struct {
	ID       string                 `json:"id"`
	VLAN     *int                   `json:"vlan,omitempty"`
	VNI      *int                   `json:"vni,omitempty"`
	Networks []networkName          `json:"networks"`
	Steps    map[string]networkStep `json:"steps"`
}

// networkPoint is one stretch of the series: each step's average in it, in
// ms, where it was timed.
type networkPoint struct {
	At    time.Time           `json:"at"`
	Steps map[string]*float64 `json:"steps"`
}

// stepSum adds up what was timed of a step.
type stepSum struct {
	n      int
	ms     float64
	most   float64
	failed int
}

func (t *stepSum) add(h conditions.StepHour) {
	t.n += h.N
	t.ms += h.MS
	t.most = max(t.most, h.Most)
	t.failed += h.Failed
}

func (t *stepSum) step() networkStep {
	out := networkStep{N: t.n, Failed: t.failed}
	if t.n > 0 {
		ms, most := t.ms/float64(t.n), t.most
		out.MS, out.Most = &ms, &most
	}
	return out
}

// whole reads a value that is a whole number, as a tree holds one.
func whole(v hierarchy.Value) (int, bool) {
	switch v := v.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), v == float64(int(v))
	case json.Number:
		n, err := v.Int64()
		return int(n), err == nil
	}
	return 0, false
}

// segmentsOf says, of every network set anywhere in Services, the segment
// its primary transport puts it on, a VLAN or a tunnel, and its SSID. A
// network set differently in two folders is read as the folder first by ID
// has it: this names what was timed, and decides nothing.
func segmentsOf(state *change.State) (seg map[string]string, ssid map[string]string) {
	type said struct {
		node hierarchy.NodeID
		v    string
	}
	vlans, vnis, ssids := map[string]said{}, map[string]said{}, map[string]said{}
	keep := func(m map[string]said, id string, node hierarchy.NodeID, v string) {
		if old, ok := m[id]; !ok || node < old.node {
			m[id] = said{node, v}
		}
	}
	state.Org.Services.EachSet(func(node hierarchy.NodeID, p hierarchy.Path, v hierarchy.Value) {
		rest, ok := strings.CutPrefix(string(p), "network.")
		if !ok {
			return
		}
		id, field, ok := strings.Cut(rest, ".")
		if !ok {
			return
		}
		switch field {
		case "ssid":
			if s, ok := v.(string); ok {
				keep(ssids, id, node, s)
			}
		case "transport.primary.vlan":
			if n, ok := whole(v); ok {
				keep(vlans, id, node, "vlan:"+strconv.Itoa(n))
			}
		case "transport.primary.vni":
			if n, ok := whole(v); ok {
				keep(vnis, id, node, "vni:"+strconv.Itoa(n))
			}
		}
	})
	// A tunnel's VNI names a network's segment only where no VLAN does.
	seg, ssid = map[string]string{}, map[string]string{}
	for id, s := range vnis {
		seg[id] = s.v
	}
	for id, s := range vlans {
		seg[id] = s.v
	}
	for id, s := range ssids {
		ssid[id] = s.v
	}
	return seg, ssid
}

// network is how long the networks' own services took to answer the APs
// the caller may view (0141), below one Locations node where ?under= names
// it, over the last ?hours= (24 unless set, at most 8760): GET /v1/network.
// segments are the VLANs and tunnels the networks are on, each with its
// networks and, for each step, how many times it was timed, its average
// and its slowest in ms, and how many times no answer came; all is the
// same of them together. series is each step's average over the time, in
// at most 48 buckets of whole hours, of every segment, or of the one
// ?segment= names by its ID.
func (s *Server) network(w http.ResponseWriter, r *http.Request, c call) error {
	q := r.URL.Query()
	hours := 24
	if v := q.Get("hours"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > trendHours {
			return badRequest("hours: from 1 to %d", trendHours)
		}
		hours = n
	}
	one := q.Get("segment")
	mine := map[hierarchy.NodeID]bool{}
	for _, id := range mayView(c, hierarchy.NodeID(q.Get("under"))) {
		mine[id] = true
	}
	segOf, ssidOf := segmentsOf(c.state)
	segment := func(network string) string {
		// The APs' own management is on the VLAN it was counted under.
		if vlan, ok := strings.CutPrefix(network, mgmtPrefix); ok && vlan != "" {
			return "vlan:" + vlan
		}
		if s, ok := segOf[network]; ok {
			return s
		}
		return "net:" + network
	}

	span := time.Duration(hours) * time.Hour
	bucket := max(time.Hour, ((span+47)/48 + time.Hour - 1).Truncate(time.Hour))
	end := time.Now().UTC().Truncate(bucket).Add(bucket)
	count := int((span + bucket - 1) / bucket)
	start := end.Add(-time.Duration(count) * bucket)

	all := map[string]*stepSum{}
	bySeg := map[string]map[string]*stepSum{}
	nets := map[string]map[string]bool{}
	series := make([]map[string]*stepSum, count)
	sum := func(m map[string]*stepSum, step string) *stepSum {
		if m[step] == nil {
			m[step] = &stepSum{}
		}
		return m[step]
	}
	err := s.conds.StepsSince(start, func(h conditions.StepHour) error {
		i := int(h.At.Sub(start) / bucket)
		if !mine[h.AP] || i < 0 || i >= count || !slices.Contains(networkSteps, h.Step) {
			return nil
		}
		id := segment(h.Network)
		if bySeg[id] == nil {
			bySeg[id], nets[id] = map[string]*stepSum{}, map[string]bool{}
		}
		nets[id][h.Network] = true
		sum(all, h.Step).add(h)
		sum(bySeg[id], h.Step).add(h)
		if one == "" || one == id {
			if series[i] == nil {
				series[i] = map[string]*stepSum{}
			}
			sum(series[i], h.Step).add(h)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// The networks the APs have of their own, not Aeolus's (0137), by the
	// VLAN each is on: said with the segment, so that every VLAN that
	// carries Wi-Fi here is listed, timed or not. Nothing times them.
	own := map[string][]string{}
	for ap := range mine {
		st, err := s.conds.LatestState(ap)
		if err != nil {
			return err
		}
		if st == nil {
			continue
		}
		var rep struct {
			Others []otherNetwork `json:"others"`
		}
		if json.Unmarshal(st.Report, &rep) != nil {
			continue
		}
		for _, o := range rep.Others {
			id := "vlan:" + strconv.Itoa(o.VLAN)
			if o.VLAN <= 0 || o.SSID == "" || o.Off || slices.Contains(own[id], o.SSID) {
				continue
			}
			own[id] = append(own[id], o.SSID)
			if bySeg[id] == nil {
				bySeg[id], nets[id] = map[string]*stepSum{}, map[string]bool{}
			}
		}
	}

	steps := func(m map[string]*stepSum) map[string]networkStep {
		out := map[string]networkStep{}
		for k, t := range m {
			out[k] = t.step()
		}
		return out
	}
	segments := []networkSegment{}
	for id, m := range bySeg {
		sg := networkSegment{ID: id, Networks: []networkName{}, Steps: steps(m)}
		if kind, n, ok := strings.Cut(id, ":"); ok {
			if v, err := strconv.Atoi(n); err == nil && kind == "vlan" {
				sg.VLAN = &v
			} else if err == nil && kind == "vni" {
				sg.VNI = &v
			}
		}
		for network := range nets[id] {
			if strings.HasPrefix(network, mgmtPrefix) {
				sg.Networks = append(sg.Networks, networkName{Network: network, Kind: "management"})
				continue
			}
			sg.Networks = append(sg.Networks, networkName{Network: network, SSID: ssidOf[network]})
		}
		for _, ssid := range own[id] {
			sg.Networks = append(sg.Networks, networkName{SSID: ssid, Kind: "own"})
		}
		// Aeolus's networks first, then the APs' management, then their own.
		order := map[string]int{"": 0, "management": 1, "own": 2}
		slices.SortFunc(sg.Networks, func(a, b networkName) int {
			if order[a.Kind] != order[b.Kind] {
				return order[a.Kind] - order[b.Kind]
			}
			return strings.Compare(a.Network+a.SSID, b.Network+b.SSID)
		})
		segments = append(segments, sg)
	}
	// VLANs first, by number; then tunnels; then what is on neither.
	rank := func(sg networkSegment) (int, int) {
		switch {
		case sg.VLAN != nil:
			return 0, *sg.VLAN
		case sg.VNI != nil:
			return 1, *sg.VNI
		}
		return 2, 0
	}
	slices.SortFunc(segments, func(a, b networkSegment) int {
		ak, an := rank(a)
		bk, bn := rank(b)
		if ak != bk {
			return ak - bk
		}
		if an != bn {
			return an - bn
		}
		return strings.Compare(a.ID, b.ID)
	})
	points := make([]networkPoint, count)
	for i := range points {
		points[i] = networkPoint{At: start.Add(time.Duration(i) * bucket), Steps: map[string]*float64{}}
		for k, t := range series[i] {
			points[i].Steps[k] = t.step().MS
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"hours": hours, "from": start, "to": end, "bucket": int(bucket / time.Second),
		"steps": networkSteps, "all": steps(all), "segments": segments, "segment": one, "series": points,
	})
	return nil
}
