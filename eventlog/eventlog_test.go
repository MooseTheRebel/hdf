package eventlog

import (
	"path/filepath"
	"testing"
)

func TestPathFor(t *testing.T) {
	got := PathFor("/home/u/.config/hdf/state.toml")
	want := "/home/u/.config/hdf/events.log"
	if got != want {
		t.Errorf("PathFor() = %q, want %q", got, want)
	}
}

func TestReadAll_MissingFileReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.log")
	entries, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

func TestAppend_CreatesFileAndReadsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.log")
	if err := Append(path, "sync_start", ""); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := Append(path, "sync_error", "fetch failed"); err != nil {
		t.Fatalf("Append: %v", err)
	}
	entries, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Event != "sync_start" || entries[1].Event != "sync_error" || entries[1].Detail != "fetch failed" {
		t.Errorf("entries = %+v, want sync_start then sync_error/fetch failed", entries)
	}
}

func TestAppend_TrimsToMaxEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.log")
	for i := 0; i < MaxEntries+10; i++ {
		if err := Append(path, "event", ""); err != nil {
			t.Fatalf("Append #%d: %v", i, err)
		}
	}
	entries, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != MaxEntries {
		t.Errorf("got %d entries, want %d (trimmed)", len(entries), MaxEntries)
	}
}
