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
