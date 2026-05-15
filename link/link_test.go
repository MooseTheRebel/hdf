package link

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")

	if err := os.WriteFile(f, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	hash, err := HashFile(f)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}
	if !strings.HasPrefix(hash, "sha256:") {
		t.Errorf("hash %q should start with sha256:", hash)
	}

	// Deterministic: same content → same hash
	hash2, _ := HashFile(f)
	if hash != hash2 {
		t.Error("hash not deterministic")
	}

	// Different content → different hash
	os.WriteFile(f, []byte("world"), 0644)
	hash3, _ := HashFile(f)
	if hash == hash3 {
		t.Error("different content should produce different hash")
	}
}

func TestEnroll(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := t.TempDir()

	homeFile := filepath.Join(homeDir, ".testrc")
	if err := os.WriteFile(homeFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	hash, err := Enroll(homeFile, repoDir)
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}

	info, err := os.Lstat(homeFile)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected homeFile to be a symlink after Enroll")
	}

	target, err := os.Readlink(homeFile)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	want := filepath.Join(repoDir, ".testrc")
	if target != want {
		t.Errorf("symlink target %q, want %q", target, want)
	}

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

func TestLink(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := t.TempDir()

	repoFile := filepath.Join(repoDir, ".vimrc")
	if err := os.WriteFile(repoFile, []byte("set nu"), 0644); err != nil {
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

	// Re-running Link should succeed (idempotent)
	if err := Link(homePath, repoFile); err != nil {
		t.Errorf("second Link: %v", err)
	}
}
