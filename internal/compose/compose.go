// Package compose builds the config an AP receives (0008): its resolved
// fields, with each network's transports filtered by the tunnels set where
// the AP is (0055), and checked whole (0029). The API shows it, and the AP
// poll serves it.
package compose

import (
	"encoding/json"
	"fmt"
	"net"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/radio"
	"github.com/ChristopherGriffin/aeolus/internal/schema"
)

// Reveal opens a sealed secret for the field it belongs to. A nil Reveal
// leaves secrets sealed.
type Reveal func(path string, v any) (any, error)

// Result is an AP's composed config.
type Result struct {
	// Unassigned: the AP is in Landing Zone and gets no config (0032).
	Unassigned bool
	// Doc is the nested config the AP receives.
	Doc map[string]any
	// Problems are the rules the config breaks; an AP whose config has
	// problems is not sent it (0029).
	Problems []string
	// Template is the AP template the AP takes, if any, and what of it
	// something else replaces (0085).
	Template *hierarchy.TemplateUse
}

var slots = []string{"primary", "fallback"}

// AP composes one AP's config:
//   - A VXLAN transport whose tunnel is not set at the AP's location is left
//     out there (0055). If that removes the primary, the fallback takes its
//     place. One with no tunnel picked, or no VNI, is a problem.
//   - A VXLAN fallback to the primary's far end is a problem.
//   - A network left with no transport is a problem.
//   - The config carries only the tunnels its transports use, so an
//     unfinished tunnel nothing uses holds no AP back. A tunnel without an
//     MTU gets the default for its far end's address family (0056).
//   - A radio width the AP's radio cannot do, by what it reported when it
//     enrolled, is a problem (0008), as is a 5 GHz channel Aeolus sets that
//     cannot carry the width Aeolus sets (0045).
func AP(s *change.State, sch *schema.Schema, ap hierarchy.NodeID, reveal Reveal) (Result, error) {
	cfg, err := s.ResolveAP(ap)
	if err != nil {
		return Result{}, err
	}
	if cfg.Unassigned {
		return Result{Unassigned: true, Problems: []string{}}, nil
	}
	fields := map[string]any{}
	for p, r := range cfg.Location {
		fields[string(p)] = r.Value
	}
	var problems []string
	used := map[string]bool{}

	ids := cfg.NetworkIDs()
	for _, id := range ids {
		net := map[string]any{}
		for f, r := range cfg.Networks[id].Fields {
			net[f] = r.Value
		}
		kept := map[string]bool{}
		for _, slot := range slots {
			prefix := "transport." + slot + "."
			if _, has := net[prefix+"type"]; !has {
				continue
			}
			if net[prefix+"type"] != "vxlan" {
				kept[slot] = true
				continue
			}
			cid, _ := net[prefix+"concentrator"].(string)
			if _, vni := net[prefix+"vni"]; cid == "" || !vni {
				problems = append(problems, fmt.Sprintf("network.%s.transport.%s: a VXLAN transport needs a tunnel and a VNI", id, slot))
				continue
			}
			if _, set := fields["concentrators."+cid+".address"]; set {
				kept[slot] = true
				used[cid] = true
			} // else: the tunnel is not set here, so the transport is left out (0055)
		}
		hadTransport := false
		_, hadFallback := net["transport.fallback.type"]
		for _, slot := range slots {
			if _, has := net["transport."+slot+".type"]; has {
				hadTransport = true
			}
			if !kept[slot] {
				drop(net, "transport."+slot+".")
			}
		}
		if !kept["primary"] && kept["fallback"] {
			promote(net)
		}
		if hadTransport && !kept["primary"] && !kept["fallback"] {
			problems = append(problems, fmt.Sprintf("network.%s: no transport is usable at this AP", id))
		}
		// Switching needs both transports (0061). Where the network has a
		// fallback, but one of the two is not usable here, there is nothing
		// to switch between, and the settings are left out.
		if !kept["primary"] || !kept["fallback"] {
			if net["transport.switching"] == "automatic" && !hadFallback {
				problems = append(problems, switchingProblem(id))
			}
			for _, f := range switchingFields {
				delete(net, f)
			}
		}
		for f, v := range net {
			fields["network."+id+"."+f] = v
		}
	}

	// A tunnel port's tunnels are used as well (0058).
	for p, v := range fields {
		rest, ok := strings.CutPrefix(p, "ports.")
		if !ok || !strings.HasSuffix(p, ".tunnel") {
			continue
		}
		port, _, _ := strings.Cut(rest, ".")
		cid, _ := v.(string)
		if _, set := fields["concentrators."+cid+".address"]; set && fields["ports."+port+".mode"] == "tunnel" {
			used[cid] = true
		}
	}
	for p := range fields {
		if rest, ok := strings.CutPrefix(p, "concentrators."); ok {
			if cid, _, _ := strings.Cut(rest, "."); !used[cid] {
				delete(fields, p)
			}
		}
	}
	// What a tunnel leaves unset is its default, written out, so the AP and
	// the render check need not know the defaults (0056, 0059).
	for cid := range used {
		if _, set := fields["concentrators."+cid+".mtu"]; !set {
			addr, _ := fields["concentrators."+cid+".address"].(string)
			fields["concentrators."+cid+".mtu"] = float64(DefaultMTU(addr))
		}
		if _, set := fields["concentrators."+cid+".probe_interval"]; !set {
			fields["concentrators."+cid+".probe_interval"] = float64(DefaultProbeInterval)
		}
	}

	doc, err := schema.Assemble(fields, reveal)
	if err != nil {
		return Result{}, err
	}
	// The AP's name is its hostname (0076).
	if n, ok := s.Org.Locations.Node(ap); ok {
		if h := Hostname(n.Name); h != "" {
			doc["ap"] = map[string]any{"hostname": h}
		}
	}
	doc = jsonShape(doc)
	sort.Strings(problems)
	problems = append(problems, sch.Problems(doc)...)
	problems = append(problems, radioProblems(doc, s.Facts[ap])...)
	problems = append(problems, modesProblems(doc, s.Facts[ap])...)
	problems = append(problems, keyVLANProblems(doc, s.Facts[ap])...)
	problems = append(problems, sixGHzProblems(doc)...)
	problems = append(problems, bondingProblems(doc)...)
	problems = append(problems, dfsProblems(doc)...)
	problems = append(problems, channelsProblems(doc)...)
	problems = append(problems, apcProblems(doc)...)
	problems = append(problems, snmpProblems(doc)...)
	problems = append(problems, portProblems(doc)...)
	problems = append(problems, uplinkProblems(doc)...)
	problems = append(problems, enterpriseProblems(doc)...)
	problems = append(problems, tunnelProblems(doc)...)
	problems = append(problems, farEndProblems(doc)...)
	if problems == nil {
		problems = []string{}
	}
	return Result{Doc: doc, Problems: problems, Template: cfg.Template}, nil
}

// Hostname makes an AP's name a hostname (0076): letters, digits and
// hyphens, any other character turned into a hyphen, at most 63, with no
// hyphen at either end. A name a rename gave is one already; one from
// before may not be.
func Hostname(name string) string {
	b := []byte(name)
	for i, c := range b {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			b[i] = '-'
		}
	}
	h := strings.Trim(string(b), "-")
	if len(h) > 63 {
		h = strings.TrimRight(h[:63], "-")
	}
	return h
}

// radioProblems refuses a width a radio cannot do, by the modes the AP
// reported for it when it enrolled (0008). A band the AP did not report, or
// reported without modes, is not checked.
func radioProblems(doc map[string]any, facts json.RawMessage) []string {
	var f struct {
		Radios []struct {
			Band    string   `json:"band"`
			HTModes []string `json:"htmodes"`
		} `json:"radios"`
	}
	if len(facts) == 0 || json.Unmarshal(facts, &f) != nil {
		return nil
	}
	widths := map[string]map[int]bool{} // band -> widths some radio of it can do
	for _, r := range f.Radios {
		if widths[r.Band] == nil {
			widths[r.Band] = map[int]bool{}
		}
		for w := range radio.Can(r.HTModes) {
			widths[r.Band][w] = true
		}
	}
	radios, _ := doc["radio"].(map[string]any)
	var out []string
	for _, band := range sortedKeys(radios) {
		set, _ := radios[band].(map[string]any)
		w, ok := set["width"].(float64)
		can := widths[band]
		if !ok || len(can) == 0 || can[int(w)] {
			continue
		}
		var list []string
		for _, x := range []int{20, 40, 80, 160, 320} {
			if can[x] {
				list = append(list, fmt.Sprint(x))
			}
		}
		out = append(out, fmt.Sprintf("radio.%s.width: this AP's radio cannot use %d MHz; it can use %s MHz", band, int(w), strings.Join(list, ", ")))
	}
	return out
}

// modesProblems checks the 802.11 generations a band allows (0089): one
// unbroken run, not 802.11be alone, which OpenWrt cannot require, and a
// width the newest carries. By what the AP reported when it enrolled, the
// oldest must be one its radio serves, else no client could join; and
// OpenWrt before 25.12 can require 802.11n or 802.11ac, not 802.11ax.
func modesProblems(doc map[string]any, facts json.RawMessage) []string {
	var f struct {
		OpenWrt string `json:"openwrt"`
		Radios  []struct {
			Band     string   `json:"band"`
			HTModes  []string `json:"htmodes"`
			Reserved bool     `json:"reserved"`
		} `json:"radios"`
	}
	if len(facts) > 0 && json.Unmarshal(facts, &f) != nil {
		f.Radios = nil // facts we cannot read are facts we do not have
	}
	radios, _ := doc["radio"].(map[string]any)
	var out []string
	for _, band := range sortedKeys(radios) {
		set, _ := radios[band].(map[string]any)
		list, ok := set["modes"].([]any)
		if !ok {
			continue
		}
		var modes []string
		for _, m := range list {
			if s, ok := m.(string); ok {
				modes = append(modes, s)
			}
		}
		where := "radio." + band + ".modes"
		oldest, newest, err := radio.Span(band, modes)
		if err != nil {
			out = append(out, where+": "+err.Error())
			continue
		}
		if oldest == "be" {
			out = append(out, where+": OpenWrt cannot require 802.11be; allow 802.11ax too")
		}
		fam := radio.Family[newest]
		if w, ok := set["width"].(float64); ok && int(w) > radio.FamilyWidth[fam] {
			out = append(out, fmt.Sprintf("radio.%s.width: %d MHz is wider than 802.11%s goes (%d MHz), the newest radio.%s.modes allows", band, int(w), newest, radio.FamilyWidth[fam], band))
		}
		for _, r := range f.Radios {
			if r.Band != band || r.Reserved {
				continue
			}
			if serves := radio.Serves(band, r.HTModes); serves != nil && !slices.Contains(serves, oldest) {
				out = append(out, fmt.Sprintf("%s: this AP's %s radio serves up to 802.11%s, so no client could join with 802.11%s the least it allows", where, radio.BandName(band), serves[len(serves)-1], oldest))
				break
			}
		}
		if radio.Required(band, oldest) == "ax" && before2512(f.OpenWrt) {
			out = append(out, fmt.Sprintf("%s: OpenWrt %s cannot require 802.11ax (25.12 can); allow an older generation too", where, f.OpenWrt))
		}
	}
	return out
}

// before2512 says an OpenWrt release is older than 25.12. A snapshot, or a
// version it cannot read, is taken for a new one.
func before2512(version string) bool {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return major < 25 || major == 25 && minor < 12
}

// sixGHzProblems refuses a network offered on 6 GHz by name whose security
// is not allowed there: WPA2 alone, or open (0086). A network whose bands
// are left unset is simply not offered on 6 GHz.
func sixGHzProblems(doc map[string]any) []string {
	var out []string
	nets, _ := doc["network"].(map[string]any)
	for _, id := range sortedKeys(nets) {
		n, _ := nets[id].(map[string]any)
		bands, _ := n["bands"].([]any)
		security, _ := n["security"].(string)
		if !slices.Contains(bands, any("6g")) {
			continue
		}
		if _, ok := radio.SixGHzEncryption(security); !ok {
			out = append(out, fmt.Sprintf("network.%s: 6 GHz takes WPA3 or OWE only, not %s; take 6g out of its bands, or make it wpa3-sae, wpa2-wpa3 or owe", id, security))
		}
	}
	return out
}

// keyVLANProblems refuses the VLANs a network offers its per-user keys
// (0070) on a radio whose driver makes no AP/VLAN interfaces, by what the AP
// reported when it enrolled (0082): hostapd would fail every network on that
// radio. A radio reported without it, or one another service owns (0081), is
// not checked.
func keyVLANProblems(doc map[string]any, facts json.RawMessage) []string {
	var f struct {
		Radios []struct {
			Radio    string `json:"radio"`
			Band     string `json:"band"`
			APVLAN   *bool  `json:"ap_vlan"`
			Reserved bool   `json:"reserved"`
		} `json:"radios"`
	}
	if len(facts) == 0 || json.Unmarshal(facts, &f) != nil {
		return nil
	}
	nets, _ := doc["network"].(map[string]any)
	var out []string
	for _, id := range sortedKeys(nets) {
		n, _ := nets[id].(map[string]any)
		if n["enabled"] == false {
			continue
		}
		keys, _ := n["keys"].(map[string]any)
		if vlans, _ := keys["vlans"].([]any); len(vlans) == 0 {
			continue
		}
		bands := map[string]bool{}
		if list, ok := n["bands"].([]any); ok {
			for _, b := range list {
				if s, ok := b.(string); ok {
					bands[s] = true
				}
			}
		}
		security, _ := n["security"].(string)
		_, onSix := radio.SixGHzEncryption(security)
		var cannot []string
		for _, r := range f.Radios {
			if r.Reserved || r.APVLAN == nil || *r.APVLAN || (len(bands) > 0 && !bands[r.Band]) || (r.Band == "6g" && !onSix) {
				continue
			}
			cannot = append(cannot, r.Radio)
		}
		if len(cannot) > 0 {
			out = append(out, fmt.Sprintf("network.%s.keys.vlans: %s cannot put clients in VLANs of their own (the driver has no AP/VLAN interfaces); offer the network on other bands, or give its keys no VLANs", id, strings.Join(cannot, ", ")))
		}
	}
	return out
}

// bondingProblems refuses a channel and width, both set by Aeolus, that do
// not fit together (0045). A channel the AP picks itself is left to the
// render check (0044).
func bondingProblems(doc map[string]any) []string {
	radios, _ := doc["radio"].(map[string]any)
	var out []string
	for _, band := range sortedKeys(radios) {
		set, _ := radios[band].(map[string]any)
		ch, ok1 := set["channel"].(float64)
		w, ok2 := set["width"].(float64)
		if !ok1 || !ok2 {
			continue
		}
		if ok, why := radio.Fits(band, int(ch), int(w)); !ok {
			out = append(out, fmt.Sprintf("radio.%s.channel: %s", band, why))
		}
	}
	return out
}

// dfsProblems refuses, with DFS avoided (0071), what could only be on a DFS
// channel: a width no block outside DFS can carry, or a channel set to one.
// An explicit setting is never quietly ignored.
func dfsProblems(doc map[string]any) []string {
	radios, _ := doc["radio"].(map[string]any)
	var out []string
	for _, band := range sortedKeys(radios) {
		set, _ := radios[band].(map[string]any)
		if set["dfs"] != "avoid" {
			continue
		}
		w, _ := set["width"].(float64)
		if radio.Radar(band, 0, int(w)) {
			out = append(out, fmt.Sprintf("radio.%s.width: every %d MHz channel uses DFS channels, but radio.%s.dfs is avoid", band, int(w), band))
			continue
		}
		if ch, ok := set["channel"].(float64); ok && radio.Radar(band, int(ch), int(w)) {
			out = append(out, fmt.Sprintf("radio.%s.channel: channel %d uses DFS channels, but radio.%s.dfs is avoid", band, int(ch), band))
		}
	}
	return out
}

// channelsProblems refuses a set of channels an automatic channel may be
// (0075) that leaves the radio nowhere to go: on 5 and 6 GHz, no block of
// the width wholly in the set, or on 5 GHz none outside DFS while it is
// avoided, or on 6 GHz no preferred scanning channel in one while only
// those may be used (0087). A channel set by hand that is not a preferred
// scanning channel is refused then too.
func channelsProblems(doc map[string]any) []string {
	radios, _ := doc["radio"].(map[string]any)
	var out []string
	for _, band := range sortedKeys(radios) {
		set, _ := radios[band].(map[string]any)
		psc := band == "6g" && set["psc"] == true
		spread := band == "6g" && set["non_overlapping"] == true
		if ch, ok := set["channel"].(float64); psc && ok && !radio.PSC(int(ch)) {
			out = append(out, fmt.Sprintf("radio.6g.channel: %d is not a preferred scanning channel (5, 21, 37 and every 16th to 229), and radio.6g.psc is on", int(ch)))
		}
		list, ok := set["channels"].([]any)
		var chans []int
		switch {
		case ok && (band == "5g" || band == "6g"):
			for _, c := range list {
				if f, ok := c.(float64); ok {
					chans = append(chans, int(f))
				}
			}
		case psc || spread:
			chans = radio.Channels6
		default:
			continue
		}
		w, _ := set["width"].(float64)
		width := max(20, int(w))
		avoid := band == "5g" && set["dfs"] == "avoid"
		if len(radio.Usable(band, chans, width, avoid, psc, spread)) > 0 {
			continue
		}
		switch {
		case psc:
			out = append(out, fmt.Sprintf("radio.6g.channels: no preferred scanning channel is in a whole %d MHz block of the set, and radio.6g.psc is on", width))
		case avoid:
			out = append(out, fmt.Sprintf("radio.%s.channels: no %d MHz block outside DFS is wholly in the set, and radio.%s.dfs is avoid", band, width, band))
		default:
			out = append(out, fmt.Sprintf("radio.%s.channels: no %d MHz block is wholly in the set", band, width))
		}
	}
	return out
}

// apcProblems refuses power control without RRM (0077): it works from RRM's
// neighbours, and would have none.
func apcProblems(doc map[string]any) []string {
	apc, _ := doc["apc"].(map[string]any)
	rrm, _ := doc["rrm"].(map[string]any)
	if apc["enabled"] == true && rrm["enabled"] != true {
		return []string{"apc.enabled: power control needs rrm.enabled: it works from RRM's neighbours"}
	}
	return nil
}

// snmpProblems refuses SNMP turned on with no way to query it: no community
// and no v3 user, or a v3 user without both passphrases (0052).
func snmpProblems(doc map[string]any) []string {
	system, _ := doc["system"].(map[string]any)
	snmp, _ := system["snmp"].(map[string]any)
	if snmp["enabled"] != true {
		return nil
	}
	v3, _ := snmp["v3"].(map[string]any)
	var out []string
	if snmp["community"] == nil && v3["user"] == nil {
		out = append(out, "system.snmp: SNMP is on, but neither a community nor a v3 user is set")
	}
	if v3["user"] != nil && (v3["auth"] == nil || v3["privacy"] == nil) {
		out = append(out, "system.snmp.v3: a v3 user needs both an auth and a privacy passphrase")
	}
	return out
}

// enterpriseProblems refuses a WPA Enterprise network the AP could not
// render (0098): one with no RADIUS server or secret to sign in against;
// with per-user keys, which are passphrases; or with 802.11r, whose keys
// for 802.1X are not rendered yet. RADIUS's VLANs and disconnects (0111)
// are for WPA Enterprise alone: on another network, nothing would use them.
func enterpriseProblems(doc map[string]any) []string {
	nets, _ := doc["network"].(map[string]any)
	var out []string
	for _, id := range sortedKeys(nets) {
		n, _ := nets[id].(map[string]any)
		security, _ := n["security"].(string)
		where := "network." + id
		r, _ := n["radius"].(map[string]any)
		if !strings.HasSuffix(security, "-enterprise") {
			if n["enabled"] != false && (r["vlans"] != nil || r["das"] != nil) {
				out = append(out, where+": the RADIUS server's VLANs and disconnects are for WPA Enterprise; "+security+" signs no client in against it")
			}
			continue
		}
		if n["enabled"] == false {
			continue
		}
		if das, _ := r["das"].(map[string]any); das != nil && das["client"] == nil {
			out = append(out, where+": radius.das needs the address its Disconnect-Requests come from: radius.das.client")
		}
		// Required, with no VLAN offered, nothing would be rendered, and a
		// client the server gives no VLAN would be let in after all.
		if vlans, _ := r["vlans"].([]any); r["vlan_required"] == true && len(vlans) == 0 {
			out = append(out, where+": radius.vlan_required needs the VLANs the server may put clients in: radius.vlans")
		}
		if r["auth_server"] == nil || r["auth_secret"] == nil {
			out = append(out, where+": WPA Enterprise needs a RADIUS server to sign clients in against: radius.auth_server and radius.auth_secret")
		}
		if keys, _ := n["keys"].(map[string]any); len(keys) > 0 {
			out = append(out, where+": per-user keys are passphrases, for a WPA2 network; WPA Enterprise signs each client in as itself already")
		}
		if roaming, _ := n["roaming"].(map[string]any); roaming["ft"] == true {
			out = append(out, where+": 802.11r with WPA Enterprise is not rendered yet; turn roaming.ft off for it")
		}
	}
	return out
}

// uplinkProblems refuses a bond taken apart without spanning tree (0096):
// its ports, apart in one bridge to one network, would loop.
func uplinkProblems(doc map[string]any) []string {
	up, _ := doc["uplink"].(map[string]any)
	if up["bond"] == false && up["stp"] != true {
		return []string{"uplink.bond: taking the bond apart needs spanning tree on (uplink.stp), or its two ports to one network loop"}
	}
	return nil
}

// portProblems refuses port VLANs the AP could not render as asked
// (0053): an access port without its one untagged VLAN, or with tagged
// ones; a VLAN both untagged and tagged; VLANs with no mode to carry them;
// and LACP, which is not applied yet.
func portProblems(doc map[string]any) []string {
	ports, _ := doc["ports"].(map[string]any)
	var out []string
	for _, name := range sortedKeys(ports) {
		set, _ := ports[name].(map[string]any)
		where := "ports." + name
		untagged, _ := set["untagged"].(float64)
		tagged, _ := set["tagged"].([]any)
		switch set["mode"] {
		case "access":
			if untagged == 0 {
				out = append(out, where+": an access port needs its untagged VLAN")
			}
			if len(tagged) > 0 {
				out = append(out, where+": an access port carries no tagged VLANs")
			}
		case "trunk":
			for _, v := range tagged {
				if v, _ := v.(float64); untagged != 0 && v == untagged {
					out = append(out, fmt.Sprintf("%s: VLAN %d is both untagged and tagged", where, int(v)))
				}
			}
		case "tunnel":
			out = append(out, tunnelPortProblems(doc, where, set)...)
		case "lacp":
			out = append(out, where+": LACP is not applied yet")
		case nil:
			if set["untagged"] != nil || set["tagged"] != nil {
				out = append(out, where+": VLANs are set, but not the mode that carries them (access or trunk)")
			}
		}
	}
	return out
}

// tunnelUse is one VNI an AP's config carries over a tunnel: a network's
// transport, or one a tunnel port maps (0058).
type tunnelUse struct {
	where     string // the field it is set at, for messages
	network   string // the network that uses it, or "" for a port
	port      string // the port that uses it, or "" for a network
	fallback  bool   // a network's fallback, which waits until switching starts it
	switching bool   // a transport of a network with a fallback (0061)
	vni       int
	tunnel    string
}

// tunnelUses lists every VNI an AP's config carries over a tunnel.
func tunnelUses(doc map[string]any) []tunnelUse {
	var out []tunnelUse
	nets, _ := doc["network"].(map[string]any)
	for _, id := range sortedKeys(nets) {
		n, _ := nets[id].(map[string]any)
		if n["enabled"] == false {
			continue // not rendered
		}
		transport, _ := n["transport"].(map[string]any)
		for _, slot := range slots {
			t, _ := transport[slot].(map[string]any)
			vni, ok := t["vni"].(float64)
			if t["type"] != "vxlan" || !ok {
				continue
			}
			cid, _ := t["concentrator"].(string)
			_, switching := transport["fallback"].(map[string]any)
			out = append(out, tunnelUse{where: "network." + id + ".transport." + slot, network: id, fallback: slot == "fallback", switching: switching, vni: int(vni), tunnel: cid})
		}
	}
	ports, _ := doc["ports"].(map[string]any)
	for _, name := range sortedKeys(ports) {
		p, _ := ports[name].(map[string]any)
		if p["mode"] != "tunnel" {
			continue
		}
		maps, _ := p["vxlan"].(map[string]any)
		for _, vlan := range sortedKeys(maps) {
			m, _ := maps[vlan].(map[string]any)
			vni, ok := m["vni"].(float64)
			cid, _ := m["tunnel"].(string)
			if !ok || cid == "" {
				continue
			}
			out = append(out, tunnelUse{where: "ports." + name + ".vxlan." + vlan, port: name, vni: int(vni), tunnel: cid})
		}
	}
	return out
}

// tunnelProblems refuses tunnels an AP cannot run (0054, 0058). An AP runs
// one tunnel per VNI, so two networks on one VNI would become one, a
// network's primary and fallback cannot share one yet, one VNI cannot reach
// two tunnels, and a port cannot use a VNI that waits as a network's
// fallback. A tunnel needs its far end's IP address, as names do not resolve
// reliably on an AP.
func tunnelProblems(doc map[string]any) []string {
	concs, _ := doc["concentrators"].(map[string]any)
	var out []string
	slotOf := map[string]map[int]string{} // network -> VNI -> the slot that has it
	nets := map[int][]string{}            // VNI -> the networks that use it
	tunnels := map[int][]string{}         // VNI -> the tunnels it reaches
	fallbackOf := map[int]string{}        // VNI -> the network whose fallback it is
	switchingOf := map[int]string{}       // VNI -> the network with a fallback whose tunnel it is (0061)
	portsOn := map[int][]string{}         // VNI -> the ports that carry it
	for _, u := range tunnelUses(doc) {
		if !slices.Contains(tunnels[u.vni], u.tunnel) {
			tunnels[u.vni] = append(tunnels[u.vni], u.tunnel)
		}
		c, _ := concs[u.tunnel].(map[string]any)
		if addr, _ := c["address"].(string); addr != "" && net.ParseIP(strings.Trim(addr, "[]")) == nil {
			out = append(out, fmt.Sprintf("%s: tunnel %s's address %q is not an IP address", u.where, u.tunnel, addr))
		}
		if u.port != "" {
			if !slices.Contains(portsOn[u.vni], u.port) {
				portsOn[u.vni] = append(portsOn[u.vni], u.port)
			}
			continue
		}
		slot := "primary"
		if u.fallback {
			slot = "fallback"
			fallbackOf[u.vni] = u.network
		} else if u.switching {
			switchingOf[u.vni] = u.network
		}
		if slotOf[u.network] == nil {
			slotOf[u.network] = map[int]string{}
		}
		if other, ok := slotOf[u.network][u.vni]; ok {
			out = append(out, fmt.Sprintf("network.%s.transport: the %s and the %s both use VNI %d; an AP runs one tunnel per VNI", u.network, other, slot, u.vni))
			continue
		}
		slotOf[u.network][u.vni] = slot
		nets[u.vni] = append(nets[u.vni], u.network)
	}
	vnis := make([]int, 0, len(tunnels))
	for v := range tunnels {
		vnis = append(vnis, v)
	}
	sort.Ints(vnis)
	for _, v := range vnis {
		if len(nets[v]) > 1 {
			out = append(out, fmt.Sprintf("VNI %d: networks %s would share one tunnel at this AP, and so become one network", v, strings.Join(nets[v], " and ")))
		}
		if len(tunnels[v]) > 1 {
			out = append(out, fmt.Sprintf("VNI %d: it reaches tunnels %s at this AP, and an AP runs one tunnel per VNI", v, strings.Join(tunnels[v], " and ")))
		}
		if fallbackOf[v] != "" && len(portsOn[v]) > 0 {
			out = append(out, fmt.Sprintf("VNI %d: it is network %s's fallback here, which waits until switching starts it, so port %s cannot carry it", v, fallbackOf[v], strings.Join(portsOn[v], " and ")))
		} else if switchingOf[v] != "" && len(portsOn[v]) > 0 {
			out = append(out, fmt.Sprintf("VNI %d: network %s has a fallback, so the AP moves its tunnel into and out of the network's own bridge (0061), and port %s cannot carry it", v, switchingOf[v], strings.Join(portsOn[v], " and ")))
		}
	}
	// The AP's MAC on a segment ends in the VNI's last 16 bits above 9999,
	// so two such VNIs that agree there would share one (0060).
	byMAC := map[int]int{}
	for _, v := range vnis {
		if v <= 9999 {
			continue
		}
		if other, ok := byMAC[v&0xffff]; ok {
			out = append(out, fmt.Sprintf("VNI %d and VNI %d would give this AP one MAC on both segments: VNIs above 9999 must differ in their last 16 bits at an AP (0060)", other, v))
			continue
		}
		byMAC[v&0xffff] = v
	}
	return out
}

// farEndProblems refuses a network whose VXLAN fallback goes to the same far
// end as its VXLAN primary: by the same tunnel, or another to the same
// address. It would fail with the primary, so it could never stand in for
// it (Griff, 2026-10-06).
func farEndProblems(doc map[string]any) []string {
	concs, _ := doc["concentrators"].(map[string]any)
	nets, _ := doc["network"].(map[string]any)
	var out []string
	for _, id := range sortedKeys(nets) {
		n, _ := nets[id].(map[string]any)
		transport, _ := n["transport"].(map[string]any)
		p, _ := transport["primary"].(map[string]any)
		f, _ := transport["fallback"].(map[string]any)
		if p["type"] != "vxlan" || f["type"] != "vxlan" {
			continue
		}
		pc, _ := p["concentrator"].(string)
		fc, _ := f["concentrator"].(string)
		if pc == "" || fc == "" {
			continue
		}
		pa, fa := farEnd(concs, pc), farEnd(concs, fc)
		switch {
		case pc == fc:
			out = append(out, fmt.Sprintf("network.%s.transport.fallback: it uses tunnel %s, as the primary does, so it would fail with it: pick a tunnel to another far end, or a VLAN", id, pc))
		case pa != nil && fa != nil && pa.Equal(fa):
			out = append(out, fmt.Sprintf("network.%s.transport.fallback: tunnel %s goes to %s, as the primary's tunnel %s does, so it would fail with it: pick a tunnel to another far end, or a VLAN", id, fc, pa, pc))
		}
	}
	return out
}

// farEnd is a tunnel's far end, as an IP address, or nil.
func farEnd(concs map[string]any, id string) net.IP {
	c, _ := concs[id].(map[string]any)
	addr, _ := c["address"].(string)
	return net.ParseIP(strings.Trim(addr, "[]"))
}

// tunnelPortProblems refuses a tunnel port's VNIs the AP could not carry as
// asked (0058): none at all, a VLAN off the wire's range, a mapping without
// its tunnel or VNI, a tunnel not set where the AP is, and one VNI on two of
// the port's VLANs.
func tunnelPortProblems(doc map[string]any, where string, set map[string]any) []string {
	concs, _ := doc["concentrators"].(map[string]any)
	maps, _ := set["vxlan"].(map[string]any)
	if len(maps) == 0 {
		return []string{where + ": a tunnel port carries no VNIs yet"}
	}
	var out []string
	seen := map[int]string{}
	for _, vlan := range sortedKeys(maps) {
		at := where + ".vxlan." + vlan
		if n, err := strconv.Atoi(vlan); vlan != "untagged" && (err != nil || n < 1 || n > 4094) {
			out = append(out, fmt.Sprintf("%s: VLAN %s is not one from 1 to 4094", at, vlan))
		}
		m, _ := maps[vlan].(map[string]any)
		cid, _ := m["tunnel"].(string)
		vni, ok := m["vni"].(float64)
		if cid == "" || !ok {
			out = append(out, at+": needs both its tunnel and its VNI")
			continue
		}
		if concs[cid] == nil {
			out = append(out, fmt.Sprintf("%s: tunnel %s is not set where this AP is", at, cid))
		}
		if other, ok := seen[int(vni)]; ok {
			out = append(out, fmt.Sprintf("%s: VNI %d is on VLAN %s of this port already", at, int(vni), other))
		}
		seen[int(vni)] = vlan
	}
	return out
}

// Reported is what an AP last said it can run; a zero or nil field was not
// reported.
type Reported struct {
	UplinkMTU     int   // what its uplink carries now (0056)
	VXLANLoaded   *bool // whether netifd has loaded vxlan (0057)
	BSSTransition *bool // whether hostapd has 802.11v (0057)
}

// ReportedProblems holds what an AP's config asks for that the AP said it
// cannot run:
//   - a tunnel its uplink cannot carry now (0056): a tunnel's packets are its
//     MTU plus VXLAN's headers, 50 bytes over IPv4 and 70 over IPv6, and the
//     uplink must carry them whole, as VXLAN endpoints seldom put fragments
//     back together;
//   - a tunnel while netifd has not loaded vxlan, which it does only when
//     it starts (0057);
//   - band steering or BSS transition while hostapd lacks 802.11v, as the
//     line they need would make hostapd refuse its whole config (0057).
func ReportedProblems(doc map[string]any, r Reported) []string {
	nets, _ := doc["network"].(map[string]any)
	concs, _ := doc["concentrators"].(map[string]any)
	var out []string
	for _, id := range sortedKeys(nets) {
		n, _ := nets[id].(map[string]any)
		if n["enabled"] == false {
			continue
		}
		roaming, _ := n["roaming"].(map[string]any)
		if (n["band_steering"] == true || roaming["btm"] == true) && r.BSSTransition != nil && !*r.BSSTransition {
			out = append(out, fmt.Sprintf("network.%s: band steering and BSS transition need 802.11v, which this AP's hostapd lacks; install a full wpad, such as wpad-mbedtls (0057)", id))
		}
	}
	for _, u := range tunnelUses(doc) {
		if r.VXLANLoaded != nil && !*r.VXLANLoaded {
			out = append(out, fmt.Sprintf("%s: this AP's netifd has not loaded vxlan; restart its network (/etc/init.d/network restart), and install vxlan first if it is missing (0057)", u.where))
		}
		if r.UplinkMTU == 0 {
			continue
		}
		c, _ := concs[u.tunnel].(map[string]any)
		mtu, _ := c["mtu"].(float64)
		addr, _ := c["address"].(string)
		need := int(mtu) + 50
		if strings.Contains(addr, ":") {
			need = int(mtu) + 70
		}
		if need > r.UplinkMTU {
			out = append(out, fmt.Sprintf("%s: tunnel %s's MTU of %d needs %d on the AP's uplink, which carries %d now; raise the uplink's MTU, or leave the tunnel's MTU at its default (0056)",
				u.where, u.tunnel, int(mtu), need, r.UplinkMTU))
		}
	}
	return out
}

// DefaultProbeInterval is the seconds between a tunnel's probes when none
// is set (0059): well inside the five minutes a concentrator keeps a quiet
// tunnel end it learned.
const DefaultProbeInterval = 30

// DefaultMTU is a tunnel's MTU when none is set (0056): what a 1500-byte
// path carries once VXLAN's headers are added, 50 bytes over IPv4 and 70
// over IPv6.
func DefaultMTU(address string) int {
	if strings.Contains(address, ":") {
		return 1430
	}
	return 1450
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Node checks what is resolved at one node as a partial config, by the
// schema's rules. The page guard uses it (0029).
func Node(s *change.State, sch *schema.Schema, tree change.TreeName, t *hierarchy.Tree, node hierarchy.NodeID, reveal Reveal) []string {
	values := map[string]any{}
	for p, r := range t.ResolveAll(node) {
		if p != hierarchy.ServicesPath {
			values[string(p)] = r.Value
		}
	}
	var problems []string
	doc, err := schema.Assemble(values, reveal)
	if err != nil {
		return append(problems, err.Error())
	}
	problems = append(problems, sch.Problems(jsonShape(doc))...)
	problems = append(problems, switchingProblems(jsonShape(doc))...)
	if problems == nil {
		problems = []string{}
	}
	return problems
}

// jsonShape passes a document through JSON so it holds only JSON types, as the
// AP will receive it.
func jsonShape(doc map[string]any) map[string]any {
	raw, err := json.Marshal(doc)
	if err != nil {
		return doc
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return doc
	}
	return out
}

func drop(net map[string]any, prefix string) {
	for f := range net {
		if strings.HasPrefix(f, prefix) {
			delete(net, f)
		}
	}
}

// switchingFields are a network's settings for switching between its
// transports (0022, 0061).
var switchingFields = []string{"transport.switching", "transport.ha", "transport.failback", "transport.holddown"}

func switchingProblem(id string) string {
	return fmt.Sprintf("network.%s.transport.switching: automatic switching needs a fallback (0061)", id)
}

// switchingProblems refuses automatic switching for a network without a
// fallback (0061), as a folder sets them.
func switchingProblems(doc map[string]any) []string {
	nets, _ := doc["network"].(map[string]any)
	var out []string
	for _, id := range sortedKeys(nets) {
		n, _ := nets[id].(map[string]any)
		tr, _ := n["transport"].(map[string]any)
		if fb, _ := tr["fallback"].(map[string]any); tr["switching"] == "automatic" && fb["type"] == nil {
			out = append(out, switchingProblem(id))
		}
	}
	return out
}

// promote moves the fallback transport into the primary's place.
func promote(net map[string]any) {
	for f, v := range net {
		if rest, ok := strings.CutPrefix(f, "transport.fallback."); ok {
			net["transport.primary."+rest] = v
			delete(net, f)
		}
	}
}
