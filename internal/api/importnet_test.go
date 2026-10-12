package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// Making an AP's own network Aeolus's (0137): asked as an action, with
// where the network goes; the AP answers with the network as it has it,
// key and all; the manager holds the answer to what was asked, tries the
// change again as it now is, and makes the network, in one change, under
// the asker's name. The key is sealed in the change, and is nowhere else.
func TestImportNetwork(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	own := func(section, radio, band, ssid, security string) map[string]any {
		return map[string]any{"section": section, "radio": radio, "band": band, "ssid": ssid, "security": security, "vlan": 20, "clients": 0}
	}
	others := []any{
		own("default_radio1", "radio1", "5g", "Sweet Spot", "psk2"),
		own("wifinet2", "radio0", "2g", "Sweet Spot", "psk2"),
		own("old", "radio0", "2g", "Old net", "psk-mixed"),
		// On 6 GHz a WPA2/WPA3 network is WPA3 alone: one network all the same.
		own("tri6", "radio2", "6g", "Tri", "sae"),
		own("tri5", "radio1", "5g", "Tri", "sae-mixed"),
	}
	// Networks to try an answer on, one each.
	for i := 0; i < 16; i++ {
		others = append(others, own(fmt.Sprintf("try%d", i), "radio0", "2g", fmt.Sprintf("Try %d", i), "psk2"))
	}
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "others": others}, nil); code != 200 {
		t.Fatalf("its report: %d %v", code, b)
	}
	const secret = "the-house-passphrase"
	ask := func(as string, b map[string]any) (int, map[string]any) {
		t.Helper()
		b["kind"] = "import-network"
		return f.do("POST", "/v1/aps/"+ap+"/actions", as, b)
	}
	good := map[string]any{"ssid": "Sweet Spot", "sections": []string{"wifinet2", "default_radio1"}, "folder": "household", "network": "sweet-spot", "bands": []string{"2g", "5g"}, "vlan": 20}
	with := func(k string, v any) map[string]any {
		b := map[string]any{}
		for key, val := range good {
			b[key] = val
		}
		if v == nil {
			delete(b, k)
		} else {
			b[k] = v
		}
		return b
	}
	// What is asked must be something that can be made.
	for what, c := range map[string]struct {
		body map[string]any
		code int
	}{
		"no VLAN":                             {with("vlan", nil), 400},
		"a VLAN there is none of":             {with("vlan", 5000), 400},
		"a band there is none of":             {with("bands", []string{"60g"}), 400},
		"no folder":                           {with("folder", nil), 400},
		"an ID no network may have":           {with("network", "Sweet Spot"), 400},
		"an ID a network there has":           {with("network", "sweet"), 409},
		"a section it has not":                {with("sections", []string{"nosuch"}), 409},
		"a security Aeolus has no name for":   {map[string]any{"ssid": "Old net", "sections": []string{"old"}, "folder": "household", "network": "old-net", "vlan": 20}, 409},
		"a client test's words, which it has": {with("band", "5g"), 400},
		"WPA2 on 6 GHz, which would be held":  {with("bands", []string{"6g"}), 409},
	} {
		if code, body := ask("griff", c.body); code != c.code {
			t.Errorf("%s: %d %v, want %d", what, code, body, c.code)
		}
	}
	// An ID a folder below has is one in use: imported above, it would
	// reach down into that network.
	if code, body := ask("griff", map[string]any{"ssid": "Sweet Spot", "sections": []string{"wifinet2"}, "folder": "symtus", "network": "sweet", "vlan": 20}); code != 409 {
		t.Errorf("an ID a folder below has: %d %v", code, body)
	}
	// One who may not act on the AP is not told it is there.
	if code, _ := ask("office", with("network", "theirs")); code != 404 {
		t.Errorf("one with no role on the AP: %d", code)
	}

	// Asked: nothing is made until the AP answers.
	code, body := ask("griff", good)
	if code != 200 {
		t.Fatalf("asked: %d %v", code, body)
	}
	id := strconv.FormatInt(int64(body["action"].(map[string]any)["id"].(float64)), 10)
	// Asked again another way while that waits: told so, and not that it
	// was asked.
	if code, body := ask("griff", with("network", "sweet-two")); code != 409 {
		t.Errorf("asked again under another ID while one waits: %d %v", code, body)
	}
	if code, body := ask("griff", with("vlan", 30)); code != 409 {
		t.Errorf("asked again on another VLAN while one waits: %d %v", code, body)
	}
	if code, again := ask("griff", good); code != 200 || again["action"].(map[string]any)["id"] != body["action"].(map[string]any)["id"] {
		t.Errorf("the same asked again: %d %v", code, again)
	}
	_, page := f.do("GET", "/v1/trees/services/nodes/household", "griff", nil)
	if _, made := page["fields"].(map[string]any)["network.sweet-spot.ssid"]; made {
		t.Fatalf("the network is there before the AP answered")
	}
	_, _, got := f.apDo("GET", "/v1/ap/actions", token, nil, nil)
	handed := got["actions"].([]any)[0].(map[string]any)
	p := handed["params"].(map[string]any)
	if handed["kind"] != "import-network" || p["ssid"] != "Sweet Spot" || len(p["sections"].([]any)) != 2 || p["tell"] != "default_radio1" || p["security"] != "wpa2-psk" {
		t.Errorf("what the AP is handed: %v", handed)
	}
	// The AP says the network as it has it.
	told := func(section, ssid string, options map[string]any) map[string]any {
		return map[string]any{"ok": true, "result": "said its settings", "detail": map[string]any{"section": section, "ssid": ssid, "vlan": 20, "options": options}}
	}
	code, _, done := f.apDo("POST", "/v1/ap/actions/"+id, token, told("default_radio1", "Sweet Spot", map[string]any{
		"device": "radio1", "network": "wifi_trusted", "mode": "ap", "ssid": "Sweet Spot", "encryption": "psk2+ccmp", "key": secret,
		"ieee80211r": "1", "ft_over_ds": "0", "ft_psk_generate_local": "1", "isolate": "true", "hidden": "on", "macfilter": "deny", "maclist": []string{"F0:AD:4E:2E:3A:1F"},
		"ieee80211w": "1", "maxassoc": "40", "dtim_period": "300", "wps_pushbutton": "1",
	}), nil)
	if code != 200 {
		t.Fatalf("the AP's answer: %d %v", code, done)
	}
	a := done["action"].(map[string]any)
	if a["state"] != "done" || !strings.HasPrefix(a["result"].(string), "made Aeolus's own as sweet-spot, in change ") || !strings.HasSuffix(a["result"].(string), "; not carried over: dtim_period, ieee80211w, wps_pushbutton") {
		t.Errorf("what came of it: %v", a)
	}
	if a["detail"] != nil {
		t.Errorf("what the AP said is kept with the action: %v", a["detail"])
	}
	// The network, as made: every field that has a place.
	_, page = f.do("GET", "/v1/trees/services/nodes/household", "griff", nil)
	fields := page["fields"].(map[string]any)
	val := func(k string) any {
		r, _ := fields["network.sweet-spot."+k].(map[string]any)
		return r["value"]
	}
	for k, want := range map[string]any{
		"ssid": "Sweet Spot", "security": "wpa2-psk", "replace_own": true, "roaming.ft": true, "isolation": true,
		"transport.primary.type": "vlan", "transport.primary.vlan": float64(20), "max_clients": float64(40), "dtim": nil,
		// hidden 'on' is not on, as OpenWrt reads a Wi-Fi option: the AP
		// says the network's name, and so does Aeolus's.
		"hidden": nil,
	} {
		if val(k) != want {
			t.Errorf("network.sweet-spot.%s = %v, want %v", k, val(k), want)
		}
	}
	if got, _ := json.Marshal(val("blocked")); string(got) != `["f0:ad:4e:2e:3a:1f"]` {
		t.Errorf("blocked = %s", got)
	}
	if got, _ := json.Marshal(val("bands")); string(got) != `["2g","5g"]` {
		t.Errorf("bands = %s", got)
	}
	// Its key: with the AP, in its config, and with no one who reads.
	_, _, cfg := f.apDo("GET", "/v1/ap/config", token, nil, nil)
	doc, _ := cfg["config"].(map[string]any)
	nets, _ := doc["network"].(map[string]any)
	net, _ := nets["sweet-spot"].(map[string]any)
	if net["passphrase"] != secret || net["replace_own"] != true {
		t.Errorf("the AP's config has the network as %v", net)
	}
	for what, path := range map[string]string{"the folder's page": "/v1/trees/services/nodes/household", "the AP's actions": "/v1/aps/" + ap + "/actions", "the changes": "/v1/changes"} {
		_, b := f.do("GET", path, "griff", nil)
		if raw, _ := json.Marshal(b); strings.Contains(string(raw), secret) {
			t.Errorf("%s shows the passphrase", what)
		}
	}
	_, log := f.do("GET", "/v1/changes", "griff", nil)
	if raw, _ := json.Marshal(log); !strings.Contains(string(raw), "Imported from") || !strings.Contains(string(raw), `"griff"`) {
		t.Errorf("the change is not in the log under the asker's name, with where it came from")
	}

	// Made, it is not made again: its own still shows on the AP until the
	// AP has applied the network and reported anew.
	if code, body := ask("griff", with("network", "sweet-again")); code != 409 || !strings.Contains(fmt.Sprint(body["error"]), "Aeolus's own already, as sweet-spot") {
		t.Errorf("asked again once it is made: %d %v", code, body)
	}
	// Asked as the security the form showed: another is refused.
	if code, body := ask("griff", map[string]any{"ssid": "Try 15", "sections": []string{"try15"}, "folder": "household", "network": "shown-other", "vlan": 20, "security": "wpa3-sae"}); code != 409 {
		t.Errorf("asked as WPA3, of a network the AP has as WPA2: %d %v", code, body)
	}

	// What an AP may answer that makes nothing, each tried on a network of
	// its own: the answer is held to what was asked.
	n := 0
	again := func(network string) (string, string, string) {
		t.Helper()
		ssid, section := fmt.Sprintf("Try %d", n), fmt.Sprintf("try%d", n)
		n++
		code, body := ask("griff", map[string]any{"ssid": ssid, "sections": []string{section}, "folder": "household", "network": network, "vlan": 20})
		if code != 200 {
			t.Fatalf("asked for %s: %d %v", ssid, code, body)
		}
		_, _, _ = f.apDo("GET", "/v1/ap/actions", token, nil, nil)
		return strconv.FormatInt(int64(body["action"].(map[string]any)["id"].(float64)), 10), ssid, section
	}
	psk := func(ssid string, more map[string]any) map[string]any {
		o := map[string]any{"ssid": ssid, "encryption": "psk2", "key": "long-enough-key"}
		for k, v := range more {
			o[k] = v
		}
		return o
	}
	for _, c := range []struct {
		what   string
		answer func(ssid, section string) map[string]any
		says   string
	}{
		{"nothing of the network", func(_, _ string) map[string]any { return map[string]any{"ok": true, "result": "said its settings"} }, "the AP said nothing of the network"},
		{"another network", func(_, _ string) map[string]any { return told("default_radio1", "Sweet Spot", psk("Sweet Spot", nil)) }, "another network than the one asked for"},
		{"another section of its own", func(ssid, _ string) map[string]any { return told("wifinet2", ssid, psk(ssid, nil)) }, "another network than the one asked for"},
		{"a key that is no passphrase", func(ssid, section string) map[string]any {
			return told(section, ssid, psk(ssid, map[string]any{"key": "tiny"}))
		}, "no passphrase of 8 to 63 characters"},
		{"open, where it was asked as WPA2", func(ssid, section string) map[string]any {
			return told(section, ssid, map[string]any{"ssid": ssid, "encryption": "none"})
		}, "the AP has it as open now, and it was asked for as wpa2-psk"},
		{"WPA3, where it was asked as WPA2", func(ssid, section string) map[string]any {
			return told(section, ssid, psk(ssid, map[string]any{"encryption": "sae"}))
		}, "the AP has it as wpa3-sae now"},
		{"a station's section", func(ssid, section string) map[string]any {
			return told(section, ssid, psk(ssid, map[string]any{"mode": "sta"}))
		}, "no access point's network"},
		{"another AP's uplink", func(ssid, section string) map[string]any {
			return told(section, ssid, psk(ssid, map[string]any{"wds": "1"}))
		}, "another AP may reach the network through it"},
		{"a list of the only clients let on", func(ssid, section string) map[string]any {
			return told(section, ssid, psk(ssid, map[string]any{"macfilter": "allow", "maclist": []string{"02:00:00:00:00:01"}}))
		}, "it lets on only the clients in a list"},
		{"passphrases of its devices' own", func(ssid, section string) map[string]any {
			return told(section, ssid, psk(ssid, map[string]any{"wpa_psk_file": "/etc/hostapd.psk"}))
		}, "passphrases of their own, in a file"},
		{"sections of its devices' own that hang on it", func(ssid, section string) map[string]any {
			a := told(section, ssid, psk(ssid, nil))
			a["detail"].(map[string]any)["stations"] = 2
			return a
		}, "in sections that go with it"},
	} {
		id, ssid, section := again(fmt.Sprintf("try-%d", n))
		_, _, done := f.apDo("POST", "/v1/ap/actions/"+id, token, c.answer(ssid, section), nil)
		a, _ := done["action"].(map[string]any)
		if a["state"] != "failed" || !strings.Contains(fmt.Sprint(a["result"]), c.says) {
			t.Errorf("%s: %v", c.what, done)
		}
	}
	// An ID taken between the asking and the answer is not written over.
	id, ssid, section := again("taken-since")
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "services", "node": "household", "values": map[string]any{
		"network.taken-since.ssid": "Someone else's", "network.taken-since.security": "open", "network.taken-since.hidden": true,
		"network.taken-since.transport.primary.type": "vlan", "network.taken-since.transport.primary.vlan": 30}}); code != 200 {
		t.Fatalf("a network made in between: %d %v", code, b)
	}
	_, _, done = f.apDo("POST", "/v1/ap/actions/"+id, token, told(section, ssid, psk(ssid, nil)), nil)
	if a, _ := done["action"].(map[string]any); a["state"] != "failed" || !strings.Contains(fmt.Sprint(a["result"]), "there is a network taken-since there already") {
		t.Errorf("an ID taken since it was asked: %v", done)
	}
	_, page = f.do("GET", "/v1/trees/services/nodes/household", "griff", nil)
	fields = page["fields"].(map[string]any)
	if r, _ := fields["network.taken-since.ssid"].(map[string]any); r["value"] != "Someone else's" {
		t.Errorf("the network made in between was written over: %v", r)
	}
	for k := range fields {
		if strings.HasPrefix(k, "network.try-") {
			t.Errorf("a network was made of an answer that made none: %s", k)
		}
	}
	// More left behind than there is room to name: as many as fit, and how
	// many more.
	id, ssid, section = again("crowded")
	many := psk(ssid, nil)
	for i := 0; i < 40; i++ {
		many[fmt.Sprintf("some_option_of_its_own_%02d", i)] = "x"
	}
	_, _, done = f.apDo("POST", "/v1/ap/actions/"+id, token, told(section, ssid, many), nil)
	if a, _ := done["action"].(map[string]any); a["state"] != "done" || len(fmt.Sprint(a["result"])) > 300 || !strings.Contains(fmt.Sprint(a["result"]), " more") {
		t.Errorf("more left behind than fits: %v", done)
	}
	// WPA2/WPA3 with WPA3 alone on 6 GHz is one network: the section off
	// 6 GHz is the one asked for, and its security the network's.
	code, body = ask("griff", map[string]any{"ssid": "Tri", "sections": []string{"tri6", "tri5"}, "folder": "household", "network": "tri", "vlan": 20})
	if p, _ := body["action"].(map[string]any)["params"].(map[string]any); code != 200 || p["tell"] != "tri5" || p["security"] != "wpa2-wpa3" {
		t.Errorf("a network that is WPA3 alone on 6 GHz: %d %v", code, body)
	}
}

// The network Aeolus makes of what an AP said: each option that has a
// place, and by name those that have none.
func TestImportValues(t *testing.T) {
	opt := func(m map[string]any) map[string]json.RawMessage {
		out := map[string]json.RawMessage{}
		for k, v := range m {
			out[k], _ = json.Marshal(v)
		}
		return out
	}
	ask := importAsk{SSID: "Corp", Network: "corp", Folder: "household", VLAN: 30}
	for i := 0; i < 50; i++ { // whichever order the options come in
		v, left, err := importValues(ask, toldNetwork{SSID: "Corp", Options: opt(map[string]any{
			"encryption": "wpa3-mixed", "auth_server": "10.0.0.5", "auth_port": "1645", "port": "1812", "auth_secret": "radius-secret", "acct_server": "10.0.0.6", "acct_secret": "acct-secret",
			"nasid": "office-ap", "dae_client": "10.0.0.5", "dae_secret": "das-secret", "dae_port": "3799", "hidden": "true", "ieee80211k": "1", "bss_transition": "true", "ieee80211r": "1",
			"multicast_to_unicast": "0", "dtim_period": "3", "macfilter": "disable", "maclist": []string{"02:00:00:00:00:01"}, "disabled": "1", "wpa_group_rekey": "3600", "bad name!": "x",
		})})
		if err != nil {
			t.Fatal(err)
		}
		for k, want := range map[string]any{
			"security": "wpa2-wpa3-enterprise", "radius.auth_server": "10.0.0.5", "radius.auth_port": 1645, "radius.auth_secret": "radius-secret",
			"radius.acct_server": "10.0.0.6", "radius.acct_secret": "acct-secret", "radius.nas_id": "office-ap",
			"radius.das.client": "10.0.0.5", "radius.das.secret": "das-secret", "radius.das.port": 3799,
			"hidden": true, "roaming.rrm": true, "roaming.btm": true, "multicast_to_unicast": false, "dtim": 3, "enabled": false,
			"transport.primary.vlan": 30, "replace_own": true,
		} {
			if v[k] != want {
				t.Fatalf("%s = %v, want %v", k, v[k], want)
			}
		}
		if _, has := v["blocked"]; has {
			t.Fatalf("a list with no filter was made a list of clients kept off: %v", v["blocked"])
		}
		if _, has := v["passphrase"]; has {
			t.Fatalf("a network that signs clients in was given a passphrase")
		}
		// Fast transition on a network that signs clients in is not
		// rendered yet: left behind and named, and the network made.
		if _, has := v["roaming.ft"]; has {
			t.Fatalf("fast transition was carried onto a network that signs clients in")
		}
		if got, want := strings.Join(left, "; "), "ieee80211r; wpa_group_rekey"; got != want {
			t.Fatalf("left behind: %s, want %s", got, want)
		}
	}
	// On is 1 or true, as OpenWrt reads a Wi-Fi option, and nothing else.
	v, _, err := importValues(ask, toldNetwork{SSID: "Plain", Options: opt(map[string]any{"encryption": "psk2", "key": "long-enough-key",
		"hidden": "on", "isolate": "yes", "ieee80211k": "enabled", "disabled": "TRUE", "wds": "on", "multicast_to_unicast": "true"})})
	if err != nil || v["hidden"] != nil || v["isolation"] != nil || v["roaming.rrm"] != nil || v["enabled"] != nil || v["multicast_to_unicast"] != true {
		t.Errorf("options spelled on, yes, enabled and TRUE: %v, %v", v, err)
	}
	// What cannot be Aeolus's at all.
	for what, o := range map[string]map[string]any{
		"WPA alone":                       {"encryption": "psk", "key": "long-enough-key"},
		"a key of 64 digits":              {"encryption": "psk2", "key": strings.Repeat("a", 64)},
		"no key":                          {"encryption": "sae"},
		"signing in, and no server":       {"encryption": "wpa2"},
		"a mesh's":                        {"encryption": "sae", "key": "long-enough-key", "mode": "mesh"},
		"a VLAN for each client by file":  {"encryption": "psk2", "key": "long-enough-key", "vlan_file": "/etc/vlans"},
		"VLANs from a RADIUS server":      {"encryption": "psk2", "key": "long-enough-key", "dynamic_vlan": "2"},
		"MAC addresses asked of a server": {"encryption": "psk2", "key": "long-enough-key", "macfilter": "radius"},
		"an open network beside it":       {"encryption": "owe", "owe_transition": "1"},
		"WPA3 passwords in a file":        {"encryption": "sae", "key": "long-enough-key", "sae_password_file": "/etc/sae"},
		"a backhaul":                      {"encryption": "psk2", "key": "long-enough-key", "multi_ap": "1"},
	} {
		if _, _, err := importValues(ask, toldNetwork{SSID: "X", Options: opt(o)}); err == nil {
			t.Errorf("%s: made a network", what)
		}
	}
	// Asked as one security, said as another: refused, whichever way.
	if _, _, err := importValues(importAsk{SSID: "X", Security: "wpa2-psk", VLAN: 20}, toldNetwork{SSID: "X", Options: opt(map[string]any{"encryption": "none"})}); err == nil {
		t.Errorf("asked as WPA2 and said as open: made a network")
	}
	// An open network has no key to carry, and one it has by mistake is
	// neither carried nor named.
	var left []string
	v, left, err = importValues(ask, toldNetwork{SSID: "Open", Options: opt(map[string]any{"encryption": "none", "key": "left-over"})})
	if err != nil || v["security"] != "open" || v["passphrase"] != nil || len(left) != 0 {
		t.Errorf("an open network: %v, left %v, %v", v, left, err)
	}
}
