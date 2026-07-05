//go:build e2e

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestThreeMachineConvergence exercises three-node scenarios: sequential
// promotions, Guard 2 blocking and then clearing, full three-way convergence,
// and a chain where accepting unlocks the next promotion.
func TestThreeMachineConvergence(t *testing.T) {
	t.Parallel()

	// Scenario 1: A promotes .bashrc, then B promotes .vimrc (after clearing Guard 2),
	// then C accepts both. All three end up with both files Synced.
	t.Run("sequential-promote", func(t *testing.T) {
		t.Parallel()
		nodes, _ := setupCluster(t, 3)
		a, b, c := nodes[0], nodes[1], nodes[2]

		// A enrolls and promotes .bashrc.
		bashrcA := filepath.Join(a.home, ".bashrc")
		if err := os.WriteFile(bashrcA, []byte("a-bashrc\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runHDFNode(t, a, "", "changes-push", "--yes", bashrcA); code != 0 {
			t.Fatalf("A changes-push .bashrc: %s", stderr)
		}
		hdfPromote(t, a)
		assertFileState(t, a, "~/.bashrc", Synced)

		// B enrolls .vimrc. Guard 2 must fire because A's .bashrc is on main.
		vimrcB := filepath.Join(b.home, ".vimrc")
		if err := os.WriteFile(vimrcB, []byte("set number\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runHDFNode(t, b, "", "changes-push", "--yes", vimrcB); code != 0 {
			t.Fatalf("B changes-push .vimrc: %s", stderr)
		}
		_, stderr, code := runHDFNode(t, b, "", "promote")
		if code == 0 {
			t.Fatal("B promote should be blocked by Guard 2 (A's .bashrc unreviewed)")
		}
		if !containsAny(stderr, "unreviewed", "changes-pull", "cannot promote") {
			t.Errorf("Guard 2 message missing in stderr: %q", stderr)
		}

		// B accepts A's .bashrc, then promotes .vimrc successfully.
		if _, stderr, code := runHDFNode(t, b, "y\n", "changes-pull"); code != 0 {
			t.Fatalf("B changes-pull: %s", stderr)
		}
		hdfPromote(t, b)
		assertFileState(t, b, "~/.vimrc", Synced)

		// C accepts both files via two changes-pull rounds.
		if _, stderr, code := runHDFNode(t, c, "y\n", "changes-pull"); code != 0 {
			t.Fatalf("C changes-pull (1): %s", stderr)
		}
		if _, stderr, code := runHDFNode(t, c, "y\n", "changes-pull"); code != 0 {
			t.Fatalf("C changes-pull (2): %s", stderr)
		}
		assertFileState(t, c, "~/.bashrc", Synced)
		assertFileState(t, c, "~/.vimrc", Synced)
	})

	// Scenario 2: Guard 2 fires, then B reviews and retries — verifying the guard
	// does not permanently block and the state machine recovers normally.
	t.Run("guard-fires-then-clears", func(t *testing.T) {
		t.Parallel()
		nodes, _ := setupCluster(t, 3)
		a, b, _ := nodes[0], nodes[1], nodes[2]

		// A promotes .bashrc.
		bashrcA := filepath.Join(a.home, ".bashrc")
		if err := os.WriteFile(bashrcA, []byte("a-content\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runHDFNode(t, a, "", "changes-push", "--yes", bashrcA); code != 0 {
			t.Fatalf("A changes-push: %s", stderr)
		}
		hdfPromote(t, a)

		// B enrolls .vimrc and tries to promote — Guard 2 fires.
		vimrcB := filepath.Join(b.home, ".vimrc")
		if err := os.WriteFile(vimrcB, []byte("set wrap\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runHDFNode(t, b, "", "changes-push", "--yes", vimrcB); code != 0 {
			t.Fatalf("B changes-push: %s", stderr)
		}
		_, _, code := runHDFNode(t, b, "", "promote")
		if code == 0 {
			t.Fatal("B promote should be blocked: A's .bashrc is unreviewed")
		}

		// B clears Guard 2 by accepting A's .bashrc.
		if _, stderr, code := runHDFNode(t, b, "y\n", "changes-pull"); code != 0 {
			t.Fatalf("B changes-pull accept: %s", stderr)
		}
		assertFileState(t, b, "~/.bashrc", Synced)

		// B can now promote its .vimrc without interference.
		hdfPromote(t, b)
		assertFileState(t, b, "~/.vimrc", Synced)
	})

	// Scenario 3: Three machines each promote a unique file; all three accept
	// the others' files and reach full Synced state across the board.
	t.Run("full-three-way-convergence", func(t *testing.T) {
		t.Parallel()
		nodes, _ := setupCluster(t, 3)
		a, b, c := nodes[0], nodes[1], nodes[2]

		// A promotes .bashrc.
		f := filepath.Join(a.home, ".bashrc")
		if err := os.WriteFile(f, []byte("a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runHDFNode(t, a, "", "changes-push", "--yes", f); code != 0 {
			t.Fatalf("A changes-push .bashrc: %s", stderr)
		}
		hdfPromote(t, a)

		// B accepts A's .bashrc, then promotes .vimrc.
		if _, stderr, code := runHDFNode(t, b, "y\n", "changes-pull"); code != 0 {
			t.Fatalf("B accept A .bashrc: %s", stderr)
		}
		f = filepath.Join(b.home, ".vimrc")
		if err := os.WriteFile(f, []byte("b\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runHDFNode(t, b, "", "changes-push", "--yes", f); code != 0 {
			t.Fatalf("B changes-push .vimrc: %s", stderr)
		}
		hdfPromote(t, b)

		// C accepts A's .bashrc and B's .vimrc, then promotes .profile.
		if _, stderr, code := runHDFNode(t, c, "y\n", "changes-pull"); code != 0 {
			t.Fatalf("C accept A .bashrc: %s", stderr)
		}
		if _, stderr, code := runHDFNode(t, c, "y\n", "changes-pull"); code != 0 {
			t.Fatalf("C accept B .vimrc: %s", stderr)
		}
		f = filepath.Join(c.home, ".profile")
		if err := os.WriteFile(f, []byte("c\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runHDFNode(t, c, "", "changes-push", "--yes", f); code != 0 {
			t.Fatalf("C changes-push .profile: %s", stderr)
		}
		hdfPromote(t, c)

		// A accepts B's .vimrc and C's .profile.
		if _, stderr, code := runHDFNode(t, a, "y\n", "changes-pull"); code != 0 {
			t.Fatalf("A accept B .vimrc: %s", stderr)
		}
		if _, stderr, code := runHDFNode(t, a, "y\n", "changes-pull"); code != 0 {
			t.Fatalf("A accept C .profile: %s", stderr)
		}

		// B accepts C's .profile.
		if _, stderr, code := runHDFNode(t, b, "y\n", "changes-pull"); code != 0 {
			t.Fatalf("B accept C .profile: %s", stderr)
		}

		// All three machines now see all three files as Synced.
		for i, node := range []Node{a, b, c} {
			assertFileState(t, node, "~/.bashrc", Synced)
			assertFileState(t, node, "~/.vimrc", Synced)
			assertFileState(t, node, "~/.profile", Synced)
			_ = i
		}
	})

	// Scenario 4: A promotes v1, B and C accept (Synced). A then promotes v2.
	// B accepts v2 (Synced). C skips (Diverged). C then accepts (Synced).
	// Verifies that diverged nodes eventually converge.
	t.Run("delayed-convergence", func(t *testing.T) {
		t.Parallel()
		nodes, _ := setupCluster(t, 3)
		a, b, c := nodes[0], nodes[1], nodes[2]

		bashrcA := filepath.Join(a.home, ".bashrc")

		// Round 1: A promotes v1; B and C accept.
		if err := os.WriteFile(bashrcA, []byte("v1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runHDFNode(t, a, "", "changes-push", "--yes", bashrcA); code != 0 {
			t.Fatalf("A changes-push v1: %s", stderr)
		}
		hdfPromote(t, a)
		if _, stderr, code := runHDFNode(t, b, "y\n", "changes-pull"); code != 0 {
			t.Fatalf("B accept v1: %s", stderr)
		}
		if _, stderr, code := runHDFNode(t, c, "y\n", "changes-pull"); code != 0 {
			t.Fatalf("C accept v1: %s", stderr)
		}
		assertFileState(t, b, "~/.bashrc", Synced)
		assertFileState(t, c, "~/.bashrc", Synced)

		// Round 2: A promotes v2.
		if err := os.WriteFile(bashrcA, []byte("v2\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runHDFNode(t, a, "", "changes-push", "--yes", bashrcA); code != 0 {
			t.Fatalf("A changes-push v2: %s", stderr)
		}
		hdfPromote(t, a)
		assertFileState(t, a, "~/.bashrc", Synced)

		// B accepts v2 promptly → Synced.
		if _, stderr, code := runHDFNode(t, b, "y\n", "changes-pull"); code != 0 {
			t.Fatalf("B accept v2: %s", stderr)
		}
		assertFileState(t, b, "~/.bashrc", Synced)

		// C skips v2 → Diverged (C still has v1 on its branch, main has v2).
		if _, stderr, code := runHDFNode(t, c, "n\n", "changes-pull"); code != 0 {
			t.Fatalf("C skip v2: %s", stderr)
		}
		assertFileState(t, c, "~/.bashrc", Diverged)

		// C later accepts → Synced. Convergence achieved.
		if _, stderr, code := runHDFNode(t, c, "y\n", "changes-pull"); code != 0 {
			t.Fatalf("C accept v2 (delayed): %s", stderr)
		}
		assertFileState(t, c, "~/.bashrc", Synced)
	})
}
