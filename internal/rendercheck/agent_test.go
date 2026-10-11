package rendercheck

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
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
	Facts   struct {
		AP string `json:"ap"` // the AP it renders for, whose segment MACs are checked (0060)
	} `json:"facts"`
	// The renderer's errors, which make the agent refuse the config: with
	// "errors" in a case, exactly these, in order; without, they are not
	// checked.
	Errors *[]string `json:"errors"`
	// Refused: the renderer held back what the AP cannot run yet, and the
	// check must refuse what it renders, so the AP keeps what runs (0095).
	Refused bool `json:"refused"`
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
		p := CheckAP(c.Intent, cfg, c.Facts.AP)
		if c.Refused && len(p) == 0 {
			t.Errorf("%s: the check passes what the agent held back", c.name)
		}
		if !c.Refused && len(p) > 0 {
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

// A radio another service owns, airscan's scan radio, comes out exactly as
// it went in: no radio settings and no network on it (0081).
func TestAgentLeavesReservedRadiosAlone(t *testing.T) {
	seen := 0
	for _, c := range agentCases(t) {
		raw, err := os.ReadFile(filepath.Join(agentDir, "test", c.Current))
		if err != nil {
			t.Fatal(err)
		}
		var current map[string]map[string]map[string]any
		if err := json.Unmarshal(raw, &current); err != nil {
			t.Fatal(err)
		}
		cfg, err := uci.Parse(c.golden)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		w := cfg.Package("wireless")
		for name, s := range current["wireless"] {
			if s[".type"] != "wifi-device" || s["airscan"] != "1" {
				continue
			}
			seen++
			got := w.Named(name)
			if got == nil {
				t.Errorf("%s: reserved radio %s was removed", c.name, name)
				continue
			}
			for k, v := range s {
				if strings.HasPrefix(k, ".") {
					continue
				}
				if have, _ := got.Option(k); have != v {
					t.Errorf("%s: reserved radio %s: %s is %q, want %v", c.name, name, k, have, v)
				}
			}
			for _, k := range got.Names() {
				if _, ok := s[k]; !ok {
					t.Errorf("%s: reserved radio %s gained %s", c.name, name, k)
				}
			}
			for _, iface := range w.OfType("wifi-iface") {
				if dev, _ := iface.Option("device"); dev == name && strings.HasPrefix(iface.Name, "aeolus_") {
					t.Errorf("%s: %s puts a network on reserved radio %s", c.name, iface.Name, name)
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("no test case has a reserved radio")
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
	// A bond's members, which take its place when Aeolus takes it apart, and
	// give it back (0096): the bond the AP has, or the one Aeolus keeps.
	bondOf := map[string]string{}
	for _, s := range current["network"] {
		if s["type"] == "bonding" {
			for _, m := range anyList(s["ports"]) {
				bondOf[m] = s["name"].(string)
			}
		}
	}
	if s := current["aeolus"][Unbond]; s != nil {
		for _, m := range anyList(s["ports"]) {
			bondOf[m] = s["name"].(string)
		}
	}
	others := func(entries any) any {
		list, ok := entries.([]any)
		if !ok {
			return entries
		}
		out := []any{}
		seen := map[string]bool{}
		for _, e := range list {
			port, flags, has := strings.Cut(e.(string), ":")
			if VLANEnd.MatchString(port) {
				continue // a network's veth, which Aeolus puts in the bridge (0061)
			}
			if b, ok := bondOf[port]; ok {
				port, e = b, b
				if has {
					e = b + ":" + flags
				}
			}
			if seen[e.(string)] {
				continue
			}
			seen[e.(string)] = true
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

// anyList is a UCI list as JSON has it, or one value as a list of one.
func anyList(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		out := []string{}
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// In ucode, a function can only name a top-level function declared above it:
// a call to one declared below compiles, and fails only when it runs, with
// "access to undeclared variable" (v0.28.0's agent, on every start). The
// agent's own code is not run in CI, so each call is checked against the
// order of declarations here instead.
func TestAgentCallsOnlyWhatIsDeclaredAbove(t *testing.T) {
	decl := regexp.MustCompile(`^(?:export\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	call := regexp.MustCompile(`(^|[^A-Za-z0-9_.$])([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	files := []string{"files/usr/sbin/aeolus-agent", "files/usr/sbin/aeolus-prober", "files/usr/sbin/aeolus-rrm", "files/usr/sbin/aeolus-journey", "files/usr/libexec/aeolus-gi"}
	mods, _ := filepath.Glob(filepath.Join(agentDir, "files", "usr", "share", "ucode", "aeolus", "*.uc"))
	for _, m := range mods {
		rel, _ := filepath.Rel(agentDir, m)
		files = append(files, filepath.ToSlash(rel))
	}
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(agentDir, f))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(raw), "\n")
		at := map[string]int{} // a top-level function -> the line it is declared on
		for i, l := range lines {
			if m := decl.FindStringSubmatch(l); m != nil {
				at[m[1]] = i
			}
		}
		within := "" // the top-level function the line is in
		for i, l := range lines {
			if m := decl.FindStringSubmatch(l); m != nil {
				within = m[1]
			} else if !strings.HasPrefix(l, "\t") && !strings.HasPrefix(l, "}") && strings.TrimSpace(l) != "" {
				within = "" // top-level code runs after the declarations above it
			}
			if within == "" {
				continue
			}
			code, _, _ := strings.Cut(l, "//")
			for _, m := range call.FindAllStringSubmatch(code, -1) {
				if d, ok := at[m[2]]; ok && d > i && m[2] != within {
					t.Errorf("%s:%d: %s calls %s, which is declared below it (line %d), so the call fails when it runs", f, i+1, within, m[2], d+1)
				}
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
		if c.Errors != nil {
			var got []string
			for _, line := range strings.Split(stderr.String(), "\n") {
				if e, ok := strings.CutPrefix(line, "render: "); ok {
					got = append(got, e)
				}
			}
			if strings.Join(got, "\n") != strings.Join(*c.Errors, "\n") {
				t.Errorf("%s: the renderer's errors are\n%s\nwant\n%s", c.name, strings.Join(got, "\n"), strings.Join(*c.Errors, "\n"))
			}
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

// The prober's frames, filters and verdicts (0059), as the agent's own ucode
// builds and reads them: agent/test/probe.uc prints them, and probe.out is
// what it must print. Its checksums were also worked out apart from ucode.
func TestAgentProbeFrames(t *testing.T) {
	ucode, err := exec.LookPath("ucode")
	if err != nil {
		t.Skip("ucode is not installed; CI builds it (0040)")
	}
	modules, err := filepath.Abs(filepath.Join(agentDir, "files", "usr", "share", "ucode", "*.uc"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(agentDir, "test", "probe.out"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(ucode, "-L", modules, filepath.Join(agentDir, "test", "probe.uc"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	if string(out) != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("the prober's frames are not what probe.out says:\n%s", out)
	}
}

// Radio resource management's pure parts (0073), as the agent's own ucode
// works them out: agent/test/rrm.uc prints them, and rrm.out is what it must
// print. Its HMACs are RFC 4231's, or worked out with Python's hmac.
func TestAgentRRM(t *testing.T) {
	ucode, err := exec.LookPath("ucode")
	if err != nil {
		t.Skip("ucode is not installed; CI builds it (0040)")
	}
	modules, err := filepath.Abs(filepath.Join(agentDir, "files", "usr", "share", "ucode", "*.uc"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(agentDir, "test", "rrm.out"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(ucode, "-L", modules, filepath.Join(agentDir, "test", "rrm.uc"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	if string(out) != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("radio resource management works out other than rrm.out says:\n%s", out)
	}
}

// The prober's RADIUS status request, how its answers read, and how hostapd's
// counts are read (0114), as the agent's own ucode works them out:
// agent/test/radius.uc prints them, and radius.out is what it must print.
// Its HMACs are RFC 2202's, and the request's was worked out with Python's
// hmac too.
func TestAgentRadius(t *testing.T) {
	ucode, err := exec.LookPath("ucode")
	if err != nil {
		t.Skip("ucode is not installed; CI builds it (0040)")
	}
	modules, err := filepath.Abs(filepath.Join(agentDir, "files", "usr", "share", "ucode", "*.uc"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(agentDir, "test", "radius.out"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(ucode, "-L", modules, filepath.Join(agentDir, "test", "radius.uc"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	if string(out) != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("the prober's RADIUS status works out other than radius.out says:\n%s", out)
	}
}

// The check catches a bond taken apart badly (0096): a VLAN the second port
// does not carry, which the AP would lose when that port is the path out,
// and a bridge without spanning tree.
func TestUnbondCheckCatchesALostVLAN(t *testing.T) {
	for _, c := range agentCases(t) {
		if c.name != "unbond" {
			continue
		}
		for bad, want := range map[string]string{
			strings.Replace(c.golden, "\tlist ports 'eth0:t'\n\tlist ports 'eth1:t'\n", "\tlist ports 'eth0:t'\n", 1): "VLAN 20 is not on eth1 as on eth0",
			strings.Replace(c.golden, "\toption stp '1'\n", "", 1):                                                    "spanning tree is off",
		} {
			cfg, err := uci.Parse(bad)
			if err != nil {
				t.Fatal(err)
			}
			if p := strings.Join(CheckAP(c.Intent, cfg, c.Facts.AP), "\n"); !strings.Contains(p, want) {
				t.Errorf("the check missed %q:\n%s", want, p)
			}
		}
		return
	}
	t.Fatal("no unbond case")
}

// A top-level function named as one of ucode's own hides it for the whole
// file: a call meant for ucode's then runs the file's, with no error. In
// aeolus/journey.uc a packet filter named filter made record count the
// filter's steps for the client's sign-in steps, and a client that left
// while associating was said to have stopped at its sign-in (v0.69.0).
func TestAgentHidesNoBuiltin(t *testing.T) {
	builtins := map[string]bool{}
	for _, b := range strings.Fields(`print printf sprintf length index rindex substr split join keys values map filter sort reverse
		push pop shift unshift splice slice uniq exists type int die ord chr hex uc lc trim ltrim rtrim replace match json include render
		warn system trace proto sleep assert regexp wildcard sourcepath min max b64dec b64enc uchr time localtime gmtime timelocal timegm
		clock hexdec hexenc gc loadstring loadfile call signal require iptoarr arrtoip getenv exit`) {
		builtins[b] = true
	}
	decl := regexp.MustCompile(`^(?:export\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	var files []string
	for _, pattern := range []string{"files/usr/sbin/aeolus-*", "files/usr/libexec/aeolus-*", "files/usr/share/ucode/aeolus/*.uc"} {
		m, _ := filepath.Glob(filepath.Join(agentDir, filepath.FromSlash(pattern)))
		files = append(files, m...)
	}
	if len(files) < 8 {
		t.Fatalf("only %d agent files found", len(files))
	}
	// The renderer's system and render are the settings it renders and the
	// module's own entry: it runs no command and expands no template, so it
	// calls neither of ucode's.
	known := map[string]bool{"render.uc system": true, "render.uc render": true}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, l := range strings.Split(string(raw), "\n") {
			if m := decl.FindStringSubmatch(l); m != nil && builtins[m[1]] && !known[filepath.Base(f)+" "+m[1]] {
				t.Errorf("%s:%d: function %s hides ucode's own %s in this file", filepath.Base(f), i+1, m[1], m[1])
			}
		}
	}
}

// A client coming online, as the AP records it (0118): what hostapd's log
// lines mean, what the packet filter keeps and how each frame reads, and
// the record made of an attempt. agent/test/journey.uc prints them, with
// frames it makes itself and a small interpreter of the kernel's filter;
// journey.out is what it must print.
func TestAgentJourney(t *testing.T) {
	ucode, err := exec.LookPath("ucode")
	if err != nil {
		t.Skip("ucode is not installed; CI builds it (0040)")
	}
	modules, err := filepath.Abs(filepath.Join(agentDir, "files", "usr", "share", "ucode", "*.uc"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(agentDir, "test", "journey.out"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(ucode, "-L", modules, filepath.Join(agentDir, "test", "journey.uc"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	if string(out) != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("a client coming online is recorded other than journey.out says:\n%s", out)
	}
}

// The check catches passphrases from the RADIUS server rendered badly
// (0115): ppsk left off, so hostapd would take the network's own passphrase
// from anyone; a key written beside it; and ppsk on a network that takes
// none from a server, where it would refuse every device.
func TestPassphrasesFromRADIUSCheck(t *testing.T) {
	seen := 0
	for _, c := range agentCases(t) {
		var bad map[string]string
		switch c.name {
		case "radius-passphrases":
			bad = map[string]string{
				strings.Replace(c.golden, "\toption ppsk '1'\n", "\toption key 'fixture-passphrase-1'\n", 1): "ppsk is \"\", want 1",
				strings.Replace(c.golden, "\toption ppsk '1'\n", "\toption ppsk '1'\n\toption key 'x'\n", 1): "a key is set, but each device's passphrase is the RADIUS server's",
			}
		case "mac-auth":
			bad = map[string]string{
				strings.Replace(c.golden, "\toption key 'fixture-passphrase-1'\n", "\toption key 'fixture-passphrase-1'\n\toption ppsk '1'\n", 1): "the network takes no passphrases from a RADIUS server",
			}
		default:
			continue
		}
		seen++
		for uciText, want := range bad {
			if uciText == c.golden {
				t.Fatalf("%s: nothing was changed for %q", c.name, want)
			}
			cfg, err := uci.Parse(uciText)
			if err != nil {
				t.Fatal(err)
			}
			if p := strings.Join(CheckAP(c.Intent, cfg, c.Facts.AP), "\n"); !strings.Contains(p, want) {
				t.Errorf("%s: the check missed %q:\n%s", c.name, want, p)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("%d of the 2 cases found", seen)
	}
}

// A module imported whole (import * as name) is hidden, in a function, by a
// local of the same name: a call through it compiles, and fails only when it
// runs, with "left-hand side expression is not an array or object" (v0.57.0's
// agent, on every state report, where ports_state had a local uplink).
func TestAgentHidesNoModule(t *testing.T) {
	imp := regexp.MustCompile(`^import \* as ([A-Za-z_][A-Za-z0-9_]*) from`)
	files := []string{"files/usr/sbin/aeolus-agent", "files/usr/sbin/aeolus-prober", "files/usr/sbin/aeolus-rrm", "files/usr/sbin/aeolus-journey", "files/usr/libexec/aeolus-gi"}
	mods, _ := filepath.Glob(filepath.Join(agentDir, "files", "usr", "share", "ucode", "aeolus", "*.uc"))
	for _, m := range mods {
		rel, _ := filepath.Rel(agentDir, m)
		files = append(files, filepath.ToSlash(rel))
	}
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(agentDir, f))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(raw), "\n")
		for _, l := range lines {
			m := imp.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			local := regexp.MustCompile(`(?:\blet|\bconst|\bfor\s*\(\s*let)\s+(?:[A-Za-z_][A-Za-z0-9_]*\s*,\s*)?` + m[1] + `\b|function\s*[A-Za-z0-9_]*\s*\([^)]*\b` + m[1] + `\b[^)]*\)|\(\s*` + m[1] + `\s*\)\s*=>|\b` + m[1] + `\s*=>`)
			for i, l := range lines {
				if code, _, _ := strings.Cut(l, "//"); local.MatchString(code) {
					t.Errorf("%s:%d: a local %s hides the module imported as %s", f, i+1, m[1], m[1])
				}
			}
		}
	}
}
