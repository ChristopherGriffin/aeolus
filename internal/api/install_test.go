package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ChristopherGriffin/aeolus/internal/bundle"
)

// fetch GETs a path with no token, as an AP that has none yet, by the Host
// given, or the fixture's own.
func (f *fixture) fetch(path, host string) (int, string) {
	f.t.Helper()
	req, err := http.NewRequest("GET", f.url+path, nil)
	must(f.t, err)
	if host != "" {
		req.Host = host
	}
	resp, err := http.DefaultClient.Do(req)
	must(f.t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	must(f.t, err)
	return resp.StatusCode, string(body)
}

// Installing from the manager (0083): the script names the manager it was
// fetched from, and nothing a Host header could put in a shell script; the
// certificate, the manifest and its files need no token; a file is handed
// out only by a SHA-256 the manifest names.
func TestInstallFromTheManager(t *testing.T) {
	f := newFixture(t)
	if code, _ := f.fetch("/install", ""); code != 404 {
		t.Fatalf("a manager without the installer: %d", code)
	}
	old, cur := f.agents()
	page := fstest.MapFS{
		"luci/www/luci-static/resources/view/aeolus/enroll.js": {Data: []byte("'use strict';\n")},
		"luci/usr/share/luci/menu.d/luci-app-aeolus.json":      {Data: []byte("{}\n")},
	}
	luci, files, err := bundle.FromFSChecked(page, "luci", cur.Version, bundle.LuCIPathOK)
	must(t, err)
	pem := "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n"
	f.api.WithInstall(Install{Script: []byte("#!/bin/sh\nMANAGER='" + ManagerPlaceholder + "'\n"), CertPEM: []byte(pem), LuCI: luci, Files: files})

	host := strings.TrimPrefix(f.url, "http://")
	code, body := f.fetch("/install", "")
	if code != 200 || body != "#!/bin/sh\nMANAGER='https://"+host+"'\n" {
		t.Fatalf("the script: %d %q", code, body)
	}
	code, body = f.fetch("/install", "aeolus.example.com:8443")
	if code != 200 || !strings.Contains(body, "MANAGER='https://aeolus.example.com:8443'") {
		t.Fatalf("the script, by name: %d %q", code, body)
	}
	for _, h := range []string{"x'$(reboot)'", "a;b", "[fd00::1]x"} {
		if code, body := f.fetch("/install", h); code != 400 || strings.Contains(body, "reboot") {
			t.Fatalf("Host %q: %d %q", h, code, body)
		}
	}
	if code, body := f.fetch("/install", "[fd00::60]:8443"); code != 200 || !strings.Contains(body, "https://[fd00::60]:8443'") {
		t.Fatalf("an IPv6 Host: %d %q", code, body)
	}

	if code, body := f.fetch("/install/manager.crt", ""); code != 200 || body != pem {
		t.Fatalf("the certificate: %d %q", code, body)
	}

	code, body = f.fetch("/install/manifest", "")
	var m struct {
		Version string        `json:"version"`
		Agent   bundle.Bundle `json:"agent"`
		LuCI    bundle.Bundle `json:"luci"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &m) != nil {
		t.Fatalf("the manifest: %d %s", code, body)
	}
	if m.Version != cur.Version || m.Agent.Hash != cur.Hash || len(m.LuCI.Files) != 2 {
		t.Fatalf("the manifest: %+v", m)
	}

	// The manager's own agent, and the page; not an older release's file,
	// nor one that isn't there.
	if code, body := f.fetch("/install/files/"+fileOf(cur, "/usr/sbin/aeolus-agent"), ""); code != 200 || body != "the new agent" {
		t.Fatalf("the agent: %d %q", code, body)
	}
	if code, body := f.fetch("/install/files/"+fileOf(luci, "/usr/share/luci/menu.d/luci-app-aeolus.json"), ""); code != 200 || body != "{}\n" {
		t.Fatalf("the menu: %d %q", code, body)
	}
	for _, sha := range []string{fileOf(old, "/usr/sbin/aeolus-agent"), strings.Repeat("0", 64), "x"} {
		if code, _ := f.fetch("/install/files/"+sha, ""); code != 404 {
			t.Fatalf("file %s: %d", sha, code)
		}
	}
}
