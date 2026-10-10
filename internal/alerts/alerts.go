// Package alerts says what needs a person's attention across the fleet
// (0099): an AP that stopped calling, a config the manager holds or an AP
// refused or put back, an agent update rolled back, and what an AP's own
// last report says is wrong, such as a tunnel down or a VLAN the switch
// does not send. It reads only what the manager has already; it stores
// nothing, so an alert ends when its cause does.
package alerts

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// The severities, most urgent first.
const (
	Critical = "critical" // clients are, or will be, without service
	Warning  = "warning"  // something is wrong that a person should look at
	Info     = "info"     // worth knowing, no harm yet
)

var rank = map[string]int{Critical: 0, Warning: 1, Info: 2}

// Alert is one thing that needs attention on one AP.
type Alert struct {
	AP       hierarchy.NodeID `json:"ap"`
	Name     string           `json:"name"`
	Severity string           `json:"severity"`
	Kind     string           `json:"kind"`
	Message  string           `json:"message"`
	Since    *time.Time       `json:"since,omitempty"`
}

// Input is what the manager knows of one AP.
type Input struct {
	AP         hierarchy.NodeID
	Name       string
	Now        time.Time
	Poll       time.Duration // between its config polls; 60 s unless set
	Unassigned bool          // waiting in Landing Zone
	Version    int64         // the config version it should run
	Problems   []string      // why the manager holds its config, if it does
	Latest     conditions.Latest
}

// StateEvery is how often an AP reports its state (0040); a report older
// than three of these is too old to speak for the AP now.
const StateEvery = 5 * time.Minute

// Offline is how long an AP may be silent before it is called offline:
// three of its polls and half a minute, and never under five minutes, so a
// restart or a slow poll is not an alert.
func Offline(poll time.Duration) time.Duration {
	if poll <= 0 {
		poll = time.Minute
	}
	return max(3*poll+30*time.Second, 5*time.Minute)
}

// For is the alerts for one AP, most urgent first.
func For(in Input) []Alert {
	var out []Alert
	add := func(sev, kind, msg string, since *time.Time) {
		out = append(out, Alert{AP: in.AP, Name: in.Name, Severity: sev, Kind: kind, Message: msg, Since: since})
	}
	l := in.Latest
	if in.Unassigned {
		add(Info, "unassigned", "waiting in Landing Zone to be adopted", seenAt(l.Seen))
		return out
	}
	if l.Seen == nil {
		add(Warning, "never-seen", "has never called the manager", nil)
	} else if silent := in.Now.Sub(l.Seen.At); silent > Offline(in.Poll) {
		add(Critical, "offline", fmt.Sprintf("offline: last heard %s ago, from %s", ago(silent), l.Seen.Source), &l.Seen.At)
	}
	if len(in.Problems) > 0 {
		more := ""
		if len(in.Problems) > 1 {
			more = fmt.Sprintf(" (and %d more)", len(in.Problems)-1)
		}
		add(Warning, "held", fmt.Sprintf("config held: %s%s", in.Problems[0], more), nil)
	}
	failed := false
	if c := l.Check; c != nil && c.Version == in.Version && c.Result != conditions.OK && c.Result != conditions.Stale {
		failed = true
		why := c.Result
		if len(c.Problems) > 0 {
			why = c.Problems[0]
		}
		add(Critical, "refused", fmt.Sprintf("version %d was refused by the render check: %s", c.Version, why), &c.At)
	}
	if a := l.Apply; a != nil && a.Version == in.Version && !a.OK {
		failed = true
		add(Critical, "apply-failed", fmt.Sprintf("version %d did not apply: %s", a.Version, a.Error), &a.At)
	}
	// Behind, with no reason above: it has not taken up the current version
	// for three polls.
	if s := l.Seen; s != nil && !failed && len(in.Problems) == 0 && s.Running != nil && *s.Running != in.Version &&
		s.RunningAt != nil && in.Now.Sub(*s.RunningAt) > Offline(in.Poll) && in.Now.Sub(s.At) <= Offline(in.Poll) {
		add(Warning, "behind", fmt.Sprintf("runs version %d, not the current %d", *s.Running, in.Version), s.RunningAt)
	}
	if st := l.State; st != nil && in.Now.Sub(st.At) <= 3*StateEvery {
		out = append(out, fromReport(in, st)...)
	}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Severity] < rank[out[j].Severity] })
	return out
}

// report is the parts of a state report alerts read (0040, 0059, 0061,
// 0064, 0065, 0069, 0079).
type report struct {
	WirelessMissing bool `json:"wireless_missing"`
	Agent           *struct {
		Update *struct {
			Version string `json:"version"`
			State   string `json:"state"`
			Why     string `json:"why"`
		} `json:"update"`
	} `json:"agent"`
	Transports map[string]struct {
		Active       string `json:"active"`
		CannotSwitch string `json:"cannot_switch"`
	} `json:"transports"`
	VXLAN *struct {
		Tunnels []struct {
			VNI     int    `json:"vni"`
			Peer    string `json:"peer"`
			Standby bool   `json:"standby"`
			Probe   *struct {
				Verdict string `json:"verdict"`
			} `json:"probe"`
		} `json:"tunnels"`
		Loops []struct {
			Port string `json:"port"`
			VNI  int    `json:"vni"`
		} `json:"loops"`
	} `json:"vxlan"`
	UplinkVLANs []struct {
		VLAN    int    `json:"vlan"`
		Verdict string `json:"verdict"`
	} `json:"uplink_vlans"`
	DHCP map[string]struct {
		Answered   int `json:"answered"`
		Unanswered int `json:"unanswered"`
	} `json:"dhcp"`
	Time *struct {
		Synced *bool `json:"synced"`
	} `json:"time"`
}

func fromReport(in Input, st *conditions.State) []Alert {
	var r report
	if json.Unmarshal(st.Report, &r) != nil {
		return nil
	}
	var out []Alert
	add := func(sev, kind, msg string) {
		out = append(out, Alert{AP: in.AP, Name: in.Name, Severity: sev, Kind: kind, Message: msg, Since: &st.At})
	}
	if r.WirelessMissing {
		add(Critical, "wireless-missing", "netifd lost its network.wireless object: the radios run, but nothing sees them or their clients until the network restarts")
	}
	if u := r.Agent; u != nil && u.Update != nil && (u.Update.State == "failed" || u.Update.State == "rolled-back") {
		add(Warning, "agent-update", fmt.Sprintf("the agent update to %s %s: %s", u.Update.Version, u.Update.State, u.Update.Why))
	}
	for _, net := range sortedKeys(r.Transports) {
		t := r.Transports[net]
		switch {
		case t.Active == "none":
			add(Critical, "no-transport", fmt.Sprintf("network %s has no transport in its bridge: its clients reach nothing", net))
		case t.CannotSwitch != "":
			add(Warning, "cannot-switch", fmt.Sprintf("network %s cannot switch: %s", net, t.CannotSwitch))
		case t.Active == "fallback":
			add(Warning, "on-fallback", fmt.Sprintf("network %s runs on its fallback: its primary is down", net))
		}
	}
	if x := r.VXLAN; x != nil {
		for _, t := range x.Tunnels {
			if !t.Standby && t.Probe != nil && t.Probe.Verdict == "down" {
				add(Warning, "tunnel-down", fmt.Sprintf("tunnel VNI %d to %s is down", t.VNI, t.Peer))
			}
		}
		for _, l := range x.Loops {
			add(Critical, "loop", fmt.Sprintf("port %s loops on VNI %d: the loop guard took it off its tunnels", l.Port, l.VNI))
		}
	}
	for _, v := range r.UplinkVLANs {
		if v.Verdict == "silent" {
			add(Warning, "vlan-silent", fmt.Sprintf("VLAN %d is silent on the uplink: the switch port may not carry it", v.VLAN))
		}
	}
	for _, net := range sortedKeys(r.DHCP) {
		if d := r.DHCP[net]; d.Unanswered > 0 && d.Answered == 0 {
			add(Warning, "dhcp-silent", fmt.Sprintf("DHCP on network %s: %d requests in the last 10 minutes, and nothing answered", net, d.Unanswered))
		}
	}
	if t := r.Time; t != nil && t.Synced != nil && !*t.Synced {
		add(Info, "clock", "the clock is not synced: logs and key expiry may be off")
	}
	return out
}

func seenAt(s *conditions.Seen) *time.Time {
	if s == nil {
		return nil
	}
	return &s.At
}

// ago writes a duration the way people say it: 12 minutes, 3 hours, 2 days.
func ago(d time.Duration) string {
	switch {
	case d < 2*time.Minute:
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	case d < 2*time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	return fmt.Sprintf("%d days", int(d.Hours()/24))
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
