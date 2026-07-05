//go:build e2e

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// assertFileStateAfterFetch fetches on node then asserts the file state.
// Used when another node has promoted and the local repo needs to be updated
// before the state is visible.
func assertFileStateAfterFetch(t *testing.T, node Node, tildeFile string, want FileState) {
	t.Helper()
	runHDFNode(t, node, "", "changes-pull") //nolint:errcheck // fetch; output ignored
	assertFileState(t, node, tildeFile, want)
}

// setupLocalOnlyNode inits a single node with no remote (option 1, empty push target).
func setupLocalOnlyNode(t *testing.T) Node {
	t.Helper()
	home := t.TempDir()
	workDir := filepath.Join(t.TempDir(), "repo")
	node := Node{home: home, branch: "local-only"}
	// option 1: local repo path, no push target
	stdin := "1\n" + workDir + "\n\n"
	_, stderr, code := runHDFNode(t, node, stdin, "init")
	if code != 0 {
		t.Fatalf("setupLocalOnlyNode: init failed (code %d): %s", code, stderr)
	}
	return node
}

// TestPromoteGuards verifies Guard 1 (no remote) and Guard 2 (unreviewed incoming)
// are enforced at the e2e level. Guard 3 (race on push) is exercised by the
// unit test TestPromoteRefusesOnConcurrentPush; the race window is too narrow
// to reliably reproduce in an e2e test without process coordination.
func TestPromoteGuards(t *testing.T) {
	t.Parallel()

	t.Run("no-remote", func(t *testing.T) {
		t.Parallel()
		node := setupLocalOnlyNode(t)
		dotfile := filepath.Join(node.home, ".bashrc")
		if err := os.WriteFile(dotfile, []byte("local\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runHDFNode(t, node, "", "changes-push", "--yes", dotfile); code != 0 {
			t.Fatalf("changes-push: %s", stderr)
		}
		_, stderr, code := runHDFNode(t, node, "", "promote")
		if code == 0 {
			t.Fatal("promote should have failed for no-remote node, but it succeeded")
		}
		if !containsAny(stderr, "no remote", "cannot promote") {
			t.Errorf("expected 'no remote' or 'cannot promote' in stderr, got: %q", stderr)
		}
	})

	t.Run("incoming-unreviewed", func(t *testing.T) {
		t.Parallel()
		nodes, _ := setupCluster(t, 2)
		nodeA := nodes[0]
		nodeB := nodes[1]

		// A enrolls and promotes .bashrc.
		dotfileA := filepath.Join(nodeA.home, ".bashrc")
		if err := os.WriteFile(dotfileA, []byte("a-content\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfileA); code != 0 {
			t.Fatalf("A changes-push: %s", stderr)
		}
		hdfPromote(t, nodeA)

		// B enrolls its own .vimrc and tries to promote without changes-pull first.
		dotfileB := filepath.Join(nodeB.home, ".vimrc")
		if err := os.WriteFile(dotfileB, []byte("set number\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runHDFNode(t, nodeB, "", "changes-push", "--yes", dotfileB); code != 0 {
			t.Fatalf("B changes-push: %s", stderr)
		}
		_, stderr, code := runHDFNode(t, nodeB, "", "promote")
		if code == 0 {
			t.Fatal("promote should have refused: B has not reviewed A's promotion")
		}
		if !containsAny(stderr, "unreviewed", "changes-pull", "cannot promote") {
			t.Errorf("expected guard message in stderr, got: %q", stderr)
		}
	})
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// TestExternalTransitions verifies the "observer" transitions in validTransitions:
// states that arise on a machine when another machine promotes, without any
// local command being responsible.
//
//   - foreign-promote-same-file:     Untracked → Promoted (A promotes a file B never touched)
//   - foreign-update-diverges-synced: Synced   → Diverged (A re-promotes after B is synced)
func TestExternalTransitions(t *testing.T) {
	t.Parallel()

	t.Run("foreign-promote-same-file", func(t *testing.T) {
		t.Parallel()
		nodes, _ := setupCluster(t, 2)
		nodeA := nodes[0]
		nodeB := nodes[1]

		dotfileA := filepath.Join(nodeA.home, ".bashrc")
		if err := os.WriteFile(dotfileA, []byte("a-content\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfileA); code != 0 {
			t.Fatalf("A changes-push: %s", stderr)
		}
		hdfPromote(t, nodeA)

		// B never touched ~/.bashrc; after A promotes it, B sees Promoted.
		assertFileStateAfterFetch(t, nodeB, "~/.bashrc", Promoted)
	})

	t.Run("foreign-update-diverges-synced", func(t *testing.T) {
		t.Parallel()
		nodes, _ := setupCluster(t, 2)
		nodeA := nodes[0]
		nodeB := nodes[1]

		// Both nodes enroll and promote .bashrc so they start Synced.
		dotfileA := filepath.Join(nodeA.home, ".bashrc")
		dotfileB := filepath.Join(nodeB.home, ".bashrc")

		if err := os.WriteFile(dotfileA, []byte("shared-v1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfileA); code != 0 {
			t.Fatalf("A changes-push v1: %s", stderr)
		}
		hdfPromote(t, nodeA)

		// B accepts A's v1 so it becomes Synced.
		runHDFNode(t, nodeB, "n\n", "changes-pull") //nolint:errcheck // seed fetch
		if _, stderr, code := runHDFNode(t, nodeB, "y\n", "changes-pull"); code != 0 {
			t.Fatalf("B accept v1: %s", stderr)
		}
		// B pushes its branch with v1 content so its branch matches main.
		if _, stderr, code := runHDFNode(t, nodeB, "", "changes-push", "--yes", dotfileB); code != 0 {
			t.Fatalf("B changes-push v1: %s", stderr)
		}
		assertFileState(t, nodeB, "~/.bashrc", Synced)

		// A now updates to v2 and re-promotes.
		if err := os.WriteFile(dotfileA, []byte("shared-v2\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runHDFNode(t, nodeA, "", "changes-push", "--yes", dotfileA); code != 0 {
			t.Fatalf("A changes-push v2: %s", stderr)
		}
		hdfPromote(t, nodeA)

		// B still has v1 on its branch; origin/main now has v2 → Diverged.
		assertFileStateAfterFetch(t, nodeB, "~/.bashrc", Diverged)
	})
}
