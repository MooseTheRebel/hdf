package daemon

import (
	"hdf/config"
	"hdf/link"
	"hdf/repo"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const testHostBranch = "test-hostabc123"

// fakeNotifier captures all notification messages for test assertions.
type fakeNotifier struct {
	mu   sync.Mutex
	msgs []string
}

func (f *fakeNotifier) Send(title, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, message)
	return nil
}

// driftCount returns the number of drift-threshold notifications sent.
func (f *fakeNotifier) driftCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, m := range f.msgs {
		if strings.Contains(m, "hunk") {
			count++
		}
	}
	return count
}

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

	err := Sync(cfgPath, statePath, nil)
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

	err := Sync(cfgPath, statePath, nil)
	if err == nil {
		t.Fatal("expected error when fetch fails, got nil")
	}
	if !strings.Contains(err.Error(), "fetching from remote") {
		t.Errorf("error = %q, want it to contain 'fetching from remote'", err.Error())
	}
}

// TestCountHunks verifies that countHunks correctly counts contiguous changed
// regions for the fixture content used by TestMultiHostIntegration.
func TestCountHunks(t *testing.T) {
	// 9-line file with alternating mutable/stable lines.
	initial := "MUTABLE-1\nstable-2\nMUTABLE-3\nstable-4\nMUTABLE-5\nstable-6\nMUTABLE-7\nstable-8\nMUTABLE-9\n"

	cases := []struct {
		name    string
		drifted string
		want    int
	}{
		{
			"1 hunk — change only line 1",
			"CHANGED-1\nstable-2\nMUTABLE-3\nstable-4\nMUTABLE-5\nstable-6\nMUTABLE-7\nstable-8\nMUTABLE-9\n",
			1,
		},
		{
			"3 hunks — change lines 1, 3, 5 (each separated by an unchanged line)",
			"CHANGED-1\nstable-2\nCHANGED-3\nstable-4\nCHANGED-5\nstable-6\nMUTABLE-7\nstable-8\nMUTABLE-9\n",
			3,
		},
		{
			"5 hunks — change lines 1, 3, 5, 7, 9",
			"CHANGED-1\nstable-2\nCHANGED-3\nstable-4\nCHANGED-5\nstable-6\nCHANGED-7\nstable-8\nCHANGED-9\n",
			5,
		},
		{
			"0 hunks — no changes",
			initial,
			0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := countHunks(initial, tc.drifted)
			if got != tc.want {
				t.Errorf("countHunks: got %d, want %d", got, tc.want)
			}
		})
	}
}

// TestMultiHostIntegration verifies the hunk-based notification threshold with
// three hosts sharing a single bare remote. The shared NotifyThreshold is 3.
//
//   - Host A drifts by 1 hunk  → below threshold → no drift notification
//   - Host B drifts by 3 hunks → at threshold    → 1 drift notification
//   - Host C drifts by 5 hunks → above threshold → 1 drift notification
func TestMultiHostIntegration(t *testing.T) {
	// --- fixture content ---
	// 9-line alternating file; each "MUTABLE-N" line is a candidate for change.
	// Changing every other mutable line creates separate (non-adjacent) hunks.
	initialContent := "MUTABLE-1\nstable-2\nMUTABLE-3\nstable-4\nMUTABLE-5\nstable-6\nMUTABLE-7\nstable-8\nMUTABLE-9\n"

	type hostSpec struct {
		branch         string
		driftedContent string // written to working tree to create drift
	}
	hostSpecs := []hostSpec{
		{
			branch:         "host-a",
			driftedContent: "CHANGED-1\nstable-2\nMUTABLE-3\nstable-4\nMUTABLE-5\nstable-6\nMUTABLE-7\nstable-8\nMUTABLE-9\n",
		},
		{
			branch:         "host-b",
			driftedContent: "CHANGED-1\nstable-2\nCHANGED-3\nstable-4\nCHANGED-5\nstable-6\nMUTABLE-7\nstable-8\nMUTABLE-9\n",
		},
		{
			branch:         "host-c",
			driftedContent: "CHANGED-1\nstable-2\nCHANGED-3\nstable-4\nCHANGED-5\nstable-6\nCHANGED-7\nstable-8\nCHANGED-9\n",
		},
	}

	// --- shared bare remote ---
	bareDir := t.TempDir()
	if _, _, err := repo.InitOrOpenBare(bareDir); err != nil {
		t.Fatalf("InitOrOpenBare: %v", err)
	}
	bareURL := "file://" + bareDir

	// Seed: push an initial commit to main so hosts can clone.
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

	// --- per-host state ---
	type hostEnv struct {
		workDir   string
		homeDir   string
		cfgPath   string
		statePath string
		r         *repo.Repo
		notifier  *fakeNotifier
	}
	envs := make([]hostEnv, len(hostSpecs))

	// --- set up each host: clone → branch → enroll → commit → push ---
	for i, spec := range hostSpecs {
		env := &envs[i]
		env.workDir = t.TempDir()
		env.homeDir = t.TempDir()
		env.cfgPath = filepath.Join(t.TempDir(), "config.toml")
		env.statePath = filepath.Join(t.TempDir(), "state.toml")
		env.notifier = &fakeNotifier{}

		r, err := repo.Clone(bareURL, env.workDir)
		if err != nil {
			t.Fatalf("host %s Clone: %v", spec.branch, err)
		}
		env.r = r

		if err := r.CreateAndCheckoutBranch(spec.branch); err != nil {
			t.Fatalf("host %s CreateAndCheckoutBranch: %v", spec.branch, err)
		}

		// Write initial file to homeDir and enroll it.
		homePath := filepath.Join(env.homeDir, "dotfile.txt")
		if err := os.WriteFile(homePath, []byte(initialContent), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := link.EnrollInHome(homePath, env.workDir, env.homeDir); err != nil {
			t.Fatalf("host %s EnrollInHome: %v", spec.branch, err)
		}

		// Compute hash of the enrolled (committed-to-be) file.
		enrolledHash, err := link.HashFile(filepath.Join(env.workDir, "dotfile.txt"))
		if err != nil {
			t.Fatalf("host %s HashFile: %v", spec.branch, err)
		}

		// Store absolute path in registry so syncWithHome can resolve it
		// using the test homeDir without touching os.UserHomeDir().
		reg := &config.Registry{
			Files: []config.ManagedFile{
				{Path: homePath, Hash: enrolledHash},
			},
		}
		if err := config.SaveRegistry(env.workDir, reg); err != nil {
			t.Fatalf("host %s SaveRegistry: %v", spec.branch, err)
		}

		if err := r.StageFile("dotfile.txt"); err != nil {
			t.Fatalf("host %s StageFile dotfile: %v", spec.branch, err)
		}
		if err := r.StageFile(".hdf/managed.toml"); err != nil {
			t.Fatalf("host %s StageFile managed.toml: %v", spec.branch, err)
		}
		if _, err := r.CommitStaged("hdf: enroll dotfile.txt"); err != nil {
			t.Fatalf("host %s CommitStaged: %v", spec.branch, err)
		}
		if err := r.Push(spec.branch); err != nil {
			t.Fatalf("host %s Push: %v", spec.branch, err)
		}

		if err := config.Save(env.cfgPath, &config.Config{
			Branch:           spec.branch,
			LocalDotfilesDir: env.workDir,
			GitPushTarget:    bareURL,
		}); err != nil {
			t.Fatalf("host %s Save config: %v", spec.branch, err)
		}
	}

	// --- commit SharedSettings (threshold=3) + main registry to main via host A ---
	ssBytes, err := config.SharedSettingsToBytes(&config.SharedSettings{NotifyThreshold: 3})
	if err != nil {
		t.Fatalf("SharedSettingsToBytes: %v", err)
	}

	// Include host A's file path in main registry so cross-host discovery works.
	hostAFilePath := filepath.Join(envs[0].homeDir, "dotfile.txt")
	mainReg := &config.Registry{
		Files: []config.ManagedFile{
			{Path: hostAFilePath, Hash: ""},
		},
	}
	mainRegBytes, err := config.RegistryToBytes(mainReg)
	if err != nil {
		t.Fatalf("RegistryToBytes: %v", err)
	}

	if _, err := envs[0].r.CommitFilesToBranch("main", []repo.BranchFile{
		{RepoRelPath: config.SharedSettingsFile, Content: ssBytes},
		{RepoRelPath: ".hdf/managed.toml", Content: mainRegBytes},
	}, "hdf: configure shared settings and register host-a file"); err != nil {
		t.Fatalf("CommitFilesToBranch main: %v", err)
	}
	if err := envs[0].r.Push("main"); err != nil {
		t.Fatalf("Push main: %v", err)
	}

	// --- create drift: write drifted content directly to the repo file ---
	// homeDir/dotfile.txt is a symlink → workDir/dotfile.txt.
	// Writing to workDir/dotfile.txt changes disk content while leaving
	// the committed content (and registry hash) at the original value.
	for i, spec := range hostSpecs {
		repoFile := filepath.Join(envs[i].workDir, "dotfile.txt")
		if err := os.WriteFile(repoFile, []byte(spec.driftedContent), 0o644); err != nil {
			t.Fatalf("host %s drift write: %v", spec.branch, err)
		}
	}

	// --- run one sync cycle per host ---
	for i, spec := range hostSpecs {
		env := envs[i]
		if err := syncWithHome(env.cfgPath, env.statePath, env.notifier, env.homeDir); err != nil {
			t.Fatalf("host %s syncWithHome: %v", spec.branch, err)
		}
	}

	// --- assert drift notification counts ---
	// threshold = 3; host-a has 1 hunk, host-b has 3, host-c has 5.
	if got := envs[0].notifier.driftCount(); got != 0 {
		t.Errorf("host-a: want 0 drift notifications (1 hunk < 3), got %d — messages: %v",
			got, envs[0].notifier.msgs)
	}
	if got := envs[1].notifier.driftCount(); got != 1 {
		t.Errorf("host-b: want 1 drift notification (3 hunks >= 3), got %d — messages: %v",
			got, envs[1].notifier.msgs)
	}
	if got := envs[2].notifier.driftCount(); got != 1 {
		t.Errorf("host-c: want 1 drift notification (5 hunks >= 3), got %d — messages: %v",
			got, envs[2].notifier.msgs)
	}

	// --- cross-host link sub-test ---
	// Verify that host B can discover host A's registered file from origin/main
	// and create a local symlink for it.
	t.Run("CrossHostLink", func(t *testing.T) {
		// Open a fresh repo handle — the envs[1].r instance predates the fetch
		// that syncWithHome performed internally, so its in-memory ref cache is stale.
		freshR, err := repo.Open(envs[1].workDir)
		if err != nil {
			t.Fatalf("open host B repo: %v", err)
		}
		regBytes, err := freshR.ReadFileFromRemoteBranch("origin", "main", ".hdf/managed.toml")
		if err != nil {
			t.Fatalf("ReadFileFromRemoteBranch managed.toml: %v", err)
		}
		if regBytes == nil {
			t.Fatal("expected managed.toml on origin/main, got nil")
		}

		discoveredReg, err := config.RegistryFromBytes(regBytes)
		if err != nil {
			t.Fatalf("RegistryFromBytes: %v", err)
		}

		found := false
		for _, f := range discoveredReg.Files {
			if f.Path != hostAFilePath {
				continue
			}
			found = true

			// Host B creates a stub at their own repo path and symlinks to it.
			stubPath := filepath.Join(envs[1].workDir, "dotfile-from-a.txt")
			if err := os.WriteFile(stubPath, []byte{}, 0o644); err != nil {
				t.Fatalf("creating stub: %v", err)
			}
			linkTarget := filepath.Join(envs[1].homeDir, "dotfile-from-a.txt")
			if err := link.Link(linkTarget, stubPath); err != nil {
				t.Fatalf("link: %v", err)
			}
			info, err := os.Lstat(linkTarget)
			if err != nil {
				t.Errorf("symlink missing after cross-host link: %v", err)
			} else if info.Mode()&os.ModeSymlink == 0 {
				t.Error("expected symlink, got regular file")
			}
		}
		if !found {
			t.Errorf("host A's file %q not found in origin/main registry; files: %v",
				hostAFilePath, discoveredReg.Files)
		}
	})
}
