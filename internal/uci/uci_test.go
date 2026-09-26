package uci

import (
	"reflect"
	"strings"
	"testing"
)

const export = `package wireless

config wifi-device 'radio0'
	option band '5g'
	option channel '36'
	option htmode 'VHT80'

config wifi-iface 'aeolus_sweet_5g'
	option device 'radio0'
	option ssid 'Pat'\''s "Wi-Fi"'
	option key "a\"b"
	list network 'aeolus_sweet'
	list network 'lan'

config wifi-iface
	option ssid 'OpenWrt'   # a comment

package system

config system
	option zonename 'America/Chicago'
`

func TestParse(t *testing.T) {
	c, err := Parse(export)
	if err != nil {
		t.Fatal(err)
	}
	w := c.Package("wireless")
	if len(w.Sections) != 3 || len(w.OfType("wifi-iface")) != 2 {
		t.Fatalf("wireless sections = %d", len(w.Sections))
	}
	iface := w.Named("aeolus_sweet_5g")
	if ssid, _ := iface.Option("ssid"); ssid != `Pat's "Wi-Fi"` {
		t.Errorf("ssid = %q", ssid)
	}
	if key, _ := iface.Option("key"); key != `a"b` {
		t.Errorf("key = %q", key)
	}
	if nets := iface.List("network"); !reflect.DeepEqual(nets, []string{"aeolus_sweet", "lan"}) {
		t.Errorf("network = %v", nets)
	}
	if band := w.Named("radio0").List("band"); !reflect.DeepEqual(band, []string{"5g"}) {
		t.Errorf("an option read as a list = %v", band)
	}
	anon := w.OfType("wifi-iface")[1]
	if anon.Name != "" || anon.Line != 15 {
		t.Errorf("anonymous section = %+v", anon)
	}
	if ssid, _ := anon.Option("ssid"); ssid != "OpenWrt" {
		t.Errorf("value before a comment = %q", ssid)
	}
	if z, _ := c.Package("system").OfType("system")[0].Option("zonename"); z != "America/Chicago" {
		t.Errorf("zonename = %q", z)
	}
	if c.Package("network") != nil || c.Package("network").Named("x") != nil {
		t.Error("a missing package should read as empty")
	}
}

func TestFlag(t *testing.T) {
	c, err := Parse("package p\nconfig t 's'\n\toption a '1'\n\toption b 'on'\n\toption c '0'\n")
	if err != nil {
		t.Fatal(err)
	}
	s := c.Package("p").Named("s")
	if !s.Flag("a") || !s.Flag("b") || s.Flag("c") || s.Flag("missing") {
		t.Fatal("flags read wrong")
	}
}

func TestParseRefuses(t *testing.T) {
	for name, text := range map[string]string{
		"config before package":  "config wifi-iface 'x'",
		"option outside section": "package wireless\noption ssid 'x'",
		"unknown keyword":        "package wireless\nconfig wifi-iface 'x'\n\tset ssid 'x'",
		"unterminated quote":     "package wireless\nconfig wifi-iface 'x",
		"duplicate package":      "package a\npackage a",
		"duplicate section":      "package a\nconfig t 's'\nconfig t 's'",
		"bad section name":       "package a\nconfig t 'a-b'",
		"option set twice":       "package a\nconfig t 's'\n\toption x '1'\n\toption x '2'",
		"option and list":        "package a\nconfig t 's'\n\toption x '1'\n\tlist x '2'",
		"option without value":   "package a\nconfig t 's'\n\toption x",
	} {
		if _, err := Parse(text); err == nil || !strings.HasPrefix(err.Error(), "line ") {
			t.Errorf("%s: %v", name, err)
		}
	}
}
