package config

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// TestUpdateStateConcurrent verifies that concurrent read-modify-write cycles
// through UpdateState do not lose updates — the daemon and CLI both mutate
// state.toml and previously raced each other with load/save pairs.
func TestUpdateStateConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.toml")
	const writers = 20

	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			errs <- UpdateState(path, func(s *State) error {
				s.PendingWarnings = append(s.PendingWarnings, fmt.Sprintf("warning-%d", n))
				return nil
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("UpdateState: %v", err)
		}
	}

	s, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.PendingWarnings) != writers {
		t.Errorf("got %d warnings, want %d — concurrent updates were lost", len(s.PendingWarnings), writers)
	}
}

// TestUpdateStateCreatesFile verifies UpdateState works when neither the state
// file nor its parent directory exists yet.
func TestUpdateStateCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.toml")
	if err := UpdateState(path, func(s *State) error {
		s.LastMainCommit = "abc123"
		return nil
	}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	s, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.LastMainCommit != "abc123" {
		t.Errorf("LastMainCommit = %q, want abc123", s.LastMainCommit)
	}
}

func TestSetPendingCrash_ThenTakePendingCrash_ReturnsAndClears(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.toml")
	if err := SetPendingCrash(path, "panic: boom"); err != nil {
		t.Fatalf("SetPendingCrash: %v", err)
	}

	got, err := TakePendingCrash(path)
	if err != nil {
		t.Fatalf("TakePendingCrash: %v", err)
	}
	if got != "panic: boom" {
		t.Errorf("TakePendingCrash = %q, want %q", got, "panic: boom")
	}

	// Second take must return empty — a crash is only ever surfaced once.
	got2, err := TakePendingCrash(path)
	if err != nil {
		t.Fatalf("TakePendingCrash (2nd): %v", err)
	}
	if got2 != "" {
		t.Errorf("TakePendingCrash (2nd) = %q, want empty", got2)
	}
}

func TestTakePendingCrash_NoneReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.toml")
	got, err := TakePendingCrash(path)
	if err != nil {
		t.Fatalf("TakePendingCrash: %v", err)
	}
	if got != "" {
		t.Errorf("TakePendingCrash = %q, want empty", got)
	}
}
