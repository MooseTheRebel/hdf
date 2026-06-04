package daemon

import (
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

// Run starts the hdf sync daemon, which syncs on a configurable interval indefinitely.
// The interval is read from SharedSettings on the main branch after each sync cycle;
// it defaults to DefaultSyncIntervalMinutes when no shared settings are available.
func Run(cfgPath string) error {
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

	fmt.Fprintf(os.Stderr, "hdf daemon started\n")
	statePath := config.DefaultStatePath()
	for {
		if err := Sync(cfgPath, statePath, nil); err != nil {
			fmt.Fprintf(os.Stderr, "sync error: %v\n", err)
		}
		interval := time.Duration(config.DefaultSyncIntervalMinutes) * time.Minute
		if freshR, err := repo.Open(cfg.LocalDotfilesDir); err == nil {
			if ssBytes, err := freshR.ReadFileFromRemoteBranch("origin", "main", config.SharedSettingsFile); err == nil && len(ssBytes) > 0 {
				if ss, err := config.SharedSettingsFromBytes(ssBytes); err == nil {
					ss.ApplyDefaults()
					interval = time.Duration(ss.SyncIntervalMinutes) * time.Minute
				}
			}
		}
		fmt.Fprintf(os.Stderr, "next sync in %s\n", interval)
		time.Sleep(interval)
	}
}

// Sync performs one sync cycle. Exported for testing.
// Pass nil for n to use notify.Default.
func Sync(cfgPath, statePath string, n notify.Notifier) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home dir: %w", err)
	}
	return syncWithHome(cfgPath, statePath, n, homeDir)
}

// syncWithHome is the testable core of Sync with an injected homeDir.
// Tests use temp directories as homeDir instead of os.UserHomeDir().
func syncWithHome(cfgPath, statePath string, n notify.Notifier, homeDir string) error {
	if n == nil {
		n = notify.Default
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return fmt.Errorf("opening repo at %s: %w", cfg.LocalDotfilesDir, err)
	}

	if r.RemoteURL() == "" {
		return fmt.Errorf("no remote configured in %s — re-run 'hdf init' to set a push target", cfg.LocalDotfilesDir)
	}
	if err := r.Fetch(); err != nil {
		return fmt.Errorf("fetching from remote: %w", err)
	}

	// 1. Check if main has advanced past our tracked commit.
	if state.LastCommit != "" {
		behind, err := r.HasNewCommitsOnMain(state.LastCommit)
		if err == nil && behind {
			_ = n.Send("hdf", "New commits on main — merge into your branch")
		}
	}

	// 2. Read shared settings from origin/main (updated by Fetch above).
	ss := config.DefaultSharedSettings()
	if ssBytes, err := r.ReadFileFromRemoteBranch("origin", "main", config.SharedSettingsFile); err == nil && len(ssBytes) > 0 {
		if parsed, err := config.SharedSettingsFromBytes(ssBytes); err == nil {
			parsed.ApplyDefaults()
			ss = parsed
		}
	}
	threshold := ss.NotifyThreshold
	cooldown := time.Duration(ss.NotifyCooldownMinutes) * time.Minute

	// 3. Count total uncommitted hunks across all managed files.
	reg, err := config.LoadRegistry(cfg.LocalDotfilesDir)
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	totalHunks := 0
	for _, f := range reg.Files {
		expanded := config.ExpandPath(f.Path)

		diskBytes, err := os.ReadFile(expanded)
		if err != nil {
			continue
		}

		registryHash := f.Hash
		for _, v := range f.Variants {
			if v.Branch == cfg.Branch {
				registryHash = v.Hash
				break
			}
		}

		diskHash, _ := link.HashFile(expanded)
		if diskHash == registryHash {
			continue // file is clean, skip diff
		}

		repoFilePath, err := link.RepoPathForHome(expanded, cfg.LocalDotfilesDir, homeDir)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(cfg.LocalDotfilesDir, repoFilePath)
		if err != nil {
			continue
		}
		repoRelPath := filepath.ToSlash(rel)

		committedBytes, err := r.ReadFileFromBranch(cfg.Branch, repoRelPath)
		if err != nil || committedBytes == nil {
			totalHunks++ // new or unreadable file counts as 1 hunk
			continue
		}

		totalHunks += countHunks(string(committedBytes), string(diskBytes))
	}

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
	return config.SaveState(statePath, state)
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
