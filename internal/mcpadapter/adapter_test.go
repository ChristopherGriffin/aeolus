package mcpadapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/api"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/changelog"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/schema"
	"github.com/ChristopherGriffin/aeolus/internal/secret"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fixture struct {
	url    string
	log    *changelog.Log
	tokens map[string]string
}

// newFixture serves the API and the adapter over a fresh log. griff is admin
// (create-org); claude is operator at both roots.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	sch, err := schema.V1()
	must(t, err)
	dir := t.TempDir()
	box, err := secret.LoadOrCreate(filepath.Join(dir, "secret.key"))
	must(t, err)
	log, err := changelog.Open(filepath.Join(dir, "aeolus.db"), changelog.Options{Check: api.Check(sch)})
	must(t, err)
	t.Cleanup(func() { log.Close() })
	for _, op := range []change.Op{
		{Kind: change.CreateOrg, Node: "symtus", Name: "Symtus", Account: "griff"},
		{Kind: change.AddFolder, Tree: change.Locations, Node: "house", Name: "House", Parent: "symtus"},
		{Kind: change.AddFolder, Tree: change.Services, Node: "household", Name: "Household", Parent: "symtus"},
		{Kind: change.AddAccount, Account: "claude", Name: "Claude"},
		{Kind: change.GrantRole, Account: "claude", Tree: change.Locations, Node: "symtus", Role: "operator"},
		{Kind: change.GrantRole, Account: "claude", Tree: change.Services, Node: "symtus", Role: "operator"},
	} {
		if _, err := log.Commit("griff", "fixture", op); err != nil {
			t.Fatal(err)
		}
	}
	tokens := map[string]string{}
	for _, who := range []string{"griff", "claude"} {
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
	apiHandler := api.New(log, sch, box, conds).Handler()
	mux := http.NewServeMux()
	mux.Handle("/mcp", New(apiHandler, "test"))
	mux.Handle("/", apiHandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &fixture{url: srv.URL, log: log, tokens: tokens}
}

// bearer adds an Authorization header to every request.
type bearer struct {
	token string
	next  http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.next.RoundTrip(r)
}

func (f *fixture) session(t *testing.T, as string) *mcp.ClientSession {
	t.Helper()
	hc := &http.Client{Transport: bearer{token: f.tokens[as], next: http.DefaultTransport}}
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	s, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: f.url + "/mcp", HTTPClient: hc, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	must(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func call(t *testing.T, s *mcp.ClientSession, name string, args any) (*mcp.CallToolResult, string) {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	must(t, err)
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	return res, text.String()
}

func TestRequestWithoutATokenIsRefused(t *testing.T) {
	f := newFixture(t)
	resp, err := http.Post(f.url+"/mcp", "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	must(t, err)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", resp.StatusCode)
	}
}

func TestToolsActAsTheCaller(t *testing.T) {
	f := newFixture(t)
	s := f.session(t, "claude")
	tools, err := s.ListTools(context.Background(), nil)
	must(t, err)
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	want := "get_ap_config get_ap_history get_library get_node get_relayed_dhcp list_changes list_detected list_tree make_change preview_change whoami"
	if strings.Join(names, " ") != want {
		t.Fatalf("tools = %v", names)
	}
	if _, text := call(t, s, "whoami", map[string]any{}); !strings.Contains(text, `"account":"claude"`) {
		t.Fatalf("whoami = %s", text)
	}
}

func TestChangeIsLoggedUnderTheCallersNameWithTheReason(t *testing.T) {
	f := newFixture(t)
	s := f.session(t, "claude")
	res, text := call(t, s, "make_change", map[string]any{
		"op":     map[string]any{"kind": "set", "tree": "locations", "node": "house", "path": "radio.5g.width", "value": 40},
		"reason": "wider channels for the house",
	})
	if res.IsError {
		t.Fatalf("make_change failed: %s", text)
	}
	entries, err := f.log.Entries(f.log.Seq()-1, 1)
	must(t, err)
	if e := entries[0]; e.Actor != "claude" || e.Reason != "wider channels for the house" || e.Op.Path != "radio.5g.width" {
		t.Fatalf("logged %+v", e)
	}
}

// A change needs no reason (0062): the log records who made it and when, and
// the tool's schema does not ask for one.
func TestChangeNeedsNoReason(t *testing.T) {
	f := newFixture(t)
	s := f.session(t, "claude")
	tools, err := s.ListTools(context.Background(), nil)
	must(t, err)
	for _, tool := range tools.Tools {
		if tool.Name != "make_change" {
			continue
		}
		raw, err := json.Marshal(tool.InputSchema)
		must(t, err)
		var schema struct {
			Required []string `json:"required"`
		}
		must(t, json.Unmarshal(raw, &schema))
		if strings.Join(schema.Required, " ") != "op" {
			t.Fatalf("make_change requires %v, want only op", schema.Required)
		}
	}
	for _, args := range []map[string]any{
		{"op": map[string]any{"kind": "set", "tree": "locations", "node": "house", "path": "radio.5g.width", "value": 40}},
		{"op": map[string]any{"kind": "set", "tree": "locations", "node": "house", "path": "radio.5g.width", "value": 80}, "reason": "  "},
	} {
		before := f.log.Seq()
		res, text := call(t, s, "make_change", args)
		if res.IsError {
			t.Fatalf("make_change %v failed: %s", args, text)
		}
		entries, err := f.log.Entries(before, 1)
		must(t, err)
		if e := entries[0]; e.Actor != "claude" || e.Reason != "" || e.At.IsZero() || e.Op.Path != "radio.5g.width" {
			t.Fatalf("logged %+v", e)
		}
	}
}

func TestRefusalsAreToolErrors(t *testing.T) {
	f := newFixture(t)
	s := f.session(t, "claude")
	cases := []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"op": map[string]any{"kind": "lock", "tree": "locations", "node": "symtus", "path": "radio.5g.width"}, "reason": "x"}, "needs admin"},
		{map[string]any{"op": map[string]any{"kind": "set", "tree": "locations", "node": "house", "path": "radio.5g.width", "value": 33}, "reason": "x"}, "value must be one of"},
	}
	for _, c := range cases {
		res, text := call(t, s, "make_change", c.args)
		if !res.IsError || !strings.Contains(text, c.want) {
			t.Errorf("%v: IsError=%v, text %q, want %q", c.args, res.IsError, text, c.want)
		}
	}
}

func TestPreviewRecordsNothing(t *testing.T) {
	f := newFixture(t)
	s := f.session(t, "claude")
	seq := f.log.Seq()
	res, text := call(t, s, "preview_change", map[string]any{
		"op": map[string]any{"kind": "set", "tree": "locations", "node": "house", "path": "radio.5g.width", "value": 80},
	})
	if res.IsError || !strings.Contains(text, `"effect"`) {
		t.Fatalf("preview: %s", text)
	}
	if f.log.Seq() != seq {
		t.Fatal("preview committed a change")
	}
}

func TestSecretsStaySealed(t *testing.T) {
	f := newFixture(t)
	s := f.session(t, "claude")
	const pass = "a-secret-passphrase-9"
	res, text := call(t, s, "make_change", map[string]any{
		"op":     map[string]any{"kind": "set", "tree": "services", "node": "household", "path": "network.sweet.passphrase", "value": pass},
		"reason": "set the household passphrase",
	})
	if res.IsError || strings.Contains(text, pass) {
		t.Fatalf("make_change: error=%v, text %s", res.IsError, text)
	}
	for _, args := range []map[string]any{{"tree": "services", "node": "household"}} {
		_, text := call(t, s, "get_node", args)
		if strings.Contains(text, pass) || !strings.Contains(text, `"sealed":true`) {
			t.Fatalf("get_node shows the passphrase or not the seal: %s", text)
		}
	}
	_, text = call(t, s, "list_changes", map[string]any{"limit": 1000})
	if strings.Contains(text, pass) {
		t.Fatal("list_changes shows the passphrase")
	}
	var out map[string]any
	must(t, json.Unmarshal([]byte(text), &out))
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
