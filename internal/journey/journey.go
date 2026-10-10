// Package journey is one Wi-Fi client's history across the fleet (0103):
// the sessions it had, each on one AP, band and network, from the state
// reports the APs sent while it was on them; where it roamed; and what
// looks wrong, such as a weak signal, no DHCP, or moving back and forth.
// It is the first part of the client journey Griff wants: from what the
// manager keeps already, with nothing new on the APs.
package journey

import (
	"fmt"
	"sort"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Client is what one state report says of a client (0066, 0067).
type Client struct {
	MAC       string  `json:"mac"`
	Network   string  `json:"network"`
	SSID      string  `json:"ssid"`
	Band      string  `json:"band"`
	Signal    *int    `json:"signal"`
	TxRate    float64 `json:"tx_rate"`
	RxRate    float64 `json:"rx_rate"`
	TxPackets int64   `json:"tx_packets"`
	TxRetries int64   `json:"tx_retries"`
	TxFailed  int64   `json:"tx_failed"`
	Connected int64   `json:"connected"`
	Address   string  `json:"address"`
	Host      string  `json:"host"`
	DHCP      string  `json:"dhcp"`
	Gen       string  `json:"gen"`
	VLAN      int     `json:"vlan"`
}

// Sample is a client as one AP's report said, at the report's time.
type Sample struct {
	AP     hierarchy.NodeID
	APName string
	At     time.Time
	C      Client
}

// Session is a client's time on one AP, band and network, unbroken.
type Session struct {
	AP        hierarchy.NodeID `json:"ap"`
	Name      string           `json:"name"`
	Network   string           `json:"network,omitempty"`
	SSID      string           `json:"ssid"`
	Band      string           `json:"band"`
	From      time.Time        `json:"from"`
	To        time.Time        `json:"to"`
	Reports   int              `json:"reports"`
	SignalMin *int             `json:"signal_min,omitempty"`
	SignalMax *int             `json:"signal_max,omitempty"`
	SignalAvg *int             `json:"signal_avg,omitempty"`
	TxRate    float64          `json:"tx_rate"` // Mbit/s, the mean of the reports
	Retries   *float64         `json:"retries,omitempty"`
	Address   string           `json:"address,omitempty"`
	DHCP      string           `json:"dhcp,omitempty"`
	Gen       string           `json:"gen,omitempty"`
}

// Issue is something that looks wrong in the journey.
type Issue struct {
	Severity string    `json:"severity"` // warning or info
	Kind     string    `json:"kind"`
	Message  string    `json:"message"`
	At       time.Time `json:"at"`
}

// Journey is a client's history.
type Journey struct {
	MAC      string    `json:"mac"`
	Host     string    `json:"host,omitempty"`
	First    time.Time `json:"first"`
	Last     time.Time `json:"last"`
	Sessions []Session `json:"sessions"`
	Roams    int       `json:"roams"`
	Issues   []Issue   `json:"issues"`
}

// Gap is how long a client may be missing from reports and still be in the
// same session: a little over two report intervals (0040).
const Gap = 12 * time.Minute

// Build makes the journey from the samples, in any order.
func Build(mac string, samples []Sample) Journey {
	j := Journey{MAC: mac, Sessions: []Session{}, Issues: []Issue{}}
	if len(samples) == 0 {
		return j
	}
	sort.SliceStable(samples, func(a, b int) bool { return samples[a].At.Before(samples[b].At) })
	j.First, j.Last = samples[0].At, samples[len(samples)-1].At
	type acc struct {
		s       Session
		signals []int
		rates   []float64
		last    Client
	}
	var cur *acc
	flush := func() {
		if cur == nil {
			return
		}
		s := cur.s
		if len(cur.signals) > 0 {
			lo, hi, sum := cur.signals[0], cur.signals[0], 0
			for _, v := range cur.signals {
				lo, hi, sum = min(lo, v), max(hi, v), sum+v
			}
			avg := (sum - len(cur.signals)/2) / len(cur.signals)
			if sum >= 0 {
				avg = (sum + len(cur.signals)/2) / len(cur.signals)
			}
			s.SignalMin, s.SignalMax, s.SignalAvg = &lo, &hi, &avg
		}
		for _, r := range cur.rates {
			s.TxRate += r / float64(len(cur.rates))
		}
		// As the Clients tab counts them: retries over what was sent.
		if sent := cur.last.TxPackets + cur.last.TxFailed; sent > 50 {
			r := float64(cur.last.TxRetries) / float64(sent)
			s.Retries = &r
		}
		s.Address, s.DHCP, s.Gen = cur.last.Address, cur.last.DHCP, cur.last.Gen
		j.Sessions = append(j.Sessions, s)
		cur = nil
	}
	for _, x := range samples {
		if x.C.Host != "" {
			j.Host = x.C.Host
		}
		// The AP's own networks have no network ID: by SSID, then.
		sameNet := cur != nil && cur.s.Network == x.C.Network && (x.C.Network != "" || cur.s.SSID == x.C.SSID)
		same := sameNet && cur.s.AP == x.AP && cur.s.Band == x.C.Band && x.At.Sub(cur.s.To) <= Gap
		if !same {
			flush()
			cur = &acc{s: Session{AP: x.AP, Name: x.APName, Network: x.C.Network, SSID: x.C.SSID, Band: x.C.Band, From: x.At.Add(-time.Duration(x.C.Connected) * time.Second), To: x.At}}
			// A session cannot start before the one it follows ended.
			if n := len(j.Sessions); n > 0 && cur.s.From.Before(j.Sessions[n-1].To) {
				cur.s.From = j.Sessions[n-1].To
			}
		}
		cur.s.To = x.At
		cur.s.Reports++
		if x.C.Signal != nil {
			cur.signals = append(cur.signals, *x.C.Signal)
		}
		if x.C.TxRate > 0 {
			cur.rates = append(cur.rates, x.C.TxRate)
		}
		cur.last = x.C
	}
	flush()
	for i := 1; i < len(j.Sessions); i++ {
		if j.Sessions[i].AP != j.Sessions[i-1].AP && j.Sessions[i].From.Sub(j.Sessions[i-1].To) <= Gap {
			j.Roams++
		}
	}
	j.Issues = issues(j)
	return j
}

// issues is what looks wrong: a weak signal, few rates, retries, DHCP that
// the client did not use, and moving back and forth between two APs.
func issues(j Journey) []Issue {
	out := []Issue{}
	add := func(sev, kind, msg string, at time.Time) {
		out = append(out, Issue{Severity: sev, Kind: kind, Message: msg, At: at})
	}
	for _, s := range j.Sessions {
		if s.SignalAvg != nil && *s.SignalAvg < -75 {
			add("warning", "weak-signal", fmt.Sprintf("a weak signal on %s (%s), %d dBm on average: it may need an AP nearer, or to roam sooner", s.Name, bandName(s.Band), *s.SignalAvg), s.From)
		}
		if s.Retries != nil && *s.Retries > 0.2 {
			add("warning", "retries", fmt.Sprintf("%.0f%% of frames to it on %s were sent again: interference, or a signal too weak for its rate", *s.Retries*100, s.Name), s.From)
		}
		if (s.Band == "5g" || s.Band == "6g") && s.TxRate > 0 && s.TxRate < 30 && s.Reports >= 2 {
			add("info", "slow", fmt.Sprintf("on %s (%s) it ran at %.0f Mbit/s on average", s.Name, bandName(s.Band), s.TxRate), s.From)
		}
		switch s.DHCP {
		case "none":
			add("warning", "no-dhcp", fmt.Sprintf("on %s it asked for no address by DHCP, and shows none", s.Name), s.From)
		case "static":
			add("info", "static", fmt.Sprintf("on %s it uses %s without asking DHCP: a static address", s.Name, s.Address), s.From)
		}
	}
	// Back and forth: four roams or more between the same two APs, A B A B
	// A, within half an hour.
	for i := 0; i+4 < len(j.Sessions); i++ {
		a, b := j.Sessions[i].AP, j.Sessions[i+1].AP
		if a == b {
			continue
		}
		roams := 0
		for k := i + 1; k < len(j.Sessions) && roams < 4; k++ {
			want := b
			if roams%2 == 1 {
				want = a
			}
			if j.Sessions[k].AP != want || j.Sessions[k].From.Sub(j.Sessions[i].To) > 30*time.Minute {
				break
			}
			roams++
		}
		if roams >= 4 {
			add("warning", "ping-pong", fmt.Sprintf("it moved back and forth between %s and %s: their coverage overlaps where it sits; a lower power or a minimum signal settles it", j.Sessions[i].Name, j.Sessions[i+1].Name), j.Sessions[i].To)
			break
		}
	}
	return out
}

func bandName(b string) string {
	switch b {
	case "2g":
		return "2.4 GHz"
	case "5g":
		return "5 GHz"
	case "6g":
		return "6 GHz"
	}
	return b
}
