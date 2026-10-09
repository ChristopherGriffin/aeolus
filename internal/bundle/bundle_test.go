package bundle

import (
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ChristopherGriffin/aeolus/agent"
)

func files(agentText string) fstest.MapFS {
	return fstest.MapFS{
		"files/usr/sbin/aeolus-agent":            {Data: []byte(agentText)},
		"files/usr/share/ucode/aeolus/render.uc": {Data: []byte("export function render() {}\n")},
		"files/etc/init.d/aeolus":                {Data: []byte("#!/bin/sh /etc/rc.common\n")},
	}
}

func TestFromFS(t *testing.T) {
	b, contents, err := FromFS(files("one"), "files", "v1.0.0-aaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Files) != 3 || b.Files[0].Path != "/etc/init.d/aeolus" || b.Files[0].Mode != "0755" {
		t.Fatalf("files %+v", b.Files)
	}
	if m := b.Files[1]; m.Path != "/usr/sbin/aeolus-agent" || m.Mode != "0755" || string(contents[m.SHA256]) != "one" {
		t.Fatalf("the agent %+v", m)
	}
	if b.Files[2].Mode != "0644" {
		t.Fatalf("a module is not a program: %+v", b.Files[2])
	}
	// The release isn't in the hash; the files are.
	same, _, _ := FromFS(files("one"), "files", "v1.0.1-bbbbbbb")
	other, _, _ := FromFS(files("two"), "files", "v1.0.0-aaaaaaa")
	if same.Hash != b.Hash || other.Hash == b.Hash || len(b.Hash) != 64 {
		t.Fatalf("hashes %s %s %s", b.Hash, same.Hash, other.Hash)
	}
	// A file outside the agent's places is refused.
	bad := files("one")
	bad["files/etc/shadow"] = &fstest.MapFile{Data: []byte("x")}
	if _, _, err := FromFS(bad, "files", "v1"); err == nil {
		t.Fatal("/etc/shadow taken for an agent file")
	}
}

// The agent the manager is built with makes a bundle, and every file in it
// is one the AP takes.
func TestTheAgentIsABundle(t *testing.T) {
	b, _, err := FromFS(agent.Files, "files", "dev")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"/usr/sbin/aeolus-agent": "0755", "/usr/libexec/aeolus-rollback": "0755", "/lib/upgrade/keep.d/aeolus": "0644"}
	for _, f := range b.Files {
		if m, ok := want[f.Path]; ok {
			if f.Mode != m {
				t.Errorf("%s: mode %s, want %s", f.Path, f.Mode, m)
			}
			delete(want, f.Path)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing from the bundle: %v", want)
	}
}

func TestPathOK(t *testing.T) {
	for p, ok := range map[string]bool{
		"/usr/sbin/aeolus-agent": true, "/usr/share/ucode/aeolus/rrm.uc": true, "/etc/hotplug.d/ntp/50-aeolus": true,
		"/lib/upgrade/keep.d/aeolus": true, "/etc/shadow": false, "/usr/sbin/../../etc/shadow": false,
		"/usr/share/ucode/uci.so": false, "/usr/sbin/a b": false,
	} {
		if PathOK(p) != ok {
			t.Errorf("PathOK(%q) = %v", p, !ok)
		}
	}
}

// LuCI's Aeolus page (0083) is a bundle of its own places, which an agent
// bundle may not hold but for its keep.d list, and the other way round.
func TestLuCIPage(t *testing.T) {
	b, _, err := FromFSChecked(agent.LuCI, "luci", "dev", LuCIPathOK)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Files) != 4 {
		t.Fatalf("LuCI's page: %+v", b.Files)
	}
	for _, f := range b.Files {
		if f.Mode != "0644" || PathOK(f.Path) && !strings.HasPrefix(f.Path, "/lib/upgrade/keep.d/") {
			t.Errorf("%s: mode %s, an agent place %v", f.Path, f.Mode, PathOK(f.Path))
		}
	}
	for p, ok := range map[string]bool{
		"/www/luci-static/resources/view/aeolus/enroll.js": true, "/usr/share/luci/menu.d/luci-app-aeolus.json": true,
		"/usr/share/rpcd/acl.d/luci-app-aeolus.json": true, "/lib/upgrade/keep.d/luci-app-aeolus": true,
		"/usr/share/rpcd/acl.d/luci-base.json": false, "/www/luci-static/resources/view/aeolus/../x.js": false,
		"/www/cgi-bin/luci": false, "/usr/sbin/aeolus-agent": false,
	} {
		if LuCIPathOK(p) != ok {
			t.Errorf("LuCIPathOK(%q) = %v", p, !ok)
		}
	}
	if _, _, err := FromFS(agent.LuCI, "luci", "dev"); err == nil {
		t.Fatal("LuCI's page taken for agent files")
	}
}

func TestStore(t *testing.T) {
	s, err := Open(t.TempDir(), 2)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	put := func(text, version string, at time.Time) Bundle {
		t.Helper()
		b, contents, err := FromFS(files(text), "files", version)
		if err != nil {
			t.Fatal(err)
		}
		kept, err := s.Put(b, contents, at)
		if err != nil {
			t.Fatal(err)
		}
		return kept
	}
	one := put("one", "v1.0.0-aaaaaaa", t0)
	// The same release again keeps the time it was first kept.
	if again := put("one", "v1.0.0-aaaaaaa", t0.Add(time.Hour)); !again.Stored.Equal(t0) {
		t.Fatalf("stored %v, want %v", again.Stored, t0)
	}
	two := put("two", "v1.1.0-bbbbbbb", t0.Add(2*time.Hour))
	if b, ok := s.Find("v1.0.0"); !ok || b.Hash != one.Hash {
		t.Fatalf("by tag: %v %v", b, ok)
	}
	if b, ok := s.Find("v1.1.0-bbbbbbb"); !ok || b.Hash != two.Hash {
		t.Fatalf("by release: %v %v", b, ok)
	}
	if _, ok := s.Find("v1.1"); ok {
		t.Fatal("v1.1 took for a tag of v1.1.0")
	}
	data, err := s.File(one.Files[1].SHA256)
	if err != nil || string(data) != "one" {
		t.Fatalf("file %q %v", data, err)
	}
	if _, err := s.File("../manifests/x"); err == nil {
		t.Fatal("a path read as a file's hash")
	}
	// A third release: the oldest goes, and the files only it had.
	put("three", "v1.2.0-ccccccc", t0.Add(3*time.Hour))
	vs, _ := s.Versions()
	if len(vs) != 2 || vs[0].Version != "v1.2.0-ccccccc" || vs[1].Version != "v1.1.0-bbbbbbb" {
		t.Fatalf("kept %v", vs)
	}
	if _, ok := s.Find("v1.0.0"); ok {
		t.Fatal("the oldest release kept")
	}
	if _, err := s.File(one.Files[1].SHA256); err == nil {
		t.Fatal("the oldest release's own file kept")
	}
	if _, err := s.File(one.Files[0].SHA256); err != nil {
		t.Fatal("a file the kept releases share dropped")
	}
}
