package link

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeHome = "/home/testuser"

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")

	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	hash, err := HashFile(f)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}
	if !strings.HasPrefix(hash, "sha256:") {
		t.Errorf("hash %q should start with sha256:", hash)
	}

	// Deterministic: same content → same hash.
	hash2, _ := HashFile(f)
	if hash != hash2 {
		t.Error("hash not deterministic")
	}

	// Different content → different hash.
	if err := os.WriteFile(f, []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash3, _ := HashFile(f)
	if hash == hash3 {
		t.Error("different content should produce different hash")
	}
}

func TestRepoPathForHome(t *testing.T) {
	repoDir := t.TempDir()

	cases := []struct {
		homePath string
		wantRel  string
	}{
		{filepath.Join(fakeHome, ".bashrc"), ".bashrc"},
		{filepath.Join(fakeHome, ".config", "fish", "config.fish"), filepath.Join(".config", "fish", "config.fish")},
		{filepath.Join(fakeHome, ".config", "nvim", "init.lua"), filepath.Join(".config", "nvim", "init.lua")},
	}

	for _, c := range cases {
		got, err := repoPathForHome(c.homePath, repoDir, fakeHome)
		if err != nil {
			t.Fatalf("repoPathForHome(%q): %v", c.homePath, err)
		}
		want := filepath.Join(repoDir, c.wantRel)
		if got != want {
			t.Errorf("repoPathForHome(%q) = %q, want %q", c.homePath, got, want)
		}
	}
}

func TestRepoPathForOutsideHome(t *testing.T) {
	repoDir := t.TempDir()

	_, err := repoPathForHome("/etc/passwd", repoDir, fakeHome)
	if err == nil {
		t.Error("expected error for path outside home directory, got nil")
	}
}

func TestRepoPathForNoCollision(t *testing.T) {
	repoDir := t.TempDir()

	path1 := filepath.Join(fakeHome, ".config", "fish", "config.fish")
	path2 := filepath.Join(fakeHome, ".config", "nvim", "config.fish") // same basename

	dst1, err := repoPathForHome(path1, repoDir, fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	dst2, err := repoPathForHome(path2, repoDir, fakeHome)
	if err != nil {
		t.Fatal(err)
	}

	if dst1 == dst2 {
		t.Errorf("files with same base name from different dirs should get different repo paths, both got %q", dst1)
	}
}

func TestEnroll(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := t.TempDir()

	homeFile := filepath.Join(homeDir, ".testrc")
	if err := os.WriteFile(homeFile, []byte("test content"), 0o644); err != nil {
		t.Fatal(err)
	}

	hash, err := enrollWithHome(homeFile, repoDir, homeDir)
	if err != nil {
		t.Fatalf("enrollWithHome: %v", err)
	}

	// homeFile should now be a symlink.
	info, err := os.Lstat(homeFile)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected homeFile to be a symlink after Enroll")
	}

	// Symlink should point to the mirrored repo path.
	target, err := os.Readlink(homeFile)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	want := filepath.Join(repoDir, ".testrc")
	if target != want {
		t.Errorf("symlink target %q, want %q", target, want)
	}

	// Content still accessible through the symlink.
	content, err := os.ReadFile(homeFile)
	if err != nil {
		t.Fatalf("ReadFile through symlink: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("content %q, want %q", string(content), "test content")
	}

	if !strings.HasPrefix(hash, "sha256:") {
		t.Errorf("hash %q should start with sha256:", hash)
	}
}

func TestEnrollMirrorsSubdirectory(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := t.TempDir()

	subDir := filepath.Join(homeDir, ".config", "fish")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	homeFile := filepath.Join(subDir, "config.fish")
	if err := os.WriteFile(homeFile, []byte("set -x PATH"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := enrollWithHome(homeFile, repoDir, homeDir)
	if err != nil {
		t.Fatalf("enrollWithHome: %v", err)
	}

	// Repo should contain the file at the mirrored path.
	repoFile := filepath.Join(repoDir, ".config", "fish", "config.fish")
	if _, err := os.Stat(repoFile); err != nil {
		t.Errorf("expected repo file at %s: %v", repoFile, err)
	}

	// homeFile should point to the repo file.
	target, _ := os.Readlink(homeFile)
	if target != repoFile {
		t.Errorf("symlink target %q, want %q", target, repoFile)
	}
}

func TestCopyFilePreservesMode(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "id_rsa")
	dst := filepath.Join(dir, "id_rsa.copy")

	// Write with SSH-key-like restricted permissions.
	if err := os.WriteFile(src, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %04o, want 0600", info.Mode().Perm())
	}
}

func TestLink(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := t.TempDir()

	repoFile := filepath.Join(repoDir, ".vimrc")
	if err := os.WriteFile(repoFile, []byte("set nu"), 0o644); err != nil {
		t.Fatal(err)
	}

	homePath := filepath.Join(homeDir, ".vimrc")

	if err := Link(homePath, repoFile); err != nil {
		t.Fatalf("Link: %v", err)
	}

	target, err := os.Readlink(homePath)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != repoFile {
		t.Errorf("symlink target %q, want %q", target, repoFile)
	}

	// Re-running Link should succeed (idempotent).
	if err := Link(homePath, repoFile); err != nil {
		t.Errorf("second Link: %v", err)
	}
}
