package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

const defaultStatePath = "~/.config/hdf/state.toml"

// State holds daemon-managed runtime state that changes frequently.
// It is kept separate from Config so the user-editable config.toml is not
// rewritten every sync cycle.
type State struct {
	LastSync       time.Time `toml:"last_sync"`
	LastCommit     string    `toml:"last_commit"`
	LastMainCommit string    `toml:"last_main_commit"`
	LastNotifiedAt time.Time `toml:"last_notified_at"`
	// LastFailureNotifyAt throttles fetch/push failure notifications so a
	// persistently failing sync alerts once per cooldown, not every cycle.
	LastFailureNotifyAt time.Time `toml:"last_failure_notify_at"`
	PendingWarnings     []string  `toml:"pending_warnings,omitempty"`
	// DeclinedOverwrites remembers per-file promote decisions: repo-relative
	// path → content hash of the main version the user declined to overwrite.
	// While main still holds exactly that content, promote keeps main's
	// version without re-prompting; new content on main prompts again.
	DeclinedOverwrites map[string]string `toml:"declined_overwrites,omitempty"`
}

// DefaultStatePath returns the default path to the hdf state file.
func DefaultStatePath() string {
	return ExpandPath(defaultStatePath)
}

// LoadState reads the state file at path. Returns an empty State if the file
// does not exist yet.
func LoadState(path string) (*State, error) {
	var s State
	if _, err := toml.DecodeFile(path, &s); err != nil {
		if os.IsNotExist(err) {
			return &State{}, nil
		}
		return nil, err
	}
	return &s, nil
}

// UpdateState applies fn to the state at path under an exclusive advisory
// file lock (<path>.lock), then saves atomically. The daemon and CLI both
// mutate state.toml; plain LoadState/SaveState pairs race each other and can
// lose updates (e.g. a warning appended by the daemon while the CLI clears
// warnings). All read-modify-write cycles must go through UpdateState.
func UpdateState(path string, fn func(*State) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	unlock, err := lockFile(path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	s, err := LoadState(path)
	if err != nil {
		return err
	}
	if err := fn(s); err != nil {
		return err
	}
	return SaveState(path, s)
}

// SaveState writes s to path atomically (via a temp file + rename), creating
// parent directories as needed.
func SaveState(path string, s *State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(s); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
