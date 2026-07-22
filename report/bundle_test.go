// report/bundle_test.go
package report

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompressRepo_ArchivesGitDirectory(t *testing.T) {
	repoDir := t.TempDir()
	gitDir := filepath.Join(repoDir, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "refs", "heads", "main"), []byte("deadbeef\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := CompressRepo(repoDir)
	if err != nil {
		t.Fatalf("CompressRepo: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	found := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("opening %s: %v", f.Name, err)
		}
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatalf("reading %s: %v", f.Name, err)
		}
		if err := rc.Close(); err != nil {
			t.Fatalf("closing %s: %v", f.Name, err)
		}
		found[f.Name] = buf.String()
	}
	if found["HEAD"] != "ref: refs/heads/main\n" {
		t.Errorf("HEAD = %q, want %q", found["HEAD"], "ref: refs/heads/main\n")
	}
	if found["refs/heads/main"] != "deadbeef\n" {
		t.Errorf("refs/heads/main = %q, want %q", found["refs/heads/main"], "deadbeef\n")
	}
}

func TestCompressRepo_TooLargeReturnsErrRepoTooLarge(t *testing.T) {
	repoDir := t.TempDir()
	gitDir := filepath.Join(repoDir, ".git", "objects")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Random bytes are incompressible, so a ~5MB blob stays well over the
	// 4MB cap after deflate.
	big := make([]byte, 5*1024*1024)
	if _, err := rand.Read(big); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "big.pack"), big, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := CompressRepo(repoDir)
	if !errors.Is(err, ErrRepoTooLarge) {
		t.Errorf("CompressRepo err = %v, want ErrRepoTooLarge", err)
	}
}

func TestCompressRepo_RedactsGitConfigCredentials(t *testing.T) {
	repoDir := t.TempDir()
	gitDir := filepath.Join(repoDir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configContent := "[core]\n\tbare = false\n[remote \"origin\"]\n\turl = " + testCredentialedURL + "\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := CompressRepo(repoDir)
	if err != nil {
		t.Fatalf("CompressRepo: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	var configEntry string
	for _, f := range zr.File {
		if f.Name != "config" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("opening config entry: %v", err)
		}
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatalf("reading config entry: %v", err)
		}
		if err := rc.Close(); err != nil {
			t.Fatalf("closing config entry: %v", err)
		}
		configEntry = buf.String()
	}
	if configEntry == "" {
		t.Fatal("zip has no \"config\" entry")
	}
	if strings.Contains(configEntry, "user:token") {
		t.Errorf("config entry still contains credentials:\n%s", configEntry)
	}
	if !strings.Contains(configEntry, testRedactedURL) {
		t.Errorf("config entry missing redacted URL:\n%s", configEntry)
	}
}
