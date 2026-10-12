package conditions

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// What was timed of a network's steps, as kept.
func stepsOf(t *testing.T, s *Store, since time.Time) string {
	t.Helper()
	var out []string
	must(t, s.StepsSince(since, func(h StepHour) error {
		out = append(out, fmt.Sprintf("%s %s %s %s n %d ms %g most %g failed %d", h.At.Format("15:04"), h.AP, h.Network, h.Step, h.N, h.MS, h.Most, h.Failed))
		return nil
	}))
	// Within an hour, the order is the database's own.
	slices.Sort(out)
	return strings.Join(out, "\n")
}

// How long a network took to answer (0141): each thing an AP timed is
// counted into its hour of that network and step, with the sum of the
// times, the slowest, and how many times no answer came; and they go with
// the other trends.
func TestSteps(t *testing.T) {
	s, clock := open(t)
	ms := func(v float64) *float64 { return &v }
	at := time.Date(2026, 9, 26, 12, 10, 0, 0, time.UTC)
	must(t, s.AddSteps("ap-1", at, []StepTime{{"sweet", "dhcp", ms(300)}, {"sweet", "gateway", ms(2)}, {"sweet", "dns", nil}}))
	must(t, s.AddSteps("ap-1", at.Add(20*time.Minute), []StepTime{{"sweet", "dhcp", ms(100)}, {"sweet", "dns", ms(40)}}))
	must(t, s.AddSteps("ap-1", at.Add(70*time.Minute), []StepTime{{"sweet", "dhcp", ms(50)}}))
	must(t, s.AddSteps("ap-2", at, []StepTime{{"guest", "aaa", ms(18.5)}}))
	must(t, s.AddSteps("ap-2", at, nil))
	want := strings.Join([]string{
		"12:00 ap-1 sweet dhcp n 2 ms 400 most 300 failed 0",
		"12:00 ap-1 sweet dns n 1 ms 40 most 40 failed 1",
		"12:00 ap-1 sweet gateway n 1 ms 2 most 2 failed 0",
		"12:00 ap-2 guest aaa n 1 ms 18.5 most 18.5 failed 0",
		"13:00 ap-1 sweet dhcp n 1 ms 50 most 50 failed 0",
	}, "\n")
	if got := stepsOf(t, s, at); got != want {
		t.Errorf("as kept:\n got\n%s\nwant\n%s", got, want)
	}
	if got := stepsOf(t, s, at.Add(55*time.Minute)); got != "13:00 ap-1 sweet dhcp n 1 ms 50 most 50 failed 0" {
		t.Errorf("since the second hour:\n%s", got)
	}
	if kept, err := s.StepsKept("signin", "dhcp"); err != nil || !kept {
		t.Errorf("something of a test was timed: %v %v", kept, err)
	}
	if kept, err := s.StepsKept("signin"); err != nil || kept {
		t.Errorf("no sign-in was timed: %v %v", kept, err)
	}
	// They go with the other trends, by when their hour began.
	*clock = at.Add(26 * time.Hour)
	n, err := s.TrimTrends(25*time.Hour + 30*time.Minute)
	must(t, err)
	if got := stepsOf(t, s, time.Time{}); n != 4 || got != "13:00 ap-1 sweet dhcp n 1 ms 50 most 50 failed 0" {
		t.Errorf("after the trim, %d gone:\n%s", n, got)
	}
}

// A store from before them gets its table.
func TestMigratedStoreGetsSteps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conditions.db")
	s, err := Open(path, nil)
	must(t, err)
	must(t, s.Close())
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path))
	must(t, err)
	for _, q := range []string{`DROP TABLE step_hours`, `PRAGMA user_version = 19`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	must(t, db.Close())
	s, err = Open(path, nil)
	must(t, err)
	defer s.Close()
	v := 12.5
	must(t, s.AddSteps("ap-1", time.Now(), []StepTime{{"sweet", "dns", &v}}))
	if kept, err := s.StepsKept("dns"); err != nil || !kept {
		t.Errorf("after the migration: %v %v", kept, err)
	}
}
