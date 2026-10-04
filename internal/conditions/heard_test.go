package conditions

import (
	"testing"
	"time"
)

func TestRelayClients(t *testing.T) {
	s, clock := open(t)
	day := 24 * time.Hour
	a := RelayClient{Subnet: "192.168.50.1", MAC: "aa:bb:cc:dd:ee:01", Relay: "192.168.20.1", First: clock.Add(-8 * day), Last: clock.Add(-8 * day), Requests: 1, Type: "discover"}
	b := RelayClient{Subnet: "192.168.50.1", MAC: "aa:bb:cc:dd:ee:02", Relay: "192.168.20.1", First: *clock, Last: *clock, Requests: 2, Type: "request",
		Host: "phone", VendorClass: "android-dhcp-16", Params: "1,3,6", Address: "192.168.50.7", Circuit: "Ethernet6", Remote: "0a0b"}
	must(t, s.SaveRelayClients([]RelayClient{a, b}))
	b.Requests = 3
	must(t, s.SaveRelayClients([]RelayClient{b}))
	cs, err := s.RelayClients()
	must(t, err)
	if len(cs) != 2 {
		t.Fatalf("clients = %+v", cs)
	}
	for _, c := range cs {
		if c.MAC == b.MAC && (c.Requests != 3 || c.Host != "phone" || c.Circuit != "Ethernet6" || !c.Last.Equal(*clock)) {
			t.Fatalf("client = %+v", c)
		}
	}
	n, err := s.TrimRelayClients(clock.Add(-7 * day))
	must(t, err)
	cs, _ = s.RelayClients()
	if n != 1 || len(cs) != 1 || cs[0].MAC != b.MAC {
		t.Fatalf("after trim (%d): %+v", n, cs)
	}
}

func TestKnocks(t *testing.T) {
	s, clock := open(t)
	k, err := s.SaveKnock(Knock{First: *clock, Last: *clock, Count: 1, Source: "192.168.50.9", SNI: "aeolus.symtus.com", Versions: "1.2 1.3"})
	must(t, err)
	if k.ID == 0 {
		t.Fatal("no ID")
	}
	k.Count, k.Last, k.Subject = 2, clock.Add(time.Minute), "CN=903cb3bb2a1c"
	_, err = s.SaveKnock(k)
	must(t, err)
	for i := 0; i < 3; i++ {
		_, err := s.SaveKnock(Knock{First: clock.Add(time.Duration(i) * time.Hour), Last: clock.Add(time.Duration(i) * time.Hour), Count: 1, Source: "192.168.50.10"})
		must(t, err)
	}
	ks, err := s.Knocks()
	must(t, err)
	if len(ks) != 4 || ks[0].Last.Before(ks[1].Last) {
		t.Fatalf("knocks = %+v", ks)
	}
	var first Knock
	for _, x := range ks {
		if x.ID == k.ID {
			first = x
		}
	}
	if first.Count != 2 || first.Subject != "CN=903cb3bb2a1c" || first.SNI != "aeolus.symtus.com" || !first.First.Equal(*clock) {
		t.Fatalf("updated knock = %+v", first)
	}
	n, err := s.TrimKnocks(clock.Add(30*time.Second), 2)
	must(t, err)
	ks, _ = s.Knocks()
	if n != 2 || len(ks) != 2 || ks[0].Source != "192.168.50.10" {
		t.Fatalf("after trim (%d): %+v", n, ks)
	}
}
