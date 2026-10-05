// Package keys holds per-user keys (0014, 0070): each its own passphrase on a
// shared WPA2-PSK network, and the VLAN its client lands in. Keys are changes
// in the change log, but not config: a key change re-versions no AP, and
// reaches APs through their own channel.
package keys

import (
	"crypto/pbkdf2"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// MaxPerNetwork is how many keys one network may have.
const MaxPerNetwork = 4096

// Key is one per-user key. Passphrase is sealed (0027) and never returned.
type Key struct {
	ID         string           `json:"id"`
	Folder     hierarchy.NodeID `json:"folder"`
	Network    string           `json:"network"`
	Name       string           `json:"name"`
	Passphrase json.RawMessage  `json:"passphrase"`
	VLAN       int              `json:"vlan,omitempty"`
	MACs       []string         `json:"macs,omitempty"`
	Expires    *time.Time       `json:"expires,omitempty"`
}

// Def is a key's definition, as a change carries it.
type Def struct {
	Name       string          `json:"name"`
	Passphrase json.RawMessage `json:"passphrase,omitempty"`
	VLAN       int             `json:"vlan,omitempty"`
	MACs       []string        `json:"macs,omitempty"`
	Expires    *time.Time      `json:"expires,omitempty"`
}

var (
	ErrNoKey     = errors.New("no such key")
	ErrKeyExists = errors.New("a key with that ID exists")
	ErrTooMany   = fmt.Errorf("a network has at most %d keys", MaxPerNetwork)

	idRE  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	macRE = regexp.MustCompile(`^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)
)

// Check checks a definition's form: a name of 1 to 64 printable characters, a
// VLAN of 1 to 4094 if any, and at most 16 MACs in lower case. Given a time,
// it also checks that an expiry is after it; the change log, replaying old
// changes, gives none. The passphrase is the caller's to check, before it is
// sealed.
func (d Def) Check(now time.Time) error {
	if d.Name == "" || len(d.Name) > 64 || !printable(d.Name) {
		return errors.New("a key's name is 1 to 64 printable characters")
	}
	if d.VLAN < 0 || d.VLAN > 4094 {
		return errors.New("a key's VLAN is 1 to 4094")
	}
	if len(d.MACs) > 16 {
		return errors.New("a key is bound to at most 16 MACs")
	}
	for _, m := range d.MACs {
		if !macRE.MatchString(m) {
			return fmt.Errorf("MAC %q: want aa:bb:cc:dd:ee:ff, in lower case", m)
		}
	}
	if !now.IsZero() && d.Expires != nil && !d.Expires.After(now) {
		return errors.New("a key's expiry must be in the future")
	}
	return nil
}

// CheckPassphrase checks a passphrase as WPA2 takes it: 8 to 63 printable
// ASCII characters.
func CheckPassphrase(p string) error {
	if len(p) < 8 || len(p) > 63 || !printable(p) {
		return errors.New("a passphrase is 8 to 63 printable ASCII characters")
	}
	return nil
}

// ValidID says whether id can name a key.
func ValidID(id string) bool { return idRE.MatchString(id) }

func printable(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < ' ' || s[i] > '~' {
			return false
		}
	}
	return true
}

// SealPath is the path a key's passphrase is sealed under, so a sealed value
// opens only as that key's.
func SealPath(id string) string { return "keys." + id + ".passphrase" }

// PSK is the 64-hex WPA2 PSK for a passphrase on an SSID: PBKDF2-SHA1, 4096
// rounds, 32 bytes. APs are sent this, so hostapd need not work it out.
func PSK(passphrase, ssid string) string {
	k, err := pbkdf2.Key(sha1.New, passphrase, []byte(ssid), 4096, 32)
	if err != nil {
		panic(err) // only for out-of-range parameters, which these are not
	}
	return hex.EncodeToString(k)
}

// Store is every key, by ID.
type Store struct {
	keys map[string]Key
}

// New returns an empty store.
func New() *Store { return &Store{keys: map[string]Key{}} }

// Clone returns an independent copy, or nil for nil.
func (s *Store) Clone() *Store {
	if s == nil {
		return nil
	}
	out := New()
	for id, k := range s.keys {
		k.MACs = append([]string(nil), k.MACs...)
		out.keys[id] = k
	}
	return out
}

// Get returns a key.
func (s *Store) Get(id string) (Key, bool) {
	k, ok := s.keys[id]
	return k, ok
}

// Put adds or replaces a key.
func (s *Store) Put(k Key) { s.keys[k.ID] = k }

// Remove removes a key.
func (s *Store) Remove(id string) { delete(s.keys, id) }

// Of returns a network's keys, by ID.
func (s *Store) Of(folder hierarchy.NodeID, network string) []Key {
	var out []Key
	for _, k := range s.keys {
		if k.Folder == folder && k.Network == network {
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// All returns every key, by ID.
func (s *Store) All() []Key {
	out := make([]Key, 0, len(s.keys))
	for _, k := range s.keys {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Len is how many keys there are.
func (s *Store) Len() int { return len(s.keys) }

// Networks returns the folder and network of every network with keys.
func (s *Store) Networks() [][2]string {
	seen := map[[2]string]bool{}
	for _, k := range s.keys {
		seen[[2]string{string(k.Folder), k.Network}] = true
	}
	out := make([][2]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return strings.Join(out[i][:], "/") < strings.Join(out[j][:], "/") })
	return out
}
