// Package eventlog records a bounded, rolling history of state transitions
// (daemon sync cycles, crashes, panics) for inclusion in hdf error reports.
package eventlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// MaxEntries bounds the log so it stays small enough to embed in a report
// without growing unbounded over the life of an install.
const MaxEntries = 200

// Entry is one recorded state transition.
type Entry struct {
	Time   time.Time `json:"time"`
	Event  string    `json:"event"`
	Detail string    `json:"detail"`
}

// PathFor returns the event log path as a sibling of statePath (e.g.
// ~/.config/hdf/state.toml -> ~/.config/hdf/events.log), so callers and
// tests that already have a statePath get an isolated event log for free.
func PathFor(statePath string) string {
	return filepath.Join(filepath.Dir(statePath), "events.log")
}

// Append records one event, trimming the log to the most recent MaxEntries
// entries. Each call rewrites the whole file; hdf's event volume (daemon
// sync cycles every 30 minutes, occasional crashes) is far too low for that
// to matter.
func Append(path, event, detail string) error {
	entries, err := ReadAll(path)
	if err != nil {
		return err
	}
	entries = append(entries, Entry{Time: time.Now(), Event: event, Detail: detail})
	if len(entries) > MaxEntries {
		entries = entries[len(entries)-MaxEntries:]
	}
	return writeAll(path, entries)
}

// ReadAll returns all entries currently in the log at path, oldest first.
// A missing file returns an empty slice, not an error.
func ReadAll(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("parsing event log line: %w", err)
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// writeAll atomically overwrites path with entries, one JSON object per line.
func writeAll(path string, entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
