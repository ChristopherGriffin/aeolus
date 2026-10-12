package conditions

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Trends (0140): what the reports of a time came to, kept long after the
// reports themselves and the rows worked out from each (0108, 0110, 0119)
// are let go. An AP's clients and traffic, and its health, an hour at a
// time; what each client moved on it, a day at a time. A row is added to as
// each report comes, so nothing is read back to make one, and what is said
// of last month costs a few hundred rows whatever the reports were.
//
// hour and day are when each began, in seconds since 1970, in UTC. Of an
// hour's health the worst is kept: the busiest processor, the least memory
// and storage free, the hottest; as a chart of many reports draws the worst
// of a run of them.
//
// The rows already kept are counted in as the tables are made, so the
// trends begin as far back as the store does. A row whose time cannot be
// read is left out, so that none can keep the store from opening.
const trendTables = `
CREATE TABLE usage_hours (
	ap      TEXT NOT NULL,
	hour    INTEGER NOT NULL,
	reports INTEGER NOT NULL,
	clients INTEGER NOT NULL,
	down    INTEGER NOT NULL,
	up      INTEGER NOT NULL,
	PRIMARY KEY (ap, hour)
) WITHOUT ROWID;
CREATE INDEX usage_hours_hour ON usage_hours (hour);
CREATE TABLE client_usage_days (
	ap      TEXT NOT NULL,
	mac     TEXT NOT NULL,
	day     INTEGER NOT NULL,
	host    TEXT NOT NULL,
	host_at TEXT NOT NULL,
	down    INTEGER NOT NULL,
	up      INTEGER NOT NULL,
	PRIMARY KEY (ap, mac, day)
) WITHOUT ROWID;
CREATE INDEX client_usage_days_day ON client_usage_days (day);
CREATE TABLE health_hours (
	ap            TEXT NOT NULL,
	hour          INTEGER NOT NULL,
	reports       INTEGER NOT NULL,
	clients       INTEGER NOT NULL,
	cpu           INTEGER,
	load          REAL,
	mem_total     INTEGER,
	mem_available INTEGER,
	storage_total INTEGER,
	storage_free  INTEGER,
	temp          REAL,
	procs         INTEGER,
	PRIMARY KEY (ap, hour)
) WITHOUT ROWID;
CREATE INDEX health_hours_hour ON health_hours (hour);
INSERT INTO usage_hours (ap, hour, reports, clients, down, up)
	SELECT ap, CAST(strftime('%s', at) AS INTEGER) / 3600 * 3600, count(*), max(clients), sum(down), sum(up)
	FROM usage WHERE strftime('%s', at) IS NOT NULL GROUP BY 1, 2;
INSERT INTO client_usage_days (ap, mac, day, host, host_at, down, up)
	SELECT t.ap, t.mac, t.day, coalesce(n.host, ''), coalesce(n.at, ''), t.down, t.up
	FROM (SELECT ap, mac, CAST(strftime('%s', at) AS INTEGER) / 86400 * 86400 AS day, sum(down) AS down, sum(up) AS up
		FROM client_usage WHERE strftime('%s', at) IS NOT NULL GROUP BY 1, 2, 3) t
	LEFT JOIN (SELECT ap, mac, CAST(strftime('%s', at) AS INTEGER) / 86400 * 86400 AS day, host, max(at) AS at
		FROM client_usage WHERE host != '' AND strftime('%s', at) IS NOT NULL GROUP BY 1, 2, 3) n USING (ap, mac, day);
INSERT INTO health_hours (ap, hour, reports, clients, cpu, load, mem_total, mem_available, storage_total, storage_free, temp, procs)
	SELECT ap, at / 3600000 * 3600, count(*), max(clients), max(cpu), max(load), max(mem_total), min(mem_available),
		max(storage_total), min(storage_free), max(temp), max(procs)
	FROM health GROUP BY 1, 2;`

// hourOf and dayOf are the hour and the day a time is in, as the trends
// name them.
func hourOf(t time.Time) int64 { return t.Unix() / 3600 * 3600 }
func dayOf(t time.Time) int64  { return t.Unix() / 86400 * 86400 }

// The worst of what a row holds and what a report says, where either may
// not be said: SQLite's max and min of two are null if one is.
const (
	worstMore = `max(coalesce(%[1]s, excluded.%[1]s), coalesce(excluded.%[1]s, %[1]s))`
	worstLess = `min(coalesce(%[1]s, excluded.%[1]s), coalesce(excluded.%[1]s, %[1]s))`
)

// addUse counts a report into its AP's hour, and what each client moved
// into that client's day: in the transaction that keeps the report's own
// rows (RecordUse), so the two never differ.
func addUse(tx *sql.Tx, ap hierarchy.NodeID, now time.Time, clients int, down, up int64, moved []ClientUse) error {
	if _, err := tx.Exec(`INSERT INTO usage_hours (ap, hour, reports, clients, down, up) VALUES (?, ?, 1, ?, ?, ?)
		ON CONFLICT (ap, hour) DO UPDATE SET reports = reports + 1, clients = max(clients, excluded.clients),
			down = down + excluded.down, up = up + excluded.up`,
		string(ap), hourOf(now), clients, down, up); err != nil {
		return err
	}
	for _, c := range moved {
		// The name it gave last stays where this report has none.
		at := ""
		if c.Host != "" {
			at = stamp(now)
		}
		if _, err := tx.Exec(`INSERT INTO client_usage_days (ap, mac, day, host, host_at, down, up) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (ap, mac, day) DO UPDATE SET down = down + excluded.down, up = up + excluded.up,
				host = CASE WHEN excluded.host != '' THEN excluded.host ELSE host END,
				host_at = CASE WHEN excluded.host != '' THEN excluded.host_at ELSE host_at END`,
			string(ap), c.MAC, dayOf(now), c.Host, at, c.Down, c.Up); err != nil {
			return err
		}
	}
	return nil
}

// addHealth counts a report's health into its AP's hour, in the
// transaction that keeps the report's own row (RecordHealth).
func addHealth(tx *sql.Tx, ap hierarchy.NodeID, now time.Time, p HealthPoint) error {
	more := func(col string) string { return col + ` = ` + fmt.Sprintf(worstMore, col) }
	less := func(col string) string { return col + ` = ` + fmt.Sprintf(worstLess, col) }
	_, err := tx.Exec(`INSERT INTO health_hours (ap, hour, reports, clients, cpu, load, mem_total, mem_available, storage_total, storage_free, temp, procs)
		VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (ap, hour) DO UPDATE SET reports = reports + 1, clients = max(clients, excluded.clients), `+
		more("cpu")+`, `+more("load")+`, `+more("mem_total")+`, `+less("mem_available")+`, `+
		more("storage_total")+`, `+less("storage_free")+`, `+more("temp")+`, `+more("procs"),
		string(ap), hourOf(now), p.Clients, p.CPU, p.Load, p.MemTotal, p.MemAvailable, p.StorageTotal, p.StorageFree, p.Temp, p.Procs)
	return err
}

// UsesByHour calls fn with every AP's use since since, an hour at a time,
// oldest first: At is when the hour began, Clients the most it had at a
// report in it, Down and Up what they moved in it. An hour partly before
// since is given whole.
func (s *Store) UsesByHour(since time.Time, fn func(Use) error) error {
	rows, err := s.db.Query(`SELECT ap, hour, clients, down, up FROM usage_hours WHERE hour >= ? ORDER BY hour`, hourOf(since))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var u Use
		var ap string
		var hour int64
		if err := rows.Scan(&ap, &hour, &u.Clients, &u.Down, &u.Up); err != nil {
			return err
		}
		u.AP, u.At = hierarchy.NodeID(ap), time.Unix(hour, 0).UTC()
		if err := fn(u); err != nil {
			return err
		}
	}
	return rows.Err()
}

// UsageHoursFrom is when the oldest hour of usage kept began, and false
// when there is none: before it, nothing says whether an AP reported.
func (s *Store) UsageHoursFrom() (time.Time, bool, error) {
	var hour sql.NullInt64
	if err := s.db.QueryRow(`SELECT MIN(hour) FROM usage_hours`).Scan(&hour); err != nil || !hour.Valid {
		return time.Time{}, false, err
	}
	return time.Unix(hour.Int64, 0).UTC(), true, nil
}

// ClientTotalsOver is what each client moved on each AP since since, over a
// span that may be longer than the reports' own rows are kept. It is read
// from those rows where they reach back to since, or as far back as the
// days kept do: then nothing older is known by the day either. Else it is
// counted by whole days, from the first that begins at or after since, so
// it is never more than was moved since since, and lacks a day of it at
// most.
func (s *Store) ClientTotalsOver(since time.Time) ([]ClientTotal, error) {
	var rows sql.NullString
	var days sql.NullInt64
	if err := s.db.QueryRow(`SELECT (SELECT MIN(at) FROM client_usage), (SELECT MIN(day) FROM client_usage_days)`).Scan(&rows, &days); err != nil {
		return nil, err
	}
	if rows.Valid {
		from, err := time.Parse(time.RFC3339Nano, rows.String)
		if err != nil {
			return nil, err
		}
		if !from.After(since) || !days.Valid || dayOf(from) <= days.Int64 {
			return s.ClientTotals(since)
		}
	}
	day := time.Unix(dayOf(since), 0)
	if day.Before(since) {
		day = day.Add(24 * time.Hour)
	}
	return s.ClientTotalsByDay(day)
}

// ClientTotalsByDay is what each client moved on each AP since the day
// since is in, counted by whole days, with the last host name it gave there
// in those days and when.
func (s *Store) ClientTotalsByDay(since time.Time) ([]ClientTotal, error) {
	// A bare column beside a lone max is of the row the max is of.
	rows, err := s.db.Query(`SELECT t.ap, t.mac, t.down, t.up, n.host, n.host_at
		FROM (SELECT ap, mac, sum(down) AS down, sum(up) AS up FROM client_usage_days WHERE day >= ? GROUP BY ap, mac) t
		LEFT JOIN (SELECT ap, mac, host, max(host_at) AS host_at FROM client_usage_days WHERE day >= ? AND host != '' GROUP BY ap, mac) n
			USING (ap, mac)`, dayOf(since), dayOf(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClientTotal
	for rows.Next() {
		var t ClientTotal
		var ap string
		var host, at sql.NullString
		if err := rows.Scan(&ap, &t.MAC, &t.Down, &t.Up, &host, &at); err != nil {
			return nil, err
		}
		t.AP, t.Host = hierarchy.NodeID(ap), host.String
		if at.Valid && at.String != "" {
			if t.HostAt, err = time.Parse(time.RFC3339Nano, at.String); err != nil {
				return nil, err
			}
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// HealthByHour is an AP's health since since, an hour at a time, oldest
// first: At is when the hour began, and each thing the worst a report in it
// said. Uptime is not kept by the hour. An hour partly before since is
// given whole.
func (s *Store) HealthByHour(ap hierarchy.NodeID, since time.Time) ([]HealthPoint, error) {
	rows, err := s.db.Query(`SELECT hour, clients, cpu, load, mem_total, mem_available, storage_total, storage_free, temp, procs
		FROM health_hours WHERE ap = ? AND hour >= ? ORDER BY hour`, string(ap), hourOf(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HealthPoint{}
	for rows.Next() {
		var p HealthPoint
		var hour int64
		var cpu, procs, mt, ma, st, sf sql.NullInt64
		var load, temp sql.NullFloat64
		if err := rows.Scan(&hour, &p.Clients, &cpu, &load, &mt, &ma, &st, &sf, &temp, &procs); err != nil {
			return nil, err
		}
		p.At = time.Unix(hour, 0).UTC()
		if cpu.Valid {
			v := int(cpu.Int64)
			p.CPU = &v
		}
		if procs.Valid {
			v := int(procs.Int64)
			p.Procs = &v
		}
		if load.Valid {
			p.Load = &load.Float64
		}
		if temp.Valid {
			p.Temp = &temp.Float64
		}
		for _, x := range []struct {
			from sql.NullInt64
			to   **int64
		}{{mt, &p.MemTotal}, {ma, &p.MemAvailable}, {st, &p.StorageTotal}, {sf, &p.StorageFree}} {
			if x.from.Valid {
				v := x.from.Int64
				*x.to = &v
			}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// TrimTrends deletes the hours and days that began more than keep ago, and
// returns how many rows.
func (s *Store) TrimTrends(keep time.Duration) (int64, error) {
	cut := s.now().Add(-keep).Unix()
	var n int64
	for _, q := range []string{
		`DELETE FROM usage_hours WHERE hour < ?`,
		`DELETE FROM client_usage_days WHERE day < ?`,
		`DELETE FROM health_hours WHERE hour < ?`,
		`DELETE FROM step_hours WHERE hour < ?`,
	} {
		res, err := s.db.Exec(q, cut)
		if err != nil {
			return n, err
		}
		m, _ := res.RowsAffected()
		n += m
	}
	return n, nil
}
