package identify

import (
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/oui"
)

func TestGuesses(t *testing.T) {
	for _, c := range []struct {
		name string
		e    Evidence
		want Guess
	}{
		{"iPhone by name", Evidence{Host: "Griffs-iPhone", Private: true}, Guess{"phone", "iOS", "host name"}},
		{"iPad before iPhone", Evidence{Host: "Kids-iPad"}, Guess{"tablet", "iPadOS", "host name"}},
		{"a Galaxy tablet", Evidence{Host: "Galaxy-Tab-S9"}, Guess{"tablet", "Android", "host name"}},
		{"a Galaxy phone", Evidence{Host: "Galaxy-S22"}, Guess{"phone", "Android", "host name"}},
		{"Windows by name", Evidence{Host: "DESKTOP-4KJ2L1Q"}, Guess{"computer", "Windows", "host name"}},
		{"Android by vendor class", Evidence{VendorClass: "android-dhcp-14", Private: true}, Guess{"phone", "Android", "DHCP vendor class"}},
		{"Windows by vendor class", Evidence{VendorClass: "MSFT 5.0"}, Guess{"computer", "Windows", "DHCP vendor class"}},
		{"Apple by fingerprint, then private", Evidence{Params: "1,121,3,6,15,108,114,119,252,95,44,46", Private: true},
			Guess{"phone, tablet or computer", "iOS or macOS", "DHCP fingerprint, private MAC"}},
		{"ESP by name", Evidence{Host: "ESP_3F0A21"}, Guess{"iot", "", "host name"}},
		{"Espressif by maker", Evidence{Maker: "Espressif Inc."}, Guess{"iot", "", "maker"}},
		{"embedded Linux by vendor class, IoT by maker", Evidence{VendorClass: "udhcp 1.36.1", Maker: "Espressif Inc."}, Guess{"iot", "Linux", "DHCP vendor class"}},
		{"Apple by maker only", Evidence{Maker: "Apple, Inc."}, Guess{"", "Apple", "maker"}},
		{"a Sonos speaker", Evidence{Maker: "Sonos, Inc."}, Guess{"speaker", "", "maker"}},
		{"nothing to go on", Evidence{Maker: "Cisco Systems, Inc"}, Guess{}},
		{"a name that says nothing", Evidence{Host: "living-room"}, Guess{}},
	} {
		if got := Of(c.e); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

// The OUI table is built in, and knows the lab's makers (0067).
func TestOUI(t *testing.T) {
	if n := oui.Size(); n < 30000 {
		t.Fatalf("the OUI table has %d prefixes", n)
	}
	for mac, want := range map[string]string{
		"28:e7:1d:ca:29:13": "Arista Networks",
		"a0:04:60:21:36:5e": "NETGEAR",
		"84:0d:8e:5a:df:f7": "Espressif Inc.",
	} {
		if got, private := oui.Lookup(mac); got != want || private {
			t.Errorf("%s: %q (private %v), want %q", mac, got, private, want)
		}
	}
	if got, private := oui.Lookup("7e:2a:ea:9b:2b:8f"); got != "" || !private {
		t.Errorf("a private MAC: %q, private %v", got, private)
	}
	if got, _ := oui.Lookup("not a mac"); got != "" {
		t.Errorf("not a MAC: %q", got)
	}
}
