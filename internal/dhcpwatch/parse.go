// Package dhcpwatch is the manager's own DHCP listening (0035, 0068): the
// copies a site's relays send it of their clients' DHCP requests, and knocks
// on the option 224 listener from OpenWiFi APs looking for a gateway. It
// never answers DHCP and never speaks uCentral; it only records.
package dhcpwatch

import (
	"encoding/hex"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// Request is what the manager keeps of one relayed DHCP request.
type Request struct {
	Type        string     // discover, request, decline, release, inform, or bootp
	MAC         string     // the client's hardware address
	Giaddr      netip.Addr // the relay's address on the client's subnet
	Address     string     // what the client asks for (option 50) or renews (ciaddr)
	Host        string     // option 12, else option 81's name
	VendorClass string     // option 60
	Params      string     // option 55, as comma-separated numbers
	Circuit     string     // option 82's circuit ID, as text or hex
	Remote      string     // option 82's remote ID, as text or hex
}

var types = map[byte]string{1: "discover", 3: "request", 4: "decline", 7: "release", 8: "inform"}

// Parse reads a BOOTP/DHCP request from a client, as a relay forwards it. It
// returns false for anything else: an answer, a request with no relay
// address, an unknown message type, or a malformed packet.
func Parse(b []byte) (Request, bool) {
	var r Request
	if len(b) < 240 || b[0] != 1 || b[1] != 1 || b[2] != 6 || string(b[236:240]) != "\x63\x82\x53\x63" {
		return r, false
	}
	r.Giaddr = netip.AddrFrom4([4]byte(b[24:28]))
	if r.Giaddr.IsUnspecified() {
		return r, false
	}
	r.MAC = mac(b[28:34])
	if ci := netip.AddrFrom4([4]byte(b[12:16])); !ci.IsUnspecified() {
		r.Address = ci.String()
	}
	r.Type = "bootp"
	opts, ok := options(b[240:])
	if !ok {
		return r, false
	}
	if t, ok := opts[53]; ok {
		if len(t) != 1 || types[t[0]] == "" {
			return r, false
		}
		r.Type = types[t[0]]
	}
	if a := opts[50]; len(a) == 4 {
		r.Address = netip.AddrFrom4([4]byte(a)).String()
	}
	r.Host = text(opts[12])
	if r.Host == "" {
		r.Host = fqdn(opts[81])
	}
	r.VendorClass = text(opts[60])
	if p := opts[55]; len(p) > 0 {
		if len(p) > 64 {
			p = p[:64]
		}
		ns := make([]string, len(p))
		for i, c := range p {
			ns[i] = strconv.Itoa(int(c))
		}
		r.Params = strings.Join(ns, ",")
	}
	if ra, ok := options(opts[82]); ok {
		r.Circuit, r.Remote = id(ra[1]), id(ra[2])
	}
	return r, true
}

// options reads a run of DHCP options, up to an end option or the end of b,
// into a map by code. Repeated options are joined, as RFC 3396 has them.
func options(b []byte) (map[byte][]byte, bool) {
	out := map[byte][]byte{}
	for i := 0; i < len(b); {
		code := b[i]
		if code == 255 {
			break
		}
		if code == 0 {
			i++
			continue
		}
		if i+1 >= len(b) || i+2+int(b[i+1]) > len(b) {
			return nil, false
		}
		out[code] = append(out[code], b[i+2:i+2+int(b[i+1])]...)
		i += 2 + int(b[i+1])
	}
	return out, true
}

func mac(b []byte) string {
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", b[0], b[1], b[2], b[3], b[4], b[5])
}

// text is b as text if it is printable ASCII of at most 64 bytes, without a
// trailing NUL, which some clients add; otherwise nothing.
func text(b []byte) string {
	b = []byte(strings.TrimRight(string(b), "\x00"))
	if len(b) == 0 || len(b) > 64 {
		return ""
	}
	for _, c := range b {
		if c < ' ' || c > '~' {
			return ""
		}
	}
	return string(b)
}

// fqdn is option 81's name: after three bytes of flags and codes, in DNS's
// labels when the E flag is set, else as text.
func fqdn(b []byte) string {
	if len(b) < 4 {
		return ""
	}
	name := b[3:]
	if b[0]&4 == 0 {
		return text(name)
	}
	var labels []string
	for i := 0; i < len(name) && name[i] != 0; {
		n := int(name[i])
		if n > 63 || i+1+n > len(name) {
			return ""
		}
		labels = append(labels, string(name[i+1:i+1+n]))
		i += 1 + n
	}
	return text([]byte(strings.Join(labels, ".")))
}

// id is a relay agent sub-option as text when printable, else as hex, of at
// most 32 bytes.
func id(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if s := text(b); s != "" && len(s) <= 32 {
		return s
	}
	if len(b) > 32 {
		b = b[:32]
	}
	return hex.EncodeToString(b)
}

// OpenWiFi says whether a parameter list asks for options 138 and 224, as
// an OpenWiFi AP's does to find its gateway (0034, 0035).
func OpenWiFi(params string) bool {
	var a, b bool
	for _, p := range strings.Split(params, ",") {
		a = a || p == "138"
		b = b || p == "224"
	}
	return a && b
}
