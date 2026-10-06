package api

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// The routes APs use (0033, 0038). An AP authenticates with its own token,
// which works only here; account tokens do not work here.

// facts is what an AP sends to enroll, and, with the address it came from,
// what the manager records for the person deciding whether to adopt it.
type facts struct {
	MAC      string          `json:"mac"`
	MACs     []string        `json:"macs,omitempty"`
	Hostname string          `json:"hostname,omitempty"`
	Model    string          `json:"model,omitempty"`
	Board    string          `json:"board,omitempty"`
	OpenWrt  string          `json:"openwrt,omitempty"`
	Radios   json.RawMessage `json:"radios,omitempty"`
	Source   string          `json:"source,omitempty"`
}

const (
	maxEnrollBody = 64 << 10
	maxMACs       = 32
	maxText       = 128
)

// apID derives an AP's ID from its MAC (0038): "ap-" and the 12 hex digits.
func apID(mac string) hierarchy.NodeID {
	return hierarchy.NodeID("ap-" + strings.ReplaceAll(mac, ":", ""))
}

// normalMAC reads a unicast EUI-48 address and writes it as aa:bb:cc:dd:ee:ff.
func normalMAC(s string) (string, error) {
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return "", badRequest("%q is not a MAC address", s)
	}
	if hw[0]&1 == 1 || string(hw) == "\x00\x00\x00\x00\x00\x00" {
		return "", badRequest("%s is not an address a device can have", hw)
	}
	return hw.String(), nil
}

// plainText accepts a short single-line string.
func plainText(field, s string, max int) error {
	if len(s) > max {
		return badRequest("%s is longer than %d characters", field, max)
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return badRequest("%s holds a character that is not printable", field)
		}
	}
	return nil
}

func (f *facts) check() error {
	var err error
	if f.MAC == "" {
		return badRequest("mac is required: the MAC address the AP is known by")
	}
	if f.MAC, err = normalMAC(f.MAC); err != nil {
		return err
	}
	if len(f.MACs) > maxMACs {
		return badRequest("at most %d MAC addresses", maxMACs)
	}
	for i, m := range f.MACs {
		if f.MACs[i], err = normalMAC(m); err != nil {
			return err
		}
	}
	for _, t := range []struct {
		field, value string
		max          int
	}{{"hostname", f.Hostname, 63}, {"model", f.Model, maxText}, {"board", f.Board, maxText}, {"openwrt", f.OpenWrt, maxText}} {
		if err := plainText(t.field, t.value, t.max); err != nil {
			return err
		}
	}
	if len(f.Radios) > 0 {
		var radios []json.RawMessage
		if json.Unmarshal(f.Radios, &radios) != nil {
			return badRequest("radios must be a list")
		}
	}
	return nil
}

// enroll records a new AP in Landing Zone and hands it its token, once
// (0033). It needs no token. The manager makes the change in its own name
// (0036), with the address the request came from as the reason.
func (s *Server) enroll(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxEnrollBody)
	var f facts
	err := readJSON(r, &f)
	if err == nil {
		err = f.check()
	}
	if err != nil {
		s.fail(w, r, "", err)
		return
	}
	f.Source, _, _ = net.SplitHostPort(r.RemoteAddr)
	id := apID(f.MAC)
	name := f.Hostname
	if name == "" {
		name = string(id)
	}
	raw, err := json.Marshal(f)
	if err != nil {
		s.fail(w, r, "", err)
		return
	}
	plain, tokenID, hash, err := access.NewAPToken()
	if err != nil {
		s.fail(w, r, "", err)
		return
	}
	op := change.Op{Kind: change.Enroll, Node: id, Name: name, TokenID: tokenID, TokenHash: hash, Value: raw}
	if _, err := s.log.Commit(change.SystemActor, "enrolled from "+f.Source, op); err != nil {
		switch {
		case errors.Is(err, hierarchy.ErrExists):
			err = &apiError{http.StatusConflict, fmt.Sprintf("%s is already enrolled; a person must remove it before it can enroll again", id)}
		case errors.Is(err, hierarchy.ErrNotFound):
			err = &apiError{http.StatusServiceUnavailable, "this manager has no Landing Zone yet"}
		}
		s.fail(w, r, "", err)
		return
	}
	slog.Info("enrolled", "ap", id, "name", name, "source", f.Source)
	writeJSON(w, http.StatusCreated, map[string]any{"ap": id, "name": name, "token": plain, "config": "/v1/ap/config"})
}

// apCall is one request from an authenticated AP.
type apCall struct {
	ap hierarchy.NodeID
}

type apHandler func(w http.ResponseWriter, r *http.Request, c apCall) error

func (s *Server) apAuth(h apHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		state := s.log.Snapshot()
		if !ok || state == nil {
			writeError(w, http.StatusUnauthorized, "missing or invalid AP token")
			return
		}
		ap, err := state.Access.AuthenticateAP(strings.TrimSpace(token))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "missing or invalid AP token")
			return
		}
		// Every AP request counts as being seen (0039). Failing to record it
		// does not stop the AP.
		source, _, _ := net.SplitHostPort(r.RemoteAddr)
		if err := s.conds.Seen(ap, source); err != nil {
			slog.Error("recording last seen", "ap", ap, "err", err)
		}
		if err := h(w, r, apCall{ap: ap}); err != nil {
			s.fail(w, r, string(ap), err)
		}
	})
}

// apPoll answers an AP's poll (0007, 0029, 0032). The AP sends the version it
// runs in If-None-Match:
//   - 304: that is still its version.
//   - "unassigned": it is in Landing Zone and gets nothing.
//   - "held": its config breaks rules; it keeps running what it has.
//   - "ready": its config, with secrets opened (0027), and the version as the
//     ETag. Only a ready config carries an ETag, so an AP only ever holds the
//     version of a config it was sent.
func (s *Server) apPoll(w http.ResponseWriter, r *http.Request, c apCall) error {
	w.Header().Set("Cache-Control", "no-store")
	// The agent it should run (0079), on every answer: an unchanged, held
	// or unassigned config still carries it.
	s.setAgentHeader(w, c.ap)
	// The version is read before the state, so the state is never older than
	// the version it is labeled with. If a change lands in between, the AP
	// gets the newer config under the older version and fetches it once more.
	version, _ := s.log.Version(c.ap)
	etag := `"` + strconv.FormatInt(version, 10) + `"`
	if matches(r.Header.Get("If-None-Match"), etag) {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return nil
	}
	res, err := s.compose(s.log.Snapshot(), c.ap, s.reveal)
	if err != nil {
		return err
	}
	out := map[string]any{"ap": c.ap, "version": version}
	switch {
	case res.Unassigned:
		out["state"] = "unassigned"
	case len(res.Problems) > 0:
		out["state"], out["problems"] = "held", res.Problems
	default:
		out["state"], out["config"] = "ready", res.Doc
		// The key the APs sign their hellos to each other with, while radio
		// resource management is on (0073). It is the same for every AP, and
		// comes beside the config, so it versions nothing.
		if rrmOn(res.Doc) {
			out["rrm"] = map[string]any{"key": hex.EncodeToString(s.box.Derive("rrm"))}
		}
		w.Header().Set("ETag", etag)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// rrmOn says whether a config turns radio resource management on (0073).
func rrmOn(doc map[string]any) bool {
	r, _ := doc["rrm"].(map[string]any)
	return r["enabled"] == true
}

// matches reports whether an If-None-Match header names the ETag.
func matches(header, etag string) bool {
	for _, t := range strings.Split(header, ",") {
		if strings.TrimPrefix(strings.TrimSpace(t), "W/") == etag {
			return true
		}
	}
	return false
}
