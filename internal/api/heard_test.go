package api

import (
	"net/netip"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/dhcpwatch"
)

// relayCopy is a client's DHCP discover as a relay forwards it, from the
// subnet whose relay address is giaddr, asking for addr, with the
// parameter list params.
func relayCopy(mac [6]byte, giaddr, addr string, params ...byte) []byte {
	b := make([]byte, 240)
	b[0], b[1], b[2], b[3] = 1, 1, 6, 1
	g := netip.MustParseAddr(giaddr).As4()
	copy(b[24:28], g[:])
	copy(b[28:34], mac[:])
	copy(b[236:240], []byte{99, 130, 83, 99})
	a := netip.MustParseAddr(addr).As4()
	b = append(b, 53, 1, 1, 50, 4, a[0], a[1], a[2], a[3], 55, byte(len(params)))
	b = append(b, params...)
	return append(b, 255)
}

func TestRelayedAndDetected(t *testing.T) {
	f := newFixture(t)
	for _, path := range []string{"/v1/dhcp/relayed", "/v1/detected"} {
		for _, who := range []string{"office", "tenant"} {
			if code, _ := f.do("GET", path, who, nil); code != 404 {
				t.Fatalf("%s as %s: %d", path, who, code)
			}
		}
	}
	if code, body := f.do("GET", "/v1/detected", "claude", nil); code != 200 || len(body["detected"].([]any)) != 0 || len(body["knocks"].([]any)) != 0 {
		t.Fatalf("nothing heard: %d %v", code, body)
	}

	arista := netip.MustParseAddr("192.168.20.1")
	openwifi := [6]byte{0x28, 0xe7, 0x1d, 0, 0, 1}
	f.watch.Relayed(arista, relayCopy(openwifi, "192.168.50.1", "192.168.50.9", 1, 3, 6, 43, 60, 138, 224))
	f.watch.Relayed(arista, relayCopy([6]byte{0x7e, 0x2a, 0xea, 0x9b, 0x2b, 0x8f}, "192.168.50.1", "192.168.50.81", 1, 3, 6, 15))

	code, body := f.do("GET", "/v1/dhcp/relayed", "claude", nil)
	subnets := body["subnets"].([]any)
	if code != 200 || len(subnets) != 1 {
		t.Fatalf("relayed: %d %v", code, body)
	}
	sn := subnets[0].(map[string]any)
	clients := sn["clients"].([]any)
	if sn["subnet"] != "192.168.50.1" || sn["relay"] != "192.168.20.1" || sn["requests"] != 2.0 || sn["new"] != 2.0 || sn["count"] != 2.0 || len(clients) != 2 {
		t.Fatalf("subnet: %v", sn)
	}
	for _, c := range clients {
		c := c.(map[string]any)
		switch c["mac"] {
		case "28:e7:1d:00:00:01":
			if c["openwifi"] != true || c["maker"] == nil || c["address"] != "192.168.50.9" {
				t.Fatalf("OpenWiFi client: %v", c)
			}
		case "7e:2a:ea:9b:2b:8f":
			if c["openwifi"] != nil || c["private"] != true {
				t.Fatalf("phone: %v", c)
			}
		}
	}

	// Possible by its fingerprint alone.
	_, body = f.do("GET", "/v1/detected", "claude", nil)
	d := body["detected"].([]any)
	if len(d) != 1 || d[0].(map[string]any)["status"] != "possible" || d[0].(map[string]any)["fingerprint"] != true {
		t.Fatalf("possible: %v", body)
	}

	// Confirmed by a knock from the address it asked for; another knock
	// matches no device.
	must(t, f.watch.Knocked(dhcpwatchHello("192.168.50.9", "aeolus.symtus.com")))
	must(t, f.watch.Knocked(dhcpwatchHello("192.168.50.77", "aeolus.symtus.com")))
	// And a Wi-Fi client an AP reports, which knocked too.
	ap, token, version := f.adopted()
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "clients": []any{
		map[string]any{"mac": "aa:bb:cc:dd:ee:05", "network": "lab", "address": "192.168.20.61", "host": "uap"}}}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, b)
	}
	must(t, f.watch.Knocked(dhcpwatchHello("192.168.20.61", "")))

	_, body = f.do("GET", "/v1/detected", "claude", nil)
	d = body["detected"].([]any)
	knocks := body["knocks"].([]any)
	if len(d) != 2 || len(knocks) != 1 || knocks[0].(map[string]any)["source"] != "192.168.50.77" {
		t.Fatalf("detected: %v", body)
	}
	byMAC := map[string]map[string]any{}
	for _, x := range d {
		byMAC[x.(map[string]any)["mac"].(string)] = x.(map[string]any)
	}
	if x := byMAC["28:e7:1d:00:00:01"]; x["status"] != "confirmed" || x["knocks"] != 1.0 || x["sni"] != "aeolus.symtus.com" || x["seen_by"] != "relay" || x["subnet"] != "192.168.50.1" {
		t.Fatalf("relayed knocker: %v", x)
	}
	if x := byMAC["aa:bb:cc:dd:ee:05"]; x["status"] != "confirmed" || x["fingerprint"] != false || x["seen_by"] != ap || x["host"] != "uap" {
		t.Fatalf("Wi-Fi knocker: %v", x)
	}
}

func dhcpwatchHello(src, sni string) dhcpwatch.Hello {
	return dhcpwatch.Hello{Source: netip.MustParseAddr(src), SNI: sni}
}
