package rendercheck

import (
	"encoding/json"
	"path"
	"strings"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/schema"
	"github.com/ChristopherGriffin/aeolus/internal/uci"
)

const pass = "sweet-spot-passphrase-42"

// intent is a composed config as compose.AP builds it, secrets opened. The
// AP has no 6 GHz radio, so the 6g settings do not apply to it.
const intent = `{
	"radio": {
		"2g": {"channel": 6, "width": 20},
		"5g": {"enabled": true, "channel": "auto", "width": 80, "power": 20},
		"6g": {"width": 160}
	},
	"system": {"country": "US", "tz": "America/Chicago", "ntp": ["0.pool.ntp.org", "1.pool.ntp.org"], "syslog": "192.168.20.50:514", "poll": 60},
	"network": {
		"sweet": {"ssid": "Sweet Spot", "security": "wpa2-psk", "passphrase": "` + pass + `", "roaming": {"ft": true},
			"transport": {"primary": {"type": "vxlan", "concentrator": "homelab", "vni": 20}, "fallback": {"type": "vlan", "vlan": 20}}},
		"sweet-iot": {"ssid": "Sweet_Spot_IoT", "security": "wpa2-psk", "passphrase": "` + pass + `", "bands": ["2g"], "hidden": true, "isolation": true,
			"transport": {"primary": {"type": "vlan", "vlan": 30}}},
		"old": {"enabled": false, "ssid": "Old", "security": "open", "transport": {"primary": {"type": "vlan", "vlan": 40}}}
	},
	"concentrators": {"homelab": {"address": "1.1.1.2", "port": 4789, "mtu": 1450}}
}`

// rendered carries the intent, plus a section Aeolus did not make.
const rendered = `package wireless

config wifi-device 'radio0'
	option band '2g'
	option channel '6'
	option htmode 'HE20'
	option country 'US'

config wifi-device 'radio1'
	option band '5g'
	option channel 'auto'
	option htmode 'VHT80'
	option txpower '20'
	option country 'US'

config wifi-iface 'aeolus_sweet_radio0'
	option device 'radio0'
	option mode 'ap'
	option ssid 'Sweet Spot'
	option encryption 'psk2+ccmp'
	option key '` + pass + `'
	option ieee80211r '1'
	option network 'aeolus_sweet'

config wifi-iface 'aeolus_sweet_radio1'
	option device 'radio1'
	option mode 'ap'
	option ssid 'Sweet Spot'
	option encryption 'psk2'
	option key '` + pass + `'
	option ieee80211r '1'
	option network 'aeolus_sweet'

config wifi-iface 'aeolus_sweet_iot_radio0'
	option device 'radio0'
	option mode 'ap'
	option ssid 'Sweet_Spot_IoT'
	option encryption 'psk2'
	option key '` + pass + `'
	option hidden '1'
	option isolate '1'
	option network 'aeolus_iot'

config wifi-iface 'default_radio0'
	option device 'radio0'
	option ssid 'OpenWrt'

package network

config interface 'aeolus_sweet'
	option proto 'none'

config interface 'aeolus_iot'
	option proto 'none'

config device
	option type '8021q'
	option ifname 'lan'
	option vid '20'
	option name 'lan.20'

config bridge-vlan
	option device 'br-lan'
	option vlan '30'

config interface 'aeolus_sweet_vx'
	option proto 'vxlan'
	option peeraddr '1.1.1.2'
	option port '4789'
	option vid '20'
	option mtu '1450'

package system

config system
	option zonename 'America/Chicago'
	option log_ip '192.168.20.50'
	option log_port '514'

config timeserver 'ntp'
	list server '1.pool.ntp.org'
	list server '0.pool.ntp.org'

package aeolus

config agent 'agent'
	option url 'https://aeolus.symtus.com:8443'
	option uplink 'wan'
	option poll '60'
`

func check(t *testing.T, text string) []string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(intent), &doc); err != nil {
		t.Fatal(err)
	}
	c, err := uci.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	return Check(doc, c)
}

func TestRenderedIntentPasses(t *testing.T) {
	if p := check(t, rendered); len(p) != 0 {
		t.Fatalf("problems: %v", p)
	}
}

func TestEachRuleCatchesItsMistake(t *testing.T) {
	cases := []struct {
		name, old, new, want string
	}{
		{"channel", "option channel 'auto'", "option channel '36'", "wireless.radio1: channel is \"36\""},
		{"width", "option htmode 'VHT80'", "option htmode 'HE40'", "want a 80 MHz mode"},
		{"power", "option txpower '20'\n", "", "txpower is missing"},
		{"enabled", "option txpower '20'", "option txpower '20'\n\toption disabled '1'", "radio.5g.enabled is true"},
		{"country", "option htmode 'HE20'\n\toption country 'US'", "option htmode 'HE20'", "wireless.radio0: country is missing"},
		{"missing iface", "config wifi-iface 'aeolus_sweet_iot_radio0'", "config wifi-iface 'something_else'", "no wifi-iface aeolus_sweet_iot_radio0 on radio0"},
		{"ssid", "option ssid 'Sweet_Spot_IoT'", "option ssid 'Sweet Spot IoT'", "ssid is \"Sweet Spot IoT\""},
		{"encryption", "option encryption 'psk2+ccmp'", "option encryption 'sae'", "encryption is \"sae\", want \"psk2\""},
		{"hidden", "option hidden '1'\n", "", "hidden is \"\", want on"},
		{"isolation", "option isolate '1'", "option isolate '0'", "isolate is \"0\", want on"},
		{"roaming", "option ieee80211r '1'\n\toption network 'aeolus_sweet'\n\nconfig wifi-iface 'aeolus_sweet_radio1'", "option network 'aeolus_sweet'\n\nconfig wifi-iface 'aeolus_sweet_radio1'", "aeolus_sweet_radio0: ieee80211r"},
		{"interface", "config interface 'aeolus_iot'", "config interface 'aeolus_things'", "network interface aeolus_iot does not exist"},
		{"device", "option device 'radio1'", "option device 'radio0'", "aeolus_sweet_radio1: device is \"radio0\""},
		{"stale", "config wifi-iface 'default_radio0'", "config wifi-iface 'aeolus_old_radio0'", "wireless.aeolus_old_radio0: no network calls for it"},
		{"vlan", "option vid '20'\n\toption name", "option vid '21'\n\toption name", "transport.fallback: VLAN 20 is not in the network config"},
		{"bridge vlan", "option vlan '30'", "option vlan '31'", "sweet-iot.transport.primary: VLAN 30"},
		{"vxlan", "option peeraddr '1.1.1.2'", "option peeraddr '1.1.1.3'", "no vxlan interface to 1.1.1.2 with VNI 20"},
		{"vxlan mtu", "option mtu '1450'", "option mtu '1500'", "aeolus_sweet_vx: mtu is \"1500\""},
		{"vxlan port", "option port '4789'", "option port '8472'", "port is \"8472\""},
		{"zonename", "option zonename 'America/Chicago'", "option zonename 'UTC'", "zonename is \"UTC\""},
		{"ntp", "\tlist server '0.pool.ntp.org'\n", "", "system.ntp: servers"},
		{"syslog", "option log_port '514'", "option log_port '515'", "log_port is \"515\""},
		{"no wireless", "package wireless", "package wifi", "package wireless is missing"},
		{"no network", "package network", "package net", "package network is missing"},
		{"poll", "option poll '60'", "option poll '30'", "aeolus.agent: poll is \"30\""},
		{"no agent", "config agent 'agent'", "config agent 'other'", "aeolus: no agent section"},
	}
	for _, c := range cases {
		if !strings.Contains(rendered, c.old) {
			t.Fatalf("%s: fixture lacks %q", c.name, c.old)
		}
		got := strings.Join(check(t, strings.Replace(rendered, c.old, c.new, 1)), "\n")
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %q in:\n%s", c.name, c.want, got)
		}
	}
}

func TestAWrongKeyIsNeverQuoted(t *testing.T) {
	got := strings.Join(check(t, strings.Replace(rendered, "option key '"+pass+"'", "option key 'wrong-passphrase-1'", 1)), "\n")
	if !strings.Contains(got, "key does not match the passphrase") {
		t.Fatalf("problems: %s", got)
	}
	if strings.Contains(got, pass) || strings.Contains(got, "wrong-passphrase-1") {
		t.Fatalf("a problem quotes a key: %s", got)
	}
}

// A 5 GHz width must fit the channel, even one Aeolus did not set.
func TestWidthMustFitTheChannel(t *testing.T) {
	c, err := uci.Parse(`package wireless
config wifi-device 'a'
	option band '5g'
	option channel '149'
	option htmode 'VHT160'
config wifi-device 'b'
	option band '5g'
	option channel '36'
	option htmode 'HE160'
config wifi-device 'c'
	option band '5g'
	option channel '165'
	option htmode 'VHT40'
config wifi-device 'd'
	option band '5g'
	option channel 'auto'
	option htmode 'VHT160'
config wifi-device 'e'
	option band '2g'
	option channel '13'
	option htmode 'HT40'
`)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(Check(map[string]any{}, c), "\n")
	for _, want := range []string{
		"wireless.a: channel 149 cannot use a 160 MHz width; that needs a channel from 36–64 or 100–128",
		"wireless.c: channel 165 cannot use a 40 MHz width",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in: %s", want, got)
		}
	}
	for _, ok := range []string{"wireless.b:", "wireless.d:", "wireless.e:"} {
		if strings.Contains(got, ok) {
			t.Errorf("%s should pass: %s", ok, got)
		}
	}
}

func TestPowerAutoMeansNoTxpower(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(`{"radio": {"5g": {"power": "auto"}}}`), &doc); err != nil {
		t.Fatal(err)
	}
	c, err := uci.Parse("package wireless\nconfig wifi-device 'radio1'\n\toption band '5g'\n\toption txpower '20'\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(Check(doc, c), "\n"); !strings.Contains(got, "want none for auto") {
		t.Fatalf("problems: %s", got)
	}
}

// Every schema field is either checked or deferred with a reason, and every
// entry names a real field (0039).
func TestCoverageMatchesTheSchema(t *testing.T) {
	sch, err := schema.V1()
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]bool{}
	for _, leaf := range sch.Leaves() {
		var by []string
		for pattern := range Coverage {
			if matches(pattern, leaf) {
				by = append(by, pattern)
				used[pattern] = true
			}
		}
		if len(by) != 1 {
			t.Errorf("%s is covered by %v, want exactly one entry", leaf, by)
		}
	}
	for pattern := range Coverage {
		if !used[pattern] {
			t.Errorf("coverage entry %s names no field", pattern)
		}
	}
}

func matches(pattern, leaf string) bool {
	p, l := strings.Split(pattern, "."), strings.Split(leaf, ".")
	if len(p) != len(l) {
		return false
	}
	for i := range p {
		if ok, _ := path.Match(p[i], l[i]); !ok && p[i] != l[i] {
			return false
		}
	}
	return true
}
