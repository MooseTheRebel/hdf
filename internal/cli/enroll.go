package cli

import (
	"fmt"
	"hdf/config"
	"hdf/daemon"
	"hdf/link"
	"hdf/repo"
	"os"
	"path/filepath"
)

// EnrollStartInfo describes the file about to be enrolled and its diff
// against the currently committed version, awaiting confirmation.
type EnrollStartInfo struct {
	Path      string `json:"path"`      // tilde-style path, e.g. "~/.bashrc"
	IsNewFile bool   `json:"isNewFile"` // true when there's no committed version to diff against
	Diff      string `json:"diff"`      // unified diff of committed (old) vs disk (new); empty for a new or unchanged file
}

// EnrollResult is returned after applying an enroll.
type EnrollResult struct {
	Message string `json:"message"`
}

// pendingEnroll carries what computeApplyEnroll needs to apply an enroll
// decision, computed once by computeEnrollStart. Not JSON-exposed —
// App-internal only.
type pendingEnroll struct {
	expanded  string
	tildeFile string
	relName   string
	filePath  string
}

// computeEnrollStartFn and computeApplyEnrollFn are indirections over their
// respective functions so App's tests can substitute fakes without
// touching a real repo/filesystem, matching the link seam convention.
var (
	computeEnrollStartFn = computeEnrollStart
	computeApplyEnrollFn = computeApplyEnroll
)

// computeEnrollStart validates path, opens the repo, ensures the machine
// branch is checked out, checks the shared ignore list, and computes the
// diff between the file's currently committed content (if any) and its
// content on disk. It performs no prompting and applies no changes; pair
// with computeApplyEnroll to apply the decision. This is the GUI-oriented
// counterpart to runEnroll's setup and diff-computation phase.
func computeEnrollStart(cfgPath, homeDir, filePath string) (*EnrollStartInfo, *pendingEnroll, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, nil, fmt.Errorf("loading config: %w", err)
	}
	expanded, tildeFile, err := expandAndValidate(filePath, homeDir)
	if err != nil {
		return nil, nil, err
	}
	if fi, err := os.Stat(expanded); err == nil && fi.IsDir() {
		return nil, nil, fmt.Errorf("%s is a directory; hdf only supports managing individual files", filePath)
	}

	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return nil, nil, fmt.Errorf("opening repo: %w", err)
	}
	if err := ensureOnMachineBranch(r, cfg); err != nil {
		return nil, nil, err
	}
	ignoredPaths, err := ignoredPathsFromRemote(r)
	if err != nil {
		return nil, nil, err
	}
	if config.IsIgnored(tildeFile, ignoredPaths) {
		return nil, nil, fmt.Errorf("%s matches an ignored path — edit %s on the main branch to override",
			filePath, config.SharedSettingsFile)
	}

	repoFilePath, err := link.RepoPathForHome(expanded, cfg.LocalDotfilesDir, homeDir)
	if err != nil {
		return nil, nil, fmt.Errorf("computing repo path: %w", err)
	}
	relName, err := filepath.Rel(cfg.LocalDotfilesDir, repoFilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("computing relative path: %w", err)
	}
	committedBytes, err := r.ReadFileFromBranch(cfg.Branch, filepath.ToSlash(relName))
	if err != nil {
		return nil, nil, fmt.Errorf("reading committed version of %s: %w", relName, err)
	}
	diskBytes, err := os.ReadFile(expanded)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", expanded, err)
	}

	info := &EnrollStartInfo{Path: tildeFile}
	if committedBytes == nil {
		info.IsNewFile = true
	} else {
		info.Diff = daemon.GenerateUnifiedDiff(string(committedBytes), string(diskBytes))
	}
	pending := &pendingEnroll{
		expanded:  expanded,
		tildeFile: tildeFile,
		relName:   relName,
		filePath:  filePath,
	}
	return info, pending, nil
}

// computeApplyEnroll applies a pending enroll decision: copies the file
// into the repo, updates and commits the registry on the machine branch,
// stubs it into main, and pushes — the GUI-oriented counterpart to
// applyEnroll, returning a result message instead of printing.
//
// Known gap versus the CLI: if main has moved on the remote (another
// machine promoted concurrently), pushBranches silently swallows that
// note via fmt.Println rather than returning it, so the GUI won't surface
// it. The push itself still succeeds — this only affects a diagnostic
// message. Left as-is because pushBranches has its own test asserting on
// stdout output; a low-risk future improvement is to have it return the
// note instead so this function (and the CLI) can decide how to surface it.
func computeApplyEnroll(cfgPath, homeDir, statePath string, p pendingEnroll) (*EnrollResult, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return nil, fmt.Errorf("opening repo: %w", err)
	}

	hash, err := link.EnrollInHome(p.expanded, cfg.LocalDotfilesDir, homeDir)
	if err != nil {
		return nil, fmt.Errorf("enrolling %s: %w", p.filePath, err)
	}
	reg, err := config.LoadRegistry(cfg.LocalDotfilesDir)
	if err != nil {
		return nil, fmt.Errorf("loading registry: %w", err)
	}
	if registryContains(reg, p.tildeFile, hash) {
		return &EnrollResult{Message: fmt.Sprintf("%s is already managed and unchanged", p.tildeFile)}, nil
	}
	upsertRegistryEntry(reg, p.tildeFile, hash)
	if err := config.SaveRegistry(cfg.LocalDotfilesDir, reg); err != nil {
		return nil, fmt.Errorf("saving registry: %w", err)
	}
	sha, err := stageAndCommit(r, p.relName, p.filePath)
	if err != nil {
		return nil, err
	}
	if err := updateMainRegistry(r, p.tildeFile, p.filePath); err != nil {
		return nil, err
	}
	if err := pushBranches(r, cfg); err != nil {
		return nil, err
	}
	if err := config.UpdateState(statePath, func(s *config.State) error {
		s.LastCommit = sha
		return nil
	}); err != nil {
		return nil, fmt.Errorf("saving state: %w", err)
	}
	return &EnrollResult{Message: fmt.Sprintf("Enrolled %s (commit %s)", p.tildeFile, sha[:8])}, nil
}
