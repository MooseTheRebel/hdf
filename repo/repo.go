package repo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gogitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

var errStop = errors.New("stop")

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
		if urls := existing.Config().URLs; len(urls) > 0 && urls[0] == url {
			return nil
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

// Push pushes the named branch to the remote.
func (r *Repo) Push(branch string) error {
	return r.r.Push(&git.PushOptions{
		Auth: authForURL(r.RemoteURL()),
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
