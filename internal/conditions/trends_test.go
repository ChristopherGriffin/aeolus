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

// trendsOf says the trends a store holds, in words, for comparing.
func trendsOf(t *testing.T, s *Store, since time.Time) string {
	t.Helper()
	var out []string
	must(t, s.UsesByHour(since, func(u Use) error {
		out = append(out, fmt.Sprintf("use %s %s clients %d down %d up %d", u.AP, u.At.Format("01-02T15:04"), u.Clients, u.Down, u.Up))
		return nil
	}))
	totals, err := s.ClientTotalsByDay(since)
	must(t, err)
	var cl []string
	for _, c := range totals {
		at := "-"
		if !c.HostAt.IsZero() {
			at = c.HostAt.Format("01-02T15:04:05.000")
		}
		cl = append(cl, fmt.Sprintf("client %s %s %q %s down %d up %d", c.AP, c.MAC, c.Host, at, c.Down, c.Up))
	}
	// The order of the clients is the database's own.
	for i := range cl {
		for j := i + 1; j < len(cl); j++ {
			if cl[j] < cl[i] {
				cl[i], cl[j] = cl[j], cl[i]
			}
		}
	}
	out = append(out, cl...)
	for _, ap := range []hierarchy.NodeID{"ap-1", "ap-2"} {
		pts, err := s.HealthByHour(ap, since)
		must(t, err)
		for _, p := range pts {
			v := func(x any) string {
				switch x := x.(type) {
				case *int:
					if x != nil {
						return fmt.Sprint(*x)
					}
				case *int64:
					if x != nil {
						return fmt.Sprint(*x)
					}
				case *float64:
					if x != nil {
						return fmt.Sprint(*x)
					}
				}
				return "-"
			}
			out = append(out, fmt.Sprintf("health %s %s clients %d cpu %s load %s mem %s of %s storage %s of %s temp %s procs %s", ap, p.At.Format("01-02T15:04"),
				p.Clients, v(p.CPU), v(p.Load), v(p.MemAvailable), v(p.MemTotal), v(p.StorageFree), v(p.StorageTotal), v(p.Temp), v(p.Procs)))
		}
	}
	return strings.Join(out, "\n")
}

// seedTrends has two APs report over three hours and two days: usage, what
// each client moved, and health, some of it not said.
func seedTrends(t *testing.T, s *Store, clock *time.Time) time.Time {
	t.Helper()
	start := time.Date(2026, 9, 26, 22, 10, 0, 0, time.UTC)
	const a, b = "aa:00:00:00:00:0a", "aa:00:00:00:00:0b"
	i, f := func(v int) *int { return &v }, func(v float64) *float64 { return &v }
	l := func(v int64) *int64 { return &v }
	for _, r := range []struct {
		after   time.Duration
		ap      hierarchy.NodeID
		clients int
		moved   []ClientUse
		health  HealthPoint
	}{
		{0, "ap-1", 2, []ClientUse{{a, "phone", 100, 10}, {b, "", 5, 5}}, HealthPoint{Clients: 2, CPU: i(10), Load: f(0.5), MemTotal: l(1000), MemAvailable: l(600), Temp: f(50)}},
		{5*time.Minute + 123456789*time.Nanosecond, "ap-1", 3, []ClientUse{{a, "", 200, 20}}, HealthPoint{Clients: 3, CPU: i(40), MemTotal: l(1000), MemAvailable: l(400), StorageTotal: l(64), StorageFree: l(30)}},
		{10 * time.Minute, "ap-2", 1, []ClientUse{{a, "phone-2", 7, 7}}, HealthPoint{Clients: 1}},
		// The next hour, the same day; then the day after.
		{55 * time.Minute, "ap-1", 1, []ClientUse{{a, "", 1000, 100}}, HealthPoint{Clients: 1, CPU: i(5), Load: f(0.1), MemTotal: l(1000), MemAvailable: l(700), Temp: f(61.5), Procs: i(80)}},
		{2 * time.Hour, "ap-1", 4, []ClientUse{{a, "phone-new", 1, 1}, {b, "", 50, 0}}, HealthPoint{Clients: 4, CPU: i(90), Temp: f(40)}},
		{2*time.Hour + 5*time.Minute, "ap-1", 0, nil, HealthPoint{}},
	} {
		*clock = start.Add(r.after)
		must(t, s.RecordUse(r.ap, r.clients, r.moved))
		must(t, s.RecordHealth(r.ap, r.health, nil))
	}
	return start
}

// The trends (0140): each report is counted into its AP's hour and each
// client's day as it comes; an hour's health is the worst said in it; and
// they outlast the reports' own rows, and go when their own time is up.
func TestTrends(t *testing.T) {
	s, clock := open(t)
	start := seedTrends(t, s, clock)
	want := strings.Join([]string{
		"use ap-1 09-26T22:00 clients 3 down 305 up 35",
		"use ap-2 09-26T22:00 clients 1 down 7 up 7",
		"use ap-1 09-26T23:00 clients 1 down 1000 up 100",
		"use ap-1 09-27T00:00 clients 4 down 51 up 1",
		// By whole days, and the name it gave last on that AP.
		`client ap-1 aa:00:00:00:00:0a "phone-new" 09-27T00:10:00.000 down 1301 up 131`,
		`client ap-1 aa:00:00:00:00:0b "" - down 55 up 5`,
		`client ap-2 aa:00:00:00:00:0a "phone-2" 09-26T22:20:00.000 down 7 up 7`,
		// The worst of each hour; what no report in it said is not said.
		"health ap-1 09-26T22:00 clients 3 cpu 40 load 0.5 mem 400 of 1000 storage 30 of 64 temp 50 procs -",
		"health ap-1 09-26T23:00 clients 1 cpu 5 load 0.1 mem 700 of 1000 storage - of - temp 61.5 procs 80",
		"health ap-1 09-27T00:00 clients 4 cpu 90 load - mem - of - storage - of - temp 40 procs -",
		"health ap-2 09-26T22:00 clients 1 cpu - load - mem - of - storage - of - temp - procs -",
	}, "\n")
	if got := trendsOf(t, s, start); got != want {
		t.Errorf("the trends:\n got\n%s\nwant\n%s", got, want)
	}
	// From a time within an hour, that hour whole; from a time within a
	// day, that day whole.
	later := trendsOf(t, s, start.Add(80*time.Minute))
	if !strings.Contains(later, "use ap-1 09-26T23:00") || strings.Contains(later, "09-26T22:00") || !strings.Contains(later, "down 1301 up 131") {
		t.Errorf("from within the second hour:\n%s", later)
	}
	if from, ok, err := s.UsageHoursFrom(); err != nil || !ok || !from.Equal(time.Date(2026, 9, 26, 22, 0, 0, 0, time.UTC)) {
		t.Errorf("the oldest hour: %v %v %v", from, ok, err)
	}

	// What each client moved over a span: from the reports' own rows while
	// they reach back to its start, to the report.
	over := func(since time.Time) string {
		t.Helper()
		totals, err := s.ClientTotalsOver(since)
		must(t, err)
		var out []string
		for _, c := range totals {
			out = append(out, fmt.Sprintf("%s %s %d", c.AP, c.MAC[len(c.MAC)-2:], c.Down))
		}
		for i := range out {
			for j := i + 1; j < len(out); j++ {
				if out[j] < out[i] {
					out[i], out[j] = out[j], out[i]
				}
			}
		}
		return strings.Join(out, ", ")
	}
	if got, want := over(start.Add(30*time.Minute)), "ap-1 0a 1001, ap-1 0b 50"; got != want {
		t.Errorf("the clients since half an hour in, the rows kept:\n got %s\nwant %s", got, want)
	}

	// The reports' own rows go, and the trends stay.
	*clock = start.Add(40 * 24 * time.Hour)
	_, err := s.TrimStates(30 * 24 * time.Hour)
	must(t, err)
	n := 0
	must(t, s.Uses(time.Time{}, func(Use) error { n++; return nil }))
	if n != 0 {
		t.Errorf("usage rows after the trim: %d", n)
	}
	if got := trendsOf(t, s, start); got != want {
		t.Errorf("the trends after the reports went:\n%s", got)
	}
	// And the clients are counted by whole days, from the first wholly in
	// the span: never more than was moved in it.
	if got, want := over(start.Add(30*time.Minute)), "ap-1 0a 1, ap-1 0b 50"; got != want {
		t.Errorf("the clients since half an hour in, the rows gone:\n got %s\nwant %s", got, want)
	}
	if got, want := over(time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)), "ap-1 0a 1301, ap-1 0b 55, ap-2 0a 7"; got != want {
		t.Errorf("the clients since the start of a day, the rows gone:\n got %s\nwant %s", got, want)
	}
	// Their own time: what began more than it ago goes, the first day here
	// and none of the second.
	*clock = time.Date(2026, 9, 27, 0, 30, 0, 0, time.UTC).Add(24 * time.Hour)
	gone, err := s.TrimTrends(24*time.Hour + 30*time.Minute)
	must(t, err)
	left := trendsOf(t, s, time.Time{})
	if gone != 9 || strings.Contains(left, "09-26T") || !strings.Contains(left, "use ap-1 09-27T00:00 clients 4") || !strings.Contains(left, "down 1 up 1") || !strings.Contains(left, "health ap-1 09-27T00:00") {
		t.Errorf("after the trends' own trim, %d gone:\n%s", gone, left)
	}
}

// A store from before the trends gets them, counted from the rows it has:
// the same as if each report had been counted as it came.
func TestMigratedStoreCountsTrends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conditions.db")
	clock := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	s, err := Open(path, func() time.Time { return clock })
	must(t, err)
	start := seedTrends(t, s, &clock)
	want := trendsOf(t, s, start)
	must(t, s.Close())
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path))
	must(t, err)
	for _, q := range []string{`DROP TABLE usage_hours`, `DROP TABLE client_usage_days`, `DROP TABLE health_hours`, `PRAGMA user_version = 18`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	must(t, db.Close())
	s, err = Open(path, func() time.Time { return clock })
	must(t, err)
	defer s.Close()
	if got := trendsOf(t, s, start); got != want {
		t.Errorf("counted from the rows kept:\n got\n%s\nwant\n%s", got, want)
	}
	// And a report after is counted into what was found.
	clock = start.Add(2*time.Hour + 10*time.Minute)
	must(t, s.RecordUse("ap-1", 9, []ClientUse{{MAC: "aa:00:00:00:00:0b", Host: "named", Down: 1, Up: 1}}))
	got := trendsOf(t, s, start)
	if !strings.Contains(got, "use ap-1 09-27T00:00 clients 9 down 52 up 2") || !strings.Contains(got, `client ap-1 aa:00:00:00:00:0b "named" 09-27T00:20:00.000 down 56 up 6`) {
		t.Errorf("a report after the migration:\n%s", got)
	}
}
