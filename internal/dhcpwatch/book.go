package dhcpwatch

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/conditions"
)

const (
	Window      = 10 * time.Minute    // requests and new clients are counted over this
	BurstSpan   = time.Minute         // a burst is more than BurstSize new clients within this
	BurstSize   = 64                  //
	BurstKept   = time.Hour           // a burst is shown this long
	ClientKeep  = 7 * 24 * time.Hour  // a client is kept this long after it was last seen
	KnockKeep   = 30 * 24 * time.Hour // a knock is kept this long
	MaxKnocks   = 1000                // and at most this many
	KnockRepeat = time.Minute         // a source's knocks this close are one, counted
	MaxSubnets  = 256                 // subnets kept; a relay copy for another is ignored
	MaxClients  = 4096                // clients kept a subnet; more are counted, not kept
)

// Book is what the manager has heard: each subnet's relayed clients and
// recent requests, and the knocks on the option 224 listener. It is safe for
// concurrent use. Clients are saved to the store every half minute, knocks
// at once; both are read back when it opens.
type Book struct {
	mu      sync.Mutex
	store   *conditions.Store
	now     func() time.Time
	subnets map[string]*subnet
	knocks  []conditions.Knock // newest last
	ignored int64
}

type subnet struct {
	relay    string
	clients  map[string]*client
	requests counter // within Window
	fresh    counter // new clients' first discovers, within Window
	burst    *Burst
}

// counter counts events by the second, so a flood takes no more room than a
// trickle.
type counter map[int64]int

func (c counter) add(t time.Time) {
	c[t.Unix()]++
}

// since is how many came from t on, forgetting those before Window ends.
func (c counter) since(t time.Time, now time.Time) int {
	n, from, keep := 0, t.Unix(), now.Add(-Window).Unix()
	for s, k := range c {
		if s < keep {
			delete(c, s)
		} else if s >= from {
			n += k
		}
	}
	return n
}

type client struct {
	conditions.RelayClient
	dirty bool
}

// Burst is a minute in which more than BurstSize clients new to a subnet
// sent their first discover, one sign of DHCP starvation.
type Burst struct {
	At      time.Time `json:"at"`
	Clients int       `json:"clients"`
}

// Open reads back what store kept. now supplies times; nil means time.Now.
func Open(store *conditions.Store, now func() time.Time) (*Book, error) {
	if now == nil {
		now = time.Now
	}
	b := &Book{store: store, now: now, subnets: map[string]*subnet{}}
	cs, err := store.RelayClients()
	if err != nil {
		return nil, err
	}
	for _, c := range cs {
		s := b.subnet(c.Subnet, c.Relay)
		if s != nil {
			s.clients[c.MAC] = &client{RelayClient: c}
		}
	}
	ks, err := store.Knocks()
	if err != nil {
		return nil, err
	}
	for i := len(ks) - 1; i >= 0; i-- {
		b.knocks = append(b.knocks, ks[i])
	}
	return b, nil
}

// subnet finds or makes a subnet's record, or returns nil when MaxSubnets
// are kept already.
func (b *Book) subnet(id, relay string) *subnet {
	s := b.subnets[id]
	if s == nil {
		if len(b.subnets) >= MaxSubnets {
			return nil
		}
		s = &subnet{relay: relay, clients: map[string]*client{}, requests: counter{}, fresh: counter{}}
		b.subnets[id] = s
	}
	if relay != "" {
		s.relay = relay
	}
	return s
}

// Relayed takes one datagram that came to the relay listener from src.
// Anything but a relayed client request is counted as ignored.
func (b *Book) Relayed(src netip.Addr, pkt []byte) {
	r, ok := Parse(pkt)
	b.mu.Lock()
	defer b.mu.Unlock()
	if !ok {
		b.ignored++
		return
	}
	now := b.now()
	s := b.subnet(r.Giaddr.String(), src.Unmap().String())
	if s == nil {
		b.ignored++
		return
	}
	s.requests.add(now)
	c := s.clients[r.MAC]
	if c == nil {
		if r.Type == "discover" {
			s.fresh.add(now)
			n := s.fresh.since(now.Add(-BurstSpan), now)
			switch {
			case n > BurstSize && (s.burst == nil || now.Sub(s.burst.At) > BurstSpan):
				s.burst = &Burst{At: now, Clients: n}
			case s.burst != nil && now.Sub(s.burst.At) <= BurstSpan && n > s.burst.Clients:
				s.burst.Clients = n
			}
		}
		if len(s.clients) >= MaxClients {
			return
		}
		c = &client{RelayClient: conditions.RelayClient{Subnet: r.Giaddr.String(), MAC: r.MAC, First: now}}
		s.clients[r.MAC] = c
	}
	c.Relay, c.Last, c.Type, c.dirty = s.relay, now, r.Type, true
	c.Requests++
	for _, f := range []struct {
		to   *string
		from string
	}{{&c.Host, r.Host}, {&c.VendorClass, r.VendorClass}, {&c.Params, r.Params}, {&c.Address, r.Address}, {&c.Circuit, r.Circuit}, {&c.Remote, r.Remote}} {
		if f.from != "" {
			*f.to = f.from
		}
	}
}

// Hello is what a connection to the option 224 listener showed of itself.
type Hello struct {
	Source   netip.Addr
	SNI      string
	Versions []uint16
	Subject  string
}

// Knocked records a connection to the option 224 listener. One from the same
// source, for the same name, within KnockRepeat of its last is counted with
// it.
func (b *Book) Knocked(h Hello) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	src := h.Source.Unmap().String()
	for i := len(b.knocks) - 1; i >= 0; i-- {
		k := &b.knocks[i]
		if k.Source == src && k.SNI == h.SNI && now.Sub(k.Last) <= KnockRepeat {
			k.Last = now
			k.Count++
			if h.Subject != "" {
				k.Subject = h.Subject
			}
			_, err := b.store.SaveKnock(*k)
			// Keep the newest last.
			x := *k
			b.knocks = append(append(b.knocks[:i], b.knocks[i+1:]...), x)
			return err
		}
	}
	var vs []string
	for _, v := range h.Versions {
		vs = append(vs, tlsVersion(v))
	}
	k, err := b.store.SaveKnock(conditions.Knock{First: now, Last: now, Count: 1, Source: src, SNI: h.SNI, Versions: strings.Join(vs, " "), Subject: h.Subject})
	if err != nil {
		return err
	}
	b.knocks = append(b.knocks, k)
	if len(b.knocks) > MaxKnocks {
		b.knocks = b.knocks[len(b.knocks)-MaxKnocks:]
	}
	return nil
}

func tlsVersion(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "1.0"
	case tls.VersionTLS11:
		return "1.1"
	case tls.VersionTLS12:
		return "1.2"
	case tls.VersionTLS13:
		return "1.3"
	}
	return "?"
}

// Flush saves the clients changed since the last flush.
func (b *Book) Flush() error {
	b.mu.Lock()
	var out []conditions.RelayClient
	var marked []*client
	for _, s := range b.subnets {
		for _, c := range s.clients {
			if c.dirty {
				out = append(out, c.RelayClient)
				marked = append(marked, c)
				c.dirty = false
			}
		}
	}
	b.mu.Unlock()
	if err := b.store.SaveRelayClients(out); err != nil {
		b.mu.Lock()
		for _, c := range marked {
			c.dirty = true
		}
		b.mu.Unlock()
		return err
	}
	return nil
}

// Trim forgets clients last seen more than ClientKeep ago, subnets left with
// none, and knocks beyond KnockKeep or MaxKnocks, here and in the store.
func (b *Book) Trim() error {
	b.mu.Lock()
	now := b.now()
	for id, s := range b.subnets {
		for mac, c := range s.clients {
			if now.Sub(c.Last) > ClientKeep {
				delete(s.clients, mac)
			}
		}
		if len(s.clients) == 0 && s.requests.since(now.Add(-Window), now) == 0 {
			delete(b.subnets, id)
		}
	}
	keep := b.knocks[:0]
	for _, k := range b.knocks {
		if now.Sub(k.Last) <= KnockKeep {
			keep = append(keep, k)
		}
	}
	b.knocks = keep
	b.mu.Unlock()
	if _, err := b.store.TrimRelayClients(now.Add(-ClientKeep)); err != nil {
		return err
	}
	_, err := b.store.TrimKnocks(now.Add(-KnockKeep), MaxKnocks)
	return err
}

// View is what the book holds now, for the API.
type View struct {
	Ignored int64              `json:"ignored"`
	Subnets []SubnetView       `json:"subnets"`
	Knocks  []conditions.Knock `json:"knocks"`
}

// SubnetView is one subnet: the relay's address on it, the address its
// copies come from, the requests of the last Window, its clients newest
// first, how many were new in the last Window, and a burst within BurstKept.
type SubnetView struct {
	Subnet   string                   `json:"subnet"`
	Relay    string                   `json:"relay"`
	Requests int                      `json:"requests"`
	New      int                      `json:"new"`
	Burst    *Burst                   `json:"burst"`
	Clients  []conditions.RelayClient `json:"clients"`
}

// View returns what the book holds, subnets in address order, clients and
// knocks newest first.
func (b *Book) View() View {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	v := View{Ignored: b.ignored, Subnets: []SubnetView{}, Knocks: []conditions.Knock{}}
	for id, s := range b.subnets {
		sv := SubnetView{Subnet: id, Relay: s.relay, Requests: s.requests.since(now.Add(-Window), now), Clients: []conditions.RelayClient{}}
		for _, c := range s.clients {
			sv.Clients = append(sv.Clients, c.RelayClient)
			if now.Sub(c.First) <= Window {
				sv.New++
			}
		}
		sort.Slice(sv.Clients, func(i, j int) bool { return sv.Clients[i].Last.After(sv.Clients[j].Last) })
		if s.burst != nil && now.Sub(s.burst.At) <= BurstKept {
			x := *s.burst
			sv.Burst = &x
		}
		v.Subnets = append(v.Subnets, sv)
	}
	sort.Slice(v.Subnets, func(i, j int) bool {
		a, _ := netip.ParseAddr(v.Subnets[i].Subnet)
		c, _ := netip.ParseAddr(v.Subnets[j].Subnet)
		return a.Less(c)
	})
	for i := len(b.knocks) - 1; i >= 0; i-- {
		v.Knocks = append(v.Knocks, b.knocks[i])
	}
	return v
}

// Keep saves changed clients every half minute and trims every hour, until
// ctx ends; then it saves once more.
func (b *Book) Keep(ctx context.Context) {
	flush := time.NewTicker(30 * time.Second)
	defer flush.Stop()
	trim := time.NewTicker(time.Hour)
	defer trim.Stop()
	for {
		select {
		case <-flush.C:
			if err := b.Flush(); err != nil {
				slog.Error("saving relayed DHCP clients", "err", err)
			}
		case <-trim.C:
			if err := b.Trim(); err != nil {
				slog.Error("trimming what the DHCP listeners heard", "err", err)
			}
		case <-ctx.Done():
			if err := b.Flush(); err != nil {
				slog.Error("saving relayed DHCP clients", "err", err)
			}
			return
		}
	}
}
