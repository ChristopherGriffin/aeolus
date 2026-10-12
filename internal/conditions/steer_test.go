package conditions

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Steering's history (0139): an AP says each asking in report after
// report, and it is kept once; the askings of a time are counted for each
// AP, network and way, with how many there were; the clients asked most
// are listed; and they go when the attempts of their time do.
func TestSteer(t *testing.T) {
	s, clock := open(t)
	now := *clock
	one := func(mac, network, method, asked, outcome string, ago time.Duration) SteerEvent {
		at := now.Add(-ago)
		return SteerEvent{At: at, Said: at.Unix(), MAC: mac, Network: network, Method: method, From: "2g", To: []string{"6g", "5g"}, Asked: asked, Outcome: outcome, TookMS: 2500}
	}
	const a, b, c, own = "aa:00:00:00:00:0a", "aa:00:00:00:00:0b", "aa:00:00:00:00:0c", "aa:00:00:00:00:0f"
	first := []SteerEvent{
		one(a, "sweet", "suggest", "btm", "moved", 50*time.Minute),
		one(a, "sweet", "suggest", "none", "back", 48*time.Minute), // what became of the one before: no asking
		one(b, "sweet", "suggest", "btm", "refused", 40*time.Minute),
		one(b, "sweet", "suggest", "btm", "refused", 30*time.Minute),
		one(b, "sweet", "hold", "held", "joined", 29*time.Minute),
		one(c, "guest", "sticky", "kick", "back", 20*time.Minute), // a weak client's asking ended so
		one(own, "sweet", "suggest", "btm", "moved", 10*time.Minute),
		one(a, "sweet", "suggest", "btm", "moved", 30*time.Hour),
	}
	code, signal := 1, -71
	first[2].Answer, first[2].Code, first[2].Signal = "refused", &code, &signal
	n, err := s.AddSteer("ap-1", first)
	must(t, err)
	if n != len(first) {
		t.Fatalf("kept %d of %d", n, len(first))
	}
	// Said again, with one more, and by another AP: only what is new is kept.
	n, err = s.AddSteer("ap-1", append(first[3:], one(c, "sweet", "suggest", "btm", "stayed", 5*time.Minute)))
	must(t, err)
	if n != 1 {
		t.Errorf("said again with one more: kept %d, want 1", n)
	}
	n, err = s.AddSteer("ap-2", []SteerEvent{one(a, "sweet", "suggest", "btm", "moved", 50*time.Minute)})
	must(t, err)
	if n != 1 {
		t.Errorf("the same by another AP: kept %d, want 1", n)
	}
	n, err = s.AddSteer("ap-2", nil)
	must(t, err)
	if n != 0 {
		t.Errorf("none said: kept %d", n)
	}

	since, both, not := now.Add(-24*time.Hour), []hierarchy.NodeID{"ap-1", "ap-2"}, []string{own}
	list, err := s.SteerSince(since, both, not, 3)
	must(t, err)
	var got []string
	for _, e := range list {
		got = append(got, fmt.Sprintf("%s %s %s %s", e.AP, e.MAC[len(e.MAC)-2:], e.Method, e.Outcome))
	}
	if g, want := strings.Join(got, ", "), "ap-1 0c suggest stayed, ap-1 0c sticky back, ap-1 0b hold joined"; g != want {
		t.Errorf("the latest three:\n got %s\nwant %s", g, want)
	}
	all, err := s.SteerSince(since, both, nil, 100)
	must(t, err)
	if len(all) != 9 {
		t.Errorf("the day's, no client left out: %d, want 9", len(all))
	}
	for _, e := range all {
		if e.MAC == b && e.Outcome == "refused" && e.Code != nil {
			if e.Answer != "refused" || *e.Code != 1 || e.Signal == nil || *e.Signal != -71 || e.TookMS != 2500 || strings.Join(e.To, ",") != "6g,5g" || e.From != "2g" || !e.At.Equal(now.Add(-40*time.Minute)) || e.Said != now.Add(-40*time.Minute).Unix() {
				t.Errorf("an asking, as kept: %+v", e)
			}
		}
	}
	if none, err := s.SteerSince(since, nil, nil, 100); err != nil || len(none) != 0 {
		t.Errorf("no AP named: %v %v", none, err)
	}

	counts, err := s.SteerCounts(since, both, not)
	must(t, err)
	got = nil
	for _, k := range counts {
		got = append(got, fmt.Sprintf("%s %s %s %s %d", k.AP, k.Network, k.Method, k.Outcome, k.N))
	}
	want := []string{
		"ap-1 guest sticky back 1",
		"ap-1 sweet hold joined 1",
		"ap-1 sweet suggest back 1", "ap-1 sweet suggest moved 1", "ap-1 sweet suggest refused 2", "ap-1 sweet suggest stayed 1",
		"ap-2 sweet suggest moved 1",
		// How many askings: a client moved up that came back down is one
		// asking; a weak client that came straight back is one too.
		"ap-1 guest sticky asked 1", "ap-1 sweet hold asked 1", "ap-1 sweet suggest asked 4", "ap-2 sweet suggest asked 1",
	}
	if g, w := strings.Join(got, "\n"), strings.Join(want, "\n"); g != w {
		t.Errorf("counts:\n got\n%s\nwant\n%s", g, w)
	}

	clients, err := s.SteerClients(since, both, not, 2)
	must(t, err)
	got = nil
	for _, k := range clients {
		got = append(got, fmt.Sprintf("%s asked %d moved %d refused %d back %d", k.MAC[len(k.MAC)-2:], k.Asked, k.Moved, k.Refused, k.Back))
	}
	if g, want := strings.Join(got, "; "), "0b asked 3 moved 0 refused 2 back 0; 0c asked 2 moved 0 refused 0 back 1"; g != want {
		t.Errorf("the two asked most:\n got %s\nwant %s", g, want)
	}
	if !clients[0].Last.Equal(now.Add(-29 * time.Minute)) {
		t.Errorf("when the first was last asked: %v", clients[0].Last)
	}

	// The host names of those last seen on an AP named.
	must(t, s.SeeClients("ap-1", []ClientSeen{{MAC: a, Host: "phone"}, {MAC: b}}))
	must(t, s.SeeClients("ap-3", []ClientSeen{{MAC: c, Host: "elsewhere"}}))
	hosts, err := s.Hosts([]string{a, b, c}, both)
	must(t, err)
	if len(hosts) != 1 || hosts[a] != "phone" {
		t.Errorf("host names: %v", hosts)
	}

	// They go with the attempts of their time.
	*clock = now.Add(2 * time.Hour)
	_, err = s.TrimConnections(24 * time.Hour)
	must(t, err)
	left, err := s.SteerSince(time.Time{}, both, nil, 100)
	must(t, err)
	if len(left) != 9 {
		t.Errorf("kept after the trim: %d, want 9", len(left))
	}
}

// A store from before steering's history gets its table, and keeps what it
// had.
func TestMigratedStoreKeepsSteer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conditions.db")
	s, err := Open(path, nil)
	must(t, err)
	must(t, s.SeeClients("ap-1", []ClientSeen{{MAC: "aa:00:00:00:00:0a", Host: "phone"}}))
	must(t, s.Close())
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path))
	must(t, err)
	for _, q := range []string{`DROP TABLE steer_events`, `PRAGMA user_version = 17`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	must(t, db.Close())
	s, err = Open(path, nil)
	must(t, err)
	defer s.Close()
	at := time.Now()
	n, err := s.AddSteer("ap-1", []SteerEvent{{At: at, Said: at.Unix(), MAC: "aa:00:00:00:00:0a", Network: "sweet", Method: "suggest", From: "2g", Asked: "btm", Outcome: "moved"}})
	must(t, err)
	if n != 1 {
		t.Errorf("kept %d after the migration", n)
	}
	if hosts, err := s.Hosts([]string{"aa:00:00:00:00:0a"}, []hierarchy.NodeID{"ap-1"}); err != nil || hosts["aa:00:00:00:00:0a"] != "phone" {
		t.Errorf("what it had: %v %v", hosts, err)
	}
}
