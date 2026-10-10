package alerts

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func events(es []Event) string {
	var out []string
	for _, e := range es {
		out = append(out, e.Kind+":"+e.Alert.Key)
	}
	return strings.Join(out, " ")
}

func TestNotifierHoldsThenSendsOnceThenResolves(t *testing.T) {
	n := NewNotifier()
	to := func(Alert) Target { return Target{Ntfy: "https://ntfy.example/topic", Resolved: true} }
	offline := Alert{AP: "ap-1", Name: "C360-AP", Severity: Critical, Kind: "offline", Key: "offline", Message: "offline"}
	t0 := now
	// The first look takes what is alerting as sent: nothing goes out.
	if got := events(n.Step(t0, nil, to)); got != "" {
		t.Fatalf("first look: %s", got)
	}
	if got := events(n.Step(t0.Add(time.Minute), []Alert{offline}, to)); got != "" {
		t.Fatalf("new, held: %s", got)
	}
	if got := events(n.Step(t0.Add(2*time.Minute), []Alert{offline}, to)); got != "" {
		t.Fatalf("held a minute: %s", got)
	}
	if got := events(n.Step(t0.Add(3*time.Minute), []Alert{offline}, to)); got != "alert:offline" {
		t.Fatalf("after the hold: %s", got)
	}
	if got := events(n.Step(t0.Add(4*time.Minute), []Alert{offline}, to)); got != "" {
		t.Fatalf("still alerting: %s", got)
	}
	if got := events(n.Step(t0.Add(5*time.Minute), nil, to)); got != "resolved:offline" {
		t.Fatalf("ended: %s", got)
	}
	// Back within the quiet time: nothing, however long it lasts.
	for m := 6; m <= 12; m++ {
		if got := events(n.Step(t0.Add(time.Duration(m)*time.Minute), []Alert{offline}, to)); got != "" {
			t.Fatalf("minute %d, quiet: %s", m, got)
		}
	}
}

func TestNotifierBlipIsNotSent(t *testing.T) {
	n := NewNotifier()
	to := func(Alert) Target { return Target{Webhook: "https://hook.example/x"} }
	a := Alert{AP: "ap-1", Severity: Critical, Kind: "offline", Key: "offline"}
	n.Step(now, nil, to)
	n.Step(now.Add(time.Minute), []Alert{a}, to)
	if got := events(n.Step(now.Add(2*time.Minute), nil, to)); got != "" {
		t.Fatalf("a blip: %s", got)
	}
	if got := events(n.Step(now.Add(5*time.Minute), []Alert{a}, to)); got != "" {
		t.Fatalf("back, held again: %s", got)
	}
}

func TestNotifierRestartSendsNothingAnew(t *testing.T) {
	n := NewNotifier()
	to := func(Alert) Target { return Target{Webhook: "https://hook.example/x", Resolved: true} }
	a := Alert{AP: "ap-1", Severity: Critical, Kind: "offline", Key: "offline"}
	if got := events(n.Step(now, []Alert{a}, to)); got != "" {
		t.Fatalf("first look: %s", got)
	}
	if got := events(n.Step(now.Add(10*time.Minute), []Alert{a}, to)); got != "" {
		t.Fatalf("already alerting at start: %s", got)
	}
	// Its end is news, and is sent.
	if got := events(n.Step(now.Add(11*time.Minute), nil, to)); got != "resolved:offline" {
		t.Fatalf("ended: %s", got)
	}
}

func TestTargetSeverity(t *testing.T) {
	warn := Alert{AP: "ap-1", Severity: Warning, Kind: "tunnel-down", Key: "tunnel-down:50"}
	n := NewNotifier()
	n.Step(now, nil, func(Alert) Target { return Target{} })
	for _, tc := range []struct {
		t    Target
		want string
	}{
		{Target{Ntfy: "https://n/x"}, ""}, // critical only, by default
		{Target{Ntfy: "https://n/x", Least: Warning}, "alert:tunnel-down:50"},
		{Target{Least: Warning}, ""}, // nowhere to send it
	} {
		n := NewNotifier()
		to := func(Alert) Target { return tc.t }
		n.Step(now, nil, to)
		n.Step(now.Add(time.Minute), []Alert{warn}, to)
		if got := events(n.Step(now.Add(4*time.Minute), []Alert{warn}, to)); got != tc.want {
			t.Fatalf("%+v: %s, want %s", tc.t, got, tc.want)
		}
	}
}

func TestSendWebhookAndNtfy(t *testing.T) {
	var hook map[string]any
	var ntfyBody string
	var ntfyHead http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/hook" {
			json.Unmarshal(b, &hook)
		} else {
			ntfyBody, ntfyHead = string(b), r.Header
		}
	}))
	defer srv.Close()
	e := Event{Kind: "alert", At: now, Target: Target{Webhook: srv.URL + "/hook", Ntfy: srv.URL + "/topic"},
		Alert: Alert{AP: "ap-1", Name: "C360-AP", Severity: Critical, Kind: "offline", Key: "offline", Message: "offline: last heard 6 minutes ago"}}
	if err := Send(context.Background(), srv.Client(), e); err != nil {
		t.Fatal(err)
	}
	if hook["event"] != "alert" || hook["alert"].(map[string]any)["name"] != "C360-AP" {
		t.Fatalf("webhook got %v", hook)
	}
	if ntfyBody != "offline: last heard 6 minutes ago" || ntfyHead.Get("Priority") != "5" || ntfyHead.Get("Title") != "Aeolus: C360-AP (critical)" {
		t.Fatalf("ntfy got %q %v", ntfyBody, ntfyHead)
	}
}

// An alert open at the first look that no target would have had is not
// taken as sent: an endpoint added later still hears of it.
func TestNotifierStartupOnlyMarksWhatWasDeliverable(t *testing.T) {
	n := NewNotifier()
	a := Alert{AP: "ap-1", Severity: Critical, Kind: "offline", Key: "offline"}
	none := func(Alert) Target { return Target{} }
	n.Step(now, []Alert{a}, none)
	to := func(Alert) Target { return Target{Ntfy: "https://n/x"} }
	n.Step(now.Add(time.Minute), []Alert{a}, to)
	if got := events(n.Step(now.Add(4*time.Minute), []Alert{a}, to)); got != "alert:offline" {
		t.Fatalf("after an endpoint was added: %s", got)
	}
}

// Errors name only the target's origin, never its sealed path or query,
// and a redirect is refused, not followed as a GET without the alert.
func TestSendKeepsTheSecretOutOfErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/moved" {
			http.Redirect(w, r, "/topic-secret", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	for _, path := range []string{"/topic-secret?auth=tok", "/moved"} {
		e := Event{Kind: "alert", Target: Target{Ntfy: srv.URL + path}, Alert: Alert{Name: "C360-AP", Severity: Critical, Message: "offline"}}
		err := Send(context.Background(), srv.Client(), e)
		if err == nil {
			t.Fatalf("%s: sent", path)
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "tok") || strings.Contains(err.Error(), "moved") {
			t.Fatalf("%s: the error names the path: %v", path, err)
		}
	}
}
