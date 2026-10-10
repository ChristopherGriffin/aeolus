package api

import (
	"fmt"
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/alerts"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// The alert log (0109): an alert that lasts alerts.Hold is logged from when
// it was first seen, or from when it says it began; a blip is not; its end
// is logged when it goes; and a manager that starts again ends what went
// while it was down. GET /v1/alerts/history reads it.
func TestAlertHistory(t *testing.T) {
	f := newFixture(t)
	ap, _, _ := f.adopted()
	id := hierarchy.NodeID(ap)
	t0 := time.Now().Add(-30 * time.Minute)
	at := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	offline := alerts.Alert{AP: id, Name: "PumphouseAP", Severity: alerts.Critical, Kind: "offline", Key: "offline", Message: "not heard from"}
	blip := alerts.Alert{AP: id, Name: "PumphouseAP", Severity: alerts.Warning, Kind: "vlan-silent", Key: "vlan-silent:20", Message: "VLAN 20 is silent"}
	since := at(11).Add(-time.Hour)
	held := alerts.Alert{AP: id, Name: "PumphouseAP", Severity: alerts.Warning, Kind: "held", Key: "held", Message: "its config is held", Since: &since}
	r := newAlertRecorder()
	for _, step := range []struct {
		m   int
		now []alerts.Alert
	}{
		{0, []alerts.Alert{offline}},
		{1, []alerts.Alert{offline}},
		{2, []alerts.Alert{offline, blip}},
		{3, []alerts.Alert{offline}},
		{10, nil},
		{11, []alerts.Alert{held}},
	} {
		f.api.recordAlerts(r, at(step.m), step.now)
	}
	// The manager starts again; held went while it was down.
	f.api.recordAlerts(newAlertRecorder(), at(12), nil)
	// Another AP's alerts, newer and more than the limit, crowd out none of
	// this one's.
	for i := 0; i < 2100; i++ {
		if _, err := f.conds.BeginAlert(conditions.LoggedAlert{AP: "office-ap", Key: fmt.Sprintf("loop:%d", i), Kind: "loop", Severity: alerts.Critical, Message: "a loop", Began: at(20)}); err != nil {
			t.Fatal(err)
		}
	}
	history := func(who string) []any {
		t.Helper()
		code, body := f.do("GET", "/v1/alerts/history?under="+ap, who, nil)
		if code != 200 {
			t.Fatalf("%d %v", code, body)
		}
		return body["alerts"].([]any)
	}
	got := history("griff")
	if len(got) != 2 {
		t.Fatalf("history = %v", got)
	}
	when := func(v any) time.Time {
		w, _ := time.Parse(time.RFC3339Nano, v.(string))
		return w
	}
	a, b := got[0].(map[string]any), got[1].(map[string]any)
	if a["key"] != "offline" || !when(a["began"]).Equal(at(0)) || !when(a["ended"]).Equal(at(10)) || a["name"] != "PumphouseAP" {
		t.Fatalf("offline = %v", a)
	}
	if b["key"] != "held" || !when(b["began"]).Equal(since) || !when(b["ended"]).Equal(at(12)) {
		t.Fatalf("held = %v", b)
	}
	if code, _ := f.do("GET", "/v1/alerts/history?hours=0", "griff", nil); code != 400 {
		t.Fatalf("hours=0: %d", code)
	}
	if got := history("office"); len(got) != 0 {
		t.Fatalf("office sees %v", got)
	}
}
