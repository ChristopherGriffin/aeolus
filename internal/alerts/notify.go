package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Target is where one AP's alerts go (0101), as its folders' notify.*
// fields say: a webhook, an ntfy topic, or both; the least severe alert
// sent; and whether an alert's end is sent too.
type Target struct {
	Webhook  string
	Ntfy     string
	Least    string // critical, warning or info; critical unless set
	Resolved bool
}

// Wants says whether the target takes an alert of the severity.
func (t Target) Wants(severity string) bool {
	least := t.Least
	if least == "" {
		least = Critical
	}
	return (t.Webhook != "" || t.Ntfy != "") && rank[severity] <= rank[least]
}

// Event is one message the notifier sends: an alert that began, or ended.
type Event struct {
	Kind   string    `json:"event"` // alert or resolved
	Alert  Alert     `json:"alert"`
	At     time.Time `json:"at"`
	Target Target    `json:"-"`
}

// Hold is how long an alert must last before it is sent, so a blip of one
// poll is not; Quiet is how long a key stays quiet after it ended, so an AP
// coming and going does not page someone every minute.
const (
	Hold  = 2 * time.Minute
	Quiet = 15 * time.Minute
)

// Notifier works out, from the alerts at each look, what to send. It is
// not safe for concurrent use. At its first look it takes what is already
// alerting as sent, so a restart of the manager sends nothing anew.
type Notifier struct {
	started bool
	first   map[string]time.Time // key -> when it was first seen, not yet sent
	sent    map[string]Alert     // key -> the alert sent, while it lasts
	quiet   map[string]time.Time // key -> until when it stays quiet
}

func NewNotifier() *Notifier {
	return &Notifier{first: map[string]time.Time{}, sent: map[string]Alert{}, quiet: map[string]time.Time{}}
}

func eventKey(a Alert) string { return string(a.AP) + "|" + a.Key }

// Step takes the alerts now, and each AP's target, and returns what to send.
func (n *Notifier) Step(now time.Time, current []Alert, target func(Alert) Target) []Event {
	var out []Event
	seen := map[string]bool{}
	for _, a := range current {
		k := eventKey(a)
		seen[k] = true
		// At the first look, what its target would have had counts as sent;
		// what nothing would have had waits, so an endpoint added later, or
		// a severity lowered, still hears of it.
		if !n.started && target(a).Wants(a.Severity) {
			n.sent[k] = a
			continue
		}
		if _, done := n.sent[k]; done {
			continue
		}
		t := target(a)
		if !t.Wants(a.Severity) || now.Before(n.quiet[k]) {
			continue
		}
		if _, ok := n.first[k]; !ok {
			n.first[k] = now
		}
		if now.Sub(n.first[k]) >= Hold {
			out = append(out, Event{Kind: "alert", Alert: a, At: now, Target: t})
			n.sent[k] = a
			delete(n.first, k)
		}
	}
	for k := range n.first {
		if !seen[k] {
			delete(n.first, k)
		}
	}
	for k, a := range n.sent {
		if seen[k] {
			continue
		}
		delete(n.sent, k)
		n.quiet[k] = now.Add(Quiet)
		if t := target(a); n.started && t.Resolved && t.Wants(a.Severity) {
			out = append(out, Event{Kind: "resolved", Alert: a, At: now, Target: t})
		}
	}
	for k, until := range n.quiet {
		if !now.Before(until) {
			delete(n.quiet, k)
		}
	}
	n.started = true
	return out
}

// Send delivers one event to its target: to the webhook as JSON, and to the
// ntfy topic as a push, its priority by severity. It returns the first
// error; the event is not tried again.
func Send(ctx context.Context, c *http.Client, e Event) error {
	var firstErr error
	note := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if e.Target.Webhook != "" {
		body, _ := json.Marshal(e)
		note(post(ctx, c, e.Target.Webhook, "application/json", body, nil))
	}
	if e.Target.Ntfy != "" {
		title, msg := Text(e)
		prio, tags := map[string]string{Critical: "5", Warning: "4", Info: "3"}[e.Alert.Severity], map[string]string{Critical: "rotating_light", Warning: "warning", Info: "information_source"}[e.Alert.Severity]
		if e.Kind == "resolved" {
			prio, tags = "3", "white_check_mark"
		}
		note(post(ctx, c, e.Target.Ntfy, "text/plain; charset=utf-8", []byte(msg), map[string]string{"Title": title, "Priority": prio, "Tags": tags}))
	}
	return firstErr
}

// Text is an event as people read it: a title and a line.
func Text(e Event) (title, msg string) {
	title = fmt.Sprintf("Aeolus: %s", e.Alert.Name)
	if e.Kind == "resolved" {
		return title + " is all right again", "resolved: " + e.Alert.Message
	}
	return fmt.Sprintf("%s (%s)", title, e.Alert.Severity), e.Alert.Message
}

// origin is a target URL as it may be logged: its scheme and host, without
// the path or query, which may hold the topic or token it is sealed for.
func origin(target string) string {
	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		return "the alert target"
	}
	return u.Scheme + "://" + u.Host
}

// post sends one alert. A redirect is refused rather than followed: Go
// would follow it as a GET, without the alert, and call it sent. Errors
// name only the target's origin.
func post(ctx context.Context, c *http.Client, target, kind string, body []byte, headers map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%s: the URL does not parse", origin(target))
	}
	req.Header.Set("Content-Type", kind)
	req.Header.Set("User-Agent", "aeolus-alerts")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	nr := *c
	nr.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := nr.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("%s: %w", origin(target), err)
	}
	res.Body.Close()
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("%s answered %s", origin(target), res.Status)
	}
	return nil
}
