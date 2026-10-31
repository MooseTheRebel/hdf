//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// remoteWithMain makes a bare remote whose main holds files (repo path →
// content), for checking the "remote main" steps without running hdf.
func remoteWithMain(t *testing.T, files map[string]string) string {
	t.Helper()
	work, bare := t.TempDir(), filepath.Join(t.TempDir(), "remote.git")
	git := func(dir string, args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir,
			"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git(work, "init", "-q", "-b", "main")
	for rel, content := range files {
		p := filepath.Join(work, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(work, "add", "-A")
	git(work, "commit", "-q", "-m", "main")
	git(work, "clone", "-q", "--bare", work, bare)
	return bare
}

// worldWithoutConfig is a world whose only machine has no hdf config, as after
// reinstalling it while offline.
func worldWithoutConfig(t *testing.T, bare string) *world {
	m := &machine{name: "gone", node: Node{home: t.TempDir(), branch: "gone"}}
	return &world{bareDir: bare, machines: map[string]*machine{"gone": m}, order: []string{"gone"}}
}

func TestRemoteFileNeedsNoMachineConfig(t *testing.T) {
	bare := remoteWithMain(t, map[string]string{
		".hdf/managed.toml": "[[files]]\npath = \"~/.bashrc\"\nhash = \"x\"\n",
		".bashrc":           "v1\n",
	})
	b, err := remoteFile(worldWithoutConfig(t, bare), "~/.bashrc")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "v1\n" {
		t.Errorf("read %q, want %q", b, "v1\n")
	}
}

// A file with per-machine variants has no single content on main, so asking
// what main has must fail rather than read the (empty) canonical path and
// claim main doesn't have it.
func TestRemoteFileRefusesVariantFiles(t *testing.T) {
	bare := remoteWithMain(t, map[string]string{
		".hdf/managed.toml": "[[files]]\npath = \"~/.bashrc\"\n" +
			"[[files.variants]]\nbranch = \"web1\"\nrepo_path = \".bashrc.web1\"\nhash = \"x\"\n",
		".bashrc.web1": "server\n",
	})
	if b, err := remoteFile(worldWithoutConfig(t, bare), "~/.bashrc"); err == nil {
		t.Errorf("read %q, want an error about variants", b)
	}
}
