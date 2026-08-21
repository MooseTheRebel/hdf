package cli

import (
	"fmt"
	"hdf/config"
	"hdf/repo"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// computeInitLocalStartFn, computeInitRemoteStartFn,
// computeResolveBranchCollisionFn, and computeFinishInitFn are indirections
// over their respective functions so App's tests can substitute fakes
// without touching a real repo/filesystem, matching the link/enroll seam
// convention.
var (
	computeInitLocalStartFn         = computeInitLocalStart
	computeInitRemoteStartFn        = computeInitRemoteStart
	computeResolveBranchCollisionFn = computeResolveBranchCollision
	computeFinishInitFn             = computeFinishInit
)

// InitBranchCollision describes a machine-branch name collision detected on
// the remote — the user must decide whether to reuse the existing remote
// branch or create a uniquely-suffixed one for this machine.
type InitBranchCollision struct {
	Branch string `json:"branch"`
}

// InitStartInfo is returned by starting init (local or remote). Collision
// is nil when there's nothing to resolve, so the GUI can call FinishInit
// immediately — the same auto-advance shape used by link/enroll when
// there's nothing to review.
type InitStartInfo struct {
	Collision *InitBranchCollision `json:"collision"`
}

// InitResult is returned after FinishInit.
type InitResult struct {
	Message string `json:"message"`
}

// pendingInit carries state across computeInitLocalStart/computeInitRemoteStart
// -> computeResolveBranchCollision -> computeFinishInit. Not JSON-exposed —
// App-internal only.
type pendingInit struct {
	repo     *repo.Repo
	repoPath string
	gitURL   string
	branch   string
	adopted  bool
	headSHA  string
}

// checkNotAlreadyInitialized mirrors runInit's very first guard.
func checkNotAlreadyInitialized(cfgPath string) error {
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("hdf is already initialized (%s).\nEdit that file to change settings, or delete it to run hdf init again", cfgPath)
	}
	return nil
}

// resolveInitPath expands a leading "~/" and resolves relative paths to
// absolute, without the CLI's interactive confirmation prompt — the GUI
// shows the resolved path in a text field before the user submits, so a
// second confirmation would be redundant.
func resolveInitPath(raw string) (string, error) {
	expanded := config.ExpandPath(raw)
	if filepath.IsAbs(expanded) {
		return expanded, nil
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}
	return abs, nil
}

// computeInitLocalStart opens or creates a local repo at repoPath, wires up
// an optional push target (a remote URL or a local path to a bare repo,
// created if needed), computes the initial commit, and checks for a
// machine-branch collision on the remote (if any). This is the GUI-oriented
// counterpart to runInit's "local directory" path plus its branch-collision
// check, minus the interactive prompts.
func computeInitLocalStart(cfgPath, repoPath, pushTarget string) (*InitStartInfo, *pendingInit, error) {
	if err := checkNotAlreadyInitialized(cfgPath); err != nil {
		return nil, nil, err
	}
	resolvedRepoPath, err := resolveInitPath(strings.TrimSpace(repoPath))
	if err != nil {
		return nil, nil, err
	}
	r, err := repo.InitOrOpen(resolvedRepoPath)
	if err != nil {
		return nil, nil, fmt.Errorf("initialising repo at %s: %w", resolvedRepoPath, err)
	}

	gitURL, err := setUpLocalPushTarget(r, resolvedRepoPath, strings.TrimSpace(pushTarget))
	if err != nil {
		return nil, nil, err
	}
	return finishInitStart(r, resolvedRepoPath, gitURL)
}

// setUpLocalPushTarget wires up repo's "origin" remote from a push target
// that is either a remote URL (used as-is) or a local path (a bare repo is
// created/opened there). Returns "" when pushTarget is blank.
func setUpLocalPushTarget(r *repo.Repo, repoPath, pushTarget string) (string, error) {
	if pushTarget == "" {
		return "", nil
	}
	var gitURL string
	if isRemoteURL(pushTarget) {
		gitURL = pushTarget
	} else {
		pushPath, err := resolveInitPath(pushTarget)
		if err != nil {
			return "", err
		}
		resolvedPush := pushPath
		if rp, err := filepath.EvalSymlinks(pushPath); err == nil {
			resolvedPush = rp
		}
		resolvedRepo := repoPath
		if rr, err := filepath.EvalSymlinks(repoPath); err == nil {
			resolvedRepo = rr
		}
		if pushPath == repoPath || resolvedPush == resolvedRepo {
			return "", fmt.Errorf("push target and working copy must differ")
		}
		if _, _, err := repo.InitOrOpenBare(pushPath); err != nil {
			return "", fmt.Errorf("initialising bare repo at %s: %w", pushPath, err)
		}
		gitURL = localPathToFileURL(pushPath)
	}
	if err := r.AddRemote("origin", gitURL); err != nil {
		return "", fmt.Errorf("adding remote: %w", err)
	}
	return gitURL, nil
}

// computeInitRemoteStart clones gitURL into cloneDir (defaulting to
// homeDir/.local/share/hdf/repo, like the CLI does, when cloneDir is
// blank), computes the initial commit, and checks for a machine-branch
// collision on the remote — the GUI-oriented counterpart to runInit's
// "remote repository" path.
func computeInitRemoteStart(cfgPath, homeDir, gitURL, cloneDir string) (*InitStartInfo, *pendingInit, error) {
	if err := checkNotAlreadyInitialized(cfgPath); err != nil {
		return nil, nil, err
	}
	gitURL = strings.TrimSpace(gitURL)
	if gitURL == "" {
		return nil, nil, fmt.Errorf("remote git URL cannot be empty")
	}
	dest := strings.TrimSpace(cloneDir)
	if dest == "" {
		dest = defaultRepoPath(homeDir)
	} else {
		resolved, err := resolveInitPath(dest)
		if err != nil {
			return nil, nil, err
		}
		dest = resolved
	}
	r, err := repo.Clone(gitURL, dest)
	if err != nil {
		return nil, nil, fmt.Errorf("cloning %s: %w", gitURL, err)
	}
	return finishInitStart(r, dest, gitURL)
}

// finishInitStart computes the initial commit and, when a remote is
// configured, checks for a machine-branch collision — the tail shared by
// computeInitLocalStart and computeInitRemoteStart.
func finishInitStart(r *repo.Repo, repoPath, gitURL string) (*InitStartInfo, *pendingInit, error) {
	headSHA, err := ensureInitialCommit(r, repoPath)
	if err != nil {
		return nil, nil, err
	}
	pending := &pendingInit{
		repo:     r,
		repoPath: repoPath,
		gitURL:   gitURL,
		branch:   branchName(),
		headSHA:  headSHA,
	}
	if gitURL == "" {
		return &InitStartInfo{}, pending, nil
	}
	hasCollision, err := detectBranchCollision(r, pending.branch)
	if err != nil {
		return nil, nil, err
	}
	if !hasCollision {
		return &InitStartInfo{}, pending, nil
	}
	return &InitStartInfo{Collision: &InitBranchCollision{Branch: pending.branch}}, pending, nil
}

// detectBranchCollision reports whether the remote already has a branch
// named branch. Mirrors resolveBranchCollision's fetch-and-check phase,
// tolerating a fetch failure the same way the CLI does (treated as no
// collision, since there's nothing further to check).
func detectBranchCollision(r *repo.Repo, branch string) (bool, error) {
	if err := r.Fetch(); err != nil {
		log.Printf("[WARN] detectBranchCollision: could not fetch from remote to check for an existing %q branch: %v", branch, err)
		return false, nil
	}
	has, err := r.RemoteHasBranch("origin", branch)
	if err != nil {
		return false, fmt.Errorf("checking remote for branch %q: %w", branch, err)
	}
	return has, nil
}

// computeResolveBranchCollision applies the user's decision for a detected
// branch collision, mutating p in place. useUnique=true suffixes the branch
// name so it doesn't collide; useUnique=false adopts the existing remote
// branch (falling back to creating it locally if the checkout fails, same
// tolerant fallback as resolveBranchCollision).
func computeResolveBranchCollision(p *pendingInit, useUnique bool) error {
	if useUnique {
		p.branch = p.branch + "-" + randomBranchSuffix()
		p.adopted = false
		return nil
	}
	if err := p.repo.CheckoutTrackingBranch(p.branch, "origin"); err != nil {
		log.Printf("[WARN] computeResolveBranchCollision: could not adopt remote branch %q (%v); creating it locally", p.branch, err)
		p.adopted = false
		return nil
	}
	p.adopted = true
	return nil
}

// computeFinishInit creates/checks out the machine branch (unless already
// adopted) and saves config and state — the GUI-oriented counterpart to the
// tail of runInit.
func computeFinishInit(cfgPath, statePath string, p *pendingInit) (*InitResult, error) {
	if !p.adopted {
		_ = p.repo.CreateAndCheckoutBranch(p.branch) // CLI tolerates "already exists" the same way
	}
	cfg := &config.Config{
		GitPushTarget:    p.gitURL,
		LocalDotfilesDir: p.repoPath,
		Branch:           p.branch,
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		return nil, fmt.Errorf("saving config: %w", err)
	}
	state := &config.State{LastCommit: p.headSHA, LastMainCommit: p.headSHA}
	if err := config.SaveState(statePath, state); err != nil {
		return nil, fmt.Errorf("saving state: %w", err)
	}
	return &InitResult{Message: fmt.Sprintf(
		"hdf initialized (branch %s). Use Enroll to start managing dot files.", p.branch)}, nil
}

// defaultRepoPath returns the CLI's default local-repo path
// (homeDir/.local/share/hdf/repo), used both as computeInitRemoteStart's
// fallback clone destination and as the GUI's pre-filled suggestion. Takes
// homeDir as a parameter rather than calling os.UserHomeDir() internally,
// per CLAUDE.md's rule for testable extracted functions.
func defaultRepoPath(homeDir string) string {
	return filepath.Join(homeDir, ".local", "share", "hdf", "repo")
}
