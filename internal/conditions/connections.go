package conditions

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Clients and their connections (0118): every Wi-Fi client an AP has seen,
// kept for good, and each attempt one made to come online, passed or
// failed, as its AP recorded it step by step. Times here are whole
// milliseconds since 1970, so they sort as they are.
const connectionsTables = `
CREATE TABLE connections (
	id      INTEGER PRIMARY KEY AUTOINCREMENT,
	ap      TEXT NOT NULL,
	mac     TEXT NOT NULL,
	started INTEGER NOT NULL,
	network TEXT NOT NULL,
	ssid    TEXT NOT NULL,
	band    TEXT NOT NULL,
	outcome TEXT NOT NULL,
	stage   TEXT NOT NULL,
	reason  TEXT NOT NULL,
	took_ms INTEGER NOT NULL,
	record  TEXT NOT NULL,
	UNIQUE (ap, mac, started)
);
CREATE INDEX connections_mac ON connections (mac, started);
CREATE INDEX connections_started ON connections (started);
CREATE TABLE clients (
	mac          TEXT PRIMARY KEY,
	first_seen   INTEGER NOT NULL,
	last_seen    INTEGER NOT NULL,
	ap           TEXT NOT NULL,
	network      TEXT NOT NULL,
	ssid         TEXT NOT NULL,
	host         TEXT NOT NULL,
	user         TEXT NOT NULL,
	address      TEXT NOT NULL,
	attempts     INTEGER NOT NULL,
	failed       INTEGER NOT NULL,
	last_attempt INTEGER NOT NULL,
	last_outcome TEXT NOT NULL
);
CREATE INDEX clients_last ON clients (last_seen);`

// connectionsAt gives each attempt the time the manager takes it to have
// begun, at. started stays what its AP said, by its own clock: with its AP
// and its client, that is what tells one attempt from another, and it is
// the same however often the AP sends it. An AP whose clock is unset says a
// time long gone; given the time it came in as started, a report sent
// twice, as when the answer to the first was lost, was kept twice.
const connectionsAt = `
ALTER TABLE connections ADD COLUMN at INTEGER NOT NULL DEFAULT 0;
UPDATE connections SET at = started;
CREATE INDEX connections_at ON connections (mac, at);`

// Connection is one attempt a client made to come online on an AP. Record
// is what the AP sent of it: its steps, and what it learned of the client.
// Started is when it began, as the manager takes it; Said is when its AP
// said, in ms since 1970, which with the AP and the client tells it from
// any other.
type Connection struct {
	ID      int64            `json:"id"`
	AP      hierarchy.NodeID `json:"ap"`
	MAC     string           `json:"mac"`
	Started time.Time        `json:"started"`
	Network string           `json:"network"`
	SSID    string           `json:"ssid"`
	Band    string           `json:"band"`
	Outcome string           `json:"outcome"`
	Stage   string           `json:"stage"`
	Reason  string           `json:"reason"`
	TookMS  int64            `json:"took_ms"`
	Record  json.RawMessage  `json:"record"`
	// What it showed of the client, for the client's own row.
	Host    string `json:"-"`
	Address string `json:"-"`
	Said    int64  `json:"-"`
}

// Client is a Wi-Fi client as last seen: where, and what it said of itself;
// how many times it tried to come online, and how many of those failed.
type Client struct {
	MAC         string           `json:"mac"`
	FirstSeen   time.Time        `json:"first_seen"`
	LastSeen    time.Time        `json:"last_seen"`
	AP          hierarchy.NodeID `json:"ap"`
	Network     string           `json:"network"`
	SSID        string           `json:"ssid"`
	Host        string           `json:"host"`
	User        string           `json:"user"`
	Address     string           `json:"address"`
	Attempts    int              `json:"attempts"`
	Failed      int              `json:"failed"`
	LastOutcome string           `json:"last_outcome"`
}

// ClientSeen is a client on an AP at a state report (0066).
type ClientSeen struct {
	MAC, Network, SSID, Host, User, Address string
}

// seeClient is one statement for both ways a client is seen: where it is
// newer than what is kept, the place it was seen; what it said of itself
// wherever it said anything; the count of its attempts; and how its latest
// attempt went, which a state report, seeing it later, leaves as it is.
const seeClient = `
INSERT INTO clients (mac, first_seen, last_seen, ap, network, ssid, host, user, address, attempts, failed, last_attempt, last_outcome)
VALUES (?1, ?2, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)
ON CONFLICT (mac) DO UPDATE SET
	first_seen = min(first_seen, excluded.first_seen),
	last_seen = max(last_seen, excluded.last_seen),
	ap = CASE WHEN excluded.last_seen >= last_seen THEN excluded.ap ELSE ap END,
	network = CASE WHEN excluded.last_seen >= last_seen THEN excluded.network ELSE network END,
	ssid = CASE WHEN excluded.last_seen >= last_seen THEN excluded.ssid ELSE ssid END,
	host = CASE WHEN excluded.host != '' THEN excluded.host ELSE host END,
	user = CASE WHEN excluded.user != '' THEN excluded.user ELSE user END,
	address = CASE WHEN excluded.address != '' THEN excluded.address ELSE address END,
	attempts = attempts + excluded.attempts,
	failed = failed + excluded.failed,
	last_attempt = max(last_attempt, excluded.last_attempt),
	last_outcome = CASE WHEN excluded.last_attempt > 0 AND excluded.last_attempt >= last_attempt THEN excluded.last_outcome ELSE last_outcome END`

// RecordConnections keeps the attempts an AP reports, and counts each on
// its client. One it has reported before, by its client and start, is not
// kept twice. It returns how many were new.
func (s *Store) RecordConnections(ap hierarchy.NodeID, list []Connection) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	added := 0
	for _, c := range list {
		at := c.Started.UnixMilli()
		said := c.Said
		if said == 0 {
			said = at
		}
		res, err := tx.Exec(`INSERT OR IGNORE INTO connections (ap, mac, started, at, network, ssid, band, outcome, stage, reason, took_ms, record)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			string(ap), c.MAC, said, at, c.Network, c.SSID, c.Band, c.Outcome, c.Stage, c.Reason, c.TookMS, string(c.Record))
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue
		}
		added++
		failed := 0
		if c.Outcome == "failed" {
			failed = 1
		}
		if _, err := tx.Exec(seeClient, c.MAC, at, string(ap), c.Network, c.SSID, c.Host, "", c.Address, 1, failed, at, c.Outcome); err != nil {
			return 0, err
		}
	}
	return added, tx.Commit()
}

// SeeClients notes the clients on an AP at a state report: where each is,
// and what it says of itself. A client seen this way and never trying to
// come online, as one already on when recording began, is kept too.
func (s *Store) SeeClients(ap hierarchy.NodeID, list []ClientSeen) error {
	if len(list) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at := s.now().UnixMilli()
	for _, c := range list {
		if _, err := tx.Exec(seeClient, c.MAC, at, string(ap), c.Network, c.SSID, c.Host, c.User, c.Address, 0, 0, 0, ""); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Clients lists the clients last seen on one of aps, the latest first, that
// match find where it is not empty: part of a MAC, a host name, a user or
// an address. It returns at most limit from offset, and how many there are
// in all.
func (s *Store) Clients(aps []hierarchy.NodeID, find string, limit, offset int) ([]Client, int, error) {
	ids, err := json.Marshal(aps)
	if err != nil {
		return nil, 0, err
	}
	const where = ` FROM clients WHERE ap IN (SELECT value FROM json_each(?1))
		AND (?2 = '' OR instr(mac, ?2) > 0 OR instr(lower(host), ?2) > 0 OR instr(lower(user), ?2) > 0 OR instr(address, ?2) > 0)`
	var total int
	if err := s.db.QueryRow(`SELECT count(*)`+where, string(ids), find).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT mac, first_seen, last_seen, ap, network, ssid, host, user, address, attempts, failed, last_outcome`+where+
		` ORDER BY last_seen DESC, mac LIMIT ?3 OFFSET ?4`, string(ids), find, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Client{}
	for rows.Next() {
		var c Client
		var first, last int64
		var ap string
		if err := rows.Scan(&c.MAC, &first, &last, &ap, &c.Network, &c.SSID, &c.Host, &c.User, &c.Address, &c.Attempts, &c.Failed, &c.LastOutcome); err != nil {
			return nil, 0, err
		}
		c.AP, c.FirstSeen, c.LastSeen = hierarchy.NodeID(ap), time.UnixMilli(first).UTC(), time.UnixMilli(last).UTC()
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// ClientOf is one client's row, and whether there is one.
func (s *Store) ClientOf(mac string) (Client, bool, error) {
	var c Client
	var first, last int64
	var ap string
	err := s.db.QueryRow(`SELECT mac, first_seen, last_seen, ap, network, ssid, host, user, address, attempts, failed, last_outcome FROM clients WHERE mac = ?`, mac).
		Scan(&c.MAC, &first, &last, &ap, &c.Network, &c.SSID, &c.Host, &c.User, &c.Address, &c.Attempts, &c.Failed, &c.LastOutcome)
	if err == sql.ErrNoRows {
		return Client{}, false, nil
	}
	if err != nil {
		return Client{}, false, err
	}
	c.AP, c.FirstSeen, c.LastSeen = hierarchy.NodeID(ap), time.UnixMilli(first).UTC(), time.UnixMilli(last).UTC()
	return c, true, nil
}

// Connections lists a client's attempts on aps, the latest first: at most
// limit, and only those begun before before, where it is not zero.
func (s *Store) Connections(mac string, aps []hierarchy.NodeID, limit int, before time.Time) ([]Connection, error) {
	ids, err := json.Marshal(aps)
	if err != nil {
		return nil, err
	}
	cut := int64(1) << 62
	if !before.IsZero() {
		cut = before.UnixMilli()
	}
	rows, err := s.db.Query(`SELECT id, ap, mac, at, network, ssid, band, outcome, stage, reason, took_ms, record FROM connections
		WHERE mac = ? AND at < ? AND ap IN (SELECT value FROM json_each(?)) ORDER BY at DESC, id DESC LIMIT ?`, mac, cut, string(ids), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Connection{}
	for rows.Next() {
		var c Connection
		var ap, record string
		var started int64
		if err := rows.Scan(&c.ID, &ap, &c.MAC, &started, &c.Network, &c.SSID, &c.Band, &c.Outcome, &c.Stage, &c.Reason, &c.TookMS, &record); err != nil {
			return nil, err
		}
		c.AP, c.Started, c.Record = hierarchy.NodeID(ap), time.UnixMilli(started).UTC(), json.RawMessage(record)
		out = append(out, c)
	}
	return out, rows.Err()
}

// TrimConnections deletes the attempts begun more than keep ago, and
// returns how many; and what happened to the APs themselves that long ago
// (0119), which is kept as long. The clients themselves are kept.
func (s *Store) TrimConnections(keep time.Duration) (int64, error) {
	if _, err := s.db.Exec(`DELETE FROM ap_events WHERE at < ?`, s.now().Add(-keep).UnixMilli()); err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`DELETE FROM connections WHERE at < ?`, s.now().Add(-keep).UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
