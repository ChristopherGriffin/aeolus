package conditions

import (
	"database/sql"
	"errors"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Action is one thing a person asked an AP to do once (0104): locate,
// restart-wifi or reboot. It is pending until the AP says it did it, or
// could not; one the AP has not taken up within ActionWait expires.
type Action struct {
	ID     int64      `json:"id"`
	AP     string     `json:"ap"`
	Kind   string     `json:"kind"`
	Actor  string     `json:"actor"`
	At     time.Time  `json:"at"`
	State  string     `json:"state"` // pending, done, failed or expired
	DoneAt *time.Time `json:"done_at,omitempty"`
	Result string     `json:"result,omitempty"`
}

// ActionWait is how long an action waits for its AP: an AP that comes back
// later should not reboot because someone asked an hour ago.
const ActionWait = 10 * time.Minute

// ErrNoAction is an action the AP does not have pending.
var ErrNoAction = errors.New("no such pending action")

const actionsTable = `
CREATE TABLE actions (
	id      INTEGER PRIMARY KEY AUTOINCREMENT,
	ap      TEXT NOT NULL,
	kind    TEXT NOT NULL,
	actor   TEXT NOT NULL,
	at      TEXT NOT NULL,
	state   TEXT NOT NULL,
	done_at TEXT,
	result  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX actions_ap ON actions (ap, id);`

// AddAction records an action for an AP, pending. One of the same kind
// already pending is returned instead, so a second click does not reboot an
// AP twice.
func (s *Store) AddAction(ap hierarchy.NodeID, kind, actor string) (Action, error) {
	s.expire(ap)
	var id int64
	err := s.db.QueryRow(`SELECT id FROM actions WHERE ap = ? AND kind = ? AND state = 'pending'`, string(ap), kind).Scan(&id)
	if err == nil {
		return s.action(id)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Action{}, err
	}
	res, err := s.db.Exec(`INSERT INTO actions (ap, kind, actor, at, state) VALUES (?, ?, ?, ?, 'pending')`, string(ap), kind, actor, stamp(s.now()))
	if err != nil {
		return Action{}, err
	}
	if id, err = res.LastInsertId(); err != nil {
		return Action{}, err
	}
	return s.action(id)
}

// expire marks an AP's actions that waited too long.
func (s *Store) expire(ap hierarchy.NodeID) {
	s.db.Exec(`UPDATE actions SET state = 'expired', done_at = ? WHERE ap = ? AND state = 'pending' AND at < ?`,
		stamp(s.now()), string(ap), stamp(s.now().Add(-ActionWait)))
}

// PendingActions is what an AP has to do, oldest first.
func (s *Store) PendingActions(ap hierarchy.NodeID) ([]Action, error) {
	s.expire(ap)
	return s.actions(`SELECT id, ap, kind, actor, at, state, done_at, result FROM actions WHERE ap = ? AND state = 'pending' ORDER BY id`, string(ap))
}

// FinishAction records what came of an AP's pending action.
func (s *Store) FinishAction(ap hierarchy.NodeID, id int64, ok bool, result string) (Action, error) {
	state := "done"
	if !ok {
		state = "failed"
	}
	res, err := s.db.Exec(`UPDATE actions SET state = ?, done_at = ?, result = ? WHERE id = ? AND ap = ? AND state = 'pending'`,
		state, stamp(s.now()), result, id, string(ap))
	if err != nil {
		return Action{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Action{}, ErrNoAction
	}
	return s.action(id)
}

// Actions is an AP's latest actions, newest first.
func (s *Store) Actions(ap hierarchy.NodeID, limit int) ([]Action, error) {
	s.expire(ap)
	return s.actions(`SELECT id, ap, kind, actor, at, state, done_at, result FROM actions WHERE ap = ? ORDER BY id DESC LIMIT ?`, string(ap), limit)
}

func (s *Store) action(id int64) (Action, error) {
	out, err := s.actions(`SELECT id, ap, kind, actor, at, state, done_at, result FROM actions WHERE id = ?`, id)
	if err != nil || len(out) == 0 {
		return Action{}, errors.Join(err, ErrNoAction)
	}
	return out[0], nil
}

func (s *Store) actions(q string, args ...any) ([]Action, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Action{}
	for rows.Next() {
		var a Action
		var at string
		var done sql.NullString
		if err := rows.Scan(&a.ID, &a.AP, &a.Kind, &a.Actor, &at, &a.State, &done, &a.Result); err != nil {
			return nil, err
		}
		if a.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, err
		}
		if done.Valid {
			t, err := time.Parse(time.RFC3339Nano, done.String)
			if err != nil {
				return nil, err
			}
			a.DoneAt = &t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
