package conditions

import (
	"database/sql"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// An AP's own health, and what happened to it (0119): how busy its
// processor was, its memory, storage and temperature at each state report,
// a row a report, so a day's chart reads a few hundred rows; and each thing
// that changed from one report to the next, such as a restart, a silence or
// a radio moving channel. Times are whole milliseconds since 1970.
const healthTables = `
CREATE TABLE health (
	ap            TEXT NOT NULL,
	at            INTEGER NOT NULL,
	uptime        INTEGER NOT NULL,
	clients       INTEGER NOT NULL,
	cpu           INTEGER,
	load          REAL,
	mem_total     INTEGER,
	mem_available INTEGER,
	storage_total INTEGER,
	storage_free  INTEGER,
	temp          REAL,
	procs         INTEGER
);
CREATE INDEX health_ap ON health (ap, at);
CREATE TABLE ap_events (
	id   INTEGER PRIMARY KEY AUTOINCREMENT,
	ap   TEXT NOT NULL,
	at   INTEGER NOT NULL,
	kind TEXT NOT NULL,
	text TEXT NOT NULL
);
CREATE INDEX ap_events_ap ON ap_events (ap, at);`

// HealthPoint is an AP's health at one state report. What an AP did not
// say is nil.
type HealthPoint struct {
	At           time.Time `json:"at"`
	Uptime       int64     `json:"uptime"`
	Clients      int       `json:"clients"`
	CPU          *int      `json:"cpu"`
	Load         *float64  `json:"load"`
	MemTotal     *int64    `json:"mem_total"`
	MemAvailable *int64    `json:"mem_available"`
	StorageTotal *int64    `json:"storage_total"`
	StorageFree  *int64    `json:"storage_free"`
	Temp         *float64  `json:"temp"`
	Procs        *int      `json:"procs"`
}

// APEvent is one thing that happened to an AP: when, its kind, and what,
// in words.
type APEvent struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"`
	Text string    `json:"text"`
}

// RecordHealth keeps an AP's health at a state report, and what changed
// since its last, in one transaction.
func (s *Store) RecordHealth(ap hierarchy.NodeID, p HealthPoint, events []APEvent) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO health (ap, at, uptime, clients, cpu, load, mem_total, mem_available, storage_total, storage_free, temp, procs)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(ap), s.now().UnixMilli(), p.Uptime, p.Clients, p.CPU, p.Load, p.MemTotal, p.MemAvailable, p.StorageTotal, p.StorageFree, p.Temp, p.Procs); err != nil {
		return err
	}
	for _, e := range events {
		if _, err := tx.Exec(`INSERT INTO ap_events (ap, at, kind, text) VALUES (?, ?, ?, ?)`, string(ap), e.At.UnixMilli(), e.Kind, e.Text); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Health is an AP's health at each report since since, oldest first.
func (s *Store) Health(ap hierarchy.NodeID, since time.Time) ([]HealthPoint, error) {
	rows, err := s.db.Query(`SELECT at, uptime, clients, cpu, load, mem_total, mem_available, storage_total, storage_free, temp, procs
		FROM health WHERE ap = ? AND at >= ? ORDER BY at`, string(ap), since.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HealthPoint{}
	for rows.Next() {
		var p HealthPoint
		var at int64
		var cpu, procs sql.NullInt64
		var load, temp sql.NullFloat64
		var mt, ma, st, sf sql.NullInt64
		if err := rows.Scan(&at, &p.Uptime, &p.Clients, &cpu, &load, &mt, &ma, &st, &sf, &temp, &procs); err != nil {
			return nil, err
		}
		p.At = time.UnixMilli(at).UTC()
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

// Events is what happened to an AP since since, newest first, at most
// limit.
func (s *Store) Events(ap hierarchy.NodeID, since time.Time, limit int) ([]APEvent, error) {
	rows, err := s.db.Query(`SELECT at, kind, text FROM ap_events WHERE ap = ? AND at >= ? ORDER BY at DESC, id DESC LIMIT ?`, string(ap), since.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APEvent{}
	for rows.Next() {
		var e APEvent
		var at int64
		if err := rows.Scan(&at, &e.Kind, &e.Text); err != nil {
			return nil, err
		}
		e.At = time.UnixMilli(at).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// AppliesSince is an AP's applies since since, newest first, at most
// limit.
func (s *Store) AppliesSince(ap hierarchy.NodeID, since time.Time, limit int) ([]Apply, error) {
	rows, err := s.db.Query(`SELECT at, version, hash, ok, error, checked FROM applies WHERE ap = ? AND at >= ? ORDER BY id DESC LIMIT ?`, string(ap), stamp(since), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Apply{}
	for rows.Next() {
		var a Apply
		var at string
		if err := rows.Scan(&at, &a.Version, &a.Hash, &a.OK, &a.Error, &a.Checked); err != nil {
			return nil, err
		}
		if a.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
