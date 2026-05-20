package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	defaultConfigPath = "~/.config/hdf/config.toml"
	managedFileName   = ".hdf/managed.toml"
)

// Config holds machine-local hdf settings stored in config.toml.
// It no longer contains the managed-files list — that lives in the repo
// at .hdf/managed.toml (see Registry).
type Config struct {
	GitPushTarget    string `toml:"git_push_target"`
	LocalDotfilesDir string `toml:"local_dotfiles_dir"`
	Branch           string `toml:"branch"`
}

// Registry is the in-repo file registry stored at <repo>/.hdf/managed.toml.
// It is committed to git and shared across machines.
type Registry struct {
	Files []ManagedFile `toml:"files"`
}

// ManagedFile records a dot file under hdf management.
// Hash is the current hash for non-variant files; empty when Variants is set.
type ManagedFile struct {
	Path     string    `toml:"path"`
	Hash     string    `toml:"hash"`
	Variants []Variant `toml:"variants,omitempty"`
}

// Variant describes a machine-specific mapping for a managed file.
// Branch must match cfg.Branch exactly (1:1).
type Variant struct {
	Branch   string `toml:"branch"`
	RepoPath string `toml:"repo_path"` // relative path within repo
	Hash     string `toml:"hash"`
}

// legacyConfig is used only during migration to read the old Files field.
type legacyConfig struct {
	GitPushTarget    string        `toml:"git_push_target"`
	LocalDotfilesDir string        `toml:"local_dotfiles_dir"`
	Branch           string        `toml:"branch"`
	Files            []ManagedFile `toml:"files"`
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

// LoadRegistry reads the registry from <repoDir>/.hdf/managed.toml.
// Returns an empty Registry if the file does not exist yet.
func LoadRegistry(repoDir string) (*Registry, error) {
	path := filepath.Join(repoDir, managedFileName)
	var reg Registry
	if _, err := toml.DecodeFile(path, &reg); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Registry{}, nil
		}
		return nil, err
	}
	return &reg, nil
}

// SaveRegistry writes reg to <repoDir>/.hdf/managed.toml atomically.
func SaveRegistry(repoDir string, reg *Registry) error {
	path := filepath.Join(repoDir, managedFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(reg); err != nil {
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

// MigrateFilesToRegistry reads any Files from an old-style config.toml and
// moves them into <repoDir>/.hdf/managed.toml. It is a no-op if config.toml
// has no files or if managed.toml already exists.
func MigrateFilesToRegistry(cfgPath, repoDir string) error {
	managedPath := filepath.Join(repoDir, managedFileName)
	if _, err := os.Stat(managedPath); err == nil {
		return nil // managed.toml already exists
	}
	var legacy legacyConfig
	if _, err := toml.DecodeFile(cfgPath, &legacy); err != nil {
		return err
	}
	if len(legacy.Files) == 0 {
		return nil
	}
	reg := &Registry{Files: legacy.Files}
	return SaveRegistry(repoDir, reg)
}
