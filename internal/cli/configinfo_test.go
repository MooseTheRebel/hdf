package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeConfigInfo_Exists(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := "git_push_target = \"file:///tmp/bare\"\nlocal_dotfiles_dir = \"/tmp/repo\"\nbranch = \"test-host\"\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := computeConfigInfo(cfgPath)
	if err != nil {
		t.Fatalf("computeConfigInfo: %v", err)
	}
	if !got.Exists {
		t.Error("Exists = false, want true")
	}
	if got.Path != cfgPath {
		t.Errorf("Path = %q, want %q", got.Path, cfgPath)
	}
	if got.Content != content {
		t.Errorf("Content = %q, want %q", got.Content, content)
	}
}

func TestComputeConfigInfo_MissingFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "no-such-config.toml")

	got, err := computeConfigInfo(cfgPath)
	if err != nil {
		t.Fatalf("computeConfigInfo: %v", err)
	}
	if got.Exists {
		t.Error("Exists = true, want false")
	}
	if got.Content != "" {
		t.Errorf("Content = %q, want empty", got.Content)
	}
	if got.Path != cfgPath {
		t.Errorf("Path = %q, want %q", got.Path, cfgPath)
	}
}

func TestComputeConfigInfo_PermissionDeniedReturnsError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses DAC — permission test not meaningful")
	}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("branch = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfgPath, 0o644) }) //nolint:gosec // restoring test file to a removable state

	_, err := computeConfigInfo(cfgPath)
	if err == nil {
		t.Fatal("computeConfigInfo: want error for unreadable file, got nil")
	}
}
