package rendercheck

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/uci"
)

// The contract between the agent and the manager (0040). Each case in
// agent/test/cases holds an intent, the AP config it starts from and, beside
// it, the UCI the agent renders (<case>.uci). That UCI must pass the render
// check and leave alone every section Aeolus does not own; and the agent must
// render exactly it, which is checked wherever ucode is installed (CI builds
// it).

const agentDir = "../../agent"

type agentCase struct {
	name    string
	Intent  map[string]any `json:"intent"`
	Current string         `json:"current"`
	Stale   []string       `json:"stale"`
	Kept    []string       `json:"kept"` // package.section
	Gone    []string       `json:"gone"`
	golden  string
}

func agentCases(t *testing.T) []agentCase {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(agentDir, "test", "cases", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no agent test cases: %v", err)
	}
	var out []agentCase
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		c := agentCase{name: strings.TrimSuffix(filepath.Base(p), ".json")}
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		golden, err := os.ReadFile(strings.TrimSuffix(p, ".json") + ".uci")
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		c.golden = string(golden)
		out = append(out, c)
	}
	return out
}

func TestAgentOutputPassesTheCheck(t *testing.T) {
	for _, c := range agentCases(t) {
		cfg, err := uci.Parse(c.golden)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if p := Check(c.Intent, cfg); len(p) > 0 {
			t.Errorf("%s: the check refuses the agent's output:\n%s", c.name, strings.Join(p, "\n"))
		}
		untouched(t, c, cfg)
		for _, name := range c.Stale {
			if cfg.Package("wireless").Named(name) != nil {
				t.Errorf("%s: stale %s was kept", c.name, name)
			}
		}
		for _, ref := range c.Kept {
			if pkg, name, _ := strings.Cut(ref, "."); cfg.Package(pkg).Named(name) == nil {
				t.Errorf("%s: %s was removed", c.name, ref)
			}
		}
		for _, ref := range c.Gone {
			if pkg, name, _ := strings.Cut(ref, "."); cfg.Package(pkg).Named(name) != nil {
				t.Errorf("%s: %s was kept", c.name, ref)
			}
		}
	}
}

// untouched checks that every named wifi-iface, interface and bridge-vlan
// Aeolus does not own comes out exactly as it went in (0040), but for the
// bridge-vlan entries of the ports the intent sets the VLANs of (0053).
func untouched(t *testing.T, c agentCase, cfg *uci.Config) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(agentDir, "test", c.Current))
	if err != nil {
		t.Fatal(err)
	}
	var current map[string]map[string]map[string]any
	if err := json.Unmarshal(raw, &current); err != nil {
		t.Fatal(err)
	}
	kept := map[string]bool{"wifi-iface": true, "interface": true, "bridge-vlan": true}
	ports, _ := c.Intent["ports"].(map[string]any)
	others := func(entries any) any {
		list, ok := entries.([]any)
		if !ok {
			return entries
		}
		out := []any{}
		for _, e := range list {
			port, _, _ := strings.Cut(e.(string), ":")
			if set, _ := ports[port].(map[string]any); set["mode"] != "access" && set["mode"] != "trunk" && set["mode"] != "tunnel" {
				out = append(out, e)
			}
		}
		return out
	}
	for pkg, sections := range current {
		for name, s := range sections {
			if strings.HasPrefix(name, "aeolus_") || s[".anonymous"] == true || !kept[s[".type"].(string)] {
				continue
			}
			got := cfg.Package(pkg).Named(name)
			if got == nil {
				t.Errorf("%s: %s.%s was removed", c.name, pkg, name)
				continue
			}
			want := map[string]any{}
			for k, v := range s {
				if !strings.HasPrefix(k, ".") {
					want[k] = v
				}
			}
			have := map[string]any{}
			for _, k := range got.Names() {
				if v, ok := got.Option(k); ok {
					have[k] = v
				} else {
					list := []any{}
					for _, e := range got.List(k) {
						list = append(list, e)
					}
					have[k] = list
				}
			}
			if s[".type"] == "bridge-vlan" {
				have["ports"], want["ports"] = others(have["ports"]), others(want["ports"])
			}
			if !reflect.DeepEqual(have, want) {
				t.Errorf("%s: %s.%s changed:\n got %v\nwant %v", c.name, pkg, name, have, want)
			}
		}
	}
}

func TestAgentRendersItsOutput(t *testing.T) {
	ucode, err := exec.LookPath("ucode")
	if err != nil {
		t.Skip("ucode is not installed; CI builds it (0040)")
	}
	// Absolute, because ucode resolves a relative search path from the
	// directory of the file doing the import, which breaks the agent's own
	// imports between its modules.
	modules, err := filepath.Abs(filepath.Join(agentDir, "files", "usr", "share", "ucode", "*.uc"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range agentCases(t) {
		cmd := exec.Command(ucode, "-L", modules, filepath.Join(agentDir, "test", "render.uc"),
			filepath.Join(agentDir, "test", "cases", c.name+".json"))
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v\n%s", c.name, err, stderr.String())
		}
		if string(out) != c.golden {
			t.Errorf("%s: the agent renders something else than %s.uci:\n%s", c.name, c.name, out)
		}
		// The MSS clamp's file, made from the rendered tunnels (0054).
		want, err := os.ReadFile(filepath.Join(agentDir, "test", "cases", c.name+".nft"))
		if err != nil {
			continue
		}
		cmd = exec.Command(ucode, "-L", modules, filepath.Join(agentDir, "test", "render.uc"),
			filepath.Join(agentDir, "test", "cases", c.name+".json"), "clamp")
		if out, err = cmd.Output(); err != nil {
			t.Fatalf("%s clamp: %v", c.name, err)
		}
		if string(out) != string(want) {
			t.Errorf("%s: the agent makes another clamp than %s.nft:\n%s", c.name, c.name, out)
		}
	}
}
