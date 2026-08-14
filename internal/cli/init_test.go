package cli

import (
	"hdf/config"
	"hdf/repo"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeInitLocalStart_AlreadyInitialized(t *testing.T) {
	cfgPath, _ := initPaths(t)
	if err := os.WriteFile(cfgPath, []byte("branch = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := computeInitLocalStart(cfgPath, t.TempDir(), "")
	if err == nil {
		t.Fatal("expected an error when cfgPath already exists, got nil")
	}
}

func TestComputeInitLocalStart_NewRepoNoPushTarget(t *testing.T) {
	cfgPath, _ := initPaths(t)
	repoDir := filepath.Join(t.TempDir(), "repo")

	info, pending, err := computeInitLocalStart(cfgPath, repoDir, "")
	if err != nil {
		t.Fatalf("computeInitLocalStart: %v", err)
	}
	if info.Collision != nil {
		t.Errorf("Collision = %+v, want nil with no push target", info.Collision)
	}
	if pending.gitURL != "" {
		t.Errorf("gitURL = %q, want empty", pending.gitURL)
	}
	if pending.repoPath != repoDir {
		t.Errorf("repoPath = %q, want %q", pending.repoPath, repoDir)
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err != nil {
		t.Errorf("expected .git dir in repo: %v", err)
	}
}

func TestComputeInitLocalStart_WithLocalPushTarget(t *testing.T) {
	cfgPath, _ := initPaths(t)
	repoDir := filepath.Join(t.TempDir(), "repo")
	bareDir := filepath.Join(t.TempDir(), "bare")

	info, pending, err := computeInitLocalStart(cfgPath, repoDir, bareDir)
	if err != nil {
		t.Fatalf("computeInitLocalStart: %v", err)
	}
	if info.Collision != nil {
		t.Errorf("Collision = %+v, want nil for a freshly created bare repo", info.Collision)
	}
	if pending.gitURL != "file://"+bareDir {
		t.Errorf("gitURL = %q, want %q", pending.gitURL, "file://"+bareDir)
	}
	if _, err := os.Stat(filepath.Join(bareDir, "HEAD")); err != nil {
		t.Errorf("expected bare repo at %s: %v", bareDir, err)
	}
}

func TestComputeInitLocalStart_RejectsSamePushTargetAsRepo(t *testing.T) {
	cfgPath, _ := initPaths(t)
	repoDir := filepath.Join(t.TempDir(), "repo")

	_, _, err := computeInitLocalStart(cfgPath, repoDir, repoDir)
	if err == nil {
		t.Fatal("expected an error when push target equals the repo path, got nil")
	}
}

func TestComputeInitRemoteStart_ClonesAndDetectsCollision(t *testing.T) {
	const shared = "shared-host"
	bareURL := seedBareWithBranch(t, shared)
	t.Setenv("HDF_BRANCH", shared)

	cfgPath, _ := initPaths(t)
	cloneDir := filepath.Join(t.TempDir(), "repo")

	info, pending, err := computeInitRemoteStart(cfgPath, t.TempDir(), bareURL, cloneDir)
	if err != nil {
		t.Fatalf("computeInitRemoteStart: %v", err)
	}
	if info.Collision == nil {
		t.Fatal("Collision = nil, want a collision for the pre-existing shared-host branch")
	}
	if info.Collision.Branch != shared {
		t.Errorf("Collision.Branch = %q, want %q", info.Collision.Branch, shared)
	}
	if pending.branch != shared {
		t.Errorf("pending.branch = %q, want %q", pending.branch, shared)
	}
	if _, err := os.Stat(filepath.Join(cloneDir, ".git")); err != nil {
		t.Errorf("expected cloned .git dir: %v", err)
	}
}

func TestComputeInitRemoteStart_EmptyURLErrors(t *testing.T) {
	cfgPath, _ := initPaths(t)
	_, _, err := computeInitRemoteStart(cfgPath, t.TempDir(), "  ", filepath.Join(t.TempDir(), "repo"))
	if err == nil {
		t.Fatal("expected an error for an empty remote URL, got nil")
	}
}

// TestComputeInitRemoteStart_BlankCloneDirUsesHomeDirDefault verifies that
// a blank cloneDir falls back to homeDir/.local/share/hdf/repo, using the
// homeDir parameter rather than reading os.UserHomeDir() internally — the
// branch defaultRepoPath's prior os.UserHomeDir() call left untested.
func TestComputeInitRemoteStart_BlankCloneDirUsesHomeDirDefault(t *testing.T) {
	const shared = "blank-clone-dir-host"
	bareURL := seedBareWithBranch(t, shared)
	t.Setenv("HDF_BRANCH", shared)

	cfgPath, _ := initPaths(t)
	homeDir := t.TempDir()

	_, pending, err := computeInitRemoteStart(cfgPath, homeDir, bareURL, "")
	if err != nil {
		t.Fatalf("computeInitRemoteStart: %v", err)
	}
	wantRepoPath := filepath.Join(homeDir, ".local", "share", "hdf", "repo")
	if pending.repoPath != wantRepoPath {
		t.Errorf("repoPath = %q, want %q", pending.repoPath, wantRepoPath)
	}
	if _, err := os.Stat(filepath.Join(wantRepoPath, ".git")); err != nil {
		t.Errorf("expected cloned .git dir at the default path: %v", err)
	}
}

func TestComputeResolveBranchCollision_UniqueName(t *testing.T) {
	const shared = "shared-host"
	bareURL := seedBareWithBranch(t, shared)
	t.Setenv("HDF_BRANCH", shared)

	cfgPath, _ := initPaths(t)
	cloneDir := filepath.Join(t.TempDir(), "repo")
	_, pending, err := computeInitRemoteStart(cfgPath, t.TempDir(), bareURL, cloneDir)
	if err != nil {
		t.Fatalf("computeInitRemoteStart: %v", err)
	}

	if err := computeResolveBranchCollision(pending, true); err != nil {
		t.Fatalf("computeResolveBranchCollision: %v", err)
	}
	if pending.branch == shared {
		t.Errorf("branch = %q, collision not avoided", pending.branch)
	}
	if !strings.HasPrefix(pending.branch, shared+"-") {
		t.Errorf("branch = %q, want prefix %q", pending.branch, shared+"-")
	}
	if pending.adopted {
		t.Error("adopted = true, want false for a uniquely suffixed branch")
	}
}

func TestComputeResolveBranchCollision_Reuse(t *testing.T) {
	const shared = "shared-host"
	bareURL := seedBareWithBranch(t, shared)
	t.Setenv("HDF_BRANCH", shared)

	cfgPath, _ := initPaths(t)
	cloneDir := filepath.Join(t.TempDir(), "repo")
	_, pending, err := computeInitRemoteStart(cfgPath, t.TempDir(), bareURL, cloneDir)
	if err != nil {
		t.Fatalf("computeInitRemoteStart: %v", err)
	}

	if err := computeResolveBranchCollision(pending, false); err != nil {
		t.Fatalf("computeResolveBranchCollision: %v", err)
	}
	if pending.branch != shared {
		t.Errorf("branch = %q, want %q (reuse)", pending.branch, shared)
	}
	if !pending.adopted {
		t.Error("adopted = false, want true after reusing the remote branch")
	}

	r, err := repo.Open(cloneDir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.ReadFileFromBranch(shared, ".machinerc")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "existing\n" {
		t.Errorf("reused branch .machinerc = %q, want previous install's content", got)
	}
}

func TestComputeFinishInit_SavesConfigAndState(t *testing.T) {
	cfgPath, statePath := initPaths(t)
	repoDir := filepath.Join(t.TempDir(), "repo")

	_, pending, err := computeInitLocalStart(cfgPath, repoDir, "")
	if err != nil {
		t.Fatalf("computeInitLocalStart: %v", err)
	}

	result, err := computeFinishInit(cfgPath, statePath, pending)
	if err != nil {
		t.Fatalf("computeFinishInit: %v", err)
	}
	if !strings.Contains(result.Message, pending.branch) {
		t.Errorf("Message = %q, want it to mention branch %q", result.Message, pending.branch)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.LocalDotfilesDir != repoDir {
		t.Errorf("LocalDotfilesDir = %q, want %q", cfg.LocalDotfilesDir, repoDir)
	}
	if cfg.Branch != pending.branch {
		t.Errorf("Branch = %q, want %q", cfg.Branch, pending.branch)
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		t.Fatalf("config.LoadState: %v", err)
	}
	if state.LastCommit == "" {
		t.Error("LastCommit is empty after computeFinishInit")
	}

	r, err := repo.Open(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	cur, err := r.CurrentBranch()
	if err != nil {
		t.Fatal(err)
	}
	if cur != pending.branch {
		t.Errorf("checked-out branch = %q, want %q", cur, pending.branch)
	}
}

func TestComputeFinishInit_ReuseSkipsBranchCreation(t *testing.T) {
	const shared = "shared-host"
	bareURL := seedBareWithBranch(t, shared)
	t.Setenv("HDF_BRANCH", shared)

	cfgPath, statePath := initPaths(t)
	cloneDir := filepath.Join(t.TempDir(), "repo")
	_, pending, err := computeInitRemoteStart(cfgPath, t.TempDir(), bareURL, cloneDir)
	if err != nil {
		t.Fatalf("computeInitRemoteStart: %v", err)
	}
	if err := computeResolveBranchCollision(pending, false); err != nil {
		t.Fatalf("computeResolveBranchCollision: %v", err)
	}

	if _, err := computeFinishInit(cfgPath, statePath, pending); err != nil {
		t.Fatalf("computeFinishInit: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Branch != shared {
		t.Errorf("Branch = %q, want %q", cfg.Branch, shared)
	}
}
