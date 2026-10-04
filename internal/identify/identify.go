// Package identify guesses what kind of device a Wi-Fi client is, and its
// operating system (0067), from what it gave away: its host name; its DHCP
// vendor class (option 60) and the options it asked for, in order (55, its
// fingerprint); its maker, by OUI; and whether its MAC is private. Each
// guess says what it went on, and stays empty rather than guess wildly.
package identify

import (
	"regexp"
	"strings"
)

// Evidence is what is known of a client.
type Evidence struct {
	Host        string // the host name it gave in DHCP
	VendorClass string // DHCP option 60
	Params      string // DHCP option 55, as "1,3,6,15"
	Maker       string // by OUI
	Private     bool   // a locally administered MAC
}

// Guess is a device's kind and operating system, and what they came from.
type Guess struct {
	Kind  string // phone, tablet, computer, tv, speaker, console, printer, camera, iot, watch; or ""
	OS    string
	Basis string // e.g. "host name, DHCP vendor class"
}

type rule struct {
	re       *regexp.Regexp
	kind, os string
}

func r(pattern, kind, os string) rule {
	return rule{regexp.MustCompile(`(?i)` + pattern), kind, os}
}

// By host name, the most telling, in order: the first that matches wins.
var hosts = []rule{
	r(`ipad`, "tablet", "iPadOS"),
	r(`iphone`, "phone", "iOS"),
	r(`apple-?watch|^watch$`, "watch", "watchOS"),
	r(`apple-?tv`, "tv", "tvOS"),
	r(`homepod`, "speaker", "audioOS"),
	r(`macbook|imac|mac-?mini|mac-?pro|mac-?studio|-mbp\b|^mbp\b`, "computer", "macOS"),
	r(`galaxy-?tab|^sm-[tx]\d`, "tablet", "Android"),
	r(`galaxy|^sm-[a-z]\d|pixel|oneplus|xiaomi|redmi|poco|huawei|honor|oppo|vivo|motorola|^moto|^android`, "phone", "Android"),
	r(`chromebook|^chromeos`, "computer", "ChromeOS"),
	r(`^desktop-|^laptop-|^win-|windows`, "computer", "Windows"),
	r(`raspberrypi|^raspberry|ubuntu|debian|fedora|archlinux`, "computer", "Linux"),
	r(`chromecast|roku|fire-?tv|^aft[a-z]|bravia|webos|samsung-?tv|vizio|hisense|shield`, "tv", ""),
	r(`echo|alexa|sonos|google-?home|nest-?(audio|mini|hub)`, "speaker", ""),
	r(`xbox|playstation|^ps[345]\b|nintendo`, "console", ""),
	r(`printer|^hp[0-9a-f]{6}|epson|brother|canon|^npi[0-9a-f]+`, "printer", ""),
	r(`camera|^cam|ring-|wyze|arlo|blink|eufy|reolink`, "camera", ""),
	r(`^esp[_-]|espressif|tasmota|shelly|tuya|wled|sonoff|^lwip|smart-?plug|wemo|kasa|meross|tapo`, "iot", ""),
}

// By DHCP vendor class (option 60).
var vendorClasses = []rule{
	r(`^android-dhcp`, "phone", "Android"),
	r(`^msft`, "computer", "Windows"),
	r(`^udhcp`, "iot", "Linux"),
	r(`^dhcpcd`, "", "Linux"),
}

// By the DHCP options asked for, in order (option 55): a few well-known ones.
var fingerprints = map[string]Guess{
	"1,121,3,6,15,108,114,119,252,95,44,46":      {OS: "iOS or macOS"},
	"1,3,6,15,26,28,51,58,59,43,114,108":         {Kind: "phone", OS: "Android"},
	"1,3,6,15,26,28,51,58,59,43,114":             {Kind: "phone", OS: "Android"},
	"1,3,6,15,31,33,43,44,46,47,119,121,249,252": {Kind: "computer", OS: "Windows"},
}

// By maker, where a maker makes mostly one kind of thing.
var makers = []rule{
	r(`espressif|tuya|shelly|itead|hui zhou gaoshengda|particle industries|lumi united`, "iot", ""),
	r(`sonos`, "speaker", ""),
	r(`roku`, "tv", ""),
	r(`raspberry pi`, "computer", "Linux"),
	r(`nintendo|sony interactive`, "console", ""),
	r(`ring llc|wyze|arlo|reolink`, "camera", ""),
	r(`seiko epson|brother industries`, "printer", ""),
}

// Of returns the guess for a client.
func Of(e Evidence) Guess {
	var g Guess
	var basis []string
	take := func(kind, os, why string) {
		used := false
		if g.Kind == "" && kind != "" {
			g.Kind, used = kind, true
		}
		if g.OS == "" && os != "" {
			g.OS, used = os, true
		}
		if used {
			basis = append(basis, why)
		}
	}
	match := func(rules []rule, s, why string) {
		if s == "" {
			return
		}
		for _, x := range rules {
			if x.re.MatchString(s) {
				take(x.kind, x.os, why)
				return
			}
		}
	}
	match(hosts, e.Host, "host name")
	match(vendorClasses, e.VendorClass, "DHCP vendor class")
	if f, ok := fingerprints[e.Params]; ok {
		take(f.Kind, f.OS, "DHCP fingerprint")
	}
	match(makers, e.Maker, "maker")
	if strings.HasPrefix(strings.ToLower(e.Maker), "apple") {
		take("", "Apple", "maker")
	}
	if e.Private {
		take("phone, tablet or computer", "", "private MAC")
	}
	g.Basis = strings.Join(basis, ", ")
	return g
}
