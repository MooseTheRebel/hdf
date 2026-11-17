//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"hdf/config"
	"hdf/link"
	"hdf/repo"
)

type FileState int

const (
	Untracked    FileState = iota
	Enrolled               // content on machine branch; main has empty placeholder
	Promoted               // real content on origin/main not yet on machine branch
	Synced                 // machine branch content matches origin/main; symlink exists
	Diverged               // machine branch content differs from origin/main
	RegistryOnly           // in registry; no machine branch content; main is empty
)

func (s FileState) String() string {
	return [...]string{"Untracked", "Enrolled", "Promoted", "Synced", "Diverged", "RegistryOnly"}[s]
}

// TestDeriveFileStateVariantFile verifies that deriveFileState uses the
// variant-specific repo path for files with branch variants, not the canonical
// path. Without the fix, a variant file enrolled on branch X would be reported
// as RegistryOnly instead of Enrolled because the canonical path has no content.
func TestDeriveFileStateVariantFile(t *testing.T) {
	t.Parallel()
	nodes, _ := setupCluster(t, 1)
	nodeA := nodes[0]

	cfg := nodeConfig(t, nodeA)
	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		t.Fatal(err)
	}

	tildeFile := "~/.testrc"
	variantRepoPath := ".testrc." + nodeA.branch

	// Register the variant file in managed.toml on main (mirrors updateMainRegistry).
	mainReg := &config.Registry{
		Files: []config.ManagedFile{{
			Path: tildeFile,
			Hash: "abc123",
			Variants: []config.Variant{{
				Branch:   nodeA.branch,
				RepoPath: variantRepoPath,
			}},
		}},
	}
	mainRegBytes, err := config.RegistryToBytes(mainReg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFilesToBranch("main", []repo.BranchFile{
		{RepoRelPath: ".hdf/managed.toml", Content: mainRegBytes},
	}, "hdf: register variant .testrc"); err != nil {
		t.Fatal(err)
	}
	if err := r.Push("main"); err != nil {
		t.Fatal(err)
	}

	// Commit the variant-specific file to nodeA's branch (mirrors stageAndCommit).
	if err := os.WriteFile(filepath.Join(cfg.LocalDotfilesDir, variantRepoPath), []byte("variant content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.StageFile(variantRepoPath); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitStaged("hdf: enroll variant .testrc"); err != nil {
		t.Fatal(err)
	}
	if err := r.Push(nodeA.branch); err != nil {
		t.Fatal(err)
	}

	// Fetch so origin/* tracking refs are up to date.
	if err := r.Fetch(); err != nil {
		t.Fatal(err)
	}

	// Branch has the variant file; origin/main does not → Enrolled.
	assertFileState(t, nodeA, tildeFile, Enrolled)
}

// Node represents one hdf-managed machine in a cluster.
// branch is unique per node and set via HDF_BRANCH during init.
type Node struct {
	home   string
	branch string
}

// runHDFNode runs the hdf binary with HOME=node.home and HDF_BRANCH=node.branch.
// Use this instead of runHDF whenever operating on a specific cluster node.
func runHDFNode(t *testing.T, node Node, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(hdfBin, args...)
	env := []string{}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "HOME=") || strings.HasPrefix(e, "USERPROFILE=") || strings.HasPrefix(e, "HDF_BRANCH=") {
			continue
		}
		env = append(env, e)
	}
	env = append(env, "HOME="+node.home, "USERPROFILE="+node.home, "HDF_BRANCH="+node.branch)
	cmd.Env = env
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return outBuf.String(), errBuf.String(), exitErr.ExitCode()
		}
	}
	return outBuf.String(), errBuf.String(), 0
}

// gitTempDir is t.TempDir for a bare repo that nodes push to: its cleanup
// retries until the directory is gone instead of failing. Pushing to the
// repo over file:// runs the system git-receive-pack, which starts
// `git maintenance run --auto --detach`; that background housekeeping (e.g.
// a geometric repack) can still be writing when the test ends. Node homes
// stay on t.TempDir, so a process hdf itself leaves behind is still caught.
func gitTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hdf-git-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		deadline := time.Now().Add(10 * time.Second)
		for {
			err := os.RemoveAll(dir)
			if err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Errorf("remove git temp dir %s: %v", dir, err)
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
	return dir
}

// setupCluster creates n nodes all sharing one bare repo.
// Node 0 uses local init (creates the bare repo); nodes 1+ clone from it.
// Each node gets a unique branch name "node-N" via HDF_BRANCH.
// Returns the nodes and the file:// URL of the shared bare repo.
func setupCluster(t *testing.T, n int) ([]Node, string) {
	t.Helper()
	bareDir := gitTempDir(t)
	bareURL := "file://" + bareDir

	nodes := make([]Node, n)
	for i := range nodes {
		home := t.TempDir()
		branch := fmt.Sprintf("node-%d", i)
		nodes[i] = Node{home: home, branch: branch}

		var stdin string
		if i == 0 {
			workDir := filepath.Join(t.TempDir(), "repo")
			stdin = "1\n" + workDir + "\n" + bareDir + "\n"
		} else {
			stdin = "2\n" + bareURL + "\n"
		}
		_, stderr, code := runHDFNode(t, nodes[i], stdin, "init")
		if code != 0 {
			t.Fatalf("setupCluster: node %d init failed (code %d): %s", i, code, stderr)
		}
		if i == 0 {
			// runInit does not push to the bare repo. Seed it so nodes 1+ can clone.
			cfg := nodeConfig(t, nodes[0])
			r, err := repo.Open(cfg.LocalDotfilesDir)
			if err != nil {
				t.Fatalf("setupCluster: open node-0 repo: %v", err)
			}
			if err := r.Push("main"); err != nil {
				t.Fatalf("setupCluster: seeding bare repo: %v", err)
			}
		}
	}
	return nodes, bareURL
}

// hdfPromote runs `hdf promote` on node and fatals the test if it fails.
func hdfPromote(t *testing.T, node Node) {
	t.Helper()
	_, stderr, code := runHDFNode(t, node, "", "promote")
	if code != 0 {
		t.Fatalf("hdf promote failed (code %d): %s", code, stderr)
	}
}

// prMerge simulates a GitHub PR merge by cloning the bare repo into a temp
// dir, merging the node's machine branch into main, and pushing back.
// Conflicts on .hdf/managed.toml are resolved by taking the union of both
// sides' registry entries — matching the semantics hdf uses for promote.
func prMerge(t *testing.T, node Node, bareURL string) {
	t.Helper()
	if err := prMergeIn(t.TempDir(), node, bareURL); err != nil {
		t.Fatalf("prMerge: %v", err)
	}
}

// prMergeIn is prMerge without a *testing.T, cloning into tmpDir.
func prMergeIn(tmpDir string, node Node, bareURL string) error {
	cfg, err := loadNodeConfig(node)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	runMayFail := func(args ...string) ([]byte, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		return cmd.CombinedOutput()
	}
	run := func(args ...string) error {
		if out, err := runMayFail(args...); err != nil {
			return fmt.Errorf("git %v: %w\n%s", args, err, out)
		}
		return nil
	}
	for _, args := range [][]string{
		{"clone", bareURL, "."},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
	} {
		if err := run(args...); err != nil {
			return err
		}
	}

	out, mergeErr := runMayFail("merge", "--no-ff", "origin/"+cfg.Branch, "-m", "Merge "+cfg.Branch+" into main")
	if mergeErr != nil {
		// Resolve managed.toml add/add or content conflict by merging registries.
		oursBytes, e1 := exec.Command("git", "-C", tmpDir, "show", "HEAD:.hdf/managed.toml").Output()
		theirsBytes, e2 := exec.Command("git", "-C", tmpDir, "show", "MERGE_HEAD:.hdf/managed.toml").Output()
		if e1 != nil || e2 != nil {
			return fmt.Errorf("conflict on unexpected file\ngit merge out: %s\nHEAD err: %v\nMERGE_HEAD err: %v", out, e1, e2)
		}
		oursReg, err := config.RegistryFromBytes(oursBytes)
		if err != nil {
			return fmt.Errorf("parse HEAD registry: %w", err)
		}
		theirsReg, err := config.RegistryFromBytes(theirsBytes)
		if err != nil {
			return fmt.Errorf("parse MERGE_HEAD registry: %w", err)
		}
		byPath := make(map[string]config.ManagedFile)
		for _, f := range oursReg.Files {
			byPath[f.Path] = f
		}
		for _, f := range theirsReg.Files {
			byPath[f.Path] = f
		}
		merged := &config.Registry{}
		for _, f := range byPath {
			merged.Files = append(merged.Files, f)
		}
		sort.Slice(merged.Files, func(i, j int) bool {
			return merged.Files[i].Path < merged.Files[j].Path
		})
		b, err := config.RegistryToBytes(merged)
		if err != nil {
			return fmt.Errorf("serialize merged registry: %w", err)
		}
		managedPath := filepath.Join(tmpDir, ".hdf", "managed.toml")
		if err := os.WriteFile(managedPath, b, 0o644); err != nil { //nolint:gosec
			return fmt.Errorf("write managed.toml: %w", err)
		}
		if err := run("add", ".hdf/managed.toml"); err != nil {
			return err
		}
		if err := run("commit", "--no-edit"); err != nil {
			return err
		}
	}
	return run("push", "origin", "main")
}

// nodeConfig loads the hdf config for a node.
func nodeConfig(t *testing.T, node Node) *config.Config {
	t.Helper()
	cfg, err := loadNodeConfig(node)
	if err != nil {
		t.Fatalf("nodeConfig: %v", err)
	}
	return cfg
}

// loadNodeConfig is nodeConfig without a *testing.T, for callers (the BDD
// steps) that report failures as errors.
func loadNodeConfig(node Node) (*config.Config, error) {
	return config.Load(filepath.Join(node.home, ".config", "hdf", "config.toml"))
}

// assertFileState derives and asserts the state of tildeFile on node.
// It inspects the registry on origin/main, file content on origin/main, and
// file content on the machine branch. It does NOT check on-disk symlinks —
// TestRunLinkSymlinksNewlyAcceptedFile covers linking separately.
func assertFileState(t *testing.T, node Node, tildeFile string, want FileState) {
	t.Helper()
	got := deriveFileState(t, node, tildeFile)
	if got != want {
		t.Errorf("assertFileState(%s): got %s, want %s", tildeFile, got, want)
	}
}

func deriveFileState(t *testing.T, node Node, tildeFile string) FileState {
	t.Helper()
	state, err := fileStateOf(node, tildeFile)
	if err != nil {
		t.Fatalf("deriveFileState: %v", err)
	}
	return state
}

// fileStateOf is deriveFileState without a *testing.T: it derives the state of
// tildeFile on node from the registry and blobs on origin/main (as this node
// last fetched it) and the machine branch.
func fileStateOf(node Node, tildeFile string) (FileState, error) {
	return fileStateAgainst(node, tildeFile, nil)
}

// fileStateAgainst derives tildeFile's state on node, reading main from
// remote when it is non-nil (the shared bare repo, i.e. the truth on the
// remote) and from node's origin/main tracking ref otherwise. Reading the bare
// repo lets an observer's state be checked without fetching on its behalf.
func fileStateAgainst(node Node, tildeFile string, remote *repo.Repo) (FileState, error) {
	mv, err := loadMachineView(node)
	if err != nil {
		return 0, err
	}
	readMain := func(rel string) ([]byte, error) {
		return mv.local.ReadFileFromRemoteBranch("origin", "main", rel)
	}
	if remote != nil {
		readMain = func(rel string) ([]byte, error) { return remote.ReadFileFromBranch("main", rel) }
	}
	return mv.stateOf(newMainView(readMain), tildeFile)
}

// mainView is main's registry, read once, and a way to read main's files.
type mainView struct {
	reg  *config.Registry // nil when main has none
	read func(rel string) ([]byte, error)
}

func newMainView(read func(rel string) ([]byte, error)) mainView {
	regBytes, _ := read(".hdf/managed.toml")
	reg, _ := config.RegistryFromBytes(regBytes)
	return mainView{reg: reg, read: read}
}

// machineView is what deriving states needs from one machine, loaded once.
type machineView struct {
	home  string
	cfg   *config.Config
	local *repo.Repo
}

func loadMachineView(node Node) (machineView, error) {
	cfg, err := loadNodeConfig(node)
	if err != nil {
		return machineView{}, fmt.Errorf("load config: %w", err)
	}
	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return machineView{}, fmt.Errorf("open repo: %w", err)
	}
	return machineView{home: node.home, cfg: cfg, local: r}, nil
}

// stateOf derives tildeFile's state on mv's machine against main.
func (mv machineView) stateOf(main mainView, tildeFile string) (FileState, error) {
	cfg := mv.cfg
	var registeredFile *config.ManagedFile
	if main.reg != nil {
		for i, f := range main.reg.Files {
			if f.Path == tildeFile {
				registeredFile = &main.reg.Files[i]
				break
			}
		}
	}
	if registeredFile == nil {
		return Untracked, nil
	}

	// Derive the repo-relative path, using variant-aware resolution so that
	// variant files are checked at their branch-specific path, not the canonical one.
	expanded := config.ExpandPathIn(tildeFile, mv.home)
	var repoPath string
	var err error
	if len(registeredFile.Variants) > 0 {
		repoPath = variantRepoPathFor(*registeredFile, cfg.Branch, cfg.LocalDotfilesDir)
		if repoPath == "" {
			return Untracked, nil
		}
	} else {
		repoPath, err = link.RepoPathForHome(expanded, cfg.LocalDotfilesDir, mv.home)
		if err != nil {
			return 0, fmt.Errorf("RepoPathForHome: %w", err)
		}
	}
	rel, err := filepath.Rel(cfg.LocalDotfilesDir, repoPath)
	if err != nil {
		return 0, fmt.Errorf("Rel: %w", err)
	}
	relSlash := filepath.ToSlash(rel)

	mainBytes, _ := main.read(relSlash)
	branchBytes, _ := mv.local.ReadFileFromBranch(cfg.Branch, relSlash)

	// Use nil checks (not len) so that legitimately empty files are not treated
	// as absent — mirrors the fix applied to hasUnreviewedPromotions.
	switch {
	case branchBytes == nil && mainBytes == nil:
		return RegistryOnly, nil
	case branchBytes == nil && mainBytes != nil:
		return Promoted, nil
	case branchBytes != nil && mainBytes == nil:
		return Enrolled, nil
	case string(branchBytes) == string(mainBytes):
		return Synced, nil
	default:
		return Diverged, nil
	}
}

// variantRepoPathFor resolves a variant file's repo path for the given
// branch, mirroring the CLI's internal variant resolution. Returns "" when
// the file has no variant for this branch.
func variantRepoPathFor(f config.ManagedFile, branch, localDotfilesDir string) string {
	for _, v := range f.Variants {
		if v.Branch == branch {
			return filepath.Join(localDotfilesDir, v.RepoPath)
		}
	}
	return ""
}
