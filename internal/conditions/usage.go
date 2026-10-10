package conditions

import (
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

// Use is one AP's clients at a report, and what they moved since its last:
// Down is what the AP sent them, Up what it received from them, in bytes.
type Use struct {
	AP      hierarchy.NodeID
	At      time.Time
	Clients int
	Down    int64
	Up      int64
}

// RecordUse records an AP's clients and traffic at a report.
func (s *Store) RecordUse(ap hierarchy.NodeID, clients int, down, up int64) error {
	_, err := s.db.Exec(`INSERT INTO usage (ap, at, clients, down, up) VALUES (?, ?, ?, ?, ?)`,
		string(ap), stamp(s.now()), clients, down, up)
	return err
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
