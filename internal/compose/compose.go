// Package compose builds the config an AP receives (0008): its resolved
// fields, with each network's transports filtered by the tunnels set where
// the AP is (0055), and checked whole (0029). The API shows it, and the AP
// poll serves it.
package compose

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
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
}

var slots = []string{"primary", "fallback"}

// AP composes one AP's config:
//   - A VXLAN transport whose tunnel is not set at the AP's location is left
//     out there (0055). If that removes the primary, the fallback takes its
//     place.
//   - A network left with no transport is a problem.
//   - The config carries only the tunnels its transports use, so an
//     unfinished tunnel nothing uses holds no AP back. A tunnel without an
//     MTU gets the default for its far end's address family (0056).
//   - A radio width the AP's radio cannot do, by what it reported when it
//     enrolled, is a problem (0008), as is a 5 GHz channel Aeolus sets that
//     cannot carry the width Aeolus sets (0045).
func AP(s *change.State, sch *schema.Schema, ap hierarchy.NodeID, reveal Reveal) (Result, error) {
	cfg, err := s.Org.ResolveAP(ap)
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
			if _, set := fields["concentrators."+cid+".address"]; set {
				kept[slot] = true
				used[cid] = true
			} // else: the tunnel is not set here, so the transport is left out (0055)
		}
		hadTransport := false
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
		for f, v := range net {
			fields["network."+id+"."+f] = v
		}
	}

	for p := range fields {
		if rest, ok := strings.CutPrefix(p, "concentrators."); ok {
			if cid, _, _ := strings.Cut(rest, "."); !used[cid] {
				delete(fields, p)
			}
		}
	}
	for cid := range used {
		if _, set := fields["concentrators."+cid+".mtu"]; !set {
			addr, _ := fields["concentrators."+cid+".address"].(string)
			fields["concentrators."+cid+".mtu"] = float64(DefaultMTU(addr))
		}
	}

	doc, err := schema.Assemble(fields, reveal)
	if err != nil {
		return Result{}, err
	}
	doc = jsonShape(doc)
	sort.Strings(problems)
	problems = append(problems, sch.Problems(doc)...)
	problems = append(problems, radioProblems(doc, s.Facts[ap])...)
	problems = append(problems, bondingProblems(doc)...)
	problems = append(problems, snmpProblems(doc)...)
	problems = append(problems, portProblems(doc)...)
	problems = append(problems, tunnelProblems(doc)...)
	if problems == nil {
		problems = []string{}
	}
	return Result{Doc: doc, Problems: problems}, nil
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

// tunnelProblems refuses VXLAN transports an AP cannot run (0054). An AP
// runs one tunnel per VNI, so two networks on one VNI would become one, and a
// network's primary and fallback cannot share one yet. A tunnel needs its far
// end's IP address, as names do not resolve reliably on an AP.
func tunnelProblems(doc map[string]any) []string {
	nets, _ := doc["network"].(map[string]any)
	concs, _ := doc["concentrators"].(map[string]any)
	users := map[int][]string{} // VNI -> the networks that use it
	var out []string
	for _, id := range sortedKeys(nets) {
		n, _ := nets[id].(map[string]any)
		if n["enabled"] == false {
			continue // not rendered
		}
		transport, _ := n["transport"].(map[string]any)
		slots := map[int]string{}
		for _, slot := range []string{"primary", "fallback"} {
			t, _ := transport[slot].(map[string]any)
			vni, ok := t["vni"].(float64)
			if t["type"] != "vxlan" || !ok {
				continue
			}
			if other, ok := slots[int(vni)]; ok {
				out = append(out, fmt.Sprintf("network.%s.transport: the %s and the %s both use VNI %d; an AP runs one tunnel per VNI", id, other, slot, int(vni)))
				continue
			}
			slots[int(vni)] = slot
			users[int(vni)] = append(users[int(vni)], id)
			cid, _ := t["concentrator"].(string)
			c, _ := concs[cid].(map[string]any)
			if addr, _ := c["address"].(string); addr != "" && net.ParseIP(strings.Trim(addr, "[]")) == nil {
				out = append(out, fmt.Sprintf("network.%s.transport.%s: tunnel %s's address %q is not an IP address", id, slot, cid, addr))
			}
		}
	}
	vnis := make([]int, 0, len(users))
	for v := range users {
		vnis = append(vnis, v)
	}
	sort.Ints(vnis)
	for _, v := range vnis {
		if len(users[v]) > 1 {
			out = append(out, fmt.Sprintf("VNI %d: networks %s would share one tunnel at this AP, and so become one network", v, strings.Join(users[v], " and ")))
		}
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
		transport, _ := n["transport"].(map[string]any)
		for _, slot := range slots {
			t, _ := transport[slot].(map[string]any)
			if t["type"] != "vxlan" {
				continue
			}
			if r.VXLANLoaded != nil && !*r.VXLANLoaded {
				out = append(out, fmt.Sprintf("network.%s.transport.%s: this AP's netifd has not loaded vxlan; restart its network (/etc/init.d/network restart), and install vxlan first if it is missing (0057)", id, slot))
			}
			if r.UplinkMTU == 0 {
				continue
			}
			cid, _ := t["concentrator"].(string)
			c, _ := concs[cid].(map[string]any)
			mtu, _ := c["mtu"].(float64)
			addr, _ := c["address"].(string)
			need := int(mtu) + 50
			if strings.Contains(addr, ":") {
				need = int(mtu) + 70
			}
			if need > r.UplinkMTU {
				out = append(out, fmt.Sprintf("network.%s.transport.%s: tunnel %s's MTU of %d needs %d on the AP's uplink, which carries %d now; raise the uplink's MTU, or leave the tunnel's MTU at its default (0056)",
					id, slot, cid, int(mtu), need, r.UplinkMTU))
			}
		}
	}
	return out
}

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

// promote moves the fallback transport into the primary's place.
func promote(net map[string]any) {
	for f, v := range net {
		if rest, ok := strings.CutPrefix(f, "transport.fallback."); ok {
			net["transport.primary."+rest] = v
			delete(net, f)
		}
	}
}
