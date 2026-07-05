//go:build e2e

package main

import (
	"os"
	"path/filepath"
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
