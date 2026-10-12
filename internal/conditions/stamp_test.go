package conditions

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// stampBefore is stamp as it was before it wrote every digit: the trailing
// zeros of the second's fraction dropped.
func stampBefore(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// tenOfASecond is ten times of one second, the earliest first, the digits
// of each beginning the next's: the second itself, then .1, .12, and so on
// to .123456789. In the form before, each sorted after the next.
func tenOfASecond(second time.Time) []time.Time {
	out := []time.Time{second}
	for ns, unit := 0, 100_000_000; unit > 0; unit /= 10 {
		ns += len(out) * unit
		out = append(out, second.Add(time.Duration(ns)))
	}
	return out
}

// A time kept as text is as long as any other, sorts as time does, and
// reads back as the time it was; and SQL's stamped makes of either form
// what stamp writes.
func TestStamp(t *testing.T) {
	s, clock := open(t)
	times := append(tenOfASecond(*clock), clock.Add(500*time.Millisecond), clock.Add(time.Second-1), clock.Add(time.Second))
	for i, at := range times {
		text := stamp(at)
		if len(text) != stampLen {
			t.Errorf("%s is %d long, not %d", text, len(text), stampLen)
		}
		if back, err := time.Parse(time.RFC3339Nano, text); err != nil || !back.Equal(at) {
			t.Errorf("%s reads back as %v, %v", text, back, err)
		}
		for j, other := range times {
			if (text < stamp(other)) != at.Before(other) {
				t.Errorf("%d and %d: %s and %s do not sort as their times do", i, j, text, stamp(other))
			}
		}
		for _, kept := range []string{stampBefore(at), text} {
			var got string
			must(t, s.db.QueryRow(`SELECT `+stamped("?1"), kept).Scan(&got))
			if got != text {
				t.Errorf("stamped of %s is %s, not %s", kept, got, text)
			}
		}
	}
	if in := time.FixedZone("", 2*3600); stamp(clock.In(in)) != stamp(*clock) {
		t.Errorf("a time of another zone is kept as %s", stamp(clock.In(in)))
	}
}

// A look from a whole second finds what was recorded later in that second,
// a look up to one does not, and a trim to one leaves it. In the form
// before, the second's own time sorted after every other time of it.
func TestWholeSecond(t *testing.T) {
	s, clock := open(t)
	second := *clock
	*clock = second.Add(500 * time.Millisecond)
	const mac = "aa:bb:cc:dd:ee:01"
	aps := []hierarchy.NodeID{"ap-1"}
	must(t, s.RecordUse("ap-1", 1, []ClientUse{{MAC: mac, Host: "phone", Down: 10, Up: 5}}))
	must(t, s.RecordState("ap-1", 1, json.RawMessage(`{"clients":["`+mac+`"]}`)))
	_, err := s.RecordApply("ap-1", Apply{Version: 1, Hash: "aa", OK: true})
	must(t, err)
	test, err := s.AddAction("ap-1", "client-test", "", "griff")
	must(t, err)
	_, err = s.FinishAction("ap-1", test.ID, true, "")
	must(t, err)
	alert, err := s.BeginAlert(LoggedAlert{AP: "ap-1", Key: "silent", Kind: "silent", Severity: "warning", Message: "silent", Began: second.Add(-time.Hour)})
	must(t, err)
	must(t, s.EndAlert(alert, *clock))
	must(t, s.SaveRelayClients([]RelayClient{{Subnet: "192.168.50.1", MAC: mac, First: *clock, Last: *clock}}))
	_, err = s.SaveKnock(Knock{First: *clock, Last: *clock, Count: 1, Source: "192.168.50.9"})
	must(t, err)

	// What is kept of the half second, by each way of asking since a time.
	kept := func(since time.Time) string {
		t.Helper()
		var out []string
		uses := 0
		must(t, s.Uses(since, func(Use) error { uses++; return nil }))
		totals, err := s.ClientTotals(since)
		must(t, err)
		states := 0
		must(t, s.StatesWith(since, mac, func(hierarchy.NodeID, State) error { states++; return nil }))
		applies, err := s.AppliesSince("ap-1", since, 10)
		must(t, err)
		tests, err := s.ClientTests(since)
		must(t, err)
		alerts, err := s.AlertLog(since, aps, 10)
		must(t, err)
		for _, n := range []struct {
			what string
			n    int
		}{{"uses", uses}, {"client totals", len(totals)}, {"states", states}, {"applies", len(applies)}, {"tests", len(tests)}, {"alerts", len(alerts)}} {
			out = append(out, fmt.Sprintf("%s %d", n.what, n.n))
		}
		return strings.Join(out, ", ")
	}
	const all = "uses 1, client totals 1, states 1, applies 1, tests 1, alerts 1"
	if got := kept(second); got != all {
		t.Errorf("since the whole second:\n got %s\nwant %s", got, all)
	}
	if got, want := kept(second.Add(time.Second)), "uses 0, client totals 0, states 0, applies 0, tests 0, alerts 0"; got != want {
		t.Errorf("since the next second:\n got %s\nwant %s", got, want)
	}
	if was, err := s.Moved("ap-1", mac, second, second.Add(time.Second)); err != nil || !was {
		t.Errorf("moved from the whole second to the next: %v, %v", was, err)
	}
	if was, err := s.Moved("ap-1", mac, second.Add(-time.Minute), second); err != nil || was {
		t.Errorf("moved before the whole second: %v, %v", was, err)
	}

	// An hour on, a trim of what is older than an hour is a trim to the
	// whole second: nothing of the half second goes.
	*clock = second.Add(time.Hour)
	if n, err := s.TrimStates(time.Hour); err != nil || n != 0 {
		t.Errorf("trimmed %d states to the whole second, %v", n, err)
	}
	if n, err := s.PruneClientTests(second); err != nil || n != 0 {
		t.Errorf("forgot %d tests asked before the whole second, %v", n, err)
	}
	if n, err := s.TrimRelayClients(second); err != nil || n != 0 {
		t.Errorf("trimmed %d relay clients to the whole second, %v", n, err)
	}
	if n, err := s.TrimKnocks(second, 10); err != nil || n != 0 {
		t.Errorf("trimmed %d knocks to the whole second, %v", n, err)
	}
	if got := kept(second); got != all {
		t.Errorf("after the trims:\n got %s\nwant %s", got, all)
	}
}

// Of two times of one second where the digits of one begin the other's, as
// .5 and .51, the shorter is the earlier, and the second's own time, with
// none, the earliest. In the form before, each sorted after the next.
func TestPrefixPair(t *testing.T) {
	s, clock := open(t)
	second := *clock
	at := []time.Time{second, second.Add(500 * time.Millisecond), second.Add(510 * time.Millisecond)}
	between := second.Add(505 * time.Millisecond)
	const mac = "aa:bb:cc:dd:ee:01"
	aps := []hierarchy.NodeID{"ap-1"}
	// Recorded out of order, so the order of the rows tells nothing.
	for _, i := range []int{2, 0, 1} {
		*clock = at[i]
		must(t, s.RecordUse("ap-1", i+1, []ClientUse{{MAC: mac, Host: fmt.Sprintf("name-%d", i+1), Down: 1}}))
		must(t, s.RecordState("ap-1", int64(i+1), json.RawMessage(`{"clients":["`+mac+`"]}`)))
		_, err := s.SaveKnock(Knock{First: at[i], Last: at[i], Count: 1, Source: fmt.Sprintf("192.168.50.%d", i+1)})
		must(t, err)
		_, err = s.BeginAlert(LoggedAlert{AP: "ap-1", Key: fmt.Sprint(i + 1), Kind: "silent", Severity: "warning", Message: "silent", Began: at[i]})
		must(t, err)
	}
	*clock = at[2]

	uses := func(since time.Time) string {
		t.Helper()
		var out []string
		must(t, s.Uses(since, func(u Use) error { out = append(out, fmt.Sprint(u.Clients)); return nil }))
		return strings.Join(out, " ")
	}
	states := func(since time.Time) string {
		t.Helper()
		var out []string
		must(t, s.StatesWith(since, mac, func(_ hierarchy.NodeID, st State) error { out = append(out, fmt.Sprint(st.Version)); return nil }))
		return strings.Join(out, " ")
	}
	knocks := func() string {
		t.Helper()
		ks, err := s.Knocks()
		must(t, err)
		var out []string
		for _, k := range ks {
			out = append(out, k.Source[len(k.Source)-1:])
		}
		return strings.Join(out, " ")
	}

	// The oldest first, where that is the order asked for.
	if got := uses(second); got != "1 2 3" {
		t.Errorf("uses, oldest first: %s", got)
	}
	if got := states(second); got != "1 2 3" {
		t.Errorf("states, oldest first: %s", got)
	}
	if from, ok, err := s.UsageFrom(); err != nil || !ok || !from.Equal(second) {
		t.Errorf("the oldest usage kept is of %v, %v, %v", from, ok, err)
	}
	// The newest first, and the name given last.
	if got := knocks(); got != "3 2 1" {
		t.Errorf("knocks, newest first: %s", got)
	}
	alerts, err := s.AlertLog(second, aps, 10)
	must(t, err)
	var keys []string
	for _, a := range alerts {
		keys = append(keys, a.Key)
	}
	if got := strings.Join(keys, " "); got != "3 2 1" {
		t.Errorf("alerts, newest first: %s", got)
	}
	totals, err := s.ClientTotals(second)
	must(t, err)
	if len(totals) != 1 || totals[0].Down != 3 || totals[0].Host != "name-3" || !totals[0].HostAt.Equal(at[2]) {
		t.Errorf("client totals: %+v", totals)
	}

	// From a time between .5 and .51, and up to it.
	if got := uses(between); got != "3" {
		t.Errorf("uses since between the two: %s", got)
	}
	if got := states(between); got != "3" {
		t.Errorf("states since between the two: %s", got)
	}
	if totals, err := s.ClientTotals(between); err != nil || len(totals) != 1 || totals[0].Down != 1 {
		t.Errorf("client totals since between the two: %+v, %v", totals, err)
	}
	if was, err := s.Moved("ap-1", mac, at[1], between); err != nil || !was {
		t.Errorf("moved from .5 to between the two: %v, %v", was, err)
	}
	if was, err := s.Moved("ap-1", mac, between, at[2]); err != nil || was {
		t.Errorf("moved from between the two to .51: %v, %v", was, err)
	}

	// A trim to between the two takes the two before it.
	*clock = between.Add(time.Hour)
	if n, err := s.TrimStates(time.Hour); err != nil || n != 2 {
		t.Errorf("trimmed %d states to between the two, %v", n, err)
	}
	if got := uses(second); got != "3" {
		t.Errorf("uses after the trim: %s", got)
	}
	if n, err := s.TrimKnocks(between, 10); err != nil || n != 2 {
		t.Errorf("trimmed %d knocks to between the two, %v", n, err)
	}
	if got := knocks(); got != "3" {
		t.Errorf("knocks after the trim: %s", got)
	}
}

// A store in use before stamp wrote every digit keeps its times in the form
// that dropped them. Opening it rewrites them, so what it kept compares as
// what it keeps from then on does; but for those of checks and applies,
// which are kept as recorded, and are still found since a time.
func TestMigrateTimesKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conditions.db")
	second := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	at := tenOfASecond(second)
	s, err := Open(path, nil)
	must(t, err)
	must(t, s.Close())
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path))
	must(t, err)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`PRAGMA user_version = 18`)
	for i, when := range at {
		w := stampBefore(when)
		exec(`INSERT INTO usage (ap, at, clients, down, up) VALUES ('ap-1', ?, ?, 1, 0)`, w, i)
		exec(`INSERT INTO client_usage (ap, at, mac, host, down, up) VALUES ('ap-1', ?, 'aa:bb:cc:dd:ee:01', ?, 1, 0)`, w, fmt.Sprintf("name-%d", i))
		exec(`INSERT INTO states (ap, at, version, report) VALUES ('ap-1', ?, ?, '{"clients":["aa:bb:cc:dd:ee:01"]}')`, w, i)
		exec(`INSERT INTO alert_log (ap, key, kind, severity, message, began, ended) VALUES ('ap-1', ?, 'silent', 'warning', 'silent', ?, ?)`, fmt.Sprint(i), w, w)
		exec(`INSERT INTO actions (ap, kind, target, actor, at, state, done_at) VALUES ('ap-1', 'client-test', ?, 'griff', ?, 'done', ?)`, fmt.Sprint(i), w, w)
		exec(`INSERT INTO relay_clients (subnet, mac, relay, first_at, last_at, requests, type, host, vendor_class, params, address, circuit, remote)
			VALUES ('192.168.50.1', ?, '', ?, ?, 1, '', '', '', '', '', '', '')`, fmt.Sprintf("aa:bb:cc:dd:ee:%02d", i), w, w)
		exec(`INSERT INTO knocks (first_at, last_at, count, source, sni, versions, subject) VALUES (?, ?, 1, ?, '', '', '')`, w, w, fmt.Sprint(i))
		exec(`INSERT INTO checks (ap, at, version, hash, result, problems) VALUES ('ap-1', ?, ?, 'aa', 'ok', '[]')`, w, i)
		exec(`INSERT INTO applies (ap, at, version, hash, ok, error, checked) VALUES ('ap-1', ?, ?, 'aa', 1, '', 1)`, w, i)
	}
	// What has no time stays without one.
	exec(`INSERT INTO alert_log (ap, key, kind, severity, message, began) VALUES ('ap-1', 'open', 'silent', 'warning', 'silent', ?)`, stampBefore(at[1]))
	exec(`INSERT INTO actions (ap, kind, target, actor, at, state) VALUES ('ap-1', 'reboot', '', 'griff', ?, 'pending')`, stampBefore(at[1]))
	exec(`INSERT INTO aps (ap, seen_at, source, running, running_at) VALUES ('ap-1', ?, '10.0.0.1', 7, ?)`, stampBefore(at[1]), stampBefore(at[2]))
	exec(`INSERT INTO aps (ap, seen_at, source) VALUES ('ap-2', ?, '10.0.0.2')`, stampBefore(at[0]))
	// The test's own note of each time of a use that is rewritten.
	exec(`CREATE TABLE rewritten (at TEXT NOT NULL)`)
	exec(`CREATE TRIGGER usage_rewritten AFTER UPDATE ON usage BEGIN INSERT INTO rewritten VALUES (old.at); END`)
	must(t, db.Close())

	clock := second.Add(time.Minute)
	s, err = Open(path, func() time.Time { return clock })
	must(t, err)
	defer func() { s.Close() }()
	texts := func(q string) []string {
		t.Helper()
		rows, err := s.db.Query(q)
		must(t, err)
		defer rows.Close()
		var out []string
		for rows.Next() {
			var v sql.NullString
			must(t, rows.Scan(&v))
			if v.Valid {
				out = append(out, v.String)
			} else {
				out = append(out, "none")
			}
		}
		must(t, rows.Err())
		return out
	}
	var version int
	must(t, s.db.QueryRow(`PRAGMA user_version`).Scan(&version))
	if version != schemaVersion {
		t.Errorf("schema version %d after the migration", version)
	}

	// Each time rewritten is the one it was, as stamp writes it.
	var now, then []string
	for _, when := range at {
		now, then = append(now, stamp(when)), append(then, stampBefore(when))
	}
	for _, c := range []string{"usage.at", "client_usage.at", "states.at", "relay_clients.first_at", "relay_clients.last_at", "knocks.first_at", "knocks.last_at"} {
		table, col, _ := strings.Cut(c, ".")
		if got := texts(`SELECT ` + col + ` FROM ` + table + ` ORDER BY rowid`); strings.Join(got, " ") != strings.Join(now, " ") {
			t.Errorf("%s after the migration: %v", c, got)
		}
	}
	for q, want := range map[string][]string{
		`SELECT began FROM alert_log ORDER BY id`: append(append([]string{}, now...), stamp(at[1])),
		`SELECT ended FROM alert_log ORDER BY id`: append(append([]string{}, now...), "none"),
		`SELECT at FROM actions ORDER BY id`:      append(append([]string{}, now...), stamp(at[1])),
		`SELECT done_at FROM actions ORDER BY id`: append(append([]string{}, now...), "none"),
		`SELECT seen_at FROM aps ORDER BY ap`:     {stamp(at[1]), stamp(at[0])},
		`SELECT running_at FROM aps ORDER BY ap`:  {stamp(at[2]), "none"},
		`SELECT at FROM checks ORDER BY id`:       then,
		`SELECT at FROM applies ORDER BY id`:      then,
		`SELECT name FROM sqlite_master WHERE type = 'trigger' AND tbl_name IN ('checks', 'applies') ORDER BY name`: {"applies_kept", "applies_kept_del", "checks_kept", "checks_kept_del"},
	} {
		if got := texts(q); strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("%s:\n got %v\nwant %v", q, got, want)
		}
	}

	// The tenth had its nine digits, and was left.
	if got := texts(`SELECT at FROM rewritten ORDER BY length(at)`); strings.Join(got, " ") != strings.Join(then[:9], " ") {
		t.Errorf("rewritten: %v", got)
	}

	// And they compare as times: the oldest first, and since the sixth of
	// the ten, the five from it on.
	var uses []string
	must(t, s.Uses(second, func(u Use) error { uses = append(uses, fmt.Sprint(u.Clients)); return nil }))
	if got := strings.Join(uses, " "); got != "0 1 2 3 4 5 6 7 8 9" {
		t.Errorf("uses, oldest first: %s", got)
	}
	totals, err := s.ClientTotals(at[5])
	must(t, err)
	if len(totals) != 1 || totals[0].Down != 5 || totals[0].Host != "name-9" || !totals[0].HostAt.Equal(at[9]) {
		t.Errorf("client totals since the sixth: %+v", totals)
	}
	states := 0
	must(t, s.StatesWith(at[5], "aa:bb:cc:dd:ee:01", func(hierarchy.NodeID, State) error { states++; return nil }))
	tests, err := s.ClientTests(at[5])
	must(t, err)
	alerts, err := s.AlertLog(at[5], []hierarchy.NodeID{"ap-1"}, 100)
	must(t, err)
	if states != 5 || len(tests) != 5 || len(alerts) != 6 {
		t.Errorf("since the sixth: %d states, %d tests, %d alerts with the open one", states, len(tests), len(alerts))
	}
	ks, err := s.Knocks()
	must(t, err)
	var sources []string
	for _, k := range ks {
		sources = append(sources, k.Source)
	}
	if got := strings.Join(sources, " "); got != "9 8 7 6 5 4 3 2 1 0" {
		t.Errorf("knocks, newest first: %s", got)
	}
	l, err := s.Latest("ap-1")
	must(t, err)
	if l.Seen == nil || !l.Seen.At.Equal(at[1]) || l.Seen.RunningAt == nil || !l.Seen.RunningAt.Equal(at[2]) {
		t.Errorf("seen: %+v", l.Seen)
	}

	// The applies, as recorded and one new, since each of the ten.
	if _, err := s.RecordApply("ap-1", Apply{Version: 10, Hash: "aa", OK: true}); err != nil {
		t.Fatal(err)
	}
	for i, when := range at {
		applies, err := s.AppliesSince("ap-1", when, 100)
		must(t, err)
		var got, want []string
		for _, a := range applies {
			got = append(got, fmt.Sprint(a.Version))
		}
		for v := 10; v >= i; v-- {
			want = append(want, fmt.Sprint(v))
		}
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("applies since %s: %v, want %v", stampBefore(when), got, want)
		}
	}
	if applies, err := s.AppliesSince("ap-1", clock.Add(time.Second), 100); err != nil || len(applies) != 0 {
		t.Errorf("applies since after the last: %+v, %v", applies, err)
	}

	// Opened again, it is as it was.
	must(t, s.Close())
	s, err = Open(path, func() time.Time { return clock })
	must(t, err)
	if got := texts(`SELECT at FROM usage ORDER BY rowid`); strings.Join(got, " ") != strings.Join(now, " ") {
		t.Errorf("usage.at, opened again: %v", got)
	}
}
