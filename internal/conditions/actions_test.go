package conditions

import (
	"path/filepath"
	"sync"
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
	// Taken up, it is running, and never handed out again: a lost report
	// of it does not have the AP do it twice.
	if c, _ := s.ClaimActions("ap-1"); len(c) != 1 || c[0].State != "running" {
		t.Fatalf("claim = %+v", c)
	}
	if c, _ := s.ClaimActions("ap-1"); len(c) != 0 {
		t.Fatalf("claimed again = %+v", c)
	}
	// While it runs, asking again gives the one running, not another.
	if b, _ := s.AddAction("ap-1", "locate", "griff"); b.ID != a.ID || b.State != "running" {
		t.Fatalf("a locate while one runs = %+v", b)
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

// Asks racing the AP taking up and finishing what it is asked (0104): every
// ask gives an action, and never are two of a kind open at once.
func TestAddActionRacesTheAP(t *testing.T) {
	now := time.Date(2026, 10, 10, 2, 0, 0, 0, time.UTC)
	s, err := Open(filepath.Join(t.TempDir(), "c.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	stop := make(chan struct{})
	ap := make(chan error, 1)
	go func() {
		for {
			select {
			case <-stop:
				ap <- nil
				return
			default:
			}
			got, err := s.ClaimActions("ap-1")
			if err != nil {
				ap <- err
				return
			}
			for _, a := range got {
				if _, err := s.FinishAction("ap-1", a.ID, true, ""); err != nil {
					ap <- err
					return
				}
			}
		}
	}()
	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 25 {
				if a, err := s.AddAction("ap-1", "reboot", "griff"); err != nil || a.ID == 0 {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(stop)
	if err := <-ap; err != nil {
		t.Fatalf("the AP: %v", err)
	}
	close(errs)
	for err := range errs {
		t.Fatalf("an ask failed: %v", err)
	}
	var open int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM actions WHERE state IN ('pending', 'running')`).Scan(&open); err != nil || open > 1 {
		t.Fatalf("%d open, %v", open, err)
	}
}
