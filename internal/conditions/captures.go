package conditions

import (
	"database/sql"
	"errors"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Captures (0135): of an attempt to come online that failed, the client's
// frames as its AP's radio heard and sent them and as they crossed its wire
// side, a file Wireshark opens. A capture is its attempt's, and is told as
// the attempt is: by its AP, its client and the start its AP gave. Few are
// kept, so that a client which fails over and over, as one with a wrong
// passphrase, fills nothing.
const capturesTables = `
CREATE TABLE captures (
	id      INTEGER PRIMARY KEY AUTOINCREMENT,
	ap      TEXT NOT NULL,
	mac     TEXT NOT NULL,
	started INTEGER NOT NULL,
	at      INTEGER NOT NULL,
	frames  INTEGER NOT NULL,
	radio   INTEGER NOT NULL,
	wire    INTEGER NOT NULL,
	file    BLOB NOT NULL,
	UNIQUE (ap, mac, started)
);
CREATE INDEX captures_ap ON captures (ap, id);`

const (
	// CapturesKept is how many of an AP's captures are kept, the latest.
	CapturesKept = 10
	// CapturesOfOne is how many of those may be one client's: the rest
	// are left for the others that fail there.
	CapturesOfOne = 3
)

// Capture is the capture of one attempt. Said is when the attempt began as
// its AP said, in ms since 1970; Frames how many frames it holds, Radio of
// them from the radio and Wire from the wire side; File the capture itself,
// as pcapng.
type Capture struct {
	AP     hierarchy.NodeID
	MAC    string
	Said   int64
	Frames int
	Radio  int
	Wire   int
	File   []byte
}

// CaptureKept is what a list of attempts says of one's capture: how many
// frames it holds, and how many bytes its file is.
type CaptureKept struct {
	Frames int `json:"frames"`
	Bytes  int `json:"bytes"`
}

// ErrNoAttempt says a capture is of no attempt that is kept as failed.
var ErrNoAttempt = errors.New("conditions: a capture of no attempt kept as failed")

// RecordCapture keeps the capture of an attempt, and says whether it was
// new: one sent before, by its AP, its client and its start, is not kept
// twice. Its attempt is one kept as failed, or it is ErrNoAttempt: a
// capture with no attempt could be read by no one, and would hold a place.
// Then the oldest go: of that client's on that AP past CapturesOfOne, and
// of that AP's past CapturesKept.
func (s *Store) RecordCapture(c Capture) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var failed int
	if err := tx.QueryRow(`SELECT count(*) FROM connections WHERE ap = ? AND mac = ? AND started = ? AND outcome = 'failed'`,
		string(c.AP), c.MAC, c.Said).Scan(&failed); err != nil {
		return false, err
	}
	if failed == 0 {
		return false, ErrNoAttempt
	}
	res, err := tx.Exec(`INSERT OR IGNORE INTO captures (ap, mac, started, at, frames, radio, wire, file) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		string(c.AP), c.MAC, c.Said, s.now().UnixMilli(), c.Frames, c.Radio, c.Wire, c.File)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	if _, err := tx.Exec(`DELETE FROM captures WHERE ap = ?1 AND mac = ?2 AND id NOT IN
		(SELECT id FROM captures WHERE ap = ?1 AND mac = ?2 ORDER BY id DESC LIMIT ?3)`, string(c.AP), c.MAC, CapturesOfOne); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`DELETE FROM captures WHERE ap = ?1 AND id NOT IN
		(SELECT id FROM captures WHERE ap = ?1 ORDER BY id DESC LIMIT ?2)`, string(c.AP), CapturesKept); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// CaptureOf is the capture kept of the attempt with that ID, with the
// attempt itself, and whether there is such an attempt; the capture is nil
// where none is kept of it.
func (s *Store) CaptureOf(connection int64) (Connection, *Capture, bool, error) {
	var c Connection
	var ap string
	var at, said int64
	err := s.db.QueryRow(`SELECT id, ap, mac, at, started, network, ssid, band, outcome, stage, reason, took_ms FROM connections WHERE id = ?`, connection).
		Scan(&c.ID, &ap, &c.MAC, &at, &said, &c.Network, &c.SSID, &c.Band, &c.Outcome, &c.Stage, &c.Reason, &c.TookMS)
	if err == sql.ErrNoRows {
		return Connection{}, nil, false, nil
	}
	if err != nil {
		return Connection{}, nil, false, err
	}
	c.AP, c.Started, c.Said = hierarchy.NodeID(ap), time.UnixMilli(at).UTC(), said
	k := Capture{AP: c.AP, MAC: c.MAC, Said: said}
	err = s.db.QueryRow(`SELECT frames, radio, wire, file FROM captures WHERE ap = ? AND mac = ? AND started = ?`, ap, c.MAC, said).
		Scan(&k.Frames, &k.Radio, &k.Wire, &k.File)
	if err == sql.ErrNoRows {
		return c, nil, true, nil
	}
	if err != nil {
		return Connection{}, nil, false, err
	}
	return c, &k, true, nil
}
