package cli

import (
	"hdf/config"
	"hdf/repo"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputePromoteStart_NoRemote(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := &config.Config{GitPushTarget: "", LocalDotfilesDir: t.TempDir(), Branch: "test-machine"}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "state.toml")

	_, _, err := computePromoteStart(cfgPath, statePath, t.TempDir())
	if err == nil {
		t.Fatal("expected error for no remote configured, got nil")
	}
	if !strings.Contains(err.Error(), "no remote configured") {
		t.Errorf("error = %q, want mention of 'no remote configured'", err.Error())
	}
}

func TestComputePromoteStart_DirtyWorktree(t *testing.T) {
	bareDir := t.TempDir()
	workDir := t.TempDir()
	cfgPath, statePath := initPaths(t)
	if err := runInit(strings.NewReader(localInitStdin(workDir, bareDir)), cfgPath, statePath, ""); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	dirty := filepath.Join(workDir, "dirty.txt")
	if err := os.WriteFile(dirty, []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := computePromoteStart(cfgPath, statePath, t.TempDir())
	if err == nil {
		t.Fatal("expected error for dirty worktree, got nil")
	}
	if !strings.Contains(err.Error(), "uncommitted") {
		t.Errorf("error = %q, want mention of 'uncommitted'", err.Error())
	}
}

func TestComputePromoteStart_DivergedFileNeedsReview(t *testing.T) {
	cfg, homeDir, _, _ := setupDivergedForPromote(t)
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "state.toml")

	info, pending, err := computePromoteStart(cfgPath, statePath, homeDir)
	if err != nil {
		t.Fatalf("computePromoteStart: %v", err)
	}
	if len(info.Preserved) != 0 {
		t.Errorf("Preserved = %v, want empty (this machine cloned at v1)", info.Preserved)
	}
	if len(info.Diverged) != 1 {
		t.Fatalf("Diverged = %v, want 1 entry", info.Diverged)
	}
	wantPath := filepath.Join(homeDir, testRCRelPath)
	if info.Diverged[0].Path != wantPath {
		t.Errorf("Diverged[0].Path = %q, want %q", info.Diverged[0].Path, wantPath)
	}
	if info.Diverged[0].Diff == "" {
		t.Error("Diverged[0].Diff is empty, want a non-empty unified diff")
	}
	if len(pending.diverged) != 1 {
		t.Errorf("pending.diverged = %v, want 1 entry", pending.diverged)
	}
}

func TestComputePromoteStart_PreviouslyDeclinedFoldedInSilently(t *testing.T) {
	cfg, homeDir, _, _ := setupDivergedForPromote(t)
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "state.toml")

	_, pending, err := computePromoteStart(cfgPath, statePath, homeDir)
	if err != nil {
		t.Fatalf("computePromoteStart: %v", err)
	}
	if err := computeResolveDivergedFile(pending, 0, false); err != nil {
		t.Fatalf("computeResolveDivergedFile: %v", err)
	}
	if _, err := computeFinishPromote(pending); err != nil {
		t.Fatalf("computeFinishPromote: %v", err)
	}

	// New local work, re-check: the decline should be remembered and folded
	// in without requiring review again.
	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.LocalDotfilesDir, ".other2"), []byte("more\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile(".other2", "machine: add .other2"); err != nil {
		t.Fatal(err)
	}

	info2, pending2, err := computePromoteStart(cfgPath, statePath, homeDir)
	if err != nil {
		t.Fatalf("second computePromoteStart: %v", err)
	}
	if len(info2.Diverged) != 0 {
		t.Errorf("Diverged = %v, want empty (decline should be remembered)", info2.Diverged)
	}
	if !pending2.preferTheirs[testRCRelPath] {
		t.Error("preferTheirs should be pre-seeded true for the remembered decline")
	}
}

func TestComputeResolveDivergedFile_KeepMineClearsDecline(t *testing.T) {
	cfg, homeDir, bare, _ := setupDivergedForPromote(t)
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "state.toml")

	_, pending, err := computePromoteStart(cfgPath, statePath, homeDir)
	if err != nil {
		t.Fatalf("computePromoteStart: %v", err)
	}
	if err := computeResolveDivergedFile(pending, 0, true); err != nil {
		t.Fatalf("computeResolveDivergedFile: %v", err)
	}
	if _, err := computeFinishPromote(pending); err != nil {
		t.Fatalf("computeFinishPromote: %v", err)
	}

	got, err := bare.ReadFileFromBranch("main", testRCRelPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v1\n" {
		t.Errorf("main %s = %q, want machine's v1 after keep-mine overwrite", testRCRelPath, got)
	}
}

func TestComputeFinishPromote_PushesBranchAndMain(t *testing.T) {
	bareDir := t.TempDir()
	workDir := t.TempDir()
	cfgPath, statePath := initPaths(t)
	if err := runInit(strings.NewReader(localInitStdin(workDir, bareDir)), cfgPath, statePath, ""); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "dot.txt"), []byte("content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("dot.txt", "add dot.txt"); err != nil {
		t.Fatal(err)
	}

	_, pending, err := computePromoteStart(cfgPath, statePath, t.TempDir())
	if err != nil {
		t.Fatalf("computePromoteStart: %v", err)
	}
	result, err := computeFinishPromote(pending)
	if err != nil {
		t.Fatalf("computeFinishPromote: %v", err)
	}
	if !strings.Contains(result.Message, "Promoted") {
		t.Errorf("Message = %q, want it to mention Promoted", result.Message)
	}

	bare, err := repo.Open(bareDir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := bare.ReadFileFromBranch("main", "dot.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "content\n" {
		t.Errorf("main dot.txt = %q, want %q", got, "content\n")
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if state.LastMainCommit == "" {
		t.Error("LastMainCommit is empty after computeFinishPromote")
	}
}
