package daemon

import (
	"context"
	"fmt"
	"hdf/config"
	"hdf/link"
	"hdf/notify"
	"hdf/repo"
	"os"
	"path/filepath"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"
)

const maxDiffFileSize = 1 << 20 // 1 MB

// Run starts the hdf sync daemon, which syncs on a configurable interval indefinitely.
// The interval is read from SharedSettings on the main branch after each sync cycle;
// it defaults to DefaultSyncIntervalMinutes when no shared settings are available.
// Run returns when ctx is cancelled (graceful shutdown).
func Run(ctx context.Context, cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("hdf is not initialized — run 'hdf init' first (%w)", err)
	}
	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return fmt.Errorf("cannot open dotfiles repo at %s: %w", cfg.LocalDotfilesDir, err)
	}
	if r.RemoteURL() == "" {
		return fmt.Errorf("no remote configured in %s — re-run 'hdf init' to set a push target", cfg.LocalDotfilesDir)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home dir: %w", err)
	}

	fmt.Fprintf(os.Stderr, "hdf daemon started\n")
	statePath := config.DefaultStatePath()
	for {
		interval, err := syncWithHome(cfgPath, statePath, nil, homeDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sync error: %v\n", err)
			interval = time.Duration(config.DefaultSyncIntervalMinutes) * time.Minute
		}
		fmt.Fprintf(os.Stderr, "next sync in %s\n", interval)
		timer := time.NewTimer(interval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
}

// Sync performs one sync cycle. Exported for testing.
// Pass nil for n to use notify.Default.
func Sync(cfgPath, statePath string, n notify.Notifier) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home dir: %w", err)
	}
	_, err = syncWithHome(cfgPath, statePath, n, homeDir)
	return err
}

// syncWithHome is the testable core of Sync with an injected homeDir.
// It returns the next sync interval derived from SharedSettings so the caller
// can reuse it without a redundant repo read.
func syncWithHome(cfgPath, statePath string, n notify.Notifier, homeDir string) (time.Duration, error) {
	if n == nil {
		n = notify.Default
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return 0, fmt.Errorf("loading config: %w", err)
	}
	state, err := config.LoadState(statePath)
	if err != nil {
		return 0, fmt.Errorf("loading state: %w", err)
	}
	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return 0, fmt.Errorf("opening repo at %s: %w", cfg.LocalDotfilesDir, err)
	}
	if r.RemoteURL() == "" {
		return 0, fmt.Errorf("no remote configured in %s — re-run 'hdf init' to set a push target", cfg.LocalDotfilesDir)
	}
	if err := r.Fetch(); err != nil {
		return 0, fmt.Errorf("fetching from remote: %w", err)
	}

	// 1. Check if main has advanced since the last sync cycle.
	checkMainProgress(state, r, n)

	// 2. Read shared settings from origin/main (updated by Fetch above).
	ss, err := loadSharedSettings(r)
	if err != nil {
		return 0, err
	}
	interval := time.Duration(ss.SyncIntervalMinutes) * time.Minute
	threshold := ss.NotifyThreshold
	cooldown := time.Duration(ss.NotifyCooldownMinutes) * time.Minute

	// 3. Count total uncommitted hunks across all managed files.
	reg, err := config.LoadRegistry(cfg.LocalDotfilesDir)
	if err != nil {
		return 0, fmt.Errorf("loading registry: %w", err)
	}
	totalHunks := countDrift(reg, cfg, r, homeDir)

	if totalHunks >= threshold {
		if state.LastNotifiedAt.IsZero() || time.Since(state.LastNotifiedAt) >= cooldown {
			_ = n.Send("hdf",
				fmt.Sprintf("%d uncommitted hunk(s) of local drift — run: hdf enroll <file> or manage as variant", totalHunks))
			state.LastNotifiedAt = time.Now()
		}
	}

	// 4. Check if the machine's branch has commits not in main.
	unpushed, err := r.HasUnpushedCommits(cfg.Branch, "main")
	if err == nil && unpushed {
		_ = n.Send("hdf", "Unpushed changes — push your branch and merge into main")
	}

	state.LastSync = time.Now()
	return interval, config.SaveState(statePath, state)
}

// checkMainProgress notifies if main has advanced past the last known commit and
// updates state.LastMainCommit. It reads origin/main (updated by Fetch) so that
// remote-only advances are detected; falls back to local main when the remote
// tracking ref is absent.
func checkMainProgress(state *config.State, r *repo.Repo, n notify.Notifier) {
	mainSHA, err := r.RemoteBranchSHA("origin", "main")
	if err != nil {
		mainSHA, err = r.BranchSHA("main")
	}
	if err != nil {
		return
	}
	if state.LastMainCommit != "" && state.LastMainCommit != mainSHA {
		_ = n.Send("hdf", "New commits on main — merge into your branch")
	}
	state.LastMainCommit = mainSHA
}

// loadSharedSettings reads SharedSettings from origin/main and returns the
// parsed result. A missing or empty file is treated as "not yet configured"
// and returns defaults with no error. A malformed file is a hard error.
func loadSharedSettings(r *repo.Repo) (*config.SharedSettings, error) {
	ssBytes, _ := r.ReadFileFromRemoteBranch("origin", "main", config.SharedSettingsFile)
	if len(ssBytes) == 0 {
		return config.DefaultSharedSettings(), nil
	}
	parsed, err := config.SharedSettingsFromBytes(ssBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing shared settings: %w", err)
	}
	parsed.ApplyDefaults()
	return parsed, nil
}

// countDrift returns the total number of uncommitted diff hunks across all
// files in the registry. Missing, unreadable, or oversized files each count
// as one hunk.
func countDrift(reg *config.Registry, cfg *config.Config, r *repo.Repo, homeDir string) int {
	total := 0
	for _, f := range reg.Files {
		total += fileDrift(f, cfg, r, homeDir)
	}
	return total
}

// fileDrift returns the hunk count for a single managed file.
func fileDrift(f config.ManagedFile, cfg *config.Config, r *repo.Repo, homeDir string) int {
	expanded := config.ExpandPathIn(f.Path, homeDir)

	info, err := os.Stat(expanded)
	if err != nil {
		return 1 // missing or unreadable counts as drift
	}
	if info.Size() > maxDiffFileSize {
		return 1 // oversized: skip diff, count as one hunk
	}

	diskBytes, err := os.ReadFile(expanded)
	if err != nil {
		return 1
	}

	registryHash := resolveHash(f, cfg.Branch)
	diskHash, _ := link.HashFile(expanded)
	if diskHash == registryHash {
		return 0 // file is clean
	}

	repoFilePath, err := link.RepoPathForHome(expanded, cfg.LocalDotfilesDir, homeDir)
	if err != nil {
		return 0
	}
	rel, err := filepath.Rel(cfg.LocalDotfilesDir, repoFilePath)
	if err != nil {
		return 0
	}

	committedBytes, err := r.ReadFileFromBranch(cfg.Branch, filepath.ToSlash(rel))
	if err != nil || committedBytes == nil {
		return 1 // new or unreadable in repo counts as one hunk
	}

	return countHunks(string(committedBytes), string(diskBytes))
}

// resolveHash returns the hash for f on the given branch, falling back to the
// base hash when no branch-specific variant exists.
func resolveHash(f config.ManagedFile, branch string) string {
	for _, v := range f.Variants {
		if v.Branch == branch {
			return v.Hash
		}
	}
	return f.Hash
}

// countHunks returns the number of contiguous non-Equal diff regions between
// committed and disk content (analogous to git diff hunk count).
// It uses DiffLinesToChars so that each line is an atomic diff unit and
// a single-line change always produces exactly one hunk (no character-level
// refinement that would split a changed line into multiple regions).
func countHunks(committed, disk string) int {
	dmp := diffmatchpatch.New()
	a, b, lines := dmp.DiffLinesToChars(committed, disk)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, lines)
	hunks, inHunk := 0, false
	for _, d := range diffs {
		if d.Type != diffmatchpatch.DiffEqual {
			if !inHunk {
				hunks++
				inHunk = true
			}
		} else {
			inHunk = false
		}
	}
	return hunks
}
