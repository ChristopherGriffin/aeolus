package change

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

func concentrator(id, value string) Op {
	return Op{Kind: SetConcentrator, Concentrator: id, Value: json.RawMessage(value)}
}

func TestLibraryChanges(t *testing.T) {
	s := people(t)
	mustApply(t, s, concentrator("homelab", `{"name":"Homelab","address":"1.1.1.2","port":4789,"mtu":1450,"scope":["gate"]}`))
	mustApply(t, s, Op{Kind: SetVNI, Concentrator: "homelab", VNI: 20, Name: "Trusted"})
	k, ok := s.Library.Get("homelab")
	if !ok || k.Scope[0] != "gate" || k.VNIs[20] != "Trusted" {
		t.Fatalf("homelab = %+v", k)
	}
	if _, _, err := Apply(s, concentrator("x", `{"name":"X","address":"1.1.1.9","port":4789,"mtu":1450,"scope":["nowhere"]}`)); err == nil {
		t.Fatal("a scope naming an unknown folder was accepted")
	}
	if _, _, err := Apply(s, concentrator("x", `{"name":"X","address":"1.1.1.9","port":4789,"mtu":1450,"vnis":{"20":{"label":"T"}}}`)); err == nil {
		t.Fatal("VNIs inside set-concentrator were accepted")
	}
}

func TestLibraryRefusesRemovingWhatIsInUse(t *testing.T) {
	s := people(t)
	mustApply(t, s, concentrator("homelab", `{"name":"Homelab","address":"1.1.1.2","port":4789,"mtu":1450}`))
	mustApply(t, s, Op{Kind: SetVNI, Concentrator: "homelab", VNI: 20, Name: "Trusted"})
	mustApply(t, s, Op{Kind: SetVNI, Concentrator: "homelab", VNI: 30, Name: "Unused"})
	mustApply(t, s, Op{Kind: Set, Tree: Services, Node: "household", Path: "network.sweet.transport.primary.concentrator", Value: json.RawMessage(`"homelab"`)})
	mustApply(t, s, Op{Kind: AddFolder, Tree: Services, Node: "hh-gate", Name: "Gate", Parent: "household"})
	// The VNI is set one folder below where the concentrator is chosen.
	mustApply(t, s, Op{Kind: Set, Tree: Services, Node: "hh-gate", Path: "network.sweet.transport.primary.vni", Value: json.RawMessage(`20`)})

	var inUse *InUseError
	if _, _, err := Apply(s, Op{Kind: RemoveVNI, Concentrator: "homelab", VNI: 20}); !errors.As(err, &inUse) || len(inUse.By) != 1 {
		t.Fatalf("removing a VNI in use: %v", err)
	}
	mustApply(t, s, Op{Kind: RemoveVNI, Concentrator: "homelab", VNI: 30})
	if _, _, err := Apply(s, Op{Kind: RemoveConcentrator, Concentrator: "homelab"}); !errors.As(err, &inUse) {
		t.Fatalf("removing a concentrator in use: %v", err)
	}
	mustApply(t, s, Op{Kind: Unset, Tree: Services, Node: "hh-gate", Path: "network.sweet.transport.primary.vni"})
	mustApply(t, s, Op{Kind: Unset, Tree: Services, Node: "household", Path: "network.sweet.transport.primary.concentrator"})
	mustApply(t, s, Op{Kind: RemoveConcentrator, Concentrator: "homelab"})
	if _, ok := s.Library.Get("homelab"); ok {
		t.Fatal("homelab still in the library")
	}
}

func TestOnlyServicesRootAdminsEditTheLibrary(t *testing.T) {
	s := people(t)
	op := concentrator("homelab", `{"name":"Homelab","address":"1.1.1.2","port":4789,"mtu":1450}`)
	if err := Authorize(s, "griff", op); err != nil {
		t.Fatalf("Org admin: %v", err)
	}
	for _, who := range []string{"claude", "office", "gatekeeper"} {
		if err := Authorize(s, who, op); !errors.Is(err, ErrForbidden) {
			t.Errorf("%s: %v, want ErrForbidden", who, err)
		}
	}
}

func TestTransportField(t *testing.T) {
	if n, slot, f, ok := TransportField("network.sweet.transport.fallback.vni"); !ok || n != "sweet" || slot != "fallback" || f != "vni" {
		t.Fatalf("got %s %s %s %v", n, slot, f, ok)
	}
	for _, p := range []string{"network.sweet.ssid", "network.sweet.transport.tertiary.vni", "radio.2g.width"} {
		if _, _, _, ok := TransportField(hierarchy.Path(p)); ok {
			t.Errorf("%s parsed as a transport field", p)
		}
	}
}
