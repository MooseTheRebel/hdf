//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
