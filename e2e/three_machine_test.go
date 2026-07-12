//go:build e2e

package e2e

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

		// C accepts both pending files in one changes-pull call (one "y" per file).
		if _, stderr, code := runHDFNode(t, c, "y\ny\n", "changes-pull"); code != 0 {
			t.Fatalf("C changes-pull: %s", stderr)
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

		// C accepts both .bashrc (A) and .vimrc (B) in one changes-pull call.
		if _, stderr, code := runHDFNode(t, c, "y\ny\n", "changes-pull"); code != 0 {
			t.Fatalf("C accept .bashrc+.vimrc: %s", stderr)
		}
		f = filepath.Join(c.home, ".profile")
		if err := os.WriteFile(f, []byte("c\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, stderr, code := runHDFNode(t, c, "", "changes-push", "--yes", f); code != 0 {
			t.Fatalf("C changes-push .profile: %s", stderr)
		}
		hdfPromote(t, c)

		// A accepts B's .vimrc and C's .profile in one changes-pull call.
		if _, stderr, code := runHDFNode(t, a, "y\ny\n", "changes-pull"); code != 0 {
			t.Fatalf("A accept .vimrc+.profile: %s", stderr)
		}

		// B accepts C's .profile.
		if _, stderr, code := runHDFNode(t, b, "y\n", "changes-pull"); code != 0 {
			t.Fatalf("B accept C .profile: %s", stderr)
		}

		// All three machines now see all three files as Synced.
		for _, node := range []Node{a, b, c} {
			assertFileState(t, node, "~/.bashrc", Synced)
			assertFileState(t, node, "~/.vimrc", Synced)
			assertFileState(t, node, "~/.profile", Synced)
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

// TestStaleNodePromoteCannotRevert is the multi-node lost-update guard: after
// A promotes v2, a node still holding v1 must not be able to silently revert
// main by promoting unrelated work. Non-interactively promote refuses; with an
// explicit "n" it proceeds while keeping main's newer version.
func TestStaleNodePromoteCannotRevert(t *testing.T) {
	t.Parallel()
	nodes, _ := setupCluster(t, 3)
	a, b, c := nodes[0], nodes[1], nodes[2]

	// Round 1: A promotes v1; B and C accept — everyone Synced on v1.
	bashrcA := filepath.Join(a.home, ".bashrc")
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

	// Round 2: A promotes v2. B does NOT pull.
	if err := os.WriteFile(bashrcA, []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runHDFNode(t, a, "", "changes-push", "--yes", bashrcA); code != 0 {
		t.Fatalf("A changes-push v2: %s", stderr)
	}
	hdfPromote(t, a)

	// B (stale: still on v1) enrolls unrelated work.
	vimrcB := filepath.Join(b.home, ".vimrc")
	if err := os.WriteFile(vimrcB, []byte("set number\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runHDFNode(t, b, "", "changes-push", "--yes", vimrcB); code != 0 {
		t.Fatalf("B changes-push .vimrc: %s", stderr)
	}

	// Non-interactive promote must refuse — B has never seen v2.
	_, stderr, code := runHDFNode(t, b, "", "promote")
	if code == 0 {
		t.Fatal("B promote should refuse non-interactively while holding stale v1")
	}
	if !containsAny(stderr, "haven't reviewed", "changes-pull") {
		t.Errorf("expected review-guard message in stderr, got: %q", stderr)
	}

	// With an explicit "n" (keep main's newer version) the promote proceeds.
	_, stderr, code = runHDFNode(t, b, "n\n", "promote")
	if code != 0 {
		t.Fatalf("B promote with decline should succeed: %s", stderr)
	}

	// A must still be Synced on v2 — B's promote did not revert main.
	assertFileStateAfterFetch(t, a, "~/.bashrc", Synced)
	// B's .vimrc landed on main.
	assertFileState(t, b, "~/.vimrc", Synced)
	// B itself remains Diverged on .bashrc (it kept v1 locally).
	assertFileState(t, b, "~/.bashrc", Diverged)
	// C sees B's .vimrc as an incoming promote.
	assertFileStateAfterFetch(t, c, "~/.vimrc", Promoted)
}
