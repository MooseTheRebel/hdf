package repo

import (
	"errors"
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

// TestInitOrOpenCreatesNestedDirectories pins the contract that InitOrOpen
// creates any missing intermediate directories, so callers do not need to
// call os.MkdirAll beforehand.
func TestInitOrOpenCreatesNestedDirectories(t *testing.T) {
	nested := filepath.Join(t.TempDir(), "a", "b", "c")
	if _, err := InitOrOpen(nested); err != nil {
		t.Fatalf("InitOrOpen on nested non-existent path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nested, ".git")); err != nil {
		t.Errorf("expected .git directory at %s: %v", nested, err)
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

// TestFastForwardFromMain verifies that FastForwardFromMain advances the local
// branch to match origin/main when main has new commits (fast-forward only).
func TestFastForwardFromMain(t *testing.T) {
	bareDir := t.TempDir()
	if _, _, err := InitOrOpenBare(bareDir); err != nil {
		t.Fatalf("InitOrOpenBare: %v", err)
	}
	bareURL := "file://" + bareDir

	seedDir := t.TempDir()
	seed, err := Init(seedDir)
	if err != nil {
		t.Fatalf("seed Init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seedDir, "seed.txt"), []byte("v1"), 0o644); err != nil {
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

	localDir := t.TempDir()
	local, err := Clone(bareURL, localDir)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// Advance main on the remote.
	if err := os.WriteFile(filepath.Join(seedDir, "seed.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.CommitFile("seed.txt", "update"); err != nil {
		t.Fatalf("seed CommitFile update: %v", err)
	}
	if err := seed.Push("main"); err != nil {
		t.Fatalf("seed Push update: %v", err)
	}
	mainSHA, err := seed.BranchSHA("main")
	if err != nil {
		t.Fatalf("BranchSHA: %v", err)
	}

	if err := local.Fetch(); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if err := local.FastForwardFromMain(); err != nil {
		t.Fatalf("FastForwardFromMain: %v", err)
	}

	head, err := local.HeadSHA()
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	if head != mainSHA {
		t.Errorf("HEAD = %s, want %s", head, mainSHA)
	}
}

func TestIsCleanForPromote(t *testing.T) {
	dir := t.TempDir()
	r, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(dir, "dot.txt")
	if err := os.WriteFile(f, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("dot.txt", "add"); err != nil {
		t.Fatal(err)
	}

	// No uncommitted changes — should be clean.
	clean, err := r.IsCleanForPromote()
	if err != nil {
		t.Fatalf("IsCleanForPromote: %v", err)
	}
	if !clean {
		t.Error("want clean after commit, got dirty")
	}

	// Modify the file without committing — should be dirty.
	if err := os.WriteFile(f, []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err := r.IsCleanForPromote()
	if err != nil {
		t.Fatalf("IsCleanForPromote: %v", err)
	}
	if dirty {
		t.Error("want dirty after uncommitted change, got clean")
	}
}

func TestMergeIntoBranch(t *testing.T) {
	// Set up a repo with a commit on main, then create a machine branch with
	// one more commit. MergeIntoBranch("main") should fast-forward main.
	bareDir := t.TempDir()
	if _, _, err := InitOrOpenBare(bareDir); err != nil {
		t.Fatalf("InitOrOpenBare: %v", err)
	}
	bareURL := "file://" + bareDir

	workDir := t.TempDir()
	r, err := Init(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("base.txt", "base commit"); err != nil {
		t.Fatal(err)
	}
	if err := r.AddRemote("origin", bareURL); err != nil {
		t.Fatal(err)
	}
	if err := r.Push("main"); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateAndCheckoutBranch("machine"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "extra.txt"), []byte("extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sha, err := r.CommitFile("extra.txt", "machine commit")
	if err != nil {
		t.Fatal(err)
	}

	// machine branch is ahead of main — MergeIntoBranch should fast-forward main.
	if err := r.MergeIntoBranch("main"); err != nil {
		t.Fatalf("MergeIntoBranch: %v", err)
	}

	mainSHA, err := r.BranchSHA("main")
	if err != nil {
		t.Fatalf("BranchSHA: %v", err)
	}
	if mainSHA != sha {
		t.Errorf("main SHA = %s, want %s", mainSHA, sha)
	}
}

// TestMergeIntoBranchDivergedCreatesMergeCommit verifies that when branches have
// diverged, MergeIntoBranch creates a merge commit using the machine branch's tree
// (equivalent to `git merge -X theirs`) without touching the working tree.
func TestMergeIntoBranchDivergedCreatesMergeCommit(t *testing.T) {
	workDir := t.TempDir()
	r, err := Init(workDir)
	if err != nil {
		t.Fatal(err)
	}
	// Initial commit on main.
	if err := os.WriteFile(filepath.Join(workDir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("a.txt", "main commit"); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateAndCheckoutBranch("machine"); err != nil {
		t.Fatal(err)
	}
	// Commit on machine branch with real content for b.txt.
	if err := os.WriteFile(filepath.Join(workDir, "b.txt"), []byte("b-real\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	machineSHA, err := r.CommitFile("b.txt", "machine commit")
	if err != nil {
		t.Fatal(err)
	}
	// Commit on main independently (diverge) with a placeholder for b.txt.
	if _, err := r.CommitFilesToBranch("main", []BranchFile{
		{RepoRelPath: "b.txt", Content: []byte("")},
	}, "main-only commit"); err != nil {
		t.Fatal(err)
	}

	// MergeIntoBranch should succeed and create a merge commit.
	if err := r.MergeIntoBranch("main"); err != nil {
		t.Fatalf("MergeIntoBranch diverged: %v", err)
	}

	// main should now point to a new merge commit descended from both branches.
	mainSHA, err := r.BranchSHA("main")
	if err != nil {
		t.Fatalf("BranchSHA main: %v", err)
	}
	if mainSHA == machineSHA {
		t.Error("main should be a new merge commit, not just the machine branch tip")
	}

	// The merge commit's tree should reflect the machine branch content (b-real).
	mainCommit, err := r.r.CommitObject(plumbing.NewHash(mainSHA))
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}
	if len(mainCommit.ParentHashes) != 2 {
		t.Errorf("merge commit should have 2 parents, got %d", len(mainCommit.ParentHashes))
	}
	// Tree should have b.txt with machine-branch content.
	tree, err := mainCommit.Tree()
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	f, err := tree.File("b.txt")
	if err != nil {
		t.Fatalf("tree.File b.txt: %v", err)
	}
	content, err := f.Contents()
	if err != nil {
		t.Fatalf("file contents: %v", err)
	}
	if content != "b-real\n" {
		t.Errorf("b.txt content = %q, want %q", content, "b-real\n")
	}
}

// TestMergeIntoBranchPreservesMainOnlyFiles verifies that when branches have
// diverged, MergeIntoBranch keeps files that exist only on main (e.g. dotfiles
// promoted by other machines) instead of silently deleting them.
func TestMergeIntoBranchPreservesMainOnlyFiles(t *testing.T) {
	workDir := t.TempDir()
	r, err := Init(workDir)
	if err != nil {
		t.Fatal(err)
	}
	// Initial commit on main: shared.txt
	if err := os.WriteFile(filepath.Join(workDir, "shared.txt"), []byte("shared\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("shared.txt", "initial"); err != nil {
		t.Fatal(err)
	}
	// Create machine branch from main.
	if err := r.CreateAndCheckoutBranch("machine"); err != nil {
		t.Fatal(err)
	}
	// Machine adds machine-only.txt (diverges from main).
	if err := os.WriteFile(filepath.Join(workDir, "machine-only.txt"), []byte("machine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("machine-only.txt", "machine adds file"); err != nil {
		t.Fatal(err)
	}
	// Main independently adds main-only.txt (another machine promoted it).
	if _, err := r.CommitFilesToBranch("main", []BranchFile{
		{RepoRelPath: "main-only.txt", Content: []byte("main-only\n")},
	}, "other machine promotes file"); err != nil {
		t.Fatal(err)
	}

	if err := r.MergeIntoBranch("main"); err != nil {
		t.Fatalf("MergeIntoBranch: %v", err)
	}

	// machine-only.txt must be present with machine content.
	got, err := r.ReadFileFromBranch("main", "machine-only.txt")
	if err != nil {
		t.Fatalf("ReadFileFromBranch machine-only.txt: %v", err)
	}
	if string(got) != "machine\n" {
		t.Errorf("machine-only.txt = %q, want %q", string(got), "machine\n")
	}

	// main-only.txt must survive — this is the regression this test guards.
	got, err = r.ReadFileFromBranch("main", "main-only.txt")
	if err != nil {
		t.Fatalf("ReadFileFromBranch main-only.txt: %v", err)
	}
	if string(got) != "main-only\n" {
		t.Errorf("main-only.txt = %q, want %q (must be preserved from main)", string(got), "main-only\n")
	}
}

// TestMergeIntoBranchDivergedParentOrder verifies that when a diverged merge
// commit is written to main, main's previous HEAD is parent[0] so that
// git log --first-parent traces main's own history, not the machine branch.
func TestMergeIntoBranchDivergedParentOrder(t *testing.T) {
	workDir := t.TempDir()
	r, err := Init(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("a.txt", "initial"); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateAndCheckoutBranch("machine"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("b.txt", "machine commit"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFilesToBranch("main", []BranchFile{
		{RepoRelPath: "c.txt", Content: []byte("c\n")},
	}, "main-only commit"); err != nil {
		t.Fatal(err)
	}

	prevMainSHA, err := r.BranchSHA("main")
	if err != nil {
		t.Fatalf("BranchSHA before merge: %v", err)
	}

	if err := r.MergeIntoBranch("main"); err != nil {
		t.Fatalf("MergeIntoBranch: %v", err)
	}

	mainSHA, err := r.BranchSHA("main")
	if err != nil {
		t.Fatalf("BranchSHA after merge: %v", err)
	}
	mergeCommit, err := r.r.CommitObject(plumbing.NewHash(mainSHA))
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}
	if len(mergeCommit.ParentHashes) != 2 {
		t.Fatalf("expected 2 parents, got %d", len(mergeCommit.ParentHashes))
	}
	// parent[0] must be main's previous HEAD so git log --first-parent
	// follows main's own lineage, not the machine branch.
	if mergeCommit.ParentHashes[0].String() != prevMainSHA {
		t.Errorf("parent[0] = %s, want main's prev HEAD %s",
			mergeCommit.ParentHashes[0].String(), prevMainSHA)
	}
}

// TestMergeIntoBranchRefusesWhenMachineDeletedFile verifies that MergeIntoBranch
// returns an error when the machine branch deleted a file that still exists on
// main, rather than silently merging and losing the deletion signal.
func TestPushNonFastForwardReturnsTypedError(t *testing.T) {
	bareDir := t.TempDir()
	if _, _, err := InitOrOpenBare(bareDir); err != nil {
		t.Fatalf("InitOrOpenBare: %v", err)
	}
	bareURL := "file://" + bareDir

	// Repo A: init, commit v1, push to bare.
	dirA := t.TempDir()
	repoA, err := Init(dirA)
	if err != nil {
		t.Fatalf("Init A: %v", err)
	}
	if err := repoA.AddRemote("origin", bareURL); err != nil {
		t.Fatalf("AddRemote A: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirA, "f.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repoA.CommitFile("f.txt", "A: v1"); err != nil {
		t.Fatalf("CommitFile A v1: %v", err)
	}
	if err := repoA.Push("main"); err != nil {
		t.Fatalf("Push A v1: %v", err)
	}

	// Repo B: clone, commit v2, push — advancing bare past A's commit.
	dirB := t.TempDir()
	repoB, err := Clone(bareURL, dirB)
	if err != nil {
		t.Fatalf("Clone B: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "f.txt"), []byte("v2-B"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repoB.CommitFile("f.txt", "B: v2"); err != nil {
		t.Fatalf("CommitFile B v2: %v", err)
	}
	if err := repoB.Push("main"); err != nil {
		t.Fatalf("Push B v2: %v", err)
	}

	// Repo A: commit something new on top of its own v1 (without pulling B's changes).
	if err := os.WriteFile(filepath.Join(dirA, "f.txt"), []byte("v2-A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repoA.CommitFile("f.txt", "A: v2"); err != nil {
		t.Fatalf("CommitFile A v2: %v", err)
	}

	// Push should fail with a typed ErrNonFastForwardUpdate.
	err = repoA.Push("main")
	if err == nil {
		t.Fatal("expected non-fast-forward error, got nil")
	}
	if !errors.Is(err, ErrNonFastForwardUpdate) {
		t.Errorf("errors.Is(err, ErrNonFastForwardUpdate) = false; got: %v", err)
	}
}

func TestResetBranchToRemote(t *testing.T) {
	bareDir := t.TempDir()
	bareURL := "file://" + bareDir
	if _, _, err := InitOrOpenBare(bareDir); err != nil {
		t.Fatalf("InitOrOpenBare: %v", err)
	}

	dir := t.TempDir()
	r, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := r.AddRemote("origin", bareURL); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}

	// Commit v1 to local main and push so origin/main tracking ref exists.
	f := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(f, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("f.txt", "v1"); err != nil {
		t.Fatalf("CommitFile v1: %v", err)
	}
	if err := r.Push("main"); err != nil {
		t.Fatalf("Push v1: %v", err)
	}

	// Advance local main to v2 WITHOUT pushing.
	if err := os.WriteFile(f, []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("f.txt", "v2"); err != nil {
		t.Fatalf("CommitFile v2: %v", err)
	}

	// Verify local main differs from origin/main before reset.
	localBytes, err := r.ReadFileFromBranch("main", "f.txt")
	if err != nil {
		t.Fatalf("ReadFileFromBranch before reset: %v", err)
	}
	remoteBytes, err := r.ReadFileFromRemoteBranch("origin", "main", "f.txt")
	if err != nil {
		t.Fatalf("ReadFileFromRemoteBranch before reset: %v", err)
	}
	if string(localBytes) == string(remoteBytes) {
		t.Fatal("setup error: local and remote main should differ before reset")
	}

	// Reset.
	if err := r.ResetBranchToRemote("main", "origin"); err != nil {
		t.Fatalf("ResetBranchToRemote: %v", err)
	}

	// After reset local main should match origin/main.
	localBytes, err = r.ReadFileFromBranch("main", "f.txt")
	if err != nil {
		t.Fatalf("ReadFileFromBranch after reset: %v", err)
	}
	if string(localBytes) != string(remoteBytes) {
		t.Errorf("after reset: local main = %q, want %q", localBytes, remoteBytes)
	}
}

func TestResetBranchToRemoteAfterFailedPush(t *testing.T) {
	bareDir := t.TempDir()
	bareURL := "file://" + bareDir
	if _, _, err := InitOrOpenBare(bareDir); err != nil {
		t.Fatalf("InitOrOpenBare: %v", err)
	}

	// Repo A: init, seed bare with an initial commit.
	dirA := t.TempDir()
	rA, err := Init(dirA)
	if err != nil {
		t.Fatalf("Init A: %v", err)
	}
	if err := rA.AddRemote("origin", bareURL); err != nil {
		t.Fatalf("AddRemote A: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirA, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := rA.CommitFile("seed.txt", "seed"); err != nil {
		t.Fatalf("CommitFile seed: %v", err)
	}
	if err := rA.Push("main"); err != nil {
		t.Fatalf("Push seed: %v", err)
	}

	// Clone B: gets the seeded state. Commits to local main (simulating guard-2-passing promote).
	dirB := filepath.Join(t.TempDir(), "repoB")
	rB, err := Clone(bareURL, dirB)
	if err != nil {
		t.Fatalf("Clone B: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "b.txt"), []byte("B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := rB.CommitFile("b.txt", "B advances main"); err != nil {
		t.Fatalf("CommitFile B: %v", err)
	}

	// Record origin/main tracking ref before the race (reflects seeded state).
	remoteBytes, err := rB.ReadFileFromRemoteBranch("origin", "main", "seed.txt")
	if err != nil {
		t.Fatalf("ReadFileFromRemoteBranch before race: %v", err)
	}

	// Race: A pushes to bare AFTER B's local advance but BEFORE B pushes.
	if err := os.WriteFile(filepath.Join(dirA, "a.txt"), []byte("A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := rA.CommitFile("a.txt", "A races"); err != nil {
		t.Fatalf("CommitFile A race: %v", err)
	}
	if err := rA.Push("main"); err != nil {
		t.Fatalf("A Push (race): %v", err)
	}

	// B's push fails: bare main has A's race commit, B's local main has its extra commit.
	pushErr := rB.Push("main")
	if !errors.Is(pushErr, ErrNonFastForwardUpdate) {
		t.Fatalf("expected ErrNonFastForwardUpdate, got: %v", pushErr)
	}

	// Rollback: reset local main to origin/main.
	if err := rB.ResetBranchToRemote("main", "origin"); err != nil {
		t.Fatalf("ResetBranchToRemote: %v", err)
	}

	// After rollback, local main should match origin/main tracking ref (pre-race state).
	localBytes, err := rB.ReadFileFromBranch("main", "seed.txt")
	if err != nil {
		t.Fatalf("ReadFileFromBranch after rollback: %v", err)
	}
	if string(localBytes) != string(remoteBytes) {
		t.Errorf("after rollback: local main seed.txt = %q, want %q", localBytes, remoteBytes)
	}
	// B's extra commit (b.txt) should no longer be on local main.
	bBytes, _ := rB.ReadFileFromBranch("main", "b.txt")
	if len(bBytes) > 0 {
		t.Error("after rollback: b.txt should not be on local main")
	}
}

func TestMergeIntoBranchRefusesWhenMachineDeletedFile(t *testing.T) {
	workDir := t.TempDir()
	r, err := Init(workDir)
	if err != nil {
		t.Fatal(err)
	}
	// Initial commit: both files present on main.
	for _, name := range []string{"shared.txt", "to-unenroll.txt"} {
		if err := os.WriteFile(filepath.Join(workDir, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := r.CommitFile(name, "add "+name); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.CreateAndCheckoutBranch("machine"); err != nil {
		t.Fatal(err)
	}
	// Machine unenrolls to-unenroll.txt — deletes it from the branch.
	w, err := r.r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Remove("to-unenroll.txt"); err != nil {
		t.Fatalf("w.Remove: %v", err)
	}
	if _, err := r.CommitStaged("machine: unenroll to-unenroll.txt"); err != nil {
		t.Fatalf("CommitStaged: %v", err)
	}
	// Main diverges independently (to-unenroll.txt still exists there).
	if _, err := r.CommitFilesToBranch("main", []BranchFile{
		{RepoRelPath: "shared.txt", Content: []byte("updated\n")},
	}, "main: update shared"); err != nil {
		t.Fatal(err)
	}

	err = r.MergeIntoBranch("main")
	if err == nil {
		t.Fatal("expected error when machine deleted a file that still exists on main, got nil")
	}
	if !strings.Contains(err.Error(), "to-unenroll.txt") {
		t.Errorf("error = %q, want mention of deleted file", err.Error())
	}
}

// TestMergeIntoBranchReturnsErrorOnTypeConflict verifies that when the machine
// branch has a regular file at a path that main has as a directory (or vice
// versa), MergeIntoBranch returns an error rather than silently discarding the
// directory's contents.
func TestMergeIntoBranchReturnsErrorOnTypeConflict(t *testing.T) {
	workDir := t.TempDir()
	r, err := Init(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("base.txt", "initial commit"); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateAndCheckoutBranch("machine"); err != nil {
		t.Fatal(err)
	}
	// Machine branch: "config" is a regular file.
	if err := os.WriteFile(filepath.Join(workDir, "config"), []byte("key=value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFile("config", "machine: add config file"); err != nil {
		t.Fatal(err)
	}
	// Main: "config" is a directory containing "settings".
	if _, err := r.CommitFilesToBranch("main", []BranchFile{
		{RepoRelPath: "config/settings", Content: []byte("setting=foo\n")},
	}, "main: add config dir"); err != nil {
		t.Fatal(err)
	}

	// MergeIntoBranch must return an error — silently discarding one side's
	// content ("config" directory on main, or "config" file on machine) would
	// cause data loss with no user-visible indication.
	err = r.MergeIntoBranch("main")
	if err == nil {
		t.Fatal("MergeIntoBranch should return an error for a file/directory type conflict, got nil")
	}
	if !strings.Contains(err.Error(), "conflicting types") {
		t.Errorf("error = %q, want 'conflicting types'", err.Error())
	}
}
