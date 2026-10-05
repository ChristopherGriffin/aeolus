package rendercheck

import (
	"encoding/json"
	"fmt"
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
		"sweet": {"ssid": "Sweet Spot", "security": "wpa2-psk", "passphrase": "` + pass + `", "roaming": {"ft": true}, "multicast_to_unicast": true,
			"transport": {"primary": {"type": "vxlan", "concentrator": "homelab", "vni": 20, "probe": "192.168.20.1"}}},
		"sweet-iot": {"ssid": "Sweet_Spot_IoT", "security": "wpa2-psk", "passphrase": "` + pass + `", "bands": ["2g"], "hidden": true, "isolation": true,
			"transport": {"primary": {"type": "vlan", "vlan": 30}}},
		"old": {"enabled": false, "ssid": "Old", "security": "open", "transport": {"primary": {"type": "vlan", "vlan": 40}}}
	},
	"concentrators": {"homelab": {"address": "1.1.1.2", "port": 4789, "mtu": 1450, "probe_interval": 20}}
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
	option multicast_to_unicast '1'
	option ieee80211r '1'
	option network 'aeolus_sweet'

config wifi-iface 'aeolus_sweet_radio1'
	option device 'radio1'
	option mode 'ap'
	option ssid 'Sweet Spot'
	option encryption 'psk2'
	option key '` + pass + `'
	option multicast_to_unicast '1'
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
	option network 'aeolus_sweet_iot'

config wifi-iface 'default_radio0'
	option device 'radio0'
	option ssid 'OpenWrt'

package network

config interface 'lan'
	option proto 'dhcp'
	option device 'br-lan.1'

config interface 'aeolus_sweet'
	option proto 'none'
	option device 'br-vx20'

config interface 'aeolus_sweet_iot'
	option proto 'none'
	option device 'br-lan.30'

config device
	option type '8021q'
	option ifname 'lan'
	option vid '20'
	option name 'lan.20'

config bridge-vlan
	option device 'br-lan'
	option vlan '30'

config interface 'aeolus_20'
	option proto 'vxlan'
	option peeraddr '1.1.1.2'
	option port '4789'
	option vid '20'
	option mtu '1450'
	option tunlink 'lan'

config device 'aeolus_20_br'
	option type 'bridge'
	option name 'br-vx20'
	option bridge_empty '1'
	list ports 'aeolus_20'

package firewall

config zone
	option name 'lan'
	list network 'lan'

config rule 'aeolus_vxlan_20'
	option name 'Aeolus VXLAN 20'
	option src 'lan'
	option proto 'udp'
	option src_ip '1.1.1.2'
	option dest_port '4789'
	option target 'ACCEPT'

config include 'aeolus_clamp'
	option type 'nftables'
	option path '/etc/aeolus/clamp.nft'
	option position 'ruleset-post'

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

config probe 'aeolus_20'
	option vni '20'
	option interval '20'
	list address '192.168.20.1'
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
		{"multicast to unicast", "option multicast_to_unicast '1'", "option multicast_to_unicast '0'", "aeolus_sweet_radio0: multicast_to_unicast is \"0\""},
		{"multicast default", "option isolate '1'", "option isolate '1'\n\toption multicast_to_unicast '1'", "multicast_to_unicast is \"1\", want none, for OpenWrt's default"},
		{"roaming", "option ieee80211r '1'\n\toption network 'aeolus_sweet'\n\nconfig wifi-iface 'aeolus_sweet_radio1'", "option network 'aeolus_sweet'\n\nconfig wifi-iface 'aeolus_sweet_radio1'", "aeolus_sweet_radio0: ieee80211r"},
		{"interface", "config interface 'aeolus_sweet_iot'", "config interface 'aeolus_things'", "network interface aeolus_sweet_iot does not exist"},
		{"no network interface", "config interface 'aeolus_sweet_iot'", "config interface 'aeolus_things'", "network.sweet-iot: no interface aeolus_sweet_iot"},
		{"vlan path", "option device 'br-lan.30'", "option device 'br-lan.31'", "interface aeolus_sweet_iot is on \"br-lan.31\", want the primary's VLAN 30"},
		{"tunnel path", "option device 'br-vx20'", "option device 'br-lan.20'", "interface aeolus_sweet is on \"br-lan.20\", want the primary's tunnel bridge br-vx20"},
		{"device", "option device 'radio1'", "option device 'radio0'", "aeolus_sweet_radio1: device is \"radio0\""},
		{"stale", "config wifi-iface 'default_radio0'", "config wifi-iface 'aeolus_old_radio0'", "wireless.aeolus_old_radio0: no network calls for it"},
		{"bridge vlan", "option vlan '30'", "option vlan '31'", "sweet-iot.transport.primary: VLAN 30"},
		{"no tunnel", "config interface 'aeolus_20'", "config interface 'aeolus_21'", "no tunnel aeolus_20 to 1.1.1.2 with VNI 20"},
		{"tunnel peer", "option peeraddr '1.1.1.2'", "option peeraddr '1.1.1.3'", "network.aeolus_20: peeraddr is \"1.1.1.3\", want \"1.1.1.2\""},
		{"tunnel proto", "option proto 'vxlan'", "option proto 'vxlan6'", "network.aeolus_20: proto is \"vxlan6\""},
		{"tunnel vni", "option vid '20'\n\toption mtu", "option vid '21'\n\toption mtu", "network.aeolus_20: vid is \"21\""},
		{"tunnel mtu", "option mtu '1450'", "option mtu '1500'", "aeolus_20: mtu is \"1500\""},
		{"uplink mtu", "option device 'br-lan.1'", "option device 'br-lan.1'\n\toption mtu '1499'", "an MTU of 1450 needs 1500 on the AP's uplink, lan (br-lan.1), which carries 1499"},
		{"tunnel port", "option port '4789'", "option port '8472'", "port is \"8472\""},
		{"tunlink", "\toption tunlink 'lan'\n", "", "aeolus_20: tunlink is missing"},
		{"tunlink names", "option tunlink 'lan'", "option tunlink 'wan'", "tunlink \"wan\" names no interface"},
		{"primary started", "option tunlink 'lan'", "option tunlink 'lan'\n\toption auto '0'", "the tunnel is not started"},
		{"tunnel bridge", "option name 'br-vx20'", "option name 'br-lan'", "network.aeolus_20_br: want a bridge br-vx20"},
		{"bridge carries", "list ports 'aeolus_20'", "list ports 'lan1'", "the bridge does not carry the tunnel aeolus_20"},
		{"firewall rule", "config rule 'aeolus_vxlan_20'", "config rule 'other'", "firewall.aeolus_vxlan_20: no rule letting the tunnel in from 1.1.1.2"},
		{"rule source", "option src_ip '1.1.1.2'", "option src_ip '0.0.0.0/0'", "firewall.aeolus_vxlan_20: src_ip is \"0.0.0.0/0\""},
		{"rule zone", "\toption src 'lan'\n", "", "firewall.aeolus_vxlan_20: src is missing"},
		{"clamp", "config include 'aeolus_clamp'", "config include 'other'", "MTU 1450 needs the MSS clamp"},
		{"no firewall", "package firewall", "package fire", "package firewall is missing"},
		{"zonename", "option zonename 'America/Chicago'", "option zonename 'UTC'", "zonename is \"UTC\""},
		{"ntp", "\tlist server '0.pool.ntp.org'\n", "", "system.ntp: servers"},
		{"syslog", "option log_port '514'", "option log_port '515'", "log_port is \"515\""},
		{"no wireless", "package wireless", "package wifi", "package wireless is missing"},
		{"no network", "package network", "package net", "package network is missing"},
		{"poll", "option poll '60'", "option poll '30'", "aeolus.agent: poll is \"30\""},
		{"no agent", "config agent 'agent'", "config agent 'other'", "aeolus: no agent section"},
		{"no probe", "config probe 'aeolus_20'", "config probe 'other'", "aeolus.aeolus_20: no probe section for the tunnel"},
		{"probe interval", "option interval '20'", "option interval '30'", "aeolus.aeolus_20: interval is \"30\", want \"20\""},
		{"probe address", "list address '192.168.20.1'", "list address '192.168.20.2'", "aeolus.aeolus_20: probe addresses are [192.168.20.2], want [192.168.20.1]"},
		{"stale probe", "config probe 'aeolus_20'", "config probe 'aeolus_99'\n\toption vni '99'\n\nconfig probe 'aeolus_20'", "aeolus.aeolus_99: no tunnel, tunnel port, network with a fallback or VLAN on the uplink calls for it"},
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

func TestBandSteering(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(strings.Replace(intent, `"roaming": {"ft": true},`, `"roaming": {"ft": true}, "band_steering": true,`, 1)), &doc); err != nil {
		t.Fatal(err)
	}
	check := func(text string) string {
		t.Helper()
		c, err := uci.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(Check(doc, c), "\n")
	}
	// Without usteer, and without 11k and 11v on the network.
	got := check(rendered)
	for _, want := range []string{
		"usteer: band steering needs usteer",
		"aeolus_sweet_radio0: ieee80211k is \"\", want on",
		"aeolus_sweet_radio0: bss_transition is \"\", want on",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	steered := strings.ReplaceAll(rendered, "\toption ieee80211r '1'\n", "\toption ieee80211r '1'\n\toption ieee80211k '1'\n\toption bss_transition '1'\n")
	const usteer = "\npackage usteer\n\nconfig usteer\n\toption network 'lan'\n\toption band_steering_interval '%s'\n%s"
	for _, c := range []struct {
		interval, list, want string
	}{
		{"30000", "\tlist ssid_list 'Sweet Spot'\n", ""},
		{"0", "\tlist ssid_list 'Sweet Spot'\n", "usteer: band_steering_interval is \"0\", want \"30000\""},
		{"30000", "", "usteer: ssid_list is [], want [\"Sweet Spot\"]"},
		{"30000", "\tlist ssid_list 'Sweet Spot'\n\tlist ssid_list 'Sweet_Spot_IoT'\n", "want [\"Sweet Spot\"]"},
	} {
		got := check(steered + fmt.Sprintf(usteer, c.interval, c.list))
		if c.want == "" && (strings.Contains(got, "usteer") || strings.Contains(got, "ieee80211k") || strings.Contains(got, "bss_transition")) {
			t.Errorf("steered right, yet: %s", got)
		}
		if c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("want %q in:\n%s", c.want, got)
		}
	}
	// With no network asking, usteer must steer nothing.
	var none map[string]any
	if err := json.Unmarshal([]byte(intent), &none); err != nil {
		t.Fatal(err)
	}
	c, err := uci.Parse(rendered + fmt.Sprintf(usteer, "30000", ""))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(Check(none, c), "\n"); !strings.Contains(got, "usteer: band_steering_interval is \"30000\", want \"0\"") {
		t.Errorf("usteer steering on its own: %s", got)
	}
}

func TestSNMP(t *testing.T) {
	const community = "secret-community"
	var on, off map[string]any
	if err := json.Unmarshal([]byte(`{"system": {"snmp": {"enabled": true, "community": "`+community+`", "location": "Pumphouse",
		"v3": {"user": "monitor", "auth": "auth-passphrase", "privacy": "privacy-passphrase"}}}}`), &on); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"system": {}}`), &off); err != nil {
		t.Fatal(err)
	}
	check := func(doc map[string]any, text string) string {
		t.Helper()
		c, err := uci.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, p := range Check(doc, c) {
			if strings.HasPrefix(p, "snmpd") {
				out = append(out, p)
			}
		}
		return strings.Join(out, "\n")
	}
	const good = `package snmpd
config agent 'agent'
	option agentaddress 'UDP:161,UDP6:161'
config com2sec 'aeolus_v2c'
	option secname 'ro'
	option source 'default'
	option community '` + community + `'
config com2sec6 'aeolus_v2c6'
	option secname 'ro'
	option source 'default'
	option community '` + community + `'
config access 'aeolus_ro'
	option group 'ro'
	option write 'none'
config system 'system'
	option sysLocation 'Pumphouse'
config v3 'aeolus_v3'
	option username 'monitor'
	option auth_type 'SHA'
	option auth_pass 'auth-passphrase'
	option privacy_type 'AES'
	option privacy_pass 'privacy-passphrase'
	option allow_write '0'
config snmpd 'general'
	option enabled '1'
	option snmp_version 'v1/v2c/v3'
`
	if got := check(on, good); got != "" {
		t.Fatalf("problems with the right config: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"off", "option enabled '1'", "option enabled '0'", `enabled is "0", want "1"`},
		{"community", "option community '" + community + "'\nconfig com2sec6", "option community 'public'\nconfig com2sec6", "communities do not match"},
		{"write", "option write 'none'", "option write 'all'", "write is \"all\", want none"},
		{"auth", "option auth_pass 'auth-passphrase'", "option auth_pass 'other-passphrase'", "auth_pass does not match"},
		{"privacy type", "option privacy_type 'AES'", "option privacy_type 'DES'", `privacy_type is "DES", want "AES"`},
		{"version", "option snmp_version 'v1/v2c/v3'", "option snmp_version 'v1/v2c'", "want one with v3"},
		{"location", "option sysLocation 'Pumphouse'", "option sysLocation 'office'", `sysLocation is "office"`},
	} {
		if !strings.Contains(good, c.old) {
			t.Fatalf("%s: fixture lacks %q", c.name, c.old)
		}
		got := check(on, strings.Replace(good, c.old, c.new, 1))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %q in:\n%s", c.name, c.want, got)
		}
		for _, secret := range []string{community, "auth-passphrase", "privacy-passphrase", "other-passphrase"} {
			if strings.Contains(got, secret) {
				t.Errorf("%s: a secret is quoted: %s", c.name, got)
			}
		}
	}
	// Off: nothing may answer, so OpenWrt's default communities must go.
	if got := check(off, "package snmpd\nconfig snmpd 'general'\n\toption enabled '0'\n"); got != "" {
		t.Fatalf("off: %s", got)
	}
	if got := check(off, "package snmpd\nconfig com2sec 'public'\n\toption community 'public'\nconfig snmpd 'general'\n\toption enabled '0'\n"); !strings.Contains(got, "1 communities are set, want none") {
		t.Fatalf("off with a community left: %s", got)
	}
	// Without snmpd: fine while off, refused while on.
	if got := check(off, "package wireless\n"); got != "" {
		t.Fatalf("off, no snmpd: %s", got)
	}
	if got := check(on, "package wireless\n"); !strings.Contains(got, "install it (apk add snmpd-ssl)") {
		t.Fatalf("on, no snmpd: %s", got)
	}
}

func TestPorts(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(`{"ports": {
		"lan2": {"enabled": true, "mode": "access", "untagged": 30},
		"lan3": {"enabled": false, "mode": "trunk", "untagged": 0, "tagged": [10, 20]},
		"lan9": {"mode": "access", "untagged": 10}}}`), &doc); err != nil {
		t.Fatal(err)
	}
	check := func(doc map[string]any, text string) string {
		t.Helper()
		c, err := uci.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(Check(doc, c), "\n")
	}
	const good = `package network
config device
	option name 'br-lan'
	option type 'bridge'
	list ports 'lan1'
	list ports 'lan2'
	list ports 'lan3'
	list ports 'wan'
config bridge-vlan 'vlan1'
	option device 'br-lan'
	option vlan '1'
	list ports 'wan:u*'
	list ports 'lan1:u*'
config bridge-vlan 'vlan10'
	option device 'br-lan'
	option vlan '10'
	list ports 'wan:t'
	list ports 'lan3:t'
config bridge-vlan 'vlan20'
	option device 'br-lan'
	option vlan '20'
	list ports 'wan:t'
	list ports 'lan3:t'
config bridge-vlan 'aeolus_vlan30'
	option device 'br-lan'
	option vlan '30'
	list ports 'wan:t'
	list ports 'lan2:u*'
config device 'aeolus_port_lan3'
	option name 'lan3'
	option enabled '0'
package aeolus
config agent 'agent'
	option uplink 'wan'
config watch 'aeolus_watch10'
	option vlan '10'
	option device 'wan'
	option tagged '1'
config watch 'aeolus_watch20'
	option vlan '20'
	option device 'wan'
	option tagged '1'
config watch 'aeolus_watch30'
	option vlan '30'
	option device 'wan'
	option tagged '1'
`
	// lan9 is not on this AP, so it is not judged.
	if got := check(doc, good); got != "" {
		t.Fatalf("problems with the right config: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"off", "option enabled '0'", "option enabled '1'", "ports.lan3: the port is on, want off"},
		{"untagged", "list ports 'lan2:u*'", "list ports 'lan2:t'", "ports.lan2: VLAN 30 is tagged, want untagged"},
		{"missing", "list ports 'wan:t'\n\tlist ports 'lan3:t'\nconfig bridge-vlan 'vlan20'", "list ports 'wan:t'\nconfig bridge-vlan 'vlan20'", "ports.lan3: VLAN 10 is not on the port, want tagged"},
		{"left over", "list ports 'lan1:u*'", "list ports 'lan1:u*'\n\tlist ports 'lan2:u*'", "ports.lan2: VLAN 1 is untagged, want not on the port"},
		{"uplink", "list ports 'wan:t'\n\tlist ports 'lan2:u*'", "list ports 'lan2:u*'", "ports.lan2: the uplink wan does not carry VLAN 30 tagged"},
		{"no bridge", "option type 'bridge'", "option type '8021q'", `ports: the uplink "wan" is in no bridge`},
	} {
		if !strings.Contains(good, c.old) {
			t.Fatalf("%s: fixture lacks %q", c.name, c.old)
		}
		if got := check(doc, strings.Replace(good, c.old, c.new, 1)); !strings.Contains(got, c.want) {
			t.Errorf("%s: want %q in:\n%s", c.name, c.want, got)
		}
	}
	// The uplink is the AP's management.
	var uplink map[string]any
	if err := json.Unmarshal([]byte(`{"ports": {"wan": {"enabled": false}}}`), &uplink); err != nil {
		t.Fatal(err)
	}
	if got := check(uplink, good); !strings.Contains(got, "ports.wan: the uplink carries the AP's management") {
		t.Fatalf("the uplink: %s", got)
	}
}

func TestTunnelFallbackAndIPv6(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(`{"network": {"sweet": {"transport": {
		"primary": {"type": "vlan", "vlan": 20},
		"fallback": {"type": "vxlan", "concentrator": "dc", "vni": 5000}}}},
		"concentrators": {"dc": {"address": "[2001:db8::2]", "port": 4789, "mtu": 1500}}}`), &doc); err != nil {
		t.Fatal(err)
	}
	check := func(text string) string {
		t.Helper()
		c, err := uci.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(Check(doc, c), "\n")
	}
	// A network with a fallback has a bridge of its own, which neither
	// transport is configured in (0061): a veth pair for the VLAN primary,
	// and the fallback's tunnel, stopped, with no bridge of its own.
	const good = `package wireless
package network
config device
	option name 'br-lan'
	option type 'bridge'
	option mtu '9000'
	list ports 'wan'
	list ports 'avp5584392f'
config interface 'lan'
	option proto 'dhcp'
	option device 'br-lan.1'
config bridge-vlan 'vlan20'
	option device 'br-lan'
	option vlan '20'
	list ports 'wan:t'
	list ports 'avp5584392f:u*'
config device 'aeolus_n5584392f_p'
	option type 'veth'
	option name 'avp5584392f'
	option peer_name 'anp5584392f'
config interface 'aeolus_5000'
	option proto 'vxlan6'
	option peer6addr '2001:db8::2'
	option vid '5000'
	option mtu '1500'
	option tunlink 'lan'
	option auto '0'
config device 'aeolus_n5584392f'
	option type 'bridge'
	option name 'br-n5584392f'
	option bridge_empty '1'
config interface 'aeolus_sweet'
	option proto 'none'
	option device 'br-n5584392f'
package firewall
config rule 'aeolus_vxlan_5000'
	option src 'lan'
	option proto 'udp'
	option src_ip '2001:db8::2'
	option dest_port '4789'
	option target 'ACCEPT'
package aeolus
config agent 'agent'
	option uplink 'wan'
config watch 'aeolus_watch20'
	option vlan '20'
	option device 'wan'
	option tagged '1'
config probe 'aeolus_5000'
	option vni '5000'
	option interval '30'
config probe 'aeolus_vlan20'
	option vlan '20'
	option device 'wan'
	option tagged '1'
	option interval '30'
config switch 'aeolus_n5584392f'
	option network 'sweet'
	option bridge 'br-n5584392f'
	option mode 'report'
	option primary 'anp5584392f'
	option primary_vlan '20'
	option fallback 'aeolus_5000'
	option fallback_vni '5000'
`
	// MTU 1500 needs no clamp, and an unset port is vxlan's default. The
	// jumbo bridge carries the tunnel's 1570-byte packets.
	if got := check(good); got != "" {
		t.Fatalf("problems with the right config: %s", got)
	}
	// Without it, the uplink is Linux's 1500 (0056).
	if got := check(strings.Replace(good, "\toption mtu '9000'\n", "", 1)); !strings.Contains(got,
		"network.sweet.transport.fallback: an MTU of 1500 needs 1570 on the AP's uplink, lan (br-lan.1), which carries 1500") {
		t.Fatalf("a 1500 uplink: %s", got)
	}
	// The interface's own MTU counts first.
	if got := check(strings.Replace(good, "\toption device 'br-lan.1'\n", "\toption device 'br-lan.1'\n\toption mtu '1500'\n", 1)); !strings.Contains(got, "which carries 1500") {
		t.Fatalf("the interface's MTU: %s", got)
	}
	if got := check(strings.Replace(good, "\toption auto '0'\n", "", 1)); !strings.Contains(got, "the fallback's tunnel is started; without HA mode, it waits") {
		t.Fatalf("a started fallback: %s", got)
	}
	if got := check(strings.Replace(good, "\toption mode 'report'\n", "\toption mode 'report'\n\toption holddown '60'\n", 1)); !strings.Contains(got,
		`aeolus.aeolus_n5584392f: holddown is "60", but the network does not switch (report mode)`) {
		t.Fatalf("a hold-down in report mode: %s", got)
	}

	// Automatic switching (0061): the plan says how, and in HA mode the
	// fallback's tunnel is started.
	checkAuto := func(ha bool, text string) string {
		t.Helper()
		var d map[string]any
		raw, _ := json.Marshal(doc)
		if err := json.Unmarshal(raw, &d); err != nil {
			t.Fatal(err)
		}
		tr := d["network"].(map[string]any)["sweet"].(map[string]any)["transport"].(map[string]any)
		tr["switching"], tr["ha"], tr["holddown"] = "automatic", ha, 60.0
		c, err := uci.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(Check(d, c), "\n")
	}
	automatic := strings.Replace(good, "\toption mode 'report'\n",
		"\toption mode 'automatic'\n\toption ha '0'\n\toption failback 'revertive'\n\toption holddown '60'\n", 1)
	if got := checkAuto(false, automatic); got != "" {
		t.Fatalf("automatic switching: %s", got)
	}
	if got := checkAuto(false, good); !strings.Contains(got, `aeolus.aeolus_n5584392f: mode is "report", want "automatic"`) {
		t.Fatalf("automatic switching, planned as report: %s", got)
	}
	withHA := strings.Replace(automatic, "\toption ha '0'\n", "\toption ha '1'\n", 1)
	if got := checkAuto(true, strings.Replace(withHA, "\toption auto '0'\n", "", 1)); got != "" {
		t.Fatalf("HA mode: %s", got)
	}
	if got := checkAuto(true, withHA); !strings.Contains(got, "network.aeolus_5000: the tunnel is not started") {
		t.Fatalf("HA mode with the fallback stopped: %s", got)
	}
}

// A tunnel can start from a VLAN of the uplink (0063): from an interface of
// Aeolus's own there, with an address by DHCP, routes in a table of their
// own and no DNS servers taken, in a zone that rejects what comes in, which
// is where the tunnel's rule lets it in from.
func TestTunnelStartsFromAVLAN(t *testing.T) {
	var doc map[string]any
	parse := func(underlay int) {
		t.Helper()
		if err := json.Unmarshal([]byte(fmt.Sprintf(`{"network": {"sweet": {"transport": {
			"primary": {"type": "vxlan", "concentrator": "arista", "vni": 50}}}},
			"concentrators": {"arista": {"address": "1.1.1.2", "port": 4789, "mtu": 1500, "underlay_vlan": %d}}}`, underlay)), &doc); err != nil {
			t.Fatal(err)
		}
	}
	parse(20)
	check := func(text string) string {
		t.Helper()
		c, err := uci.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(Check(doc, c), "\n")
	}
	const good = `package wireless
package network
config device
	option name 'br-lan'
	option type 'bridge'
	option mtu '9000'
config interface 'lan'
	option proto 'dhcp'
	option device 'br-lan.1'
config bridge-vlan 'vlan20'
	option device 'br-lan'
	option vlan '20'
config interface 'aeolus_vlan20_tunnels'
	option proto 'dhcp'
	option device 'br-lan.20'
	option ip4table '1020'
	option peerdns '0'
config interface 'aeolus_50'
	option proto 'vxlan'
	option peeraddr '1.1.1.2'
	option vid '50'
	option mtu '1500'
	option tunlink 'aeolus_vlan20_tunnels'
config device 'aeolus_50_br'
	option type 'bridge'
	option name 'br-vx50'
	list ports 'aeolus_50'
config interface 'aeolus_sweet'
	option proto 'none'
	option device 'br-vx50'
package firewall
config zone 'aeolus_zone_ul'
	option name 'aeolus_ul'
	option input 'REJECT'
	option output 'ACCEPT'
	option forward 'REJECT'
	list network 'aeolus_vlan20_tunnels'
config rule 'aeolus_vxlan_50'
	option src 'aeolus_ul'
	option proto 'udp'
	option src_ip '1.1.1.2'
	option dest_port '4789'
	option target 'ACCEPT'
package aeolus
config probe 'aeolus_50'
	option vni '50'
	option interval '30'
`
	if got := check(good); got != "" {
		t.Fatalf("problems with the right config: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"from management", "option tunlink 'aeolus_vlan20_tunnels'", "option tunlink 'lan'",
			`network.aeolus_50: tunlink is "lan", want aeolus_vlan20_tunnels, as the tunnel starts from VLAN 20 (0063)`},
		{"no interface", "config interface 'aeolus_vlan20_tunnels'", "config interface 'other'", "network.aeolus_vlan20_tunnels: no interface for the tunnels that start from VLAN 20"},
		{"another VLAN", "option device 'br-lan.20'", "option device 'br-lan.30'", `device is "br-lan.30", want VLAN 20 on the uplink's bridge`},
		{"not carried", "option vlan '20'", "option vlan '21'", "VLAN 20 is not in the network config"},
		{"static", "option proto 'dhcp'\n\toption device 'br-lan.20'", "option proto 'static'\n\toption device 'br-lan.20'", `proto is "static", want "dhcp"`},
		{"main table", "\toption ip4table '1020'\n", "", `network.aeolus_vlan20_tunnels: ip4table is missing, want "1020"`},
		{"its DNS", "\toption peerdns '0'\n", "", `network.aeolus_vlan20_tunnels: peerdns is missing, want "0"`},
		{"no zone", "list network 'aeolus_vlan20_tunnels'", "list network 'lan'", "firewall: want aeolus_vlan20_tunnels in the zone aeolus_ul"},
		{"open zone", "option input 'REJECT'", "option input 'ACCEPT'", `firewall.aeolus_zone_ul: input is "ACCEPT", want "REJECT"`},
		{"let in elsewhere", "option src 'aeolus_ul'", "option src 'lan'", `firewall.aeolus_vxlan_50: src is "lan", want "aeolus_ul"`},
	} {
		if !strings.Contains(good, c.old) {
			t.Fatalf("%s: fixture lacks %q", c.name, c.old)
		}
		if got := check(strings.Replace(good, c.old, c.new, 1)); !strings.Contains(got, c.want) {
			t.Errorf("%s: want %q in:\n%s", c.name, c.want, got)
		}
	}
	// 0 is the management VLAN, as unset is.
	parse(0)
	if got := check(good); !strings.Contains(got, "network.aeolus_50: the tunnel starts from aeolus_vlan20_tunnels, want the management interface (0063)") {
		t.Fatalf("the management VLAN: %s", got)
	}
	if got := check(strings.Replace(good, "option tunlink 'aeolus_vlan20_tunnels'", "option tunlink 'lan'", 1)); strings.Contains(got, "tunlink") || strings.Contains(got, "starts from") {
		t.Fatalf("from management, as it should: %s", got)
	}
}

// A network with a fallback has a bridge of its own, br-n and its hash,
// which its Wi-Fi joins and neither transport is configured in: the prober
// attaches the one that carries it (0061). A VLAN transport reaches it
// through a veth pair whose VLAN end is in the uplink's bridge, untagged in
// the VLAN, and is probed on the uplink from the VLAN's MAC; a VXLAN
// transport is its tunnel, with no bridge of its own. The prober's plan says
// which is which.
func TestNetworkWithAFallback(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(`{"network": {"lab": {"transport": {
		"primary": {"type": "vxlan", "concentrator": "arista", "vni": 50},
		"fallback": {"type": "vlan", "vlan": 20, "probe": "192.168.20.1"}}}},
		"concentrators": {"arista": {"address": "1.1.1.2", "port": 4789, "mtu": 1450}}}`), &doc); err != nil {
		t.Fatal(err)
	}
	check := func(text string) string {
		t.Helper()
		c, err := uci.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(CheckAP(doc, c, "ap-a0046021365e"), "\n")
	}
	if h := NetHash("lab"); h != "626092f4" {
		t.Fatalf("NetHash(lab) = %s", h)
	}
	const good = `package wireless
package network
config device
	option name 'br-lan'
	option type 'bridge'
	list ports 'wan'
	list ports 'avf626092f4'
config interface 'lan'
	option proto 'dhcp'
	option device 'br-lan.1'
config bridge-vlan 'vlan20'
	option device 'br-lan'
	option vlan '20'
	list ports 'wan:t'
	list ports 'avf626092f4:u*'
config interface 'aeolus_50'
	option proto 'vxlan'
	option peeraddr '1.1.1.2'
	option port '4789'
	option vid '50'
	option mtu '1450'
	option tunlink 'lan'
config device 'aeolus_n626092f4_f'
	option type 'veth'
	option name 'avf626092f4'
	option peer_name 'anf626092f4'
config device 'aeolus_n626092f4'
	option type 'bridge'
	option name 'br-n626092f4'
	option bridge_empty '1'
	option macaddr '02:21:36:5e:00:50'
config interface 'aeolus_lab'
	option proto 'none'
	option device 'br-n626092f4'
package firewall
config zone
	option name 'lan'
	list network 'lan'
config rule 'aeolus_vxlan_50'
	option src 'lan'
	option proto 'udp'
	option src_ip '1.1.1.2'
	option dest_port '4789'
	option target 'ACCEPT'
config include 'aeolus_clamp'
	option type 'nftables'
	option path '/etc/aeolus/clamp.nft'
package aeolus
config agent 'agent'
	option uplink 'wan'
config probe 'aeolus_50'
	option vni '50'
	option interval '30'
	option mac '02:21:36:5e:00:50'
config probe 'aeolus_vlan20'
	option vlan '20'
	option device 'wan'
	option tagged '1'
	option interval '30'
	list address '192.168.20.1'
	option mac '06:21:36:5e:00:20'
config switch 'aeolus_n626092f4'
	option network 'lab'
	option bridge 'br-n626092f4'
	option mode 'report'
	option primary 'aeolus_50'
	option primary_vni '50'
	option fallback 'anf626092f4'
	option fallback_vlan '20'
config watch 'aeolus_watch20'
	option vlan '20'
	option device 'wan'
	option tagged '1'
	option mac '06:21:36:5e:00:20'
`
	if got := check(good); got != "" {
		t.Fatalf("problems with the right config: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"not on its bridge", "option device 'br-n626092f4'", "option device 'br-lan.20'", `network.lab: interface aeolus_lab is on "br-lan.20", want the network's own bridge br-n626092f4 (0061)`},
		{"no bridge", "config device 'aeolus_n626092f4'\n", "config device 'other_bridge'\n", "network.aeolus_n626092f4: want a bridge br-n626092f4 of the network's own, as it has a fallback (0061)"},
		{"transport configured", "\toption bridge_empty '1'\n", "\toption bridge_empty '1'\n\tlist ports 'aeolus_50'\n", "network.aeolus_n626092f4: the bridge has [aeolus_50] configured"},
		{"bridge MAC", "option macaddr '02:21:36:5e:00:50'", "option macaddr '06:21:36:5e:00:20'", `network.aeolus_n626092f4: macaddr is "06:21:36:5e:00:20", want "02:21:36:5e:00:50"`},
		{"no veth", "config device 'aeolus_n626092f4_f'", "config device 'other_veth'", "network.aeolus_n626092f4_f: want a veth pair, avf626092f4 and anf626092f4, for VLAN 20 (0061)"},
		{"veth ends", "option peer_name 'anf626092f4'", "option peer_name 'anp626092f4'", "want a veth pair, avf626092f4 and anf626092f4"},
		{"veth off the uplink", "\tlist ports 'avf626092f4'\n", "", "network.lab.transport.fallback: the veth's VLAN end avf626092f4 is not in the uplink's bridge"},
		{"veth tagged", "list ports 'avf626092f4:u*'", "list ports 'avf626092f4:t'", "network.lab.transport.fallback: the veth's VLAN end avf626092f4 is not untagged in VLAN 20"},
		{"no plan", "config switch 'aeolus_n626092f4'", "config switch 'other_plan'", "aeolus.aeolus_n626092f4: no plan for the prober, which attaches the transport that carries network lab (0061)"},
		{"plan primary", "option primary 'aeolus_50'", "option primary 'anf626092f4'", `aeolus.aeolus_n626092f4: primary is "anf626092f4", want "aeolus_50"`},
		{"plan fallback", "option fallback_vlan '20'", "option fallback_vlan '21'", `aeolus.aeolus_n626092f4: fallback_vlan is "21", want "20"`},
		{"plan mode", "option mode 'report'", "option mode 'automatic'", `aeolus.aeolus_n626092f4: mode is "automatic", want "report"`},
		{"no VLAN probe", "config probe 'aeolus_vlan20'", "config probe 'other_probe'", "aeolus.aeolus_vlan20: no probe for VLAN 20, which a network with a fallback uses (0061)"},
		{"no watch", "config watch 'aeolus_watch20'", "config watch 'other_watch'", "aeolus.aeolus_watch20: no watch for VLAN 20, which the AP carries on its uplink for the intent; the prober tells from it whether the VLAN reaches the AP (0064)"},
		{"watch untagged", "option tagged '1'\n\toption mac '06:21:36:5e:00:20'", "option tagged '0'\n\toption mac '06:21:36:5e:00:20'", `aeolus.aeolus_watch20: tagged is "0", want "1"`},
		{"watch MAC", "option tagged '1'\n\toption mac '06:21:36:5e:00:20'", "option tagged '1'\n\toption mac '06:21:36:5e:00:21'", `aeolus.aeolus_watch20: mac is "06:21:36:5e:00:21", want "06:21:36:5e:00:20"`},
		{"stale watch", "config watch 'aeolus_watch20'", "config watch 'aeolus_watch21'\n\toption vlan '21'\nconfig watch 'aeolus_watch20'", "aeolus.aeolus_watch21: no tunnel, tunnel port, network with a fallback or VLAN on the uplink calls for it"},
		{"VLAN probe untagged", "option tagged '1'", "option tagged '0'", `aeolus.aeolus_vlan20: tagged is "0", want "1"`},
		{"VLAN probe address", "list address '192.168.20.1'", "list address '192.168.20.2'", "aeolus.aeolus_vlan20: probe addresses are [192.168.20.2], want [192.168.20.1]"},
		{"VLAN probe MAC", "option mac '06:21:36:5e:00:20'", "option mac '02:21:36:5e:00:20'", `aeolus.aeolus_vlan20: mac is "02:21:36:5e:00:20", want "06:21:36:5e:00:20"`},
		{"stale plan", "config switch 'aeolus_n626092f4'", "config switch 'aeolus_nffffffff'\n\toption network 'gone'\n\nconfig switch 'aeolus_n626092f4'", "aeolus.aeolus_nffffffff: no tunnel, tunnel port, network with a fallback or VLAN on the uplink calls for it"},
	} {
		if !strings.Contains(good, c.old) {
			t.Fatalf("%s: fixture lacks %q", c.name, c.old)
		}
		if got := check(strings.Replace(good, c.old, c.new, 1)); !strings.Contains(got, c.want) {
			t.Errorf("%s: want %q in:\n%s", c.name, c.want, got)
		}
	}
}

// A tunnel port is out of the uplink's bridge, and carries its VNIs on their
// tunnels' bridges: the port for the untagged one, an 802.1Q device of it
// for each tagged one (0058).
func TestTunnelPort(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(`{"ports": {"lan3": {"mode": "tunnel", "vxlan": {
		"untagged": {"tunnel": "arista", "vni": 50, "probe": "192.168.50.1"}, "10": {"tunnel": "arista", "vni": 10}}}},
		"concentrators": {"arista": {"address": "1.1.1.2", "port": 4789, "mtu": 1500}}}`), &doc); err != nil {
		t.Fatal(err)
	}
	check := func(text string) string {
		t.Helper()
		c, err := uci.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(Check(doc, c), "\n")
	}
	const good = `package network
config device
	option name 'br-lan'
	option type 'bridge'
	option mtu '9000'
	list ports 'lan1'
	list ports 'wan'
config bridge-vlan 'vlan1'
	option device 'br-lan'
	option vlan '1'
	list ports 'wan:u*'
	list ports 'lan1:u*'
config interface 'lan'
	option proto 'dhcp'
	option device 'br-lan.1'
config interface 'aeolus_50'
	option proto 'vxlan'
	option peeraddr '1.1.1.2'
	option port '4789'
	option vid '50'
	option mtu '1500'
	option tunlink 'lan'
config device 'aeolus_50_br'
	option type 'bridge'
	option name 'br-vx50'
	list ports 'aeolus_50'
	list ports 'lan3'
config interface 'aeolus_50_ports'
	option proto 'none'
	option device 'br-vx50'
config interface 'aeolus_10'
	option proto 'vxlan'
	option peeraddr '1.1.1.2'
	option port '4789'
	option vid '10'
	option mtu '1500'
	option tunlink 'lan'
config device 'aeolus_10_br'
	option type 'bridge'
	option name 'br-vx10'
	list ports 'aeolus_10'
	list ports 'lan3.10'
config interface 'aeolus_10_ports'
	option proto 'none'
	option device 'br-vx10'
config device 'aeolus_port_lan3_10'
	option type '8021q'
	option ifname 'lan3'
	option vid '10'
	option name 'lan3.10'
package firewall
config rule 'aeolus_vxlan_50'
	option src 'lan'
	option proto 'udp'
	option src_ip '1.1.1.2'
	option dest_port '4789'
	option target 'ACCEPT'
config rule 'aeolus_vxlan_10'
	option src 'lan'
	option proto 'udp'
	option src_ip '1.1.1.2'
	option dest_port '4789'
	option target 'ACCEPT'
package aeolus
config agent 'agent'
	option uplink 'wan'
config probe 'aeolus_50'
	option vni '50'
	option interval '30'
	list address '192.168.50.1'
config probe 'aeolus_10'
	option vni '10'
	option interval '30'
config guard 'aeolus_guard_lan3'
	option port 'lan3'
	list device 'lan3'
	list device 'lan3.10'
`
	if got := check(good); got != "" {
		t.Fatalf("problems with the right config: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		// The loop guard sends on the port and on its 802.1Q devices (0059).
		{"no guard", "config guard 'aeolus_guard_lan3'", "config guard 'other'", "aeolus.aeolus_guard_lan3: no loop guard for lan3, which is on a tunnel"},
		{"guard devices", "\tlist device 'lan3.10'\n", "", "aeolus.aeolus_guard_lan3: guards [lan3], want [lan3 lan3.10]"},
		{"port probe", "list address '192.168.50.1'", "list address '192.168.50.9'", "aeolus.aeolus_50: probe addresses are [192.168.50.9], want [192.168.50.1]"},
		{"still in br-lan", "list ports 'lan1'\n", "list ports 'lan1'\n\tlist ports 'lan3'\n", "ports.lan3: the port is still in the uplink's bridge br-lan"},
		{"still on a VLAN", "list ports 'lan1:u*'", "list ports 'lan1:u*'\n\tlist ports 'lan3:u*'", "ports.lan3: VLAN 1 of the uplink's bridge still has the port"},
		{"untagged not bridged", "list ports 'lan3'\n", "", "ports.lan3.vxlan.untagged: lan3 is not in the tunnel's bridge br-vx50"},
		{"tagged not bridged", "list ports 'lan3.10'\n", "", "ports.lan3.vxlan.10: lan3.10 is not in the tunnel's bridge br-vx10"},
		{"wrong VLAN device", "option vid '10'\n\toption name 'lan3.10'", "option vid '11'\n\toption name 'lan3.10'", "network.aeolus_port_lan3_10: want an 802.1Q device lan3.10, VLAN 10 on lan3"},
		// netifd makes a bridge only for an interface on it, so a VNI only
		// ports carry needs one of its own, with no address.
		{"bridge not held", "config interface 'aeolus_10_ports'\n\toption proto 'none'\n\toption device 'br-vx10'\n", "",
			"ports.lan3.vxlan.10: no interface is on the tunnel's bridge br-vx10, so netifd never makes it and lan3.10 is on nothing"},
		{"bridge held with an address", "config interface 'aeolus_50_ports'\n\toption proto 'none'", "config interface 'aeolus_50_ports'\n\toption proto 'dhcp'",
			"ports.lan3.vxlan.untagged: no interface is on the tunnel's bridge br-vx50"},
		{"tunnel not started", "option vid '10'\n\toption mtu '1500'\n\toption tunlink 'lan'", "option vid '10'\n\toption mtu '1500'\n\toption tunlink 'lan'\n\toption auto '0'", "the tunnel is not started"},
	} {
		if !strings.Contains(good, c.old) {
			t.Fatalf("%s: fixture lacks %q", c.name, c.old)
		}
		if got := check(strings.Replace(good, c.old, c.new, 1)); !strings.Contains(got, c.want) {
			t.Errorf("%s: want %q in:\n%s", c.name, c.want, got)
		}
	}
	// Back in access mode, the port must be in the uplink's bridge again.
	doc["ports"] = map[string]any{"lan3": map[string]any{"mode": "access", "untagged": float64(1)}}
	if got := check(good); !strings.Contains(got, "ports.lan3: the port is not in the uplink's bridge br-lan") {
		t.Fatalf("access, still on the tunnel: %s", got)
	}
}

// The MAC an AP uses on a segment says which AP and which segment (0060).
func TestSegmentMAC(t *testing.T) {
	for _, c := range []struct {
		ap, kind string
		n        int
		want     string
	}{
		{"ap-a0046021365e", "vni", 50, "02:21:36:5e:00:50"},
		{"ap-a0046021365e", "vni", 1234, "02:21:36:5e:12:34"},
		{"ap-a0046021365e", "vni", 9999, "02:21:36:5e:99:99"},
		{"ap-a0046021365e", "vni", 10000, "0a:21:36:5e:27:10"},
		{"ap-a0046021365e", "vni", 70000, "0a:21:36:5e:11:70"},
		{"ap-a0046021365e", "vlan", 20, "06:21:36:5e:00:20"},
		{"ap-a0046021365e", "vlan", 4094, "06:21:36:5e:40:94"},
		{"office-ap", "vni", 50, ""},
		{"ap-A0046021365E", "vni", 50, ""},
	} {
		if got := SegmentMAC(c.ap, c.kind, c.n); got != c.want {
			t.Errorf("SegmentMAC(%q, %s, %d) = %q, want %q", c.ap, c.kind, c.n, got, c.want)
		}
	}
}

// Checked for an AP, a tunnel's bridge and its probe take the AP's MAC for
// the segment (0060).
func TestTheTunnelsMACIsTheAPs(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(intent), &doc); err != nil {
		t.Fatal(err)
	}
	check := func(text string) string {
		t.Helper()
		c, err := uci.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(CheckAP(doc, c, "ap-a0046021365e"), "\n")
	}
	got := check(rendered)
	for _, want := range []string{
		"network.aeolus_20_br: macaddr is missing, want \"02:21:36:5e:00:20\"",
		"aeolus.aeolus_20: mac is missing, want \"02:21:36:5e:00:20\"",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in:\n%s", want, got)
		}
	}
	good := strings.Replace(strings.Replace(rendered,
		"option name 'br-vx20'", "option name 'br-vx20'\n\toption macaddr '02:21:36:5e:00:20'", 1),
		"option interval '20'", "option interval '20'\n\toption mac '02:21:36:5e:00:20'", 1)
	if got := check(good); got != "" {
		t.Fatalf("problems with the AP's MACs: %s", got)
	}
	if got := check(strings.Replace(good, "macaddr '02:21:36:5e:00:20'", "macaddr '02:21:36:5e:00:21'", 1)); !strings.Contains(got, "macaddr is \"02:21:36:5e:00:21\"") {
		t.Fatalf("another MAC: %s", got)
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

func TestAutomaticChannelLists(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(`{"radio": {"2g": {"channel": "auto"}, "5g": {"channel": "auto"}}}`), &doc); err != nil {
		t.Fatal(err)
	}
	check := func(text string) string {
		t.Helper()
		c, err := uci.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(Check(doc, c), "\n")
	}
	// 2.4 GHz picks from 1, 6 and 11; 5 GHz from anything that fits.
	ok := "package wireless\nconfig wifi-device 'radio1'\n\toption band '2g'\n\toption channel 'auto'\n\tlist channels '1'\n\tlist channels '6'\n\tlist channels '11'\n" +
		"config wifi-device 'radio0'\n\toption band '5g'\n\toption channel 'auto'\n"
	if got := check(ok); strings.Contains(got, "channels") {
		t.Fatalf("problems: %s", got)
	}
	if got := check("package wireless\nconfig wifi-device 'radio1'\n\toption band '2g'\n\toption channel 'auto'\n"); !strings.Contains(got, `wireless.radio1: channels is [], want ["1" "6" "11"]`) {
		t.Fatalf("2.4 GHz without the list: %s", got)
	}
	if got := check("package wireless\nconfig wifi-device 'radio0'\n\toption band '5g'\n\toption channel 'auto'\n\tlist channels '36'\n"); !strings.Contains(got, `wireless.radio0: channels is ["36"], want []`) {
		t.Fatalf("5 GHz with a stale list: %s", got)
	}
}

// With DFS avoided, an automatic channel is picked outside DFS and a channel
// the AP keeps must not be a DFS one; allowed, nothing holds the AP to
// avoiding them (0071).
func TestDFS(t *testing.T) {
	check := func(dfs, text string) string {
		t.Helper()
		var doc map[string]any
		if err := json.Unmarshal([]byte(`{"radio": {"5g": {"dfs": "`+dfs+`"}}}`), &doc); err != nil {
			t.Fatal(err)
		}
		c, err := uci.Parse("package wireless\n" + text)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(Check(doc, c), "\n")
	}
	avoid := check("avoid", `config wifi-device 'a'
	option band '5g'
	option channel 'auto'
	option acs_exclude_dfs '1'
config wifi-device 'b'
	option band '5g'
	option channel 'auto'
	option acs_exclude_dfs '0'
config wifi-device 'c'
	option band '5g'
	option channel '100'
	option htmode 'VHT80'
config wifi-device 'd'
	option band '5g'
	option channel '48'
	option htmode 'HE80'
	option acs_exclude_dfs '0'
config wifi-device 'e'
	option band '5g'
	option channel '48'
	option htmode 'HE160'
config wifi-device 'f'
	option band '5g'
	option channel '36'
	option acs_exclude_dfs '1'
`)
	for _, want := range []string{
		`wireless.b: acs_exclude_dfs is "0", want it on for radio.5g.dfs avoid`,
		"wireless.c: channel 100 at 80 MHz uses DFS channels, but radio.5g.dfs is avoid",
		"wireless.e: channel 48 at 160 MHz uses DFS channels, but radio.5g.dfs is avoid",
		`wireless.f: acs_exclude_dfs is "1", want it off for radio.5g.dfs avoid`,
	} {
		if !strings.Contains(avoid, want) {
			t.Errorf("missing %q in: %s", want, avoid)
		}
	}
	for _, ok := range []string{"wireless.a:", "wireless.d:"} {
		if strings.Contains(avoid, ok) {
			t.Errorf("%s should pass: %s", ok, avoid)
		}
	}
	allow := check("allow", `config wifi-device 'a'
	option band '5g'
	option channel 'auto'
	option acs_exclude_dfs '1'
config wifi-device 'b'
	option band '5g'
	option channel '100'
	option acs_exclude_dfs '0'
`)
	if !strings.Contains(allow, `wireless.a: acs_exclude_dfs is "1", want it off for radio.5g.dfs allow`) || strings.Contains(allow, "wireless.b:") {
		t.Errorf("allow: %s", allow)
	}
}

// Radio resource management on puts the daemon's section in the agent's
// package, naming the AP; off, there is none (0073).
func TestRRM(t *testing.T) {
	check := func(on bool, text string) string {
		t.Helper()
		var doc map[string]any
		if err := json.Unmarshal([]byte(fmt.Sprintf(`{"rrm": {"enabled": %v}}`, on)), &doc); err != nil {
			t.Fatal(err)
		}
		c, err := uci.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(CheckAP(doc, c, "ap-a0046021365e"), "\n")
	}
	section := "package aeolus\nconfig rrm 'aeolus_rrm'\n\toption enabled '1'\n\toption ap 'ap-a0046021365e'\n"
	if got := check(true, section); strings.Contains(got, "aeolus_rrm") {
		t.Errorf("on, with its section: %s", got)
	}
	if got := check(true, "package aeolus\n"); !strings.Contains(got, "aeolus.aeolus_rrm: radio resource management is on, but its section is missing") {
		t.Errorf("on, without: %s", got)
	}
	if got := check(true, strings.Replace(section, "a0046021365e", "2005b6018be0", 1)); !strings.Contains(got, `aeolus.aeolus_rrm: ap is "ap-2005b6018be0", want "ap-a0046021365e"`) {
		t.Errorf("another AP's: %s", got)
	}
	if got := check(false, section); !strings.Contains(got, "aeolus.aeolus_rrm: radio resource management is not on, but its section is there") {
		t.Errorf("off, with its section: %s", got)
	}
	if got := check(false, "package aeolus\n"); strings.Contains(got, "aeolus_rrm") {
		t.Errorf("off, without: %s", got)
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
