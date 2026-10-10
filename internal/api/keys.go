package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/keys"
)

// Per-user keys (0014, 0070). They are made and changed through POST
// /v1/changes, listed by network, and fetched by each AP's key agent, which
// holds its request open until its keys change.

// keyWaitMax is the longest an AP's request for its keys is held, inside the
// server's write timeout. When its keys change while it is held, the answer
// waits until they have been still for keySettle, up to keySettleMax, so a
// burst of changes, such as many keys pasted at once, reaches the AP as one.
const (
	keyWaitMax   = 50 * time.Second
	keySettle    = 2 * time.Second
	keySettleMax = 10 * time.Second
)

// prepareKey readies an add-key or set-key: a new key gets an ID if it has
// none, and a passphrase given in plain text is checked, found unlike the
// network's own and its other keys', and sealed (0027).
func (s *Server) prepareKey(state *change.State, op change.Op, now time.Time) (change.Op, error) {
	if op.Kind == change.RemoveKey {
		return op, nil
	}
	if op.Kind == change.AddKey && op.Key == "" {
		b := make([]byte, 5)
		rand.Read(b)
		op.Key = "k" + hex.EncodeToString(b)
	}
	var def struct {
		keys.Def
		Passphrase *string `json:"passphrase"`
	}
	if err := json.Unmarshal(op.Value, &def); err != nil {
		return op, badRequest("key: %v", err)
	}
	if err := def.Def.Check(now); err != nil {
		return op, badRequest("%v", err)
	}
	out := def.Def
	if def.Passphrase != nil {
		p := *def.Passphrase
		if err := keys.CheckPassphrase(p); err != nil {
			return op, badRequest("%v", err)
		}
		if same, err := s.passphraseTaken(state, op, p); err != nil {
			return op, err
		} else if same != "" {
			return op, badRequest("that passphrase is %s's: each key on a network needs its own", same)
		}
		sealed, err := s.box.Seal(keys.SealPath(op.Key), p)
		if err != nil {
			return op, err
		}
		if out.Passphrase, err = json.Marshal(sealed); err != nil {
			return op, err
		}
	}
	v, err := json.Marshal(out)
	op.Value = v
	return op, err
}

// passphraseTaken names what already uses a passphrase on the key's network:
// the network itself, or another of its keys.
func (s *Server) passphraseTaken(state *change.State, op change.Op, p string) (string, error) {
	path := "network." + op.Network + ".passphrase"
	if r, ok := state.Org.Services.Resolve(op.Node, hierarchy.Path(path)); ok {
		if v, err := s.box.Open(path, r.Value); err == nil && v == p {
			return "the network", nil
		}
	}
	if state.Keys == nil {
		return "", nil
	}
	for _, k := range state.Keys.Of(op.Node, op.Network) {
		if k.ID == op.Key {
			continue
		}
		var sealed any
		if json.Unmarshal(k.Passphrase, &sealed) != nil {
			continue
		}
		if v, err := s.box.Open(keys.SealPath(k.ID), sealed); err == nil && v == p {
			return "key " + k.Name, nil
		}
	}
	return "", nil
}

// keyList lists a network's keys, as a Services folder offers it: without
// their passphrases, and with whether each has expired. A viewer there may
// read them.
func (s *Server) keyList(w http.ResponseWriter, r *http.Request, c call) error {
	folder := hierarchy.NodeID(r.URL.Query().Get("folder"))
	network := r.URL.Query().Get("network")
	t := c.state.Org.Services
	if n, ok := t.Node(folder); !ok || n.Kind == hierarchy.KindAP || roleOn(c, change.Services, t, folder) < access.Viewer {
		return errNotFound
	}
	now := time.Now()
	out := []map[string]any{}
	if c.state.Keys != nil {
		for _, k := range c.state.Keys.Of(folder, network) {
			item := map[string]any{"id": k.ID, "name": k.Name, "vlan": k.VLAN, "macs": k.MACs, "expires": k.Expires,
				"expired": k.Expires != nil && !k.Expires.After(now)}
			if k.MACs == nil {
				item["macs"] = []string{}
			}
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["name"].(string) < out[j]["name"].(string) })
	writeJSON(w, http.StatusOK, map[string]any{"folder": folder, "network": network,
		"can_edit": roleOn(c, change.Services, t, folder) >= access.Operator, "keys": out})
	return nil
}

// apKeyRef is one key an AP is to have, before its PSK is worked out.
type apKeyRef struct {
	ID     string   `json:"id"`
	Sealed string   `json:"sealed"`
	VLAN   int      `json:"vlan,omitempty"`
	MACs   []string `json:"macs,omitempty"`
}

type apKeyNet struct {
	SSID string     `json:"ssid"`
	Keys []apKeyRef `json:"keys"`
}

// keyRefs works out the keys an AP is to have now, by network, and their
// version: a hash of them, sealed passphrases and all, so rotating one or an
// expiry passing changes it. A key reaches an AP whose network comes from
// the key's folder or one below it, while the network is WPA2-PSK and on.
func keyRefs(state *change.State, ap hierarchy.NodeID, now time.Time) (map[string]apKeyNet, string) {
	out := map[string]apKeyNet{}
	if state.Keys != nil && state.Keys.Len() > 0 {
		if cfg, err := state.Org.ResolveAP(ap); err == nil && !cfg.Unassigned {
			for id, net := range cfg.Networks {
				if net.Fields["security"].Value != "wpa2-psk" || net.Fields["enabled"].Value == false {
					continue
				}
				ssid, _ := net.Fields["ssid"].Value.(string)
				chain := map[hierarchy.NodeID]bool{}
				for _, f := range state.Org.Services.Ancestry(net.From) {
					chain[f] = true
				}
				var refs []apKeyRef
				for _, k := range state.Keys.All() {
					if k.Network != id || !chain[k.Folder] || (k.Expires != nil && !k.Expires.After(now)) {
						continue
					}
					var sealed map[string]any
					json.Unmarshal(k.Passphrase, &sealed)
					text, _ := sealed["$sealed"].(string)
					refs = append(refs, apKeyRef{ID: k.ID, Sealed: text, VLAN: k.VLAN, MACs: k.MACs})
				}
				if len(refs) > 0 {
					out[id] = apKeyNet{SSID: ssid, Keys: refs}
				}
			}
		}
	}
	b, _ := json.Marshal(out) // maps marshal sorted
	sum := sha256.Sum256(b)
	return out, hex.EncodeToString(sum[:16])
}

// pskCache keeps the PSKs worked out, so an AP's keys are not run through
// 4,096 rounds of PBKDF2 each at every request.
type pskCache struct {
	mu sync.Mutex
	m  map[string]string
}

func (c *pskCache) get(sealed, ssid string, open func() (string, error)) (string, error) {
	k := sealed + "\x00" + ssid
	c.mu.Lock()
	psk, ok := c.m[k]
	c.mu.Unlock()
	if ok {
		return psk, nil
	}
	p, err := open()
	if err != nil {
		return "", err
	}
	psk = keys.PSK(p, ssid)
	c.mu.Lock()
	if c.m == nil || len(c.m) > 20000 {
		c.m = map[string]string{}
	}
	c.m[k] = psk
	c.mu.Unlock()
	return psk, nil
}

// apKeys answers an AP's key agent (0070): its whole key set, by network, each
// key as the 64-hex PSK for its network's SSID, with the set's version. With
// the version it has in If-None-Match and ?wait=N, the request is held up to N
// seconds (at most 50) until the set changes; then 304 if it hasn't.
func (s *Server) apKeys(w http.ResponseWriter, r *http.Request, c apCall) error {
	wait := time.Duration(0)
	if v := r.URL.Query().Get("wait"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return badRequest("wait is a number of seconds")
		}
		wait = min(time.Duration(n)*time.Second, keyWaitMax)
	}
	have := strings.Trim(r.Header.Get("If-None-Match"), `"`)
	deadline := time.Now().Add(wait)
	held := false
	for {
		state := s.log.Snapshot()
		refs, version := keyRefs(state, c.ap, time.Now())
		if version != have {
			if held {
				version = s.settle(r, c.ap, version)
				refs, version = keyRefs(s.log.Snapshot(), c.ap, time.Now())
			}
			return s.sendKeys(w, refs, version)
		}
		if !time.Now().Before(deadline) {
			w.Header().Set("ETag", `"`+version+`"`)
			w.WriteHeader(http.StatusNotModified)
			return nil
		}
		select {
		case <-r.Context().Done():
			// The manager is stopping, or the AP gave up: unchanged, as the
			// protocol says, so an AP still listening asks again at once
			// rather than taking an empty answer for a failure.
			w.Header().Set("ETag", `"`+version+`"`)
			w.WriteHeader(http.StatusNotModified)
			return nil
		case <-time.After(time.Second):
		}
		held = true
	}
}

// settle waits until an AP's key set has been still for keySettle, up to
// keySettleMax, and returns its version then.
func (s *Server) settle(r *http.Request, ap hierarchy.NodeID, version string) string {
	until := time.Now().Add(keySettleMax)
	for time.Now().Before(until) {
		select {
		case <-r.Context().Done():
			return version
		case <-time.After(keySettle):
		}
		_, now := keyRefs(s.log.Snapshot(), ap, time.Now())
		if now == version {
			return version
		}
		version = now
	}
	return version
}

func (s *Server) sendKeys(w http.ResponseWriter, refs map[string]apKeyNet, version string) error {
	type sentKey struct {
		ID   string   `json:"id"`
		PSK  string   `json:"psk"`
		VLAN int      `json:"vlan,omitempty"`
		MACs []string `json:"macs,omitempty"`
	}
	type sentNet struct {
		SSID string    `json:"ssid"`
		Keys []sentKey `json:"keys"`
	}
	out := map[string]sentNet{}
	for id, n := range refs {
		sn := sentNet{SSID: n.SSID, Keys: []sentKey{}}
		for _, k := range n.Keys {
			psk, err := s.psks.get(k.Sealed, n.SSID, func() (string, error) {
				v, err := s.box.Open(keys.SealPath(k.ID), map[string]any{"$sealed": k.Sealed})
				if err != nil {
					return "", err
				}
				p, ok := v.(string)
				if !ok {
					return "", fmt.Errorf("key %s: its passphrase is not text", k.ID)
				}
				return p, nil
			})
			if err != nil {
				return err
			}
			sn.Keys = append(sn.Keys, sentKey{ID: k.ID, PSK: psk, VLAN: k.VLAN, MACs: k.MACs})
		}
		out[id] = sn
	}
	w.Header().Set("ETag", `"`+version+`"`)
	writeJSON(w, http.StatusOK, map[string]any{"version": version, "networks": out})
	return nil
}
