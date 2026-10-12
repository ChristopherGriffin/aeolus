package conditions

import (
	"database/sql"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// How long the network took to answer (0141): what an AP timed of a
// network's own services, an hour at a time. A client test (0121, 0128)
// times how long an address took, the gateway, a name looked up, a sign-in;
// a state report says how long the network's RADIUS server took (0114).
// Each is counted into its AP's hour of that network and that step as it
// comes: how many times it was timed, the sum of those times and the
// slowest, and how many times no answer came. Kept as long as the other
// trends are (0140).
//
// hour is when it began, in seconds since 1970, in UTC; ms and most are in
// milliseconds.
//
// marks are the things done once ever, by name, and when: such as counting
// in the tests kept from before this was (AddStepsOnce).
const stepTable = `
CREATE TABLE step_hours (
	ap      TEXT NOT NULL,
	network TEXT NOT NULL,
	step    TEXT NOT NULL,
	hour    INTEGER NOT NULL,
	n       INTEGER NOT NULL,
	ms      REAL NOT NULL,
	most    REAL NOT NULL,
	failed  INTEGER NOT NULL,
	PRIMARY KEY (ap, network, step, hour)
) WITHOUT ROWID;
CREATE INDEX step_hours_hour ON step_hours (hour);
CREATE TABLE marks (
	name TEXT NOT NULL PRIMARY KEY,
	at   INTEGER NOT NULL
) WITHOUT ROWID;`

// StepTime is one thing timed on a network: which step, and how long its
// answer took, in ms; nil where none came.
type StepTime struct {
	Network string
	Step    string
	MS      *float64
}

// StepHour is what an AP timed of one step of a network in an hour: At is
// when the hour began, N how many times it was timed, MS the sum of those
// times and Most the slowest, in ms, and Failed how many times no answer
// came.
type StepHour struct {
	AP      hierarchy.NodeID
	Network string
	Step    string
	At      time.Time
	N       int
	MS      float64
	Most    float64
	Failed  int
}

// StepsAt is what one AP timed at one time.
type StepsAt struct {
	AP    hierarchy.NodeID
	At    time.Time
	Steps []StepTime
}

// AddSteps counts what an AP timed at a time into the hours of its
// networks' steps.
func (s *Store) AddSteps(ap hierarchy.NodeID, at time.Time, steps []StepTime) error {
	if len(steps) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := addSteps(tx, ap, at, steps); err != nil {
		return err
	}
	return tx.Commit()
}

// Marked says whether the thing named was done: see AddStepsOnce.
func (s *Store) Marked(name string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM marks WHERE name = ?)`, name).Scan(&n)
	return n != 0, err
}

// AddStepsOnce counts all in, once ever, by name: all of it and the mark
// that it was done in one transaction, so that a manager stopped part way
// has done none of it and does it whole the next time; and one that has
// done it, whatever else was counted since, never does it again. It says
// whether this was the time.
func (s *Store) AddStepsOnce(name string, all []StepsAt) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT OR IGNORE INTO marks (name, at) VALUES (?, ?)`, name, s.now().Unix())
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	for _, a := range all {
		if err := addSteps(tx, a.AP, a.At, a.Steps); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

func addSteps(tx *sql.Tx, ap hierarchy.NodeID, at time.Time, steps []StepTime) error {
	for _, st := range steps {
		n, ms, failed := 1, 0.0, 0
		if st.MS == nil {
			n, failed = 0, 1
		} else {
			ms = *st.MS
		}
		if _, err := tx.Exec(`INSERT INTO step_hours (ap, network, step, hour, n, ms, most, failed) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (ap, network, step, hour) DO UPDATE SET n = n + excluded.n, ms = ms + excluded.ms,
				most = max(most, excluded.most), failed = failed + excluded.failed`,
			string(ap), st.Network, st.Step, hourOf(at), n, ms, ms, failed); err != nil {
			return err
		}
	}
	return nil
}

// StepsSince calls fn with what each AP timed of each network's steps since
// since, an hour at a time, oldest first. An hour partly before since is
// given whole.
func (s *Store) StepsSince(since time.Time, fn func(StepHour) error) error {
	rows, err := s.db.Query(`SELECT ap, network, step, hour, n, ms, most, failed FROM step_hours WHERE hour >= ? ORDER BY hour`, hourOf(since))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var h StepHour
		var ap string
		var hour int64
		if err := rows.Scan(&ap, &h.Network, &h.Step, &hour, &h.N, &h.MS, &h.Most, &h.Failed); err != nil {
			return err
		}
		h.AP, h.At = hierarchy.NodeID(ap), time.Unix(hour, 0).UTC()
		if err := fn(h); err != nil {
			return err
		}
	}
	return rows.Err()
}
