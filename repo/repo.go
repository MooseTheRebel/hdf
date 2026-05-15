package repo

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var errStop = errors.New("stop")

type Repo struct {
	r    *git.Repository
	path string
}

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

func Open(path string) (*Repo, error) {
	r, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}
	return &Repo{r: r, path: path}, nil
}

func Clone(url, path string) (*Repo, error) {
	r, err := git.PlainClone(path, false, &git.CloneOptions{URL: url})
	if err != nil {
		return nil, err
	}
	return &Repo{r: r, path: path}, nil
}

func (r *Repo) Path() string {
	return r.path
}

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

func (r *Repo) HeadSHA() (string, error) {
	head, err := r.r.Head()
	if err != nil {
		return "", err
	}
	return head.Hash().String(), nil
}

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

func (r *Repo) Fetch() error {
	err := r.r.Fetch(&git.FetchOptions{})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	return err
}

func (r *Repo) Push(branch string) error {
	return r.r.Push(&git.PushOptions{
		RefSpecs: []gitconfig.RefSpec{
			gitconfig.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch)),
		},
	})
}

// HasNewCommitsOnMain returns true if the main branch HEAD differs from lastCommitSHA.
func (r *Repo) HasNewCommitsOnMain(lastCommitSHA string) (bool, error) {
	ref, err := r.r.Reference(plumbing.NewBranchReferenceName("main"), true)
	if err != nil {
		return false, err
	}
	return ref.Hash().String() != lastCommitSHA, nil
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
