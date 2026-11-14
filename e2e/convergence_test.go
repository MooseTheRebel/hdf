//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hdf/config"
	"hdf/repo"
)

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
