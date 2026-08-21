package cli

import (
	"fmt"
	"hdf/config"
	"hdf/daemon"
	"hdf/link"
	"hdf/repo"
	"path/filepath"
)

// IncomingFile describes one registry file with content differences
// between the local machine branch and origin/main, awaiting an
// accept/skip decision — the GUI's equivalent of one iteration of the CLI's
// interactive merge-diff prompt.
type IncomingFile struct {
	Path string `json:"path"` // tilde-style path, e.g. "~/.bashrc"
	Diff string `json:"diff"` // unified diff: local (old) vs incoming main (new)
}

// LinkStartInfo is returned by starting a link operation.
type LinkStartInfo struct {
	// Message explains a no-op fetch outcome ("No remote configured;
	// skipping fetch.", "Already up to date."), or is empty when
	// IncomingFiles is non-empty or fetch was skipped entirely (noFetch).
	Message       string         `json:"message"`
	IncomingFiles []IncomingFile `json:"incomingFiles"`
}

// LinkedFile is one managed file's outcome from the final relink pass.
type LinkedFile struct {
	Path  string `json:"path"`
	Error string `json:"error"` // empty on success
}

// computeLinkStartFn, acceptIncomingFileFn, and computeRelinkFn are
// indirections over their respective functions so App's tests can
// substitute fakes without touching a real repo/filesystem, matching the
// svcInstall/runDaemon seam convention used for the daemon App methods.
var (
	computeLinkStartFn   = computeLinkStart
	acceptIncomingFileFn = acceptIncomingFile
	computeRelinkFn      = computeRelink
)

// pendingIncomingFile carries what acceptIncomingFile needs to apply one
// file's accept decision, computed once by computeLinkStart and consumed
// positionally by index. Not JSON-exposed — App-internal only.
type pendingIncomingFile struct {
	relPath   string
	tildePath string
	mainBytes []byte
}

// computeLinkStart opens the repo, ensures the machine branch is checked
// out, loads the registry, and — unless noFetch — fetches from origin and
// computes the list of registry files whose content differs between the
// local machine branch and origin/main. It performs no prompting and
// applies no changes; pair with acceptIncomingFile to apply a decision and
// computeRelink to finish the operation. This is the GUI-oriented
// counterpart to fetchAndShowIncoming's setup and diff-computation phase.
func computeLinkStart(cfgPath, homeDir string, noFetch bool) (*LinkStartInfo, []pendingIncomingFile, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, nil, fmt.Errorf("loading config: %w", err)
	}
	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return nil, nil, fmt.Errorf("opening repo: %w", err)
	}
	if err := ensureOnMachineBranch(r, cfg); err != nil {
		return nil, nil, err
	}
	reg, err := config.LoadRegistry(cfg.LocalDotfilesDir)
	if err != nil {
		return nil, nil, fmt.Errorf("loading registry: %w", err)
	}

	if noFetch {
		return &LinkStartInfo{IncomingFiles: []IncomingFile{}}, nil, nil
	}
	if r.RemoteURL() == "" {
		return &LinkStartInfo{Message: "No remote configured; skipping fetch.", IncomingFiles: []IncomingFile{}}, nil, nil
	}
	if err := r.Fetch(); err != nil {
		return nil, nil, fmt.Errorf("fetching from remote: %w", err)
	}
	hasIncoming, err := r.HasIncomingCommits()
	if err != nil {
		return nil, nil, fmt.Errorf("checking incoming commits: %w", err)
	}
	if !hasIncoming {
		return &LinkStartInfo{Message: "Already up to date.", IncomingFiles: []IncomingFile{}}, nil, nil
	}

	reg, err = remoteRegistry(r, reg)
	if err != nil {
		return nil, nil, err
	}

	incoming, pending, err := computeIncomingDiffs(r, cfg, reg, homeDir)
	if err != nil {
		return nil, nil, err
	}
	if incoming == nil {
		incoming = []IncomingFile{}
	}
	return &LinkStartInfo{IncomingFiles: incoming}, pending, nil
}

// computeIncomingDiffs walks reg.Files and, for each one whose content
// differs between the local machine branch and origin/main, computes its
// diff and the pendingIncomingFile needed to later accept it. Split out of
// computeLinkStart to keep that function's cyclomatic complexity in check.
func computeIncomingDiffs(r *repo.Repo, cfg *config.Config, reg *config.Registry, homeDir string) ([]IncomingFile, []pendingIncomingFile, error) {
	var incoming []IncomingFile
	var pending []pendingIncomingFile
	for _, f := range reg.Files {
		expanded := config.ExpandPathIn(f.Path, homeDir)
		var repoFile string
		if len(f.Variants) > 0 {
			repoFile, _ = resolveRepoPath(f, cfg.Branch, cfg.LocalDotfilesDir)
		} else {
			repoFile, _ = link.RepoPathForHome(expanded, cfg.LocalDotfilesDir, homeDir)
		}
		if repoFile == "" {
			continue
		}
		relPath, err := filepath.Rel(cfg.LocalDotfilesDir, repoFile)
		if err != nil {
			continue
		}
		relPath = filepath.ToSlash(relPath)
		mainBytes, err := r.ReadFileFromRemoteBranch("origin", "main", relPath)
		if err != nil {
			return nil, nil, fmt.Errorf("reading %s from origin/main: %w", relPath, err)
		}
		if mainBytes == nil {
			continue
		}
		branchBytes, err := r.ReadFileFromBranch(cfg.Branch, relPath)
		if err != nil {
			return nil, nil, fmt.Errorf("reading %s from branch %s: %w", relPath, cfg.Branch, err)
		}
		if branchBytes != nil && string(mainBytes) == string(branchBytes) {
			continue
		}
		incoming = append(incoming, IncomingFile{
			Path: f.Path,
			Diff: daemon.GenerateUnifiedDiff(string(branchBytes), string(mainBytes)),
		})
		pending = append(pending, pendingIncomingFile{
			relPath:   relPath,
			tildePath: f.Path,
			mainBytes: mainBytes,
		})
	}
	return incoming, pending, nil
}

// acceptIncomingFile applies one pending incoming file's accept decision:
// writes main's content, updates and commits the registry, on the local
// machine branch. Reopens the repo/config fresh from cfgPath rather than
// reusing state from computeLinkStart, since local git operations are
// cheap and this avoids keeping repo/config objects alive across calls.
func acceptIncomingFile(cfgPath string, item pendingIncomingFile) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return fmt.Errorf("opening repo: %w", err)
	}
	return acceptPromotedFile(r, cfg, item.relPath, item.mainBytes, item.tildePath)
}

// computeRelink reloads the registry fresh (picking up any accepts already
// committed by acceptIncomingFile) and re-creates symlinks for every
// managed file, collecting a per-file result instead of printing — the
// GUI-oriented counterpart to runLink's final loop.
func computeRelink(cfgPath, homeDir string) ([]LinkedFile, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	reg, err := config.LoadRegistry(cfg.LocalDotfilesDir)
	if err != nil {
		return nil, fmt.Errorf("loading registry: %w", err)
	}

	results := []LinkedFile{}
	for _, f := range reg.Files {
		expanded := config.ExpandPathIn(f.Path, homeDir)
		var repoFile string
		var err error
		if len(f.Variants) > 0 {
			repoFile, err = resolveRepoPath(f, cfg.Branch, cfg.LocalDotfilesDir)
		} else {
			repoFile, err = link.RepoPathForHome(expanded, cfg.LocalDotfilesDir, homeDir)
		}
		if err != nil {
			results = append(results, LinkedFile{Path: f.Path, Error: err.Error()})
			continue
		}
		if repoFile == "" {
			results = append(results, LinkedFile{Path: f.Path, Error: fmt.Sprintf(
				"no variant for branch %q — add a variant for this branch to %s to manage the file here",
				cfg.Branch, managedTOMLPath,
			)})
			continue
		}
		if err := link.Link(expanded, repoFile); err != nil {
			results = append(results, LinkedFile{Path: f.Path, Error: err.Error()})
			continue
		}
		results = append(results, LinkedFile{Path: f.Path})
	}
	return results, nil
}
