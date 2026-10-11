package conditions

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// ConnectionsSince (0130): the attempts since a time on the APs named, but
// for the clients named, the latest first and no more than asked for. No
// client to leave out is none left out; no AP named is no attempt.
func TestConnectionsSince(t *testing.T) {
	s, clock := open(t)
	now := *clock
	one := func(ap, mac string, ago time.Duration) {
		t.Helper()
		_, err := s.RecordConnections(hierarchy.NodeID(ap), []Connection{{MAC: mac, Started: now.Add(-ago), Network: "sweet", SSID: "Sweet Spot",
			Band: "5g", Outcome: "online", Stage: "dns", Record: json.RawMessage(`{"events":[]}`)}})
		must(t, err)
	}
	one("ap-1", "aa:00:00:00:00:01", 50*time.Minute)
	one("ap-1", "aa:00:00:00:00:02", 40*time.Minute)
	one("ap-2", "aa:00:00:00:00:03", 30*time.Minute)
	one("ap-1", "aa:00:00:00:00:0f", 20*time.Minute) // a test's own client
	one("ap-3", "aa:00:00:00:00:04", 10*time.Minute)
	one("ap-1", "aa:00:00:00:00:05", 30*time.Hour)
	got := func(aps []hierarchy.NodeID, not []string, limit int) string {
		t.Helper()
		list, err := s.ConnectionsSince(now.Add(-24*time.Hour), aps, not, limit)
		must(t, err)
		var out []string
		for _, c := range list {
			out = append(out, string(c.AP)+" "+c.MAC[len(c.MAC)-2:])
		}
		return strings.Join(out, ", ")
	}
	for name, c := range map[string]struct {
		aps   []hierarchy.NodeID
		not   []string
		limit int
		want  string
	}{
		"two APs, the test client left out": {[]hierarchy.NodeID{"ap-1", "ap-2"}, []string{"aa:00:00:00:00:0f"}, 10, "ap-2 03, ap-1 02, ap-1 01"},
		"no client to leave out":            {[]hierarchy.NodeID{"ap-1", "ap-2"}, nil, 10, "ap-1 0f, ap-2 03, ap-1 02, ap-1 01"},
		"the latest two":                    {[]hierarchy.NodeID{"ap-1", "ap-2", "ap-3"}, []string{"aa:00:00:00:00:0f"}, 2, "ap-3 04, ap-2 03"},
		"the latest two of one AP":          {[]hierarchy.NodeID{"ap-1"}, []string{"aa:00:00:00:00:0f"}, 2, "ap-1 02, ap-1 01"},
		"no AP named":                       {nil, nil, 10, ""},
		"an AP with none since":             {[]hierarchy.NodeID{"ap-9"}, nil, 10, ""},
		"every client of an AP left out":    {[]hierarchy.NodeID{"ap-3"}, []string{"aa:00:00:00:00:04"}, 10, ""},
	} {
		if g := got(c.aps, c.not, c.limit); g != c.want {
			t.Errorf("%s:\n got %s\nwant %s", name, g, c.want)
		}
	}
}
