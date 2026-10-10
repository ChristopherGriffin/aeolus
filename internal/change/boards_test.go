package change

import (
	"strings"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// A kind of AP's own settings (0092) replace the plain ones set on the same
// node or above, for its APs alone; a plain one set closer to the AP, or
// locked above, stays; and none is left in an AP's settings as it is.
func TestBoardSettings(t *testing.T) {
	s := templateOrg(t)
	set := func(node hierarchy.NodeID, p hierarchy.Path, v string) {
		t.Helper()
		mustApply(t, s, Op{Kind: Set, Tree: Locations, Node: node, Path: p, Value: raw(v)})
	}
	value := func(ap hierarchy.NodeID, p hierarchy.Path) any {
		t.Helper()
		return resolved(t, s, ap, p).Value
	}
	c360 := BoardPath("arista,c360", "ports.eth1.mode")
	set("house", "ports.eth1.mode", `"access"`)
	set("house", "ports.eth1.untagged", `30`)
	set("house", c360, `"trunk"`)
	set("house", BoardPath("netgear,r7800", "ports.eth1.mode"), `"tunnel"`)
	if got := value("c360", "ports.eth1.mode"); got != "trunk" {
		t.Errorf("the C-360's own, on the same folder: %v", got)
	}
	if r := resolved(t, s, "c360", "ports.eth1.mode"); r.From != "house" {
		t.Errorf("it comes from %s", r.From)
	}
	if got := value("c360", "ports.eth1.untagged"); got != float64(30) {
		t.Errorf("the plain field it doesn't replace: %v", got)
	}
	if got := value("other", "ports.eth1.mode"); got != "access" {
		t.Errorf("an AP with no board takes the plain one: %v", got)
	}
	cfg, err := s.ResolveAP("c360")
	if err != nil {
		t.Fatal(err)
	}
	for p := range cfg.Location {
		if strings.HasPrefix(string(p), BoardsPrefix) {
			t.Errorf("%s is left in the AP's settings", p)
		}
	}
	// A plain one closer to the AP stays.
	set("barn", "ports.eth1.mode", `"access"`)
	if got := value("c360", "ports.eth1.mode"); got != "access" {
		t.Errorf("a plain one closer to the AP: %v", got)
	}
	// So does a lock above.
	s = templateOrg(t)
	set("symtus", "ports.eth1.mode", `"access"`)
	mustApply(t, s, Op{Kind: Lock, Tree: Locations, Node: "symtus", Path: "ports.eth1.mode"})
	set("house", c360, `"trunk"`)
	if got := value("c360", "ports.eth1.mode"); got != "access" {
		t.Errorf("under a lock above: %v", got)
	}
}
