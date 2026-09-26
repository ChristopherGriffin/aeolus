package conditions

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func open(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	clock := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	s, err := Open(filepath.Join(t.TempDir(), "conditions.db"), func() time.Time { return clock })
	must(t, err)
	t.Cleanup(func() { s.Close() })
	return s, &clock
}

func TestSeenAndRunning(t *testing.T) {
	s, clock := open(t)
	l, err := s.Latest("ap-1")
	must(t, err)
	if l.Seen != nil || l.Check != nil || l.Apply != nil || l.State != nil {
		t.Fatalf("new AP has records: %+v", l)
	}
	must(t, s.Seen("ap-1", "192.168.1.38"))
	*clock = clock.Add(time.Minute)
	must(t, s.Running("ap-1", 12))
	must(t, s.Seen("ap-1", "192.168.1.39"))
	l, err = s.Latest("ap-1")
	must(t, err)
	if l.Seen.Source != "192.168.1.39" || !l.Seen.At.Equal(*clock) || l.Seen.Running == nil || *l.Seen.Running != 12 {
		t.Fatalf("seen = %+v", l.Seen)
	}
}

func TestApplyKnowsWhetherItWasChecked(t *testing.T) {
	s, _ := open(t)
	must(t, s.RecordCheck("ap-1", Check{Version: 12, Hash: "aa", Result: Refused, Problems: []string{"x"}}))
	must(t, s.RecordCheck("ap-1", Check{Version: 12, Hash: "bb", Result: OK}))
	for hash, want := range map[string]bool{"aa": false, "bb": true, "cc": false} {
		a, err := s.RecordApply("ap-1", Apply{Version: 12, Hash: hash, OK: true})
		must(t, err)
		if a.Checked != want {
			t.Errorf("apply of %s: checked %v, want %v", hash, a.Checked, want)
		}
	}
	if a, _ := s.RecordApply("ap-2", Apply{Version: 12, Hash: "bb", OK: true}); a.Checked {
		t.Error("another AP's check covered an apply")
	}
	h, err := s.History("ap-1", 10)
	must(t, err)
	if len(h.Checks) != 2 || h.Checks[0].Hash != "bb" || len(h.Checks[0].Problems) != 0 || h.Checks[1].Problems[0] != "x" || len(h.Applies) != 3 {
		t.Fatalf("history = %+v", h)
	}
}

func TestChecksAndAppliesAreKept(t *testing.T) {
	s, _ := open(t)
	must(t, s.RecordCheck("ap-1", Check{Version: 1, Hash: "aa", Result: OK}))
	_, err := s.RecordApply("ap-1", Apply{Version: 1, Hash: "aa", OK: true})
	must(t, err)
	for _, q := range []string{`DELETE FROM checks`, `UPDATE checks SET result = 'ok'`, `DELETE FROM applies`, `UPDATE applies SET ok = 0`} {
		if _, err := s.db.Exec(q); err == nil || !strings.Contains(err.Error(), "kept as recorded") {
			t.Errorf("%s: %v", q, err)
		}
	}
}

func TestStatesAreTrimmed(t *testing.T) {
	s, clock := open(t)
	must(t, s.RecordState("ap-1", 1, json.RawMessage(`{"uptime":1}`)))
	*clock = clock.Add(20 * 24 * time.Hour)
	must(t, s.RecordState("ap-1", 2, json.RawMessage(`{"uptime":2}`)))
	*clock = clock.Add(15 * 24 * time.Hour)
	n, err := s.TrimStates(30 * 24 * time.Hour)
	must(t, err)
	if n != 1 {
		t.Fatalf("trimmed %d", n)
	}
	l, err := s.Latest("ap-1")
	must(t, err)
	if l.State == nil || l.State.Version != 2 || string(l.State.Report) != `{"uptime":2}` {
		t.Fatalf("latest state = %+v", l.State)
	}
}

func TestReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conditions.db")
	s, err := Open(path, nil)
	must(t, err)
	must(t, s.Seen("ap-1", "10.0.0.1"))
	must(t, s.Close())
	s, err = Open(path, nil)
	must(t, err)
	defer s.Close()
	if l, _ := s.Latest("ap-1"); l.Seen == nil {
		t.Fatal("lost on reopen")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
