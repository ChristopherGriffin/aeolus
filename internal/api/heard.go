package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/dhcpwatch"
	"github.com/ChristopherGriffin/aeolus/internal/identify"
	"github.com/ChristopherGriffin/aeolus/internal/oui"
)

// What the manager hears itself (0068): relays' copies of DHCP requests,
// and knocks on the option 224 listener. Both are the Org's, so reading them
// takes a viewer's role on the Org.

// WithWatch gives the server the book its DHCP listeners write to.
func (s *Server) WithWatch(b *dhcpwatch.Book) *Server {
	s.watch = b
	return s
}

// maxRelayedShown is how many of a subnet's clients a read returns, newest
// first.
const maxRelayedShown = 500

// relayedClient is a relayed client as a read shows it: what the relay
// copies said, with its maker and the manager's guess at it (0067), and
// whether it asks for what an OpenWiFi AP asks for.
type relayedClient struct {
	conditions.RelayClient
	Maker    string `json:"maker,omitempty"`
	Private  bool   `json:"private,omitempty"`
	Kind     string `json:"kind,omitempty"`
	OS       string `json:"os,omitempty"`
	Basis    string `json:"basis,omitempty"`
	OpenWiFi bool   `json:"openwifi,omitempty"`
}

func describeRelayed(c conditions.RelayClient) relayedClient {
	out := relayedClient{RelayClient: c, OpenWiFi: dhcpwatch.OpenWiFi(c.Params)}
	out.Maker, out.Private = oui.Lookup(c.MAC)
	g := identify.Of(identify.Evidence{Host: c.Host, VendorClass: c.VendorClass, Params: c.Params, Maker: out.Maker, Private: out.Private})
	out.Kind, out.OS, out.Basis = g.Kind, g.OS, g.Basis
	return out
}

func orgViewer(c call) bool {
	t := c.state.Org.Locations
	return roleOn(c, change.Locations, t, t.Root()) >= access.Viewer
}

func (s *Server) view() dhcpwatch.View {
	if s.watch == nil {
		return dhcpwatch.View{Subnets: []dhcpwatch.SubnetView{}, Knocks: []conditions.Knock{}}
	}
	return s.watch.View()
}

// relayed lists each subnet relays copy requests from: the relay, the
// requests of the last 10 minutes, the clients new in that time, a burst of
// new ones, and the clients, newest first.
func (s *Server) relayed(w http.ResponseWriter, _ *http.Request, c call) error {
	if !orgViewer(c) {
		return errNotFound
	}
	v := s.view()
	subnets := []map[string]any{}
	for _, sn := range v.Subnets {
		clients := []relayedClient{}
		for i, rc := range sn.Clients {
			if i == maxRelayedShown {
				break
			}
			clients = append(clients, describeRelayed(rc))
		}
		subnets = append(subnets, map[string]any{
			"subnet": sn.Subnet, "relay": sn.Relay, "requests": sn.Requests, "new": sn.New, "burst": sn.Burst,
			"count": len(sn.Clients), "clients": clients,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ignored": v.Ignored, "subnets": subnets})
	return nil
}

// detectedAP is a device that may be an unconfigured OpenWiFi AP (0034):
// possible when its DHCP asks for options 138 and 224, confirmed when it
// knocked on the option 224 listener.
type detectedAP struct {
	MAC         string    `json:"mac"`
	Maker       string    `json:"maker,omitempty"`
	Private     bool      `json:"private,omitempty"`
	Status      string    `json:"status"`
	Subnet      string    `json:"subnet,omitempty"`
	Address     string    `json:"address,omitempty"`
	Host        string    `json:"host,omitempty"`
	VendorClass string    `json:"vendor_class,omitempty"`
	Fingerprint bool      `json:"fingerprint"`
	SeenBy      string    `json:"seen_by"`
	Knocks      int64     `json:"knocks"`
	SNI         string    `json:"sni,omitempty"`
	Last        time.Time `json:"last"`
}

// detected lists the possible and confirmed OpenWiFi APs, and the knocks no
// device could be matched to. A knock is matched by its source address: to
// the address a relayed client asked for or renewed, or to one an AP reports
// a Wi-Fi client using.
func (s *Server) detected(w http.ResponseWriter, _ *http.Request, c call) error {
	if !orgViewer(c) {
		return errNotFound
	}
	v := s.view()
	byMAC := map[string]*detectedAP{}
	byAddr := map[string]*detectedAP{}
	add := func(d detectedAP) *detectedAP {
		if x := byMAC[d.MAC]; x != nil {
			return x
		}
		d.Maker, d.Private = oui.Lookup(d.MAC)
		byMAC[d.MAC] = &d
		return &d
	}
	addrs := map[string]time.Time{}
	for _, sn := range v.Subnets {
		for _, rc := range sn.Clients {
			d := detectedAP{MAC: rc.MAC, Subnet: rc.Subnet, Address: rc.Address, Host: rc.Host, VendorClass: rc.VendorClass,
				Fingerprint: dhcpwatch.OpenWiFi(rc.Params), SeenBy: "relay", Last: rc.Last}
			if d.Fingerprint {
				add(d).Status = "possible"
			}
			if rc.Address != "" && rc.Last.After(addrs[rc.Address]) {
				addrs[rc.Address] = rc.Last
				x := d
				byAddr[rc.Address] = &x
			}
		}
	}
	// The addresses APs report their Wi-Fi clients using, where no relay
	// copy gave one.
	t := c.state.Org.Locations
	for _, ap := range t.APs() {
		st, err := s.conds.LatestState(ap)
		if err != nil {
			return err
		}
		if st == nil {
			continue
		}
		var r struct {
			Clients []wifiClient `json:"clients"`
		}
		if json.Unmarshal(st.Report, &r) != nil {
			continue
		}
		for _, wc := range r.Clients {
			if wc.Address == "" || byAddr[wc.Address] != nil {
				continue
			}
			byAddr[wc.Address] = &detectedAP{MAC: wc.MAC, Address: wc.Address, Host: wc.Host, VendorClass: wc.VendorClass,
				Fingerprint: dhcpwatch.OpenWiFi(wc.Params), SeenBy: string(ap), Last: st.At}
		}
	}
	unmatched := []conditions.Knock{}
	for _, k := range v.Knocks {
		m := byAddr[k.Source]
		if m == nil {
			unmatched = append(unmatched, k)
			continue
		}
		d := add(*m)
		d.Status = "confirmed"
		d.Knocks += k.Count
		if d.SNI == "" {
			d.SNI = k.SNI
		}
		if k.Last.After(d.Last) {
			d.Last = k.Last
		}
	}
	out := []detectedAP{}
	for _, d := range byMAC {
		if d.Status != "" {
			out = append(out, *d)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Status != out[j].Status {
			return out[i].Status == "confirmed"
		}
		return out[i].Last.After(out[j].Last)
	})
	writeJSON(w, http.StatusOK, map[string]any{"detected": out, "knocks": unmatched})
	return nil
}
