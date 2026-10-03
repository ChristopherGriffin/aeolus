package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/changelog"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/schema"
	"github.com/ChristopherGriffin/aeolus/internal/secret"
)

const passphrase = "sweet-spot-passphrase-42"

type fixture struct {
	t      *testing.T
	url    string
	dir    string
	log    *changelog.Log
	conds  *conditions.Store
	tokens map[string]string // account -> plain token
}

// newFixture starts an API over a fresh log:
//   - griff:  admin at both roots (create-org)
//   - claude: operator at both roots
//   - office: operator on Services › Household, viewer at the Services root
//   - tenant: viewer on Services › Household only
func newFixture(t *testing.T) *fixture {
	t.Helper()
	sch, err := schema.V1()
	must(t, err)
	dir := t.TempDir()
	box, err := secret.LoadOrCreate(filepath.Join(dir, "secret.key"))
	must(t, err)
	log, err := changelog.Open(filepath.Join(dir, "aeolus.db"), changelog.Options{Check: Check(sch)})
	must(t, err)
	t.Cleanup(func() { log.Close() })

	L, S := change.Locations, change.Services
	set := func(tree change.TreeName, node, path string, v any) change.Op {
		raw, _ := json.Marshal(v)
		prepared, err := sch.Prepare(hierarchy.Path(path), raw, box)
		must(t, err)
		return change.Op{Kind: change.Set, Tree: tree, Node: hierarchy.NodeID(node), Path: hierarchy.Path(path), Value: prepared}
	}
	ops := []change.Op{
		{Kind: change.CreateOrg, Node: "symtus", Name: "Symtus", Account: "griff"},
		{Kind: change.AddFolder, Tree: L, Node: "house", Name: "House", Parent: "symtus"},
		{Kind: change.AddFolder, Tree: L, Node: "office", Name: "Office", Parent: "house"},
		{Kind: change.AddAP, Tree: L, Node: "office-ap", Name: "OfficeOpenWrt", Parent: "office"},
		{Kind: change.AddFolder, Tree: S, Node: "household", Name: "Household", Parent: "symtus"},
		{Kind: change.AddFolder, Tree: S, Node: "guest", Name: "Guest", Parent: "symtus"},
		set(L, "symtus", "system.poll", 60),
		{Kind: change.Lock, Tree: L, Node: "symtus", Path: "system.poll"},
		set(L, "symtus", "radio.5g.width", 80),
		set(L, "house", "radio.5g.width", 40),
		set(S, "household", "network.sweet.ssid", "Sweet Spot"),
		set(S, "household", "network.sweet.security", "wpa2-psk"),
		set(S, "household", "network.sweet.passphrase", passphrase),
		set(S, "household", "network.sweet.transport.primary.type", "vlan"),
		set(S, "household", "network.sweet.transport.primary.vlan", 20),
		set(S, "guest", "network.guest.ssid", "Guest"),
		set(S, "guest", "network.guest.security", "wpa2-psk"),
		{Kind: change.AssignServices, Node: "symtus", Services: []hierarchy.NodeID{"household"}},
		{Kind: change.AddAccount, Account: "claude", Name: "Claude"},
		{Kind: change.AddAccount, Account: "office", Name: "Leasing office"},
		{Kind: change.AddAccount, Account: "tenant", Name: "Tenant"},
		{Kind: change.GrantRole, Account: "claude", Tree: L, Node: "symtus", Role: "operator"},
		{Kind: change.GrantRole, Account: "claude", Tree: S, Node: "symtus", Role: "operator"},
		{Kind: change.GrantRole, Account: "office", Tree: S, Node: "household", Role: "operator"},
		{Kind: change.GrantRole, Account: "office", Tree: S, Node: "symtus", Role: "viewer"},
		{Kind: change.GrantRole, Account: "tenant", Tree: S, Node: "household", Role: "viewer"},
	}
	for _, op := range ops {
		if _, err := log.Commit("griff", "fixture", op); err != nil {
			t.Fatalf("%+v: %v", op, err)
		}
	}
	tokens := map[string]string{}
	for _, who := range []string{"griff", "claude", "office", "tenant"} {
		plain, id, hash, err := access.NewToken()
		must(t, err)
		if _, err := log.Commit("griff", "fixture", change.Op{Kind: change.IssueToken, Account: access.AccountID(who), TokenID: id, TokenHash: hash}); err != nil {
			t.Fatal(err)
		}
		tokens[who] = plain
	}
	conds, err := conditions.Open(filepath.Join(dir, "conditions.db"), nil)
	must(t, err)
	t.Cleanup(func() { conds.Close() })
	srv := httptest.NewServer(New(log, sch, box, conds).Handler())
	t.Cleanup(srv.Close)
	return &fixture{t: t, url: srv.URL, dir: dir, log: log, conds: conds, tokens: tokens}
}

// do sends a request as an account ("" for none) and returns the status and
// decoded body.
func (f *fixture) do(method, path, as string, body any) (int, map[string]any) {
	f.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		must(f.t, err)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, f.url+path, rd)
	must(f.t, err)
	if as != "" {
		req.Header.Set("Authorization", "Bearer "+f.tokens[as])
	}
	resp, err := http.DefaultClient.Do(req)
	must(f.t, err)
	defer resp.Body.Close()
	var out map[string]any
	must(f.t, json.NewDecoder(resp.Body).Decode(&out))
	return resp.StatusCode, out
}

func (f *fixture) change(as string, op map[string]any) (int, map[string]any) {
	return f.do("POST", "/v1/changes", as, map[string]any{"op": op, "reason": "test"})
}

func TestHealthAndAuthentication(t *testing.T) {
	f := newFixture(t)
	if code, body := f.do("GET", "/", "", nil); code != 200 || body["api"] != "/v1" {
		t.Fatalf("root = %d %v", code, body)
	}
	if code, body := f.do("GET", "/healthz", "", nil); code != 200 || body["ok"] != true {
		t.Fatalf("healthz = %d %v", code, body)
	}
	if code, _ := f.do("GET", "/v1/whoami", "", nil); code != 401 {
		t.Fatalf("no token: %d", code)
	}
	f.tokens["forger"] = "aeolus1.0000000000000000.nope"
	if code, _ := f.do("GET", "/v1/whoami", "forger", nil); code != 401 {
		t.Fatalf("bad token: %d", code)
	}
	code, body := f.do("GET", "/v1/whoami", "claude", nil)
	if code != 200 || body["account"] != "claude" || len(body["grants"].([]any)) != 2 {
		t.Fatalf("whoami = %d %v", code, body)
	}
}

func TestTreeShowsOnlyWhatTheCallerCanView(t *testing.T) {
	f := newFixture(t)
	count := func(tree, as string) int {
		code, body := f.do("GET", "/v1/trees/"+tree, as, nil)
		if code != 200 {
			t.Fatalf("%s as %s: %d %v", tree, as, code, body)
		}
		return len(body["nodes"].([]any))
	}
	if n := count("locations", "claude"); n != 4 {
		t.Errorf("claude sees %d location nodes, want 4", n)
	}
	if n := count("locations", "office"); n != 0 {
		t.Errorf("office sees %d location nodes, want 0", n)
	}
	if n := count("services", "tenant"); n != 1 {
		t.Errorf("tenant sees %d service nodes, want 1", n)
	}
	if code, _ := f.do("GET", "/v1/trees/sites", "claude", nil); code != 404 {
		t.Errorf("unknown tree: %d", code)
	}
}

func TestNodePage(t *testing.T) {
	f := newFixture(t)
	code, body := f.do("GET", "/v1/trees/locations/nodes/office-ap", "claude", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	width := body["fields"].(map[string]any)["radio.5g.width"].(map[string]any)
	if width["value"] != float64(40) || width["from"] != "house" || width["origin"] != "inherited" {
		t.Fatalf("width = %v", width)
	}
	poll := body["fields"].(map[string]any)["system.poll"].(map[string]any)
	if poll["origin"] != "locked" {
		t.Fatalf("poll = %v", poll)
	}
	if body["role"] != "operator" || len(body["problems"].([]any)) != 0 {
		t.Fatalf("role %v, problems %v", body["role"], body["problems"])
	}
	inEffect := body["overrides"].(map[string]any)["in_effect"].([]any)
	if len(inEffect) != 1 || inEffect[0].(map[string]any)["node"] != "house" {
		t.Fatalf("overrides in effect = %v", inEffect)
	}
	if code, _ := f.do("GET", "/v1/trees/locations/nodes/office-ap", "office", nil); code != 404 {
		t.Fatalf("office reading a Locations node: %d, want 404", code)
	}
	if code, _ := f.do("GET", "/v1/trees/locations/nodes/nope", "claude", nil); code != 404 {
		t.Fatalf("unknown node: %d", code)
	}
}

func TestNodeReportsUnmetRulesForThePageGuard(t *testing.T) {
	f := newFixture(t)
	_, body := f.do("GET", "/v1/trees/services/nodes/guest", "griff", nil)
	problems, _ := json.Marshal(body["problems"])
	if len(body["problems"].([]any)) != 2 || !strings.Contains(string(problems), "'transport'") || !strings.Contains(string(problems), "'passphrase'") {
		t.Fatalf("guest problems = %s", problems)
	}
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "services", "node": "guest", "path": "network.guest.transport.primary.type", "value": "vlan"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	_, body = f.do("GET", "/v1/trees/services/nodes/guest", "griff", nil)
	if got := body["problems"].([]any); len(got) != 2 {
		t.Fatalf("after a partial transport, problems = %v", got)
	}
	for _, op := range []map[string]any{
		{"kind": "set", "tree": "services", "node": "guest", "path": "network.guest.transport.primary.vlan", "value": 50},
		{"kind": "set", "tree": "services", "node": "guest", "path": "network.guest.passphrase", "value": "guest-pass-1"},
	} {
		if code, b := f.change("griff", op); code != 200 {
			t.Fatalf("%d %v", code, b)
		}
	}
	_, body = f.do("GET", "/v1/trees/services/nodes/guest", "griff", nil)
	if got := body["problems"].([]any); len(got) != 0 {
		t.Fatalf("complete network still has problems: %v", got)
	}
}

func TestChangeThroughTheAPI(t *testing.T) {
	f := newFixture(t)
	code, body := f.change("claude", map[string]any{"kind": "set", "tree": "locations", "node": "house", "path": "radio.5g.width", "value": 20})
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	if body["change"].(map[string]any)["actor"] != "claude" {
		t.Fatalf("change = %v", body["change"])
	}
	if rv := body["reversioned"].([]any); len(rv) != 1 || rv[0] != "office-ap" {
		t.Fatalf("reversioned = %v", rv)
	}
	_, node := f.do("GET", "/v1/trees/locations/nodes/office-ap", "claude", nil)
	if w := node["fields"].(map[string]any)["radio.5g.width"].(map[string]any); w["value"] != float64(20) {
		t.Fatalf("width after change = %v", w)
	}
}

func TestSecretsAreSealedAndNeverReturned(t *testing.T) {
	f := newFixture(t)
	const newPass = "brand-new-passphrase-7"
	code, body := f.change("office", map[string]any{"kind": "set", "tree": "services", "node": "household", "path": "network.sweet.passphrase", "value": newPass})
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	op := body["change"].(map[string]any)["op"].(map[string]any)
	if op["value"].(map[string]any)["sealed"] != true {
		t.Fatalf("returned op value = %v", op["value"])
	}
	for _, path := range []string{"/v1/changes?limit=1000", "/v1/trees/services/nodes/household", "/v1/aps/office-ap/config"} {
		code, b := f.do("GET", path, "claude", nil)
		raw, _ := json.Marshal(b)
		if code != 200 || strings.Contains(string(raw), newPass) || strings.Contains(string(raw), passphrase) {
			t.Fatalf("%s: %d, or it shows a passphrase: %s", path, code, raw)
		}
	}
	must(t, f.log.Close())
	files, _ := filepath.Glob(filepath.Join(f.dir, "aeolus.db*"))
	for _, file := range files {
		data, err := os.ReadFile(file)
		must(t, err)
		if strings.Contains(string(data), newPass) {
			t.Fatalf("%s contains the passphrase", filepath.Base(file))
		}
	}
}

func TestErrorsMapToStatuses(t *testing.T) {
	f := newFixture(t)
	set := func(tree, node, path string, v any) map[string]any {
		return map[string]any{"kind": "set", "tree": tree, "node": node, "path": path, "value": v}
	}
	cases := []struct {
		name string
		as   string
		op   map[string]any
		code int
		msg  string
	}{
		{"no role in this tree", "office", set("locations", "house", "radio.5g.width", 20), 403, "needs operator"},
		{"under a lock", "claude", set("locations", "house", "system.poll", 30), 409, "locked by symtus"},
		{"invalid value", "claude", set("locations", "house", "radio.5g.width", 33), 400, "value must be one of"},
		{"unknown field", "claude", set("locations", "house", "radio.9g.width", 20), 400, "unknown field"},
		{"wrong tree", "claude", set("services", "household", "radio.5g.width", 20), 400, "other tree"},
		{"unknown node", "claude", set("locations", "nope", "radio.5g.width", 20), 404, "not found"},
		{"create-org", "griff", map[string]any{"kind": "create-org", "node": "x", "name": "X", "account": "griff"}, 400, "aeolus init"},
		{"issue-token", "griff", map[string]any{"kind": "issue-token", "account": "claude", "token_id": "x"}, 400, "/v1/tokens"},
	}
	for _, c := range cases {
		code, body := f.change(c.as, c.op)
		if code != c.code || !strings.Contains(body["error"].(string), c.msg) {
			t.Errorf("%s: %d %v, want %d containing %q", c.name, code, body, c.code, c.msg)
		}
	}
	code, _ := f.do("POST", "/v1/changes", "claude", map[string]any{"op": map[string]any{"kind": "set"}, "surprise": 1})
	if code != 400 {
		t.Errorf("unknown request field: %d", code)
	}
}

func TestPreviewRecordsNothing(t *testing.T) {
	f := newFixture(t)
	seq := f.log.Seq()
	code, body := f.do("POST", "/v1/preview", "griff", map[string]any{"op": map[string]any{"kind": "lock", "tree": "locations", "node": "symtus", "path": "radio.5g.width"}})
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	removed := body["effect"].(map[string]any)["removed"].([]any)
	if len(removed) != 1 || removed[0].(map[string]any)["node"] != "house" {
		t.Fatalf("removed = %v", removed)
	}
	if rv := body["reversioned"].([]any); len(rv) != 1 || rv[0] != "office-ap" {
		t.Fatalf("reversioned = %v", rv)
	}
	if f.log.Seq() != seq {
		t.Fatal("preview committed")
	}
	if code, _ := f.do("POST", "/v1/preview", "claude", map[string]any{"op": map[string]any{"kind": "lock", "tree": "locations", "node": "symtus", "path": "radio.5g.width"}}); code != 403 {
		t.Fatalf("preview checks permissions too: %d", code)
	}
}

func TestTokens(t *testing.T) {
	f := newFixture(t)
	code, body := f.do("POST", "/v1/tokens", "claude", map[string]any{"reason": "MCP adapter"})
	if code != 200 || body["account"] != "claude" {
		t.Fatalf("%d %v", code, body)
	}
	f.tokens["claude2"] = body["token"].(string)
	if code, who := f.do("GET", "/v1/whoami", "claude2", nil); code != 200 || who["account"] != "claude" {
		t.Fatalf("new token: %d %v", code, who)
	}
	if code, _ := f.do("DELETE", "/v1/tokens/"+body["id"].(string), "claude", nil); code != 200 {
		t.Fatalf("revoke: %d", code)
	}
	if code, _ := f.do("GET", "/v1/whoami", "claude2", nil); code != 401 {
		t.Fatalf("revoked token: %d", code)
	}
	if code, _ := f.do("POST", "/v1/tokens", "claude", map[string]any{"account": "office"}); code != 403 {
		t.Fatalf("token for someone else: %d", code)
	}
}

func TestAPConfig(t *testing.T) {
	f := newFixture(t)
	code, body := f.do("GET", "/v1/aps/office-ap/config", "claude", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	if body["check"].(map[string]any)["ok"] != true || body["version"].(float64) < 1 {
		t.Fatalf("check %v, version %v", body["check"], body["version"])
	}
	sweet := body["networks"].(map[string]any)["sweet"].(map[string]any)["fields"].(map[string]any)
	if sweet["passphrase"].(map[string]any)["value"].(map[string]any)["sealed"] != true {
		t.Fatalf("passphrase = %v", sweet["passphrase"])
	}
	if code, _ := f.do("GET", "/v1/aps/office-ap/config", "office", nil); code != 404 {
		t.Fatalf("office reading an AP: %d", code)
	}
}

func TestChangeLogNeedsViewerAtTheRoot(t *testing.T) {
	f := newFixture(t)
	if code, body := f.do("GET", "/v1/changes?after=0&limit=5", "office", nil); code != 200 || len(body["changes"].([]any)) != 5 {
		t.Fatalf("office: %d %v", code, body)
	}
	if code, _ := f.do("GET", "/v1/changes", "tenant", nil); code != 403 {
		t.Fatalf("tenant: %d", code)
	}
	if code, _ := f.do("GET", "/v1/changes?limit=0", "claude", nil); code != 400 {
		t.Fatalf("bad limit: %d", code)
	}
	_, body := f.do("GET", "/v1/changes?limit=1000", "claude", nil)
	for _, c := range body["changes"].([]any) {
		if _, ok := c.(map[string]any)["op"].(map[string]any)["token_hash"]; ok {
			t.Fatal("a token hash was returned")
		}
	}
}

func TestLibraryThroughTheAPI(t *testing.T) {
	f := newFixture(t)
	homelab := map[string]any{"kind": "set-concentrator", "concentrator": "homelab",
		"value": map[string]any{"name": "Homelab", "address": "1.1.1.2", "port": 4789, "mtu": 1450}}
	if code, body := f.change("claude", homelab); code != 403 {
		t.Fatalf("operator editing the library: %d %v", code, body)
	}
	bad := map[string]any{"kind": "set-concentrator", "concentrator": "bad",
		"value": map[string]any{"name": "Bad", "address": "1.1.1.3", "port": 70000, "mtu": 1450}}
	if code, body := f.change("griff", bad); code != 400 || !strings.Contains(body["error"].(string), "port") {
		t.Fatalf("bad port: %d %v", code, body)
	}
	if code, body := f.change("griff", homelab); code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	if code, body := f.change("griff", map[string]any{"kind": "set-vni", "concentrator": "homelab", "vni": 20, "name": "Trusted"}); code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	code, body := f.do("GET", "/v1/library", "tenant", nil)
	concs := body["concentrators"].([]any)
	if code != 200 || len(concs) != 1 || concs[0].(map[string]any)["vnis"].(map[string]any)["20"] != "Trusted" {
		t.Fatalf("library = %d %v", code, body)
	}

	ref := func(id string) map[string]any {
		return map[string]any{"kind": "set", "tree": "services", "node": "household", "path": "network.sweet.transport.fallback.concentrator", "value": id}
	}
	// A transport names a tunnel set in Locations, not a library entry (0055).
	if code, body := f.change("griff", ref("nowhere")); code != 200 {
		t.Fatalf("a tunnel the library lacks: %d %v", code, body)
	}
	if code, body := f.change("griff", ref("homelab")); code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	if code, body := f.change("griff", map[string]any{"kind": "remove-concentrator", "concentrator": "homelab"}); code != 409 || !strings.Contains(body["error"].(string), "in use") {
		t.Fatalf("removing a concentrator in use: %d %v", code, body)
	}
}

func TestAPConfigShowsTheComposedDocument(t *testing.T) {
	f := newFixture(t)
	_, body := f.do("GET", "/v1/aps/office-ap/config", "claude", nil)
	doc := body["document"].(map[string]any)
	sweet := doc["network"].(map[string]any)["sweet"].(map[string]any)
	if sweet["ssid"] != "Sweet Spot" || sweet["passphrase"].(map[string]any)["sealed"] != true {
		t.Fatalf("document network = %v", sweet)
	}
	if body["unassigned"] != false {
		t.Fatalf("unassigned = %v", body["unassigned"])
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
