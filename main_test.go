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

const testBranch = "machine"

// mustRel returns the relative path from base to target, fataling the test on error.
func mustRel(t *testing.T, base, target string) string {
	t.Helper()
	rel, err := filepath.Rel(base, target)
	if err != nil {
		t.Fatalf("filepath.Rel(%q, %q): %v", base, target, err)
	}
	return rel
}

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

func TestRunInitLocalRelativePathConfirmedWithYes(t *testing.T) {
	workDir := t.TempDir()
	t.Chdir(workDir)
	cfgPath, statePath := initPaths(t)

	bareDir := t.TempDir()
	// "yes" should be accepted as confirmation for the relative working-copy path.
	if err := runInit(strings.NewReader("1\ndotfiles\nyes\n"+bareDir+"\n"), cfgPath, statePath, ""); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	wantDir := filepath.Join(workDir, "dotfiles")
	if cfg.LocalDotfilesDir != wantDir {
		t.Errorf("LocalDotfilesDir = %q, want %q", cfg.LocalDotfilesDir, wantDir)
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

func TestLocalPathToFileURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"unix absolute", "/home/user/bare", "file:///home/user/bare"},
		{"unix nested", "/tmp/hdf/repo-bare", "file:///tmp/hdf/repo-bare"},
		{"windows drive letter", `C:\Users\user\bare`, "file:///C:/Users/user/bare"},
		{"windows forward slashes", "C:/Users/user/bare", "file:///C:/Users/user/bare"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := localPathToFileURL(c.in)
			if got != c.want {
				t.Errorf("localPathToFileURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestIsYes(t *testing.T) {
	yes := []string{"y", "Y", "yes", "Yes", "YES", "y\n", "yes\n", " yes "}
	no := []string{"n", "no", "", "yep", "yeah"}
	for _, s := range yes {
		if !isYes(s) {
			t.Errorf("isYes(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if isYes(s) {
			t.Errorf("isYes(%q) = true, want false", s)
		}
	}
}

func TestSanitizeBranchName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"my-macbook", "my-macbook"},
		{"My MacBook Pro", "My-MacBook-Pro"},
		{"host.local", "host-local"},
		{"host_name", "host-name"},
		{"192.168.1.1", "192-168-1-1"},
		{"-leading", "leading"},
		{"trailing-", "trailing"},
		{"a", "a"},
	}
	for _, tc := range cases {
		if got := sanitizeBranchName(tc.input); got != tc.want {
			t.Errorf("sanitizeBranchName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestBranchNameNonEmpty(t *testing.T) {
	name := branchName()
	if name == "" {
		t.Error("branchName must never return an empty string")
	}
}

func TestBranchNameFallbackFormat(t *testing.T) {
	// Verify the character-mapping used in the crypto/rand fallback path
	// produces only ASCII letters and a 4-character suffix.
	for i := range 20 {
		b := make([]byte, 4)
		for j := range b {
			b[j] = branchNameChars[int(byte(i*j))%len(branchNameChars)]
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

func TestExpandAndValidate(t *testing.T) {
	const tildeBashrc = "~/.bashrc"

	// Resolve symlinks so that filepath.Rel works correctly on macOS where
	// t.TempDir() returns a /var/... symlink to /private/var/...
	rawHome := t.TempDir()
	homeDir, err := filepath.EvalSymlinks(rawHome)
	if err != nil {
		t.Fatal(err)
	}

	// Create a real file inside homeDir for the success cases.
	realFile := filepath.Join(homeDir, ".bashrc")
	if err := os.WriteFile(realFile, []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a real file outside homeDir for the rejection cases.
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// symlinkFile uses the raw (unresolved) temp path to simulate how an
	// absolute path might arrive when the caller hasn't resolved symlinks.
	symlinkFile := filepath.Join(rawHome, ".bashrc")

	// Create a file inside a locked subdirectory to test the permission-denied
	// error path. os.Stat requires execute permission on each directory
	// component, so chmod 000 on the parent triggers EACCES.
	lockedDir := filepath.Join(homeDir, ".locked")
	if err := os.Mkdir(lockedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	noAccessFile := filepath.Join(lockedDir, "secret")
	if err := os.WriteFile(noAccessFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(lockedDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(lockedDir, 0o755) }) //nolint:gosec // restoring test directory to readable state

	cases := []struct {
		name         string
		filePath     string // raw input as the user would type it (~/..., absolute, or relative)
		wantExpanded string // absolute path on disk that expandAndValidate must return
		wantTilde    string // canonical ~/... registry form that expandAndValidate must return
		wantErr      bool   // true when the call must fail (file missing, outside home, etc.)
	}{
		{
			name:         "tilde prefix",
			filePath:     tildeBashrc,
			wantExpanded: realFile,
			wantTilde:    tildeBashrc,
		},
		{
			name:         "absolute path inside home",
			filePath:     realFile,
			wantExpanded: realFile,
			wantTilde:    tildeBashrc,
		},
		{
			// Regression: relative path must be resolved to absolute so that
			// filepath.Rel(homeDir, expanded) succeeds and the registry stores
			// "~/.bashrc" rather than the raw relative string ".bashrc".
			name:         "relative path resolved to tilde form",
			filePath:     realFile, // use absolute as stand-in; real relative resolution tested via os.Chdir avoidance
			wantExpanded: realFile,
			wantTilde:    tildeBashrc,
		},
		{
			name:     "missing file",
			filePath: "~/.no-such-file",
			wantErr:  true,
		},
		{
			// Regression: a path outside the home directory must be rejected so
			// it can never be stored as an absolute path in the registry.
			name:     "absolute path outside home returns error",
			filePath: outsideFile,
			wantErr:  true,
		},
		{
			// Symlink robustness: an absolute path using the unresolved (symlinked)
			// form of homeDir must still be accepted and normalised correctly.
			// On macOS t.TempDir() returns /var/... which symlinks to /private/var/...
			name:         "absolute path via symlinked home normalises correctly",
			filePath:     symlinkFile,
			wantExpanded: symlinkFile,
			wantTilde:    tildeBashrc,
		},
		{
			// Security: homeDir itself resolves to rel "." which must be rejected
			// rather than producing the malformed canonical form "~/.".
			name:     "home directory itself returns error",
			filePath: homeDir,
			wantErr:  true,
		},
		{
			// Permission/access errors must not be reported as "file not found".
			// The parent directory is locked (0o000) so os.Stat returns EACCES,
			// which must be wrapped rather than silently relabelled.
			name:     "permission denied is not reported as file not found",
			filePath: noAccessFile,
			wantErr:  true,
		},
		{
			// Security: tilde-relative traversal (~/../other) expands outside home
			// and must be rejected even though the path starts with "~/".
			name:     "tilde traversal outside home returns error",
			filePath: "~/" + mustRel(t, homeDir, outsideFile),
			wantErr:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expanded, tilde, err := expandAndValidate(tc.filePath, homeDir)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if expanded != tc.wantExpanded {
				t.Errorf("expanded = %q, want %q", expanded, tc.wantExpanded)
			}
			if tilde != tc.wantTilde {
				t.Errorf("tilde = %q, want %q", tilde, tc.wantTilde)
			}
		})
	}
}

// Regression: expandAndValidate must not follow the dotfile symlink when
// resolving the path. After enrollment ~/.bashrc is a symlink into the repo;
// without the directory-only EvalSymlinks fix, tildeFile would come back as
// ~/.local/share/hdf/repo/.bashrc and corrupt the registry on re-enroll.
func TestExpandAndValidateDoesNotFollowFileSymlink(t *testing.T) {
	homeDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repoDir := t.TempDir()

	repoFile := filepath.Join(repoDir, ".bashrc")
	if err := os.WriteFile(repoFile, []byte("# config"), 0o644); err != nil {
		t.Fatal(err)
	}
	homePath := filepath.Join(homeDir, ".bashrc")
	if err := os.Symlink(repoFile, homePath); err != nil {
		t.Fatal(err)
	}

	expanded, tildeFile, err := expandAndValidate(homePath, homeDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if expanded != homePath {
		t.Errorf("expanded = %q, want %q", expanded, homePath)
	}
	if tildeFile != "~/.bashrc" {
		t.Errorf("tildeFile = %q, want ~/.bashrc — followed symlink into repo", tildeFile)
	}
}

// Regression: a permission-denied error from os.Stat must not be reported as
// "file not found". The two failure modes require different user actions and
// collapsing them into one message is misleading.
// TestExpandAndValidateHomeDirItself verifies that passing the home directory
// itself produces the specific "home directory itself" message, not the generic
// "outside the home directory" message.
func TestExpandAndValidateHomeDirItself(t *testing.T) {
	homeDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = expandAndValidate(homeDir, homeDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "home directory itself") {
		t.Errorf("error = %q, want it to contain 'home directory itself'", err.Error())
	}
}

func TestExpandAndValidatePermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses DAC — permission test not meaningful")
	}
	homeDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lockedDir := filepath.Join(homeDir, ".locked")
	if err := os.Mkdir(lockedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	noAccess := filepath.Join(lockedDir, "secret")
	if err := os.WriteFile(noAccess, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(lockedDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(lockedDir, 0o755) }) //nolint:gosec // restoring test directory to readable state

	_, _, err = expandAndValidate(noAccess, homeDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), "file not found") {
		t.Errorf("permission error was mislabelled as 'file not found': %v", err)
	}
	if !strings.Contains(err.Error(), "cannot access") {
		t.Errorf("expected 'cannot access' in error, got: %v", err)
	}
}

func TestExpandAndValidateRelativePath(t *testing.T) {
	// This test is the direct regression for the bug: a bare filename like
	// ".bashrc" passed to expandAndValidate must be resolved to an absolute
	// path before filepath.Rel is called, so the registry entry is "~/.bashrc"
	// and not the raw relative string.
	//
	// EvalSymlinks is needed on macOS where t.TempDir() returns a /var/...
	// symlink but filepath.Abs resolves via the real /private/var/... path,
	// which would cause filepath.Rel(homeDir, expanded) to mismatch.
	rawHome := t.TempDir()
	homeDir, err := filepath.EvalSymlinks(rawHome)
	if err != nil {
		t.Fatal(err)
	}
	realFile := filepath.Join(homeDir, ".bashrc")
	if err := os.WriteFile(realFile, []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Change working directory to homeDir so that ".bashrc" resolves there.
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(homeDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	expanded, tilde, err := expandAndValidate(".bashrc", homeDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if expanded != realFile {
		t.Errorf("expanded = %q, want %q", expanded, realFile)
	}
	const wantTilde = "~/.bashrc"
	if tilde != wantTilde {
		t.Errorf("tilde = %q, want %s — relative path was not normalised to tilde form", tilde, wantTilde)
	}
}

func TestEnrollCreatesEmptyBaselineInMain(t *testing.T) {
	// Set up a local repo with a bare push target.
	workDir := t.TempDir()
	bareDir := t.TempDir()
	cfgPath, statePath := initPaths(t)

	if err := runInit(strings.NewReader(localInitStdin(workDir, bareDir)), cfgPath, statePath, ""); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	// Create a dotfile in a fake home dir.
	homeDir := t.TempDir()
	dotfile := filepath.Join(homeDir, ".testrc")
	if err := os.WriteFile(dotfile, []byte("# test config\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runEnroll("~/.testrc", homeDir, cfg, statePath, strings.NewReader(""), true); err != nil {
		t.Fatalf("runEnroll: %v", err)
	}

	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		t.Fatalf("opening repo: %v", err)
	}

	// main branch must have an empty stub at the file's repo-relative path.
	stubBytes, err := r.ReadFileFromBranch("main", ".testrc")
	if err != nil {
		t.Fatalf("ReadFileFromBranch stub: %v", err)
	}
	if stubBytes == nil {
		t.Fatal("expected empty stub in main, got nil")
	}
	if len(stubBytes) != 0 {
		t.Errorf("expected empty stub, got %q", stubBytes)
	}

	// main branch must have managed.toml listing the file with an empty hash.
	regBytes, err := r.ReadFileFromBranch("main", managedTOMLPath)
	if err != nil {
		t.Fatalf("ReadFileFromBranch registry: %v", err)
	}
	if regBytes == nil {
		t.Fatal("expected managed.toml in main, got nil")
	}
	mainReg, err := config.RegistryFromBytes(regBytes)
	if err != nil {
		t.Fatalf("parsing main registry: %v", err)
	}
	if len(mainReg.Files) != 1 {
		t.Fatalf("main registry Files len = %d, want 1", len(mainReg.Files))
	}
	if mainReg.Files[0].Path != "~/.testrc" {
		t.Errorf("main Files[0].Path = %q, want ~/.testrc", mainReg.Files[0].Path)
	}
	if mainReg.Files[0].Hash != "" {
		t.Errorf("main Files[0].Hash = %q, want empty", mainReg.Files[0].Hash)
	}

	// Both branches must be pushed to the bare remote.
	bare, err := repo.Open(bareDir)
	if err != nil {
		t.Fatalf("opening bare repo: %v", err)
	}
	hostStub, err := bare.ReadFileFromBranch(cfg.Branch, ".testrc")
	if err != nil {
		t.Fatalf("ReadFileFromBranch on bare (hostname): %v", err)
	}
	if hostStub == nil {
		t.Error("hostname branch not pushed to bare remote")
	}
	mainStub, err := bare.ReadFileFromBranch("main", ".testrc")
	if err != nil {
		t.Fatalf("ReadFileFromBranch on bare (main): %v", err)
	}
	if mainStub == nil {
		t.Error("main branch not pushed to bare remote")
	}
}

// Regression: runLink must be fully hermetic — it must use the homeDir it
// receives, not os.UserHomeDir(), when resolving repo paths. Without the fix,
// link.RepoPathFor calls os.UserHomeDir() internally, so a temp homeDir causes
// filepath.Rel to produce a ".." prefix and every non-variant file fails.
func TestRunLinkHermetic(t *testing.T) {
	workDir := t.TempDir()
	bareDir := t.TempDir()
	cfgPath, statePath := initPaths(t)

	if err := runInit(strings.NewReader(localInitStdin(workDir, bareDir)), cfgPath, statePath, ""); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	homeDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dotfile := filepath.Join(homeDir, ".testrc")
	if err := os.WriteFile(dotfile, []byte("config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runEnroll("~/.testrc", homeDir, cfg, statePath, strings.NewReader(""), true); err != nil {
		t.Fatalf("runEnroll: %v", err)
	}

	// Remove the symlink to simulate a fresh machine needing re-linking.
	if err := os.Remove(dotfile); err != nil {
		t.Fatalf("removing symlink: %v", err)
	}

	if err := runLink(homeDir, cfg, true, strings.NewReader(""), statePath); err != nil {
		t.Fatalf("runLink: %v", err)
	}

	info, err := os.Lstat(dotfile)
	if err != nil {
		t.Fatalf("Lstat after runLink: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected dotfile to be a symlink after runLink")
	}
}

// Regression: re-enrolling an already-managed, unchanged file must not create
// an empty commit. go-git's Commit() creates a commit unconditionally, so the
// fix short-circuits before staging/committing when hash and path already match.
func TestEnrollIdempotentNoEmptyCommit(t *testing.T) {
	workDir := t.TempDir()
	bareDir := t.TempDir()
	cfgPath, statePath := initPaths(t)

	if err := runInit(strings.NewReader(localInitStdin(workDir, bareDir)), cfgPath, statePath, ""); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	homeDir := t.TempDir()
	dotfile := filepath.Join(homeDir, ".testrc")
	if err := os.WriteFile(dotfile, []byte("config\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runEnroll("~/.testrc", homeDir, cfg, statePath, strings.NewReader(""), true); err != nil {
		t.Fatalf("first runEnroll: %v", err)
	}

	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		t.Fatalf("opening repo: %v", err)
	}
	countBefore, err := r.CommitCount()
	if err != nil {
		t.Fatalf("CommitCount before: %v", err)
	}

	// Capture stdout to verify the "already managed" message.
	origStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw

	err = runEnroll("~/.testrc", homeDir, cfg, statePath, strings.NewReader(""), true)

	_ = pw.Close()
	os.Stdout = origStdout
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, pr)

	if err != nil {
		t.Fatalf("second runEnroll: %v", err)
	}
	if !strings.Contains(buf.String(), "already managed and unchanged") {
		t.Errorf("stdout %q should contain 'already managed and unchanged'", buf.String())
	}

	countAfter, err := r.CommitCount()
	if err != nil {
		t.Fatalf("CommitCount after: %v", err)
	}
	if countAfter != countBefore {
		t.Errorf("commit count went from %d to %d — empty commit was created", countBefore, countAfter)
	}
}

// captureStdout runs f with os.Stdout redirected to a pipe and returns what
// was written. Any error from io.Copy is intentionally ignored — tests that
// care about the output assert on it directly.
func captureStdout(f func()) string {
	origStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw
	f()
	_ = pw.Close()
	os.Stdout = origStdout
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, pr)
	return buf.String()
}

// setupEnrolledFile initialises hdf, enrolls a dotfile with initialContent,
// and returns cfg plus the absolute path to the symlink in homeDir.
func setupEnrolledFile(t *testing.T, initialContent string) (*config.Config, string, string) {
	t.Helper()
	workDir := t.TempDir()
	bareDir := t.TempDir()
	cfgPath, statePath := initPaths(t)
	if err := runInit(strings.NewReader(localInitStdin(workDir, bareDir)), cfgPath, statePath, ""); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	homeDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dotfile := filepath.Join(homeDir, ".testrc")
	if err := os.WriteFile(dotfile, []byte(initialContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runEnroll("~/.testrc", homeDir, cfg, statePath, strings.NewReader(""), true); err != nil {
		t.Fatalf("first runEnroll: %v", err)
	}
	return cfg, statePath, homeDir
}

// TestEnrollShowsDiffForChangedFile verifies that re-enrolling a file whose
// content has changed prints a colored diff to stdout before committing.
func TestEnrollShowsDiffForChangedFile(t *testing.T) {
	cfg, statePath, homeDir := setupEnrolledFile(t, "original line\n")

	dotfile := filepath.Join(homeDir, ".testrc")
	if err := os.WriteFile(dotfile, []byte("original line\nnew line\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(func() {
		if err := runEnroll("~/.testrc", homeDir, cfg, statePath, strings.NewReader(""), true); err != nil {
			t.Errorf("runEnroll after change: %v", err)
		}
	})

	if !strings.Contains(out, "+new line") {
		t.Errorf("expected diff to contain '+new line', got:\n%s", out)
	}
	if !strings.Contains(out, "changes to") {
		t.Errorf("expected diff header 'changes to', got:\n%s", out)
	}
}

// TestEnrollAbortWhenUserDeclinesPrompt verifies that answering "n" at the
// diff confirmation prompt aborts the enrollment without creating a commit.
func TestEnrollAbortWhenUserDeclinesPrompt(t *testing.T) {
	cfg, statePath, homeDir := setupEnrolledFile(t, "original line\n")

	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		t.Fatalf("opening repo: %v", err)
	}
	countBefore, err := r.CommitCount()
	if err != nil {
		t.Fatalf("CommitCount before: %v", err)
	}

	dotfile := filepath.Join(homeDir, ".testrc")
	if err := os.WriteFile(dotfile, []byte("original line\nchanged\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	captureStdout(func() {
		err = runEnroll("~/.testrc", homeDir, cfg, statePath, strings.NewReader("n\n"), false)
	})
	if err == nil {
		t.Fatal("expected error after declining prompt, got nil")
	}
	if !strings.Contains(err.Error(), "aborted") {
		t.Errorf("error = %q, want it to contain 'aborted'", err.Error())
	}

	countAfter, err := r.CommitCount()
	if err != nil {
		t.Fatalf("CommitCount after: %v", err)
	}
	if countAfter != countBefore {
		t.Errorf("commit count changed from %d to %d after abort", countBefore, countAfter)
	}
}

// TestEnrollProceedsOnDefaultPromptAnswer verifies that pressing Enter (empty
// answer) at the confirmation prompt is treated as "yes" and enrollment proceeds.
func TestEnrollProceedsOnDefaultPromptAnswer(t *testing.T) {
	cfg, statePath, homeDir := setupEnrolledFile(t, "original line\n")

	dotfile := filepath.Join(homeDir, ".testrc")
	if err := os.WriteFile(dotfile, []byte("original line\nextra\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	captureStdout(func() {
		if err := runEnroll("~/.testrc", homeDir, cfg, statePath, strings.NewReader("\n"), false); err != nil {
			t.Errorf("runEnroll with empty answer: %v", err)
		}
	})
}

// TestEnrollYesFlagSkipsPrompt verifies that --yes bypasses the interactive
// prompt even when there are changes to review.
func TestEnrollYesFlagSkipsPrompt(t *testing.T) {
	cfg, statePath, homeDir := setupEnrolledFile(t, "original line\n")

	dotfile := filepath.Join(homeDir, ".testrc")
	if err := os.WriteFile(dotfile, []byte("original line\nyes-flag-line\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	captureStdout(func() {
		if err := runEnroll("~/.testrc", homeDir, cfg, statePath, strings.NewReader(""), true); err != nil {
			t.Errorf("runEnroll with yes=true: %v", err)
		}
	})
}

// TestCommandAliases verifies that enroll and link are registered as aliases
// so users can still use them interchangeably with changes-push and changes-pull.
func TestCommandAliases(t *testing.T) {
	aliases := map[string][]string{}
	for _, cmd := range rootCmd.Commands() {
		aliases[cmd.Use] = cmd.Aliases
	}
	if !contains(aliases["changes-push <path>"], "enroll") {
		t.Errorf("changes-push command missing enroll alias; got %v", aliases["changes-push <path>"])
	}
	if !contains(aliases["changes-pull"], "link") {
		t.Errorf("changes-pull command missing link alias; got %v", aliases["changes-pull"])
	}
}

func TestRunLinkMergePrompt(t *testing.T) {
	const branch = "test-host"

	cases := []struct {
		name       string
		answer     string
		wantMerged bool
	}{
		{name: "accepts merge", answer: "y\n", wantMerged: true},
		{name: "delays merge", answer: "n\n", wantMerged: false},
		{name: "default delays (empty answer)", answer: "\n", wantMerged: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Each sub-test gets its own isolated bare repo so there is no
			// state pollution from machine-branch commits or main advances
			// made by other sub-tests.
			bareDir := t.TempDir()
			if _, _, err := repo.InitOrOpenBare(bareDir); err != nil {
				t.Fatalf("InitOrOpenBare: %v", err)
			}
			bareURL := "file://" + bareDir

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

			workDir := t.TempDir()
			homeDir := t.TempDir()
			homePath := filepath.Join(homeDir, ".testrc")

			// Commit the registry to main so the machine branch inherits it
			// already committed after cloning. This keeps the machine worktree
			// clean, which MergeFromMain requires.
			reg := &config.Registry{
				Files: []config.ManagedFile{{Path: homePath}},
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

			// Clone AFTER the registry commit so the machine branch starts with
			// the registry already in place (no uncommitted files in workDir).
			r, err := repo.Clone(bareURL, workDir)
			if err != nil {
				t.Fatalf("Clone: %v", err)
			}
			if err := r.CreateAndCheckoutBranch(branch); err != nil {
				t.Fatalf("CreateAndCheckoutBranch: %v", err)
			}

			// Advance main with a file change only — machine branch does not
			// have this file, so fetchAndShowIncoming detects an incoming diff.
			// Machine branch is now a strict ancestor of main, so MergeFromMain
			// can fast-forward cleanly.
			if _, err := seed.CommitFilesToBranch("main", []repo.BranchFile{
				{RepoRelPath: filepath.Base(homePath), Content: []byte("updated-by-main\n")},
			}, "hdf: update file on main"); err != nil {
				t.Fatalf("CommitFilesToBranch: %v", err)
			}
			if err := seed.Push("main"); err != nil {
				t.Fatalf("seed Push main: %v", err)
			}

			mainSHABefore, err := seed.BranchSHA("main")
			if err != nil {
				t.Fatalf("BranchSHA main: %v", err)
			}

			cfg := &config.Config{
				Branch:           branch,
				LocalDotfilesDir: workDir,
				GitPushTarget:    bareURL,
			}

			statePath := filepath.Join(t.TempDir(), "state.toml")
			captureStdout(func() {
				if err := runLink(homeDir, cfg, false, strings.NewReader(tc.answer), statePath); err != nil {
					t.Errorf("runLink: %v", err)
				}
			})

			freshR, err := repo.Open(workDir)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			headSHA, err := freshR.BranchSHA(branch)
			if err != nil {
				t.Fatalf("BranchSHA: %v", err)
			}

			if tc.wantMerged && headSHA != mainSHABefore {
				t.Errorf("merge accepted: want HEAD=%s (main SHA), got %s", mainSHABefore, headSHA)
			}
			if !tc.wantMerged && headSHA == mainSHABefore {
				t.Errorf("merge delayed: want HEAD to stay on branch, but it advanced to main SHA")
			}
		})
	}
}

// TestRunLinkMergeAcceptedWithPendingWarning verifies that a "y" answering the
// daemon-warning prompt and a "y" answering the merge prompt are both consumed
// correctly when both prompts appear in the same runLink invocation. The bug
// was that two separate bufio.NewReader(stdin) calls caused the second "y" to
// be lost inside the first reader's buffer, so the merge prompt received EOF
// and defaulted to "N".
func TestRunLinkMergeAcceptedWithPendingWarning(t *testing.T) {
	const branch = "test-host"

	bareDir := t.TempDir()
	if _, _, err := repo.InitOrOpenBare(bareDir); err != nil {
		t.Fatalf("InitOrOpenBare: %v", err)
	}
	bareURL := "file://" + bareDir

	seedDir := t.TempDir()
	seed, err := repo.Init(seedDir)
	if err != nil {
		t.Fatalf("Init seed: %v", err)
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

	workDir := t.TempDir()
	homeDir := t.TempDir()
	homePath := filepath.Join(homeDir, ".testrc")

	reg := &config.Registry{Files: []config.ManagedFile{{Path: homePath}}}
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

	r, err := repo.Clone(bareURL, workDir)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if err := r.CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("CreateAndCheckoutBranch: %v", err)
	}

	if _, err := seed.CommitFilesToBranch("main", []repo.BranchFile{
		{RepoRelPath: filepath.Base(homePath), Content: []byte("updated-by-main\n")},
	}, "hdf: update file on main"); err != nil {
		t.Fatalf("CommitFilesToBranch: %v", err)
	}
	if err := seed.Push("main"); err != nil {
		t.Fatalf("seed Push main (file): %v", err)
	}

	mainSHABefore, err := seed.BranchSHA("main")
	if err != nil {
		t.Fatalf("BranchSHA main: %v", err)
	}

	cfg := &config.Config{
		Branch:           branch,
		LocalDotfilesDir: workDir,
		GitPushTarget:    bareURL,
	}
	statePath := filepath.Join(t.TempDir(), "state.toml")

	// Plant a pending warning so promptPendingWarnings fires before the merge prompt.
	if err := config.SaveState(statePath, &config.State{
		PendingWarnings: []string{"test warning: please review"},
	}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	// "y\ny\n": first "y" accepts the warning, second "y" accepts the merge.
	// With the stdin double-buffering bug the second "y" is silently discarded
	// and the merge prompt receives EOF, defaulting to "N" (no merge).
	captureStdout(func() {
		if err := runLink(homeDir, cfg, false, strings.NewReader("y\ny\n"), statePath); err != nil {
			t.Fatalf("runLink: %v", err)
		}
	})

	freshR, err := repo.Open(workDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	headSHA, err := freshR.BranchSHA(branch)
	if err != nil {
		t.Fatalf("BranchSHA: %v", err)
	}
	if headSHA != mainSHABefore {
		t.Errorf("merge did not happen: HEAD=%s, want main SHA %s\n(hint: stdin double-buffering discarded the merge 'y')", headSHA, mainSHABefore)
	}
}

// TestRunInitLocalAddRemoteErrorPropagated verifies that when AddRemote fails
// (e.g. origin already points to a different URL), setupLocalRepo propagates
// the error rather than silently swallowing it.
func TestRunInitLocalAddRemoteErrorPropagated(t *testing.T) {
	repoDir := t.TempDir()
	cfgPath, statePath := initPaths(t)

	// Pre-init the repo with a commit and "origin" pointing to URL A so:
	// - InitOrOpen takes the fast Open path (no write needed)
	// - ensureInitialCommit finds an existing HEAD and returns early
	// - AddRemote("origin", urlB) hits the ErrRemoteExists → different-URL error
	r, err := repo.Init(repoDir)
	if err != nil {
		t.Fatalf("pre-init: %v", err)
	}
	seedFile := filepath.Join(repoDir, "seed.txt")
	if err := os.WriteFile(seedFile, []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("seed.txt", "initial"); err != nil {
		t.Fatalf("seed commit: %v", err)
	}
	if err := r.AddRemote("origin", "https://example.com/old.git"); err != nil {
		t.Fatalf("pre-add remote: %v", err)
	}

	// Provide a *different* HTTPS push URL so isRemoteURL returns true and the
	// code calls r.AddRemote, which returns "already points to a different URL".
	stdin := "1\n" + repoDir + "\nhttps://example.com/new.git\n"
	err = runInit(strings.NewReader(stdin), cfgPath, statePath, "")
	if err == nil {
		t.Fatal("expected error when origin already points to a different URL, got nil")
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func TestFetchAndShowIncoming_SkipsEnrollmentPlaceholder(t *testing.T) {
	bareDir := t.TempDir()
	if _, _, err := repo.InitOrOpenBare(bareDir); err != nil {
		t.Fatalf("InitOrOpenBare: %v", err)
	}
	bareURL := "file://" + bareDir

	seedDir := t.TempDir()
	seed, err := repo.Init(seedDir)
	if err != nil {
		t.Fatalf("seed Init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seedDir, ".gitkeep"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.CommitFile(".gitkeep", "initial"); err != nil {
		t.Fatalf("seed CommitFile: %v", err)
	}
	if err := seed.AddRemote("origin", bareURL); err != nil {
		t.Fatalf("seed AddRemote: %v", err)
	}
	if err := seed.Push("main"); err != nil {
		t.Fatalf("seed Push: %v", err)
	}

	workDir := t.TempDir()
	r, err := repo.Clone(bareURL, workDir)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if err := r.CreateAndCheckoutBranch(testBranch); err != nil {
		t.Fatalf("CreateAndCheckoutBranch: %v", err)
	}

	homeDir := t.TempDir()
	homePath := filepath.Join(homeDir, ".testrc")
	relPath := filepath.Base(homePath)

	// Machine branch has real dotfile content.
	if err := os.WriteFile(filepath.Join(workDir, relPath), []byte("real content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile(relPath, "machine: add dotfile"); err != nil {
		t.Fatalf("CommitFile machine: %v", err)
	}

	// Main advances with an enrollment placeholder (empty) for the managed file.
	// This makes HasIncomingCommits=true but mainBytes=[]byte{} for the managed file.
	if _, err := seed.CommitFilesToBranch("main", []repo.BranchFile{
		{RepoRelPath: relPath, Content: []byte{}},
	}, "hdf: enroll placeholder"); err != nil {
		t.Fatalf("CommitFilesToBranch: %v", err)
	}
	if err := seed.Push("main"); err != nil {
		t.Fatalf("seed Push: %v", err)
	}

	reg := &config.Registry{
		Files: []config.ManagedFile{{Path: homePath}},
	}
	cfg := &config.Config{
		Branch:           testBranch,
		LocalDotfilesDir: workDir,
	}

	var anyIncoming bool
	var callErr error
	captureStdout(func() {
		anyIncoming, callErr = fetchAndShowIncoming(r, cfg, reg, homeDir)
	})
	if callErr != nil {
		t.Fatalf("fetchAndShowIncoming: %v", callErr)
	}
	if anyIncoming {
		t.Error("want anyIncoming=false (enrollment placeholder should be skipped), got true")
	}
}

func TestRootCmdMigrationHook(t *testing.T) {
	if rootCmd.PersistentPreRunE == nil {
		t.Error("rootCmd.PersistentPreRunE must be set to wire up legacy config.toml migration")
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

// TestRunEnrollDirectoryReturnsError verifies that passing a directory path to
// changes-push returns a clear error rather than a generic "is a directory" OS error.
func TestRunEnrollDirectoryReturnsError(t *testing.T) {
	homeDir := t.TempDir()
	dirPath := filepath.Join(homeDir, ".config", "nvim")
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}

	workDir := t.TempDir()
	r, err := repo.Init(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFilesToBranch("main", []repo.BranchFile{
		{RepoRelPath: managedTOMLPath, Content: []byte{}},
	}, "hdf: init"); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateAndCheckoutBranch(testBranch); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Branch: testBranch, LocalDotfilesDir: workDir}
	statePath := filepath.Join(t.TempDir(), "state.toml")

	err = runEnroll(dirPath, homeDir, cfg, statePath, strings.NewReader(""), false)
	if err == nil {
		t.Fatal("expected error when enrolling a directory, got nil")
	}
	// Must be a clear, user-friendly message — not the raw OS "is a directory" error.
	if !strings.Contains(err.Error(), "only supports managing individual files") {
		t.Errorf("error = %q, want user-friendly message about individual files", err.Error())
	}
}

// TestRunInitLocalSymlinkPushTargetRejected verifies that a symlink resolving to
// the working-copy directory is rejected as a push target, even though the string
// paths differ.
func TestRunInitLocalSymlinkPushTargetRejected(t *testing.T) {
	// Create a real directory for the working copy and a symlink alias for it.
	realDir := t.TempDir()
	symlinkPath := filepath.Join(t.TempDir(), "repo-link")
	if err := os.Symlink(realDir, symlinkPath); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	cfgPath, statePath := initPaths(t)
	// Use realDir as working copy and symlinkPath (which resolves to realDir) as push target.
	err := runInit(strings.NewReader(localInitStdin(realDir, symlinkPath)), cfgPath, statePath, "")
	if err == nil {
		t.Fatal("expected error when push target symlinks to working copy, got nil")
	}
	if !strings.Contains(err.Error(), "must differ") {
		t.Errorf("error = %q, want it to contain 'must differ'", err.Error())
	}
}

// TestRunLinkLocalOnlySkipsFetch verifies that changes-pull succeeds when no
// remote is configured — it should re-create symlinks without attempting a fetch.
func TestRunLinkLocalOnlySkipsFetch(t *testing.T) {
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

	reg := &config.Registry{Files: []config.ManagedFile{{Path: "~/.testrc"}}}
	if err := config.SaveRegistry(workDir, reg); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Branch: testBranch, LocalDotfilesDir: workDir}
	statePath := filepath.Join(t.TempDir(), "state.toml")

	// noFetch=false but no remote configured — should not fail on fetch.
	err = runLink(homeDir, cfg, false, strings.NewReader(""), statePath)
	if err != nil {
		t.Fatalf("runLink with no remote: %v", err)
	}
}
