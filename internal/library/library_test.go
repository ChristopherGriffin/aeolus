package library

import (
	"errors"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

func TestSetKeepsVNIsAndReturnsThePrevious(t *testing.T) {
	l := New()
	if prev, err := l.Set(Concentrator{ID: "homelab", Name: "Homelab", Address: "1.1.1.2", Port: 4789, MTU: 1450}); err != nil || prev != nil {
		t.Fatalf("first Set = %v, %v", prev, err)
	}
	if _, err := l.SetVNI("homelab", 20, "Trusted"); err != nil {
		t.Fatal(err)
	}
	prev, err := l.Set(Concentrator{ID: "homelab", Name: "Homelab", Address: "1.1.1.3", Port: 4789, MTU: 1450})
	if err != nil || prev == nil || prev.Address != "1.1.1.2" {
		t.Fatalf("second Set = %+v, %v", prev, err)
	}
	k, _ := l.Get("homelab")
	if k.Address != "1.1.1.3" || k.VNIs[20] != "Trusted" {
		t.Fatalf("after replace: %+v", k)
	}
}

func TestVNIRules(t *testing.T) {
	l := New()
	must(t, set(l, "homelab"))
	for _, c := range []struct {
		vni   int
		label string
		want  error
	}{{0, "x", ErrBadVNI}, {16777216, "x", ErrBadVNI}, {20, "", ErrNoLabel}} {
		if _, err := l.SetVNI("homelab", c.vni, c.label); !errors.Is(err, c.want) {
			t.Errorf("SetVNI(%d, %q) = %v, want %v", c.vni, c.label, err, c.want)
		}
	}
	if _, err := l.SetVNI("nope", 20, "x"); !errors.Is(err, ErrNoConcentrator) {
		t.Errorf("unknown concentrator: %v", err)
	}
	if prev, _ := l.SetVNI("homelab", 20, "Trusted"); prev != "" {
		t.Errorf("new VNI had a previous label %q", prev)
	}
	if prev, _ := l.SetVNI("homelab", 20, "Household"); prev != "Trusted" {
		t.Errorf("relabel returned %q", prev)
	}
	if err := l.RemoveVNI("homelab", 30); !errors.Is(err, ErrNoVNI) {
		t.Errorf("removing a missing VNI: %v", err)
	}
	must(t, l.RemoveVNI("homelab", 20))
}

func TestAvailableAt(t *testing.T) {
	everywhere := Concentrator{ID: "cloud"}
	gateOnly := Concentrator{ID: "homelab", Scope: []hierarchy.NodeID{"gate"}}
	underGate := []hierarchy.NodeID{"symtus", "gate", "gate-ap"}
	house := []hierarchy.NodeID{"symtus", "house", "office-ap"}
	if !everywhere.AvailableAt(house) || !gateOnly.AvailableAt(underGate) || gateOnly.AvailableAt(house) {
		t.Fatal("scope rules wrong")
	}
}

func TestCloneIsIndependent(t *testing.T) {
	l := New()
	must(t, set(l, "homelab"))
	c := l.Clone()
	if _, err := c.SetVNI("homelab", 20, "Trusted"); err != nil {
		t.Fatal(err)
	}
	must(t, c.Remove("homelab"))
	if k, ok := l.Get("homelab"); !ok || len(k.VNIs) != 0 {
		t.Fatalf("clone changes reached the original: %+v, %v", k, ok)
	}
}

func set(l *Library, id string) error {
	_, err := l.Set(Concentrator{ID: id, Name: id, Address: "1.1.1.2", Port: 4789, MTU: 1450})
	return err
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
