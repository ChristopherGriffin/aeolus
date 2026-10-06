package api

import (
	"io"
	"net/http"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/bundle"
)

// agents gives the fixture's server two releases' agents: an older one,
// and the manager's own, which differ in the agent itself.
func (f *fixture) agents() (old, cur bundle.Bundle) {
	f.t.Helper()
	store, err := bundle.Open(f.t.TempDir(), 10)
	must(f.t, err)
	t0 := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	put := func(text, release string, at time.Time) bundle.Bundle {
		fsys := fstest.MapFS{
			"files/usr/sbin/aeolus-agent":         {Data: []byte(text)},
			"files/usr/libexec/aeolus-rollback":   {Data: []byte("#!/bin/sh\n")},
			"files/usr/share/ucode/aeolus/rrm.uc": {Data: []byte("export const N = 3;\n")},
		}
		b, contents, err := bundle.FromFS(fsys, "files", release)
		must(f.t, err)
		b, err = store.Put(b, contents, at)
		must(f.t, err)
		return b
	}
	old = put("the old agent", "v1.0.0-aaaaaaa", t0)
	cur = put("the new agent", "v1.1.0-bbbbbbb", t0.Add(time.Hour))
	f.api.WithAgents(store, cur)
	return old, cur
}

// raw GETs a path as an AP and returns the status and body unparsed.
func (f *fixture) raw(path, token string) (int, string) {
	f.t.Helper()
	req, err := http.NewRequest("GET", f.url+path, nil)
	must(f.t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	must(f.t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	must(f.t, err)
	return resp.StatusCode, string(body)
}

func fileOf(b bundle.Bundle, path string) string {
	for _, f := range b.Files {
		if f.Path == path {
			return f.SHA256
		}
	}
	return ""
}

// Fleet updates (0079): every answer to a poll names the bundle the AP
// should run; it fetches the manifest and the files it lacks; a folder can
// pin an older release the manager keeps, never one it doesn't.
func TestFleetUpdates(t *testing.T) {
	f := newFixture(t)
	old, cur := f.agents()
	ap, token, version := f.adopted()

	// The poll names the manager's own release, on a 304 too.
	code, h, _ := f.apDo("GET", "/v1/ap/config", token, nil, nil)
	if code != 200 || h.Get(AgentHeader) != cur.Hash {
		t.Fatalf("poll: %d, agent %q, want %s", code, h.Get(AgentHeader), cur.Hash)
	}
	etag := h.Get("ETag")
	code, h, _ = f.apDo("GET", "/v1/ap/config", token, nil, map[string]string{"If-None-Match": etag})
	if code != 304 || h.Get(AgentHeader) != cur.Hash {
		t.Fatalf("unchanged poll: %d, agent %q", code, h.Get(AgentHeader))
	}

	// The manifest, and its files; not another release's.
	code, _, m := f.apDo("GET", "/v1/ap/agent", token, nil, nil)
	if code != 200 || m["hash"] != cur.Hash || m["version"] != cur.Version || len(m["files"].([]any)) != 3 {
		t.Fatalf("manifest: %d %v", code, m)
	}
	if code, body := f.raw("/v1/ap/agent/files/"+fileOf(cur, "/usr/sbin/aeolus-agent"), token); code != 200 || body != "the new agent" {
		t.Fatalf("file: %d %q", code, body)
	}
	if code, _ := f.raw("/v1/ap/agent/files/"+fileOf(old, "/usr/sbin/aeolus-agent"), token); code != 404 {
		t.Fatalf("another release's file: %d", code)
	}
	if code, _ := f.raw("/v1/ap/agent", "aeolusap1.not-a-token"); code != 401 {
		t.Fatalf("no token: %d", code)
	}

	// The releases kept, for the setting.
	code, vs := f.do("GET", "/v1/agent/versions", "griff", nil)
	if list, _ := vs["versions"].([]any); code != 200 || vs["current"] != cur.Version || len(list) != 2 {
		t.Fatalf("versions: %d %v", code, vs)
	}

	// A pin to a release the manager doesn't keep is refused; to one it
	// does, by its tag, the AP is told that one.
	pin := func(v string) (int, map[string]any) {
		return f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "system.agent", "value": v})
	}
	if code, b := pin("v9.9.9"); code != 400 {
		t.Fatalf("pin to a release not kept: %d %v", code, b)
	}
	if code, b := pin("v1.0.0"); code != 200 {
		t.Fatalf("pin: %d %v", code, b)
	}
	if _, h, _ := f.apDo("GET", "/v1/ap/config", token, nil, nil); h.Get(AgentHeader) != old.Hash {
		t.Fatalf("pinned poll: agent %q, want %s", h.Get(AgentHeader), old.Hash)
	}
	if code, body := f.raw("/v1/ap/agent/files/"+fileOf(old, "/usr/sbin/aeolus-agent"), token); code != 200 || body != "the old agent" {
		t.Fatalf("pinned file: %d %q", code, body)
	}

	// The AP says which it runs, and its last update; the fleet view shows
	// it beside the one it should run.
	report := map[string]any{"version": version, "uptime": 60, "agent": map[string]any{
		"version": old.Version, "hash": old.Hash,
		"update": map[string]any{"version": cur.Version, "hash": cur.Hash, "state": "rolled-back", "why": "it did not confirm itself within 180 s", "ago": 30},
	}}
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, report, nil); code != 200 {
		t.Fatalf("report: %d %v", code, b)
	}
	for name, bad := range map[string]map[string]any{
		"hash":  {"version": "v1", "hash": "not-hex"},
		"state": {"version": "v1", "hash": old.Hash, "update": map[string]any{"version": "v2", "hash": cur.Hash, "state": "exploded", "ago": 1}},
	} {
		if code, _, _ := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "agent": bad}, nil); code != 400 {
			t.Errorf("bad agent %s: %d", name, code)
		}
	}
	_, fleet := f.do("GET", "/v1/aps", "griff", nil)
	var view map[string]any
	for _, a := range fleet["aps"].([]any) {
		if a.(map[string]any)["id"] == ap {
			view = a.(map[string]any)["agent"].(map[string]any)
		}
	}
	runs, _ := view["runs"].(map[string]any)
	wants, _ := view["wants"].(map[string]any)
	if runs["hash"] != old.Hash || wants["hash"] != old.Hash || view["pin"] != "v1.0.0" {
		t.Fatalf("fleet view: %v", view)
	}
	if u, _ := runs["update"].(map[string]any); u["state"] != "rolled-back" {
		t.Fatalf("fleet view's update: %v", runs)
	}
}
