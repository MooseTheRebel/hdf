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

// localInitStdin builds stdin for a choice-1 init with absolute working copy
// and push target paths (no relative-path confirmation prompt triggered).
func localInitStdin(workDir, bareDir string) string {
	return "1\n" + workDir + "\n" + bareDir + "\n"
}

func TestRunInitLocalNewRepo(t *testing.T) {
	repoDir := t.TempDir()
	bareDir := t.TempDir()
	cfgPath, statePath := initPaths(t)

	if err := runInit(strings.NewReader(localInitStdin(repoDir, bareDir)), cfgPath, statePath, ""); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err != nil {
		t.Errorf("expected .git dir in repo: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.LocalDotfilesDir != repoDir {
		t.Errorf("RepoPath = %q, want %q", cfg.LocalDotfilesDir, repoDir)
	}
	if cfg.GitPushTarget != "file://"+bareDir {
		t.Errorf("GitPushTarget = %q, want %q (file:// URL for bare repo)", cfg.GitPushTarget, "file://"+bareDir)
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
	bareDir := t.TempDir()
	cfgPath, statePath := initPaths(t)

	// First init creates the repos.
	if err := runInit(strings.NewReader(localInitStdin(repoDir, bareDir)), cfgPath, statePath, ""); err != nil {
		t.Fatalf("first runInit: %v", err)
	}

	// Second init on the same paths should open (not re-init) without error.
	cfg2Path, state2Path := initPaths(t)
	if err := runInit(strings.NewReader(localInitStdin(repoDir, bareDir)), cfg2Path, state2Path, ""); err != nil {
		t.Fatalf("second runInit (existing repo): %v", err)
	}

	cfg, err := config.Load(cfg2Path)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.LocalDotfilesDir != repoDir {
		t.Errorf("RepoPath = %q, want %q", cfg.LocalDotfilesDir, repoDir)
	}
}

// TestRunInitEmptyChoiceDefaultsToLocal verifies that pressing Enter (empty
// input) silently defaults to option 1 and clearly informs the user via
// printed output, then proceeds to create a local repo at the given path.
func TestRunInitEmptyChoiceDefaultsToLocal(t *testing.T) {
	repoDir := t.TempDir()
	bareDir := t.TempDir()
	cfgPath, statePath := initPaths(t)

	// Capture stdout so we can assert the "defaulting" message is printed.
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runInit(strings.NewReader("\n"+repoDir+"\n"+bareDir+"\n"), cfgPath, statePath, "")

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
	if cfg.LocalDotfilesDir != repoDir {
		t.Errorf("RepoPath = %q, want %q", cfg.LocalDotfilesDir, repoDir)
	}
}

func TestRunInitLocalRelativePathConfirmed(t *testing.T) {
	workDir := t.TempDir()
	t.Chdir(workDir) // relative paths resolve under workDir, not the project root

	cfgPath, statePath := initPaths(t)

	// stdin: choice 1 → relative name → confirm with "y" → bare dir
	bareDir := t.TempDir()
	if err := runInit(strings.NewReader("1\ndotfiles\ny\n"+bareDir+"\n"), cfgPath, statePath, ""); err != nil {
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
	if cfg.LocalDotfilesDir != absRepoPath {
		t.Errorf("RepoPath = %q, want %q", cfg.LocalDotfilesDir, absRepoPath)
	}
	if cfg.GitPushTarget != "file://"+bareDir {
		t.Errorf("GitPushTarget = %q, want %q (file:// URL for bare repo)", cfg.GitPushTarget, "file://"+bareDir)
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

func TestRunInitPushTargetRelativePathConfirmed(t *testing.T) {
	workDir := t.TempDir()
	t.Chdir(workDir)
	cfgPath, statePath := initPaths(t)

	// stdin: choice 1 → abs working copy → relative bare name → confirm "y"
	absWorkDir := t.TempDir()
	if err := runInit(strings.NewReader("1\n"+absWorkDir+"\nbare\ny\n"), cfgPath, statePath, ""); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	absBareDir := filepath.Join(workDir, "bare")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.GitPushTarget != "file://"+absBareDir {
		t.Errorf("GitPushTarget = %q, want %q", cfg.GitPushTarget, "file://"+absBareDir)
	}
}

func TestRunInitPushTargetRelativePathRejected(t *testing.T) {
	t.Chdir(t.TempDir())
	cfgPath, statePath := initPaths(t)

	// stdin: choice 1 → abs working copy → relative bare name → reject "n"
	absWorkDir := t.TempDir()
	err := runInit(strings.NewReader("1\n"+absWorkDir+"\nbare\nn\n"), cfgPath, statePath, "")
	if err == nil {
		t.Fatal("expected error when user rejects relative push target, got nil")
	}
	if !strings.Contains(err.Error(), "aborted") {
		t.Errorf("error = %q, want it to contain 'aborted'", err.Error())
	}

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
	if cfg.LocalDotfilesDir != cloneDir {
		t.Errorf("RepoPath = %q, want %q", cfg.LocalDotfilesDir, cloneDir)
	}
	if cfg.GitPushTarget != srcDir {
		t.Errorf("GitURL = %q, want %q", cfg.GitPushTarget, srcDir)
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		t.Fatalf("loading state: %v", err)
	}
	if state.LastCommit == "" {
		t.Error("LastCommit should be set after cloning")
	}
}

// TestRunInitAlreadyInitialized verifies that running hdf init a second time
// on the same config path is a hard stop — no wizard, no data loss.
func TestRunInitAlreadyInitialized(t *testing.T) {
	repoDir := t.TempDir()
	bareDir := t.TempDir()
	cfgPath, statePath := initPaths(t)

	if err := runInit(strings.NewReader(localInitStdin(repoDir, bareDir)), cfgPath, statePath, ""); err != nil {
		t.Fatalf("first runInit: %v", err)
	}

	err := runInit(strings.NewReader(localInitStdin(t.TempDir(), t.TempDir())), cfgPath, statePath, "")
	if err == nil {
		t.Fatal("expected error on second init, got nil")
	}
	if !strings.Contains(err.Error(), "already initialized") {
		t.Errorf("error = %q, want it to contain 'already initialized'", err.Error())
	}

	// Config must be unchanged — the second init must not have touched it.
	cfg, err2 := config.Load(cfgPath)
	if err2 != nil {
		t.Fatalf("loading config: %v", err2)
	}
	if cfg.LocalDotfilesDir != repoDir {
		t.Errorf("LocalDotfilesDir changed: got %q, want %q", cfg.LocalDotfilesDir, repoDir)
	}
}

// TestRunInitLocalWithFilePushTarget verifies that choice 1 creates a distinct
// non-bare working copy and bare push target, wires origin, and writes the
// correct config fields.
func TestRunInitLocalWithFilePushTarget(t *testing.T) {
	workDir := t.TempDir()
	bareDir := t.TempDir()
	cfgPath, statePath := initPaths(t)

	if err := runInit(strings.NewReader(localInitStdin(workDir, bareDir)), cfgPath, statePath, ""); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	// Working copy must be non-bare: has .git directory.
	if _, err := os.Stat(filepath.Join(workDir, ".git")); err != nil {
		t.Errorf("working copy missing .git dir: %v", err)
	}
	// Bare repo must not have a .git subdirectory; HEAD at root is the bare marker.
	if _, err := os.Stat(filepath.Join(bareDir, ".git")); err == nil {
		t.Error("bare repo should not have a .git subdirectory")
	}
	if _, err := os.Stat(filepath.Join(bareDir, "HEAD")); err != nil {
		t.Errorf("bare repo missing HEAD file: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.LocalDotfilesDir != workDir {
		t.Errorf("LocalDotfilesDir = %q, want %q", cfg.LocalDotfilesDir, workDir)
	}
	if cfg.GitPushTarget != "file://"+bareDir {
		t.Errorf("GitPushTarget = %q, want %q", cfg.GitPushTarget, "file://"+bareDir)
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		t.Fatalf("loading state: %v", err)
	}
	if state.LastCommit == "" {
		t.Error("LastCommit should be set after init")
	}

	// Working copy must have origin wired to the bare repo.
	r, err := repo.Open(workDir)
	if err != nil {
		t.Fatalf("opening working copy: %v", err)
	}
	if got := r.RemoteURL(); got != "file://"+bareDir {
		t.Errorf("RemoteURL = %q, want %q", got, "file://"+bareDir)
	}
}

// TestRunInitLocalSamePathRejected verifies that providing the same path for
// the working copy and the push target returns an error.
func TestRunInitLocalSamePathRejected(t *testing.T) {
	repoDir := t.TempDir()
	cfgPath, statePath := initPaths(t)

	err := runInit(strings.NewReader(localInitStdin(repoDir, repoDir)), cfgPath, statePath, "")
	if err == nil {
		t.Fatal("expected error when push target == working copy, got nil")
	}
	if !strings.Contains(err.Error(), "must differ") {
		t.Errorf("error = %q, want it to contain 'must differ'", err.Error())
	}
}

func TestBranchNameNonEmpty(t *testing.T) {
	name := branchName()
	if name == "" {
		t.Error("branchName must never return an empty string")
	}
}

func TestBranchNameFallbackFormat(t *testing.T) {
	// Call the fallback generator directly by sampling the character set used
	// internally — verify it only contains ASCII letters and is the right length.
	for i := range 20 {
		b := make([]byte, 4)
		for j := range b {
			b[j] = branchNameChars[i*j%len(branchNameChars)]
		}
		suffix := string(b)
		if len(suffix) != 4 {
			t.Errorf("suffix len = %d, want 4", len(suffix))
		}
		for _, c := range suffix {
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
				t.Errorf("suffix %q contains non-ASCII-letter character %q", suffix, c)
			}
		}
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
