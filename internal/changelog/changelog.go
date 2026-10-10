// Package changelog is the manager's system of record (0009): an append-only
// log of changes in SQLite, and the state derived by replaying it. The trees
// are never stored directly; they are rebuilt from the log at startup, and the
// state at any past sequence number can be rebuilt the same way.
//
// One database holds one Org, whose changes form one sequence (0007).
package changelog

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	_ "github.com/ncruces/go-sqlite3/driver"
)

const schemaVersion = 1

const schema = `
CREATE TABLE changes (
	seq    INTEGER PRIMARY KEY AUTOINCREMENT,
	at     TEXT NOT NULL,
	actor  TEXT NOT NULL CHECK (actor <> ''),
	reason TEXT NOT NULL,
	kind   TEXT NOT NULL,
	tree   TEXT NOT NULL,
	node   TEXT NOT NULL,
	path   TEXT NOT NULL,
	op     TEXT NOT NULL,
	effect TEXT NOT NULL
);
CREATE TRIGGER changes_no_update BEFORE UPDATE ON changes
BEGIN SELECT RAISE(ABORT, 'the change log is append-only'); END;
CREATE TRIGGER changes_no_delete BEFORE DELETE ON changes
BEGIN SELECT RAISE(ABORT, 'the change log is append-only'); END;
`

var (
	ErrNoActor = errors.New("every change needs an actor")
	// ErrInUse: another process has the change log open. Only one may, since
	// each keeps the state in memory (0043).
	ErrInUse = errors.New("the change log is in use by another process; stop the aeolus service first")
)

// APBreakError reports a change refused because an AP that resolved before
// the change would no longer resolve after it.
type APBreakError struct {
	AP  hierarchy.NodeID
	Err error
}

func (e *APBreakError) Error() string {
	return fmt.Sprintf("refused: AP %s would no longer resolve: %v", e.AP, e.Err)
}

func (e *APBreakError) Unwrap() error { return e.Err }

// Entry is one row of the change log.
type Entry struct {
	Seq    int64
	At     time.Time
	Actor  string
	Reason string
	Op     change.Op
	Effect change.Effect
}

// Log is the change log and the state derived from it. It is safe for
// concurrent use; changes are committed one at a time.
type Log struct {
	mu       sync.Mutex
	db       *sql.DB
	now      func() time.Time
	check    func(*change.State, string, change.Op) error
	seq      int64
	state    *change.State
	versions map[hierarchy.NodeID]int64
	since    map[hierarchy.NodeID]time.Time // when each AP's version was made
}

// Options configures a Log.
type Options struct {
	// Now supplies commit times; nil means time.Now.
	Now func() time.Time
	// Check vets every change before it is committed: against the field
	// schema (0027) and the actor's permissions (0025, 0030). It runs under
	// the log's lock with the current state, so the decision and the commit
	// see the same state. It is not applied on replay: what is already in the
	// log stays in the log.
	Check func(state *change.State, actor string, op change.Op) error
}

// Open opens or creates the log at path and replays it.
func Open(path string, opts Options) (*Log, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	db, err := sql.Open("sqlite3", dsn(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	l := &Log{db: db, now: opts.Now, check: opts.Check, versions: map[hierarchy.NodeID]int64{}, since: map[hierarchy.NodeID]time.Time{}}
	if err := l.migrate(); err != nil {
		db.Close()
		if strings.Contains(err.Error(), "database is locked") {
			return nil, fmt.Errorf("%w (%s)", ErrInUse, path)
		}
		return nil, err
	}
	if err := l.replay(); err != nil {
		db.Close()
		return nil, err
	}
	return l, nil
}

// dsn builds a file: URI without an authority part ("file:/var/lib/..." or
// "file:C:/..."), because the driver hands the path to Go's os package as is.
// Exclusive locking keeps a second process out while one has the log open:
// each keeps the state in memory, so two would drift apart (0043). With it,
// the log uses a single connection.
func dsn(path string) string {
	p := (&url.URL{Path: filepath.ToSlash(path)}).EscapedPath()
	return "file:" + p + "?_pragma=busy_timeout(5000)&_pragma=locking_mode(exclusive)&_pragma=journal_mode(wal)&_txlock=immediate"
}

func (l *Log) migrate() error {
	var v int
	if err := l.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	switch v {
	case schemaVersion:
		return nil
	case 0:
		tx, err := l.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.Exec(schema); err != nil {
			return err
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
			return err
		}
		return tx.Commit()
	}
	return fmt.Errorf("change log schema version %d is newer than this manager (%d)", v, schemaVersion)
}

func (l *Log) replay() error {
	entries, err := l.Entries(0, 0)
	if err != nil {
		return err
	}
	for _, e := range entries {
		state, _, changed, err := run(l.state, e.Op)
		if err != nil {
			return fmt.Errorf("replaying change %d: %w", e.Seq, err)
		}
		l.state, l.seq = state, e.Seq
		for _, ap := range changed {
			l.versions[ap], l.since[ap] = e.Seq, e.At
		}
	}
	return nil
}

// Close closes the database.
func (l *Log) Close() error { return l.db.Close() }

// Commit validates a change against the current state, records it and applies
// it. A refused change leaves both the log and the state untouched.
func (l *Log) Commit(actor, reason string, op change.Op) (Entry, error) {
	if actor == "" {
		return Entry{}, ErrNoActor
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.check != nil {
		if err := l.check(l.state, actor, op); err != nil {
			return Entry{}, err
		}
	}

	state, eff, changed, err := run(l.state.Clone(), op)
	if err != nil {
		return Entry{}, err
	}
	e := Entry{At: l.now().UTC(), Actor: actor, Reason: reason, Op: op, Effect: eff}
	if e.Seq, err = l.insert(e); err != nil {
		return Entry{}, err
	}
	l.state, l.seq = state, e.Seq
	for _, ap := range changed {
		l.versions[ap], l.since[ap] = e.Seq, e.At
	}
	return e, nil
}

// Preview runs a change exactly as Commit would, with the same checks,
// against a copy of the current state, and records nothing (0026). It returns
// the state the change would produce, its effect, and the APs it would
// re-version.
func (l *Log) Preview(actor string, op change.Op) (*change.State, change.Effect, []hierarchy.NodeID, error) {
	if actor == "" {
		return nil, change.Effect{}, nil, ErrNoActor
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.check != nil {
		if err := l.check(l.state, actor, op); err != nil {
			return nil, change.Effect{}, nil, err
		}
	}
	return run(l.state.Clone(), op)
}

func (l *Log) insert(e Entry) (int64, error) {
	op, err := json.Marshal(e.Op)
	if err != nil {
		return 0, err
	}
	eff, err := json.Marshal(e.Effect)
	if err != nil {
		return 0, err
	}
	res, err := l.db.Exec(`INSERT INTO changes (at, actor, reason, kind, tree, node, path, op, effect)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.At.Format(time.RFC3339Nano), e.Actor, e.Reason, string(e.Op.Kind), string(e.Op.Tree),
		string(e.Op.Node), string(e.Op.Path), string(op), string(eff))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Seq returns the sequence number of the latest change, 0 for an empty log.
func (l *Log) Seq() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.seq
}

// Snapshot returns a copy of the current state, or nil before the Org exists.
func (l *Log) Snapshot() *change.State {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.state.Clone()
}

// Version returns an AP's config version: the sequence number of the latest
// change that altered its resolved values (0007).
func (l *Log) Version(ap hierarchy.NodeID) (int64, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.versions[ap]
	return v, ok
}

// VersionSince is when an AP's current version was made: the time of the
// change that last re-versioned it, zero if none has (0099).
func (l *Log) VersionSince(ap hierarchy.NodeID) time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.since[ap]
}

// Entries returns changes after seq in order; limit 0 means all.
func (l *Log) Entries(after int64, limit int) ([]Entry, error) {
	q := `SELECT seq, at, actor, reason, op, effect FROM changes WHERE seq > ? ORDER BY seq`
	args := []any{after}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := l.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var at, op, eff string
		if err := rows.Scan(&e.Seq, &at, &e.Actor, &e.Reason, &op, &eff); err != nil {
			return nil, err
		}
		if e.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, fmt.Errorf("change %d: %w", e.Seq, err)
		}
		if err := json.Unmarshal([]byte(op), &e.Op); err != nil {
			return nil, fmt.Errorf("change %d: %w", e.Seq, err)
		}
		if err := json.Unmarshal([]byte(eff), &e.Effect); err != nil {
			return nil, fmt.Errorf("change %d: %w", e.Seq, err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// At rebuilds the state as it was right after change seq, by replaying the log
// up to it. It returns nil for a point before the Org was created.
func (l *Log) At(seq int64) (*change.State, error) {
	entries, err := l.Entries(0, 0)
	if err != nil {
		return nil, err
	}
	var state *change.State
	for _, e := range entries {
		if e.Seq > seq {
			break
		}
		if state, _, err = change.Apply(state, e.Op); err != nil {
			return nil, fmt.Errorf("replaying change %d: %w", e.Seq, err)
		}
	}
	return state, nil
}

// run applies op to s, which it may modify, and returns the resulting state,
// the change's effect, and the APs whose resolved values changed. It refuses a
// change that would leave a previously resolvable AP unresolvable.
//
// It resolves every AP before and after, which is simple and fine at
// prototype scale; narrowing it to the affected subtree can come later.
func run(s *change.State, op change.Op) (*change.State, change.Effect, []hierarchy.NodeID, error) {
	before := fingerprints(s)
	s, eff, err := change.Apply(s, op)
	if err != nil {
		return nil, change.Effect{}, nil, err
	}
	after := fingerprints(s)
	var changed []hierarchy.NodeID
	for _, ap := range s.Org.Locations.APs() {
		a := after[ap]
		b, existed := before[ap]
		if existed && b.err == nil && a.err != nil {
			return nil, change.Effect{}, nil, &APBreakError{AP: ap, Err: a.err}
		}
		if !existed || !reflect.DeepEqual(a.values, b.values) {
			changed = append(changed, ap)
		}
	}
	return s, eff, changed, nil
}

type fingerprint struct {
	values map[string]any
	err    error
}

// fingerprints records, for every AP, everything its composed config is built
// from (0007, 0023): its resolved values, without origins, and what it uses of
// the library. A change that only moves where a value comes from (a break,
// for example) does not change the AP's config; a new address for a
// concentrator it uses does, and so does the AP moving in or out of that
// concentrator's scope.
func fingerprints(s *change.State) map[hierarchy.NodeID]fingerprint {
	out := map[hierarchy.NodeID]fingerprint{}
	if s == nil {
		return out
	}
	o := s.Org
	for _, ap := range o.Locations.APs() {
		cfg, err := s.ResolveAP(ap) // with its template (0085)
		if err != nil {
			out[ap] = fingerprint{err: err}
			continue
		}
		v := map[string]any{"services": cfg.Services}
		// Its name is its hostname, in its config (0076).
		if n, ok := o.Locations.Node(ap); ok {
			v["name"] = n.Name
		}
		for p, r := range cfg.Location {
			v["location/"+string(p)] = r.Value
		}
		ancestry := o.Locations.Ancestry(ap)
		for id, n := range cfg.Networks {
			for f, r := range n.Fields {
				v["network/"+id+"/"+f] = r.Value
			}
			for _, slot := range []string{"primary", "fallback"} {
				if cid, ok := n.Fields["transport."+slot+".concentrator"]; ok {
					vni := n.Fields["transport."+slot+".vni"]
					v["library/"+id+"/"+slot] = uses(s, cid.Value, vni.Value, ancestry)
				}
			}
		}
		out[ap] = fingerprint{values: v}
	}
	return out
}

// libraryUse is what one transport takes from the library, exactly as the
// composed config does (compose.AP): whether its concentrator exists and may
// be used here, and if so where it is and whether it has the VNI. A
// concentrator's name and VNI labels are left out: the AP never sees them.
type libraryUse struct {
	Found, Available, VNIDefined bool
	Address                      string
	Port, MTU                    int
}

func uses(s *change.State, concentrator, vni any, ancestry []hierarchy.NodeID) libraryUse {
	id, _ := concentrator.(string)
	k, ok := s.Library.Get(id)
	if !ok {
		return libraryUse{}
	}
	if !k.AvailableAt(ancestry) {
		return libraryUse{Found: true}
	}
	n, _ := vni.(float64)
	_, defined := k.VNIs[int(n)]
	return libraryUse{Found: true, Available: true, VNIDefined: defined, Address: k.Address, Port: k.Port, MTU: k.MTU}
}
