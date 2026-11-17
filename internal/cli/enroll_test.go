package cli

import (
	"hdf/config"
	"hdf/repo"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupEnrollTestConfig runs a local `hdf init` (repo + bare push target)
// into a fresh cfgPath/statePath pair and returns them alongside the
// isolated home directory tests should pass to computeEnrollStart /
// computeApplyEnroll.
func setupEnrollTestConfig(t *testing.T) (cfgPath, statePath, homeDir string) {
	t.Helper()
	workDir := t.TempDir()
	bareDir := t.TempDir()

	cfgPath, statePath = initPaths(t)

	if err := runInit(strings.NewReader(localInitStdin(workDir, bareDir)), cfgPath, statePath, ""); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	homeDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	return cfgPath, statePath, homeDir
}

func TestComputeEnrollStart_NewFile(t *testing.T) {
	cfgPath, _, homeDir := setupEnrollTestConfig(t)

	dotfile := filepath.Join(homeDir, ".testrc")
	if err := os.WriteFile(dotfile, []byte("export PS1='$ '\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	info, pending, err := computeEnrollStart(cfgPath, homeDir, dotfile)
	if err != nil {
		t.Fatalf("computeEnrollStart: %v", err)
	}

	if info.Path != tildeTestRC {
		t.Errorf("Path = %q, want %q", info.Path, tildeTestRC)
	}

	if !info.IsNewFile {
		t.Error("IsNewFile = false, want true for a never-enrolled file")
	}

	if info.Diff != "" {
		t.Errorf("Diff = %q, want empty for a new file", info.Diff)
	}

	if pending == nil {
		t.Fatal("pending = nil, want a populated pendingEnroll")
	}
}

func TestComputeEnrollStart_ModifiedFile(t *testing.T) {
	cfgPath, statePath, homeDir := setupEnrollTestConfig(t)

	dotfile := filepath.Join(homeDir, ".testrc")
	if err := os.WriteFile(dotfile, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runEnroll(tildeTestRC, homeDir, mustLoadConfig(t, cfgPath), statePath, strings.NewReader(""), true); err != nil {
		t.Fatalf("runEnroll (seed): %v", err)
	}

	if err := os.WriteFile(dotfile, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	info, _, err := computeEnrollStart(cfgPath, homeDir, dotfile)
	if err != nil {
		t.Fatalf("computeEnrollStart: %v", err)
	}

	if info.IsNewFile {
		t.Error("IsNewFile = true, want false for an already-enrolled file")
	}

	if info.Diff == "" {
		t.Error("Diff is empty, want a non-empty unified diff for modified content")
	}
}

func TestComputeEnrollStart_RejectsDirectory(t *testing.T) {
	cfgPath, _, homeDir := setupEnrollTestConfig(t)

	dirPath := filepath.Join(homeDir, "adir")
	if err := os.Mkdir(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := computeEnrollStart(cfgPath, homeDir, dirPath)
	if err == nil {
		t.Fatal("expected an error for a directory path, got nil")
	}
}

func TestComputeApplyEnroll_CommitsAndPushes(t *testing.T) {
	cfgPath, statePath, homeDir := setupEnrollTestConfig(t)

	dotfile := filepath.Join(homeDir, ".testrc")
	if err := os.WriteFile(dotfile, []byte("export PS1='$ '\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, pending, err := computeEnrollStart(cfgPath, homeDir, dotfile)
	if err != nil {
		t.Fatalf("computeEnrollStart: %v", err)
	}

	result, err := computeApplyEnroll(cfgPath, homeDir, statePath, *pending)
	if err != nil {
		t.Fatalf("computeApplyEnroll: %v", err)
	}

	if !strings.Contains(result.Message, "Enrolled") {
		t.Errorf("Message = %q, want it to mention Enrolled", result.Message)
	}

	info, err := os.Lstat(dotfile)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		t.Error(".testrc is not a symlink after computeApplyEnroll")
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if state.LastCommit == "" {
		t.Error("state.LastCommit is empty after computeApplyEnroll")
	}
}

func TestComputeApplyEnroll_AlreadyManagedAndUnchanged(t *testing.T) {
	cfgPath, statePath, homeDir := setupEnrollTestConfig(t)

	dotfile := filepath.Join(homeDir, ".testrc")
	if err := os.WriteFile(dotfile, []byte("export PS1='$ '\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runEnroll(tildeTestRC, homeDir, mustLoadConfig(t, cfgPath), statePath, strings.NewReader(""), true); err != nil {
		t.Fatalf("runEnroll (seed): %v", err)
	}

	_, pending, err := computeEnrollStart(cfgPath, homeDir, dotfile)
	if err != nil {
		t.Fatalf("computeEnrollStart: %v", err)
	}

	result, err := computeApplyEnroll(cfgPath, homeDir, statePath, *pending)
	if err != nil {
		t.Fatalf("computeApplyEnroll: %v", err)
	}

	if !strings.Contains(result.Message, "already managed and unchanged") {
		t.Errorf("Message = %q, want it to mention already managed and unchanged", result.Message)
	}
}

func TestComputeApplyEnroll_RetryPushesUnpushedMain(t *testing.T) {
	cfgPath, statePath, homeDir := enrollPushingOnlyBranch(t)

	_, pending, err := computeEnrollStart(cfgPath, homeDir, filepath.Join(homeDir, ".testrc"))
	if err != nil {
		t.Fatalf("computeEnrollStart: %v", err)
	}

	if _, err := computeApplyEnroll(cfgPath, homeDir, statePath, *pending); err != nil {
		t.Fatalf("computeApplyEnroll: %v", err)
	}

	assertMainPushed(t, mustLoadConfig(t, cfgPath))
}

func TestComputeApplyEnroll_RetryRecordsLastCommit(t *testing.T) {
	cfgPath, statePath, homeDir := enrollWithFailedPush(t)

	_, pending, err := computeEnrollStart(cfgPath, homeDir, filepath.Join(homeDir, ".testrc"))
	if err != nil {
		t.Fatalf("computeEnrollStart: %v", err)
	}

	if _, err := computeApplyEnroll(cfgPath, homeDir, statePath, *pending); err != nil {
		t.Fatalf("computeApplyEnroll: %v", err)
	}

	assertLastCommitIsBranchHead(t, mustLoadConfig(t, cfgPath), statePath)
}

// enrollPushingOnlyBranch enrolls ~/.testrc and pushes only the machine
// branch, leaving main's registry commit local — what a main push that fails
// after the machine-branch push leaves behind.
func enrollPushingOnlyBranch(t *testing.T) (cfgPath, statePath, homeDir string) {
	t.Helper()

	cfgPath, statePath, homeDir = setupEnrollTestConfig(t)
	cfg := mustLoadConfig(t, cfgPath)

	if err := os.WriteFile(filepath.Join(homeDir, ".testrc"), []byte("export PS1='$ '\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	offline := *cfg
	offline.GitPushTarget = ""

	if err := runEnroll(tildeTestRC, homeDir, &offline, statePath, strings.NewReader(""), true); err != nil {
		t.Fatalf("runEnroll (offline): %v", err)
	}

	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		t.Fatal(err)
	}

	if err := r.Push(cfg.Branch); err != nil {
		t.Fatalf("pushing machine branch: %v", err)
	}

	return cfgPath, statePath, homeDir
}

// enrollWithFailedPush enrolls ~/.testrc while the push target is
// unreachable, so the commit is made but never pushed, then makes the push
// target reachable again.
func enrollWithFailedPush(t *testing.T) (cfgPath, statePath, homeDir string) {
	t.Helper()

	cfgPath, statePath, homeDir = setupEnrollTestConfig(t)
	cfg := mustLoadConfig(t, cfgPath)

	if err := os.WriteFile(filepath.Join(homeDir, ".testrc"), []byte("export PS1='$ '\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bare := strings.TrimPrefix(cfg.GitPushTarget, "file://")
	if err := os.Rename(bare, bare+".offline"); err != nil {
		t.Fatal(err)
	}

	if err := runEnroll(tildeTestRC, homeDir, cfg, statePath, strings.NewReader(""), true); err == nil {
		t.Fatal("runEnroll with unreachable push target succeeded")
	}

	if err := os.Rename(bare+".offline", bare); err != nil {
		t.Fatal(err)
	}

	return cfgPath, statePath, homeDir
}

// assertLastCommitIsBranchHead fails unless state.LastCommit is the machine
// branch's head.
func assertLastCommitIsBranchHead(t *testing.T, cfg *config.Config, statePath string) {
	t.Helper()

	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		t.Fatal(err)
	}

	want, err := r.BranchSHA(cfg.Branch)
	if err != nil {
		t.Fatal(err)
	}

	s, err := config.LoadState(statePath)
	if err != nil {
		t.Fatal(err)
	}

	if s.LastCommit != want {
		t.Errorf("LastCommit = %q, want machine branch head %q", s.LastCommit, want)
	}
}

// assertMainPushed fails unless the push target's main matches local main.
func assertMainPushed(t *testing.T, cfg *config.Config) {
	t.Helper()

	local, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		t.Fatal(err)
	}

	want, err := local.BranchSHA("main")
	if err != nil {
		t.Fatal(err)
	}

	bare, err := repo.Open(strings.TrimPrefix(cfg.GitPushTarget, "file://"))
	if err != nil {
		t.Fatal(err)
	}

	if got, _ := bare.BranchSHA("main"); got != want {
		t.Errorf("remote main = %q, want local main %q", got, want)
	}
}

func mustLoadConfig(t *testing.T, cfgPath string) *config.Config {
	t.Helper()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	return cfg
}
