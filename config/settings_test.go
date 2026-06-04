package config

import "testing"

func TestSharedSettingsRoundTrip(t *testing.T) {
	want := &SharedSettings{
		NotifyThreshold:       5,
		SyncIntervalMinutes:   15,
		NotifyCooldownMinutes: 120,
		IgnoredPaths:          DefaultIgnoredPaths[:2],
	}

	data, err := SharedSettingsToBytes(want)
	if err != nil {
		t.Fatalf("SharedSettingsToBytes: %v", err)
	}

	got, err := SharedSettingsFromBytes(data)
	if err != nil {
		t.Fatalf("SharedSettingsFromBytes: %v", err)
	}
	if got.NotifyThreshold != want.NotifyThreshold {
		t.Errorf("NotifyThreshold: got %d, want %d", got.NotifyThreshold, want.NotifyThreshold)
	}
	if got.SyncIntervalMinutes != want.SyncIntervalMinutes {
		t.Errorf("SyncIntervalMinutes: got %d, want %d", got.SyncIntervalMinutes, want.SyncIntervalMinutes)
	}
	if got.NotifyCooldownMinutes != want.NotifyCooldownMinutes {
		t.Errorf("NotifyCooldownMinutes: got %d, want %d", got.NotifyCooldownMinutes, want.NotifyCooldownMinutes)
	}
	if len(got.IgnoredPaths) != len(want.IgnoredPaths) {
		t.Errorf("IgnoredPaths len: got %d, want %d", len(got.IgnoredPaths), len(want.IgnoredPaths))
	}
}

func TestSharedSettingsApplyDefaults(t *testing.T) {
	s := &SharedSettings{}
	s.ApplyDefaults()
	if s.NotifyThreshold != DefaultNotifyThreshold {
		t.Errorf("NotifyThreshold: got %d, want %d", s.NotifyThreshold, DefaultNotifyThreshold)
	}
	if s.SyncIntervalMinutes != DefaultSyncIntervalMinutes {
		t.Errorf("SyncIntervalMinutes: got %d, want %d", s.SyncIntervalMinutes, DefaultSyncIntervalMinutes)
	}
	if s.NotifyCooldownMinutes != DefaultNotifyCooldownMinutes {
		t.Errorf("NotifyCooldownMinutes: got %d, want %d", s.NotifyCooldownMinutes, DefaultNotifyCooldownMinutes)
	}
	if len(s.IgnoredPaths) == 0 {
		t.Error("IgnoredPaths: expected default list, got empty")
	}
}

func TestSharedSettingsApplyDefaultsExplicitEmpty(t *testing.T) {
	// An explicit empty slice must be preserved — the operator intentionally
	// cleared the blocklist.
	s := &SharedSettings{IgnoredPaths: []string{}}
	s.ApplyDefaults()
	if s.IgnoredPaths == nil || len(s.IgnoredPaths) != 0 {
		t.Errorf("explicit empty IgnoredPaths should be preserved, got %v", s.IgnoredPaths)
	}
}

func TestDefaultSharedSettings(t *testing.T) {
	s := DefaultSharedSettings()
	if s.NotifyThreshold != DefaultNotifyThreshold {
		t.Errorf("NotifyThreshold: got %d, want %d", s.NotifyThreshold, DefaultNotifyThreshold)
	}
	if s.SyncIntervalMinutes != DefaultSyncIntervalMinutes {
		t.Errorf("SyncIntervalMinutes: got %d, want %d", s.SyncIntervalMinutes, DefaultSyncIntervalMinutes)
	}
	if s.NotifyCooldownMinutes != DefaultNotifyCooldownMinutes {
		t.Errorf("NotifyCooldownMinutes: got %d, want %d", s.NotifyCooldownMinutes, DefaultNotifyCooldownMinutes)
	}
}

func TestSharedSettingsFromBytesEmpty(t *testing.T) {
	s, err := SharedSettingsFromBytes([]byte(""))
	if err != nil {
		t.Fatalf("SharedSettingsFromBytes empty: %v", err)
	}
	s.ApplyDefaults()
	if s.NotifyThreshold != DefaultNotifyThreshold {
		t.Errorf("empty bytes should default notify_threshold to %d, got %d", DefaultNotifyThreshold, s.NotifyThreshold)
	}
	if s.SyncIntervalMinutes != DefaultSyncIntervalMinutes {
		t.Errorf("empty bytes should default sync_interval_minutes to %d, got %d", DefaultSyncIntervalMinutes, s.SyncIntervalMinutes)
	}
	if s.NotifyCooldownMinutes != DefaultNotifyCooldownMinutes {
		t.Errorf("empty bytes should default notify_cooldown_minutes to %d, got %d", DefaultNotifyCooldownMinutes, s.NotifyCooldownMinutes)
	}
}

func TestIsIgnored(t *testing.T) {
	// Use the package defaults so this test stays in sync with changes to the
	// default blocklist without repeating the literal strings.
	patterns := DefaultIgnoredPaths

	blocked := []string{
		"~/.ssh/id_ed25519",
		"~/.ssh/known_hosts",
		"~/.gnupg/secring.gpg",
		"~/.aws/credentials",
		"~/.config/hdf/config.toml",
	}
	for _, p := range blocked {
		if !IsIgnored(p, patterns) {
			t.Errorf("IsIgnored(%q) = false, want true", p)
		}
	}

	allowed := []string{
		"~/.bashrc",
		"~/.vimrc",
		"~/.config/fish/config.fish",
		"~/.ssh",        // directory itself, no wildcard match
		"~/.aws/config", // only credentials is blocked, not the whole dir
	}
	for _, p := range allowed {
		if IsIgnored(p, patterns) {
			t.Errorf("IsIgnored(%q) = true, want false", p)
		}
	}
}

func TestIsIgnoredEmptyPatterns(t *testing.T) {
	if IsIgnored("~/.ssh/id_ed25519", []string{}) {
		t.Error("IsIgnored with empty patterns should return false")
	}
}
