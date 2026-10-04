// Package rendercheck decides whether the UCI an AP rendered carries its
// intent (0008, 0039): the manager's half of the render check. What it
// checks is the rendering contract the agent is written against.
//
// Aeolus names what it renders, so the check knows what to look at: each
// network's wifi-iface is aeolus_<network>_<radio>. Sections without that
// prefix are not judged, except radios, which carry the Location settings.
package rendercheck

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ChristopherGriffin/aeolus/internal/radio"
	"github.com/ChristopherGriffin/aeolus/internal/uci"
)

// Coverage says how the render check covers every field of the v1 schema:
// "" means it is checked; anything else says why not yet. "*" stands for any
// one name. A test holds it to the schema, so no field is left out unnoticed.
var Coverage = map[string]string{
	"radio.*.enabled": "",
	"radio.*.channel": "",
	"radio.*.width":   "",
	"radio.*.power":   "",

	"system.country":               "",
	"system.tz":                    "",
	"system.ntp":                   "",
	"system.syslog":                "",
	"system.poll":                  "",
	"system.ssh_keys":              "kept in dropbear's authorized_keys, not UCI; checked with the agent in M5",
	"system.management.vlan":       layout,
	"system.management.addressing": layout,
	"system.management.address":    layout,
	"system.management.gateway":    layout,
	"system.management.dns":        layout,

	"ports.*.enabled":  "",
	"ports.*.uplink":   "the agent's own setting (0040), not the intent's",
	"ports.*.mode":     "",
	"ports.*.untagged": "",
	"ports.*.tagged":   "",
	"ports.*.bond":     "LACP is not applied yet; the config check holds it (0053)",

	"ports.*.vxlan.*.tunnel": "",
	"ports.*.vxlan.*.vni":    "",
	"ports.*.vxlan.*.probe":  "",

	"network.*.enabled":                  "",
	"network.*.ssid":                     "",
	"network.*.hidden":                   "",
	"network.*.bands":                    "",
	"network.*.security":                 "",
	"network.*.passphrase":               "",
	"network.*.isolation":                "",
	"network.*.roaming.ft":               "",
	"network.*.roaming.rrm":              "",
	"network.*.roaming.btm":              "",
	"network.*.multicast_to_unicast":     "",
	"network.*.band_steering":            "",
	"system.snmp.enabled":                "",
	"system.snmp.community":              "",
	"system.snmp.v3.user":                "",
	"system.snmp.v3.auth":                "",
	"system.snmp.v3.privacy":             "",
	"system.snmp.location":               "",
	"system.snmp.contact":                "",
	"network.*.rate_limit.down_kbps":     "no standard UCI form; the agent's own setting, checked with it in M5",
	"network.*.rate_limit.up_kbps":       "no standard UCI form; the agent's own setting, checked with it in M5",
	"network.*.transport.*.type":         "",
	"network.*.transport.*.vlan":         "",
	"network.*.transport.*.concentrator": "",
	"network.*.transport.*.vni":          "",
	"network.*.transport.*.probe":        "", // a VLAN transport's is not used until switching (0059)
	"network.*.transport.ha":             "the agent's own behavior (0022); checked with the agent in M5",
	"network.*.transport.failback":       "the agent's own behavior (0022); checked with the agent in M5",
	"network.*.transport.holddown":       "the agent's own behavior (0022); checked with the agent in M5",

	"concentrators.*.address":        "",
	"concentrators.*.port":           "",
	"concentrators.*.mtu":            "",
	"concentrators.*.probe_interval": "",
	"concentrators.*.underlay_vlan":  "",
}

const layout = "depends on the device's port layout; checked with the agent in M5"

// encryption is the UCI encryption for each security mode. A cipher suffix
// ("psk2+ccmp") is accepted.
var encryption = map[string]string{
	"open":      "none",
	"owe":       "owe",
	"wpa2-psk":  "psk2",
	"wpa3-sae":  "sae",
	"wpa2-wpa3": "sae-mixed",
}

var needsKey = map[string]bool{"wpa2-psk": true, "wpa3-sae": true, "wpa2-wpa3": true}

// IfaceName is the wifi-iface Aeolus renders for a network on a radio.
func IfaceName(network, dev string) string {
	return "aeolus_" + strings.ReplaceAll(network, "-", "_") + "_" + dev
}

// InterfaceName is a network's interface, which its wifi-ifaces join.
func InterfaceName(network string) string {
	return "aeolus_" + strings.ReplaceAll(network, "-", "_")
}

// TunnelName is the interface of the VXLAN tunnel for a VNI, and so its
// Linux device: at most 15 characters for every VNI (0054).
func TunnelName(vni string) string {
	return "aeolus_" + vni
}

// The clamp's firewall include, and the file the agent makes for it (0054).
const (
	clampInclude = "aeolus_clamp"
	clampPath    = "/etc/aeolus/clamp.nft"
)

// StartInterface is the interface tunnels start from on a VLAN of the
// uplink (0063). Its routes are in table StartTable plus the VLAN, and it is
// in the firewall zone StartZone.
func StartInterface(vlan int) string {
	return fmt.Sprintf("aeolus_vlan%d_tunnels", vlan)
}

const (
	StartTable = 1000
	StartZone  = "aeolus_ul"
)

// startSection names the interfaces StartInterface makes.
var startSection = regexp.MustCompile(`^aeolus_vlan[0-9]+_tunnels$`)

type checker struct {
	c        *uci.Config
	ap       string // the AP's ID, or "" to leave its segment MACs unchecked
	problems []string
	noNet    bool         // "package network is missing" was said
	starts   map[int]bool // the VLANs whose start interface was checked (0063)
}

func (k *checker) add(format string, args ...any) {
	k.problems = append(k.problems, fmt.Sprintf(format, args...))
}

// Check returns every way the rendered UCI fails to carry the composed
// config doc (compose.AP, secrets opened). It never quotes a secret.
func Check(doc map[string]any, c *uci.Config) []string {
	return CheckAP(doc, c, "")
}

// CheckAP is Check for one AP, by its ID, from which it renders the MACs it
// uses on the segments it probes (0060): those are checked too.
func CheckAP(doc map[string]any, c *uci.Config, ap string) []string {
	k := &checker{c: c, ap: ap}
	radios := k.radios(doc)
	k.radioSettings(doc, radios)
	k.networks(doc, radios)
	k.system(obj(doc, "system"))
	k.agent(obj(doc, "system"))
	k.steering(obj(doc, "network"))
	k.snmp(obj(obj(doc, "system"), "snmp"))
	k.ports(obj(doc, "ports"), obj(doc, "concentrators"))
	k.probes(doc)
	sort.Strings(k.problems)
	if k.problems == nil {
		return []string{}
	}
	return k.problems
}

type device struct {
	band string
	s    *uci.Section
}

// radios lists the AP's radios in name order, and checks that the wireless
// package is there if anything needs it.
func (k *checker) radios(doc map[string]any) []device {
	w := k.c.Package("wireless")
	if w == nil {
		if doc["radio"] != nil || doc["network"] != nil {
			k.add("package wireless is missing")
		}
		return nil
	}
	var out []device
	for _, d := range w.OfType("wifi-device") {
		band, _ := d.Option("band")
		if d.Name == "" {
			k.add("wireless: the wifi-device at line %d has no name", d.Line)
			continue
		}
		out = append(out, device{band: band, s: d})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].s.Name < out[j].s.Name })
	return out
}

// auto2g are the channels an automatic 2.4 GHz radio picks from: the only
// ones that do not overlap (0045).
var auto2g = []string{"1", "6", "11"}

var htmodeRE = regexp.MustCompile(`^(NOHT|HT|VHT|HE|EHT)([0-9]*)$`)

func (k *checker) radioSettings(doc map[string]any, radios []device) {
	country, hasCountry := obj(doc, "system")["country"].(string)
	for _, r := range radios {
		where := "wireless." + r.s.Name
		if hasCountry {
			k.option(where, r.s, "country", country)
		}
		set := obj(obj(doc, "radio"), r.band)
		if v, ok := set["enabled"].(bool); ok && r.s.Flag("disabled") == v {
			k.add("%s: radio.%s.enabled is %v, but disabled is %q", where, r.band, v, value(r.s, "disabled"))
		}
		if v, ok := set["channel"]; ok {
			k.option(where, r.s, "channel", text(v))
			var want []string
			if v == "auto" && r.band == "2g" {
				want = auto2g
			}
			if got := r.s.List("channels"); !slices.Equal(got, want) {
				k.add("%s: channels is %q, want %q", where, got, want)
			}
		}
		if v, ok := set["width"].(float64); ok {
			m := htmodeRE.FindStringSubmatch(value(r.s, "htmode"))
			width := ""
			if m != nil {
				width = m[2]
				if m[1] == "NOHT" {
					width = "20"
				}
			}
			if width != text(v) {
				k.add("%s: htmode is %q, want a %v MHz mode", where, value(r.s, "htmode"), v)
			}
		}
		switch v := set["power"].(type) {
		case string: // auto
			if p, ok := r.s.Option("txpower"); ok {
				k.add("%s: txpower is %q, want none for auto", where, p)
			}
		case float64:
			k.option(where, r.s, "txpower", text(v))
		}
		if r.band == "5g" {
			if msg := bonding(value(r.s, "channel"), value(r.s, "htmode")); msg != "" {
				k.add("%s: %s", where, msg)
			}
		}
	}
}

// bonding checks that a 5 GHz radio's channel can carry its width, whoever
// set the channel: a width the channel cannot carry would leave the radio
// off the air while the AP still reaches the manager, so no revert would
// catch it.
func bonding(channel, htmode string) string {
	ch, err := strconv.Atoi(channel)
	if err != nil {
		return "" // auto, or not set: the radio picks a channel that fits
	}
	m := htmodeRE.FindStringSubmatch(htmode)
	if m == nil || m[1] == "NOHT" {
		return ""
	}
	w, _ := strconv.Atoi(m[2])
	if ok, why := radio.Fits("5g", ch, w); !ok {
		return why
	}
	return ""
}

func (k *checker) networks(doc map[string]any, radios []device) {
	w := k.c.Package("wireless")
	nets := obj(doc, "network")
	expected := map[string]bool{}
	for _, id := range keys(nets) {
		n := obj(nets, id)
		if on, ok := n["enabled"].(bool); ok && !on {
			continue
		}
		bands := map[string]bool{}
		for _, b := range list(n["bands"]) {
			bands[b] = true
		}
		for _, r := range radios {
			if len(bands) > 0 && !bands[r.band] {
				continue
			}
			name := IfaceName(id, r.s.Name)
			expected[name] = true
			s := w.Named(name)
			if s == nil || s.Type != "wifi-iface" {
				k.add("network.%s: no wifi-iface %s on %s", id, name, r.s.Name)
				continue
			}
			k.iface(id, n, r, s)
		}
		k.transports(id, n, obj(doc, "concentrators"))
	}
	for _, s := range w.OfType("wifi-iface") {
		if strings.HasPrefix(s.Name, "aeolus_") && !expected[s.Name] {
			k.add("wireless.%s: no network calls for it", s.Name)
		}
	}
}

func (k *checker) iface(id string, n map[string]any, r device, s *uci.Section) {
	where := "wireless." + s.Name
	k.option(where, s, "device", r.s.Name)
	k.option(where, s, "mode", "ap")
	if ssid, _ := n["ssid"].(string); ssid != "" {
		k.option(where, s, "ssid", ssid)
	}
	security, _ := n["security"].(string)
	if want, ok := encryption[security]; ok {
		got, _ := s.Option("encryption")
		if base, _, _ := strings.Cut(got, "+"); base != want {
			k.add("%s: encryption is %q, want %q for %s", where, got, want, security)
		}
	}
	if needsKey[security] {
		pass, _ := n["passphrase"].(string)
		if key, _ := s.Option("key"); key != pass {
			k.add("%s: key does not match the passphrase", where)
		}
	}
	roaming := obj(n, "roaming")
	rrm, _ := roaming["rrm"].(bool)
	btm, _ := roaming["btm"].(bool)
	steer, _ := n["band_steering"].(bool)
	for opt, v := range map[string]any{
		"hidden":         n["hidden"],
		"isolate":        n["isolation"],
		"ieee80211r":     roaming["ft"],
		"ieee80211k":     rrm || steer, // band steering works through 11k and 11v (0050)
		"bss_transition": btm || steer,
	} {
		want, _ := v.(bool)
		if s.Flag(opt) != want {
			k.add("%s: %s is %q, want %s", where, opt, value(s, opt), onOff(want))
		}
	}
	// Unset, it is left out, so OpenWrt's default holds (0049).
	if v, ok := n["multicast_to_unicast"].(bool); ok {
		k.option(where, s, "multicast_to_unicast", map[bool]string{true: "1", false: "0"}[v])
	} else if got, ok := s.Option("multicast_to_unicast"); ok {
		k.add("%s: multicast_to_unicast is %q, want none, for OpenWrt's default", where, got)
	}
	if s.Flag("disabled") {
		k.add("%s: disabled", where)
	}
	var ifaces []string
	for _, v := range s.List("network") {
		ifaces = append(ifaces, strings.Fields(v)...)
	}
	if len(ifaces) == 0 {
		k.add("%s: no network interface", where)
	}
	for _, name := range ifaces {
		if i := k.c.Package("network").Named(name); i == nil || i.Type != "interface" {
			k.add("%s: network interface %s does not exist", where, name)
		}
	}
}

// steering checks that usteer steers exactly the networks that ask for it,
// and nothing while none does (0050).
func (k *checker) steering(nets map[string]any) {
	var want []string
	for _, id := range keys(nets) {
		n := obj(nets, id)
		if on, _ := n["band_steering"].(bool); on && n["enabled"] != false {
			ssid, _ := n["ssid"].(string)
			want = append(want, ssid)
		}
	}
	sort.Strings(want)
	want = slices.Compact(want)
	var s *uci.Section
	if p := k.c.Package("usteer"); p != nil {
		if all := p.OfType("usteer"); len(all) > 0 {
			s = all[0]
		}
	}
	if s == nil {
		if len(want) > 0 {
			k.add("usteer: band steering needs usteer, which this AP does not have; install it (apk add usteer)")
		}
		return
	}
	interval := "0"
	if len(want) > 0 {
		interval = "30000"
	}
	k.option("usteer", s, "band_steering_interval", interval)
	if got := s.List("ssid_list"); !slices.Equal(got, want) {
		k.add("usteer: ssid_list is %q, want %q", got, want)
	}
}

// snmp checks that snmpd answers exactly what the intent asks, read-only,
// and nothing at all while SNMP is off (0052). Aeolus owns snmpd's whole
// config, so OpenWrt's default communities must be gone. A secret is never
// quoted.
func (k *checker) snmp(want map[string]any) {
	on, _ := want["enabled"].(bool)
	p := k.c.Package("snmpd")
	if p == nil {
		if on {
			k.add("snmpd: SNMP needs snmpd, which this AP does not have; install it (apk add snmpd-ssl)")
		}
		return
	}
	general := p.Named("general")
	if general == nil {
		k.add("snmpd: there is no general section")
		return
	}
	k.option("snmpd.general", general, "enabled", map[bool]string{true: "1", false: "0"}[on])
	community, _ := want["community"].(string)
	v3 := obj(want, "v3")
	user, _ := v3["user"].(string)

	var communities []string
	for _, typ := range []string{"com2sec", "com2sec6"} {
		for _, s := range p.OfType(typ) {
			c, _ := s.Option("community")
			communities = append(communities, c)
		}
	}
	if !on || community == "" {
		if len(communities) > 0 {
			k.add("snmpd: %d communities are set, want none", len(communities))
		}
	} else if len(communities) != 2 || communities[0] != community || communities[1] != community {
		k.add("snmpd: the communities do not match the intent's community, for IPv4 and IPv6")
	}
	for _, s := range p.OfType("access") {
		if w, _ := s.Option("write"); w != "none" {
			k.add("snmpd.%s: write is %q, want none: SNMP is read-only", s.Name, w)
		}
	}

	users := p.OfType("v3")
	switch {
	case !on || user == "":
		if len(users) > 0 {
			k.add("snmpd: %d v3 users are set, want none", len(users))
		}
	case len(users) != 1:
		k.add("snmpd: %d v3 users are set, want 1", len(users))
	default:
		u := users[0]
		where := "snmpd." + u.Name
		k.option(where, u, "username", user)
		k.option(where, u, "auth_type", "SHA")
		k.option(where, u, "privacy_type", "AES")
		auth, _ := v3["auth"].(string)
		privacy, _ := v3["privacy"].(string)
		if got, _ := u.Option("auth_pass"); got != auth {
			k.add("%s: auth_pass does not match the intent", where)
		}
		if got, _ := u.Option("privacy_pass"); got != privacy {
			k.add("%s: privacy_pass does not match the intent", where)
		}
		if u.Flag("allow_write") {
			k.add("%s: allow_write is on, want off: SNMP is read-only", where)
		}
		if v, _ := general.Option("snmp_version"); !strings.Contains(v, "v3") {
			k.add("snmpd.general: snmp_version is %q, want one with v3", v)
		}
	}
	if !on {
		return
	}
	systems := p.OfType("system")
	for name, opt := range map[string]string{"location": "sysLocation", "contact": "sysContact"} {
		v, set := want[name].(string)
		if !set {
			continue
		}
		if len(systems) != 1 {
			k.add("snmpd: %d system sections, want 1 for %s", len(systems), opt)
			continue
		}
		k.option("snmpd."+systems[0].Name, systems[0], opt, v)
	}
}

// ports checks the Ethernet ports the config sets (0053). Only ports in the
// VLAN-filtering bridge the uplink is in count; one the AP does not have is
// not judged. For each: whether it is on, its entries in the bridge's
// bridge-vlan sections, exactly, and that the uplink carries each of its
// VLANs tagged. The uplink itself is the AP's management, and is refused.
func (k *checker) ports(want, concentrators map[string]any) {
	if len(want) == 0 {
		return
	}
	net := k.c.Package("network")
	if net == nil {
		if !k.noNet {
			k.add("package network is missing")
			k.noNet = true
		}
		return
	}
	uplink := ""
	if s := k.c.Package("aeolus").Named("agent"); s != nil {
		uplink = value(s, "uplink")
	}
	var bridge *uci.Section
	for _, d := range net.OfType("device") {
		if value(d, "type") == "bridge" && slices.Contains(d.List("ports"), uplink) {
			bridge = d
			break
		}
	}
	if bridge == nil {
		k.add("ports: the uplink %q is in no bridge", uplink)
		return
	}
	var vlans []*uci.Section
	for _, bv := range net.OfType("bridge-vlan") {
		if value(bv, "device") == value(bridge, "name") {
			vlans = append(vlans, bv)
		}
	}
	// A port is on the AP if it is in a bridge, the uplink's or a tunnel's,
	// or carries an 802.1Q device (0058).
	onAP := func(p string) bool {
		for _, d := range net.OfType("device") {
			if value(d, "type") == "bridge" && slices.Contains(d.List("ports"), p) ||
				value(d, "type") == "8021q" && value(d, "ifname") == p {
				return true
			}
		}
		return false
	}
	for _, p := range keys(want) {
		set := obj(want, p)
		where := "ports." + p
		if !onAP(p) {
			continue
		}
		if p == uplink {
			k.add("%s: the uplink carries the AP's management; Aeolus leaves it alone", where)
			continue
		}
		if on, ok := set["enabled"].(bool); ok {
			got := true
			for _, d := range net.OfType("device") {
				if value(d, "name") == p && value(d, "type") == "" {
					if v, set := d.Option("enabled"); set {
						got = !slices.Contains([]string{"0", "no", "off", "false", "disabled"}, v)
					}
				}
			}
			if got != on {
				k.add("%s: the port is %s, want %s", where, onOff(got), onOff(on))
			}
		}
		mode, _ := set["mode"].(string)
		if mode == "tunnel" {
			k.tunnelPort(where, p, obj(set, "vxlan"), bridge, vlans, concentrators)
			continue
		}
		if mode != "access" && mode != "trunk" {
			continue
		}
		if !slices.Contains(bridge.List("ports"), p) {
			k.add("%s: the port is not in the uplink's bridge %s", where, value(bridge, "name"))
		}
		entries := map[string]string{} // VLAN -> u* or t
		if u := text(set["untagged"]); u != "" && u != "0" {
			entries[u] = "u*"
		}
		if mode == "trunk" {
			for _, v := range list(set["tagged"]) {
				entries[v] = "t"
			}
		}
		got := map[string]string{}
		for _, bv := range vlans {
			for _, e := range bv.List("ports") {
				if port, flags, _ := strings.Cut(e, ":"); port == p {
					got[value(bv, "vlan")] = flags
				}
			}
		}
		for _, vlan := range slices.Sorted(maps.Keys(entries)) {
			if got[vlan] != entries[vlan] {
				k.add("%s: VLAN %s is %s, want %s", where, vlan, portEntry(got[vlan]), portEntry(entries[vlan]))
			}
			carried := false
			for _, bv := range vlans {
				if value(bv, "vlan") == vlan && slices.Contains(bv.List("ports"), uplink+":t") {
					carried = true
				}
			}
			if !carried {
				k.add("%s: the uplink %s does not carry VLAN %s tagged", where, uplink, vlan)
			}
		}
		for _, vlan := range slices.Sorted(maps.Keys(got)) {
			if _, ok := entries[vlan]; !ok {
				k.add("%s: VLAN %s is %s, want not on the port", where, vlan, portEntry(got[vlan]))
			}
		}
	}
}

// start checks, once for each VLAN, the interface tunnels start from there
// (0063): on the uplink's bridge at that VLAN, its address by DHCP, its
// routes in a table of their own and its DNS servers unused, in a zone that
// rejects what comes in and what it would forward.
func (k *checker) start(vlan int) {
	if k.starts[vlan] {
		return
	}
	if k.starts == nil {
		k.starts = map[int]bool{}
	}
	k.starts[vlan] = true
	name := StartInterface(vlan)
	at := "network." + name
	s := k.c.Package("network").Named(name)
	if s == nil || s.Type != "interface" {
		k.add("%s: no interface for the tunnels that start from VLAN %d (0063)", at, vlan)
		return
	}
	k.option(at, s, "proto", "dhcp")
	if dev := value(s, "device"); !strings.HasSuffix(dev, "."+strconv.Itoa(vlan)) || strings.HasPrefix(dev, "aeolus_") {
		k.add("%s: device is %q, want VLAN %d on the uplink's bridge", at, dev, vlan)
	}
	carried := false
	for _, bv := range k.c.Package("network").OfType("bridge-vlan") {
		if value(bv, "vlan") == strconv.Itoa(vlan) {
			carried = true
		}
	}
	if !carried {
		k.add("%s: VLAN %d is not in the network config, so the uplink does not carry it", at, vlan)
	}
	k.option(at, s, "ip4table", strconv.Itoa(StartTable+vlan))
	k.option(at, s, "peerdns", "0")
	fw := k.c.Package("firewall")
	var zone *uci.Section
	if fw != nil {
		for _, z := range fw.OfType("zone") {
			if value(z, "name") == StartZone {
				zone = z
			}
		}
	}
	if zone == nil || !slices.Contains(zone.List("network"), name) {
		k.add("firewall: want %s in the zone %s, which keeps what comes in on VLAN %d out of the AP (0063)", name, StartZone, vlan)
		return
	}
	for _, opt := range []string{"input", "forward"} {
		k.option("firewall."+zone.Name, zone, opt, "REJECT")
	}
}

// underlayMTU is the MTU of the interface a tunnel runs over, as the AP's
// config sets it (0056): the interface's own mtu, or else its device's, or
// else, for a VLAN on a device (br-lan.1), that device's. Where nothing
// sets one, it is Linux's 1500. It returns the device too, to name it.
func underlayMTU(net *uci.Package, iface *uci.Section) (int, string) {
	dev := value(iface, "device")
	set := func(v string) (int, bool) {
		n, err := strconv.Atoi(v)
		return n, err == nil && n > 0
	}
	if n, ok := set(value(iface, "mtu")); ok {
		return n, dev
	}
	of := func(name string) string {
		for _, d := range net.OfType("device") {
			if value(d, "name") == name {
				return value(d, "mtu")
			}
		}
		return ""
	}
	if n, ok := set(of(dev)); ok {
		return n, dev
	}
	if i := strings.LastIndex(dev, "."); i > 0 {
		if n, ok := set(of(dev[:i])); ok {
			return n, dev
		}
	}
	return 1500, dev
}

// sectionSafe is a port's name as it can stand in a section's name.
var sectionSafe = regexp.MustCompile(`[^a-z0-9_]`)

// tunnelPort checks a port in tunnel mode (0058): it is out of the uplink's
// bridge and its VLANs, and each VNI it carries is on its tunnel's bridge,
// the port itself for the untagged one, and an 802.1Q device of the port,
// <port>.<vlan>, for each tagged one. Each tunnel is checked as a network's
// is, and must be started.
func (k *checker) tunnelPort(where, p string, maps map[string]any, bridge *uci.Section, vlans []*uci.Section, concentrators map[string]any) {
	net := k.c.Package("network")
	if slices.Contains(bridge.List("ports"), p) {
		k.add("%s: the port is still in the uplink's bridge %s", where, value(bridge, "name"))
	}
	for _, bv := range vlans {
		for _, e := range bv.List("ports") {
			if port, _, _ := strings.Cut(e, ":"); port == p {
				k.add("%s: VLAN %s of the uplink's bridge still has the port", where, value(bv, "vlan"))
			}
		}
	}
	for _, vlan := range keys(maps) {
		m := obj(maps, vlan)
		at := where + ".vxlan." + vlan
		vni := text(m["vni"])
		cid, _ := m["tunnel"].(string)
		if k.tunnel(at, "primary", m, obj(concentrators, cid)) == "" {
			continue
		}
		member := p
		if vlan != "untagged" {
			member = p + "." + vlan
			name := "aeolus_port_" + sectionSafe.ReplaceAllString(p, "_") + "_" + vlan
			if d := net.Named(name); d == nil || d.Type != "device" || value(d, "type") != "8021q" ||
				value(d, "ifname") != p || value(d, "vid") != vlan || value(d, "name") != member {
				k.add("network.%s: want an 802.1Q device %s, VLAN %s on %s", name, member, vlan, p)
			}
		}
		if b := net.Named(TunnelName(vni) + "_br"); b == nil || !slices.Contains(b.List("ports"), member) {
			k.add("%s: %s is not in the tunnel's bridge br-vx%s", at, member, vni)
		}
		if !k.held("br-vx" + vni) {
			k.add("%s: no interface is on the tunnel's bridge br-vx%s, so netifd never makes it and %s is on nothing; want network.%s_ports, proto none, on it", at, vni, member, TunnelName(vni))
		}
	}
}

// held says whether an interface that takes no address is on a bridge.
// netifd makes a bridge only for an interface on it: a network's, or for a
// VNI that only ports carry, one of its own (0058).
func (k *checker) held(bridge string) bool {
	for _, s := range k.c.Package("network").OfType("interface") {
		if value(s, "device") == bridge && value(s, "proto") == "none" {
			return true
		}
	}
	return false
}

// portEntry says what a bridge-vlan entry's flags make of the VLAN.
func portEntry(flags string) string {
	switch {
	case flags == "u*":
		return "untagged"
	case flags == "t":
		return "tagged"
	case flags == "":
		return "not on the port"
	}
	return fmt.Sprintf("%q", flags)
}

// transports checks that every transport a network keeps has its path in
// the network config. Both are rendered, as the AP chooses between them
// (0020), and the network's interface is on the primary's path. Until
// switching is built, a VXLAN fallback's tunnel is rendered but not
// started (0054).
func (k *checker) transports(id string, n map[string]any, concentrators map[string]any) {
	net := k.c.Package("network")
	if net == nil {
		if len(obj(n, "transport")) > 0 && !k.noNet {
			k.add("package network is missing")
			k.noNet = true
		}
		return
	}
	for _, slot := range []string{"primary", "fallback"} {
		t := obj(obj(n, "transport"), slot)
		where := "network." + id + ".transport." + slot
		path := "" // the device the network's interface is on, for this transport
		switch t["type"] {
		case "vlan":
			vlan := text(t["vlan"])
			found := false
			for _, d := range net.OfType("device") {
				if value(d, "type") == "8021q" && value(d, "vid") == vlan {
					found = true
				}
			}
			for _, bv := range net.OfType("bridge-vlan") {
				if value(bv, "vlan") == vlan {
					found = true
				}
			}
			if !found {
				k.add("%s: VLAN %s is not in the network config", where, vlan)
			}
			path = "." + vlan
		case "vxlan":
			path = k.tunnel(where, slot, t, obj(concentrators, text(t["concentrator"])))
		}
		if slot != "primary" || path == "" {
			continue
		}
		iface := net.Named(InterfaceName(id))
		if iface == nil || iface.Type != "interface" {
			k.add("network.%s: no interface %s", id, InterfaceName(id))
		} else if dev := value(iface, "device"); path[0] == '.' && !strings.HasSuffix(dev, path) {
			k.add("network.%s: interface %s is on %q, want the primary's VLAN %s", id, iface.Name, dev, path[1:])
		} else if path[0] != '.' && dev != path {
			k.add("network.%s: interface %s is on %q, want the primary's tunnel bridge %s", id, iface.Name, dev, path)
		}
	}
}

// tunnel checks a VXLAN transport's tunnel (0054): the interface named for
// its VNI, to the concentrator, started only for the primary; its bridge;
// the firewall rule that lets it in; and the MSS clamp below 1500. It
// returns the bridge the network's interface goes on.
func (k *checker) tunnel(where, slot string, t, conc map[string]any) string {
	net := k.c.Package("network")
	vni := text(t["vni"])
	address := strings.Trim(text(conc["address"]), "[]")
	port := text(conc["port"])
	name := TunnelName(vni)
	at := "network." + name
	s := net.Named(name)
	if s == nil || s.Type != "interface" {
		k.add("%s: no tunnel %s to %s with VNI %s (is the vxlan package installed?)", where, name, address, vni)
		return ""
	}
	proto, peer := "vxlan", "peeraddr"
	if strings.Contains(address, ":") {
		proto, peer = "vxlan6", "peer6addr"
	}
	k.option(at, s, "proto", proto)
	k.option(at, s, peer, address)
	k.option(at, s, "vid", vni)
	got := value(s, "port")
	if got == "" {
		got = "4789" // the vxlan protocol's default
	}
	if got != port {
		k.add("%s: port is %q, want %q", at, got, port)
	}
	k.option(at, s, "mtu", text(conc["mtu"]))
	if link := value(s, "tunlink"); link == "" {
		k.add("%s: tunlink is missing, want the management interface", at)
	} else if l := net.Named(link); l == nil || l.Type != "interface" {
		k.add("%s: tunlink %q names no interface", at, link)
	} else {
		// The tunnel's packets are its MTU plus VXLAN's headers, and the
		// AP's uplink must carry them whole: VXLAN endpoints seldom put
		// fragments back together (0056).
		overhead := 50
		if strings.Contains(address, ":") {
			overhead = 70
		}
		mtu, _ := conc["mtu"].(float64)
		if under, dev := underlayMTU(net, l); int(mtu)+overhead > under {
			if dev != "" {
				link += " (" + dev + ")"
			}
			k.add("%s: an MTU of %d needs %d on the AP's uplink, %s, which carries %d; raise the uplink's MTU, or leave the tunnel's MTU at its default (0056)",
				where, int(mtu), int(mtu)+overhead, link, under)
		}
	}
	// Where it starts (0063): the management interface, or the interface of
	// Aeolus's own on the VLAN the tunnel names.
	start := 0
	if v, ok := conc["underlay_vlan"].(float64); ok {
		start = int(v)
	}
	if link := value(s, "tunlink"); start > 0 && link != StartInterface(start) {
		k.add("%s: tunlink is %q, want %s, as the tunnel starts from VLAN %d (0063)", at, link, StartInterface(start), start)
	} else if start > 0 {
		k.start(start)
	} else if startSection.MatchString(link) {
		k.add("%s: the tunnel starts from %s, want the management interface (0063)", at, link)
	}
	auto := value(s, "auto") != "0"
	if slot == "primary" && !auto {
		k.add("%s: the primary's tunnel is not started", at)
	}
	if slot == "fallback" && auto {
		k.add("%s: the fallback's tunnel is started; it waits until the AP switches to it (0054)", at)
	}

	bridge := "br-vx" + vni
	if b := net.Named(name + "_br"); b == nil || b.Type != "device" || value(b, "type") != "bridge" || value(b, "name") != bridge {
		k.add("network.%s_br: want a bridge %s for the tunnel", name, bridge)
	} else if !slices.Contains(b.List("ports"), name) {
		k.add("network.%s_br: the bridge does not carry the tunnel %s", name, name)
	}

	fw := k.c.Package("firewall")
	if fw == nil {
		k.add("%s: package firewall is missing, for the tunnel's rule", where)
		return bridge
	}
	rule := "aeolus_vxlan_" + vni
	if r := fw.Named(rule); r == nil || r.Type != "rule" {
		k.add("firewall.%s: no rule letting the tunnel in from %s", rule, address)
	} else {
		for opt, want := range map[string]string{"proto": "udp", "src_ip": address, "dest_port": port, "target": "ACCEPT"} {
			k.option("firewall."+rule, r, opt, want)
		}
		if start > 0 {
			k.option("firewall."+rule, r, "src", StartZone)
		} else if value(r, "src") == "" {
			k.add("firewall.%s: src is missing, want the management interface's zone", rule)
		}
	}
	if mtu, _ := conc["mtu"].(float64); mtu < 1500 {
		if inc := fw.Named(clampInclude); inc == nil || inc.Type != "include" {
			k.add("%s: MTU %s needs the MSS clamp, firewall.%s (is kmod-nft-bridge installed?)", where, text(conc["mtu"]), clampInclude)
		} else {
			k.option("firewall."+clampInclude, inc, "type", "nftables")
			k.option("firewall."+clampInclude, inc, "path", clampPath)
		}
	}
	return bridge
}

var apHex = regexp.MustCompile(`^ap-([0-9a-f]{12})$`)

// SegmentMAC is the MAC an AP uses on one segment it probes (0060), made
// from its ID ("ap-a0046021365e"): 02 for a VNI, its number in decimal
// digits; 06 for a VLAN, the same; 0a for a VNI above 9999, its last 16
// bits in hex. Between them, the last three bytes of the AP's own MAC. ""
// for an ID that is not an AP's.
func SegmentMAC(ap, kind string, n int) string {
	m := apHex.FindStringSubmatch(ap)
	if m == nil || n < 0 {
		return ""
	}
	first, tail := "02", fmt.Sprintf("%04d", n)
	switch {
	case kind == "vlan":
		first = "06"
	case n > 9999:
		first, tail = "0a", fmt.Sprintf("%04x", n&0xffff)
	}
	h := m[1]
	return fmt.Sprintf("%s:%s:%s:%s:%s:%s", first, h[6:8], h[8:10], h[10:12], tail[:2], tail[2:])
}

// The tunnels Aeolus makes and their bridges, by name (0054).
var (
	tunnelSection = regexp.MustCompile(`^aeolus_([0-9]+)$`)
	bridgeSection = regexp.MustCompile(`^aeolus_[0-9]+_br$`)
)

// probes checks the prober's plan (0059), in the agent's own package: for
// each tunnel Aeolus made, a probe section named for it, with its tunnel's
// probe interval and every probe address set for its VNI, by a network or a
// tunnel port; and for each port on a tunnel, a guard section naming the
// port, and the devices the loop guard sends on: the port itself, and its
// 802.1Q devices on tunnels. Like the renderer, it takes the tunnels and the
// ports on them from the network config as rendered, which the other checks
// hold to the intent.
func (k *checker) probes(doc map[string]any) {
	net := k.c.Package("network")
	if net == nil {
		return
	}
	type use struct {
		interval string
		address  []string
	}
	uses := map[string]*use{}
	concs := obj(doc, "concentrators")
	add := func(vni, tunnel string, probe any) {
		u := uses[vni]
		if u == nil {
			u = &use{}
			uses[vni] = u
		}
		if iv, ok := obj(concs, tunnel)["probe_interval"]; ok && u.interval == "" {
			u.interval = text(iv)
		}
		if a, ok := probe.(string); ok && !slices.Contains(u.address, a) {
			u.address = append(u.address, a)
		}
	}
	for _, id := range keys(obj(doc, "network")) {
		n := obj(obj(doc, "network"), id)
		if n["enabled"] == false {
			continue
		}
		for _, slot := range []string{"primary", "fallback"} {
			if t := obj(obj(n, "transport"), slot); t["type"] == "vxlan" {
				add(text(t["vni"]), text(t["concentrator"]), t["probe"])
			}
		}
	}
	for _, p := range keys(obj(doc, "ports")) {
		set := obj(obj(doc, "ports"), p)
		if set["mode"] != "tunnel" {
			continue
		}
		for _, vlan := range keys(obj(set, "vxlan")) {
			m := obj(obj(set, "vxlan"), vlan)
			add(text(m["vni"]), text(m["tunnel"]), m["probe"])
		}
	}

	a := k.c.Package("aeolus")
	expected := map[string]bool{}
	for _, s := range net.OfType("interface") {
		m := tunnelSection.FindStringSubmatch(s.Name)
		if p := value(s, "proto"); m == nil || p != "vxlan" && p != "vxlan6" {
			continue
		}
		expected[s.Name] = true
		interval, address := "30", []string(nil)
		if u := uses[m[1]]; u != nil {
			if u.interval != "" {
				interval = u.interval
			}
			address = u.address
		}
		where := "aeolus." + s.Name
		p := a.Named(s.Name)
		if p == nil || p.Type != "probe" {
			k.add("%s: no probe section for the tunnel, which the prober needs (0059)", where)
			continue
		}
		k.option(where, p, "vni", m[1])
		k.option(where, p, "interval", interval)
		if got := p.List("address"); !sameSet(got, address) {
			k.add("%s: probe addresses are %v, want %v", where, got, address)
		}
		// The AP's MAC on the segment: the probe's, for its lease, and the
		// bridge's, so what answers it stays at the AP (0060).
		if k.ap != "" {
			n, _ := strconv.Atoi(m[1])
			want := SegmentMAC(k.ap, "vni", n)
			k.option(where, p, "mac", want)
			if b := net.Named(s.Name + "_br"); b != nil {
				k.option("network."+b.Name, b, "macaddr", want)
			}
		}
	}

	ports := map[string]string{} // an 802.1Q device Aeolus made -> the port under it
	for _, d := range net.OfType("device") {
		if strings.HasPrefix(d.Name, "aeolus_") && value(d, "type") == "8021q" && value(d, "name") != "" {
			ports[value(d, "name")] = value(d, "ifname")
		}
	}
	guards := map[string][]string{}
	for _, b := range net.OfType("device") {
		if !bridgeSection.MatchString(b.Name) {
			continue
		}
		for _, e := range b.List("ports") {
			if strings.HasPrefix(e, "aeolus_") {
				continue // the tunnel itself
			}
			p := e
			if q, ok := ports[e]; ok {
				p = q
			}
			if !slices.Contains(guards[p], e) {
				guards[p] = append(guards[p], e)
			}
		}
	}
	for _, p := range slices.Sorted(maps.Keys(guards)) {
		name := "aeolus_guard_" + sectionSafe.ReplaceAllString(p, "_")
		expected[name] = true
		where := "aeolus." + name
		g := a.Named(name)
		if g == nil || g.Type != "guard" {
			k.add("%s: no loop guard for %s, which is on a tunnel (0059)", where, p)
			continue
		}
		k.option(where, g, "port", p)
		if got := g.List("device"); !sameSet(got, guards[p]) {
			k.add("%s: guards %v, want %v", where, got, guards[p])
		}
	}
	if a != nil {
		for _, s := range a.Sections {
			if strings.HasPrefix(s.Name, "aeolus_") && !expected[s.Name] {
				k.add("aeolus.%s: no tunnel or tunnel port calls for it", s.Name)
			}
		}
	}
}

func (k *checker) system(sys map[string]any) {
	if len(sys) == 0 {
		return
	}
	p := k.c.Package("system")
	checked := sys["tz"] != nil || sys["ntp"] != nil || sys["syslog"] != nil
	if p == nil {
		if checked {
			k.add("package system is missing")
		}
		return
	}
	var s *uci.Section
	if all := p.OfType("system"); len(all) > 0 {
		s = all[0]
	}
	if s == nil {
		if sys["tz"] != nil || sys["syslog"] != nil {
			k.add("system: no system section")
		}
	} else {
		if tz, ok := sys["tz"].(string); ok {
			k.option("system", s, "zonename", tz)
		}
		if syslog, ok := sys["syslog"].(string); ok {
			host, port := splitHostPort(syslog)
			k.option("system", s, "log_ip", host)
			if port != "" {
				k.option("system", s, "log_port", port)
			}
		}
	}
	if ntp := list(sys["ntp"]); sys["ntp"] != nil {
		var got []string
		if t := p.Named("ntp"); t != nil && t.Type == "timeserver" {
			got = t.List("server")
		}
		if !sameSet(got, ntp) {
			k.add("system.ntp: servers are %v, want %v", got, ntp)
		}
	}
}

// agent checks the settings rendered for the agent itself, in its own
// package (0040): the poll interval.
func (k *checker) agent(sys map[string]any) {
	poll, ok := sys["poll"]
	if !ok {
		return
	}
	s := k.c.Package("aeolus").Named("agent")
	if s == nil || s.Type != "agent" {
		k.add("aeolus: no agent section, want poll %s", text(poll))
		return
	}
	k.option("aeolus.agent", s, "poll", text(poll))
}

// option checks one option's value. Only non-secret options go through here.
func (k *checker) option(where string, s *uci.Section, name, want string) {
	if got, ok := s.Option(name); !ok {
		k.add("%s: %s is missing, want %q", where, name, want)
	} else if got != want {
		k.add("%s: %s is %q, want %q", where, name, got, want)
	}
}

// splitHostPort reads "host", "host:port", "[v6]" or "[v6]:port".
func splitHostPort(s string) (host, port string) {
	if strings.HasPrefix(s, "[") {
		end := strings.Index(s, "]")
		if end < 0 {
			return s, ""
		}
		host, rest := s[1:end], s[end+1:]
		return host, strings.TrimPrefix(rest, ":")
	}
	if strings.Count(s, ":") == 1 {
		host, port, _ := strings.Cut(s, ":")
		return host, port
	}
	return s, ""
}

func obj(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func list(v any) []string {
	var out []string
	for _, e := range asSlice(v) {
		out = append(out, text(e))
	}
	return out
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

// text writes a JSON value as UCI holds it: numbers without a decimal point.
func text(v any) string {
	switch x := v.(type) {
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		return x
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}

func value(s *uci.Section, name string) string {
	v, _ := s.Option(name)
	return v
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, x := range a {
		seen[x]++
	}
	for _, x := range b {
		seen[x]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}
