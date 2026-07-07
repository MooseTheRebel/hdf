//go:build e2e

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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

// setupCluster creates n nodes all sharing one bare repo.
// Node 0 uses local init (creates the bare repo); nodes 1+ clone from it.
// Each node gets a unique branch name "node-N" via HDF_BRANCH.
// Returns the nodes and the file:// URL of the shared bare repo.
func setupCluster(t *testing.T, n int) ([]Node, string) {
	t.Helper()
	bareDir := t.TempDir()
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
func prMerge(t *testing.T, node Node, bareURL string) {
	t.Helper()
	cfgPath := filepath.Join(node.home, ".config", "hdf", "config.toml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("prMerge: load config: %v", err)
	}

	tmpDir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("prMerge git %v: %v\n%s", args, err, out)
		}
	}
	run("clone", bareURL, ".")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	// -X theirs auto-resolves conflicts in favour of the machine branch, which
	// matches the assumption that the machine branch content is authoritative when
	// a PR is accepted. Real GitHub merges don't use -X theirs, so this won't catch
	// bugs where the machine branch accidentally clobbers unrelated main content.
	run("merge", "--no-ff", "-X", "theirs", "origin/"+cfg.Branch, "-m", "Merge "+cfg.Branch+" into main")
	run("push", "origin", "main")
}

// nodeConfig loads the hdf config for a node.
func nodeConfig(t *testing.T, node Node) *config.Config {
	t.Helper()
	cfg, err := config.Load(filepath.Join(node.home, ".config", "hdf", "config.toml"))
	if err != nil {
		t.Fatalf("nodeConfig: %v", err)
	}
	return cfg
}

// assertFileState derives and asserts the state of tildeFile on node.
// It inspects the registry on origin/main, file content on origin/main,
// file content on the machine branch, and whether a symlink exists on disk.
func assertFileState(t *testing.T, node Node, tildeFile string, want FileState) {
	t.Helper()
	got := deriveFileState(t, node, tildeFile)
	if got != want {
		t.Errorf("assertFileState(%s): got %s, want %s", tildeFile, got, want)
	}
}

func deriveFileState(t *testing.T, node Node, tildeFile string) FileState {
	t.Helper()
	cfg := nodeConfig(t, node)

	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		t.Fatalf("deriveFileState: open repo: %v", err)
	}

	// Check registry on origin/main.
	regBytes, _ := r.ReadFileFromRemoteBranch("origin", "main", ".hdf/managed.toml")
	reg, _ := config.RegistryFromBytes(regBytes)
	var registeredFile *config.ManagedFile
	if reg != nil {
		for i, f := range reg.Files {
			if f.Path == tildeFile {
				registeredFile = &reg.Files[i]
				break
			}
		}
	}
	if registeredFile == nil {
		return Untracked
	}

	// Derive the repo-relative path, using variant-aware resolution so that
	// variant files are checked at their branch-specific path, not the canonical one.
	expanded := config.ExpandPathIn(tildeFile, node.home)
	var repoPath string
	if len(registeredFile.Variants) > 0 {
		repoPath, err = resolveRepoPath(*registeredFile, cfg.Branch, cfg.LocalDotfilesDir, expanded)
		if err != nil || repoPath == "" {
			return Untracked
		}
	} else {
		repoPath, err = link.RepoPathForHome(expanded, cfg.LocalDotfilesDir, node.home)
		if err != nil {
			t.Fatalf("deriveFileState: RepoPathForHome: %v", err)
		}
	}
	rel, err := filepath.Rel(cfg.LocalDotfilesDir, repoPath)
	if err != nil {
		t.Fatalf("deriveFileState: Rel: %v", err)
	}
	relSlash := filepath.ToSlash(rel)

	mainBytes, _ := r.ReadFileFromRemoteBranch("origin", "main", relSlash)
	branchBytes, _ := r.ReadFileFromBranch(cfg.Branch, relSlash)

	// Use nil checks (not len) so that legitimately empty files are not treated
	// as absent — mirrors the fix applied to hasUnreviewedPromotions.
	switch {
	case branchBytes == nil && mainBytes == nil:
		return RegistryOnly
	case branchBytes == nil && mainBytes != nil:
		return Promoted
	case branchBytes != nil && mainBytes == nil:
		return Enrolled
	case string(branchBytes) == string(mainBytes):
		return Synced
	default:
		return Diverged
	}
}
