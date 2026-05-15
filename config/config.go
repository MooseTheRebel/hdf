package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const defaultConfigPath = "~/.config/hdf/config.toml"

// Config holds the user-editable hdf configuration stored in config.toml.
type Config struct {
	GitURL   string        `toml:"git_url"`
	RepoPath string        `toml:"repo_path"`
	Files    []ManagedFile `toml:"files"`
}

// ManagedFile records a dot file under hdf management and its last-known hash.
type ManagedFile struct {
	Path string `toml:"path"`
	Hash string `toml:"hash"`
}

// DefaultPath returns the default path to the hdf config file.
func DefaultPath() string {
	return ExpandPath(defaultConfigPath)
}

// ExpandPath replaces a leading ~ with the user's home directory.
// Returns path unchanged if the home directory cannot be determined.
func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// Load reads and parses the config file at path.
func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save writes cfg to path atomically (via a temp file + rename), creating
// parent directories as needed.
func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
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
