package conditions

import (
	"path/filepath"
	"testing"
	"time"
)

func TestActions(t *testing.T) {
	now := time.Date(2026, 10, 10, 2, 0, 0, 0, time.UTC)
	s, err := Open(filepath.Join(t.TempDir(), "c.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, err := s.AddAction("ap-1", "locate", "griff")
	if err != nil || a.State != "pending" || a.Kind != "locate" {
		t.Fatalf("add = %+v, %v", a, err)
	}
	// Asked twice, it is the one action.
	if b, _ := s.AddAction("ap-1", "locate", "griff"); b.ID != a.ID {
		t.Fatalf("a second locate made %+v", b)
	}
	if p, _ := s.PendingActions("ap-1"); len(p) != 1 {
		t.Fatalf("pending = %+v", p)
	}
	// Another AP cannot finish it.
	if _, err := s.FinishAction("ap-2", a.ID, true, ""); err != ErrNoAction {
		t.Fatalf("another AP: %v", err)
	}
	done, err := s.FinishAction("ap-1", a.ID, true, "blinking for 60 s")
	if err != nil || done.State != "done" || done.DoneAt == nil || done.Result != "blinking for 60 s" {
		t.Fatalf("finish = %+v, %v", done, err)
	}
	// One that waits too long expires, and is not done late.
	r, _ := s.AddAction("ap-1", "reboot", "griff")
	now = now.Add(ActionWait + time.Minute)
	if p, _ := s.PendingActions("ap-1"); len(p) != 0 {
		t.Fatalf("pending after the wait = %+v", p)
	}
	if list, _ := s.Actions("ap-1", 10); len(list) != 2 || list[0].ID != r.ID || list[0].State != "expired" {
		t.Fatalf("actions = %+v", list)
	}
}
