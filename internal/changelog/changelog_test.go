package changelog

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// scenario is the UI mockup, built as a sequence of changes.
func scenario() []change.Op {
	L, S := change.Locations, change.Services
	folder := func(tree change.TreeName, id, name, parent string) change.Op {
		return change.Op{Kind: change.AddFolder, Tree: tree, Node: hierarchy.NodeID(id), Name: name, Parent: hierarchy.NodeID(parent)}
	}
	ap := func(id, name, parent string) change.Op {
		return change.Op{Kind: change.AddAP, Tree: L, Node: hierarchy.NodeID(id), Name: name, Parent: hierarchy.NodeID(parent)}
	}
	set := func(tree change.TreeName, node, path string, v any) change.Op {
		return change.Op{Kind: change.Set, Tree: tree, Node: hierarchy.NodeID(node), Path: hierarchy.Path(path), Value: js(v)}
	}
	assign := func(node string, folders ...hierarchy.NodeID) change.Op {
		return change.Op{Kind: change.AssignServices, Node: hierarchy.NodeID(node), Services: folders}
	}
	return []change.Op{
		{Kind: change.CreateOrg, Node: "symtus", Name: "Symtus"},
		folder(L, "house", "House", "symtus"),
		folder(L, "office", "Office", "house"),
		ap("office-ap", "OfficeOpenWrt", "office"),
		folder(L, "pump", "Pumphouse", "symtus"),
		ap("pump-ap", "PumphouseAP", "pump"),
		folder(L, "gate", "Main Gate", "symtus"),
		ap("gate-ap", "GateOpenWrt", "gate"),
		folder(S, "household", "Household", "symtus"),
		folder(S, "household-gate", "Gate", "household"),
		folder(S, "iot", "IoT", "symtus"),
		set(L, "symtus", "radio.5g.width", "80"),
		set(L, "symtus", "radio.5g.channel", "auto"),
		set(L, "symtus", "system.poll", "60"),
		{Kind: change.Lock, Tree: L, Node: "symtus", Path: "system.poll"},
		set(S, "household", "network.sweet.ssid", "Sweet Spot"),
		set(S, "household", "network.sweet.segment", "vlan 20"),
		set(S, "household-gate", "network.sweet.segment", "vxlan vx20"),
		set(S, "iot", "network.iot.ssid", "Sweet_Spot_IoT"),
		assign("symtus", "household", "iot"),
		set(L, "house", "radio.5g.width", "40"),
		set(L, "office-ap", "radio.5g.channel", "48"),
		assign("pump", "iot"),
	}
}

func TestReplayRebuildsState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aeolus.db")
	l := openAt(t, path)
	commitAll(t, l, scenario())
	wantSeq, wantState, wantVersions := l.Seq(), resolved(t, l.Snapshot()), versions(l)
	must(t, l.Close())

	l = openAt(t, path)
	if l.Seq() != wantSeq {
		t.Fatalf("seq after reopen = %d, want %d", l.Seq(), wantSeq)
	}
	if got := resolved(t, l.Snapshot()); !reflect.DeepEqual(got, wantState) {
		t.Fatalf("state after replay differs:\n%v\nwant\n%v", got, wantState)
	}
	if got := versions(l); !reflect.DeepEqual(got, wantVersions) {
		t.Fatalf("versions after replay = %v, want %v", got, wantVersions)
	}
}

func TestLogIsAppendOnly(t *testing.T) {
	l := open(t)
	commitAll(t, l, scenario())
	for _, q := range []string{`UPDATE changes SET actor = 'mallory'`, `DELETE FROM changes`} {
		if _, err := l.db.Exec(q); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("%s: got %v, want an append-only refusal", q, err)
		}
	}
}

func TestAtRebuildsThePast(t *testing.T) {
	l := open(t)
	commitAll(t, l, scenario())
	a := commit(t, l, setOp(change.Locations, "house", "radio.5g.width", "20"))
	b := commit(t, l, setOp(change.Locations, "house", "radio.5g.width", "160"))

	for _, c := range []struct {
		seq  int64
		want string
	}{{a.Seq, "20"}, {b.Seq, "160"}, {a.Seq - 1, "40"}} {
		org, err := l.At(c.seq)
		must(t, err)
		if r, _ := org.Locations.Resolve("office-ap", "radio.5g.width"); r.Value != c.want {
			t.Errorf("At(%d) width = %v, want %s", c.seq, r.Value, c.want)
		}
	}
	if org, err := l.At(0); err != nil || org != nil {
		t.Fatalf("At(0) = %v, %v; want no Org", org, err)
	}
}

func TestVersionsBumpOnlyForAPsWhoseConfigChanged(t *testing.T) {
	l := open(t)
	commitAll(t, l, scenario())
	start := versions(l)

	e := commit(t, l, setOp(change.Locations, "house", "radio.5g.width", "20"))
	wantVersions(t, l, start, map[hierarchy.NodeID]int64{"office-ap": e.Seq})

	// A break moves where values come from but changes no value (0005).
	after := versions(l)
	commit(t, l, change.Op{Kind: change.BreakHierarchy, Tree: change.Locations, Node: "gate"})
	wantVersions(t, l, after, nil)

	e = commit(t, l, setOp(change.Locations, "gate", "system.poll", "300"))
	wantVersions(t, l, after, map[hierarchy.NodeID]int64{"gate-ap": e.Seq})

	// A Services change reaches only APs whose service folders see it. No AP
	// is assigned Household › Gate yet; Pumphouse offers IoT only.
	after = versions(l)
	commit(t, l, setOp(change.Services, "household-gate", "network.sweet.segment", "vxlan vx21"))
	wantVersions(t, l, after, nil)
	e = commit(t, l, setOp(change.Services, "household", "network.sweet.ssid", "Sweet Spot 2"))
	wantVersions(t, l, after, map[hierarchy.NodeID]int64{"office-ap": e.Seq, "gate-ap": e.Seq})
}

func TestNewAPGetsAVersion(t *testing.T) {
	l := open(t)
	commitAll(t, l, scenario())
	e := commit(t, l, change.Op{Kind: change.AddAP, Tree: change.Locations, Node: "barn-ap", Name: "BarnAP", Parent: "house"})
	if v, ok := l.Version("barn-ap"); !ok || v != e.Seq {
		t.Fatalf("barn-ap version = %d, %v; want %d", v, ok, e.Seq)
	}
}

func TestRefusedChangesLeaveNoTrace(t *testing.T) {
	l := open(t)
	commitAll(t, l, scenario())
	seq := l.Seq()

	var le *hierarchy.LockedError
	if _, err := l.Commit("claude", "", setOp(change.Locations, "house", "system.poll", "30")); !errors.As(err, &le) {
		t.Fatalf("set under lock: got %v", err)
	}
	var be *APBreakError
	conflict := change.Op{Kind: change.AssignServices, Node: "pump", Services: []hierarchy.NodeID{"household", "household-gate"}}
	if _, err := l.Commit("claude", "", conflict); !errors.As(err, &be) || be.AP != "pump-ap" {
		t.Fatalf("conflicting services: got %v, want APBreakError for pump-ap", err)
	}
	if _, err := l.Commit("", "", setOp(change.Locations, "house", "system.tz", "UTC")); !errors.Is(err, ErrNoActor) {
		t.Fatalf("no actor: got %v", err)
	}

	if l.Seq() != seq {
		t.Fatalf("seq moved from %d to %d", seq, l.Seq())
	}
	entries, err := l.Entries(seq, 0)
	must(t, err)
	if len(entries) != 0 {
		t.Fatalf("refused changes were logged: %v", entries)
	}
	cfg, err := l.Snapshot().ResolveAP("pump-ap")
	must(t, err)
	if !reflect.DeepEqual(cfg.Services, []hierarchy.NodeID{"iot"}) {
		t.Fatalf("pump services = %v after a refused change", cfg.Services)
	}
}

func TestEntriesRecordWhoWhenWhyAndEffect(t *testing.T) {
	l := open(t)
	commitAll(t, l, scenario())
	commit(t, l, setOp(change.Locations, "house", "system.tz", "UTC"))
	commit(t, l, setOp(change.Locations, "symtus", "system.tz", "America/Chicago"))
	lock, err := l.Commit("griff", "one timezone for the property", change.Op{Kind: change.Lock, Tree: change.Locations, Node: "symtus", Path: "system.tz"})
	must(t, err)

	entries, err := l.Entries(lock.Seq-1, 1)
	must(t, err)
	if len(entries) != 1 {
		t.Fatalf("got %d entries", len(entries))
	}
	got := entries[0]
	if got.Actor != "griff" || got.Reason != "one timezone for the property" || !got.At.Equal(lock.At) {
		t.Fatalf("entry = %+v", got)
	}
	if !reflect.DeepEqual(got.Effect.Removed, []hierarchy.Override{{Node: "house", Path: "system.tz", Value: "UTC"}}) {
		t.Fatalf("removed = %v", got.Effect.Removed)
	}
	if got.Op.Kind != change.Lock || got.Op.Path != "system.tz" {
		t.Fatalf("op = %+v", got.Op)
	}
}

func TestFirstChangeMustCreateTheOrg(t *testing.T) {
	l := open(t)
	if _, err := l.Commit("claude", "", setOp(change.Locations, "symtus", "a", "b")); !errors.Is(err, change.ErrNoOrg) {
		t.Fatalf("got %v, want ErrNoOrg", err)
	}
	if l.Snapshot() != nil {
		t.Fatal("an Org exists after a refused first change")
	}
}

func TestDSN(t *testing.T) {
	const q = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(wal)&_txlock=immediate"
	for in, want := range map[string]string{
		"/var/lib/aeolus/aeolus.db": "file:/var/lib/aeolus/aeolus.db" + q,
		"/tmp/a b/c#d.db":           "file:/tmp/a%20b/c%23d.db" + q,
	} {
		if got := dsn(in); got != want {
			t.Errorf("dsn(%q) = %q, want %q", in, got, want)
		}
	}
}

// helpers

func open(t *testing.T) *Log {
	t.Helper()
	return openAt(t, filepath.Join(t.TempDir(), "aeolus.db"))
}

func openAt(t *testing.T, path string) *Log {
	t.Helper()
	clock := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	l, err := Open(path, func() time.Time { clock = clock.Add(time.Second); return clock })
	must(t, err)
	t.Cleanup(func() { l.Close() })
	return l
}

func commit(t *testing.T, l *Log, op change.Op) Entry {
	t.Helper()
	e, err := l.Commit("claude", "test", op)
	if err != nil {
		t.Fatalf("%+v: %v", op, err)
	}
	return e
}

func commitAll(t *testing.T, l *Log, ops []change.Op) {
	t.Helper()
	for _, op := range ops {
		commit(t, l, op)
	}
}

func setOp(tree change.TreeName, node, path string, v any) change.Op {
	return change.Op{Kind: change.Set, Tree: tree, Node: hierarchy.NodeID(node), Path: hierarchy.Path(path), Value: js(v)}
}

func js(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func resolved(t *testing.T, o *hierarchy.Org) map[hierarchy.NodeID]hierarchy.APConfig {
	t.Helper()
	out := map[hierarchy.NodeID]hierarchy.APConfig{}
	for _, ap := range o.Locations.APs() {
		cfg, err := o.ResolveAP(ap)
		must(t, err)
		out[ap] = cfg
	}
	return out
}

func versions(l *Log) map[hierarchy.NodeID]int64 {
	out := map[hierarchy.NodeID]int64{}
	for _, ap := range l.Snapshot().Locations.APs() {
		out[ap], _ = l.Version(ap)
	}
	return out
}

// wantVersions checks that only the APs in bumped changed version, to the
// given numbers, relative to before.
func wantVersions(t *testing.T, l *Log, before, bumped map[hierarchy.NodeID]int64) {
	t.Helper()
	want := map[hierarchy.NodeID]int64{}
	for ap, v := range before {
		want[ap] = v
	}
	for ap, v := range bumped {
		want[ap] = v
	}
	if got := versions(l); !reflect.DeepEqual(got, want) {
		t.Fatalf("versions = %v, want %v", got, want)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
