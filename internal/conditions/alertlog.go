package conditions

import (
	"database/sql"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// The alert log (0109): each alert that lasted, when it began and when it
// ended, so what happened while nobody looked can be read after. The
// alerts themselves are worked out on each look (0099); this keeps their
// history.
const alertLogTable = `
CREATE TABLE alert_log (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	ap       TEXT NOT NULL,
	key      TEXT NOT NULL,
	kind     TEXT NOT NULL,
	severity TEXT NOT NULL,
	message  TEXT NOT NULL,
	began    TEXT NOT NULL,
	ended    TEXT
);
CREATE INDEX alert_log_began ON alert_log (began);
CREATE INDEX alert_log_ended ON alert_log (ended);`

// LoggedAlert is one alert in the log: Ended is nil while it lasts.
type LoggedAlert struct {
	ID       int64            `json:"id"`
	AP       hierarchy.NodeID `json:"ap"`
	Key      string           `json:"key"`
	Kind     string           `json:"kind"`
	Severity string           `json:"severity"`
	Message  string           `json:"message"`
	Began    time.Time        `json:"began"`
	Ended    *time.Time       `json:"ended,omitempty"`
}

// BeginAlert logs an alert that began, and returns its ID.
func (s *Store) BeginAlert(a LoggedAlert) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO alert_log (ap, key, kind, severity, message, began) VALUES (?, ?, ?, ?, ?, ?)`,
		string(a.AP), a.Key, a.Kind, a.Severity, a.Message, stamp(a.Began))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// EndAlert logs that an alert ended at at.
func (s *Store) EndAlert(id int64, at time.Time) error {
	_, err := s.db.Exec(`UPDATE alert_log SET ended = ? WHERE id = ? AND ended IS NULL`, stamp(at), id)
	return err
}

// OpenAlerts is every alert in the log that has not ended.
func (s *Store) OpenAlerts() ([]LoggedAlert, error) {
	return s.loggedAlerts(`SELECT id, ap, key, kind, severity, message, began, ended FROM alert_log WHERE ended IS NULL ORDER BY id`)
}

// AlertLog is every alert that lasted at some time since since, newest
// first, at most limit.
func (s *Store) AlertLog(since time.Time, limit int) ([]LoggedAlert, error) {
	return s.loggedAlerts(`SELECT id, ap, key, kind, severity, message, began, ended FROM alert_log
		WHERE ended IS NULL OR ended >= ? ORDER BY began DESC, id DESC LIMIT ?`, stamp(since), limit)
}

func (s *Store) loggedAlerts(q string, args ...any) ([]LoggedAlert, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LoggedAlert{}
	for rows.Next() {
		var a LoggedAlert
		var ap, began string
		var ended sql.NullString
		if err := rows.Scan(&a.ID, &ap, &a.Key, &a.Kind, &a.Severity, &a.Message, &began, &ended); err != nil {
			return nil, err
		}
		a.AP = hierarchy.NodeID(ap)
		if a.Began, err = time.Parse(time.RFC3339Nano, began); err != nil {
			return nil, err
		}
		if ended.Valid {
			t, err := time.Parse(time.RFC3339Nano, ended.String)
			if err != nil {
				return nil, err
			}
			a.Ended = &t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
