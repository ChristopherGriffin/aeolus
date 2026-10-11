package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/access"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/radio"
)

// An AP's own health, and what happened to it (0119).

// apHealth is how an AP says it is doing itself: how busy its processor
// was since its last report, in percent of all its cores, and how many it
// has; its load over 1, 5 and 15 minutes; its memory and the space where
// its config is kept, in kB; the hottest of its temperature sensors, in
// °C; how many processes it runs; and its connection table.
type apHealth struct {
	CPU          *int      `json:"cpu,omitempty"`
	Cores        int       `json:"cores,omitempty"`
	Load         []float64 `json:"load,omitempty"`
	MemTotal     int64     `json:"mem_total,omitempty"`
	MemAvailable int64     `json:"mem_available,omitempty"`
	StorageTotal int64     `json:"storage_total,omitempty"`
	StorageFree  int64     `json:"storage_free,omitempty"`
	Temp         *float64  `json:"temp,omitempty"`
	Procs        int       `json:"procs,omitempty"`
	Conntrack    int64     `json:"conntrack,omitempty"`
	ConntrackMax int64     `json:"conntrack_max,omitempty"`
}

func (h *apHealth) check() error {
	if h == nil {
		return nil
	}
	bad := badRequest("health: a processor busy 0 to 100 percent, up to 1024 cores, three loads, memory and storage in kB with no more free than there is, a temperature from -50 to 200, and counts not negative")
	if (h.CPU != nil && (*h.CPU < 0 || *h.CPU > 100)) || h.Cores < 0 || h.Cores > 1024 || (len(h.Load) != 0 && len(h.Load) != 3) {
		return bad
	}
	for _, l := range h.Load {
		if l < 0 || l > 100000 {
			return bad
		}
	}
	if h.MemTotal < 0 || h.MemAvailable < 0 || h.MemAvailable > h.MemTotal || h.StorageTotal < 0 || h.StorageFree < 0 || h.StorageFree > h.StorageTotal ||
		(h.Temp != nil && (*h.Temp < -50 || *h.Temp > 200)) || h.Procs < 0 || h.Procs > 1000000 || h.Conntrack < 0 || h.ConntrackMax < 0 {
		return bad
	}
	return nil
}

// point is the report's health as it is kept, a row a report.
func (st *stateReport) point() conditions.HealthPoint {
	p := conditions.HealthPoint{Uptime: st.Uptime, Clients: len(st.Clients)}
	h := st.Health
	if h == nil {
		return p
	}
	p.CPU, p.Temp = h.CPU, h.Temp
	if len(h.Load) == 3 {
		p.Load = &h.Load[0]
	}
	if h.MemTotal > 0 {
		p.MemTotal, p.MemAvailable = &h.MemTotal, &h.MemAvailable
	}
	if h.StorageTotal > 0 {
		p.StorageTotal, p.StorageFree = &h.StorageTotal, &h.StorageFree
	}
	if h.Procs > 0 {
		p.Procs = &h.Procs
	}
	return p
}

// silence is how long an AP may go without a report before its next one
// counts as coming back: three reports missed.
const silence = 3*stateEvery + time.Minute

func span(d time.Duration) string {
	switch {
	case d < 2*time.Minute:
		return fmt.Sprintf("%d s", int(d.Seconds()))
	case d < 2*time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()))
	}
	return fmt.Sprintf("%d days", int(d.Hours()/24))
}

// changes is what happened to an AP between its last report and this one
// (0119): a restart, a silence, another agent or firmware, a radio that
// went down, came up or moved channel, and an uplink on another switch
// port. An AP's first report is one too.
func changes(prev *conditions.State, now time.Time, cur *stateReport) []conditions.APEvent {
	at := func(kind, format string, args ...any) conditions.APEvent {
		return conditions.APEvent{At: now, Kind: kind, Text: fmt.Sprintf(format, args...)}
	}
	agent := func(a *agentState) string {
		if a == nil {
			return ""
		}
		return a.Version
	}
	if prev == nil {
		return []conditions.APEvent{at("first", "first report: up %s", span(time.Duration(cur.Uptime)*time.Second))}
	}
	var old stateReport
	if json.Unmarshal(prev.Report, &old) != nil {
		return nil
	}
	var out []conditions.APEvent
	quiet := now.Sub(prev.At)
	// It restarted where it has been up less long than it would have been,
	// had it run on since its last report; two minutes are allowed for.
	if cur.Uptime+120 < old.Uptime+int64(quiet.Seconds()) {
		e := at("restart", "restarted: it had been up %s", span(time.Duration(old.Uptime)*time.Second+quiet-time.Duration(cur.Uptime)*time.Second))
		e.At = now.Add(-time.Duration(cur.Uptime) * time.Second)
		if quiet > silence {
			e.Text += fmt.Sprintf(", and was not heard from for %s", span(quiet))
		}
		out = append(out, e)
	} else if quiet > silence {
		out = append(out, at("silence", "reports again, after %s without one; it did not restart", span(quiet)))
	}
	if a, b := agent(old.Agent), agent(cur.Agent); a != b && b != "" {
		if a == "" {
			out = append(out, at("agent", "agent %s", b))
		} else {
			out = append(out, at("agent", "agent updated from %s to %s", a, b))
		}
	}
	if old.OpenWrt != cur.OpenWrt && old.OpenWrt != "" && cur.OpenWrt != "" {
		out = append(out, at("firmware", "firmware changed from %s to %s", old.OpenWrt, cur.OpenWrt))
	}
	was := map[string]radioState{}
	for _, r := range old.Radios {
		was[r.Radio] = r
	}
	isUp := func(r radioState) bool { return r.Up == nil || *r.Up }
	for _, r := range cur.Radios {
		o, ok := was[r.Radio]
		if !ok || o.Band != r.Band {
			continue
		}
		name := radio.BandName(r.Band)
		switch {
		case isUp(o) && !isUp(r):
			out = append(out, at("radio", "%s radio went down", name))
		case !isUp(o) && isUp(r):
			out = append(out, at("radio", "%s radio came up", name))
		case isUp(r) && o.Channel != 0 && r.Channel != 0 && o.Channel != r.Channel:
			e := at("channel", "%s moved from channel %d to %d", name, o.Channel, r.Channel)
			if o.Width != r.Width && r.Width != 0 {
				e.Text += fmt.Sprintf(", now %d MHz", r.Width)
			}
			out = append(out, e)
		case isUp(r) && o.Width != 0 && r.Width != 0 && o.Width != r.Width:
			out = append(out, at("channel", "%s went from %d to %d MHz on channel %d", name, o.Width, r.Width, r.Channel))
		}
	}
	port := func(n *uplinkNeighbor) string {
		if n == nil || (n.System == "" && n.Port == "") {
			return ""
		}
		return n.System + " " + n.Port
	}
	if a, b := port(old.UplinkNeighbor), port(cur.UplinkNeighbor); a != b && a != "" && b != "" {
		out = append(out, at("uplink", "uplink moved from %s to %s", a, b))
	}
	return out
}

// apNode answers 404 for a node that is no AP the caller may view.
func apNode(c call, id hierarchy.NodeID) error {
	t := c.state.Org.Locations
	if n, ok := t.Node(id); !ok || n.Kind != hierarchy.KindAP || roleOn(c, change.Locations, t, id) < access.Viewer {
		return errNotFound
	}
	return nil
}

func hoursOf(r *http.Request, def int) (int, error) {
	q := r.URL.Query().Get("hours")
	if q == "" {
		return def, nil
	}
	n, err := strconv.Atoi(q)
	if err != nil || n < 1 || n > 720 {
		return 0, badRequest("hours: from 1 to 720")
	}
	return n, nil
}

// apHealthView is an AP's health over the last ?hours= (24 unless set, at
// most 720): GET /v1/aps/{ap}/health. now is its latest report's: how busy
// its processor was, its load and cores, memory, storage, temperature,
// processes and connections. points is the series, oldest first, at most
// 300: where there are more reports, each point is the worst of a run of
// them, the busiest processor, the least memory free, the hottest.
func (s *Server) apHealthView(w http.ResponseWriter, r *http.Request, c call) error {
	id := hierarchy.NodeID(r.PathValue("ap"))
	if err := apNode(c, id); err != nil {
		return err
	}
	hours, err := hoursOf(r, 24)
	if err != nil {
		return err
	}
	pts, err := s.conds.Health(id, time.Now().Add(-time.Duration(hours)*time.Hour))
	if err != nil {
		return err
	}
	type point struct {
		At      time.Time `json:"at"`
		CPU     *int      `json:"cpu"`
		Load    *float64  `json:"load"`
		Mem     *int      `json:"mem"` // percent of memory in use
		Temp    *float64  `json:"temp"`
		Clients int       `json:"clients"`
	}
	used := func(p conditions.HealthPoint) *int {
		if p.MemTotal == nil || p.MemAvailable == nil || *p.MemTotal <= 0 {
			return nil
		}
		v := int(100 - 100**p.MemAvailable / *p.MemTotal)
		return &v
	}
	step := (len(pts) + 299) / 300
	out := []point{}
	for i := 0; i < len(pts); i += max(step, 1) {
		q := point{At: pts[i].At, CPU: pts[i].CPU, Load: pts[i].Load, Mem: used(pts[i]), Temp: pts[i].Temp, Clients: pts[i].Clients}
		for _, p := range pts[i:min(i+max(step, 1), len(pts))] {
			if p.CPU != nil && (q.CPU == nil || *p.CPU > *q.CPU) {
				q.CPU = p.CPU
			}
			if p.Load != nil && (q.Load == nil || *p.Load > *q.Load) {
				q.Load = p.Load
			}
			if m := used(p); m != nil && (q.Mem == nil || *m > *q.Mem) {
				q.Mem = m
			}
			if p.Temp != nil && (q.Temp == nil || *p.Temp > *q.Temp) {
				q.Temp = p.Temp
			}
			q.Clients = max(q.Clients, p.Clients)
		}
		out = append(out, q)
	}
	res := map[string]any{"hours": hours, "points": out}
	// Its latest report's own words, which hold what the series leaves out.
	if st, err := s.conds.LatestState(id); err != nil {
		return err
	} else if st != nil {
		var rep struct {
			Uptime int64     `json:"uptime"`
			Health *apHealth `json:"health"`
		}
		if json.Unmarshal(st.Report, &rep) == nil {
			res["now"] = map[string]any{"at": st.At, "uptime": rep.Uptime, "health": rep.Health}
		}
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}

// apTimeline is what happened to an AP over the last ?hours= (168 unless
// set, at most 720), newest first, at most 300: GET /v1/aps/{ap}/timeline.
// Each entry has when, a kind and what, in words: what changed between its
// reports (restart, silence, agent, firmware, radio, channel, uplink,
// first); each config it applied or failed to (config); each alert that
// began or ended (alert); and each thing it was asked to do once (action).
// severity is set where it is trouble: critical, warning or info.
func (s *Server) apTimeline(w http.ResponseWriter, r *http.Request, c call) error {
	id := hierarchy.NodeID(r.PathValue("ap"))
	if err := apNode(c, id); err != nil {
		return err
	}
	hours, err := hoursOf(r, 168)
	if err != nil {
		return err
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	type entry struct {
		At       time.Time `json:"at"`
		Kind     string    `json:"kind"`
		Text     string    `json:"text"`
		Severity string    `json:"severity,omitempty"`
	}
	var out []entry
	events, err := s.conds.Events(id, since, 300)
	if err != nil {
		return err
	}
	for _, e := range events {
		sev := ""
		if e.Kind == "restart" || e.Kind == "silence" {
			sev = "warning"
		}
		out = append(out, entry{e.At, e.Kind, e.Text, sev})
	}
	applies, err := s.conds.AppliesSince(id, since, 300)
	if err != nil {
		return err
	}
	for _, a := range applies {
		if a.OK {
			out = append(out, entry{a.At, "config", fmt.Sprintf("applied config version %d", a.Version), ""})
		} else {
			out = append(out, entry{a.At, "config", fmt.Sprintf("could not apply config version %d: %s", a.Version, a.Error), "warning"})
		}
	}
	logged, err := s.conds.AlertLog(since, []hierarchy.NodeID{id}, 300)
	if err != nil {
		return err
	}
	for _, a := range logged {
		if !a.Began.Before(since) {
			out = append(out, entry{a.Began, "alert", a.Message, a.Severity})
		}
		if a.Ended != nil {
			out = append(out, entry{*a.Ended, "alert", "ended: " + a.Message, ""})
		}
	}
	actions, err := s.conds.Actions(id, 300)
	if err != nil {
		return err
	}
	for _, a := range actions {
		if a.At.Before(since) {
			continue
		}
		text := fmt.Sprintf("%s asked it to %s", a.Actor, a.Kind)
		if a.Target != "" {
			text += " " + a.Target
		}
		text += ": " + a.State
		if a.Result != "" {
			text += ", " + a.Result
		}
		out = append(out, entry{a.At, "action", text, ""})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	if len(out) > 300 {
		out = out[:300]
	}
	if out == nil {
		out = []entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"hours": hours, "events": out})
	return nil
}
