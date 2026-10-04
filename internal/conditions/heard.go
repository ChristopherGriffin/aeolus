package conditions

import "time"

// What the manager hears itself (0068), kept here beside what APs say: the
// clients in a relay's copies of DHCP requests, and knocks on the option 224
// listener. Both are trimmed, unlike checks and applies.

// RelayClient is a client the manager heard of in a relay's copies of its
// DHCP requests. Subnet is the relay's address on the client's subnet
// (giaddr); Relay is the address the copy came from. Type is its last
// request's DHCP message type, and Address the address it asked for or
// renewed. Circuit and Remote are option 82's IDs, as text or hex.
type RelayClient struct {
	Subnet      string    `json:"subnet"`
	MAC         string    `json:"mac"`
	Relay       string    `json:"relay"`
	First       time.Time `json:"first"`
	Last        time.Time `json:"last"`
	Requests    int64     `json:"requests"`
	Type        string    `json:"type"`
	Host        string    `json:"host"`
	VendorClass string    `json:"vendor_class"`
	Params      string    `json:"params"`
	Address     string    `json:"address"`
	Circuit     string    `json:"circuit"`
	Remote      string    `json:"remote"`
}

// Knock is one source's connections to the option 224 listener, those a
// minute or less apart counted as one: the name it asked for (SNI), the TLS
// versions it offered, and its certificate's subject, should it send one.
type Knock struct {
	ID       int64     `json:"id"`
	First    time.Time `json:"first"`
	Last     time.Time `json:"last"`
	Count    int64     `json:"count"`
	Source   string    `json:"source"`
	SNI      string    `json:"sni"`
	Versions string    `json:"versions"`
	Subject  string    `json:"subject"`
}

// SaveRelayClients writes clients, replacing any with the same subnet and
// MAC.
func (s *Store) SaveRelayClients(clients []RelayClient) error {
	if len(clients) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.Prepare(`INSERT INTO relay_clients (subnet, mac, relay, first_at, last_at, requests, type, host, vendor_class, params, address, circuit, remote)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (subnet, mac) DO UPDATE SET relay = excluded.relay, first_at = excluded.first_at, last_at = excluded.last_at,
			requests = excluded.requests, type = excluded.type, host = excluded.host, vendor_class = excluded.vendor_class,
			params = excluded.params, address = excluded.address, circuit = excluded.circuit, remote = excluded.remote`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, c := range clients {
		if _, err := st.Exec(c.Subnet, c.MAC, c.Relay, stamp(c.First), stamp(c.Last), c.Requests, c.Type, c.Host, c.VendorClass, c.Params, c.Address, c.Circuit, c.Remote); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RelayClients returns every client kept.
func (s *Store) RelayClients() ([]RelayClient, error) {
	rows, err := s.db.Query(`SELECT subnet, mac, relay, first_at, last_at, requests, type, host, vendor_class, params, address, circuit, remote FROM relay_clients`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RelayClient
	for rows.Next() {
		var c RelayClient
		var first, last string
		if err := rows.Scan(&c.Subnet, &c.MAC, &c.Relay, &first, &last, &c.Requests, &c.Type, &c.Host, &c.VendorClass, &c.Params, &c.Address, &c.Circuit, &c.Remote); err != nil {
			return nil, err
		}
		if c.First, err = time.Parse(time.RFC3339Nano, first); err != nil {
			return nil, err
		}
		if c.Last, err = time.Parse(time.RFC3339Nano, last); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// TrimRelayClients deletes the clients last seen before t, and returns how
// many.
func (s *Store) TrimRelayClients(t time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM relay_clients WHERE last_at < ?`, stamp(t))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SaveKnock writes a knock: a new one when its ID is 0, which it returns
// set, else an update of the one with its ID.
func (s *Store) SaveKnock(k Knock) (Knock, error) {
	if k.ID == 0 {
		res, err := s.db.Exec(`INSERT INTO knocks (first_at, last_at, count, source, sni, versions, subject) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			stamp(k.First), stamp(k.Last), k.Count, k.Source, k.SNI, k.Versions, k.Subject)
		if err != nil {
			return k, err
		}
		k.ID, err = res.LastInsertId()
		return k, err
	}
	_, err := s.db.Exec(`UPDATE knocks SET last_at = ?, count = ?, subject = ? WHERE id = ?`, stamp(k.Last), k.Count, k.Subject, k.ID)
	return k, err
}

// Knocks returns the knocks kept, newest first.
func (s *Store) Knocks() ([]Knock, error) {
	rows, err := s.db.Query(`SELECT id, first_at, last_at, count, source, sni, versions, subject FROM knocks ORDER BY last_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Knock
	for rows.Next() {
		var k Knock
		var first, last string
		if err := rows.Scan(&k.ID, &first, &last, &k.Count, &k.Source, &k.SNI, &k.Versions, &k.Subject); err != nil {
			return nil, err
		}
		if k.First, err = time.Parse(time.RFC3339Nano, first); err != nil {
			return nil, err
		}
		if k.Last, err = time.Parse(time.RFC3339Nano, last); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// TrimKnocks deletes the knocks last made before t, and all but the newest
// max, and returns how many it deleted.
func (s *Store) TrimKnocks(t time.Time, max int) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM knocks WHERE last_at < ? OR id NOT IN (SELECT id FROM knocks ORDER BY last_at DESC, id DESC LIMIT ?)`, stamp(t), max)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
