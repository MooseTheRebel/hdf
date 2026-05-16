package main

import (
	"bytes"
	"hdf/config"
	"hdf/repo"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// initPaths returns temp cfgPath and statePath inside a single temp dir.
func initPaths(t *testing.T) (cfgPath, statePath string) {
	t.Helper()
	d := t.TempDir()
	return filepath.Join(d, "config.toml"), filepath.Join(d, "state.toml")
}

// makeFixtureRepo creates a local git repo with one committed file, suitable
// for use as a clone source in tests.
func makeFixtureRepo(t *testing.T) string {
	t.Helper()
	srcDir := t.TempDir()
	src, err := repo.Init(srcDir)
	if err != nil {
		t.Fatalf("init fixture repo: %v", err)
	}
	dotfile := filepath.Join(srcDir, ".bashrc")
	if err := os.WriteFile(dotfile, []byte("export PATH=$PATH:~/bin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := src.CommitFile(".bashrc", "add .bashrc"); err != nil {
		t.Fatalf("commit fixture: %v", err)
	}
	return srcDir
}

func TestRunInitLocalNewRepo(t *testing.T) {
	repoDir := t.TempDir()
	cfgPath, statePath := initPaths(t)

	if err := runInit(strings.NewReader("1\n"+repoDir+"\n"), cfgPath, statePath, ""); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err != nil {
		t.Errorf("expected .git dir in repo: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.RepoPath != repoDir {
		t.Errorf("RepoPath = %q, want %q", cfg.RepoPath, repoDir)
	}
	if cfg.GitURL != repoDir {
		t.Errorf("GitURL = %q, want %q", cfg.GitURL, repoDir)
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		t.Fatalf("loading state: %v", err)
	}
	if state.LastCommit == "" {
		t.Error("LastCommit should be set after init")
	}
}

func TestRunInitLocalExistingRepo(t *testing.T) {
	repoDir := t.TempDir()
	cfgPath, statePath := initPaths(t)

	// First init creates the repo.
	if err := runInit(strings.NewReader("1\n"+repoDir+"\n"), cfgPath, statePath, ""); err != nil {
		t.Fatalf("first runInit: %v", err)
	}

	// Second init on the same path should open (not re-init) without error.
	cfg2Path, state2Path := initPaths(t)
	if err := runInit(strings.NewReader("1\n"+repoDir+"\n"), cfg2Path, state2Path, ""); err != nil {
		t.Fatalf("second runInit (existing repo): %v", err)
	}

	cfg, err := config.Load(cfg2Path)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.RepoPath != repoDir {
		t.Errorf("RepoPath = %q, want %q", cfg.RepoPath, repoDir)
	}
}

// TestRunInitEmptyChoiceDefaultsToLocal verifies that pressing Enter (empty
// input) silently defaults to option 1 and clearly informs the user via
// printed output, then proceeds to create a local repo at the given path.
func TestRunInitEmptyChoiceDefaultsToLocal(t *testing.T) {
	repoDir := t.TempDir()
	cfgPath, statePath := initPaths(t)

	// Capture stdout so we can assert the "defaulting" message is printed.
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runInit(strings.NewReader("\n"+repoDir+"\n"), cfgPath, statePath, "")

	_ = w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if err != nil {
		t.Fatalf("runInit with empty choice: %v", err)
	}

	if !strings.Contains(output, "efaulting") {
		t.Errorf("expected output to mention defaulting, got:\n%s", output)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.RepoPath != repoDir {
		t.Errorf("RepoPath = %q, want %q", cfg.RepoPath, repoDir)
	}
}

func TestRunInitLocalRelativePathConfirmed(t *testing.T) {
	workDir := t.TempDir()
	t.Chdir(workDir) // relative paths resolve under workDir, not the project root

	cfgPath, statePath := initPaths(t)

	// stdin: choice 1 → relative name → confirm with "y"
	if err := runInit(strings.NewReader("1\ndotfiles\ny\n"), cfgPath, statePath, ""); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	absRepoPath := filepath.Join(workDir, "dotfiles")
	if _, err := os.Stat(filepath.Join(absRepoPath, ".git")); err != nil {
		t.Errorf("expected .git in resolved path %s: %v", absRepoPath, err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.RepoPath != absRepoPath {
		t.Errorf("RepoPath = %q, want %q", cfg.RepoPath, absRepoPath)
	}
}

func TestRunInitLocalRelativePathRejected(t *testing.T) {
	t.Chdir(t.TempDir())
	cfgPath, statePath := initPaths(t)

	// stdin: choice 1 → relative name → reject with "n"
	err := runInit(strings.NewReader("1\ndotfiles\nn\n"), cfgPath, statePath, "")
	if err == nil {
		t.Fatal("expected error when user rejects relative path, got nil")
	}
	if !strings.Contains(err.Error(), "aborted") {
		t.Errorf("error = %q, want it to contain 'aborted'", err.Error())
	}

	// Config must not have been written.
	if _, statErr := os.Stat(cfgPath); statErr == nil {
		t.Error("config file should not exist after rejection")
	}
}

func TestRunInitInvalidChoice(t *testing.T) {
	cfgPath, statePath := initPaths(t)

	err := runInit(strings.NewReader("9\n"), cfgPath, statePath, "")
	if err == nil {
		t.Fatal("expected error for invalid choice, got nil")
	}
	if !strings.Contains(err.Error(), "invalid choice") {
		t.Errorf("error = %q, want it to contain 'invalid choice'", err.Error())
	}
}

func TestRunInitRemoteEmptyURL(t *testing.T) {
	cfgPath, statePath := initPaths(t)

	err := runInit(strings.NewReader("2\n\n"), cfgPath, statePath, "")
	if err == nil {
		t.Fatal("expected error for empty remote URL, got nil")
	}
	if !strings.Contains(err.Error(), "remote git URL cannot be empty") {
		t.Errorf("error = %q, want it to contain 'remote git URL cannot be empty'", err.Error())
	}
}

// TestRunInitRemoteClone verifies that choosing option 2 with a valid local
// git URL successfully clones the repository, writes the config and state, and
// makes the committed files accessible in the clone destination.
func TestRunInitRemoteClone(t *testing.T) {
	srcDir := makeFixtureRepo(t)

	cloneDir := filepath.Join(t.TempDir(), "clone")
	cfgPath, statePath := initPaths(t)

	if err := runInit(strings.NewReader("2\n"+srcDir+"\n"), cfgPath, statePath, cloneDir); err != nil {
		t.Fatalf("runInit remote clone: %v", err)
	}

	// Clone destination must be a valid git repo.
	if _, err := os.Stat(filepath.Join(cloneDir, ".git")); err != nil {
		t.Errorf("expected .git in clone dir: %v", err)
	}

	// The fixture file committed in the source must be present in the clone.
	if _, err := os.Stat(filepath.Join(cloneDir, ".bashrc")); err != nil {
		t.Errorf("expected .bashrc in cloned repo: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.RepoPath != cloneDir {
		t.Errorf("RepoPath = %q, want %q", cfg.RepoPath, cloneDir)
	}
	if cfg.GitURL != srcDir {
		t.Errorf("GitURL = %q, want %q", cfg.GitURL, srcDir)
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		t.Fatalf("loading state: %v", err)
	}
	if state.LastCommit == "" {
		t.Error("LastCommit should be set after cloning")
	}
}

func TestRootCmdSilenceErrors(t *testing.T) {
	if !rootCmd.SilenceErrors {
		t.Error("rootCmd.SilenceErrors must be true to prevent Cobra from double-printing errors")
	}
}

func TestRootCmdSilenceUsage(t *testing.T) {
	if !rootCmd.SilenceUsage {
		t.Error("rootCmd.SilenceUsage must be true to suppress usage output on runtime errors")
	}
}
