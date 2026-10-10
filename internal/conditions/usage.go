package conditions

import (
	"database/sql"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Usage (0108): how many Wi-Fi clients an AP had at each state report, and
// how much they moved since the report before, which the manager works out
// as the report comes. A row a report, so a day's chart reads a few
// hundred rows, not a day of reports.
const usageTable = `
CREATE TABLE usage (
	ap      TEXT NOT NULL,
	at      TEXT NOT NULL,
	clients INTEGER NOT NULL,
	down    INTEGER NOT NULL,
	up      INTEGER NOT NULL
);
CREATE INDEX usage_at ON usage (at);`

// clientUsageTable keeps what each client moved at each report (0110), a
// row for each that moved anything.
const clientUsageTable = `
CREATE TABLE client_usage (
	ap   TEXT NOT NULL,
	at   TEXT NOT NULL,
	mac  TEXT NOT NULL,
	host TEXT NOT NULL,
	down INTEGER NOT NULL,
	up   INTEGER NOT NULL
);
CREATE INDEX client_usage_at ON client_usage (at);`

// clientUsageByClient lets the host name a client last gave on an AP be
// found at once, not by reading back through the whole table.
const clientUsageByClient = `CREATE INDEX client_usage_client ON client_usage (ap, mac, at);`

// ClientUse is what one client moved since its AP's last report (0110),
// with the host name it gave in DHCP, if any.
type ClientUse struct {
	MAC  string
	Host string
	Down int64
	Up   int64
}

// ClientTotal is what one client moved on one AP since a time, with the
// host name it last gave there and when.
type ClientTotal struct {
	AP     hierarchy.NodeID
	MAC    string
	Host   string
	HostAt time.Time
	Down   int64
	Up     int64
}

// Use is one AP's clients at a report, and what they moved since its last:
// Down is what the AP sent them, Up what it received from them, in bytes.
type Use struct {
	AP      hierarchy.NodeID
	At      time.Time
	Clients int
	Down    int64
	Up      int64
}

// RecordUse records an AP's clients at a report and what each moved, in
// one transaction: the AP's row, its traffic the sum of theirs, and a row
// for each client (0110).
func (s *Store) RecordUse(ap hierarchy.NodeID, clients int, moved []ClientUse) error {
	at := stamp(s.now())
	var down, up int64
	for _, c := range moved {
		down += c.Down
		up += c.Up
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO usage (ap, at, clients, down, up) VALUES (?, ?, ?, ?, ?)`, string(ap), at, clients, down, up); err != nil {
		return err
	}
	for _, c := range moved {
		if _, err := tx.Exec(`INSERT INTO client_usage (ap, at, mac, host, down, up) VALUES (?, ?, ?, ?, ?, ?)`, string(ap), at, c.MAC, c.Host, c.Down, c.Up); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ClientTotals is what each client moved on each AP since since (0110),
// with the last host name it gave there.
func (s *Store) ClientTotals(since time.Time) ([]ClientTotal, error) {
	rows, err := s.db.Query(`SELECT c.ap, c.mac, SUM(c.down), SUM(c.up),
		(SELECT h.host FROM client_usage h WHERE h.ap = c.ap AND h.mac = c.mac AND h.host != '' ORDER BY h.at DESC LIMIT 1),
		(SELECT h.at FROM client_usage h WHERE h.ap = c.ap AND h.mac = c.mac AND h.host != '' ORDER BY h.at DESC LIMIT 1)
		FROM client_usage c WHERE c.at >= ? GROUP BY c.ap, c.mac`, stamp(since))
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
		if at.Valid {
			if t.HostAt, err = time.Parse(time.RFC3339Nano, at.String); err != nil {
				return nil, err
			}
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UsageFrom is when the oldest usage row kept was recorded, and false when
// there is none: before it, nothing says whether an AP reported.
func (s *Store) UsageFrom() (time.Time, bool, error) {
	var at sql.NullString
	if err := s.db.QueryRow(`SELECT MIN(at) FROM usage`).Scan(&at); err != nil || !at.Valid {
		return time.Time{}, false, err
	}
	t, err := time.Parse(time.RFC3339Nano, at.String)
	return t, err == nil, err
}

// Uses calls fn with every AP's use since since, oldest first.
func (s *Store) Uses(since time.Time, fn func(Use) error) error {
	rows, err := s.db.Query(`SELECT ap, at, clients, down, up FROM usage WHERE at >= ? ORDER BY at`, stamp(since))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var u Use
		var ap, at string
		if err := rows.Scan(&ap, &at, &u.Clients, &u.Down, &u.Up); err != nil {
			return err
		}
		if u.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return err
		}
		u.AP = hierarchy.NodeID(ap)
		if err := fn(u); err != nil {
			return err
		}
	}
	return rows.Err()
}
