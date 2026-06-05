package config

import (
	"bytes"
	"path"

	"github.com/BurntSushi/toml"
)

const (
	// DefaultNotifyThreshold is the hunk count at or above which the daemon
	// sends a drift notification.
	DefaultNotifyThreshold = 3

	// DefaultSyncIntervalMinutes is the default interval between daemon sync
	// cycles when no SharedSettings are available on the main branch.
	DefaultSyncIntervalMinutes = 30

	// DefaultNotifyCooldownMinutes is the minimum number of minutes between
	// successive drift notifications for the same host.
	DefaultNotifyCooldownMinutes = 240

	// SharedSettingsFile is the repo-relative path where SharedSettings is
	// stored on the main branch.
	SharedSettingsFile = ".hdf/settings.toml"
)

// DefaultIgnoredPaths is the out-of-the-box blocklist enforced at enroll time.
// It covers common locations for credentials and private keys. Operators can
// replace this list via SharedSettings on the main branch.
var DefaultIgnoredPaths = []string{
	"~/.ssh/*",
	"~/.gnupg/*",
	"~/.aws/credentials",
	"~/.config/hdf/*",
}

// SharedSettings is stored at SharedSettingsFile on the main branch and is
// read by every host on every sync cycle. It controls behaviour that must be
// consistent across all machines in the same dotfiles repo.
type SharedSettings struct {
	NotifyThreshold       int      `toml:"notify_threshold"`
	SyncIntervalMinutes   int      `toml:"sync_interval_minutes"`
	NotifyCooldownMinutes int      `toml:"notify_cooldown_minutes"`
	IgnoredPaths          []string `toml:"ignored_paths"`
}

// DefaultSharedSettings returns a SharedSettings populated with package defaults.
func DefaultSharedSettings() *SharedSettings {
	return &SharedSettings{
		NotifyThreshold:       DefaultNotifyThreshold,
		SyncIntervalMinutes:   DefaultSyncIntervalMinutes,
		NotifyCooldownMinutes: DefaultNotifyCooldownMinutes,
		IgnoredPaths:          DefaultIgnoredPaths,
	}
}

// ApplyDefaults fills in any zero/nil values with the package defaults.
// A nil IgnoredPaths means the field was absent in TOML (use the default
// blocklist). An explicit empty slice means the operator intentionally cleared
// the list and is left untouched.
func (s *SharedSettings) ApplyDefaults() {
	if s.NotifyThreshold <= 0 {
		s.NotifyThreshold = DefaultNotifyThreshold
	}
	if s.SyncIntervalMinutes <= 0 {
		s.SyncIntervalMinutes = DefaultSyncIntervalMinutes
	}
	if s.NotifyCooldownMinutes <= 0 {
		s.NotifyCooldownMinutes = DefaultNotifyCooldownMinutes
	}
	if s.IgnoredPaths == nil {
		s.IgnoredPaths = DefaultIgnoredPaths
	}
}

// IsIgnored reports whether path matches any of the glob patterns.
// path and patterns are expected in ~/... form (e.g. "~/.ssh/id_rsa").
// A single-level wildcard (*) matches any filename but not a path separator,
// so "~/.ssh/*" blocks "~/.ssh/id_rsa" but not "~/.ssh/sub/key".
func IsIgnored(p string, patterns []string) bool {
	for _, pat := range patterns {
		if matched, _ := path.Match(pat, p); matched {
			return true
		}
	}
	return false
}

// SharedSettingsFromBytes parses TOML-encoded bytes into a SharedSettings.
// Call ApplyDefaults after parsing to fill in any omitted fields.
func SharedSettingsFromBytes(data []byte) (*SharedSettings, error) {
	var s SharedSettings
	if _, err := toml.Decode(string(data), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// SharedSettingsToBytes serialises s to TOML bytes.
func SharedSettingsToBytes(s *SharedSettings) ([]byte, error) {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(s); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
