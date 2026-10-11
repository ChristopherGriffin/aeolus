package api

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// pcapOf is a capture as an agent writes one: a section, its two
// interfaces, and so many frames of so many bytes each.
func pcapOf(frames, size int) []byte {
	var b bytes.Buffer
	block := func(kind uint32, body []byte) {
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
		n := uint32(12 + len(body))
		binary.Write(&b, binary.LittleEndian, kind)
		binary.Write(&b, binary.LittleEndian, n)
		b.Write(body)
		binary.Write(&b, binary.LittleEndian, n)
	}
	le := func(v ...any) []byte {
		var x bytes.Buffer
		for _, one := range v {
			binary.Write(&x, binary.LittleEndian, one)
		}
		return x.Bytes()
	}
	block(pcapSection, le(uint32(pcapOrder), uint16(1), uint16(0), int64(-1)))
	block(pcapInterface, le(uint16(127), uint16(0), uint32(2048)))
	block(pcapInterface, le(uint16(1), uint16(0), uint32(2048)))
	for i := 0; i < frames; i++ {
		block(pcapFrame, append(le(uint32(0), uint32(0), uint32(i), uint32(size), uint32(size)), make([]byte, size)...))
	}
	return b.Bytes()
}

// file reads a path as an account, as it comes, with its headers.
func (f *fixture) file(path, as string) (int, http.Header, []byte) {
	f.t.Helper()
	req, err := http.NewRequest("GET", f.url+path, nil)
	must(f.t, err)
	req.Header.Set("Authorization", "Bearer "+f.tokens[as])
	resp, err := http.DefaultClient.Do(req)
	must(f.t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	must(f.t, err)
	return resp.StatusCode, resp.Header, body
}

// The capture of an attempt that failed (0135): its AP sends it after the
// attempt, the manager keeps a few an AP, the list of a client's attempts
// says which have one, and one is read as a file by those with operator on
// its AP.
func TestCapturesOfFailedAttempts(t *testing.T) {
	f := newFixture(t)
	_, token, _ := f.adopted()
	now := time.Now().UnixMilli()
	attempt := func(mac string, started int64, outcome string) map[string]any {
		stage, reason := "key", "wrong passphrase"
		if outcome != "failed" {
			stage, reason = "dns", ""
		}
		return map[string]any{
			"mac": mac, "bss": "phy0-ap0", "network": "sweet", "ssid": "Sweet Spot", "band": "5g",
			"started": started, "took_ms": 4200, "outcome": outcome, "stage": stage, "reason": reason,
			"events": []any{map[string]any{"t": 0, "stage": "auth", "what": "authenticated", "ok": true}},
		}
	}
	report := func(list ...any) {
		t.Helper()
		if code, _, body := f.apDo("POST", "/v1/ap/connections", token, map[string]any{"connections": list}, nil); code != 200 || body["refused"] != float64(0) {
			t.Fatalf("reporting attempts: %d %v", code, body)
		}
	}
	send := func(mac string, started int64, frames int, file []byte) (int, map[string]any) {
		code, _, body := f.apDo("POST", "/v1/ap/captures", token, map[string]any{
			"id": "1791745000000-1", "mac": mac, "bss": "phy0-ap0", "started": started,
			"frames": frames, "radio": frames - 1, "wire": 1, "file": base64.StdEncoding.EncodeToString(file),
		}, nil)
		return code, body
	}
	const mac = "72:bb:66:12:2a:94"
	failed := attempt(mac, now-30000, "failed")
	// Its record says there is one, by the AP's own name for it.
	failed["capture"] = map[string]any{"id": "1791745000000-1", "frames": 9}
	report(attempt(mac, now-60000, "online"), failed)

	file := pcapOf(9, 180)
	if code, body := send(mac, now-30000, 9, file); code != 200 || body["kept"] != true {
		t.Fatalf("a capture of the failed attempt: %d %v", code, body)
	}
	// Sent again, as where the answer to the first was lost, it is not kept twice.
	if code, body := send(mac, now-30000, 9, file); code != 200 || body["kept"] != false {
		t.Fatalf("sent again: %d %v", code, body)
	}
	// One of an attempt that did not fail, or of none reported, is refused.
	for what, started := range map[string]int64{"an attempt that came online": now - 60000, "no attempt": now - 1} {
		if code, body := send(mac, started, 9, file); code != 400 || !strings.Contains(fmt.Sprint(body["error"]), "no attempt") {
			t.Errorf("a capture of %s: %d %v", what, code, body)
		}
	}
	// So is a file that is no capture, one that holds other than it says,
	// and one too large.
	for what, c := range map[string]struct {
		frames int
		file   []byte
		code   int
	}{
		"a file cut short":         {9, file[:len(file)-3], 400},
		"a file of something else": {9, []byte("GIF89a, and so on, for a while"), 400},
		"more frames than it says": {8, file, 400},
		"an empty file":            {9, nil, 400},
		"a file past 1 MB":         {600, pcapOf(600, 1800), 413},
	} {
		if code, body := send(mac, now-30000, c.frames, c.file); code != c.code {
			t.Errorf("%s: %d %v, want %d", what, code, body, c.code)
		}
	}

	// The client's attempts say which has a capture, and that griff, the
	// org's admin, may read it.
	said := func(as string) []string {
		t.Helper()
		code, got := f.do("GET", "/v1/clients/"+mac+"/connections", as, nil)
		if code != 200 {
			t.Fatalf("%s reading the attempts: %d %v", as, code, got)
		}
		var out []string
		for _, c := range got["connections"].([]any) {
			c := c.(map[string]any)
			k, _ := c["capture"].(map[string]any)
			out = append(out, fmt.Sprintf("%v: %v frames, %v bytes, may %v", c["outcome"], k["frames"], k["bytes"], k["may"]))
		}
		return out
	}
	if got, want := strings.Join(said("griff"), "; "), fmt.Sprintf("failed: 9 frames, %d bytes, may true; online: <nil> frames, <nil> bytes, may <nil>", len(file)); got != want {
		t.Errorf("the attempts = %q, want %q", got, want)
	}
	_, got := f.do("GET", "/v1/clients/"+mac+"/connections", "griff", nil)
	id := int64(got["connections"].([]any)[0].(map[string]any)["id"].(float64))
	other := int64(got["connections"].([]any)[1].(map[string]any)["id"].(float64))
	code, head, body := f.file(fmt.Sprintf("/v1/connections/%d/capture", id), "griff")
	if code != 200 || !bytes.Equal(body, file) || head.Get("Content-Type") != "application/x-pcapng" ||
		!strings.HasPrefix(head.Get("Content-Disposition"), `attachment; filename="aeolus-PumphouseAP-72bb66122a94-`) || !strings.HasSuffix(head.Get("Content-Disposition"), `.pcapng"`) {
		t.Errorf("the capture read: %d, %d bytes, %v", code, len(body), head)
	}
	// An attempt with none, and no attempt at all.
	for what, path := range map[string]string{"an attempt with no capture": fmt.Sprintf("/v1/connections/%d/capture", other), "no attempt": "/v1/connections/999999/capture", "no number": "/v1/connections/x/capture"} {
		if code, _, body := f.file(path, "griff"); code != 404 {
			t.Errorf("%s: %d %s", what, code, body)
		}
	}
	// One who may only view the AP sees that there is one, and may not
	// read it: it holds the client's key handshake. One with no role on
	// the AP is told nothing.
	if code, b := f.change("griff", map[string]any{"kind": "grant", "account": "tenant", "tree": "locations", "node": "office", "role": "viewer"}); code != 200 {
		t.Fatalf("grant: %d %v", code, b)
	}
	if got, want := said("tenant")[0], fmt.Sprintf("failed: 9 frames, %d bytes, may false", len(file)); got != want {
		t.Errorf("as a viewer the attempt = %q, want %q", got, want)
	}
	if code, _, body := f.file(fmt.Sprintf("/v1/connections/%d/capture", id), "tenant"); code != 403 || !strings.Contains(string(body), "operator") {
		t.Errorf("a viewer reading it: %d %s", code, body)
	}
	if code, _, body := f.file(fmt.Sprintf("/v1/connections/%d/capture", id), "office"); code != 404 {
		t.Errorf("one with no role on the AP reading it: %d %s", code, body)
	}

	// Few are kept. Of one client that fails over and over, the latest
	// three: the rest of an AP's ten are left for the others.
	count := func() int {
		n := 0
		for _, s := range said("griff") {
			if strings.Contains(s, "may true") {
				n++
			}
		}
		return n
	}
	for i := int64(1); i <= 5; i++ {
		report(attempt(mac, now-20000+i*1000, "failed"))
		if code, body := send(mac, now-20000+i*1000, 2, pcapOf(2, 60)); code != 200 || body["kept"] != true {
			t.Fatalf("capture %d of the one client: %d %v", i, code, body)
		}
	}
	if got := count(); got != 3 {
		t.Errorf("captures kept of a client that fails over and over = %d, want 3", got)
	}
	if got := said("griff"); !strings.Contains(got[0], "2 frames") || !strings.Contains(got[2], "2 frames") || strings.Contains(got[3], "may true") {
		t.Errorf("they are the latest three: %q", got)
	}
	// And of an AP's, the latest ten, whoever they are of.
	kept := 0
	for i := 0; i < 12; i++ {
		other := fmt.Sprintf("72:bb:66:12:2b:%02x", i)
		report(attempt(other, now-5000+int64(i), "failed"))
		if code, body := send(other, now-5000+int64(i), 2, pcapOf(2, 60)); code != 200 || body["kept"] != true {
			t.Fatalf("a capture of client %d: %d %v", i, code, body)
		}
	}
	for i := 0; i < 12; i++ {
		_, got := f.do("GET", "/v1/clients/"+fmt.Sprintf("72:bb:66:12:2b:%02x", i)+"/connections", "griff", nil)
		if got["connections"].([]any)[0].(map[string]any)["capture"] != nil {
			kept++
		} else if i >= 2 {
			t.Errorf("client %d's capture went, and it is among the latest ten", i)
		}
	}
	if got := kept + count(); got != 10 {
		t.Errorf("captures kept of the AP = %d, want 10", got)
	}
}
