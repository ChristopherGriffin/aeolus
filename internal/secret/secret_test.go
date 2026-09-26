package secret

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	b := box(t)
	sealed, err := b.Seal("network.sweet.passphrase", "correct horse battery")
	must(t, err)
	if !IsSealed(sealed) {
		t.Fatal("IsSealed(sealed) = false")
	}
	raw, _ := json.Marshal(sealed)
	if strings.Contains(string(raw), "horse") {
		t.Fatalf("sealed value shows the plain text: %s", raw)
	}
	plain, err := b.Open("network.sweet.passphrase", sealed)
	must(t, err)
	if plain != "correct horse battery" {
		t.Fatalf("Open = %v", plain)
	}
}

func TestSealedValueSurvivesJSON(t *testing.T) {
	b := box(t)
	sealed, err := b.Seal("p", "x1234567")
	must(t, err)
	raw, _ := json.Marshal(sealed)
	var back any
	must(t, json.Unmarshal(raw, &back))
	if plain, err := b.Open("p", back); err != nil || plain != "x1234567" {
		t.Fatalf("Open after JSON = %v, %v", plain, err)
	}
}

func TestSealedValueIsBoundToItsPath(t *testing.T) {
	b := box(t)
	sealed, err := b.Seal("network.sweet.passphrase", "x1234567")
	must(t, err)
	if _, err := b.Open("network.sweet.ssid", sealed); err == nil {
		t.Fatal("a value sealed for one field opened under another")
	}
}

func TestWrongKeyAndTamperingFail(t *testing.T) {
	sealed, err := box(t).Seal("p", "x1234567")
	must(t, err)
	if _, err := box(t).Open("p", sealed); err == nil {
		t.Fatal("opened with a different key")
	}
	s := sealed[sealedKey].(string)
	flipped := map[string]any{sealedKey: s[:len(s)-4] + "AAAA"}
	if _, err := box(t).Open("p", flipped); err == nil {
		t.Fatal("opened a tampered value")
	}
}

func TestOpenRefusesPlainValues(t *testing.T) {
	if _, err := box(t).Open("p", "plain"); !errors.Is(err, ErrNotSealed) {
		t.Fatalf("got %v, want ErrNotSealed", err)
	}
	if IsSealed(map[string]any{sealedKey: "x", "other": 1}) {
		t.Fatal("an object with extra keys counted as sealed")
	}
}

func TestLoadOrCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.key")
	a, err := LoadOrCreate(path)
	must(t, err)
	sealed, err := a.Seal("p", "x1234567")
	must(t, err)
	b, err := LoadOrCreate(path)
	must(t, err)
	if plain, err := b.Open("p", sealed); err != nil || plain != "x1234567" {
		t.Fatalf("reloaded key cannot open: %v, %v", plain, err)
	}
	if runtime.GOOS != "windows" {
		must(t, os.Chmod(path, 0o644))
		if _, err := LoadOrCreate(path); !errors.Is(err, ErrKeyPerms) {
			t.Fatalf("world-readable key: got %v, want ErrKeyPerms", err)
		}
	}
}

func TestLoadNeverCreates(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.key")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load of a missing key: %v", err)
	}
}

func box(t *testing.T) *Box {
	t.Helper()
	b, err := LoadOrCreate(filepath.Join(t.TempDir(), "secret.key"))
	must(t, err)
	return b
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
