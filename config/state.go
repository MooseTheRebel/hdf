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
	LastSync   time.Time `toml:"last_sync"`
	LastCommit string    `toml:"last_commit"`
}

func DefaultStatePath() string {
	return ExpandPath(defaultStatePath)
}

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

func SaveState(path string, s *State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(s)
}
