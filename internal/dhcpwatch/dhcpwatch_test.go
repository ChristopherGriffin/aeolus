package dhcpwatch

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/conditions"
)

// frame builds a client's DHCP request as a relay forwards it: giaddr set,
// with the options given as code, value pairs.
func frame(msgType byte, mac [6]byte, giaddr, ciaddr string, opts ...any) []byte {
	b := make([]byte, 240)
	b[0], b[1], b[2], b[3] = 1, 1, 6, 1
	copy(b[4:8], []byte{0x12, 0x34, 0x56, 0x78})
	if ciaddr != "" {
		a := netip.MustParseAddr(ciaddr).As4()
		copy(b[12:16], a[:])
	}
	if giaddr != "" {
		a := netip.MustParseAddr(giaddr).As4()
		copy(b[24:28], a[:])
	}
	copy(b[28:34], mac[:])
	copy(b[236:240], []byte{99, 130, 83, 99})
	if msgType != 0 {
		b = append(b, 53, 1, msgType)
	}
	for i := 0; i+1 < len(opts); i += 2 {
		v := opts[i+1].([]byte)
		b = append(b, byte(opts[i].(int)), byte(len(v)))
		b = append(b, v...)
	}
	return append(b, 255)
}

var phone = [6]byte{0x7e, 0x2a, 0xea, 0x9b, 0x2b, 0x8f}

func TestParse(t *testing.T) {
	fq := append([]byte{4, 0, 0, 5}, append([]byte("phone"), append([]byte{4}, append([]byte("home"), 0)...)...)...)
	agent := append([]byte{1, 9}, append([]byte("Ethernet6"), 2, 3, 0xde, 0xad, 0x01)...)
	cases := []struct {
		name string
		pkt  []byte
		ok   bool
		want Request
	}{
		{"a discover", frame(1, phone, "192.168.50.1", "", 12, []byte("Galaxy-S23\x00"), 60, []byte("android-dhcp-16"), 55, []byte{1, 3, 6, 15, 138, 224}, 50, []byte{192, 168, 50, 81}, 82, agent), true,
			Request{Type: "discover", MAC: "7e:2a:ea:9b:2b:8f", Address: "192.168.50.81", Host: "Galaxy-S23", VendorClass: "android-dhcp-16", Params: "1,3,6,15,138,224", Circuit: "Ethernet6", Remote: "dead01"}},
		{"a renewal, its name as an FQDN", frame(3, phone, "192.168.50.1", "192.168.50.81", 81, fq), true,
			Request{Type: "request", MAC: "7e:2a:ea:9b:2b:8f", Address: "192.168.50.81", Host: "phone.home"}},
		{"BOOTP", frame(0, phone, "192.168.50.1", ""), true, Request{Type: "bootp", MAC: "7e:2a:ea:9b:2b:8f"}},
		{"not relayed", frame(1, phone, "", ""), false, Request{}},
		{"an offer", func() []byte { f := frame(2, phone, "192.168.50.1", ""); f[0] = 2; return f }(), false, Request{}},
		{"a message type a client doesn't send", frame(5, phone, "192.168.50.1", ""), false, Request{}},
		{"an option running past the end", frame(1, phone, "192.168.50.1", "")[:242], false, Request{}},
		{"a name that isn't text", frame(1, phone, "192.168.50.1", "", 12, []byte{0x01, 0x02}), true, Request{Type: "discover", MAC: "7e:2a:ea:9b:2b:8f"}},
		{"too short", make([]byte, 100), false, Request{}},
	}
	for _, c := range cases {
		r, ok := Parse(c.pkt)
		if ok != c.ok {
			t.Errorf("%s: ok = %v", c.name, ok)
			continue
		}
		if !ok {
			continue
		}
		r.Giaddr = netip.Addr{}
		if r != c.want {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, r, c.want)
		}
	}
	if !OpenWiFi("1,3,6,15,138,224") || OpenWiFi("1,3,6,224") || OpenWiFi("1,3,1381,224") {
		t.Error("OpenWiFi fingerprint")
	}
}

func open(t *testing.T) (*Book, *conditions.Store, *time.Time) {
	t.Helper()
	clock := time.Date(2026, 10, 4, 22, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "conditions.db")
	st, err := conditions.Open(path, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	b, err := Open(st, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	return b, st, &clock
}

var arista = netip.MustParseAddr("192.168.20.1")

func TestRelayedClients(t *testing.T) {
	b, st, clock := open(t)
	b.Relayed(arista, frame(1, phone, "192.168.50.1", "", 12, []byte("Galaxy-S23"), 55, []byte{1, 3, 6}))
	*clock = clock.Add(2 * time.Second)
	b.Relayed(arista, frame(3, phone, "192.168.50.1", "", 50, []byte{192, 168, 50, 81}))
	b.Relayed(arista, frame(1, phone, "", ""))   // not relayed
	b.Relayed(arista, []byte("not DHCP at all")) // not DHCP
	v := b.View()
	if v.Ignored != 2 || len(v.Subnets) != 1 {
		t.Fatalf("view = %+v", v)
	}
	s := v.Subnets[0]
	c := s.Clients[0]
	if s.Subnet != "192.168.50.1" || s.Relay != "192.168.20.1" || s.Requests != 2 || s.New != 1 || len(s.Clients) != 1 ||
		c.Requests != 2 || c.Type != "request" || c.Host != "Galaxy-S23" || c.Params != "1,3,6" || c.Address != "192.168.50.81" || !c.Last.Equal(*clock) {
		t.Fatalf("subnet = %+v", s)
	}

	// Saved, and read back by a book opened afresh.
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	b2, err := Open(st, func() time.Time { return *clock })
	if err != nil {
		t.Fatal(err)
	}
	if v := b2.View(); len(v.Subnets) != 1 || v.Subnets[0].Clients[0].Host != "Galaxy-S23" || v.Subnets[0].Requests != 0 {
		t.Fatalf("reopened: %+v", v)
	}

	// Forgotten a week after last seen.
	*clock = clock.Add(ClientKeep + time.Minute)
	if err := b.Trim(); err != nil {
		t.Fatal(err)
	}
	if v := b.View(); len(v.Subnets) != 0 {
		t.Fatalf("after trim: %+v", v)
	}
	if cs, _ := st.RelayClients(); len(cs) != 0 {
		t.Fatalf("store after trim: %+v", cs)
	}
}

func TestBurst(t *testing.T) {
	b, _, clock := open(t)
	// Renewals from clients new to the manager are no burst: only discovers count.
	for i := 0; i < 100; i++ {
		b.Relayed(arista, frame(3, [6]byte{2, 0, 0, 0, 1, byte(i)}, "192.168.50.1", "192.168.50.9"))
	}
	if v := b.View(); v.Subnets[0].Burst != nil {
		t.Fatalf("renewals made a burst: %+v", v.Subnets[0].Burst)
	}
	for i := 0; i < BurstSize; i++ {
		b.Relayed(arista, frame(1, [6]byte{2, 0, 0, 0, 2, byte(i)}, "192.168.50.1", ""))
		*clock = clock.Add(500 * time.Millisecond)
	}
	if v := b.View(); v.Subnets[0].Burst != nil {
		t.Fatalf("%d discovers made a burst", BurstSize)
	}
	for i := 0; i < 10; i++ {
		b.Relayed(arista, frame(1, [6]byte{2, 0, 0, 0, 3, byte(i)}, "192.168.50.1", ""))
	}
	v := b.View()
	if v.Subnets[0].Burst == nil || v.Subnets[0].Burst.Clients < BurstSize+1 {
		t.Fatalf("burst = %+v", v.Subnets[0].Burst)
	}
	*clock = clock.Add(BurstKept + time.Minute)
	if v := b.View(); v.Subnets[0].Burst != nil || v.Subnets[0].Requests != 0 {
		t.Fatalf("an hour on: %+v", v.Subnets[0])
	}
}

func TestKnocksCount(t *testing.T) {
	b, st, clock := open(t)
	ap := netip.MustParseAddr("192.168.50.9")
	must(t, b.Knocked(Hello{Source: ap, SNI: "aeolus.symtus.com", Versions: []uint16{tls.VersionTLS13, tls.VersionTLS12}}))
	*clock = clock.Add(30 * time.Second)
	must(t, b.Knocked(Hello{Source: ap, SNI: "aeolus.symtus.com"}))
	*clock = clock.Add(2 * time.Minute)
	must(t, b.Knocked(Hello{Source: ap, SNI: "aeolus.symtus.com"}))
	v := b.View()
	if len(v.Knocks) != 2 || v.Knocks[1].Count != 2 || v.Knocks[1].Versions != "1.3 1.2" || v.Knocks[0].Count != 1 {
		t.Fatalf("knocks = %+v", v.Knocks)
	}
	ks, _ := st.Knocks()
	if len(ks) != 2 || ks[1].Count != 2 {
		t.Fatalf("stored = %+v", ks)
	}
}

func TestListeners(t *testing.T) {
	b, _, _ := open(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// A relay's copy, over UDP.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	must(t, err)
	done := make(chan error, 1)
	go func() { done <- ListenRelay(ctx, pc, b) }()
	out, err := net.Dial("udp", pc.LocalAddr().String())
	must(t, err)
	_, err = out.Write(frame(1, phone, "192.168.50.1", ""))
	must(t, err)
	out.Close()

	// A knock, over TLS, with the name an OpenWiFi AP would ask for.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	go ServeKnocks(ctx, ln, testCert(t), b)
	conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{ServerName: "aeolus.symtus.com", InsecureSkipVerify: true})
	must(t, err)
	conn.Close()
	// A connection that isn't TLS is no knock.
	plain, err := net.Dial("tcp", ln.Addr().String())
	must(t, err)
	plain.Write([]byte("GET / HTTP/1.0\r\n\r\n"))
	plain.Close()

	deadline := time.Now().Add(5 * time.Second)
	for {
		v := b.View()
		if len(v.Subnets) == 1 && len(v.Knocks) == 1 {
			if k := v.Knocks[0]; k.Source != "127.0.0.1" || k.SNI != "aeolus.symtus.com" || k.Versions == "" {
				t.Fatalf("knock = %+v", k)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("view = %+v", v)
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	if v := b.View(); len(v.Knocks) != 1 {
		t.Fatalf("a plain connection knocked: %+v", v.Knocks)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func testCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(t, err)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "aeolus"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"aeolus"}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	must(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
