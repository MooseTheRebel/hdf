//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// hdf prints resolved paths, so a scenario's root must have no symlinks in it
// even when the temp dir is reached through one (macOS's /var → /private/var).
func TestNewWorldRootHasNoSymlinks(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "tmp")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", link)

	w, err := newWorld("s", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.cleanup() }()

	wantParent, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(w.root) != wantParent {
		t.Errorf("root %s is not directly under %s", w.root, wantParent)
	}
	if resolved, err := filepath.EvalSymlinks(w.root); err != nil || resolved != w.root {
		t.Errorf("root %s resolves to %s (%v)", w.root, resolved, err)
	}
}
