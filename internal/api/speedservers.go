package api

import (
	"net"
	"regexp"
	"strconv"
)

// A speed server near, and one far (0134). A client test measures speed
// against the local server (system.speed_server), which shows what the air
// and the wire to it carry, and against a remote one
// (system.speed_server_remote), out on the internet, which shows the whole
// way out. The remote one is a public server by its name here, or an
// OpenSpeedTest server of one's own by its URL.

// speedTarget is a server a test measures against, as the AP is told it:
// any HTTP server that gives bytes for a GET and takes them for a POST.
// Name is how it is said to people; Down the path a GET of which gives
// many bytes, and Up the path a POST to which takes them; Body how many
// bytes a POST carries, where the server takes fewer than the agent would
// send. Plain HTTP, as the test's own interface sends it.
type speedTarget struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
	Down string `json:"down"`
	Up   string `json:"up"`
	Body int    `json:"body,omitempty"`
}

// The bytes a POST to a public server carries: the LibreSpeed servers
// refuse one of 32 MB (413), which is what the agent sends its own.
const publicBody = 4 << 20

// publicSpeed are the public speed servers a remote one may be, by the
// name the setting takes: those that answer plain HTTP both ways, as tried
// from the lab on 2026-10-11. Cloudflare's is the nearest of its sites,
// wherever the AP is; the others are where they say. The schema lists the
// same names, with how each is said (x-aeolus-choices).
var publicSpeed = map[string]speedTarget{
	"cloudflare":               {Name: "Cloudflare", Host: "speed.cloudflare.com", Down: "/__down?bytes=50000000", Up: "/__up"},
	"librespeed-atlanta":       libreSpeed("LibreSpeed, Atlanta (Clouvider)", "atl.speedtest.clouvider.net", "/backend"),
	"librespeed-los-angeles":   libreSpeed("LibreSpeed, Los Angeles (Clouvider)", "la.speedtest.clouvider.net", "/backend"),
	"librespeed-london":        libreSpeed("LibreSpeed, London (Clouvider)", "lon.speedtest.clouvider.net", "/backend"),
	"librespeed-amsterdam":     libreSpeed("LibreSpeed, Amsterdam (Clouvider)", "ams.speedtest.clouvider.net", "/backend"),
	"librespeed-frankfurt":     libreSpeed("LibreSpeed, Frankfurt (Clouvider)", "fra.speedtest.clouvider.net", "/backend"),
	"librespeed-prague-cesnet": libreSpeed("LibreSpeed, Prague (CESNET)", "speedtest.cesnet.cz", "/backend"),
	"librespeed-prague-turris": libreSpeed("LibreSpeed, Prague (Turris)", "librespeed.turris.cz", "/backend"),
	"librespeed-belgrade":      libreSpeed("LibreSpeed, Belgrade (SOX)", "speedtest1.sox.rs", "/librespeed/backend"),
	"tele2":                    {Name: "Tele2, Europe", Host: "speedtest.tele2.net", Down: "/1GB.zip", Up: "/upload.php"},
}

// libreSpeed is a LibreSpeed server: 100 MB of bytes a GET, and a POST
// that takes what it is sent.
func libreSpeed(name, host, backend string) speedTarget {
	return speedTarget{Name: name, Host: host, Down: backend + "/garbage.php?ckSize=100", Up: backend + "/empty.php"}
}

var ownSpeedRE = regexp.MustCompile(`^http://([A-Za-z0-9.-]{1,253})(?::([0-9]{1,5}))?/?$`)

// remoteSpeed is the remote speed server a setting names: a public one, or
// an OpenSpeedTest server by its URL, which gives bytes at /downloading
// and takes them at /upload. Nil for a value that is neither, as one a
// later manager's list has and this one's has not.
func remoteSpeed(v string) *speedTarget {
	if t, ok := publicSpeed[v]; ok {
		t.Port, t.Body = 80, publicBody
		return &t
	}
	m := ownSpeedRE.FindStringSubmatch(v)
	if m == nil {
		return nil
	}
	port := 80
	if m[2] != "" {
		p, err := strconv.Atoi(m[2])
		if err != nil || p < 1 || p > 65535 {
			return nil
		}
		port = p
	}
	name := m[1]
	if port != 80 {
		name = net.JoinHostPort(m[1], m[2])
	}
	return &speedTarget{Name: name, Host: m[1], Port: port, Down: "/downloading", Up: "/upload"}
}

// speedWant says which speeds a test is asked to measure, where their
// servers are set: against the local one, and against the remote one.
type speedWant struct{ Local, Remote bool }
