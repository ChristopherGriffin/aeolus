package change

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/keys"
	"github.com/ChristopherGriffin/aeolus/internal/secret"
)

// Per-user keys (0014, 0070) are changes, but not config: they live in the
// state's key store, not in the trees, so no AP is re-versioned by one. A key
// belongs to a network as a Services folder offers it, and reaches every AP
// that gets the network from that folder or one below it.

var (
	// ErrBadKey is what every refused key definition is, for the API to
	// answer 400.
	ErrBadKey    = errors.New("bad key")
	ErrNoNetwork = &keyError{"change needs a network"}
	ErrNoKeyID   = &keyError{"change needs a key"}
	ErrNotPSK    = &keyError{"per-user keys need a wpa2-psk network"}
	// ErrRADIUSKeys refuses a key on a network that takes each device's
	// passphrase from its RADIUS server (0115): hostapd would take the key
	// only from a device the server also gave a passphrase. It is GuardIn's,
	// for a new change, not Apply's.
	ErrRADIUSKeys = &keyError{"per-user keys need a network with passphrases of its own; this one takes each device's from its RADIUS server (radius.passphrases)"}
)

// keyError is a refused key definition, with its own message.
type keyError struct{ msg string }

func (e *keyError) Error() string { return e.msg }
func (e *keyError) Unwrap() error { return ErrBadKey }

func badKey(format string, args ...any) error {
	return &keyError{fmt.Sprintf(format, args...)}
}

// KeyVLANsField is a network's field listing the VLANs its keys may put
// clients in (0070).
const KeyVLANsField = "keys.vlans"

func validateKey(op Op) error {
	if op.Node == "" {
		return ErrNoNode
	}
	if op.Network == "" {
		return ErrNoNetwork
	}
	if op.Key == "" {
		return ErrNoKeyID
	}
	if !keys.ValidID(op.Key) {
		return badKey("key ID %q: lower-case letters, digits and -, at most 32", op.Key)
	}
	return nil
}

func applyKey(s *State, op Op) (Effect, error) {
	if s.Keys == nil {
		s.Keys = keys.New()
	}
	if err := keyNetwork(s, op.Node, op.Network); err != nil {
		return Effect{}, err
	}
	prev, exists := s.Keys.Get(op.Key)
	if exists && (prev.Folder != op.Node || prev.Network != op.Network) {
		return Effect{}, fmt.Errorf("%w: %s belongs to %s/%s", keys.ErrKeyExists, op.Key, prev.Folder, prev.Network)
	}
	switch op.Kind {
	case RemoveKey:
		if !exists {
			return Effect{}, fmt.Errorf("%w: %s", keys.ErrNoKey, op.Key)
		}
		s.Keys.Remove(op.Key)
		return Effect{Before: keyView(prev)}, nil
	case AddKey:
		if exists {
			return Effect{}, fmt.Errorf("%w: %s", keys.ErrKeyExists, op.Key)
		}
		if len(s.Keys.Of(op.Node, op.Network)) >= keys.MaxPerNetwork {
			return Effect{}, keys.ErrTooMany
		}
	case SetKey:
		if !exists {
			return Effect{}, fmt.Errorf("%w: %s", keys.ErrNoKey, op.Key)
		}
	}
	var def keys.Def
	dec := json.NewDecoder(bytes.NewReader(op.Value))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&def); err != nil {
		return Effect{}, badKey("key: %v", err)
	}
	// Whether an expiry is still ahead is checked when the change is made,
	// by the API: a replay of the log must not refuse old changes.
	if err := def.Check(time.Time{}); err != nil {
		return Effect{}, badKey("%v", err)
	}
	k := keys.Key{ID: op.Key, Folder: op.Node, Network: op.Network, Name: def.Name, Passphrase: def.Passphrase,
		VLAN: def.VLAN, MACs: def.MACs, Expires: def.Expires}
	if len(k.Passphrase) == 0 {
		if !exists {
			return Effect{}, badKey("a new key needs a passphrase")
		}
		k.Passphrase = prev.Passphrase // set-key without one keeps it
	}
	var sealed any
	if json.Unmarshal(k.Passphrase, &sealed) != nil || !secret.IsSealed(sealed) {
		return Effect{}, &plainKeyError{}
	}
	if k.VLAN != 0 && !offers(s, op.Node, op.Network, k.VLAN) {
		return Effect{}, badKey("network %s does not offer VLAN %d to its keys (network.%s.%s)", op.Network, k.VLAN, op.Network, KeyVLANsField)
	}
	s.Keys.Put(k)
	eff := Effect{After: keyView(k)}
	if exists {
		eff.Before = keyView(prev)
	}
	return eff, nil
}

// ErrPlainSecret is a secret that reached a change unsealed.
var ErrPlainSecret = errors.New("secret values must be sealed before they are committed")

// plainKeyError is a key's passphrase that reached a change unsealed: both
// ErrPlainSecret and ErrBadKey.
type plainKeyError struct{}

func (*plainKeyError) Error() string { return ErrPlainSecret.Error() + ": a key's passphrase" }
func (*plainKeyError) Is(target error) bool {
	return target == ErrPlainSecret || target == ErrBadKey
}

// keyView is a key for an effect: everything but its passphrase, which stays
// sealed in the change itself.
func keyView(k keys.Key) map[string]any {
	out := map[string]any{"folder": k.Folder, "network": k.Network, "name": k.Name}
	if k.VLAN != 0 {
		out["vlan"] = k.VLAN
	}
	if len(k.MACs) > 0 {
		out["macs"] = k.MACs
	}
	if k.Expires != nil {
		out["expires"] = k.Expires
	}
	return out
}

// networkFields returns a network's fields as they resolve at a Services
// folder, and whether it has any.
func networkFields(s *State, folder hierarchy.NodeID, network string) (map[string]any, bool) {
	out := map[string]any{}
	prefix := "network." + network + "."
	for p, r := range s.Org.Services.ResolveAll(folder) {
		if f, ok := strings.CutPrefix(string(p), prefix); ok {
			out[f] = r.Value
		}
	}
	return out, len(out) > 0
}

// keyNetwork checks that a Services folder offers a WPA2-PSK network, which
// keys can belong to.
func keyNetwork(s *State, folder hierarchy.NodeID, network string) error {
	n, ok := s.Org.Services.Node(folder)
	if !ok || n.Kind == hierarchy.KindAP {
		return fmt.Errorf("%w: services folder %s", hierarchy.ErrNotFound, folder)
	}
	f, ok := networkFields(s, folder, network)
	if !ok {
		return fmt.Errorf("%w: network %s at %s", hierarchy.ErrNotFound, network, folder)
	}
	if f["security"] != "wpa2-psk" {
		return ErrNotPSK
	}
	return nil
}

// GuardIn checks what only a new change must meet, as Guard does, where it
// takes the state to tell: a network has per-user keys or takes each
// device's passphrase from its RADIUS server, not both (0115). A key is
// refused on a network that does; and a change in the Services tree is
// refused if after it some key's network would. It is not Apply's to hold
// to: a log from before the rule may have both, and must still replay.
func GuardIn(s *State, op Op) error {
	if s == nil {
		return nil
	}
	switch op.Kind {
	case AddKey, SetKey:
		if f, ok := networkFields(s, op.Node, op.Network); ok && f["radius.passphrases"] == true {
			return ErrRADIUSKeys
		}
		return nil
	case RemoveKey:
		return nil
	}
	if op.Tree != Services || s.Keys == nil || s.Keys.Len() == 0 {
		return nil
	}
	next, _, err := Apply(s.Clone(), op)
	if err != nil {
		return nil // the change itself is refused, and says why
	}
	var by []string
	for _, k := range next.Keys.All() {
		if f, ok := networkFields(next, k.Folder, k.Network); ok && f["radius.passphrases"] == true {
			by = append(by, k.ID)
		}
	}
	if len(by) == 0 {
		return nil
	}
	if len(by) > 8 {
		by = append(by[:8], fmt.Sprintf("and %d more", len(by)-8))
	}
	return fmt.Errorf("the network's per-user keys are %w: %v. A network has keys, or takes each device's passphrase from its RADIUS server (radius.passphrases), not both", ErrInUse, by)
}

// offers says whether a network offers a VLAN to its keys.
func offers(s *State, folder hierarchy.NodeID, network string, vlan int) bool {
	f, _ := networkFields(s, folder, network)
	list, _ := f[KeyVLANsField].([]any)
	for _, v := range list {
		if n, ok := number(v); ok && n == vlan {
			return true
		}
	}
	return false
}

func number(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), n == float64(int(n))
	case int:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	}
	return 0, false
}

// checkKeys keeps every key's network able to take it after a change to the
// trees: still offered by its folder, still WPA2-PSK, and still offering the
// key's VLAN. A change that would break one is refused, naming the keys.
func checkKeys(s *State) error {
	if s.Keys == nil || s.Keys.Len() == 0 {
		return nil
	}
	var by []string
	for _, k := range s.Keys.All() {
		if keyNetwork(s, k.Folder, k.Network) != nil || (k.VLAN != 0 && !offers(s, k.Folder, k.Network, k.VLAN)) {
			by = append(by, k.ID)
		}
	}
	if len(by) == 0 {
		return nil
	}
	if len(by) > 8 {
		by = append(by[:8], fmt.Sprintf("and %d more", len(by)-8))
	}
	return &InUseError{What: "the network or VLAN", By: by}
}
