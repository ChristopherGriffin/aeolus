package schema

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/changelog"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/secret"
)

const passphrase = "sweet-spot-passphrase-42"

// scenario is the UI mockup with v1 fields. Secret values are prepared
// (sealed) the way the API will prepare them.
func scenario(t *testing.T, s *Schema, box *secret.Box) []change.Op {
	t.Helper()
	L, S := change.Locations, change.Services
	folder := func(tree change.TreeName, id, name, parent string) change.Op {
		return change.Op{Kind: change.AddFolder, Tree: tree, Node: hierarchy.NodeID(id), Name: name, Parent: hierarchy.NodeID(parent)}
	}
	ap := func(id, name, parent string) change.Op {
		return change.Op{Kind: change.AddAP, Tree: L, Node: hierarchy.NodeID(id), Name: name, Parent: hierarchy.NodeID(parent)}
	}
	set := func(tree change.TreeName, node, path string, v any) change.Op {
		raw, err := json.Marshal(v)
		must(t, err)
		prepared, err := s.Prepare(hierarchy.Path(path), raw, box)
		must(t, err)
		return change.Op{Kind: change.Set, Tree: tree, Node: hierarchy.NodeID(node), Path: hierarchy.Path(path), Value: prepared}
	}
	return []change.Op{
		{Kind: change.CreateOrg, Node: "symtus", Name: "Symtus", Account: "griff"},
		folder(L, "house", "House", "symtus"),
		folder(L, "office", "Office", "house"),
		ap("office-ap", "OfficeOpenWrt", "office"),
		folder(L, "gate", "Main Gate", "symtus"),
		ap("gate-ap", "GateOpenWrt", "gate"),
		folder(S, "household", "Household", "symtus"),
		folder(S, "household-gate", "Gate", "household"),

		set(L, "symtus", "system.country", "US"),
		set(L, "symtus", "system.tz", "America/Chicago"),
		set(L, "symtus", "system.poll", 60),
		{Kind: change.Lock, Tree: L, Node: "symtus", Path: "system.poll"},
		set(L, "symtus", "radio.2g.width", 20),
		set(L, "symtus", "radio.5g.channel", "auto"),
		set(L, "symtus", "radio.5g.width", 80),
		set(L, "symtus", "ports.eth0.uplink", true),
		set(L, "symtus", "ports.eth0.mode", "trunk"),
		set(L, "symtus", "ports.eth0.untagged", 1),
		set(L, "symtus", "ports.eth0.tagged", []int{10, 20, 50}),
		set(L, "house", "radio.5g.width", 40),
		set(L, "office-ap", "radio.5g.channel", 48),
		set(L, "gate", "ports.eth0.mode", "access"),

		set(S, "household", "network.sweet.ssid", "Sweet Spot"),
		set(S, "household", "network.sweet.security", "wpa2-wpa3"),
		set(S, "household", "network.sweet.passphrase", passphrase),
		set(S, "household", "network.sweet.bands", []string{"2g", "5g"}),
		set(S, "household", "network.sweet.transport.primary.type", "vlan"),
		set(S, "household", "network.sweet.transport.primary.vlan", 20),
		set(S, "household", "network.sweet.transport.fallback.type", "vxlan"),
		set(S, "household", "network.sweet.transport.fallback.concentrator", "homelab"),
		set(S, "household", "network.sweet.transport.fallback.vni", 20),
		set(S, "household", "network.sweet.transport.ha", true),
		set(S, "household", "network.sweet.transport.failback", "revertive"),
		set(S, "household", "network.sweet.transport.holddown", 120),
		set(S, "household-gate", "network.sweet.transport.primary.type", "vxlan"),
		set(S, "household-gate", "network.sweet.transport.primary.concentrator", "homelab"),
		set(S, "household-gate", "network.sweet.transport.primary.vni", 20),

		{Kind: change.AssignServices, Node: "symtus", Services: []hierarchy.NodeID{"household"}},
		{Kind: change.AssignServices, Node: "gate", Services: []hierarchy.NodeID{"household-gate"}},
	}
}

func TestScenarioPassesTheGuardAndBuildsValidConfigs(t *testing.T) {
	s, box := v1(t), newBox(t)
	var state *change.State
	for _, op := range scenario(t, s, box) {
		if err := s.CheckOp(op); err != nil {
			t.Fatalf("%+v refused: %v", op, err)
		}
		var err error
		if state, _, err = change.Apply(state, op); err != nil {
			t.Fatalf("%+v: %v", op, err)
		}
	}
	org := state.Org
	for _, ap := range org.Locations.APs() {
		cfg, err := org.ResolveAP(ap)
		must(t, err)
		doc, err := document(cfg, box.Open)
		must(t, err)
		if err := s.CheckDocument(doc); err != nil {
			t.Fatalf("%s config invalid: %v\n%v", ap, err, doc)
		}
	}

	cfg, err := org.ResolveAP("office-ap")
	must(t, err)
	doc, err := document(cfg, box.Open)
	must(t, err)
	radio5 := doc["radio"].(map[string]any)["5g"].(map[string]any)
	if radio5["width"] != float64(40) || radio5["channel"] != float64(48) {
		t.Fatalf("office 5 GHz = %v", radio5)
	}
	sweet := doc["network"].(map[string]any)["sweet"].(map[string]any)
	if sweet["passphrase"] != passphrase {
		t.Fatalf("revealed passphrase = %v", sweet["passphrase"])
	}

	gate, err := org.ResolveAP("gate-ap")
	must(t, err)
	gdoc, err := document(gate, nil)
	must(t, err)
	primary := gdoc["network"].(map[string]any)["sweet"].(map[string]any)["transport"].(map[string]any)["primary"].(map[string]any)
	if primary["type"] != "vxlan" || primary["vni"] != float64(20) {
		t.Fatalf("gate primary transport = %v", primary)
	}
	if !secret.IsSealed(gdoc["network"].(map[string]any)["sweet"].(map[string]any)["passphrase"]) {
		t.Fatal("Document without reveal exposed the passphrase")
	}
}

func TestDescribe(t *testing.T) {
	sch, err := V1()
	if err != nil {
		t.Fatal(err)
	}
	d := sch.Describe()
	ssid := d.Fields["network.*.ssid"]
	if ssid["type"] != "string" || ssid["maxLength"] != json.Number("32") && ssid["maxLength"] != float64(32) || ssid["x-aeolus-tree"] != "services" {
		t.Fatalf("network.*.ssid = %v", ssid)
	}
	if vlan := d.Fields["network.*.transport.primary.vlan"]; vlan["type"] != "integer" || vlan["x-aeolus-tree"] != "services" {
		t.Fatalf("vlan = %v", vlan) // through two $refs
	}
	if pass := d.Fields["network.*.passphrase"]; pass["writeOnly"] != true {
		t.Fatalf("passphrase = %v", pass)
	}
	if w := d.Fields["radio.5g.width"]; w["x-aeolus-tree"] != "locations" {
		t.Fatalf("radio.5g.width = %v", w)
	}
	if d.Names["network"] != "^[a-z0-9][a-z0-9-]{0,31}$" || d.Names["ports"] == "" {
		t.Fatalf("names = %v", d.Names)
	}
	// Every leaf but those the manager fills in.
	for _, l := range sch.Leaves() {
		_, described := d.Fields[l]
		if described == strings.HasPrefix(l, "concentrators.") {
			t.Errorf("%s described: %v", l, described)
		}
	}
}

func TestLeaves(t *testing.T) {
	sch := v1(t)
	leaves := sch.Leaves()
	has := map[string]bool{}
	for _, l := range leaves {
		has[l] = true
	}
	for _, want := range []string{"radio.5g.channel", "system.management.vlan", "system.ntp", "ports.*.tagged", "network.*.transport.primary.vni", "network.*.roaming.ft", "concentrators.*.mtu"} {
		if !has[want] {
			t.Errorf("missing %s", want)
		}
	}
	for _, l := range leaves {
		if strings.Contains(l, "libraryConcentrator") || strings.HasSuffix(l, ".transport") || strings.HasPrefix(l, ".") {
			t.Errorf("unexpected leaf %s", l)
		}
		if f, err := sch.Field(hierarchy.Path(strings.ReplaceAll(l, "*", "x"))); err != nil && !strings.HasPrefix(l, "concentrators.") {
			t.Errorf("%s is not a field: %v", l, err)
		} else if err == nil && f.Path == "" {
			t.Errorf("%s resolved to nothing", l)
		}
	}
}

func TestFieldPaths(t *testing.T) {
	s := v1(t)
	ok := map[hierarchy.Path]change.TreeName{
		"radio.2g.channel":                     change.Locations,
		"radio.6g.width":                       change.Locations,
		"system.management.vlan":               change.Locations,
		"ports.eth0.tagged":                    change.Locations,
		"ports.lan1.bond":                      change.Locations,
		"network.sweet.ssid":                   change.Services,
		"network.sweet.transport.primary.vni":  change.Services,
		"network.sweet.roaming.ft":             change.Services,
		"network.guest-2.rate_limit.down_kbps": change.Services,
	}
	for p, tree := range ok {
		f, err := s.Field(p)
		if err != nil || f.Tree != tree {
			t.Errorf("Field(%s) = %+v, %v; want tree %s", p, f, err, tree)
		}
	}
	bad := map[hierarchy.Path]error{
		"radio.2g":                         ErrNotAField,
		"network.sweet.transport.primary":  ErrNotAField,
		"radio.9g.channel":                 ErrUnknownField,
		"network.Bad_Name.ssid":            ErrUnknownField,
		"network.sweet.ssid.extra":         ErrUnknownField,
		"system.nope":                      ErrUnknownField,
		"concentrators.homelab.address":    ErrReadOnly,
		"":                                 ErrUnknownField,
		"network..ssid":                    ErrUnknownField,
		"ports.Eth0.mode":                  ErrUnknownField,
		"network.sweet.transport.tertiary": ErrUnknownField,
	}
	for p, want := range bad {
		if _, err := s.Field(p); !errors.Is(err, want) {
			t.Errorf("Field(%q): got %v, want %v", p, err, want)
		}
	}
	if f, _ := s.Field("network.sweet.passphrase"); !f.Secret {
		t.Error("passphrase is not marked secret")
	}
	if f, _ := s.Field("network.sweet.ssid"); f.Secret {
		t.Error("ssid is marked secret")
	}
}

func TestFieldValues(t *testing.T) {
	s := v1(t)
	cases := []struct {
		path hierarchy.Path
		v    any
		ok   bool
	}{
		{"radio.2g.channel", 6, true},
		// SNMP's values go into snmpd's config lines (0052).
		{"system.snmp.community", "aeolus-ro", true},
		{"system.snmp.community", "two words", false},
		{"system.snmp.v3.user", "monitor", true},
		{"system.snmp.v3.auth", "long enough passphrase", true},
		{"system.snmp.v3.auth", "short", false},
		{"system.snmp.v3.auth", `has a "quote"`, false},
		{"system.snmp.location", "Pumphouse, north wall", true},
		{"system.snmp.location", "two\nlines", false},
		{"radio.2g.channel", "auto", true},
		{"radio.2g.channel", 36, false},
		{"radio.5g.channel", 48, true},
		{"radio.5g.channel", 50, false},
		{"radio.6g.channel", 37, true},
		{"radio.6g.channel", 38, false},
		{"radio.5g.width", 80, true},
		{"radio.5g.width", 320, false},
		{"radio.6g.width", 320, true},
		{"radio.2g.power", 20, true},
		{"radio.2g.power", 40, false},
		{"system.country", "US", true},
		{"system.country", "us", false},
		{"system.poll", 5, false},
		{"system.poll", 60.5, false},
		{"system.ssh_keys", []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJFp griff@laptop"}, true},
		{"system.ssh_keys", []string{"not a key"}, false},
		{"network.sweet.ssid", strings.Repeat("x", 32), true},
		{"network.sweet.ssid", strings.Repeat("x", 33), false},
		{"network.sweet.security", "wep", false},
		{"network.sweet.bands", []string{"5g", "5g"}, false},
		{"network.sweet.transport.primary.vlan", 4094, true},
		{"network.sweet.transport.primary.vlan", 4095, false},
		{"network.sweet.transport.primary.vni", 16777216, false},
		{"network.sweet.transport.failback", "sticky", false},
		{"ports.eth0.tagged", []int{10, 20}, true},
		{"ports.eth0.tagged", []int{0}, false},
	}
	for _, c := range cases {
		v := roundTrip(t, c.v)
		err := s.Check(c.path, v)
		if (err == nil) != c.ok {
			t.Errorf("Check(%s, %v) = %v, want ok=%v", c.path, c.v, err, c.ok)
		}
	}
}

func TestPrepareSealsSecretsAndGuardRefusesPlainOnes(t *testing.T) {
	s, box := v1(t), newBox(t)
	raw, _ := json.Marshal(passphrase)

	sealed, err := s.Prepare("network.sweet.passphrase", raw, box)
	must(t, err)
	if strings.Contains(string(sealed), passphrase) {
		t.Fatalf("prepared secret shows the plain text: %s", sealed)
	}
	good := change.Op{Kind: change.Set, Tree: change.Services, Node: "household", Path: "network.sweet.passphrase", Value: sealed}
	must(t, s.CheckOp(good))

	plain := good
	plain.Value = raw
	if err := s.CheckOp(plain); !errors.Is(err, ErrPlainSecret) {
		t.Fatalf("plain secret: got %v, want ErrPlainSecret", err)
	}
	if _, err := s.Prepare("network.sweet.passphrase", raw, nil); !errors.Is(err, ErrNoKey) {
		t.Fatalf("no key: got %v, want ErrNoKey", err)
	}
	short, _ := json.Marshal("short")
	if _, err := s.Prepare("network.sweet.passphrase", short, box); err == nil {
		t.Fatal("a too-short passphrase was prepared")
	}
}

func TestGuardKeepsFieldsInTheirTree(t *testing.T) {
	s := v1(t)
	cases := []change.Op{
		{Kind: change.Set, Tree: change.Services, Node: "household", Path: "radio.2g.channel", Value: json.RawMessage(`6`)},
		{Kind: change.Set, Tree: change.Locations, Node: "house", Path: "network.sweet.ssid", Value: json.RawMessage(`"x"`)},
		{Kind: change.Lock, Tree: change.Services, Node: "household", Path: "system.poll"},
	}
	for _, op := range cases {
		if err := s.CheckOp(op); !errors.Is(err, ErrWrongTree) {
			t.Errorf("%+v: got %v, want ErrWrongTree", op, err)
		}
	}
	for _, op := range []change.Op{
		{Kind: change.Lock, Tree: change.Locations, Node: "symtus", Path: hierarchy.ServicesPath},
		{Kind: change.AddFolder, Tree: change.Locations, Node: "x", Name: "X", Parent: "symtus"},
	} {
		must(t, s.CheckOp(op))
	}
}

func TestDocumentRulesTieFieldsTogether(t *testing.T) {
	s := v1(t)
	complete := func() map[string]any {
		return roundTrip(t, map[string]any{
			"network": map[string]any{"sweet": map[string]any{
				"ssid": "Sweet Spot", "security": "wpa2-psk", "passphrase": passphrase,
				"transport": map[string]any{"primary": map[string]any{"type": "vxlan", "concentrator": "homelab", "vni": 20}},
			}},
		}).(map[string]any)
	}
	must(t, s.CheckDocument(complete()))

	noPass := complete()
	delete(noPass["network"].(map[string]any)["sweet"].(map[string]any), "passphrase")
	if err := s.CheckDocument(noPass); err == nil {
		t.Error("WPA2 network without a passphrase passed")
	}
	noVNI := complete()
	delete(noVNI["network"].(map[string]any)["sweet"].(map[string]any)["transport"].(map[string]any)["primary"].(map[string]any), "vni")
	if err := s.CheckDocument(noVNI); err == nil {
		t.Error("VXLAN transport without a VNI passed")
	}
	noTransport := complete()
	delete(noTransport["network"].(map[string]any)["sweet"].(map[string]any), "transport")
	if err := s.CheckDocument(noTransport); err == nil {
		t.Error("network without a transport passed")
	}
}

func TestErrorMessagesAreReadable(t *testing.T) {
	s := v1(t)
	check := func(p hierarchy.Path, v any) string {
		err := s.Check(p, roundTrip(t, v))
		if err == nil {
			t.Fatalf("Check(%s, %v) passed", p, v)
		}
		return err.Error()
	}
	for _, c := range []struct{ got, want string }{
		{check("network.sweet.ssid", strings.Repeat("x", 33)), "network.sweet.ssid: maxLength: got 33, want 32"},
		{check("system.poll", 5), "system.poll: minimum: got 5, want 10"},
		{check("radio.2g.channel", 36), "radio.2g.channel: value must be 'auto' or maximum: got 36, want 14"},
	} {
		if c.got != c.want {
			t.Errorf("got  %q\nwant %q", c.got, c.want)
		}
	}
	doc := roundTrip(t, map[string]any{"network": map[string]any{"sweet": map[string]any{
		"ssid": "S", "security": "wpa2-psk",
		"transport": map[string]any{"primary": map[string]any{"type": "vxlan", "concentrator": "homelab"}},
	}}}).(map[string]any)
	err := s.CheckDocument(doc)
	want := "network.sweet.transport.primary: missing property 'vni'; network.sweet: missing property 'passphrase'"
	if err == nil || err.Error() != want {
		t.Errorf("document error:\ngot  %v\nwant %s", err, want)
	}
}

func TestSecretsNeverReachTheLogInPlainText(t *testing.T) {
	s, box := v1(t), newBox(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "aeolus.db")
	log, err := changelog.Open(path, changelog.Options{Check: schemaOnly(s)})
	must(t, err)
	for _, op := range scenario(t, s, box) {
		if _, err := log.Commit("claude", "scenario", op); err != nil {
			t.Fatalf("%+v: %v", op, err)
		}
	}
	plain := change.Op{Kind: change.Set, Tree: change.Services, Node: "household", Path: "network.sweet.passphrase", Value: json.RawMessage(`"` + passphrase + `"`)}
	if _, err := log.Commit("claude", "", plain); !errors.Is(err, ErrPlainSecret) {
		t.Fatalf("plain passphrase commit: got %v, want ErrPlainSecret", err)
	}
	must(t, log.Close())

	files, err := filepath.Glob(filepath.Join(dir, "aeolus.db*"))
	must(t, err)
	for _, f := range files {
		data, err := os.ReadFile(f)
		must(t, err)
		if strings.Contains(string(data), passphrase) {
			t.Fatalf("%s contains the passphrase in plain text", filepath.Base(f))
		}
	}

	log, err = changelog.Open(path, changelog.Options{Check: schemaOnly(s)})
	must(t, err)
	defer log.Close()
	cfg, err := log.Snapshot().Org.ResolveAP("office-ap")
	must(t, err)
	doc, err := document(cfg, box.Open)
	must(t, err)
	if got := doc["network"].(map[string]any)["sweet"].(map[string]any)["passphrase"]; got != passphrase {
		t.Fatalf("passphrase after replay = %v", got)
	}
}

// document assembles an AP's resolved fields; the manager does this, with the
// library, in package compose.
func document(cfg hierarchy.APConfig, reveal func(string, any) (any, error)) (map[string]any, error) {
	fields := map[string]any{}
	for p, r := range cfg.Location {
		fields[string(p)] = r.Value
	}
	for id, n := range cfg.Networks {
		for f, r := range n.Fields {
			fields["network."+id+"."+f] = r.Value
		}
	}
	return Assemble(fields, reveal)
}

// schemaOnly adapts CheckOp to the change log's commit check.
func schemaOnly(s *Schema) func(*change.State, string, change.Op) error {
	return func(_ *change.State, _ string, op change.Op) error { return s.CheckOp(op) }
}

func v1(t *testing.T) *Schema {
	t.Helper()
	s, err := V1()
	must(t, err)
	return s
}

func newBox(t *testing.T) *secret.Box {
	t.Helper()
	b, err := secret.LoadOrCreate(filepath.Join(t.TempDir(), "secret.key"))
	must(t, err)
	return b
}

// roundTrip passes a value through JSON, as every committed value does.
func roundTrip(t *testing.T, v any) any {
	t.Helper()
	raw, err := json.Marshal(v)
	must(t, err)
	var out any
	must(t, json.Unmarshal(raw, &out))
	return out
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
