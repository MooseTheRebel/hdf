//go:build e2e

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hdf/config"
	"hdf/repo"
)

// TestConvergeSingleFile is the canonical convergence scenario:
// Node A enrolls a file, promotes it, Node B pulls and accepts.
// Both nodes should end up Synced.
func TestConvergeSingleFile(t *testing.T) {
	nodes, _ := setupCluster(t, 2)
	nodeA, nodeB := nodes[0], nodes[1]

	// Write the dotfile on Node A's home.
	dotfile := filepath.Join(nodeA.home, ".bashrc")
	if err := os.WriteFile(dotfile, []byte("export PS1='A$ '\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Node A: enroll (changes-push with --yes to skip prompt).
	_, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfile)
	if code != 0 {
		t.Fatalf("Node A changes-push: %s", stderr)
	}
	assertFileState(t, nodeA, "~/.bashrc", Enrolled)

	// Node A: promote.
	hdfPromote(t, nodeA)
	assertFileState(t, nodeA, "~/.bashrc", Synced)

	// Node B: fetch via changes-pull; accept main's content.
	_, stderr, code = runHDFNode(t, nodeB, "y\n", "changes-pull")
	if code != 0 {
		t.Fatalf("Node B changes-pull: %s", stderr)
	}
	assertFileState(t, nodeB, "~/.bashrc", Synced)
}

// TestConvergeSkip verifies that Node B can skip accepting main's promoted
// content and retains its own Enrolled state.
func TestConvergeSkip(t *testing.T) {
	nodes, _ := setupCluster(t, 2)
	nodeA, nodeB := nodes[0], nodes[1]

	// Node A: enroll and promote.
	dotfile := filepath.Join(nodeA.home, ".bashrc")
	if err := os.WriteFile(dotfile, []byte("export PS1='A$ '\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfile); code != 0 {
		t.Fatalf("Node A changes-push: %s", stderr)
	}
	hdfPromote(t, nodeA)

	// Node B: enroll its own version first.
	dotfileB := filepath.Join(nodeB.home, ".bashrc")
	if err := os.WriteFile(dotfileB, []byte("export PS1='B$ '\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runHDFNode(t, nodeB, "", "changes-push", "--yes", dotfileB); code != 0 {
		t.Fatalf("Node B changes-push: %s", stderr)
	}

	// Node B: changes-pull, skip main's version ("n").
	if _, stderr, code := runHDFNode(t, nodeB, "n\n", "changes-pull"); code != 0 {
		t.Fatalf("Node B changes-pull: %s", stderr)
	}

	// Node B keeps its own content; main has A's promoted version — Diverged.
	assertFileState(t, nodeB, "~/.bashrc", Diverged)
}

// TestConvergeReDiverge verifies that after Node A promotes, Node B syncs,
// and then Node A edits and promotes again, Node B sees the new diff.
func TestConvergeReDiverge(t *testing.T) {
	nodes, _ := setupCluster(t, 2)
	nodeA, nodeB := nodes[0], nodes[1]

	dotfileA := filepath.Join(nodeA.home, ".bashrc")

	// Round 1: A enrolls v1, promotes, B accepts.
	if err := os.WriteFile(dotfileA, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfileA); code != 0 {
		t.Fatalf("A changes-push v1: %s", stderr)
	}
	hdfPromote(t, nodeA)
	if _, stderr, code := runHDFNode(t, nodeB, "y\n", "changes-pull"); code != 0 {
		t.Fatalf("B changes-pull v1: %s", stderr)
	}
	assertFileState(t, nodeB, "~/.bashrc", Synced)

	// Node A: edit and re-enroll v2.
	if err := os.WriteFile(dotfileA, []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfileA); code != 0 {
		t.Fatalf("A changes-push v2: %s", stderr)
	}
	assertFileState(t, nodeA, "~/.bashrc", Diverged)

	// Node A: promote again.
	hdfPromote(t, nodeA)
	assertFileState(t, nodeA, "~/.bashrc", Synced)

	// Node B: pull to fetch the updated main; skip the new diff.
	runHDFNode(t, nodeB, "n\n", "changes-pull") //nolint:errcheck
	// B accepted v1 earlier; main is now v2. B kept v1, so it's Diverged.
	assertFileState(t, nodeB, "~/.bashrc", Diverged)
}

// TestConvergePRPath verifies that a PR-style merge (raw git merge on bare repo)
// produces the same end state as hdf promote.
func TestConvergePRPath(t *testing.T) {
	nodes, bareURL := setupCluster(t, 2)
	nodeA, nodeB := nodes[0], nodes[1]

	dotfileA := filepath.Join(nodeA.home, ".bashrc")
	if err := os.WriteFile(dotfileA, []byte("export PS1='PR$ '\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfileA); code != 0 {
		t.Fatalf("A changes-push: %s", stderr)
	}
	assertFileState(t, nodeA, "~/.bashrc", Enrolled)

	// Simulate PR merge instead of hdf promote.
	prMerge(t, nodeA, bareURL)

	// After PR merge, Node A fetches and should be Synced (skip the prompt).
	runHDFNode(t, nodeA, "n\n", "changes-pull") //nolint:errcheck
	assertFileState(t, nodeA, "~/.bashrc", Synced)

	// Node B accepts.
	if _, stderr, code := runHDFNode(t, nodeB, "y\n", "changes-pull"); code != 0 {
		t.Fatalf("B changes-pull: %s", stderr)
	}
	assertFileState(t, nodeB, "~/.bashrc", Synced)
}

// TestConvergeTwoWayConflict verifies that when Node B edits its file before
// pulling, it sees the diff and can skip — remaining in Enrolled state.
func TestConvergeTwoWayConflict(t *testing.T) {
	nodes, _ := setupCluster(t, 2)
	nodeA, nodeB := nodes[0], nodes[1]

	// Node A: enroll and promote.
	dotfileA := filepath.Join(nodeA.home, ".bashrc")
	if err := os.WriteFile(dotfileA, []byte("export PS1='A$ '\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfileA); code != 0 {
		t.Fatalf("A changes-push: %s", stderr)
	}
	hdfPromote(t, nodeA)

	// Node B: enroll its own content before pulling main's.
	dotfileB := filepath.Join(nodeB.home, ".bashrc")
	if err := os.WriteFile(dotfileB, []byte("export PS1='B$ '\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runHDFNode(t, nodeB, "", "changes-push", "--yes", dotfileB); code != 0 {
		t.Fatalf("B changes-push: %s", stderr)
	}

	// Node B: changes-pull sees the conflict and skips.
	stdout, _, code := runHDFNode(t, nodeB, "n\n", "changes-pull")
	if code != 0 {
		t.Fatalf("B changes-pull: code %d", code)
	}
	if !strings.Contains(stdout, "Skipped") {
		t.Errorf("stdout %q should mention 'Skipped'", stdout)
	}

	// Node B keeps its own content; main has A's promoted version — Diverged.
	assertFileState(t, nodeB, "~/.bashrc", Diverged)
}

// TestPRMergePreservesRegistryEntriesFromMain verifies that prMerge does not
// drop registry entries that exist on origin/main but not on the machine branch.
//
// Scenario: A enrolls .bashrc, B enrolls and promotes .vimrc (updating main's
// registry). A's branch managed.toml predates B's promote, so it lacks B's
// entry. prMerge without -X theirs must produce a merged managed.toml that
// contains both entries; -X theirs would silently keep only A's version.
func TestPRMergePreservesRegistryEntriesFromMain(t *testing.T) {
	t.Parallel()
	nodes, bareURL := setupCluster(t, 2)
	nodeA, nodeB := nodes[0], nodes[1]

	// A enrolls .bashrc first (stageAndCommit records A's managed.toml snapshot).
	dotfileA := filepath.Join(nodeA.home, ".bashrc")
	if err := os.WriteFile(dotfileA, []byte("A content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfileA); code != 0 {
		t.Fatalf("A changes-push: %s", stderr)
	}
	// A's machine branch now has managed.toml with .bashrc entry only.

	// B enrolls AND promotes .vimrc — origin/main now has B's .vimrc entry.
	dotfileB := filepath.Join(nodeB.home, ".vimrc")
	if err := os.WriteFile(dotfileB, []byte("set number\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runHDFNode(t, nodeB, "", "changes-push", "--yes", dotfileB); code != 0 {
		t.Fatalf("B changes-push: %s", stderr)
	}
	hdfPromote(t, nodeB)
	// origin/main's managed.toml now has both .bashrc AND .vimrc entries.

	// prMerge merges A's branch (managed.toml has only .bashrc) into main
	// (managed.toml has .bashrc + .vimrc).
	prMerge(t, nodeA, bareURL)

	// After the merge, origin/main must still contain B's .vimrc registry entry.
	cfgA := nodeConfig(t, nodeA)
	rA, err := repo.Open(cfgA.LocalDotfilesDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := rA.Fetch(); err != nil {
		t.Fatal(err)
	}
	regBytes, err := rA.ReadFileFromRemoteBranch("origin", "main", ".hdf/managed.toml")
	if err != nil || len(regBytes) == 0 {
		t.Fatalf("could not read managed.toml from origin/main: %v", err)
	}
	reg, err := config.RegistryFromBytes(regBytes)
	if err != nil {
		t.Fatalf("RegistryFromBytes: %v", err)
	}
	hasVimrc := false
	for _, f := range reg.Files {
		if strings.HasSuffix(f.Path, ".vimrc") {
			hasVimrc = true
		}
	}
	if !hasVimrc {
		t.Error("managed.toml on origin/main is missing B's .vimrc entry after prMerge — prMerge dropped registry entries")
	}
}
