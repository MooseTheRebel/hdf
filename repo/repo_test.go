package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

func TestAddRemote(t *testing.T) {
	dir := t.TempDir()
	r, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	const remoteA = "file:///tmp/bare-a"
	const remoteB = "file:///tmp/bare-b"

	// First call: remote does not exist — should succeed.
	if err := r.AddRemote("origin", remoteA); err != nil {
		t.Fatalf("AddRemote (create): %v", err)
	}

	// Second call with the same URL: no-op, should succeed.
	if err := r.AddRemote("origin", remoteA); err != nil {
		t.Fatalf("AddRemote (same URL no-op): %v", err)
	}

	// Third call with a different URL: must return an error.
	err = r.AddRemote("origin", remoteB)
	if err == nil {
		t.Fatal("expected error when adding remote with different URL, got nil")
	}
	if !strings.Contains(err.Error(), "already points to a different URL") {
		t.Errorf("error = %q, want it to mention 'already points to a different URL'", err.Error())
	}
}

func TestInitOrOpenBare(t *testing.T) {
	t.Run("creates bare repo", func(t *testing.T) {
		dir := t.TempDir()
		_, created, err := InitOrOpenBare(dir)
		if err != nil {
			t.Fatalf("InitOrOpenBare (create): %v", err)
		}
		if !created {
			t.Error("expected created=true for a new bare repo")
		}
		if _, err := os.Stat(filepath.Join(dir, "HEAD")); err != nil {
			t.Errorf("bare repo missing HEAD file: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			t.Error("bare repo should not have a .git subdirectory")
		}
	})

	t.Run("opens existing bare repo", func(t *testing.T) {
		dir := t.TempDir()
		if _, _, err := InitOrOpenBare(dir); err != nil {
			t.Fatalf("first InitOrOpenBare: %v", err)
		}
		_, created, err := InitOrOpenBare(dir)
		if err != nil {
			t.Fatalf("second InitOrOpenBare (open): %v", err)
		}
		if created {
			t.Error("expected created=false when opening existing bare repo")
		}
	})

	t.Run("errors on non-bare repo", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := Init(dir); err != nil {
			t.Fatalf("Init: %v", err)
		}
		_, _, err := InitOrOpenBare(dir)
		if err == nil {
			t.Fatal("expected error when path contains a non-bare repo, got nil")
		}
		if !strings.Contains(err.Error(), "not a bare repository") {
			t.Errorf("error = %q, want it to mention 'not a bare repository'", err.Error())
		}
	})
}

func TestCommitHistory(t *testing.T) {
	dir := t.TempDir()
	r, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	letters := []string{"a", "b", "c", "d", "e", "f"}
	var shas []string

	for i, letter := range letters {
		if err := os.WriteFile(filepath.Join(dir, "testfile.txt"), []byte(letter), 0o644); err != nil {
			t.Fatalf("WriteFile commit %d: %v", i+1, err)
		}
		sha, err := r.CommitFile("testfile.txt", fmt.Sprintf("commit %d", i+1))
		if err != nil {
			t.Fatalf("CommitFile %d: %v", i+1, err)
		}
		if sha == "" {
			t.Errorf("commit %d: empty SHA", i+1)
		}
		shas = append(shas, sha)
	}

	// Expect 6 commits
	count, err := r.CommitCount()
	if err != nil {
		t.Fatalf("CommitCount: %v", err)
	}
	if count != 6 {
		t.Errorf("CommitCount = %d, want 6", count)
	}

	// HEAD should be the last commit
	head, err := r.HeadSHA()
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	if head != shas[5] {
		t.Errorf("HEAD = %s, want %s", head, shas[5])
	}

	// All SHAs should be unique
	seen := map[string]bool{}
	for i, sha := range shas {
		if seen[sha] {
			t.Errorf("duplicate SHA at commit %d: %s", i+1, sha)
		}
		seen[sha] = true
	}
}

func TestBranchCreation(t *testing.T) {
	dir := t.TempDir()
	r, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Need at least one commit before branching
	if err := os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("init.txt", "initial commit"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	const branchName = "test-machine"
	if err := r.CreateAndCheckoutBranch(branchName); err != nil {
		t.Fatalf("CreateAndCheckoutBranch: %v", err)
	}

	branch, err := r.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != branchName {
		t.Errorf("CurrentBranch = %q, want %q", branch, branchName)
	}
}

func TestHasNewCommitsOnMain(t *testing.T) {
	dir := t.TempDir()
	r, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	sha1, _ := r.CommitFile("f.txt", "first")

	// Tracked at sha1; no new commits yet
	behind, err := r.HasNewCommitsOnMain(sha1)
	if err != nil {
		t.Fatalf("HasNewCommitsOnMain: %v", err)
	}
	if behind {
		t.Error("should not be behind when last_commit == HEAD")
	}

	// Add a second commit
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _ = r.CommitFile("f.txt", "second")

	// Now main is ahead of our tracked sha1
	behind, err = r.HasNewCommitsOnMain(sha1)
	if err != nil {
		t.Fatalf("HasNewCommitsOnMain after second commit: %v", err)
	}
	if !behind {
		t.Error("should be behind after second commit")
	}
}

func TestPushToFilePushTarget(t *testing.T) {
	workDir := t.TempDir()
	bareDir := t.TempDir()

	r, err := Init(workDir)
	if err != nil {
		t.Fatalf("Init workDir: %v", err)
	}

	_, _, err = InitOrOpenBare(bareDir)
	if err != nil {
		t.Fatalf("InitOrOpenBare: %v", err)
	}

	if err := r.AddRemote("origin", "file://"+bareDir); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}

	if got := r.RemoteURL(); got != "file://"+bareDir {
		t.Errorf("RemoteURL = %q, want %q", got, "file://"+bareDir)
	}

	if err := os.WriteFile(filepath.Join(workDir, "dot.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("dot.txt", "add dot.txt"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	if err := r.Push("main"); err != nil {
		t.Fatalf("Push: %v", err)
	}

	bare, err := Open(bareDir)
	if err != nil {
		t.Fatalf("Open bareDir: %v", err)
	}
	sha, err := bare.HeadSHA()
	if err != nil {
		t.Fatalf("HeadSHA on bare repo: %v", err)
	}
	if sha == "" {
		t.Error("expected non-empty SHA in bare repo after push")
	}
}

func TestCommitFilesToBranch(t *testing.T) {
	dir := t.TempDir()
	r, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Initial commit on main.
	if err := os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("init.txt", "initial"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	// Check out a hostname branch — from now on main is NOT the working branch.
	if err := r.CreateAndCheckoutBranch("my-host"); err != nil {
		t.Fatalf("CreateAndCheckoutBranch: %v", err)
	}

	// Write two files to main without touching the working tree.
	sha, err := r.CommitFilesToBranch("main", []BranchFile{
		{RepoRelPath: "stub.txt", Content: []byte{}},
		{RepoRelPath: ".hdf/managed.toml", Content: []byte("[files]\n")},
	}, "hdf: register baseline")
	if err != nil {
		t.Fatalf("CommitFilesToBranch: %v", err)
	}
	if sha == "" {
		t.Fatal("CommitFilesToBranch returned empty SHA")
	}

	// Verify: still on my-host (working tree unchanged).
	branch, err := r.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "my-host" {
		t.Errorf("CurrentBranch = %q, want %q", branch, "my-host")
	}

	// Verify: main ref points to the new commit.
	mainRef, err := r.r.Reference(plumbing.NewBranchReferenceName("main"), true)
	if err != nil {
		t.Fatalf("resolving main: %v", err)
	}
	if mainRef.Hash().String() != sha {
		t.Errorf("main HEAD = %s, want %s", mainRef.Hash(), sha)
	}

	// Verify: the new main commit's tree contains stub.txt and .hdf/managed.toml.
	mainCommit, err := r.r.CommitObject(mainRef.Hash())
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}
	mainTree, err := r.r.TreeObject(mainCommit.TreeHash)
	if err != nil {
		t.Fatalf("TreeObject: %v", err)
	}
	foundStub := false
	foundHDF := false
	for _, e := range mainTree.Entries {
		if e.Name == "stub.txt" {
			foundStub = true
		}
		if e.Name == ".hdf" {
			// Verify the subtree contains managed.toml.
			sub, err := r.r.TreeObject(e.Hash)
			if err == nil {
				for _, se := range sub.Entries {
					if se.Name == "managed.toml" {
						foundHDF = true
					}
				}
			}
		}
	}
	if !foundStub {
		t.Error("stub.txt not found in main tree")
	}
	if !foundHDF {
		t.Error(".hdf/managed.toml not found in main tree")
	}
}

func TestStageAndCommitMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	r, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	files := []string{"a.txt", "b.txt", "c.txt"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := r.StageFile(name); err != nil {
			t.Fatalf("StageFile(%s): %v", name, err)
		}
	}

	sha, err := r.CommitStaged("batch commit")
	if err != nil {
		t.Fatalf("CommitStaged: %v", err)
	}
	if sha == "" {
		t.Error("CommitStaged returned empty SHA")
	}

	count, err := r.CommitCount()
	if err != nil {
		t.Fatalf("CommitCount: %v", err)
	}
	if count != 1 {
		t.Errorf("CommitCount = %d, want 1", count)
	}

	head, err := r.HeadSHA()
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	if head != sha {
		t.Errorf("HEAD = %s, want %s", head, sha)
	}
}

func TestHasUnpushedCommits(t *testing.T) {
	dir := t.TempDir()
	r, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Initial commit on main
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("f.txt", "initial"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// Create hostname branch from main
	const hostname = "myhost"
	if err := r.CreateAndCheckoutBranch(hostname); err != nil {
		t.Fatalf("CreateAndCheckoutBranch: %v", err)
	}

	// No divergence yet — hostname is at the same commit as main
	unpushed, err := r.HasUnpushedCommits(hostname, "main")
	if err != nil {
		t.Fatalf("HasUnpushedCommits (no divergence): %v", err)
	}
	if unpushed {
		t.Error("should have no unpushed commits when branch == base")
	}

	// Commit something on hostname branch
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("change"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("f.txt", "hostname change"); err != nil {
		t.Fatalf("hostname commit: %v", err)
	}

	unpushed, err = r.HasUnpushedCommits(hostname, "main")
	if err != nil {
		t.Fatalf("HasUnpushedCommits (after commit): %v", err)
	}
	if !unpushed {
		t.Error("should have unpushed commits after committing on hostname branch")
	}
}

func TestHasIncomingCommits(t *testing.T) {
	// seedBare creates a bare repo with an initial commit on main and returns
	// the file:// URL and a seed Repo that pushes to it.
	seedBare := func(t *testing.T) (bareURL string, seed *Repo) {
		t.Helper()
		bareDir := t.TempDir()
		if _, _, err := InitOrOpenBare(bareDir); err != nil {
			t.Fatalf("InitOrOpenBare: %v", err)
		}
		bareURL = "file://" + bareDir
		seedDir := t.TempDir()
		var err error
		seed, err = Init(seedDir)
		if err != nil {
			t.Fatalf("seed Init: %v", err)
		}
		if err := os.WriteFile(filepath.Join(seedDir, "seed.txt"), []byte("seed"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := seed.CommitFile("seed.txt", "initial"); err != nil {
			t.Fatalf("seed CommitFile: %v", err)
		}
		if err := seed.AddRemote("origin", bareURL); err != nil {
			t.Fatalf("seed AddRemote: %v", err)
		}
		if err := seed.Push("main"); err != nil {
			t.Fatalf("seed Push: %v", err)
		}
		return bareURL, seed
	}

	cases := []struct {
		name  string
		setup func(t *testing.T, local *Repo, localDir string, seed *Repo)
		want  bool
	}{
		{
			name:  "HEAD == origin/main (up to date)",
			setup: func(t *testing.T, local *Repo, localDir string, seed *Repo) {},
			want:  false,
		},
		{
			name: "origin/main is ahead (incoming commits)",
			setup: func(t *testing.T, local *Repo, localDir string, seed *Repo) {
				if err := os.WriteFile(filepath.Join(seed.Path(), "extra.txt"), []byte("extra"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, err := seed.CommitFile("extra.txt", "main advances"); err != nil {
					t.Fatal(err)
				}
				if err := seed.Push("main"); err != nil {
					t.Fatal(err)
				}
				if err := local.Fetch(); err != nil {
					t.Fatal(err)
				}
			},
			want: true,
		},
		{
			name: "HEAD is ahead of origin/main (local ahead)",
			setup: func(t *testing.T, local *Repo, localDir string, seed *Repo) {
				if err := os.WriteFile(filepath.Join(localDir, "local.txt"), []byte("local"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, err := local.CommitFile("local.txt", "local commit"); err != nil {
					t.Fatal(err)
				}
			},
			want: false,
		},
		{
			name: "diverged (both have new commits)",
			setup: func(t *testing.T, local *Repo, localDir string, seed *Repo) {
				if err := os.WriteFile(filepath.Join(localDir, "local.txt"), []byte("local"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, err := local.CommitFile("local.txt", "local commit"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(seed.Path(), "remote.txt"), []byte("remote"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, err := seed.CommitFile("remote.txt", "remote commit"); err != nil {
					t.Fatal(err)
				}
				if err := seed.Push("main"); err != nil {
					t.Fatal(err)
				}
				if err := local.Fetch(); err != nil {
					t.Fatal(err)
				}
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bareURL, seed := seedBare(t)
			localDir := t.TempDir()
			local, err := Clone(bareURL, localDir)
			if err != nil {
				t.Fatalf("Clone: %v", err)
			}
			tc.setup(t, local, localDir, seed)
			got, err := local.HasIncomingCommits()
			if err != nil {
				t.Fatalf("HasIncomingCommits: %v", err)
			}
			if got != tc.want {
				t.Errorf("want %v, got %v", tc.want, got)
			}
		})
	}
}
