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
// The passphrase is in what the AP answers and in the change, where it is
// sealed as any passphrase is (0027). It is in no report, is not kept with
// the action, and is never logged.

// importAsk is what an import was asked with, kept with its action until
// the AP answers.
type importAsk struct {
	SSID     string   `json:"ssid"`
	Sections []string `json:"sections"`
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

// importValues is the network Aeolus makes of one an AP said: its fields,
// by their paths below network.<id>., and the options of the AP's that
// have no place among them, by name alone, for whoever imports to see what
// was left behind. It fails where the network cannot be Aeolus's at all:
// a security Aeolus has no name for, or a key that is no passphrase.
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
	on := func(k string) bool { return get(k) == "1" }
	number := func(k string, lo, hi int) (int, bool) {
		n, err := strconv.Atoi(get(k))
		return n, err == nil && n >= lo && n <= hi
	}
	var left []string

	sec, ok := aeolusSecurity(get("encryption"))
	if !ok {
		return nil, nil, fmt.Errorf("its security (%s) is none Aeolus has a name for", printableText(get("encryption"), 32))
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
		for opt, field := range map[string]string{"auth_port": "radius.auth_port", "port": "radius.auth_port", "acct_port": "radius.acct_port", "dae_port": "radius.das.port"} {
			if n, ok := number(opt, 1, 65535); ok {
				v[field] = n
			}
		}
		for opt, field := range map[string]string{"acct_server": "radius.acct_server", "acct_secret": "radius.acct_secret", "nasid": "radius.nas_id", "dae_client": "radius.das.client", "dae_secret": "radius.das.secret"} {
			if s := get(opt); s != "" {
				v[field] = s
			}
		}
	}
	// A key on a network that takes none is not carried, and not named
	// beyond that it was there.
	used["key"] = true
	for opt, field := range map[string]string{"hidden": "hidden", "isolate": "isolation", "ieee80211r": "roaming.ft", "ieee80211k": "roaming.rrm", "bss_transition": "roaming.btm"} {
		if on(opt) {
			v[field] = true
		}
	}
	if on("disabled") {
		v["enabled"] = false
	}
	for opt, field := range map[string]string{"maxassoc": "max_clients", "dtim_period": "dtim"} {
		if get(opt) == "" {
			continue
		}
		if n, ok := number(opt, 1, 2007); ok {
			v[field] = n
		} else {
			left = append(left, opt)
		}
	}
	for _, opt := range []string{"multicast_to_unicast", "multicast_to_unicast_all"} {
		if s := get(opt); s == "1" || s == "0" {
			v["multicast_to_unicast"] = s == "1"
		}
	}
	// The clients it turns away, by MAC (0100). A list of the only clients
	// it lets on has no place in Aeolus.
	macs := list("maclist")
	switch get("macfilter") {
	case "deny":
		var blocked []string
		for _, m := range macs {
			if macRE.MatchString(strings.ToLower(m)) && !slices.Contains(blocked, strings.ToLower(m)) {
				blocked = append(blocked, strings.ToLower(m))
			}
		}
		if len(blocked) > 0 {
			v["blocked"] = blocked
		}
	case "allow":
		left = append(left, "macfilter (a list of the only clients let on)")
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

// importFor is the asking for an AP's own network to be made Aeolus's:
// the sections and SSID are what the AP last said it has, as for a taking
// out; its security is one Aeolus has; the folder and ID are free; and the
// change it will be, tried now with a stand-in for what only the AP knows,
// is one the asker may make and no AP's config would be held for.
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
	encryption := ""
	for _, o := range rep.Others {
		if o.SSID == ssid && slices.Contains(ask.Sections, o.Section) {
			encryption = o.Security
			break
		}
	}
	sec, ok := aeolusSecurity(encryption)
	if !ok {
		return ask, &apiError{http.StatusConflict, fmt.Sprintf("its security (%s) is none Aeolus has a name for: it cannot be imported", printableText(encryption, 32))}
	}
	t := c.state.Org.Services
	if n, ok := t.Node(hierarchy.NodeID(folder)); !ok || n.Kind == hierarchy.KindAP {
		return ask, badRequest("import-network: folder, the Services folder the network goes in")
	}
	if _, has := t.Resolve(hierarchy.NodeID(folder), hierarchy.Path("network."+id+".ssid")); has {
		return ask, &apiError{http.StatusConflict, fmt.Sprintf("there is a network %s there already: give this one another ID", id)}
	}
	// What only the AP knows has a stand-in here: the change is tried, and
	// nothing of it is kept.
	told := toldNetwork{SSID: ssid, Options: map[string]json.RawMessage{"encryption": mustJSON(encryption)}}
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
	op, err := importOp(ask, values)
	if err != nil {
		return ask, err
	}
	if op, err = s.prepare(op); err != nil {
		return ask, err
	}
	state, _, changed, err := s.log.Preview(string(c.actor), op)
	if err != nil {
		return ask, err
	}
	for held, problems := range s.checks(state, changed) {
		name := string(held)
		if n, ok := state.Org.Locations.Node(held); ok {
			name = n.Name
		}
		return ask, &apiError{http.StatusConflict, fmt.Sprintf("Aeolus would hold %s's config with this network: %s", name, problems[0])}
	}
	return ask, nil
}

var bandsOK = map[string]bool{"2g": true, "5g": true, "6g": true}

func mustJSON(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}

// importNetwork makes the network, once the AP has said it: one change,
// under the name of whoever asked. It says whether it was made, and what
// came of it in plain words: the change it is, and the options of the AP's
// that had no place in Aeolus. What the AP said is not kept.
func (s *Server) importNetwork(ap hierarchy.NodeID, a conditions.Action, detail json.RawMessage) (bool, string) {
	var ask importAsk
	var told toldNetwork
	if json.Unmarshal(a.Params, &ask) != nil || json.Unmarshal(detail, &told) != nil {
		return false, "what the AP said of the network could not be read"
	}
	if told.SSID != ask.SSID || !slices.Contains(ask.Sections, told.Section) {
		return false, "the AP said another network than the one asked for: nothing was made"
	}
	values, left, err := importValues(ask, told)
	if err != nil {
		return false, "not imported: " + printableText(err.Error(), 240)
	}
	op, err := importOp(ask, values)
	if err == nil {
		op, err = s.prepare(op)
	}
	if err != nil {
		return false, "not imported: " + printableText(err.Error(), 240)
	}
	name := string(ap)
	if state := s.log.Snapshot(); state != nil {
		if n, ok := state.Org.Locations.Node(ap); ok {
			name = n.Name
		}
	}
	e, err := s.log.Commit(a.Actor, fmt.Sprintf("Imported from %s, which had it as a network of its own", name), op)
	if err != nil {
		return false, "not imported: " + printableText(err.Error(), 240)
	}
	slog.Info("network imported", "ap", ap, "network", ask.Network, "folder", ask.Folder, "by", a.Actor, "change", e.Seq)
	result := fmt.Sprintf("made Aeolus's own as %s, in change %d", ask.Network, e.Seq)
	if len(left) > 0 {
		result += "; not carried over: " + strings.Join(left, ", ")
	}
	return true, printableText(result, 300)
}
