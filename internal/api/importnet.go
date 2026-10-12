package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Making an AP's own network Aeolus's (0137): import. A person asks it of
// a network an AP reports as its own, with where the network is to go: a
// Services folder, an ID, its bands and its VLAN. The rest, the passphrase
// above all, only the AP has: it is asked for it by an action,
// import-network, and when it answers, the manager makes the network, in
// one change, under the name of whoever asked. The network says it
// replaces the APs' own (replace_own): each AP that has the same network
// of its own takes that out as Aeolus's goes in.
//
// What the AP answers is held to what was asked: the same network, of the
// security the form showed, an access point's, with nothing in it Aeolus
// cannot keep and its clients may depend on; and the change is tried again
// as it now is, before it is made. An AP's report is up to five minutes
// old, and a network may have changed since; and an AP is believed for its
// own network, and for no more than that.
//
// The passphrase is in what the AP answers and in the change, where it is
// sealed as any passphrase is (0027). It is in no report, is not kept with
// the action, and is never logged.

// importAsk is what an import was asked with, kept with its action until
// the AP answers: the network, by its SSID and the AP's sections; the
// section the AP is to say, Tell; the security it was asked as, as Aeolus
// names it; and where it goes.
type importAsk struct {
	SSID     string   `json:"ssid"`
	Sections []string `json:"sections"`
	Tell     string   `json:"tell"`
	Security string   `json:"security"`
	Folder   string   `json:"folder"`
	Network  string   `json:"network"`
	Bands    []string `json:"bands,omitempty"`
	VLAN     int      `json:"vlan"`
}

// toldNetwork is what an AP says of one of its own networks when asked
// (the agent's others.told): its section as it stands in the AP's config,
// every option, its key among them, and the VLAN it is on.
type toldNetwork struct {
	Section string                     `json:"section"`
	SSID    string                     `json:"ssid"`
	Options map[string]json.RawMessage `json:"options"`
	VLAN    int                        `json:"vlan"`
}

// aeolusSecurity is a network's security as Aeolus names it, from its
// encryption as OpenWrt does, and whether Aeolus has a name for it: not
// for WPA alone, or WEP.
func aeolusSecurity(encryption string) (string, bool) {
	base, _, _ := strings.Cut(encryption, "+")
	if base == "" {
		base = "none"
	}
	s, ok := map[string]string{
		"none": "open", "owe": "owe", "psk2": "wpa2-psk", "sae": "wpa3-sae", "sae-mixed": "wpa2-wpa3",
		"wpa2": "wpa2-enterprise", "wpa3": "wpa3-enterprise", "wpa3-mixed": "wpa2-wpa3-enterprise",
	}[base]
	return s, ok
}

// importQuiet is the options of an AP's own network that go with no word:
// where it is and what it is, which the network's own settings say, and
// how fast transition is set up, which Aeolus does its own way.
var importQuiet = map[string]bool{
	"device": true, "network": true, "mode": true, "ifname": true, "ssid": true,
	"ft_over_ds": true, "ft_psk_generate_local": true, "mobility_domain": true, "reassociation_deadline": true,
	"pmk_r1_push": true, "r0kh": true, "r1kh": true,
}

// importNeeds is the options Aeolus has no place for and a network's
// clients, or another AP, may depend on: a network that has one is not
// imported, as Aeolus's would be offered without it. Each with why.
var importNeeds = map[string]string{
	"wds":          "another AP may reach the network through it (wds)",
	"wpa_psk_file": "its devices have passphrases of their own, in a file (wpa_psk_file)",
	"ppsk":         "its devices have passphrases of their own, from a RADIUS server (ppsk)",
	"sae_password": "it has WPA3 passwords of its own (sae_password)",
	"vlan_file":    "it puts clients in VLANs by a file (vlan_file)",
	"dynamic_vlan": "it puts clients in VLANs as a RADIUS server says (dynamic_vlan)",
}

// uciTrue says a UCI option is on, as OpenWrt reads one: 1, true, on, yes
// or enabled.
func uciTrue(s string) bool {
	switch strings.ToLower(s) {
	case "1", "true", "on", "yes", "enabled":
		return true
	}
	return false
}

// importValues is the network Aeolus makes of one an AP said: its fields,
// by their paths below network.<id>., and the options of the AP's that
// have no place among them, by name alone, for whoever imports to see what
// was left behind. It fails where the network cannot be Aeolus's: what the
// AP said is no access point's, or of another security than it was asked
// as; a security Aeolus has no name for; a key that is no passphrase; or
// something Aeolus cannot keep that its clients may depend on.
func importValues(ask importAsk, told toldNetwork) (map[string]any, []string, error) {
	used := map[string]bool{}
	list := func(k string) []string {
		used[k] = true
		raw, ok := told.Options[k]
		if !ok {
			return nil
		}
		var many []string
		if json.Unmarshal(raw, &many) == nil {
			return many
		}
		var one string
		if json.Unmarshal(raw, &one) == nil {
			return []string{one}
		}
		return nil
	}
	get := func(k string) string {
		if v := list(k); len(v) > 0 {
			return v[0]
		}
		return ""
	}
	on := func(k string) bool { return uciTrue(get(k)) }
	number := func(k string, lo, hi int) (int, bool) {
		n, err := strconv.Atoi(get(k))
		return n, err == nil && n >= lo && n <= hi
	}
	var left []string

	if mode := get("mode"); mode != "" && mode != "ap" {
		return nil, nil, fmt.Errorf("what the AP said is no access point's network (mode %s)", printableText(mode, 16))
	}
	sec, ok := aeolusSecurity(get("encryption"))
	if !ok {
		return nil, nil, fmt.Errorf("its security (%s) is none Aeolus has a name for", printableText(get("encryption"), 32))
	}
	if ask.Security != "" && sec != ask.Security {
		return nil, nil, fmt.Errorf("the AP has it as %s now, and it was asked for as %s: look again, and ask again", sec, ask.Security)
	}
	// What Aeolus cannot keep, and its clients or another AP may depend on.
	var needs []string
	if on("wds") {
		needs = append(needs, importNeeds["wds"])
	}
	for _, opt := range []string{"wpa_psk_file", "vlan_file", "sae_password"} {
		if len(list(opt)) > 0 {
			needs = append(needs, importNeeds[opt])
		}
	}
	if on("ppsk") {
		needs = append(needs, importNeeds["ppsk"])
	}
	if d := get("dynamic_vlan"); d != "" && d != "0" {
		needs = append(needs, importNeeds["dynamic_vlan"])
	}
	filter := get("macfilter")
	switch filter {
	case "allow":
		needs = append(needs, "it lets on only the clients in a list (macfilter allow)")
	case "radius":
		needs = append(needs, "it asks a RADIUS server of each client's MAC its own way (macfilter radius)")
	}
	if len(needs) > 0 {
		return nil, nil, fmt.Errorf("Aeolus cannot keep what it has: %s. It is left as it is", strings.Join(needs, "; "))
	}

	v := map[string]any{
		"ssid": told.SSID, "security": sec, "replace_own": true,
		"transport.primary.type": "vlan", "transport.primary.vlan": ask.VLAN,
	}
	if len(ask.Bands) > 0 {
		v["bands"] = ask.Bands
	}
	switch {
	case needsPassphrase[sec]:
		key := get("key")
		if len(key) < 8 || len(key) > 63 {
			return nil, nil, fmt.Errorf("its key is no passphrase of 8 to 63 characters, as Aeolus takes: a key of 64 digits is not one")
		}
		v["passphrase"] = key
	case enterprise[sec]:
		server := get("auth_server")
		if server == "" {
			server = get("server")
		}
		if server == "" {
			return nil, nil, fmt.Errorf("it signs clients in, and names no RADIUS server")
		}
		v["radius.auth_server"] = server
		secret := get("auth_secret")
		if secret == "" {
			secret = get("key")
		}
		if secret != "" {
			v["radius.auth_secret"] = secret
		}
		// The option that names it outright, before OpenWrt's older word.
		for _, p := range [][2]string{{"port", "radius.auth_port"}, {"auth_port", "radius.auth_port"}, {"acct_port", "radius.acct_port"}, {"dae_port", "radius.das.port"}} {
			if n, ok := number(p[0], 1, 65535); ok {
				v[p[1]] = n
			}
		}
		for _, p := range [][2]string{{"acct_server", "radius.acct_server"}, {"acct_secret", "radius.acct_secret"}, {"nasid", "radius.nas_id"}, {"dae_client", "radius.das.client"}, {"dae_secret", "radius.das.secret"}} {
			if s := get(p[0]); s != "" {
				v[p[1]] = s
			}
		}
	}
	// A key on a network that takes none is not carried, and not named
	// beyond that it was there.
	used["key"] = true
	for _, p := range [][2]string{{"hidden", "hidden"}, {"isolate", "isolation"}, {"ieee80211r", "roaming.ft"}, {"ieee80211k", "roaming.rrm"}, {"bss_transition", "roaming.btm"}} {
		if on(p[0]) {
			v[p[1]] = true
		}
	}
	if on("disabled") {
		v["enabled"] = false
	}
	// The most clients (0097), and the DTIM period, each within what
	// Aeolus takes: one outside it is left behind, and said.
	for _, p := range []struct {
		opt, field string
		hi         int
	}{{"maxassoc", "max_clients", 2007}, {"dtim_period", "dtim", 255}} {
		if get(p.opt) == "" {
			continue
		}
		if n, ok := number(p.opt, 1, p.hi); ok {
			v[p.field] = n
		} else {
			left = append(left, p.opt)
		}
	}
	for _, opt := range []string{"multicast_to_unicast_all", "multicast_to_unicast"} {
		if s := get(opt); s != "" {
			v["multicast_to_unicast"] = uciTrue(s)
		}
	}
	// The clients it turns away, by MAC (0100).
	macs := list("maclist")
	if filter == "deny" {
		var blocked []string
		for _, m := range macs {
			if macRE.MatchString(strings.ToLower(m)) && !slices.Contains(blocked, strings.ToLower(m)) {
				blocked = append(blocked, strings.ToLower(m))
			}
		}
		if len(blocked) > 0 {
			v["blocked"] = blocked
		}
	}
	// Management frame protection is as the security has it in Aeolus.
	if w := get("ieee80211w"); w != "" && w != "0" {
		left = append(left, "ieee80211w")
	}
	for k := range told.Options {
		if !used[k] && !importQuiet[k] && otherSectionRE.MatchString(k) {
			left = append(left, k)
		}
	}
	sort.Strings(left)
	return v, slices.Compact(left), nil
}

// needsPassphrase is the securities that sign every client in with the
// network's passphrase.
var needsPassphrase = map[string]bool{"wpa2-psk": true, "wpa3-sae": true, "wpa2-wpa3": true}

// printableText is s cut to n bytes, with what is not plain ASCII as a
// question mark: for a result, which the manager keeps as plain words.
func printableText(s string, n int) string {
	if len(s) > n {
		s = s[:n]
	}
	b := []byte(s)
	for i, c := range b {
		if c < ' ' || c > '~' {
			b[i] = '?'
		}
	}
	return string(b)
}

// importOp is the change that makes a network of values: one set, on the
// Services folder, of every field.
func importOp(ask importAsk, values map[string]any) (change.Op, error) {
	op := change.Op{Kind: change.Set, Tree: change.Services, Node: hierarchy.NodeID(ask.Folder), Values: map[hierarchy.Path]json.RawMessage{}}
	for k, v := range values {
		raw, err := json.Marshal(v)
		if err != nil {
			return op, err
		}
		op.Values[hierarchy.Path("network."+ask.Network+"."+k)] = raw
	}
	return op, nil
}

// importReady tries the change an import would be, as actor, and keeps
// nothing: the folder is one, the ID is no network's there, above it or
// below it; the values are ones the schema takes; the actor may make the
// change; and no AP's config would be held for it (0029). It is asked
// twice: when the import is asked, with a stand-in for what only the AP
// has, and again when the AP has answered, with what it said, before the
// change is made. It gives back the change, ready to be made.
func (s *Server) importReady(actor string, state *change.State, ask importAsk, values map[string]any) (change.Op, error) {
	t := state.Org.Services
	folder := hierarchy.NodeID(ask.Folder)
	if n, ok := t.Node(folder); !ok || n.Kind == hierarchy.KindAP {
		return change.Op{}, badRequest("import-network: folder, the Services folder the network goes in")
	}
	mark := hierarchy.Path("network." + ask.Network + ".ssid")
	_, has := t.Resolve(folder, mark)
	for _, below := range t.Descendants(folder) {
		if _, set := t.Own(below, mark); set {
			has = true
		}
	}
	if has {
		return change.Op{}, &apiError{http.StatusConflict, fmt.Sprintf("there is a network %s there already, or in a folder below: give this one another ID", ask.Network)}
	}
	op, err := importOp(ask, values)
	if err != nil {
		return op, err
	}
	if op, err = s.prepare(op); err != nil {
		return op, err
	}
	after, _, changed, err := s.log.Preview(actor, op)
	if err != nil {
		return op, err
	}
	held := s.checks(after, changed)
	ids := make([]string, 0, len(held))
	for id := range held {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, id := range ids {
		name := id
		if n, ok := after.Org.Locations.Node(hierarchy.NodeID(id)); ok {
			name = n.Name
		}
		return op, &apiError{http.StatusConflict, fmt.Sprintf("Aeolus would hold %s's config with this network: %s", name, held[hierarchy.NodeID(id)][0])}
	}
	return op, nil
}

// importFor is the asking for an AP's own network to be made Aeolus's:
// the sections and SSID are what the AP last said it has, as for a taking
// out; its security is one Aeolus has; and the change it will be, tried
// now with a stand-in for what only the AP knows, is one that can be made.
func (s *Server) importFor(c call, ap hierarchy.NodeID, ssid string, sections []string, folder, id string, bands []string, vlan int) (importAsk, error) {
	sel, err := s.removeNetworkFor(ap, ssid, sections)
	if err != nil {
		return importAsk{}, err
	}
	ask := importAsk{SSID: ssid, Sections: sel.Sections, Folder: folder, Network: id, Bands: bands, VLAN: vlan}
	if vlan < 1 || vlan > 4094 {
		return ask, badRequest("import-network: vlan, the VLAN its clients are on, from 1 to 4094")
	}
	for _, b := range bands {
		if !bandsOK[b] {
			return ask, badRequest("import-network: bands are 2g, 5g and 6g")
		}
	}
	st, err := s.conds.LatestState(ap)
	if err != nil {
		return ask, err
	}
	var rep struct {
		Others []otherNetwork `json:"others"`
	}
	if st != nil {
		_ = json.Unmarshal(st.Report, &rep)
	}
	// The section the AP is to say: one off 6 GHz where there is one, as
	// a WPA2/WPA3 network is WPA3 alone there (0086), and it is the
	// network's own security that is asked for.
	encryption := ""
	for _, o := range rep.Others {
		if o.SSID != ssid || !slices.Contains(ask.Sections, o.Section) {
			continue
		}
		if ask.Tell == "" || o.Band != "6g" {
			ask.Tell, encryption = o.Section, o.Security
			if o.Band != "6g" {
				break
			}
		}
	}
	sec, ok := aeolusSecurity(encryption)
	if !ok {
		return ask, &apiError{http.StatusConflict, fmt.Sprintf("its security (%s) is none Aeolus has a name for: it cannot be imported", printableText(encryption, 32))}
	}
	ask.Security = sec
	// What only the AP knows has a stand-in here: the change is tried, and
	// nothing of it is kept.
	told := toldNetwork{Section: ask.Tell, SSID: ssid, Options: map[string]json.RawMessage{"encryption": mustJSON(encryption)}}
	switch {
	case needsPassphrase[sec]:
		told.Options["key"] = mustJSON("a-stand-in-for-its-key")
	case enterprise[sec]:
		told.Options["auth_server"], told.Options["auth_secret"] = mustJSON("192.0.2.1"), mustJSON("a-stand-in-for-its-secret")
	}
	values, _, err := importValues(ask, told)
	if err != nil {
		return ask, badRequest("import-network: %v", err)
	}
	if _, err := s.importReady(string(c.actor), c.state, ask, values); err != nil {
		return ask, err
	}
	return ask, nil
}

var bandsOK = map[string]bool{"2g": true, "5g": true, "6g": true}

func mustJSON(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}

// importNetwork makes the network, once the AP has said it: one change,
// under the name of whoever asked. What the AP said is held to what was
// asked, and the change tried again as it now is, before it is made. It
// says whether the network was made, and what came of it in plain words:
// the change it is, and the options of the AP's that had no place in
// Aeolus. What the AP said is not kept.
func (s *Server) importNetwork(ap hierarchy.NodeID, a conditions.Action, detail json.RawMessage) (bool, string) {
	var ask importAsk
	var told toldNetwork
	if json.Unmarshal(a.Params, &ask) != nil || json.Unmarshal(detail, &told) != nil {
		return false, "what the AP said of the network could not be read"
	}
	if told.SSID != ask.SSID || told.Section != ask.Tell {
		return false, "the AP said another network than the one asked for: nothing was made"
	}
	no := func(err error) (bool, string) { return false, "not imported: " + printableText(err.Error(), 280) }
	values, left, err := importValues(ask, told)
	if err != nil {
		return no(err)
	}
	state := s.log.Snapshot()
	if state == nil {
		return false, "not imported: the manager has no state to make it in"
	}
	op, err := s.importReady(a.Actor, state, ask, values)
	if err != nil {
		return no(err)
	}
	name := string(ap)
	if n, ok := state.Org.Locations.Node(ap); ok {
		name = n.Name
	}
	e, err := s.log.Commit(a.Actor, fmt.Sprintf("Imported from %s, which had it as a network of its own", name), op)
	if err != nil {
		return no(err)
	}
	slog.Info("network imported", "ap", ap, "network", ask.Network, "folder", ask.Folder, "by", a.Actor, "change", e.Seq)
	// What was left behind, as many as there is room to name, and how
	// many more there are.
	result := fmt.Sprintf("made Aeolus's own as %s, in change %d", ask.Network, e.Seq)
	for i, k := range left {
		sep, rest := ", ", fmt.Sprintf(" and %d more", len(left)-i)
		if i == 0 {
			sep = "; not carried over: "
		}
		if len(result)+len(sep)+len(k)+len(rest) > 300 {
			result += rest
			break
		}
		result += sep + k
	}
	return true, printableText(result, 300)
}
