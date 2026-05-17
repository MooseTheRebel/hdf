package daemon

import (
	"hdf/config"
	"hdf/repo"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunFailsWhenNotInitialized(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.toml")

	err := Run(cfgPath)
	if err == nil {
		t.Fatal("expected error when config missing, got nil")
	}
	if !strings.Contains(err.Error(), "hdf is not initialized") {
		t.Errorf("error = %q, want it to contain 'hdf is not initialized'", err.Error())
	}
}

func TestRunFailsWhenNoRemote(t *testing.T) {
	workDir := t.TempDir()
	r, err := repo.Init(workDir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Commit so the repo is valid but has no origin remote.
	if err := os.WriteFile(filepath.Join(workDir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("f.txt", "init"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	cfg := &config.Config{LocalDotfilesDir: workDir, GitPushTarget: "", Branch: "test-host"}
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	err = Run(cfgPath)
	if err == nil {
		t.Fatal("expected error when no remote configured, got nil")
	}
	if !strings.Contains(err.Error(), "no remote configured") {
		t.Errorf("error = %q, want it to contain 'no remote configured'", err.Error())
	}
}
