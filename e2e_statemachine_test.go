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
	inRegistry := false
	if reg != nil {
		for _, f := range reg.Files {
			if f.Path == tildeFile {
				inRegistry = true
				break
			}
		}
	}
	if !inRegistry {
		return Untracked
	}

	// Derive the repo-relative path for this file.
	expanded := config.ExpandPathIn(tildeFile, node.home)
	repoPath, err := link.RepoPathForHome(expanded, cfg.LocalDotfilesDir, node.home)
	if err != nil {
		t.Fatalf("deriveFileState: RepoPathForHome: %v", err)
	}
	rel, err := filepath.Rel(cfg.LocalDotfilesDir, repoPath)
	if err != nil {
		t.Fatalf("deriveFileState: Rel: %v", err)
	}
	relSlash := filepath.ToSlash(rel)

	mainBytes, _ := r.ReadFileFromRemoteBranch("origin", "main", relSlash)
	branchBytes, _ := r.ReadFileFromBranch(cfg.Branch, relSlash)
	fi, lerr := os.Lstat(expanded)
	symlinkExists := lerr == nil && fi.Mode()&os.ModeSymlink != 0
	_ = symlinkExists

	switch {
	case len(branchBytes) == 0 && len(mainBytes) == 0:
		return RegistryOnly
	case len(branchBytes) == 0 && len(mainBytes) > 0:
		return Promoted
	case len(branchBytes) > 0 && len(mainBytes) == 0:
		return Enrolled
	case string(branchBytes) == string(mainBytes):
		return Synced
	default:
		return Diverged
	}
}
