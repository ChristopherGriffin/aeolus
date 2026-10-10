package api

import (
	"strconv"
	"testing"
)

// Actions (0104): a person with operator on an AP asks it to do something
// once; the AP hears of it on its next poll, fetches it, and says what came
// of it.
func TestActions(t *testing.T) {
	f := newFixture(t)
	ap, token, _ := f.adopted()
	if code, body := f.do("POST", "/v1/aps/"+ap+"/actions", "griff", map[string]any{"kind": "explode"}); code != 400 {
		t.Fatalf("a bad kind: %d %v", code, body)
	}
	code, body := f.do("POST", "/v1/aps/"+ap+"/actions", "griff", map[string]any{"kind": "locate"})
	if code != 200 {
		t.Fatalf("locate: %d %v", code, body)
	}
	id := body["action"].(map[string]any)["id"].(float64)
	// The poll says one waits.
	_, hdr, _ := f.apDo("GET", "/v1/ap/config", token, nil, nil)
	if hdr.Get("Aeolus-Actions") != "1" {
		t.Fatalf("poll header = %q", hdr.Get("Aeolus-Actions"))
	}
	_, _, got := f.apDo("GET", "/v1/ap/actions", token, nil, nil)
	list, _ := got["actions"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["kind"] != "locate" {
		t.Fatalf("pending = %v", got)
	}
	// Taken up: the next poll does not hand it out again.
	_, hdr, _ = f.apDo("GET", "/v1/ap/config", token, nil, nil)
	if hdr.Get("Aeolus-Actions") != "" {
		t.Fatalf("poll header once taken up = %q", hdr.Get("Aeolus-Actions"))
	}
	if _, _, again := f.apDo("GET", "/v1/ap/actions", token, nil, nil); len(again["actions"].([]any)) != 0 {
		t.Fatalf("handed out again: %v", again)
	}
	if code, _, body := f.apDo("POST", "/v1/ap/actions/"+strconv.FormatInt(int64(id), 10), token, map[string]any{"ok": true, "result": "blinking for 60 s"}, nil); code != 200 {
		t.Fatalf("done: %d %v", code, body)
	}
	_, hdr, _ = f.apDo("GET", "/v1/ap/config", token, nil, nil)
	if hdr.Get("Aeolus-Actions") != "" {
		t.Fatalf("poll header after = %q", hdr.Get("Aeolus-Actions"))
	}
	_, body = f.do("GET", "/v1/aps/"+ap+"/actions", "griff", nil)
	if a := body["actions"].([]any)[0].(map[string]any); a["state"] != "done" || a["actor"] != "griff" {
		t.Fatalf("history = %v", a)
	}
	// office may not see the AP, so it is not there for it.
	if code, _ := f.do("POST", "/v1/aps/"+ap+"/actions", "office", map[string]any{"kind": "reboot"}); code != 404 {
		t.Fatalf("office: %d", code)
	}
}

// Disconnect (0107) is of one client, by MAC: two clients at once are two
// actions, one client asked twice is one, and the AP is handed its target.
func TestDisconnect(t *testing.T) {
	f := newFixture(t)
	ap, token, _ := f.adopted()
	for _, b := range []map[string]any{{"kind": "disconnect"}, {"kind": "disconnect", "target": "not-a-mac"}, {"kind": "locate", "target": "7e:2a:ea:9b:2b:8f"}} {
		if code, body := f.do("POST", "/v1/aps/"+ap+"/actions", "griff", b); code != 400 {
			t.Fatalf("%v: %d %v", b, code, body)
		}
	}
	ask := func(mac string) float64 {
		t.Helper()
		code, body := f.do("POST", "/v1/aps/"+ap+"/actions", "griff", map[string]any{"kind": "disconnect", "target": mac})
		if code != 200 {
			t.Fatalf("disconnect %s: %d %v", mac, code, body)
		}
		return body["action"].(map[string]any)["id"].(float64)
	}
	a, b := ask("7E:2A:EA:9B:2B:8F"), ask("84:0d:8e:5a:df:f7")
	if a == b || ask("7e:2a:ea:9b:2b:8f") != a {
		t.Fatalf("ids %v %v", a, b)
	}
	_, _, got := f.apDo("GET", "/v1/ap/actions", token, nil, nil)
	list, _ := got["actions"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["target"] != "7e:2a:ea:9b:2b:8f" || list[1].(map[string]any)["target"] != "84:0d:8e:5a:df:f7" {
		t.Fatalf("handed out = %v", got)
	}
}
