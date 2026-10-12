package api

// An AP's own way out (0142). Its prober finds the interface with its
// default route, and asks that interface's gateway, the name servers it
// was given, and the internet, where the config names a target
// (system.internet): each every 30 seconds, with how long its answer took.
// The agent adds how long the manager took to answer its last poll. It is
// kept as the AP said it, in its state report, and shown on the AP's Path
// tab.

// pathState is the way out as a state report says it: the interface, its
// device, and the AP's address there with its prefix length; the gateway;
// the name servers, and the name they are asked for; the internet; and the
// manager.
type pathState struct {
	Interface string     `json:"interface,omitempty"`
	Device    string     `json:"device,omitempty"`
	Address   string     `json:"address,omitempty"`
	Mask      *int       `json:"mask,omitempty"`
	Gateway   *pathHop   `json:"gateway,omitempty"`
	DNS       []pathName `json:"dns,omitempty"`
	Name      string     `json:"name,omitempty"`
	Internet  *pathHop   `json:"internet,omitempty"`
	Manager   *pathTime  `json:"manager,omitempty"`
}

// pathHop is the gateway, or the internet: its address; for the internet
// the target the config names, which may be a name; whether it answers
// (up, silent once three asks in a row went unanswered, unknown until
// asked that long, or unresolved for a name no server gave an address
// for); how long its last answer took, in ms, while it answers; and how
// many seconds ago that answer was.
type pathHop struct {
	Target  string   `json:"target,omitempty"`
	Address string   `json:"address,omitempty"`
	State   string   `json:"state,omitempty"`
	MS      *float64 `json:"ms"`
	Ago     *int64   `json:"ago"`
}

// pathName is one name server: its address, whether it answers, how long
// its last answer took and how long ago, and the answer's code (0 for an
// answer with no error).
type pathName struct {
	Server string   `json:"server"`
	State  string   `json:"state,omitempty"`
	MS     *float64 `json:"ms"`
	Ago    *int64   `json:"ago"`
	Rcode  *int     `json:"rcode"`
}

// pathTime is how long the manager took to answer the AP's last config
// poll, in ms: the whole request, its connection too.
type pathTime struct {
	MS float64 `json:"ms"`
}

var pathStates = map[string]bool{"": true, "up": true, "silent": true, "unknown": true, "unresolved": true}

// shortWord is a short printable word of a report, such as an interface or an
// address.
func shortWord(s string, most int) bool {
	return len(s) <= most && printable(s)
}

func pathTimes(ms *float64, ago *int64) bool {
	return (ms == nil || (*ms >= 0 && *ms <= 60000)) && (ago == nil || *ago >= 0)
}

func (h *pathHop) ok() bool {
	return h == nil || (shortWord(h.Target, 253) && shortWord(h.Address, 45) && pathStates[h.State] && pathTimes(h.MS, h.Ago))
}

// clean is the way out where it is well said, and nil where it is not: an
// AP's report is kept without it, as one that said none is.
func (p *pathState) clean() *pathState {
	if p == nil {
		return nil
	}
	if !shortWord(p.Interface, 64) || !shortWord(p.Device, 64) || !shortWord(p.Address, 45) || !shortWord(p.Name, 253) ||
		(p.Mask != nil && (*p.Mask < 0 || *p.Mask > 32)) || !p.Gateway.ok() || !p.Internet.ok() || len(p.DNS) > 4 ||
		(p.Manager != nil && (p.Manager.MS < 0 || p.Manager.MS > 600000)) {
		return nil
	}
	for _, d := range p.DNS {
		if d.Server == "" || !shortWord(d.Server, 45) || !pathStates[d.State] || !pathTimes(d.MS, d.Ago) || (d.Rcode != nil && (*d.Rcode < 0 || *d.Rcode > 15)) {
			return nil
		}
	}
	return p
}
