package change

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// templateOrg is an Org with House › Barn › a C-360, and another AP of an
// unknown board in House, both adopted.
func templateOrg(t *testing.T) *State {
	t.Helper()
	s := org(t)
	mustApply(t, s, Op{Kind: AddFolder, Tree: Locations, Node: "barn", Name: "Barn", Parent: "house"})
	mustApply(t, s, Op{Kind: AddAP, Tree: Locations, Node: "c360", Name: "C360", Parent: "barn"})
	mustApply(t, s, Op{Kind: AddAP, Tree: Locations, Node: "other", Name: "Other", Parent: "house"})
	s.Facts["c360"] = json.RawMessage(`{"board":"arista,c360","model":"Arista C-360"}`)
	s.Facts["other"] = json.RawMessage(`{"model":"No board"}`)
	return s
}

func raw(v string) json.RawMessage { return json.RawMessage(v) }

func resolved(t *testing.T, s *State, ap hierarchy.NodeID, p hierarchy.Path) hierarchy.Resolved {
	t.Helper()
	cfg, err := s.ResolveAP(ap)
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Location[p]
}

// A template's values count as set at the node that picks it, ahead of
// that node's own; what is set below it, or locked, replaces them.
func TestTemplatePrecedence(t *testing.T) {
	s := templateOrg(t)
	mustApply(t, s, Op{Kind: AddTemplate, Template: "c360", Name: "C-360", Parent: "symtus", Boards: []string{"arista,c360"}, Default: true})
	mustApply(t, s, Op{Kind: SetTemplate, Template: "c360", Values: map[hierarchy.Path]json.RawMessage{
		"radio.5g.width": raw(`160`), "radio.2g.channel": raw(`11`), "radio.6g.enabled": raw(`true`)}})
	mustApply(t, s, Op{Kind: Set, Tree: Locations, Node: "symtus", Path: "radio.5g.width", Value: raw(`80`)})
	mustApply(t, s, Op{Kind: Set, Tree: Locations, Node: "house", Path: "radio.2g.channel", Value: raw(`1`)})

	// The Org's own 80 is where the template is picked: the template wins.
	if r := resolved(t, s, "c360", "radio.5g.width"); r.Value != float64(160) || r.Origin != hierarchy.OriginTemplate || r.From != "symtus" {
		t.Fatalf("radio.5g.width = %+v", r)
	}
	// House is below: House wins.
	if r := resolved(t, s, "c360", "radio.2g.channel"); r.Value != float64(1) || r.From != "house" {
		t.Fatalf("radio.2g.channel = %+v", r)
	}
	// Not the AP of another board, nor one without a board.
	if r := resolved(t, s, "other", "radio.5g.width"); r.Value != float64(80) {
		t.Fatalf("other's radio.5g.width = %+v", r)
	}
	cfg, _ := s.ResolveAP("c360")
	want := []hierarchy.Override{{Node: "house", Path: "radio.2g.channel", Value: float64(1)}}
	if cfg.Template == nil || cfg.Template.ID != "c360" || cfg.Template.At != "symtus" || !reflect.DeepEqual(cfg.Template.Replaced, want) {
		t.Fatalf("template = %+v", cfg.Template)
	}
	if _, sent := cfg.Location["templates.arista,c360"]; sent {
		t.Fatal("the pick is in the AP's fields")
	}

	// A lock at the Org beats the template picked there.
	mustApply(t, s, Op{Kind: Lock, Tree: Locations, Node: "symtus", Path: "radio.5g.width"})
	if r := resolved(t, s, "c360", "radio.5g.width"); r.Value != float64(80) || r.Origin != hierarchy.OriginLocked {
		t.Fatalf("locked radio.5g.width = %+v", r)
	}
	mustApply(t, s, Op{Kind: Unlock, Tree: Locations, Node: "symtus", Path: "radio.5g.width"})

	// Barn picks its own: it takes over whole, at Barn, so House's channel,
	// above it now, loses to it.
	mustApply(t, s, Op{Kind: AddTemplate, Template: "c360-barn", Name: "C-360, barn", Parent: "barn", Boards: []string{"arista,c360"}, Default: true})
	mustApply(t, s, Op{Kind: SetTemplate, Template: "c360-barn", Path: "radio.2g.channel", Value: raw(`6`)})
	if r := resolved(t, s, "c360", "radio.2g.channel"); r.Value != float64(6) || r.Origin != hierarchy.OriginTemplate || r.From != "barn" {
		t.Fatalf("Barn's radio.2g.channel = %+v", r)
	}
	if r := resolved(t, s, "c360", "radio.5g.width"); r.Value != float64(80) || r.From != "symtus" {
		t.Fatalf("one template at a time: radio.5g.width = %+v", r)
	}
	// The AP itself, below Barn, wins over it.
	mustApply(t, s, Op{Kind: Set, Tree: Locations, Node: "c360", Path: "radio.2g.channel", Value: raw(`3`)})
	if r := resolved(t, s, "c360", "radio.2g.channel"); r.Value != float64(3) || r.Origin != hierarchy.OriginSelf {
		t.Fatalf("the AP's own radio.2g.channel = %+v", r)
	}
}

// A break copies the pick, as any setting, and the template stays offered
// below its level.
func TestTemplateAcrossABreak(t *testing.T) {
	s := templateOrg(t)
	mustApply(t, s, Op{Kind: AddTemplate, Template: "c360", Name: "C-360", Parent: "symtus", Boards: []string{"arista,c360"}, Default: true})
	mustApply(t, s, Op{Kind: SetTemplate, Template: "c360", Path: "radio.6g.enabled", Value: raw(`false`)})
	mustApply(t, s, Op{Kind: BreakHierarchy, Tree: Locations, Node: "barn"})
	if r := resolved(t, s, "c360", "radio.6g.enabled"); r.Value != false || r.Origin != hierarchy.OriginTemplate || r.From != "barn" {
		t.Fatalf("radio.6g.enabled across a break = %+v", r)
	}
}

func TestTemplateChecks(t *testing.T) {
	s := templateOrg(t)
	mustApply(t, s, Op{Kind: AddTemplate, Template: "barn-c360", Name: "Barn C-360", Parent: "barn", Boards: []string{"arista,c360"}})
	var pe *TemplatePickError
	for _, c := range []struct {
		name string
		op   Op
		want error
	}{
		{"picked above its level", Op{Kind: Set, Tree: Locations, Node: "house", Path: "templates.arista,c360", Value: raw(`"barn-c360"`)}, nil},
		{"for another board", Op{Kind: Set, Tree: Locations, Node: "barn", Path: "templates.netgear,r7800", Value: raw(`"barn-c360"`)}, nil},
		{"no such template", Op{Kind: Set, Tree: Locations, Node: "barn", Path: "templates.arista,c360", Value: raw(`"nope"`)}, nil},
		{"a tunnel", Op{Kind: SetTemplate, Template: "barn-c360", Path: "concentrators.lab.address", Value: raw(`"1.1.1.2"`)}, ErrTemplateField},
		{"the agent's release", Op{Kind: SetTemplate, Template: "barn-c360", Path: "system.agent", Value: raw(`"v1"`)}, ErrTemplateField},
		{"at an AP", Op{Kind: AddTemplate, Template: "x", Name: "X", Parent: "c360", Boards: []string{"arista,c360"}}, ErrTemplateLevel},
		{"in Landing Zone", Op{Kind: AddTemplate, Template: "x", Name: "X", Parent: LandingZone, Boards: []string{"arista,c360"}}, nil},
		{"no boards", Op{Kind: AddTemplate, Template: "x", Name: "X", Parent: "house"}, nil},
	} {
		if c.name == "in Landing Zone" {
			mustApply(t, s, Op{Kind: AddBuiltins})
		}
		_, _, err := Apply(s.Clone(), c.op)
		switch {
		case err == nil:
			t.Errorf("%s: accepted", c.name)
		case c.want != nil && !errors.Is(err, c.want):
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		case c.want == nil && c.op.Kind == Set && !errors.As(err, &pe):
			t.Errorf("%s: %v, want a TemplatePickError", c.name, err)
		}
	}
	// Moving a folder out from under a template it picks is refused.
	mustApply(t, s, Op{Kind: AddFolder, Tree: Locations, Node: "shed", Name: "Shed", Parent: "barn"})
	mustApply(t, s, Op{Kind: Set, Tree: Locations, Node: "shed", Path: "templates.arista,c360", Value: raw(`"barn-c360"`)})
	if _, _, err := Apply(s.Clone(), Op{Kind: Move, Tree: Locations, Node: "shed", Parent: "house"}); !errors.As(err, &pe) {
		t.Errorf("moving a pick away from its template: %v", err)
	}
	if _, _, err := Apply(s.Clone(), Op{Kind: RemoveTemplate, Template: "barn-c360"}); !errors.Is(err, ErrInUse) {
		t.Errorf("removing a template in use: %v", err)
	}
}

// The manager makes a template for each kind of AP adopted that none is for,
// and may make nothing else in its own name.
func TestNewKinds(t *testing.T) {
	s := templateOrg(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	mustApply(t, s, Op{Kind: AddAP, Tree: Locations, Node: "waiting", Name: "Waiting", Parent: LandingZone})
	s.Facts["waiting"] = json.RawMessage(`{"board":"netgear,r7800"}`)
	ops := NewKinds(s)
	if len(ops) != 1 || ops[0].Template != "arista-c360" || ops[0].Name != "Arista C-360" || ops[0].Parent != "symtus" || !ops[0].Default ||
		!reflect.DeepEqual(ops[0].Boards, []string{"arista,c360"}) {
		t.Fatalf("new kinds = %+v", ops)
	}
	if err := Authorize(s, SystemActor, ops[0]); err != nil {
		t.Fatalf("the manager making it: %v", err)
	}
	mustApply(t, s, ops[0])
	if ops := NewKinds(s); len(ops) != 0 {
		t.Fatalf("again: %+v", ops)
	}
	if tm, at, ok := s.TemplateFor("c360"); !ok || tm.ID != "arista-c360" || at != "symtus" || len(tm.Values) != 0 {
		t.Fatalf("the C-360 takes %+v at %s", tm, at)
	}
	for _, op := range []Op{
		{Kind: AddTemplate, Template: "again", Name: "Again", Parent: "symtus", Boards: []string{"arista,c360"}, Default: true},
		{Kind: AddTemplate, Template: "elsewhere", Name: "Elsewhere", Parent: "house", Boards: []string{"x,y"}, Default: true},
		{Kind: AddTemplate, Template: "unpicked", Name: "Unpicked", Parent: "symtus", Boards: []string{"x,y"}},
		{Kind: SetTemplate, Template: "arista-c360", Path: "radio.6g.enabled", Value: raw(`false`)},
	} {
		if err := Authorize(s, SystemActor, op); err == nil {
			t.Errorf("the manager may %s %s", op.Kind, op.Template)
		}
	}
	if got := TemplateIDFor(s.Library, "arista,c360"); got != "arista-c360-2" {
		t.Fatalf("a taken ID: %s", got)
	}
}

// An imported template is made with its settings in one change (0090): what
// a template may not set is refused, and leaves nothing behind; the manager
// may make one only empty.
func TestAddTemplateWithValues(t *testing.T) {
	s := templateOrg(t)
	op := Op{Kind: AddTemplate, Template: "c360-6e", Name: "Arista C-360", Parent: "house", Boards: []string{"arista,c360"}, Default: true,
		Values: map[hierarchy.Path]json.RawMessage{"radio.6g.width": raw(`160`), "radio.6g.psc": raw(`true`), "system.country": raw(`"US"`)}}
	_, eff, err := Apply(s, op)
	if err != nil {
		t.Fatal(err)
	}
	tm, ok := s.Library.Template("c360-6e")
	if !ok || len(tm.Values) != 3 || tm.Values["radio.6g.width"] != float64(160) {
		t.Fatalf("template = %+v", tm)
	}
	if v, _ := eff.After.(map[string]any)["values"].(map[string]any); len(v) != 3 {
		t.Errorf("effect = %+v", eff.After)
	}
	if r := resolved(t, s, "c360", "radio.6g.width"); r.Value != float64(160) || r.Origin != hierarchy.OriginTemplate || r.From != "house" {
		t.Errorf("radio.6g.width = %+v", r)
	}
	bad := Op{Kind: AddTemplate, Template: "with-a-tunnel", Name: "X", Parent: "house", Boards: []string{"arista,c360"},
		Values: map[hierarchy.Path]json.RawMessage{"radio.6g.width": raw(`160`), "concentrators.lab.address": raw(`"1.1.1.2"`)}}
	c := s.Clone()
	if _, _, err := Apply(c, bad); !errors.Is(err, ErrTemplateField) {
		t.Errorf("a tunnel in an imported template: %v", err)
	}
	if _, _, err := Apply(s.Clone(), Op{Kind: AddTemplate, Template: "x", Name: "X", Parent: "house", Boards: []string{"arista,c360"},
		Path: "radio.6g.width", Value: raw(`160`)}); !errors.Is(err, ErrTwoForms) {
		t.Errorf("path and value on add-template: %v", err)
	}
	empty := templateOrg(t)
	sys := Op{Kind: AddTemplate, Template: "arista-c360", Name: "Arista C-360", Parent: empty.Org.Locations.Root(), Boards: []string{"arista,c360"}, Default: true,
		Values: map[hierarchy.Path]json.RawMessage{"radio.6g.width": raw(`160`)}}
	if err := Authorize(empty, SystemActor, sys); !errors.Is(err, ErrDefaultTemplate) {
		t.Errorf("the manager making a template with settings: %v", err)
	}
}
