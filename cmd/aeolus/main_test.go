package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/changelog"
)

type paths struct{ dir, db, key, mcp, cert, tlsKey string }

func newPaths(t *testing.T) paths {
	d := t.TempDir()
	return paths{d, filepath.Join(d, "aeolus.db"), filepath.Join(d, "secret.key"), filepath.Join(d, "mcp.token"),
		filepath.Join(d, "tls.crt"), filepath.Join(d, "tls.key")}
}

func initArgs(p paths) []string {
	return []string{"-db", p.db, "-key", p.key, "-org", "symtus", "-org-name", "Symtus",
		"-admin", "griff", "-mcp-account", "claude", "-mcp-role", "operator", "-mcp-token", p.mcp}
}

var tokenRE = regexp.MustCompile(`aeolus1\.[0-9a-f]{16}\.[A-Za-z0-9_-]{43}`)

func runInitOK(t *testing.T, p paths) string {
	t.Helper()
	var out, errOut bytes.Buffer
	if err := run(append([]string{"init"}, initArgs(p)...), &out, &errOut); err != nil {
		t.Fatalf("init: %v\n%s", err, errOut.String())
	}
	token := tokenRE.FindString(out.String())
	if token == "" {
		t.Fatalf("no admin token in output:\n%s", out.String())
	}
	return token
}

func TestInit(t *testing.T) {
	p := newPaths(t)
	adminToken := runInitOK(t, p)

	mcpRaw, err := os.ReadFile(p.mcp)
	must(t, err)
	mcpToken := strings.TrimSpace(string(mcpRaw))
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(p.mcp)
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("MCP token file mode %v", info.Mode().Perm())
		}
	}

	log, err := changelog.Open(p.db, changelog.Options{})
	must(t, err)
	defer log.Close()
	state := log.Snapshot()
	if who, err := state.Access.Authenticate(adminToken); err != nil || who != "griff" {
		t.Fatalf("admin token: %q, %v", who, err)
	}
	if who, err := state.Access.Authenticate(mcpToken); err != nil || who != "claude" {
		t.Fatalf("MCP token: %q, %v", who, err)
	}
	if r := state.Access.RoleAt("claude", "services", state.Org.Services.Ancestry("symtus")); r != access.Operator {
		t.Fatalf("claude's role = %v", r)
	}
	entries, err := log.Entries(0, 0)
	must(t, err)
	for _, e := range entries {
		if e.Actor != "griff" {
			t.Fatalf("init logged a change under %q", e.Actor)
		}
	}
	must(t, log.Close())

	var out, errOut bytes.Buffer
	if err := run(append([]string{"init"}, initArgs(p)...), &out, &errOut); err == nil {
		t.Fatal("a second init succeeded")
	}
}

func TestInitNeverPrintsTheMCPToken(t *testing.T) {
	p := newPaths(t)
	var out bytes.Buffer
	must(t, run(append([]string{"init"}, initArgs(p)...), &out, &bytes.Buffer{}))
	mcpRaw, err := os.ReadFile(p.mcp)
	must(t, err)
	if strings.Contains(out.String(), strings.TrimSpace(string(mcpRaw))) {
		t.Fatal("the MCP adapter's token was printed")
	}
	if n := len(tokenRE.FindAllString(out.String(), -1)); n != 1 {
		t.Fatalf("%d tokens printed, want 1 (the admin's)", n)
	}
}

func TestEnsureCert(t *testing.T) {
	p := newPaths(t)
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	must(t, ensureCert(p.cert, p.tlsKey, []string{"aeolus.symtus.com", "192.168.20.60"}, now))
	pemBytes, err := os.ReadFile(p.cert)
	must(t, err)
	block, _ := pem.Decode(pemBytes)
	cert, err := x509.ParseCertificate(block.Bytes)
	must(t, err)
	if err := cert.VerifyHostname("aeolus.symtus.com"); err != nil {
		t.Fatal(err)
	}
	if err := cert.VerifyHostname("192.168.20.60"); err != nil {
		t.Fatal(err)
	}
	must(t, ensureCert(p.cert, p.tlsKey, nil, now))
	again, _ := os.ReadFile(p.cert)
	if !bytes.Equal(again, pemBytes) {
		t.Fatal("ensureCert replaced an existing certificate")
	}
	must(t, os.Remove(p.tlsKey))
	if err := ensureCert(p.cert, p.tlsKey, nil, now); err == nil {
		t.Fatal("ensureCert accepted a certificate without its key")
	}
}

func TestServeRefusesWithoutInitOrKey(t *testing.T) {
	p := newPaths(t)
	if _, _, err := newServer([]string{"-db", p.db, "-key", p.key, "-cert", p.cert, "-tls-key", p.tlsKey}, &bytes.Buffer{}); err == nil {
		t.Fatal("serve started without a secret key")
	}
}

func TestServeOverTLS(t *testing.T) {
	p := newPaths(t)
	adminToken := runInitOK(t, p)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	srv, closeLog, err := newServer([]string{"-db", p.db, "-key", p.key, "-cert", p.cert, "-tls-key", p.tlsKey, "-listen", ln.Addr().String()}, &bytes.Buffer{})
	must(t, err)
	defer closeLog()
	go srv.ServeTLS(ln, "", "")
	defer srv.Close()

	pool := x509.NewCertPool()
	pemBytes, err := os.ReadFile(p.cert)
	must(t, err)
	pool.AppendCertsFromPEM(pemBytes)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}, Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", "https://"+ln.Addr().String()+"/v1/whoami", nil)
	must(t, err)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := client.Do(req)
	must(t, err)
	defer resp.Body.Close()
	var body map[string]any
	must(t, json.NewDecoder(resp.Body).Decode(&body))
	if resp.StatusCode != 200 || body["account"] != "griff" {
		t.Fatalf("whoami over TLS = %d %v", resp.StatusCode, body)
	}
}

func TestServeAddsBuiltInFoldersOnce(t *testing.T) {
	p := newPaths(t)
	runInitOK(t, p)
	args := []string{"-db", p.db, "-key", p.key, "-cert", p.cert, "-tls-key", p.tlsKey}
	for i := 0; i < 2; i++ {
		_, closeLog, err := newServer(args, &bytes.Buffer{})
		must(t, err)
		must(t, closeLog())
	}
	log, err := changelog.Open(p.db, changelog.Options{})
	must(t, err)
	defer log.Close()
	entries, err := log.Entries(0, 0)
	must(t, err)
	var builtins []changelog.Entry
	for _, e := range entries {
		if e.Op.Kind == "add-builtins" {
			builtins = append(builtins, e)
		}
	}
	if len(builtins) != 1 || builtins[0].Actor != "aeolus" {
		t.Fatalf("add-builtins entries = %+v", builtins)
	}
	lz, ok := log.Snapshot().Org.Locations.Node("landing-zone")
	if !ok || !lz.Isolated {
		t.Fatalf("Landing Zone = %+v, %v", lz, ok)
	}
}

func TestServeKeepsConditionsBesideTheLog(t *testing.T) {
	p := newPaths(t)
	runInitOK(t, p)
	args := []string{"-db", p.db, "-key", p.key, "-cert", p.cert, "-tls-key", p.tlsKey}
	_, closeAll, err := newServer(args, &bytes.Buffer{})
	must(t, err)
	must(t, closeAll())
	if _, err := os.Stat(filepath.Join(filepath.Dir(p.db), "conditions.db")); err != nil {
		t.Fatalf("conditions database: %v", err)
	}
	if _, _, err := newServer(append(args, "-keep-state-days", "0"), &bytes.Buffer{}); err == nil {
		t.Fatal("serve accepted keeping state reports for 0 days")
	}
	t.Setenv("AEOLUS_KEEP_STATE_DAYS", "90")
	if n := defaultKeepDays(); n != 90 {
		t.Fatalf("keep days from the environment = %d", n)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
