package journey

import (
	"strings"
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

var t0 = time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)

func dbm(v int) *int { return &v }

func sample(ap string, min int, band string, signal int, c Client) Sample {
	c.Band, c.Signal = band, dbm(signal)
	if c.Network == "" {
		c.Network, c.SSID = "lab", "Aeolus Lab"
	}
	return Sample{AP: hierarchy.NodeID("ap-" + ap), APName: ap, At: t0.Add(time.Duration(min) * time.Minute), C: c}
}

func TestSessionsAndRoams(t *testing.T) {
	j := Build("aa:bb:cc:00:11:22", []Sample{
		sample("office", 0, "5g", -55, Client{Connected: 120, TxRate: 400, Host: "griffs-phone"}),
		sample("office", 5, "5g", -58, Client{TxRate: 300}),
		// Roamed to the C-360 at 6 GHz.
		sample("c360", 10, "6g", -50, Client{TxRate: 1200}),
		sample("c360", 15, "6g", -52, Client{TxRate: 1100}),
		// Gone for an hour, then back on the office AP: a new session, not a roam.
		sample("office", 80, "5g", -60, Client{TxRate: 350}),
	})
	if len(j.Sessions) != 3 || j.Roams != 1 || j.Host != "griffs-phone" {
		t.Fatalf("journey = %+v", j)
	}
	s := j.Sessions[0]
	if !s.From.Equal(t0.Add(-2*time.Minute)) || !s.To.Equal(t0.Add(5*time.Minute)) || *s.SignalMin != -58 || *s.SignalMax != -55 || s.TxRate != 350 {
		t.Fatalf("first session = %+v", s)
	}
	if j.Sessions[1].From.Before(j.Sessions[0].To) {
		t.Fatalf("the second session starts before the first ended: %+v", j.Sessions[1])
	}
	if len(j.Issues) != 0 {
		t.Fatalf("issues = %+v", j.Issues)
	}
}

func TestIssues(t *testing.T) {
	var samples []Sample
	// Back and forth between two APs, each visit weak.
	for i := 0; i < 6; i++ {
		ap := "office"
		if i%2 == 1 {
			ap = "pumphouse"
		}
		samples = append(samples, sample(ap, i*5, "5g", -80, Client{TxRate: 20, TxPackets: 1000, TxRetries: 400, DHCP: "none"}))
	}
	j := Build("aa:bb:cc:00:11:22", samples)
	got := map[string]bool{}
	for _, is := range j.Issues {
		got[is.Kind] = true
	}
	for _, k := range []string{"weak-signal", "retries", "no-dhcp", "ping-pong"} {
		if !got[k] {
			t.Errorf("no %s issue: %+v", k, j.Issues)
		}
	}
	for _, is := range j.Issues {
		if is.Kind == "ping-pong" && !strings.Contains(is.Message, "office and pumphouse") {
			t.Fatalf("ping-pong = %q", is.Message)
		}
	}
}

func TestEmpty(t *testing.T) {
	if j := Build("aa:bb:cc:00:11:22", nil); len(j.Sessions) != 0 || len(j.Issues) != 0 {
		t.Fatalf("journey = %+v", j)
	}
}

// From the review of #133: the AP's own SSIDs are kept apart; retries are
// counted as the Clients tab counts them; a session without a band is not
// slow on 5 GHz; and three roams are not yet back and forth.
func TestReviewFixes(t *testing.T) {
	own := func(ssid string, min int) Sample {
		s := sample("office", min, "5g", -60, Client{TxRate: 300})
		s.C.Network, s.C.SSID = "", ssid
		return s
	}
	if j := Build("aa:bb:cc:00:11:22", []Sample{own("home", 0), own("guest", 5)}); len(j.Sessions) != 2 {
		t.Fatalf("two of the AP's own SSIDs: %+v", j.Sessions)
	}
	j := Build("aa:bb:cc:00:11:22", []Sample{sample("office", 0, "5g", -60, Client{TxRate: 300, TxPackets: 100, TxRetries: 25})})
	if j.Sessions[0].Retries == nil || *j.Sessions[0].Retries != 0.25 {
		t.Fatalf("retries = %v", j.Sessions[0].Retries)
	}
	j = Build("aa:bb:cc:00:11:22", []Sample{sample("office", 0, "", -60, Client{TxRate: 10}), sample("office", 5, "", -60, Client{TxRate: 10})})
	for _, is := range j.Issues {
		if is.Kind == "slow" {
			t.Fatalf("a session with no band is slow: %+v", is)
		}
	}
	var three []Sample
	for i, ap := range []string{"a", "b", "a", "b", "c"} {
		at := i * 5
		if ap == "c" {
			at = 300
		}
		three = append(three, sample(ap, at, "5g", -60, Client{TxRate: 300}))
	}
	for _, is := range Build("aa:bb:cc:00:11:22", three).Issues {
		if is.Kind == "ping-pong" {
			t.Fatalf("three roams called back and forth: %+v", is)
		}
	}
}
