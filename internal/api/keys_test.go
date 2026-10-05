package api

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/keys"
)

func (f *fixture) key(as, kind, id string, def map[string]any) (int, map[string]any) {
	f.t.Helper()
	op := map[string]any{"kind": kind, "node": "household", "network": "sweet", "key": id, "value": def}
	return f.change(as, op)
}

func TestKeysThroughTheAPI(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("office", map[string]any{"kind": "set", "tree": "services", "node": "household",
		"path": "network.sweet.keys.vlans", "value": []int{101}}); code != 200 {
		t.Fatalf("keys.vlans: %d %v", code, b)
	}

	// The leasing office adds a key; its ID is made, its passphrase sealed.
	code, body := f.key("office", "add-key", "", map[string]any{"name": "Unit 101", "passphrase": "unit-101-secret", "vlan": 101})
	if code != 200 {
		t.Fatalf("add-key: %d %v", code, body)
	}
	op := body["change"].(map[string]any)["op"].(map[string]any)
	id, _ := op["key"].(string)
	if !strings.HasPrefix(id, "k") || strings.Contains(jsonText(body), "unit-101-secret") {
		t.Fatalf("add-key showed: %v", body)
	}
	if p := op["value"].(map[string]any)["passphrase"]; p == nil || jsonText(p) != `{"sealed":true}` {
		t.Fatalf("passphrase shown as %v", p)
	}

	for name, c := range map[string]struct {
		as   string
		def  map[string]any
		code int
	}{
		"the network's passphrase": {"office", map[string]any{"name": "X", "passphrase": passphrase}, 400},
		"another key's passphrase": {"office", map[string]any{"name": "X", "passphrase": "unit-101-secret"}, 400},
		"too short":                {"office", map[string]any{"name": "X", "passphrase": "short"}, 400},
		"expired already":          {"office", map[string]any{"name": "X", "passphrase": "another-secret", "expires": time.Now().Add(-time.Hour)}, 400},
		"a VLAN not offered":       {"office", map[string]any{"name": "X", "passphrase": "another-secret", "vlan": 300}, 400},
		"a viewer":                 {"tenant", map[string]any{"name": "X", "passphrase": "another-secret"}, 403},
		"a sealed value handed in": {"office", map[string]any{"name": "X", "passphrase": map[string]any{"$sealed": "eA=="}}, 400},
	} {
		if code, b := f.key(c.as, "add-key", "", c.def); code != c.code {
			t.Errorf("%s: %d %v", name, code, b)
		}
	}
	if code, _ := f.key("office", "set-key", "nobody", map[string]any{"name": "X"}); code != 404 {
		t.Errorf("set-key of no key: %d", code)
	}

	// Listed without passphrases; a viewer may read, not edit.
	_, list := f.do("GET", "/v1/keys?folder=household&network=sweet", "tenant", nil)
	ks := list["keys"].([]any)
	if len(ks) != 1 || list["can_edit"] != false || ks[0].(map[string]any)["name"] != "Unit 101" || strings.Contains(jsonText(list), "sealed") {
		t.Fatalf("list: %v", list)
	}
	if code, _ := f.do("GET", "/v1/keys?folder=guest&network=guest", "tenant", nil); code != 404 {
		t.Fatalf("list where tenant has no role: %d", code)
	}
	if _, changes := f.do("GET", "/v1/changes?after=0&limit=1000", "griff", nil); strings.Contains(jsonText(changes), "unit-101-secret") {
		t.Fatal("the change log shows a passphrase")
	}

	// An AP that gets the network gets the key, as the PSK for its SSID.
	ap, token, _ := f.adopted()
	code, _, got := f.apDo("GET", "/v1/ap/keys", token, nil, nil)
	version, _ := got["version"].(string)
	sweet := got["networks"].(map[string]any)["sweet"].(map[string]any)
	k0 := sweet["keys"].([]any)[0].(map[string]any)
	if code != 200 || version == "" || sweet["ssid"] != "Sweet Spot" || k0["id"] != id || k0["vlan"] != 101.0 || k0["psk"] != keys.PSK("unit-101-secret", "Sweet Spot") {
		t.Fatalf("ap keys: %d %v", code, got)
	}
	if code, _, _ := f.apDo("GET", "/v1/ap/keys?wait=0", token, nil, map[string]string{"If-None-Match": `"` + version + `"`}); code != 304 {
		t.Fatalf("unchanged: %d", code)
	}

	// A held request answers when a key changes.
	type answer struct {
		code int
		body map[string]any
	}
	held := make(chan answer, 1)
	go func() {
		code, _, b := f.apDo("GET", "/v1/ap/keys?wait=20", token, nil, map[string]string{"If-None-Match": `"` + version + `"`})
		held <- answer{code, b}
	}()
	time.Sleep(1500 * time.Millisecond)
	if code, b := f.key("office", "set-key", id, map[string]any{"name": "Unit 101", "passphrase": "unit-101-rotated", "vlan": 101}); code != 200 {
		t.Fatalf("rotate: %d %v", code, b)
	}
	select {
	case a := <-held:
		k := a.body["networks"].(map[string]any)["sweet"].(map[string]any)["keys"].([]any)[0].(map[string]any)
		if a.code != 200 || a.body["version"] == version || k["psk"] != keys.PSK("unit-101-rotated", "Sweet Spot") {
			t.Fatalf("after rotating: %d %v", a.code, a.body)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the held request did not answer")
	}

	// Keys added one after another while a request is held reach the AP
	// together, once they have settled.
	_, _, got = f.apDo("GET", "/v1/ap/keys", token, nil, nil)
	version, _ = got["version"].(string)
	go func() {
		code, _, b := f.apDo("GET", "/v1/ap/keys?wait=20", token, nil, map[string]string{"If-None-Match": `"` + version + `"`})
		held <- answer{code, b}
	}()
	time.Sleep(1200 * time.Millisecond)
	for i, name := range []string{"Unit 102", "Unit 103"} {
		if code, b := f.key("office", "add-key", "", map[string]any{"name": name, "passphrase": fmt.Sprintf("unit-10%d-secret", i+2)}); code != 200 {
			t.Fatalf("%s: %d %v", name, code, b)
		}
		time.Sleep(700 * time.Millisecond)
	}
	select {
	case a := <-held:
		if n := len(a.body["networks"].(map[string]any)["sweet"].(map[string]any)["keys"].([]any)); a.code != 200 || n != 3 {
			t.Fatalf("after a burst: %d, %d keys: %v", a.code, n, a.body)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the held request did not answer after a burst")
	}
	for _, k := range keysOf(t, f) {
		if k != id {
			f.key("office", "remove-key", k, nil)
		}
	}

	// The AP's page says how many keys it should have, and the AP reports it.
	_, cfg := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	if cfg["keys"].(map[string]any)["count"] != 1.0 {
		t.Fatalf("config keys: %v", cfg["keys"])
	}
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": 1, "keys": map[string]any{"version": "zz", "count": 1}}, nil); code != 400 {
		t.Fatalf("a bad keys report: %d %v", code, b)
	}

	// Removed, it leaves the AP's set.
	if code, b := f.key("office", "remove-key", id, nil); code != 200 {
		t.Fatalf("remove-key: %d %v", code, b)
	}
	_, _, got = f.apDo("GET", "/v1/ap/keys", token, nil, nil)
	if len(got["networks"].(map[string]any)) != 0 {
		t.Fatalf("after remove: %v", got)
	}
}

func keysOf(t *testing.T, f *fixture) []string {
	t.Helper()
	_, list := f.do("GET", "/v1/keys?folder=household&network=sweet", "office", nil)
	var out []string
	for _, k := range list["keys"].([]any) {
		out = append(out, k.(map[string]any)["id"].(string))
	}
	return out
}
