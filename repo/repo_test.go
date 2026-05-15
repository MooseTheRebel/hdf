package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCommitHistory(t *testing.T) {
	dir := t.TempDir()
	r, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	letters := []string{"a", "b", "c", "d", "e", "f"}
	var shas []string

	for i, letter := range letters {
		if err := os.WriteFile(filepath.Join(dir, "testfile.txt"), []byte(letter), 0644); err != nil {
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
	if err := os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0644); err != nil {
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

	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a"), 0644)
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
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("b"), 0644)
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

func TestHasUnpushedCommits(t *testing.T) {
	dir := t.TempDir()
	r, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Initial commit on main
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("init"), 0644)
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
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("change"), 0644)
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
