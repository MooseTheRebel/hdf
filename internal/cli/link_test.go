package cli

import (
	"encoding/json"
	"hdf/config"
	"hdf/link"
	"hdf/repo"
	"os"
	"path/filepath"
	"testing"
)

// assertJSONFieldNotNull marshals v to JSON and fails the test if field is
// present but set to JSON null. A Go nil slice/map marshals to null, and
// the frontend's Wails bindings hand that null straight to JS with no
// wrapping — an unconditional `.length`/`.map()` call on it throws. Struct
// fields meant to represent "empty list" must marshal to `[]`, not `null`.
func assertJSONFieldNotNull(t *testing.T, v any, field string) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	raw, ok := m[field]
	if !ok {
		t.Fatalf("field %q not present in JSON: %s", field, b)
	}
	if string(raw) == "null" {
		t.Errorf("field %q is JSON null, want a non-null value (e.g. []): %s", field, b)
	}
}

// setupLinkRemote creates a bare "origin", a seed repo that pushes the
// registry to main, and a clone checked out on testBranch — the shared
// fixture for computeLinkStart tests that need a real remote to fetch
// from. Returns the bare repo's file:// URL and the machine-branch repo.
func setupLinkRemote(t *testing.T, reg *config.Registry) (bareURL, workDir string) {
	t.Helper()

	bareDir := t.TempDir()
	if _, _, err := repo.InitOrOpenBare(bareDir); err != nil {
		t.Fatalf("InitOrOpenBare: %v", err)
	}
	bareURL = "file://" + bareDir

	seedDir := t.TempDir()
	seed, err := repo.Init(seedDir)
	if err != nil {
		t.Fatalf("seed Init: %v", err)
	}
	hdfDir := filepath.Join(seedDir, ".hdf")
	if err := os.MkdirAll(hdfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hdfDir, ".gitkeep"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.CommitFile(".hdf/.gitkeep", "hdf: initial"); err != nil {
		t.Fatalf("seed CommitFile: %v", err)
	}
	if err := seed.AddRemote("origin", bareURL); err != nil {
		t.Fatalf("seed AddRemote: %v", err)
	}
	if err := seed.Push("main"); err != nil {
		t.Fatalf("seed Push main: %v", err)
	}

	regBytes, err := config.RegistryToBytes(reg)
	if err != nil {
		t.Fatalf("RegistryToBytes: %v", err)
	}
	if _, err := seed.CommitFilesToBranch("main", []repo.BranchFile{
		{RepoRelPath: managedTOMLPath, Content: regBytes},
	}, "hdf: write registry"); err != nil {
		t.Fatalf("CommitFilesToBranch registry: %v", err)
	}
	if err := seed.Push("main"); err != nil {
		t.Fatalf("seed Push main (registry): %v", err)
	}

	workDir = t.TempDir()
	r, err := repo.Clone(bareURL, workDir)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if err := r.CreateAndCheckoutBranch(testBranch); err != nil {
		t.Fatalf("CreateAndCheckoutBranch: %v", err)
	}
	return bareURL, workDir
}

func TestComputeLinkStart_NoRemoteConfigured(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()

	r, err := repo.Init(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("seed.txt", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateAndCheckoutBranch(testBranch); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveRegistry(workDir, &config.Registry{}); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := &config.Config{Branch: testBranch, LocalDotfilesDir: workDir}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}

	info, pending, err := computeLinkStart(cfgPath, homeDir, false)
	if err != nil {
		t.Fatalf("computeLinkStart: %v", err)
	}
	if info.Message != "No remote configured; skipping fetch." {
		t.Errorf("Message = %q, want the no-remote message", info.Message)
	}
	if len(info.IncomingFiles) != 0 {
		t.Errorf("IncomingFiles = %v, want empty", info.IncomingFiles)
	}
	if len(pending) != 0 {
		t.Errorf("pending = %v, want empty", pending)
	}
}

func TestComputeLinkStart_NoFetchSkipsRemoteEntirely(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()

	r, err := repo.Init(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("seed.txt", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateAndCheckoutBranch(testBranch); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveRegistry(workDir, &config.Registry{}); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := &config.Config{Branch: testBranch, LocalDotfilesDir: workDir, GitPushTarget: "file:///nonexistent-should-not-be-touched"}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}

	info, pending, err := computeLinkStart(cfgPath, homeDir, true)
	if err != nil {
		t.Fatalf("computeLinkStart: %v", err)
	}
	if info.Message != "" {
		t.Errorf("Message = %q, want empty for noFetch", info.Message)
	}
	if len(info.IncomingFiles) != 0 || len(pending) != 0 {
		t.Errorf("expected no incoming files for noFetch, got %v / %v", info.IncomingFiles, pending)
	}
}

func TestComputeLinkStart_AlreadyUpToDate(t *testing.T) {
	homeDir := t.TempDir()
	bareURL, workDir := setupLinkRemote(t, &config.Registry{})

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := &config.Config{Branch: testBranch, LocalDotfilesDir: workDir, GitPushTarget: bareURL}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}

	info, pending, err := computeLinkStart(cfgPath, homeDir, false)
	if err != nil {
		t.Fatalf("computeLinkStart: %v", err)
	}
	if info.Message != "Already up to date." {
		t.Errorf("Message = %q, want %q", info.Message, "Already up to date.")
	}
	if len(info.IncomingFiles) != 0 || len(pending) != 0 {
		t.Errorf("expected no incoming files, got %v / %v", info.IncomingFiles, pending)
	}
}

func TestComputeLinkStart_IncomingFileDiff(t *testing.T) {
	homeDir := t.TempDir()
	homePath := filepath.Join(homeDir, ".testrc")
	reg := &config.Registry{Files: []config.ManagedFile{{Path: tildeTestRC}}}
	bareURL, workDir := setupLinkRemote(t, reg)

	// Advance main with a file the machine branch doesn't have yet, via a
	// fresh clone (setupLinkRemote's seed repo isn't exposed to callers).
	updater, err := repo.Clone(bareURL, t.TempDir())
	if err != nil {
		t.Fatalf("Clone updater: %v", err)
	}
	if _, err := updater.CommitFilesToBranch("main", []repo.BranchFile{
		{RepoRelPath: filepath.Base(homePath), Content: []byte(updatedByMain)},
	}, "hdf: update file on main"); err != nil {
		t.Fatalf("CommitFilesToBranch: %v", err)
	}
	if err := updater.Push("main"); err != nil {
		t.Fatalf("Push main: %v", err)
	}

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := &config.Config{Branch: testBranch, LocalDotfilesDir: workDir, GitPushTarget: bareURL}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}

	info, pending, err := computeLinkStart(cfgPath, homeDir, false)
	if err != nil {
		t.Fatalf("computeLinkStart: %v", err)
	}
	if len(info.IncomingFiles) != 1 {
		t.Fatalf("IncomingFiles = %v, want 1 entry", info.IncomingFiles)
	}
	if info.IncomingFiles[0].Path != tildeTestRC {
		t.Errorf("Path = %q, want %q", info.IncomingFiles[0].Path, tildeTestRC)
	}
	if info.IncomingFiles[0].Diff == "" {
		t.Errorf("Diff is empty, want a non-empty unified diff")
	}
	if len(pending) != 1 {
		t.Fatalf("pending = %v, want 1 entry", pending)
	}

	// acceptIncomingFile applies main's version and commits it locally.
	if err := acceptIncomingFile(cfgPath, pending[0]); err != nil {
		t.Fatalf("acceptIncomingFile: %v", err)
	}
	freshR, err := repo.Open(workDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	content, err := freshR.ReadFileFromBranch(testBranch, filepath.Base(homePath))
	if err != nil {
		t.Fatalf("ReadFileFromBranch: %v", err)
	}
	if string(content) != updatedByMain {
		t.Errorf("branch file = %q, want %q", string(content), updatedByMain)
	}
}

func TestComputeRelink_LinksManagedFiles(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()

	r, err := repo.Init(workDir)
	if err != nil {
		t.Fatal(err)
	}
	homeDotfile := filepath.Join(homeDir, ".testrc")
	if err := os.WriteFile(homeDotfile, []byte("export PS1='$ '\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	relPath := filepath.Base(homeDotfile)
	if err := os.WriteFile(filepath.Join(workDir, relPath), []byte("export PS1='$ '\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile(relPath, "add .testrc"); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateAndCheckoutBranch(testBranch); err != nil {
		t.Fatal(err)
	}
	reg := &config.Registry{Files: []config.ManagedFile{{Path: tildeTestRC}}}
	if err := config.SaveRegistry(workDir, reg); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := &config.Config{Branch: testBranch, LocalDotfilesDir: workDir}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}

	results, err := computeRelink(cfgPath, homeDir)
	if err != nil {
		t.Fatalf("computeRelink: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %v, want 1 entry", results)
	}
	if results[0].Path != tildeTestRC {
		t.Errorf("Path = %q, want %q", results[0].Path, tildeTestRC)
	}
	if results[0].Error != "" {
		t.Errorf("Error = %q, want empty", results[0].Error)
	}
	info, err := os.Lstat(homeDotfile)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf(".testrc is not a symlink after computeRelink")
	}
}

func TestComputeRelink_ReportsPerFileError(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()

	r, err := repo.Init(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("seed.txt", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateAndCheckoutBranch(testBranch); err != nil {
		t.Fatal(err)
	}
	// Registry entry has variants but none matching this machine's branch —
	// resolveRepoPath returns ("", nil), and computeRelink should report
	// that per-file rather than aborting.
	reg := &config.Registry{Files: []config.ManagedFile{{
		Path:     tildeMissingRC,
		Variants: []config.Variant{{Branch: "some-other-branch", RepoPath: "missingrc", Hash: link.HashBytes([]byte("x"))}},
	}}}
	if err := config.SaveRegistry(workDir, reg); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := &config.Config{Branch: testBranch, LocalDotfilesDir: workDir}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}

	results, err := computeRelink(cfgPath, homeDir)
	if err != nil {
		t.Fatalf("computeRelink: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %v, want 1 entry", results)
	}
	if results[0].Error == "" {
		t.Errorf("Error is empty, want a link failure reported")
	}
}

// TestComputeLinkStart_IncomingFilesNeverNilForJSON verifies that
// LinkStartInfo.IncomingFiles is always a non-nil (possibly empty) slice,
// never a bare nil, across every path that reports "nothing to review":
// noFetch, no remote configured, and already up to date. A nil slice
// marshals to JSON `null`, and the frontend calls
// `info.incomingFiles.length` unconditionally — `null.length` throws,
// crashing the Link flow on every one of these common, non-error paths.
func TestComputeLinkStart_IncomingFilesNeverNilForJSON(t *testing.T) {
	seedLocalRepo := func(t *testing.T) (cfgPath, homeDir string) {
		t.Helper()
		workDir := t.TempDir()
		homeDir = t.TempDir()
		r, err := repo.Init(workDir)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := r.CommitFile("seed.txt", "seed"); err != nil {
			t.Fatal(err)
		}
		if err := r.CreateAndCheckoutBranch(testBranch); err != nil {
			t.Fatal(err)
		}
		if err := config.SaveRegistry(workDir, &config.Registry{}); err != nil {
			t.Fatal(err)
		}
		cfgPath = filepath.Join(t.TempDir(), "config.toml")
		cfg := &config.Config{Branch: testBranch, LocalDotfilesDir: workDir}
		if err := config.Save(cfgPath, cfg); err != nil {
			t.Fatal(err)
		}
		return cfgPath, homeDir
	}

	t.Run("noFetch", func(t *testing.T) {
		cfgPath, homeDir := seedLocalRepo(t)
		info, _, err := computeLinkStart(cfgPath, homeDir, true)
		if err != nil {
			t.Fatalf("computeLinkStart: %v", err)
		}
		assertJSONFieldNotNull(t, info, "incomingFiles")
	})

	t.Run("noRemoteConfigured", func(t *testing.T) {
		cfgPath, homeDir := seedLocalRepo(t)
		info, _, err := computeLinkStart(cfgPath, homeDir, false)
		if err != nil {
			t.Fatalf("computeLinkStart: %v", err)
		}
		assertJSONFieldNotNull(t, info, "incomingFiles")
	})

	t.Run("alreadyUpToDate", func(t *testing.T) {
		homeDir := t.TempDir()
		bareURL, workDir := setupLinkRemote(t, &config.Registry{})
		cfgPath := filepath.Join(t.TempDir(), "config.toml")
		cfg := &config.Config{Branch: testBranch, LocalDotfilesDir: workDir, GitPushTarget: bareURL}
		if err := config.Save(cfgPath, cfg); err != nil {
			t.Fatal(err)
		}
		info, _, err := computeLinkStart(cfgPath, homeDir, false)
		if err != nil {
			t.Fatalf("computeLinkStart: %v", err)
		}
		assertJSONFieldNotNull(t, info, "incomingFiles")
	})
}

// TestComputeRelink_ResultsNeverNilForJSON verifies that computeRelink
// returns a non-nil (possibly empty) slice when there are no managed
// files, rather than a nil slice that marshals to JSON `null` and crashes
// the frontend's `results.map(...)` call.
func TestComputeRelink_ResultsNeverNilForJSON(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()

	r, err := repo.Init(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("seed.txt", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateAndCheckoutBranch(testBranch); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveRegistry(workDir, &config.Registry{}); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := &config.Config{Branch: testBranch, LocalDotfilesDir: workDir}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}

	results, err := computeRelink(cfgPath, homeDir)
	if err != nil {
		t.Fatalf("computeRelink: %v", err)
	}
	if results == nil {
		t.Error("results is nil, want a non-nil empty slice (marshals to JSON [] not null)")
	}
}
