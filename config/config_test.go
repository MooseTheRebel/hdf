package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testBashrcPath = "~/.bashrc"

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	want := &Config{
		GitPushTarget:    "https://github.com/test/dotfiles.git",
		LocalDotfilesDir: "/home/user/.local/share/hdf/repo",
		Branch:           "my-macbook",
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.GitPushTarget != want.GitPushTarget {
		t.Errorf("GitPushTarget: got %q, want %q", got.GitPushTarget, want.GitPushTarget)
	}
	if got.LocalDotfilesDir != want.LocalDotfilesDir {
		t.Errorf("LocalDotfilesDir: got %q, want %q", got.LocalDotfilesDir, want.LocalDotfilesDir)
	}
	if got.Branch != want.Branch {
		t.Errorf("Branch: got %q, want %q", got.Branch, want.Branch)
	}
}

func TestRegistryRoundTrip(t *testing.T) {
	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, ".hdf"), 0o755); err != nil {
		t.Fatal(err)
	}

	want := &Registry{
		Files: []ManagedFile{
			{Path: testBashrcPath, Hash: "sha256:deadbeef"},
			{Path: "~/.vimrc", Hash: "sha256:cafebabe"},
		},
	}

	if err := SaveRegistry(repoDir, want); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	got, err := LoadRegistry(repoDir)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
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

func TestRegistryWithVariants(t *testing.T) {
	repoDir := t.TempDir()

	want := &Registry{
		Files: []ManagedFile{
			{
				Path: "~/.ssh/id_rsa",
				Hash: "",
				Variants: []Variant{
					{Branch: "work-macbook", RepoPath: ".ssh/id_rsa_work-macbook", Hash: "sha256:aaa"},
					{Branch: "home-laptop", RepoPath: ".ssh/id_rsa_home-laptop", Hash: "sha256:bbb"},
				},
			},
		},
	}

	if err := SaveRegistry(repoDir, want); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}
	got, err := LoadRegistry(repoDir)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	if len(got.Files) != 1 {
		t.Fatalf("Files len: got %d, want 1", len(got.Files))
	}
	f := got.Files[0]
	if f.Path != "~/.ssh/id_rsa" {
		t.Errorf("Path: got %q, want ~/.ssh/id_rsa", f.Path)
	}
	if len(f.Variants) != 2 {
		t.Fatalf("Variants len: got %d, want 2", len(f.Variants))
	}
	if f.Variants[0].Branch != "work-macbook" || f.Variants[0].RepoPath != ".ssh/id_rsa_work-macbook" {
		t.Errorf("Variants[0]: got %+v", f.Variants[0])
	}
	if f.Variants[1].Branch != "home-laptop" || f.Variants[1].RepoPath != ".ssh/id_rsa_home-laptop" {
		t.Errorf("Variants[1]: got %+v", f.Variants[1])
	}
}

func TestLoadRegistryMissing(t *testing.T) {
	reg, err := LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("LoadRegistry on missing file should return empty registry, got error: %v", err)
	}
	if len(reg.Files) != 0 {
		t.Errorf("expected empty registry, got %d files", len(reg.Files))
	}
}

func TestRegistryMigrationFromConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	repoDir := t.TempDir()

	// Write a legacy config.toml that has the old Files field.
	legacy := `git_push_target = "file:///tmp/bare"
local_dotfiles_dir = "/tmp/repo"
branch = "test-host"

[[files]]
path = "~/.bashrc"
hash = "sha256:deadbeef"

[[files]]
path = "~/.vimrc"
hash = "sha256:cafebabe"
`
	if err := os.WriteFile(cfgPath, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := MigrateFilesToRegistry(cfgPath, repoDir); err != nil {
		t.Fatalf("MigrateFilesToRegistry: %v", err)
	}

	reg, err := LoadRegistry(repoDir)
	if err != nil {
		t.Fatalf("LoadRegistry after migration: %v", err)
	}
	if len(reg.Files) != 2 {
		t.Fatalf("Files len after migration: got %d, want 2", len(reg.Files))
	}
	if reg.Files[0].Path != testBashrcPath || reg.Files[0].Hash != "sha256:deadbeef" {
		t.Errorf("Files[0]: got %+v", reg.Files[0])
	}

	// Running migration again must be idempotent.
	if err := MigrateFilesToRegistry(cfgPath, repoDir); err != nil {
		t.Fatalf("second MigrateFilesToRegistry: %v", err)
	}
	reg2, err := LoadRegistry(repoDir)
	if err != nil {
		t.Fatalf("second LoadRegistry call failed: %v", err)
	}
	if len(reg2.Files) != 2 {
		t.Errorf("idempotent: Files len = %d, want 2", len(reg2.Files))
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.toml")

	want := &State{
		LastSync:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		LastCommit: "abc123def456",
	}

	if err := SaveState(path, want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	got, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if !got.LastSync.Equal(want.LastSync) {
		t.Errorf("LastSync: got %v, want %v", got.LastSync, want.LastSync)
	}
	if got.LastCommit != want.LastCommit {
		t.Errorf("LastCommit: got %q, want %q", got.LastCommit, want.LastCommit)
	}
}

func TestLoadStateMissing(t *testing.T) {
	state, err := LoadState("/nonexistent/path/state.toml")
	if err != nil {
		t.Fatalf("LoadState on missing file should return empty state, got error: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.LastCommit != "" {
		t.Errorf("expected empty LastCommit, got %q", state.LastCommit)
	}
}

func TestNormalizePath(t *testing.T) {
	homeDir := "/home/alice"

	cases := []struct {
		name string
		path string
		want string
	}{
		{
			name: "tilde form unchanged",
			path: testBashrcPath,
			want: testBashrcPath,
		},
		{
			name: "absolute path under home normalized",
			path: "/home/alice/.bashrc",
			want: testBashrcPath,
		},
		{
			name: "nested absolute path under home normalized",
			path: "/home/alice/.config/fish/config.fish",
			want: "~/.config/fish/config.fish",
		},
		{
			name: "absolute path outside home unchanged",
			path: "/etc/hosts",
			want: "/etc/hosts",
		},
		{
			name: "absolute path from different machine unchanged",
			path: "/home/bob/.bashrc",
			want: "/home/bob/.bashrc",
		},
		{
			name: "relative path unchanged",
			path: ".bashrc",
			want: ".bashrc",
		},
		{
			name: "path equal to homeDir returns unchanged",
			path: homeDir,
			want: homeDir,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizePath(c.path, homeDir)
			if got != c.want {
				t.Errorf("NormalizePath(%q, %q) = %q, want %q", c.path, homeDir, got, c.want)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	cases := []struct {
		in   string
		want string
	}{
		{testBashrcPath, filepath.Join(home, ".bashrc")},
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
