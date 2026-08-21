package cli

import (
	"hdf/config"
	"hdf/link"
	"hdf/repo"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestComputeStatus(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := filepath.Join(homeDir, "dotfiles")

	r, err := repo.Init(repoDir)
	if err != nil {
		t.Fatalf("repo.Init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "seed.txt"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("seed.txt", "initial"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}
	branch, err := r.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}

	okContent := []byte("in sync\n")
	if err := os.WriteFile(filepath.Join(homeDir, ".okrc"), okContent, 0o644); err != nil {
		t.Fatal(err)
	}
	reg := &config.Registry{Files: []config.ManagedFile{
		{Path: "~/.okrc", Hash: link.HashBytes(okContent)},
		{Path: tildeMissingRC, Hash: "whatever"},
	}}
	if err := config.SaveRegistry(repoDir, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	cfgPath := filepath.Join(homeDir, "config.toml")
	cfg := &config.Config{
		GitPushTarget:    "file:///tmp/bare",
		LocalDotfilesDir: repoDir,
		Branch:           "test-branch",
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("config.Save: %v", err)
	}

	statePath := filepath.Join(homeDir, "state.toml")
	lastSync := time.Date(2026, 7, 26, 10, 30, 0, 0, time.UTC)
	if err := config.SaveState(statePath, &config.State{LastCommit: "abc123", LastSync: lastSync}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	got, err := computeStatus(cfgPath, statePath, homeDir)
	if err != nil {
		t.Fatalf("computeStatus: %v", err)
	}

	if got.GitPushTarget != "file:///tmp/bare" {
		t.Errorf("GitPushTarget = %q, want %q", got.GitPushTarget, "file:///tmp/bare")
	}
	if got.LocalDotfilesDir != repoDir {
		t.Errorf("LocalDotfilesDir = %q, want %q", got.LocalDotfilesDir, repoDir)
	}
	if got.Branch != branch {
		t.Errorf("Branch = %q, want %q", got.Branch, branch)
	}
	if got.LastCommit != "abc123" {
		t.Errorf("LastCommit = %q, want %q", got.LastCommit, "abc123")
	}
	if got.LastSync != "2026-07-26 10:30:00" {
		t.Errorf("LastSync = %q, want %q", got.LastSync, "2026-07-26 10:30:00")
	}

	want := []FileStatus{
		{Path: "~/.okrc", Status: statusOk},
		{Path: tildeMissingRC, Status: statusMissing},
	}
	if len(got.Files) != len(want) {
		t.Fatalf("Files = %+v, want %d entries", got.Files, len(want))
	}
	for i, f := range want {
		if got.Files[i] != f {
			t.Errorf("Files[%d] = %+v, want %+v", i, got.Files[i], f)
		}
	}
}

func TestComputeStatus_MissingConfigReturnsError(t *testing.T) {
	homeDir := t.TempDir()
	_, err := computeStatus(filepath.Join(homeDir, "no-such-config.toml"), filepath.Join(homeDir, "state.toml"), homeDir)
	if err == nil {
		t.Fatal("computeStatus: want error for missing config, got nil")
	}
}
