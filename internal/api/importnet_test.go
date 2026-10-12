package api

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// Making an AP's own network Aeolus's (0137): asked as an action, with
// where the network goes; the AP answers with the network as it has it,
// key and all; the manager makes the network then, in one change, under
// the asker's name. The key is sealed in the change, and is nowhere else.
func TestImportNetwork(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	own := func(section, radio, band, ssid, security string) map[string]any {
		return map[string]any{"section": section, "radio": radio, "band": band, "ssid": ssid, "security": security, "vlan": 20, "clients": 0}
	}
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "others": []any{
		own("default_radio1", "radio1", "5g", "Sweet Spot", "psk2"),
		own("wifinet2", "radio0", "2g", "Sweet Spot", "psk2"),
		own("old", "radio0", "2g", "Old net", "psk-mixed"),
		own("spare", "radio0", "2g", "Spare", "psk2"),
		own("short", "radio0", "2g", "Short", "psk2"),
	}}, nil); code != 200 {
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
	} {
		if code, body := ask("griff", c.body); code != c.code {
			t.Errorf("%s: %d %v, want %d", what, code, body, c.code)
		}
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
	_, page := f.do("GET", "/v1/trees/services/nodes/household", "griff", nil)
	if _, made := page["fields"].(map[string]any)["network.sweet-spot.ssid"]; made {
		t.Fatalf("the network is there before the AP answered")
	}
	_, _, got := f.apDo("GET", "/v1/ap/actions", token, nil, nil)
	handed := got["actions"].([]any)[0].(map[string]any)
	p := handed["params"].(map[string]any)
	if handed["kind"] != "import-network" || p["ssid"] != "Sweet Spot" || len(p["sections"].([]any)) != 2 {
		t.Errorf("what the AP is handed: %v", handed)
	}
	// The AP says the network as it has it.
	told := func(section, ssid string, options map[string]any) map[string]any {
		return map[string]any{"ok": true, "result": "said its settings", "detail": map[string]any{"section": section, "ssid": ssid, "vlan": 20, "options": options}}
	}
	code, _, done := f.apDo("POST", "/v1/ap/actions/"+id, token, told("default_radio1", "Sweet Spot", map[string]any{
		"device": "radio1", "network": "wifi_trusted", "mode": "ap", "ssid": "Sweet Spot", "encryption": "psk2+ccmp", "key": secret,
		"ieee80211r": "1", "ft_over_ds": "0", "ft_psk_generate_local": "1", "isolate": "1", "macfilter": "deny", "maclist": []string{"F0:AD:4E:2E:3A:1F"},
		"wds": "1", "ieee80211w": "1", "maxassoc": "40",
	}), nil)
	if code != 200 {
		t.Fatalf("the AP's answer: %d %v", code, done)
	}
	a := done["action"].(map[string]any)
	if a["state"] != "done" || !strings.HasPrefix(a["result"].(string), "made Aeolus's own as sweet-spot, in change ") || !strings.HasSuffix(a["result"].(string), "; not carried over: ieee80211w, wds") {
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
		"transport.primary.type": "vlan", "transport.primary.vlan": float64(20), "max_clients": float64(40),
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
	if net == nil {
		t.Logf("the AP's config: %v", cfg)
	}
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

	// What an AP may answer that makes nothing: nothing of the network;
	// another network than the one asked for; a key that is no passphrase.
	again := func(ssid, section, network string) string {
		t.Helper()
		code, body := ask("griff", map[string]any{"ssid": ssid, "sections": []string{section}, "folder": "household", "network": network, "vlan": 20})
		if code != 200 {
			t.Fatalf("asked for %s: %d %v", ssid, code, body)
		}
		_, _, _ = f.apDo("GET", "/v1/ap/actions", token, nil, nil)
		return strconv.FormatInt(int64(body["action"].(map[string]any)["id"].(float64)), 10)
	}
	for what, c := range map[string]struct {
		ssid, section, network string
		answer                 map[string]any
		says                   string
	}{
		"nothing of the network": {"Spare", "spare", "spare-a", map[string]any{"ok": true, "result": "said its settings"}, "the AP said nothing of the network"},
		"another network":        {"Short", "short", "short-a", told("spare", "Spare", map[string]any{"ssid": "Spare", "encryption": "psk2", "key": "long-enough-key"}), "another network than the one asked for"},
	} {
		id := again(c.ssid, c.section, c.network)
		_, _, done := f.apDo("POST", "/v1/ap/actions/"+id, token, c.answer, nil)
		a, _ := done["action"].(map[string]any)
		if a["state"] != "failed" || !strings.Contains(a["result"].(string), c.says) {
			t.Errorf("%s: %v", what, done)
		}
	}
	id = again("Short", "short", "short-b")
	_, _, done = f.apDo("POST", "/v1/ap/actions/"+id, token, told("short", "Short", map[string]any{"ssid": "Short", "encryption": "psk2", "key": "tiny"}), nil)
	if a, _ := done["action"].(map[string]any); a["state"] != "failed" || !strings.Contains(a["result"].(string), "no passphrase of 8 to 63 characters") {
		t.Errorf("a key that is no passphrase: %v", done)
	}
	_, page = f.do("GET", "/v1/trees/services/nodes/household", "griff", nil)
	for k := range page["fields"].(map[string]any) {
		if strings.HasPrefix(k, "network.spare-a.") || strings.HasPrefix(k, "network.short-") {
			t.Errorf("a network was made of an answer that made none: %s", k)
		}
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
	v, left, err := importValues(ask, toldNetwork{SSID: "Corp", Options: opt(map[string]any{
		"encryption": "wpa3-mixed", "auth_server": "10.0.0.5", "auth_port": "1645", "auth_secret": "radius-secret", "acct_server": "10.0.0.6", "acct_secret": "acct-secret",
		"nasid": "office-ap", "dae_client": "10.0.0.5", "dae_secret": "das-secret", "dae_port": "3799", "hidden": "1", "ieee80211k": "1", "bss_transition": "1",
		"multicast_to_unicast": "0", "dtim_period": "3", "macfilter": "allow", "maclist": []string{"02:00:00:00:00:01"}, "disabled": "1", "wpa_group_rekey": "3600", "bad name!": "x",
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
			t.Errorf("%s = %v, want %v", k, v[k], want)
		}
	}
	if _, has := v["blocked"]; has {
		t.Errorf("a list of the only clients let on was made a list of those kept off: %v", v["blocked"])
	}
	if _, has := v["passphrase"]; has {
		t.Errorf("a network that signs clients in was given a passphrase")
	}
	if got, want := strings.Join(left, "; "), "macfilter (a list of the only clients let on); wpa_group_rekey"; got != want {
		t.Errorf("left behind: %s, want %s", got, want)
	}
	// What cannot be Aeolus's at all.
	for what, o := range map[string]map[string]any{
		"WPA alone":                 {"encryption": "psk", "key": "long-enough-key"},
		"a key of 64 digits":        {"encryption": "psk2", "key": strings.Repeat("a", 64)},
		"no key":                    {"encryption": "sae"},
		"signing in, and no server": {"encryption": "wpa2"},
	} {
		if _, _, err := importValues(ask, toldNetwork{SSID: "X", Options: opt(o)}); err == nil {
			t.Errorf("%s: made a network", what)
		}
	}
	// An open network has no key to carry, and one it has by mistake is
	// neither carried nor named.
	v, left, err = importValues(ask, toldNetwork{SSID: "Open", Options: opt(map[string]any{"encryption": "none", "key": "left-over"})})
	if err != nil || v["security"] != "open" || v["passphrase"] != nil || len(left) != 0 {
		t.Errorf("an open network: %v, left %v, %v", v, left, err)
	}
}
