package daemon

import (
	"hdf/config"
	"hdf/repo"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testHostBranch = "test-hostabc123"

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

func repoWithCommit(t *testing.T, dir string) *repo.Repo {
	t.Helper()
	r, err := repo.Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("f.txt", "init"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}
	return r
}

func saveConfig(t *testing.T, cfg *config.Config) string {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	return cfgPath
}

func TestRunFailsWhenNoRemote(t *testing.T) {
	workDir := t.TempDir()
	repoWithCommit(t, workDir)

	cfgPath := saveConfig(t, &config.Config{LocalDotfilesDir: workDir, GitPushTarget: "", Branch: testHostBranch})

	err := Run(cfgPath)
	if err == nil {
		t.Fatal("expected error when no remote configured, got nil")
	}
	if !strings.Contains(err.Error(), "no remote configured") {
		t.Errorf("error = %q, want it to contain 'no remote configured'", err.Error())
	}
}

func TestSyncFailsWhenNoRemote(t *testing.T) {
	workDir := t.TempDir()
	repoWithCommit(t, workDir)

	cfgPath := saveConfig(t, &config.Config{LocalDotfilesDir: workDir, GitPushTarget: "", Branch: testHostBranch})
	statePath := filepath.Join(t.TempDir(), "state.toml")

	err := Sync(cfgPath, statePath)
	if err == nil {
		t.Fatal("expected error when no remote configured, got nil")
	}
	if !strings.Contains(err.Error(), "no remote configured") {
		t.Errorf("error = %q, want it to contain 'no remote configured'", err.Error())
	}
}

func TestSyncFailsWhenFetchFails(t *testing.T) {
	workDir := t.TempDir()
	r := repoWithCommit(t, workDir)

	// Point origin at a path that does not exist — fetch must fail.
	if err := r.AddRemote("origin", "file:///nonexistent/path/dotfiles-bare"); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}

	cfgPath := saveConfig(t, &config.Config{LocalDotfilesDir: workDir, GitPushTarget: "file:///nonexistent/path/dotfiles-bare", Branch: testHostBranch})
	statePath := filepath.Join(t.TempDir(), "state.toml")

	err := Sync(cfgPath, statePath)
	if err == nil {
		t.Fatal("expected error when fetch fails, got nil")
	}
	if !strings.Contains(err.Error(), "fetching from remote") {
		t.Errorf("error = %q, want it to contain 'fetching from remote'", err.Error())
	}
}
