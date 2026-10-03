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

	"ports.*.enabled":  layout,
	"ports.*.uplink":   layout,
	"ports.*.mode":     layout,
	"ports.*.untagged": layout,
	"ports.*.tagged":   layout,
	"ports.*.bond":     layout,

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
	"network.*.rate_limit.down_kbps":     "no standard UCI form; the agent's own setting, checked with it in M5",
	"network.*.rate_limit.up_kbps":       "no standard UCI form; the agent's own setting, checked with it in M5",
	"network.*.transport.*.type":         "",
	"network.*.transport.*.vlan":         "",
	"network.*.transport.*.concentrator": "",
	"network.*.transport.*.vni":          "",
	"network.*.transport.ha":             "the agent's own behavior (0022); checked with the agent in M5",
	"network.*.transport.failback":       "the agent's own behavior (0022); checked with the agent in M5",
	"network.*.transport.holddown":       "the agent's own behavior (0022); checked with the agent in M5",

	"concentrators.*.address": "",
	"concentrators.*.port":    "",
	"concentrators.*.mtu":     "",
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

type checker struct {
	c        *uci.Config
	problems []string
	noNet    bool // "package network is missing" was said
}

func (k *checker) add(format string, args ...any) {
	k.problems = append(k.problems, fmt.Sprintf(format, args...))
}

// Check returns every way the rendered UCI fails to carry the composed
// config doc (compose.AP, secrets opened). It never quotes a secret.
func Check(doc map[string]any, c *uci.Config) []string {
	k := &checker{c: c}
	radios := k.radios(doc)
	k.radioSettings(doc, radios)
	k.networks(doc, radios)
	k.system(obj(doc, "system"))
	k.agent(obj(doc, "system"))
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
	for opt, v := range map[string]any{
		"hidden":         n["hidden"],
		"isolate":        n["isolation"],
		"ieee80211r":     roaming["ft"],
		"ieee80211k":     roaming["rrm"],
		"bss_transition": roaming["btm"],
	} {
		want, _ := v.(bool)
		if s.Flag(opt) != want {
			k.add("%s: %s is %q, want %s", where, opt, value(s, opt), onOff(want))
		}
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

// transports checks that every transport a network keeps has its path in
// the network config. Both are rendered; the AP chooses between them (0020).
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
		case "vxlan":
			cid, _ := t["concentrator"].(string)
			conc := obj(concentrators, cid)
			vni, address := text(t["vni"]), text(conc["address"])
			var tunnel *uci.Section
			for _, i := range net.OfType("interface") {
				if value(i, "proto") == "vxlan" && value(i, "vid") == vni && value(i, "peeraddr") == address {
					tunnel = i
				}
			}
			if tunnel == nil {
				k.add("%s: no vxlan interface to %s with VNI %s", where, address, vni)
				continue
			}
			port := value(tunnel, "port")
			if port == "" {
				port = "4789" // the vxlan protocol's default
			}
			if want := text(conc["port"]); port != want {
				k.add("network.%s: port is %q, want %q", tunnel.Name, port, want)
			}
			k.option("network."+tunnel.Name, tunnel, "mtu", text(conc["mtu"]))
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
