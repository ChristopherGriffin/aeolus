package api

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/conditions"
)

// GET /v1/usage (0108): what the APs' clients moved, worked out from each
// report and the one before, and the most clients each AP had.
func TestUsage(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	client := func(mac string, connected, rx, tx int) map[string]any {
		return map[string]any{"mac": mac, "network": "sweet", "ssid": "Sweet Spot", "band": "5g", "signal": -60, "signal_avg": -60,
			"tx_rate": 400, "rx_rate": 300, "connected": connected, "inactive_ms": 10, "rx_bytes": rx, "tx_bytes": tx, "rx_packets": 1, "tx_packets": 1}
	}
	for i, clients := range [][]any{
		// It joined in the last interval: all it moved counts.
		{client("7e:2a:ea:9b:2b:8f", 30, 1000, 5000)},
		// It moved 4000 down and 500 up since; another, on for two hours,
		// was not in the last report and counts nothing yet.
		{client("7e:2a:ea:9b:2b:8f", 330, 1500, 9000), client("84:0d:8e:5a:df:f7", 7200, 10, 10)},
		// It joined again: from zero.
		{client("7e:2a:ea:9b:2b:8f", 20, 100, 200)},
	} {
		if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60 + i, "clients": clients}, nil); code != 200 {
			t.Fatalf("state %d: %d %v", i, code, body)
		}
	}
	code, body := f.do("GET", "/v1/usage?under=office&hours=24", "griff", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	if body["down"] != 9200.0 || body["up"] != 1600.0 || len(body["buckets"].([]any)) != 48 || body["bucket"] != 1800.0 {
		t.Fatalf("usage = down %v up %v, %d buckets of %v s", body["down"], body["up"], len(body["buckets"].([]any)), body["bucket"])
	}
	aps := body["aps"].([]any)
	if a := aps[0].(map[string]any); a["ap"] != ap || a["peak"] != 2.0 || a["down"] != 9200.0 {
		t.Fatalf("aps = %v", aps)
	}
	if code, _ := f.do("GET", "/v1/usage?hours=0", "griff", nil); code != 400 {
		t.Fatalf("hours=0: %d", code)
	}
	// office has no role in Locations: no APs, nothing moved.
	if _, body := f.do("GET", "/v1/usage", "office", nil); len(body["aps"].([]any)) != 0 || body["down"] != 0.0 {
		t.Fatalf("office = %v", body)
	}
}

// After a gap longer than usageGap, a client that was there before counts
// nothing, as the gap's bytes would all fall in one bucket; one that joined
// in the last interval counts all it moved.
func TestUsageOfAGap(t *testing.T) {
	now := time.Now()
	prev, _ := json.Marshal(map[string]any{"clients": []map[string]any{{"mac": "7e:2a:ea:9b:2b:8f", "connected": 1000, "rx_bytes": 1000, "tx_bytes": 1000}}})
	down, up := usageOf(&conditions.State{At: now.Add(-2 * time.Hour), Report: prev}, now, []wifiClient{
		{MAC: "7e:2a:ea:9b:2b:8f", Connected: 8200, RxBytes: 9e9, TxBytes: 9e9},
		{MAC: "84:0d:8e:5a:df:f7", Connected: 120, RxBytes: 300, TxBytes: 700},
	})
	if down != 700 || up != 300 {
		t.Fatalf("down %d, up %d", down, up)
	}
}

// Never more than 48 buckets, for any span.
func TestUsageBuckets(t *testing.T) {
	f := newFixture(t)
	for _, hours := range []int{1, 5, 6, 7, 13, 24, 25, 168, 719, 720} {
		_, body := f.do("GET", "/v1/usage?hours="+fmt.Sprint(hours), "griff", nil)
		n, b := len(body["buckets"].([]any)), body["bucket"].(float64)
		if n > 48 || float64(n)*b < float64(hours*3600) || int(b)%60 != 0 {
			t.Fatalf("hours=%d: %d buckets of %v s", hours, n, b)
		}
	}
}

// A counter that went back without the client joining again, as a 32-bit
// one does when it wraps, counts nothing that time.
func TestUsageOfAWrap(t *testing.T) {
	now := time.Now()
	prev, _ := json.Marshal(map[string]any{"clients": []map[string]any{{"mac": "7e:2a:ea:9b:2b:8f", "connected": 1000, "rx_bytes": 4294967000, "tx_bytes": 10}}})
	down, up := usageOf(&conditions.State{At: now.Add(-5 * time.Minute), Report: prev}, now,
		[]wifiClient{{MAC: "7e:2a:ea:9b:2b:8f", Connected: 1300, RxBytes: 200, TxBytes: 50}})
	if down != 0 || up != 0 {
		t.Fatalf("down %d, up %d", down, up)
	}
}
