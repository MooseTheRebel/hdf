package cli

import (
	"fmt"
	"hdf/config"
	"hdf/link"
	"hdf/repo"
)

// Status label constants
const (
	statusNoVariant = "no variant for this branch"
	statusMissing   = "missing"
	statusOk        = "ok"
	statusChanged   = "CHANGED (uncommitted)"
)

// fileStatus returns the status label for one managed file on the given
// branch. A file whose variants have no entry for this branch is not managed
// on this machine, so it gets its own state rather than being misreported as
// drift or as missing.
func fileStatus(f config.ManagedFile, branch, homeDir string) string {
	v, res := f.ResolveVariant(branch)
	if res == config.VariantNoBranchMatch {
		return statusNoVariant
	}
	expectedHash := f.Hash
	if res == config.VariantMatch {
		expectedHash = v.Hash
	}
	expanded := config.ExpandPathIn(f.Path, homeDir)
	currentHash, err := link.HashFile(expanded)
	if err != nil {
		return statusMissing
	}
	if currentHash != expectedHash {
		return statusChanged
	}
	return statusOk
}

// FileStatus is one managed file's path and current sync-state label, as
// shown by both `hdf status` and the GUI's status view.
type FileStatus struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// StatusInfo is the full result of computeStatus: everything `hdf status`
// and the GUI's status view need to display. LastSync is pre-formatted
// ("2006-01-02 15:04:05") here so the display format lives in exactly one
// place instead of being duplicated in Go and TypeScript.
type StatusInfo struct {
	GitPushTarget    string       `json:"git_push_target"`
	LocalDotfilesDir string       `json:"local_dotfiles_dir"`
	Branch           string       `json:"branch"`
	LastCommit       string       `json:"last_commit"`
	LastSync         string       `json:"last_sync"`
	Files            []FileStatus `json:"files"`
}

// computeStatus gathers everything needed to describe hdf's current state:
// config, current git branch, last sync info, and each managed file's
// status. It contains no formatting/printing — callers (the CLI's
// statusCmd and the GUI's App.GetStatus) each present the result their own
// way. homeDir is passed in explicitly (rather than read via
// os.UserHomeDir() internally) so this function can be tested against a
// temp directory.
func computeStatus(cfgPath, statePath, homeDir string) (*StatusInfo, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("loading config (run 'hdf init' first): %w", err)
	}

	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return nil, fmt.Errorf("opening repo: %w", err)
	}
	branch, _ := r.CurrentBranch()
	state, _ := config.LoadState(statePath)

	reg, err := config.LoadRegistry(cfg.LocalDotfilesDir)
	if err != nil {
		return nil, fmt.Errorf("loading registry: %w", err)
	}

	files := make([]FileStatus, 0, len(reg.Files))
	for _, f := range reg.Files {
		files = append(files, FileStatus{
			Path:   f.Path,
			Status: fileStatus(f, cfg.Branch, homeDir),
		})
	}

	return &StatusInfo{
		GitPushTarget:    cfg.GitPushTarget,
		LocalDotfilesDir: cfg.LocalDotfilesDir,
		Branch:           branch,
		LastCommit:       state.LastCommit,
		LastSync:         state.LastSync.Format("2006-01-02 15:04:05"),
		Files:            files,
	}, nil
}
