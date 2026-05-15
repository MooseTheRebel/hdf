package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	want := &Config{
		GitURL:     "https://github.com/test/dotfiles.git",
		RepoPath:   "/home/user/.local/share/hdf/repo",
		LastSync:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		LastCommit: "abc123def456",
		Files: []ManagedFile{
			{Path: "~/.bashrc", Hash: "sha256:deadbeef"},
			{Path: "~/.vimrc", Hash: "sha256:cafebabe"},
		},
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.GitURL != want.GitURL {
		t.Errorf("GitURL: got %q, want %q", got.GitURL, want.GitURL)
	}
	if got.RepoPath != want.RepoPath {
		t.Errorf("RepoPath: got %q, want %q", got.RepoPath, want.RepoPath)
	}
	if !got.LastSync.Equal(want.LastSync) {
		t.Errorf("LastSync: got %v, want %v", got.LastSync, want.LastSync)
	}
	if got.LastCommit != want.LastCommit {
		t.Errorf("LastCommit: got %q, want %q", got.LastCommit, want.LastCommit)
	}
	if len(got.Files) != len(want.Files) {
		t.Fatalf("Files len: got %d, want %d", len(got.Files), len(want.Files))
	}
	for i, f := range want.Files {
		if got.Files[i].Path != f.Path {
			t.Errorf("Files[%d].Path: got %q, want %q", i, got.Files[i].Path, f.Path)
		}
		if got.Files[i].Hash != f.Hash {
			t.Errorf("Files[%d].Hash: got %q, want %q", i, got.Files[i].Hash, f.Hash)
		}
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	cases := []struct {
		in   string
		want string
	}{
		{"~/.bashrc", filepath.Join(home, ".bashrc")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, c := range cases {
		got := ExpandPath(c.in)
		if got != c.want {
			t.Errorf("ExpandPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
