package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const defaultConfigPath = "~/.config/hdf/config.toml"

type Config struct {
	GitURL   string        `toml:"git_url"`
	RepoPath string        `toml:"repo_path"`
	Files    []ManagedFile `toml:"files"`
}

type ManagedFile struct {
	Path string `toml:"path"`
	Hash string `toml:"hash"`
}

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

func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
