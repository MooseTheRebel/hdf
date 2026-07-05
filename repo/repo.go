package repo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gogitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

var errStop = errors.New("stop")

// ErrNonFastForwardUpdate is returned by Push when the remote rejects the push
// because it is not a fast-forward update. Callers can test for this with errors.Is.
var ErrNonFastForwardUpdate = errors.New("non-fast-forward update")

// authForURL returns an appropriate go-git auth method for rawURL, or nil for
// public / unauthenticated access.
//
// SSH URLs ("git@…" or "ssh://…"): uses the running SSH agent.
// HTTPS URLs: uses HDF_GIT_TOKEN environment variable if set.
func authForURL(rawURL string) transport.AuthMethod {
	if strings.HasPrefix(rawURL, "git@") || strings.HasPrefix(rawURL, "ssh://") {
		if auth, err := gogitssh.NewSSHAgentAuth("git"); err == nil {
			return auth
		}
	}
	if token := os.Getenv("HDF_GIT_TOKEN"); token != "" {
		return &githttp.BasicAuth{Username: "hdf", Password: token}
	}
	return nil
}

// RemoteURL returns the fetch URL of the "origin" remote, or "" if unavailable.
func (r *Repo) RemoteURL() string {
	cfg, err := r.r.Config()
	if err != nil {
		return ""
	}
	remote, ok := cfg.Remotes["origin"]
	if !ok || len(remote.URLs) == 0 {
		return ""
	}
	return remote.URLs[0]
}

// Repo wraps a go-git repository with hdf-specific operations.
type Repo struct {
	r    *git.Repository
	path string
}

// Init creates a new git repository at path with "main" as the default branch.
func Init(path string) (*Repo, error) {
	r, err := git.PlainInitWithOptions(path, &git.PlainInitOptions{
		InitOptions: git.InitOptions{
			DefaultBranch: plumbing.NewBranchReferenceName("main"),
		},
	})
	if err != nil {
		return nil, err
	}
	return &Repo{r: r, path: path}, nil
}

// InitOrOpen initializes a non-bare repository at path, or opens it if it
// already exists. Missing intermediate directories are created automatically.
func InitOrOpen(path string) (*Repo, error) {
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return Open(path)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, fmt.Errorf("creating repo directory: %w", err)
	}
	return Init(path)
}

// Open opens an existing git repository at path.
func Open(path string) (*Repo, error) {
	r, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}
	return &Repo{r: r, path: path}, nil
}

// Clone clones the repository at url into path.
func Clone(url, path string) (*Repo, error) {
	r, err := git.PlainClone(path, false, &git.CloneOptions{
		URL:  url,
		Auth: authForURL(url),
	})
	if err != nil {
		return nil, err
	}
	return &Repo{r: r, path: path}, nil
}

// InitOrOpenBare initializes a bare repository at path, or opens it if it
// already exists. The bool return is true when a new repo was created.
// Returns an error if a non-bare repository already exists at path.
func InitOrOpenBare(path string) (*Repo, bool, error) {
	// A non-bare repo stores git data in a .git subdirectory. go-git's bare
	// init does not detect this as a conflict, so we catch it first.
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return nil, false, fmt.Errorf("repository at %s is not a bare repository; hdf requires a bare repo as push target", path)
	}
	r, err := git.PlainInitWithOptions(path, &git.PlainInitOptions{
		Bare: true,
		InitOptions: git.InitOptions{
			DefaultBranch: plumbing.NewBranchReferenceName("main"),
		},
	})
	if err == nil {
		return &Repo{r: r, path: path}, true, nil
	}
	if errors.Is(err, git.ErrRepositoryAlreadyExists) {
		existing, openErr := git.PlainOpen(path)
		if openErr != nil {
			return nil, false, openErr
		}
		cfg, cfgErr := existing.Config()
		if cfgErr != nil {
			return nil, false, cfgErr
		}
		if !cfg.Core.IsBare {
			return nil, false, fmt.Errorf("repository at %s is not a bare repository; hdf requires a bare repo as push target", path)
		}
		return &Repo{r: existing, path: path}, false, nil
	}
	return nil, false, err
}

// AddRemote adds a named remote to the repository. Silently no-ops if the
// remote already exists.
func (r *Repo) AddRemote(name, url string) error {
	_, err := r.r.CreateRemote(&gitconfig.RemoteConfig{
		Name: name,
		URLs: []string{url},
	})
	if errors.Is(err, git.ErrRemoteExists) {
		existing, remoteErr := r.r.Remote(name)
		if remoteErr != nil {
			return remoteErr
		}
		for _, u := range existing.Config().URLs {
			if u == url {
				return nil
			}
		}
		return fmt.Errorf("remote %q already points to a different URL — remove it manually before running hdf init", name)
	}
	return err
}

// Path returns the local filesystem path of the repository.
func (r *Repo) Path() string {
	return r.path
}

// CreateAndCheckoutBranch creates a new branch and checks it out.
func (r *Repo) CreateAndCheckoutBranch(name string) error {
	w, err := r.r.Worktree()
	if err != nil {
		return err
	}
	return w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(name),
		Create: true,
	})
}

// CurrentBranch returns the short name of the currently checked-out branch.
func (r *Repo) CurrentBranch() (string, error) {
	head, err := r.r.Head()
	if err != nil {
		return "", err
	}
	return head.Name().Short(), nil
}

// CommitFile stages filename (relative to repo root) and creates a commit.
// Returns the commit SHA.
func (r *Repo) CommitFile(filename, message string) (string, error) {
	w, err := r.r.Worktree()
	if err != nil {
		return "", err
	}
	if _, err := w.Add(filename); err != nil {
		return "", err
	}
	hash, err := w.Commit(message, &git.CommitOptions{
		Author: gitAuthor(),
	})
	if err != nil {
		return "", err
	}
	return hash.String(), nil
}

// StageFile stages a single file (repo-relative path) without committing.
func (r *Repo) StageFile(filename string) error {
	w, err := r.r.Worktree()
	if err != nil {
		return err
	}
	_, err = w.Add(filename)
	return err
}

// CommitStaged creates a commit from whatever is currently staged.
// Returns the commit SHA.
func (r *Repo) CommitStaged(message string) (string, error) {
	w, err := r.r.Worktree()
	if err != nil {
		return "", err
	}
	hash, err := w.Commit(message, &git.CommitOptions{
		Author: gitAuthor(),
	})
	if err != nil {
		return "", err
	}
	return hash.String(), nil
}

// gitAuthor loads the user identity from the global git config.
// Falls back to a generic "hdf" identity if the config is unavailable.
func gitAuthor() *object.Signature {
	cfg, err := gitconfig.LoadConfig(gitconfig.GlobalScope)
	if err == nil && cfg.User.Name != "" {
		return &object.Signature{
			Name:  cfg.User.Name,
			Email: cfg.User.Email,
			When:  time.Now(),
		}
	}
	return &object.Signature{
		Name:  "hdf",
		Email: "hdf@localhost",
		When:  time.Now(),
	}
}

// HeadSHA returns the SHA of the current HEAD commit.
func (r *Repo) HeadSHA() (string, error) {
	head, err := r.r.Head()
	if err != nil {
		return "", err
	}
	return head.Hash().String(), nil
}

// ReadFileFromBranch returns the bytes of repoRelPath from the given branch's
// committed tree. Returns nil, nil when the branch or file does not exist.
func (r *Repo) ReadFileFromBranch(branch, repoRelPath string) ([]byte, error) {
	ref, err := r.r.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return nil, nil
		}
		return nil, err
	}
	commit, err := r.r.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}
	file, err := commit.File(repoRelPath)
	if err != nil {
		if errors.Is(err, object.ErrFileNotFound) {
			return nil, nil
		}
		return nil, err
	}
	contents, err := file.Contents()
	if err != nil {
		return nil, err
	}
	return []byte(contents), nil
}

// ReadFileFromRemoteBranch returns the bytes of repoRelPath from the given
// remote tracking branch (e.g. remote="origin", branch="main"). This reads
// refs/remotes/<remote>/<branch>, which is updated by Fetch without touching
// local branch refs. Returns nil, nil when the ref or file does not exist.
func (r *Repo) ReadFileFromRemoteBranch(remote, branch, repoRelPath string) ([]byte, error) {
	ref, err := r.r.Reference(plumbing.NewRemoteReferenceName(remote, branch), true)
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return nil, nil
		}
		return nil, err
	}
	commit, err := r.r.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}
	file, err := commit.File(repoRelPath)
	if err != nil {
		if errors.Is(err, object.ErrFileNotFound) {
			return nil, nil
		}
		return nil, err
	}
	contents, err := file.Contents()
	if err != nil {
		return nil, err
	}
	return []byte(contents), nil
}

// CommitCount returns the total number of commits reachable from HEAD.
func (r *Repo) CommitCount() (int, error) {
	head, err := r.r.Head()
	if err != nil {
		return 0, err
	}
	iter, err := r.r.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return 0, err
	}
	count := 0
	if err := iter.ForEach(func(_ *object.Commit) error {
		count++
		return nil
	}); err != nil {
		return 0, err
	}
	return count, nil
}

// Fetch fetches updates from the remote. Returns nil if already up to date.
func (r *Repo) Fetch() error {
	err := r.r.Fetch(&git.FetchOptions{Auth: authForURL(r.RemoteURL())})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	return err
}

// Push pushes the named branch to the remote. Returns nil when already up to date.
// Returns ErrNonFastForwardUpdate when the remote rejects the push as non-fast-forward.
func (r *Repo) Push(branch string) error {
	err := r.r.Push(&git.PushOptions{
		Auth: authForURL(r.RemoteURL()),
		RefSpecs: []gitconfig.RefSpec{
			gitconfig.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch)),
		},
	})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	// go-git does not expose a sentinel for push non-fast-forward errors; detect
	// by message and wrap in our own sentinel so callers can use errors.Is.
	if err != nil && strings.HasPrefix(err.Error(), "non-fast-forward update") {
		return fmt.Errorf("%w: %w", ErrNonFastForwardUpdate, err)
	}
	return err
}

// HasNewCommitsOnMain returns true if the main branch HEAD differs from lastCommitSHA.
func (r *Repo) HasNewCommitsOnMain(lastCommitSHA string) (bool, error) {
	ref, err := r.r.Reference(plumbing.NewBranchReferenceName("main"), true)
	if err != nil {
		return false, err
	}
	return ref.Hash().String() != lastCommitSHA, nil
}

// BranchSHA returns the current HEAD SHA of a local branch.
func (r *Repo) BranchSHA(branch string) (string, error) {
	ref, err := r.r.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		return "", err
	}
	return ref.Hash().String(), nil
}

// RemoteBranchSHA returns the SHA of a remote tracking branch
// (refs/remotes/<remote>/<branch>), updated by Fetch without touching local refs.
func (r *Repo) RemoteBranchSHA(remote, branch string) (string, error) {
	ref, err := r.r.Reference(plumbing.NewRemoteReferenceName(remote, branch), true)
	if err != nil {
		return "", err
	}
	return ref.Hash().String(), nil
}

// ResetBranchToRemote resets the named local branch to match the current
// remote tracking ref without touching the working tree. Used to roll back
// a local branch after a failed push.
func (r *Repo) ResetBranchToRemote(branch, remote string) error {
	remoteRef, err := r.r.Reference(plumbing.NewRemoteReferenceName(remote, branch), true)
	if err != nil {
		return fmt.Errorf("resolving %s/%s: %w", remote, branch, err)
	}
	return r.r.Storer.SetReference(
		plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), remoteRef.Hash()),
	)
}

// HasIncomingCommits returns true when origin/main has commits not yet in HEAD.
func (r *Repo) HasIncomingCommits() (bool, error) {
	remoteRef, err := r.r.Reference(plumbing.NewRemoteReferenceName("origin", "main"), true)
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("resolving origin/main: %w", err)
	}
	head, err := r.r.Head()
	if err != nil {
		return false, fmt.Errorf("resolving HEAD: %w", err)
	}
	if head.Hash() == remoteRef.Hash() {
		return false, nil
	}
	headCommit, err := r.r.CommitObject(head.Hash())
	if err != nil {
		return false, fmt.Errorf("reading HEAD commit: %w", err)
	}
	remoteCommit, err := r.r.CommitObject(remoteRef.Hash())
	if err != nil {
		return false, fmt.Errorf("reading origin/main commit: %w", err)
	}
	bases, err := headCommit.MergeBase(remoteCommit)
	if err != nil {
		return false, fmt.Errorf("computing merge base: %w", err)
	}
	if len(bases) == 0 {
		return true, nil
	}
	// Only bases[0] is inspected. Multiple merge bases can occur in criss-cross
	// merges, but the dotfiles repo always has linear history so this is safe.
	return bases[0].Hash != remoteRef.Hash(), nil
}

// FastForwardFromMain advances the current branch to origin/main.
// Returns an error if the branches have diverged (manual merge required).
func (r *Repo) FastForwardFromMain() error {
	remoteRef, err := r.r.Reference(plumbing.NewRemoteReferenceName("origin", "main"), true)
	if err != nil {
		return fmt.Errorf("resolving origin/main: %w", err)
	}

	head, err := r.r.Head()
	if err != nil {
		return fmt.Errorf("resolving HEAD: %w", err)
	}

	if head.Hash() == remoteRef.Hash() {
		return nil
	}

	headCommit, err := r.r.CommitObject(head.Hash())
	if err != nil {
		return fmt.Errorf("reading HEAD commit: %w", err)
	}
	remoteCommit, err := r.r.CommitObject(remoteRef.Hash())
	if err != nil {
		return fmt.Errorf("reading origin/main commit: %w", err)
	}

	bases, err := headCommit.MergeBase(remoteCommit)
	if err != nil {
		return fmt.Errorf("computing merge base: %w", err)
	}
	if len(bases) == 0 {
		return fmt.Errorf("no common ancestor between HEAD and origin/main")
	}
	if bases[0].Hash == remoteRef.Hash() {
		return nil // already at or ahead of origin/main
	}
	if bases[0].Hash != head.Hash() {
		return fmt.Errorf("cannot fast-forward: HEAD and origin/main have diverged; run 'git merge' manually")
	}

	w, err := r.r.Worktree()
	if err != nil {
		return fmt.Errorf("getting worktree: %w", err)
	}
	status, err := w.Status()
	if err != nil {
		return fmt.Errorf("checking worktree status: %w", err)
	}
	if !status.IsClean() {
		return fmt.Errorf("cannot merge: uncommitted changes in your dotfiles repository — commit or stash them first")
	}

	return w.Reset(&git.ResetOptions{
		Commit: remoteRef.Hash(),
		Mode:   git.HardReset,
	})
}

// IsCleanForPromote returns true when the worktree has no uncommitted changes.
func (r *Repo) IsCleanForPromote() (bool, error) {
	w, err := r.r.Worktree()
	if err != nil {
		return false, fmt.Errorf("getting worktree: %w", err)
	}
	status, err := w.Status()
	if err != nil {
		return false, fmt.Errorf("checking status: %w", err)
	}
	return status.IsClean(), nil
}

// MergeIntoBranch fast-forwards targetBranch to the current HEAD.
// Returns an error if the branches have diverged — run 'hdf changes-pull'
// to merge targetBranch into the current branch first, then retry.
func (r *Repo) MergeIntoBranch(targetBranch string) error {
	targetRefName := plumbing.NewBranchReferenceName(targetBranch)
	targetRef, err := r.r.Reference(targetRefName, true)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", targetBranch, err)
	}
	head, err := r.r.Head()
	if err != nil {
		return fmt.Errorf("resolving HEAD: %w", err)
	}
	if head.Hash() == targetRef.Hash() {
		return nil
	}
	headCommit, err := r.r.CommitObject(head.Hash())
	if err != nil {
		return fmt.Errorf("reading HEAD commit: %w", err)
	}
	targetCommit, err := r.r.CommitObject(targetRef.Hash())
	if err != nil {
		return fmt.Errorf("reading %s commit: %w", targetBranch, err)
	}
	bases, err := headCommit.MergeBase(targetCommit)
	if err != nil {
		return fmt.Errorf("computing merge base: %w", err)
	}
	if len(bases) == 0 {
		return fmt.Errorf("no common ancestor between HEAD and %s", targetBranch)
	}
	// targetBranch is already at or ahead of HEAD — nothing to do or already merged.
	if bases[0].Hash == head.Hash() {
		return fmt.Errorf("%s is already ahead of the current branch; run 'hdf changes-pull' first", targetBranch)
	}
	// Fast-forward possible: targetBranch is an ancestor of HEAD.
	if bases[0].Hash == targetRef.Hash() {
		return r.r.Storer.SetReference(plumbing.NewHashReference(targetRefName, head.Hash()))
	}
	// Branches have diverged. Refuse if the machine branch deleted any file
	// that still exists on targetBranch — the user must pull first and decide.
	deleted, err := filesMissingFromHeadStillInTarget(r.r, bases[0].TreeHash, headCommit.TreeHash, targetCommit.TreeHash)
	if err != nil {
		return fmt.Errorf("checking for deletions: %w", err)
	}
	if len(deleted) > 0 {
		return fmt.Errorf(
			"cannot promote: %s deleted file(s) that still exist on %s (%s) — run 'hdf changes-pull' first",
			head.Name().Short(), targetBranch, strings.Join(deleted, ", "),
		)
	}
	// Merge the two trees so files promoted by other machines (present in
	// targetBranch but not HEAD) are preserved, while HEAD's versions win.
	mergedTreeHash, err := mergeTrees(r.r, headCommit.TreeHash, targetCommit.TreeHash)
	if err != nil {
		return fmt.Errorf("merging trees: %w", err)
	}
	author := gitAuthor()
	mergeCommit := &object.Commit{
		Author:    *author,
		Committer: *author,
		Message:   fmt.Sprintf("hdf: promote %s into %s\n", head.Name().Short(), targetBranch),
		TreeHash:  mergedTreeHash,
		// parent[0] is targetBranch's previous HEAD so git log --first-parent
		// follows targetBranch's own lineage, not the machine branch.
		ParentHashes: []plumbing.Hash{targetRef.Hash(), head.Hash()},
	}
	obj := r.r.Storer.NewEncodedObject()
	if err := mergeCommit.Encode(obj); err != nil {
		return fmt.Errorf("encoding merge commit: %w", err)
	}
	commitHash, err := r.r.Storer.SetEncodedObject(obj)
	if err != nil {
		return fmt.Errorf("storing merge commit: %w", err)
	}
	return r.r.Storer.SetReference(plumbing.NewHashReference(targetRefName, commitHash))
}

// mergeTrees recursively merges two git trees. Entries in treeA win when both
// trees contain the same name; entries only in treeB are preserved.
//
// This is a two-way merge (no merge base). The potential resurrection problem —
// where a file deleted in treeA but present in treeB gets silently re-added —
// is prevented upstream: MergeIntoBranch calls filesMissingFromHeadStillInTarget
// and refuses to proceed whenever treeA deleted something that is still in treeB.
// By the time mergeTrees runs, all treeB-only entries are genuinely new files
// added by other machines, and preserving them is the correct behaviour.
func mergeTrees(r *git.Repository, treeA, treeB plumbing.Hash) (plumbing.Hash, error) {
	if treeA == treeB || treeB.IsZero() {
		return treeA, nil
	}
	if treeA.IsZero() {
		return treeB, nil
	}

	a, err := r.TreeObject(treeA)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	b, err := r.TreeObject(treeB)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	bEntries := make(map[string]object.TreeEntry, len(b.Entries))
	for _, e := range b.Entries {
		bEntries[e.Name] = e
	}

	merged := make([]object.TreeEntry, 0, len(a.Entries)+len(b.Entries))
	aNames := make(map[string]struct{}, len(a.Entries))

	for _, ea := range a.Entries {
		aNames[ea.Name] = struct{}{}
		entry, err := mergeEntry(r, ea, bEntries)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		merged = append(merged, entry)
	}

	for _, eb := range b.Entries {
		if _, inA := aNames[eb.Name]; !inA {
			merged = append(merged, eb)
		}
	}

	sort.Sort(object.TreeEntrySorter(merged))
	newTree := &object.Tree{Entries: merged}
	obj := r.Storer.NewEncodedObject()
	if err := newTree.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return r.Storer.SetEncodedObject(obj)
}

// mergeEntry resolves a single tree entry from treeA against the entries of
// treeB. treeA wins on conflict; directories are merged recursively.
func mergeEntry(r *git.Repository, ea object.TreeEntry, bEntries map[string]object.TreeEntry) (object.TreeEntry, error) {
	eb, exists := bEntries[ea.Name]
	if !exists || ea.Hash == eb.Hash {
		return ea, nil
	}
	if ea.Mode == filemode.Dir && eb.Mode == filemode.Dir {
		h, err := mergeTrees(r, ea.Hash, eb.Hash)
		if err != nil {
			return ea, err
		}
		ea.Hash = h
	}
	return ea, nil
}

// filesMissingFromHeadStillInTarget returns the repo-relative paths of files
// that existed in baseTree, were removed in headTree, but are still present in
// targetTree. These are files the machine branch intentionally deleted that
// the target branch (main) still holds — a signal to pull first and decide.
func filesMissingFromHeadStillInTarget(r *git.Repository, baseHash, headHash, targetHash plumbing.Hash) ([]string, error) {
	collectPaths := func(hash plumbing.Hash) (map[string]struct{}, error) {
		tree, err := r.TreeObject(hash)
		if err != nil {
			return nil, err
		}
		iter := tree.Files()
		paths := make(map[string]struct{})
		err = iter.ForEach(func(f *object.File) error {
			paths[f.Name] = struct{}{}
			return nil
		})
		return paths, err
	}

	inBase, err := collectPaths(baseHash)
	if err != nil {
		return nil, err
	}
	inHead, err := collectPaths(headHash)
	if err != nil {
		return nil, err
	}
	inTarget, err := collectPaths(targetHash)
	if err != nil {
		return nil, err
	}

	var deleted []string
	for path := range inBase {
		if _, ok := inHead[path]; ok {
			continue
		}
		if _, ok := inTarget[path]; ok {
			deleted = append(deleted, path)
		}
	}
	sort.Strings(deleted)
	return deleted, nil
}

// HasUnpushedCommits returns true if branch has commits that are not reachable from base.
func (r *Repo) HasUnpushedCommits(branch, base string) (bool, error) {
	branchRef, err := r.r.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		return false, err
	}
	baseRef, err := r.r.Reference(plumbing.NewBranchReferenceName(base), true)
	if err != nil {
		return false, err
	}
	if branchRef.Hash() == baseRef.Hash() {
		return false, nil
	}
	// Walk base commits; if branch HEAD appears, it's already merged.
	iter, err := r.r.Log(&git.LogOptions{From: baseRef.Hash()})
	if err != nil {
		return false, err
	}
	merged := false
	err = iter.ForEach(func(c *object.Commit) error {
		if c.Hash == branchRef.Hash() {
			merged = true
			return errStop
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStop) {
		return false, err
	}
	return !merged, nil
}

// BranchFile is a file to be written into a branch via CommitFilesToBranch.
type BranchFile struct {
	RepoRelPath string // slash-separated path relative to repo root
	Content     []byte
}

// CommitFilesToBranch writes the given files directly to the named branch
// using git plumbing without touching the working tree or index. Safe to call
// while a different branch is checked out.
func (r *Repo) CommitFilesToBranch(branch string, files []BranchFile, message string) (string, error) {
	storer := r.r.Storer

	// Resolve current branch state (parent commit + root tree).
	var treeHash, parentHash plumbing.Hash
	ref, err := r.r.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err == nil {
		parentHash = ref.Hash()
		parent, err := r.r.CommitObject(parentHash)
		if err != nil {
			return "", fmt.Errorf("reading parent commit: %w", err)
		}
		treeHash = parent.TreeHash
	}
	// If the branch doesn't exist yet, treeHash is zero (empty tree).

	// Apply each file, updating the root tree incrementally.
	for _, f := range files {
		blobHash, err := storeBlobObject(storer, f.Content)
		if err != nil {
			return "", fmt.Errorf("storing blob %s: %w", f.RepoRelPath, err)
		}
		parts := strings.Split(f.RepoRelPath, "/")
		treeHash, err = upsertTreeEntry(r.r, storer, treeHash, parts, blobHash)
		if err != nil {
			return "", fmt.Errorf("updating tree for %s: %w", f.RepoRelPath, err)
		}
	}

	// Build and store the commit object.
	author := gitAuthor()
	c := &object.Commit{
		Author:    *author,
		Committer: *author,
		Message:   message,
		TreeHash:  treeHash,
	}
	if !parentHash.IsZero() {
		c.ParentHashes = []plumbing.Hash{parentHash}
	}
	obj := storer.NewEncodedObject()
	if err := c.Encode(obj); err != nil {
		return "", fmt.Errorf("encoding commit: %w", err)
	}
	commitHash, err := storer.SetEncodedObject(obj)
	if err != nil {
		return "", fmt.Errorf("storing commit: %w", err)
	}

	// Advance the branch ref.
	newRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), commitHash)
	if err := storer.SetReference(newRef); err != nil {
		return "", fmt.Errorf("updating branch ref: %w", err)
	}
	return commitHash.String(), nil
}

// storeBlobObject writes raw content as a git blob and returns its hash.
func storeBlobObject(storer interface {
	NewEncodedObject() plumbing.EncodedObject
	SetEncodedObject(plumbing.EncodedObject) (plumbing.Hash, error)
}, content []byte,
) (plumbing.Hash, error) {
	obj := storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	obj.SetSize(int64(len(content)))
	w, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if _, err := w.Write(content); err != nil {
		_ = w.Close()
		return plumbing.ZeroHash, err
	}
	if err := w.Close(); err != nil {
		return plumbing.ZeroHash, err
	}
	return storer.SetEncodedObject(obj)
}

// upsertTreeEntry recursively updates a tree so that the file described by
// pathParts ends up pointing to blobHash. Returns the hash of the new root tree.
func upsertTreeEntry(
	gitRepo *git.Repository,
	storer interface {
		NewEncodedObject() plumbing.EncodedObject
		SetEncodedObject(plumbing.EncodedObject) (plumbing.Hash, error)
	},
	treeHash plumbing.Hash,
	pathParts []string,
	blobHash plumbing.Hash,
) (plumbing.Hash, error) {
	// Load existing entries (empty if tree is zero).
	var entries []object.TreeEntry
	if !treeHash.IsZero() {
		tree, err := gitRepo.TreeObject(treeHash)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		entries = make([]object.TreeEntry, len(tree.Entries))
		copy(entries, tree.Entries)
	}

	name := pathParts[0]

	if len(pathParts) == 1 {
		// Leaf: update or insert a blob entry.
		found := false
		for i, e := range entries {
			if e.Name == name {
				entries[i] = object.TreeEntry{Name: name, Mode: filemode.Regular, Hash: blobHash}
				found = true
				break
			}
		}
		if !found {
			entries = append(entries, object.TreeEntry{Name: name, Mode: filemode.Regular, Hash: blobHash})
		}
	} else {
		// Intermediate directory: descend, then update the subtree entry.
		var subHash plumbing.Hash
		for _, e := range entries {
			if e.Name == name {
				subHash = e.Hash
				break
			}
		}
		newSubHash, err := upsertTreeEntry(gitRepo, storer, subHash, pathParts[1:], blobHash)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		found := false
		for i, e := range entries {
			if e.Name == name {
				entries[i] = object.TreeEntry{Name: name, Mode: filemode.Dir, Hash: newSubHash}
				found = true
				break
			}
		}
		if !found {
			entries = append(entries, object.TreeEntry{Name: name, Mode: filemode.Dir, Hash: newSubHash})
		}
	}

	// go-git requires tree entries sorted by treeEntrySortName (dirs get "/").
	sort.Sort(object.TreeEntrySorter(entries))

	// Store the updated tree object.
	tree := &object.Tree{Entries: entries}
	obj := storer.NewEncodedObject()
	if err := tree.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return storer.SetEncodedObject(obj)
}
