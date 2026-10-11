package api

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
)

// Captures (0135): of each attempt to come online that fails, its AP keeps
// the client's frames, as its radio heard and sent them and as they crossed
// its wire side, and sends them after the attempt's record. The manager
// keeps a few an AP, and gives one out as a file Wireshark opens.

const (
	maxCapture    = 1 << 20 // bytes of a capture's file: an agent's holds half that of frames, and their headers
	captureFrames = 1024    // frames in one, at the most
)

var captureIDRE = regexp.MustCompile(`^[0-9]{1,20}-[0-9]{1,9}$`)

// pcapng's blocks, as an agent writes them: a section's header, with the
// mark that says its numbers are little-endian; an interface; a frame.
const (
	pcapSection   = 0x0a0d0d0a
	pcapOrder     = 0x1a2b3c4d
	pcapInterface = 1
	pcapFrame     = 6
)

// pcapFrames holds a file to what an agent writes, and counts its frames:
// one section, little-endian, of blocks each as long as it says at both
// ends, the first the section's header and nothing after it but interfaces
// and frames.
func pcapFrames(b []byte) (int, error) {
	frames := 0
	for at := 0; at < len(b); {
		if len(b)-at < 12 {
			return 0, errors.New("it ends within a block")
		}
		kind, n := binary.LittleEndian.Uint32(b[at:]), int(binary.LittleEndian.Uint32(b[at+4:]))
		if n < 12 || n%4 != 0 || n > len(b)-at || int(binary.LittleEndian.Uint32(b[at+n-4:])) != n {
			return 0, errors.New("a block is not as long as it says")
		}
		switch {
		case at == 0:
			if kind != pcapSection || n < 28 || binary.LittleEndian.Uint32(b[8:]) != pcapOrder {
				return 0, errors.New("it does not begin as a pcapng file does")
			}
		case kind == pcapFrame:
			frames++
		case kind != pcapInterface:
			return 0, errors.New("it holds a block that is neither an interface nor a frame")
		}
		at += n
	}
	if len(b) == 0 {
		return 0, errors.New("it is empty")
	}
	return frames, nil
}

// capture takes the capture an AP kept of an attempt that failed (0135):
// POST /v1/ap/captures {id, mac, bss, started, frames, radio, wire, file},
// its AP's name for it, the client, the interface it was on, the attempt's
// start as its record gave it, how many frames it holds, from the radio and
// from the wire side, and the file itself, pcapng in base64, at most 1 MB.
// It is of an attempt that failed and was reported before it: any other is
// refused. One sent before is not kept twice. An AP's latest ten are kept,
// three of one client at the most.
func (s *Server) capture(w http.ResponseWriter, r *http.Request, c apCall) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxCapture/3*4+4096)
	var req struct {
		ID      string `json:"id"`
		MAC     string `json:"mac"`
		BSS     string `json:"bss"`
		Started int64  `json:"started"`
		Frames  int    `json:"frames"`
		Radio   int    `json:"radio"`
		Wire    int    `json:"wire"`
		File    string `json:"file"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		var big *http.MaxBytesError
		if errors.As(err, &big) {
			return &apiError{http.StatusRequestEntityTooLarge, "a capture: its file is at most 1 MB"}
		}
		return badRequest("request body: %v", err)
	}
	if err := unadopted(s.log.Snapshot(), c.ap); err != nil {
		return err
	}
	bad := func(what string) error { return badRequest("a capture: %s", what) }
	if !captureIDRE.MatchString(req.ID) || !macRE.MatchString(req.MAC) || req.BSS == "" || !shortText(req.BSS, 32) || req.Started <= 0 {
		return bad("it has its AP's name for it, its client by MAC in lower case, the interface it was on, and its attempt's start")
	}
	if req.Frames < 1 || req.Frames > captureFrames || req.Radio < 0 || req.Wire < 0 || req.Radio+req.Wire != req.Frames {
		return bad("it has from 1 to 1024 frames, each from the radio or from the wire side")
	}
	file, err := base64.StdEncoding.DecodeString(req.File)
	if err != nil {
		return bad("its file is in base64")
	}
	if len(file) > maxCapture {
		return &apiError{http.StatusRequestEntityTooLarge, "a capture: its file is at most 1 MB"}
	}
	frames, err := pcapFrames(file)
	if err != nil {
		return bad("its file is not a capture: " + err.Error())
	}
	if frames != req.Frames {
		return bad(fmt.Sprintf("its file holds %d frames, and it says %d", frames, req.Frames))
	}
	kept, err := s.conds.RecordCapture(conditions.Capture{
		AP: c.ap, MAC: req.MAC, Said: req.Started, Frames: req.Frames, Radio: req.Radio, Wire: req.Wire, File: file,
	})
	if errors.Is(err, conditions.ErrNoAttempt) {
		return bad("it is of no attempt this AP has reported as failed")
	}
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"kept": kept})
	return nil
}

// captureSaid is what a list of attempts says of one's capture: how many
// frames it holds, how large its file is, and whether the caller may read
// it.
type captureSaid struct {
	Frames int  `json:"frames"`
	Bytes  int  `json:"bytes"`
	May    bool `json:"may"`
}

// fileName is a name for a file with only what any system takes in one.
func fileName(s string) string {
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, s)
	return strings.Trim(s, "-.")
}

// connectionCapture is the capture kept of one attempt (0135), as a file
// Wireshark opens: GET /v1/connections/{id}/capture, id being the
// attempt's, as a client's attempts give it. It needs operator on the
// attempt's AP: a capture holds the client's key handshake, from which a
// passphrase can be guessed at off the network. 404 where no capture is
// kept of it: it did not fail, nothing was heard of it, its AP keeps none,
// or newer ones took its place.
func (s *Server) connectionCapture(w http.ResponseWriter, r *http.Request, c call) error {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return errNotFound
	}
	cn, k, ok, err := s.conds.CaptureOf(id)
	if err != nil {
		return err
	}
	t := c.state.Org.Locations
	if !ok || roleOn(c, change.Locations, t, cn.AP) < access.Viewer {
		return errNotFound
	}
	if roleOn(c, change.Locations, t, cn.AP) < access.Operator {
		return &apiError{http.StatusForbidden, "a capture holds a client's key handshake: reading one needs operator on its AP"}
	}
	if k == nil {
		return &apiError{http.StatusNotFound, "no capture is kept of this attempt"}
	}
	name := string(cn.AP)
	if n, ok := t.Node(cn.AP); ok && fileName(n.Name) != "" {
		name = fileName(n.Name)
	}
	name = fmt.Sprintf("aeolus-%s-%s-%s.pcapng", name, strings.ReplaceAll(cn.MAC, ":", ""), cn.Started.UTC().Format("20060102-150405Z"))
	slog.Info("capture read", "ap", cn.AP, "client", cn.MAC, "attempt", cn.ID, "by", c.actor)
	w.Header().Set("Content-Type", "application/x-pcapng")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(k.File)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, err = w.Write(k.File)
	if err != nil {
		slog.Warn("capture read: writing it", "err", err)
	}
	return nil
}
