package api

import (
	"testing"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Usage and an AP's health over more than two days are read from the
// trends (0140): by the hour, and a client by the day. They say what the
// reports came to when the reports' own rows are gone, as far back as a
// year.
func TestUsageAndHealthFromTheTrends(t *testing.T) {
	f := newFixture(t)
	ap, _, _ := f.adopted()
	id := hierarchy.NodeID(ap)
	const mac = "7e:2a:ea:9b:2b:8f"
	n := func(v int) *int { return &v }
	for _, r := range []struct {
		ago      time.Duration
		clients  int
		down, up int64
		cpu      int
	}{
		{40 * 24 * time.Hour, 5, 1000, 100, 80},
		{10 * 24 * time.Hour, 2, 200, 20, 30},
		{3 * time.Hour, 1, 30, 3, 10},
	} {
		*f.skew = -r.ago
		if err := f.conds.RecordUse(id, r.clients, []conditions.ClientUse{{MAC: mac, Host: "phone", Down: r.down, Up: r.up}}); err != nil {
			t.Fatal(err)
		}
		if err := f.conds.RecordHealth(id, conditions.HealthPoint{Clients: r.clients, CPU: n(r.cpu)}, nil); err != nil {
			t.Fatal(err)
		}
	}
	*f.skew = 0
	// The reports' own rows of 40 days ago go, as they do after 30.
	if _, err := f.conds.TrimStates(30 * 24 * time.Hour); err != nil {
		t.Fatal(err)
	}

	usage := func(hours string) map[string]any {
		t.Helper()
		code, body := f.do("GET", "/v1/usage?hours="+hours, "griff", nil)
		if code != 200 {
			t.Fatalf("usage over %s h: %d %v", hours, code, body)
		}
		return body
	}
	top := func(body map[string]any) map[string]any {
		t.Helper()
		cl := body["clients"].([]any)
		if len(cl) != 1 {
			t.Fatalf("clients: %v", cl)
		}
		return cl[0].(map[string]any)
	}
	// A day: from the reports' own rows, as before.
	if d := usage("24"); d["down"] != 30.0 || d["up"] != 3.0 || d["peak"] != 1.0 || top(d)["down"] != 30.0 {
		t.Errorf("a day: down %v up %v peak %v clients %v", d["down"], d["up"], d["peak"], d["clients"])
	}
	// 30 days: by the hour, in whole hours.
	m := usage("720")
	if m["down"] != 230.0 || m["up"] != 23.0 || m["peak"] != 2.0 || int(m["bucket"].(float64))%3600 != 0 || len(m["buckets"].([]any)) > 48 {
		t.Errorf("30 days: down %v up %v peak %v, %d buckets of %v s", m["down"], m["up"], m["peak"], len(m["buckets"].([]any)), m["bucket"])
	}
	if c := top(m); c["down"] != 230.0 || c["up"] != 23.0 || c["host"] != "phone" {
		t.Errorf("30 days, its client: %v", c)
	}
	// A year: what the rows let go said is there still.
	y := usage("8760")
	if y["down"] != 1230.0 || y["up"] != 123.0 || y["peak"] != 5.0 || y["recorded_from"] == nil || len(y["buckets"].([]any)) > 48 {
		t.Errorf("a year: down %v up %v peak %v from %v, %d buckets", y["down"], y["up"], y["peak"], y["recorded_from"], len(y["buckets"].([]any)))
	}
	if c := top(y); c["down"] != 1230.0 || c["up"] != 123.0 {
		t.Errorf("a year, its client: %v", c)
	}
	a := y["aps"].([]any)[0].(map[string]any)
	heard := 0
	for _, h := range a["heard"].([]any) {
		if h == true {
			heard++
		}
	}
	if a["ap"] != ap || a["down"] != 1230.0 || a["peak"] != 5.0 || heard != 3 {
		t.Errorf("a year, its AP: %v, heard in %d", a, heard)
	}

	health := func(hours string) []any {
		t.Helper()
		code, body := f.do("GET", "/v1/aps/"+ap+"/health?hours="+hours, "griff", nil)
		if code != 200 {
			t.Fatalf("health over %s h: %d %v", hours, code, body)
		}
		return body["points"].([]any)
	}
	cpus := func(pts []any) []float64 {
		var out []float64
		for _, p := range pts {
			out = append(out, p.(map[string]any)["cpu"].(float64))
		}
		return out
	}
	if got := cpus(health("24")); len(got) != 1 || got[0] != 10 {
		t.Errorf("health over a day: %v", got)
	}
	if got := cpus(health("8760")); len(got) != 3 || got[0] != 80 || got[1] != 30 || got[2] != 10 {
		t.Errorf("health over a year: %v", got)
	}
	for _, q := range []string{"/v1/usage?hours=8761", "/v1/aps/" + ap + "/health?hours=8761", "/v1/aps/" + ap + "/timeline?hours=721"} {
		if code, b := f.do("GET", q, "griff", nil); code != 400 {
			t.Errorf("%s: %d %v", q, code, b)
		}
	}
}
