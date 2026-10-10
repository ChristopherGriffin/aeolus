// Package conditions records what APs say and do (0009, 0039): when each was
// last seen, every render check and apply, and their periodic state reports.
// It also keeps what the manager hears itself (0068): clients in relays'
// copies of DHCP requests, and knocks on the option 224 listener.
//
// It is a second SQLite database beside the change log, and not the system of
// record. Checks and applies are kept for good and cannot be changed, like
// the change log: they are what each AP was told and what it did. A check
// keeps the UCI the AP sent, with its secrets blanked (0041). State reports
// are trimmed after a retention period.
package conditions

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	_ "github.com/ncruces/go-sqlite3/driver"
)

const schemaVersion = 8

const schema = `
CREATE TABLE aps (
	ap         TEXT PRIMARY KEY,
	seen_at    TEXT NOT NULL,
	source     TEXT NOT NULL,
	running    INTEGER,
	running_at TEXT
);
CREATE TABLE checks (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	ap       TEXT NOT NULL,
	at       TEXT NOT NULL,
	version  INTEGER NOT NULL,
	hash     TEXT NOT NULL,
	result   TEXT NOT NULL,
	problems TEXT NOT NULL
);
CREATE INDEX checks_ap ON checks (ap, id);
CREATE TABLE applies (
	id      INTEGER PRIMARY KEY AUTOINCREMENT,
	ap      TEXT NOT NULL,
	at      TEXT NOT NULL,
	version INTEGER NOT NULL,
	hash    TEXT NOT NULL,
	ok      INTEGER NOT NULL,
	error   TEXT NOT NULL,
	checked INTEGER NOT NULL
);
CREATE INDEX applies_ap ON applies (ap, id);
CREATE TABLE states (
	id      INTEGER PRIMARY KEY AUTOINCREMENT,
	ap      TEXT NOT NULL,
	at      TEXT NOT NULL,
	version INTEGER NOT NULL,
	report  TEXT NOT NULL
);
CREATE INDEX states_ap ON states (ap, id);
CREATE INDEX states_at ON states (at);
CREATE TRIGGER checks_kept BEFORE UPDATE ON checks
BEGIN SELECT RAISE(ABORT, 'render checks are kept as recorded'); END;
CREATE TRIGGER checks_kept_del BEFORE DELETE ON checks
BEGIN SELECT RAISE(ABORT, 'render checks are kept as recorded'); END;
CREATE TRIGGER applies_kept BEFORE UPDATE ON applies
BEGIN SELECT RAISE(ABORT, 'applies are kept as recorded'); END;
CREATE TRIGGER applies_kept_del BEFORE DELETE ON applies
BEGIN SELECT RAISE(ABORT, 'applies are kept as recorded'); END;
`

// migrations bring an older store up to schemaVersion, one step each.
var migrations = map[int]string{
	// 0041: checks keep the UCI they checked, secrets blanked.
	1: `ALTER TABLE checks ADD COLUMN uci TEXT NOT NULL DEFAULT ''`,
	// 0068: what the manager's DHCP listeners hear.
	2: `
CREATE TABLE relay_clients (
	subnet       TEXT NOT NULL,
	mac          TEXT NOT NULL,
	relay        TEXT NOT NULL,
	first_at     TEXT NOT NULL,
	last_at      TEXT NOT NULL,
	requests     INTEGER NOT NULL,
	type         TEXT NOT NULL,
	host         TEXT NOT NULL,
	vendor_class TEXT NOT NULL,
	params       TEXT NOT NULL,
	address      TEXT NOT NULL,
	circuit      TEXT NOT NULL,
	remote       TEXT NOT NULL,
	PRIMARY KEY (subnet, mac)
);
CREATE INDEX relay_clients_last ON relay_clients (last_at);
CREATE TABLE knocks (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	first_at TEXT NOT NULL,
	last_at  TEXT NOT NULL,
	count    INTEGER NOT NULL,
	source   TEXT NOT NULL,
	sni      TEXT NOT NULL,
	versions TEXT NOT NULL,
	subject  TEXT NOT NULL
);
CREATE INDEX knocks_last ON knocks (last_at);`,
	// 0104: what people ask APs to do once.
	3: actionsTable,
	// 0104: one pending action of a kind for an AP, by the database's rule.
	4: actionsOnce,
	// 0104: one open, pending or running, so a read-back that raced the
	// AP taking it up finds it.
	5: actionsOpen,
	// 0107: an action's target, the client a disconnect is of.
	6: actionsTarget,
	// 0108: each AP's clients and traffic, a row a report.
	7: usageTable,
}

// Results of a render check (0039).
const (
	OK      = "ok"
	Refused = "refused"
	Stale   = "stale"
)

// Seen is when an AP last called and from where, and the version it last
// said it runs.
type Seen struct {
	At        time.Time  `json:"at"`
	Source    string     `json:"source"`
	Running   *int64     `json:"running,omitempty"`
	RunningAt *time.Time `json:"running_at,omitempty"`
}

// Check is one render check. UCI is what the AP sent, secrets blanked; it is
// empty for a stale check, which will never run.
type Check struct {
	At       time.Time `json:"at"`
	Version  int64     `json:"version"`
	Hash     string    `json:"hash"`
	Result   string    `json:"result"`
	Problems []string  `json:"problems"`
	UCI      string    `json:"uci,omitempty"`
}

// Apply is one apply attempt. Checked says whether an ok render check covers
// the version and hash applied.
type Apply struct {
	At      time.Time `json:"at"`
	Version int64     `json:"version"`
	Hash    string    `json:"hash"`
	OK      bool      `json:"ok"`
	Error   string    `json:"error,omitempty"`
	Checked bool      `json:"checked"`
}

// State is one state report.
type State struct {
	At      time.Time       `json:"at"`
	Version int64           `json:"version"`
	Report  json.RawMessage `json:"report"`
}

// Latest is the newest of each record for one AP; nil where there is none.
type Latest struct {
	Seen  *Seen  `json:"seen"`
	Check *Check `json:"check"`
	Apply *Apply `json:"apply"`
	State *State `json:"state"`
}

// History is an AP's recent records, newest first.
type History struct {
	Checks  []Check `json:"checks"`
	Applies []Apply `json:"applies"`
	States  []State `json:"states"`
}

// Store is the conditions database. It is safe for concurrent use.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open opens or creates the store at path. now supplies record times; nil
// means time.Now.
func Open(path string, now func() time.Time) (*Store, error) {
	if now == nil {
		now = time.Now
	}
	p := (&url.URL{Path: filepath.ToSlash(path)}).EscapedPath()
	db, err := sql.Open("sqlite3", "file:"+p+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(wal)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, now: now}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v > schemaVersion {
		return fmt.Errorf("conditions schema version %d is newer than this manager (%d)", v, schemaVersion)
	}
	if v == schemaVersion {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if v == 0 {
		if _, err := tx.Exec(schema); err != nil {
			return err
		}
		v = 1
	}
	for ; v < schemaVersion; v++ {
		if _, err := tx.Exec(migrations[v]); err != nil {
			return fmt.Errorf("conditions migration %d: %w", v, err)
		}
	}
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return err
	}
	return tx.Commit()
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// Seen records that an AP called, from source.
func (s *Store) Seen(ap hierarchy.NodeID, source string) error {
	_, err := s.db.Exec(`INSERT INTO aps (ap, seen_at, source) VALUES (?, ?, ?)
		ON CONFLICT (ap) DO UPDATE SET seen_at = excluded.seen_at, source = excluded.source`,
		string(ap), stamp(s.now()), source)
	return err
}

// Running records the version an AP says it runs.
func (s *Store) Running(ap hierarchy.NodeID, version int64) error {
	now := stamp(s.now())
	_, err := s.db.Exec(`INSERT INTO aps (ap, seen_at, source, running, running_at) VALUES (?, ?, '', ?, ?)
		ON CONFLICT (ap) DO UPDATE SET running = excluded.running, running_at = excluded.running_at`,
		string(ap), now, version, now)
	return err
}

// RecordCheck records a render check.
func (s *Store) RecordCheck(ap hierarchy.NodeID, c Check) error {
	if c.Problems == nil {
		c.Problems = []string{}
	}
	problems, err := json.Marshal(c.Problems)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO checks (ap, at, version, hash, result, problems, uci) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		string(ap), stamp(s.now()), c.Version, c.Hash, c.Result, string(problems), c.UCI)
	return err
}

// RecordApply records an apply attempt, working out whether an ok render
// check covers it, and returns what it recorded.
func (s *Store) RecordApply(ap hierarchy.NodeID, a Apply) (Apply, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM checks WHERE ap = ? AND version = ? AND hash = ? AND result = ?`,
		string(ap), a.Version, a.Hash, OK).Scan(&n)
	if err != nil {
		return Apply{}, err
	}
	a.Checked = n > 0
	a.At = s.now().UTC()
	_, err = s.db.Exec(`INSERT INTO applies (ap, at, version, hash, ok, error, checked) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		string(ap), stamp(a.At), a.Version, a.Hash, a.OK, a.Error, a.Checked)
	return a, err
}

// RecordState records a state report.
func (s *Store) RecordState(ap hierarchy.NodeID, version int64, report json.RawMessage) error {
	_, err := s.db.Exec(`INSERT INTO states (ap, at, version, report) VALUES (?, ?, ?, ?)`,
		string(ap), stamp(s.now()), version, string(report))
	return err
}

// StatesWith calls fn with every state report since since that holds text,
// oldest first, such as a client's MAC (0103): the reports are searched in
// the database, so only those are read.
func (s *Store) StatesWith(since time.Time, text string, fn func(ap hierarchy.NodeID, st State) error) error {
	rows, err := s.db.Query(`SELECT ap, at, version, report FROM states WHERE at >= ? AND instr(report, ?) > 0 ORDER BY at`, stamp(since), text)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ap, at, report string
		var st State
		if err := rows.Scan(&ap, &at, &st.Version, &report); err != nil {
			return err
		}
		if st.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return err
		}
		st.Report = json.RawMessage(report)
		if err := fn(hierarchy.NodeID(ap), st); err != nil {
			return err
		}
	}
	return rows.Err()
}

// TrimStates deletes state reports older than keep, and what was worked out
// from them (0108), and returns how many reports.
func (s *Store) TrimStates(keep time.Duration) (int64, error) {
	cut := stamp(s.now().Add(-keep))
	if _, err := s.db.Exec(`DELETE FROM usage WHERE at < ?`, cut); err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`DELETE FROM states WHERE at < ?`, cut)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Latest returns the newest record of each kind for an AP.
func (s *Store) Latest(ap hierarchy.NodeID) (Latest, error) {
	var out Latest
	seen, err := s.seen(ap)
	if err != nil {
		return out, err
	}
	out.Seen = seen
	h, err := s.History(ap, 1)
	if err != nil {
		return out, err
	}
	if len(h.Checks) > 0 {
		out.Check = &h.Checks[0]
	}
	if len(h.Applies) > 0 {
		out.Apply = &h.Applies[0]
	}
	if len(h.States) > 0 {
		out.State = &h.States[0]
	}
	return out, nil
}

// LatestState is an AP's newest state report, or nil if it has sent none.
func (s *Store) LatestState(ap hierarchy.NodeID) (*State, error) {
	var st State
	var at, report string
	err := s.db.QueryRow(`SELECT at, version, report FROM states WHERE ap = ? ORDER BY id DESC LIMIT 1`, string(ap)).Scan(&at, &st.Version, &report)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if st.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
		return nil, err
	}
	st.Report = json.RawMessage(report)
	return &st, nil
}

func (s *Store) seen(ap hierarchy.NodeID) (*Seen, error) {
	var at, source string
	var running sql.NullInt64
	var runningAt sql.NullString
	err := s.db.QueryRow(`SELECT seen_at, source, running, running_at FROM aps WHERE ap = ?`, string(ap)).Scan(&at, &source, &running, &runningAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := &Seen{Source: source}
	if out.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
		return nil, err
	}
	if running.Valid {
		v := running.Int64
		out.Running = &v
	}
	if runningAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, runningAt.String)
		if err != nil {
			return nil, err
		}
		out.RunningAt = &t
	}
	return out, nil
}

// History returns an AP's newest records of each kind, up to limit each.
func (s *Store) History(ap hierarchy.NodeID, limit int) (History, error) {
	h := History{Checks: []Check{}, Applies: []Apply{}, States: []State{}}
	rows, err := s.db.Query(`SELECT at, version, hash, result, problems, uci FROM checks WHERE ap = ? ORDER BY id DESC LIMIT ?`, string(ap), limit)
	if err != nil {
		return h, err
	}
	for rows.Next() {
		var c Check
		var at, problems string
		if err := rows.Scan(&at, &c.Version, &c.Hash, &c.Result, &problems, &c.UCI); err != nil {
			rows.Close()
			return h, err
		}
		if c.At, err = time.Parse(time.RFC3339Nano, at); err == nil {
			err = json.Unmarshal([]byte(problems), &c.Problems)
		}
		if err != nil {
			rows.Close()
			return h, err
		}
		h.Checks = append(h.Checks, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return h, err
	}

	rows, err = s.db.Query(`SELECT at, version, hash, ok, error, checked FROM applies WHERE ap = ? ORDER BY id DESC LIMIT ?`, string(ap), limit)
	if err != nil {
		return h, err
	}
	for rows.Next() {
		var a Apply
		var at string
		if err := rows.Scan(&at, &a.Version, &a.Hash, &a.OK, &a.Error, &a.Checked); err != nil {
			rows.Close()
			return h, err
		}
		if a.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			rows.Close()
			return h, err
		}
		h.Applies = append(h.Applies, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return h, err
	}

	rows, err = s.db.Query(`SELECT at, version, report FROM states WHERE ap = ? ORDER BY id DESC LIMIT ?`, string(ap), limit)
	if err != nil {
		return h, err
	}
	defer rows.Close()
	for rows.Next() {
		var st State
		var at, report string
		if err := rows.Scan(&at, &st.Version, &report); err != nil {
			return h, err
		}
		if st.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return h, err
		}
		st.Report = json.RawMessage(report)
		h.States = append(h.States, st)
	}
	return h, rows.Err()
}
