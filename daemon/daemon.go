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
	"strings"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"
)

const maxDiffFileSize = 1 << 20 // 1 MB

// addWarning logs msg at WARN level and appends it to state.toml so that
// warnings survive daemon restarts and are readable by the CLI process.
func addWarning(msg, statePath string) {
	notify.LogAndNotify(notify.LevelWarning, "hdf", msg)
	_ = config.UpdateState(statePath, func(state *config.State) error {
		state.PendingWarnings = append(state.PendingWarnings, msg)
		return nil
	})
}

// notifyFailureThrottled sends a failure notification and records a pending
// warning at most once per cooldown window. Sync failures (offline remote,
// broken auth, branch collisions) tend to persist across cycles; without
// throttling they alert on every sync interval and pile up duplicate warnings.
func notifyFailureThrottled(n notify.Notifier, title, msg, statePath string, cooldown time.Duration) {
	throttled := false
	_ = config.UpdateState(statePath, func(s *config.State) error {
		if !s.LastFailureNotifyAt.IsZero() && time.Since(s.LastFailureNotifyAt) < cooldown {
			throttled = true
			return nil
		}
		s.LastFailureNotifyAt = time.Now()
		return nil
	})
	if throttled {
		// Still log for debuggability; skip the user-facing channels.
		notify.LogAndNotify(notify.LevelWarning, title, msg)
		return
	}
	_ = n.Send(title, msg)
	addWarning(msg, statePath)
}

// PendingWarnings returns and clears all warnings written to statePath by the
// daemon since the last call. Called by hdf changes-push/changes-pull (via
// promptPendingWarnings) before proceeding so the user can be prompted to act.
func PendingWarnings(statePath string) ([]string, error) {
	var warnings []string
	err := config.UpdateState(statePath, func(state *config.State) error {
		warnings = state.PendingWarnings
		state.PendingWarnings = nil
		return nil
	})
	if err != nil {
		return nil, err
	}
	return warnings, nil
}

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
		interval, err := syncWithHome(cfgPath, statePath, nil, nil, homeDir)
		if err != nil {
			// Sync errors include transient network issues; use LevelWarning to
			// avoid modal OS alerts on every offline period.
			notify.LogAndNotify(notify.LevelWarning, "hdf sync error", err.Error())
			addWarning(fmt.Sprintf("sync error: %v", err), statePath)
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
	_, err = syncWithHome(cfgPath, statePath, n, nil, homeDir)
	return err
}

// syncWithHome is the testable core of Sync with an injected homeDir.
// n is the standard notifier (drift/unpushed); cn is the critical notifier
// (fetch failures, repo errors). Pass nil to use the package-level defaults.
// Returns the next sync interval derived from SharedSettings.
func syncWithHome(cfgPath, statePath string, n, cn notify.Notifier, homeDir string) (time.Duration, error) {
	if n == nil {
		n = notify.Default
	}
	if cn == nil {
		cn = notify.Critical
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return 0, fmt.Errorf("loading config: %w", err)
	}
	state, err := config.LoadState(statePath)
	if err != nil {
		return 0, fmt.Errorf("loading state: %w", err)
	}
	// Snapshot the loaded values before this cycle mutates state; the final
	// save uses it to detect fields changed by concurrent processes.
	loadedSnapshot := *state
	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return 0, fmt.Errorf("opening repo at %s: %w", cfg.LocalDotfilesDir, err)
	}
	if r.RemoteURL() == "" {
		return 0, fmt.Errorf("no remote configured in %s — re-run 'hdf init' to set a push target", cfg.LocalDotfilesDir)
	}
	if err := r.Fetch(); err != nil {
		// Fetch failures are often transient (offline). Use the standard notifier
		// to avoid intrusive modal alerts, and throttle so an offline laptop is
		// not re-notified every sync cycle. SharedSettings are unreachable
		// before a successful fetch, so use the default cooldown.
		msg := fmt.Sprintf("fetch from remote failed: %v", err)
		notifyFailureThrottled(n, "hdf: remote fetch failed", msg, statePath,
			time.Duration(config.DefaultNotifyCooldownMinutes)*time.Minute)
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
			msg := fmt.Sprintf("%d uncommitted hunk(s) of local drift — run: hdf changes-push <file> or manage as variant", totalHunks)
			_ = n.Send("hdf", msg)
			// Also record as a warning so hdf changes-push/changes-pull can surface it.
			addWarning(msg, statePath)
			state.LastNotifiedAt = time.Now()
		}
	}

	// 4. Push the machine branch so changes-push keeps its name honest.
	// repo.Push already maps "already up to date" to nil.
	if err := r.Push(cfg.Branch); err != nil {
		msg := fmt.Sprintf("push %s failed: %v", cfg.Branch, err)
		notifyFailureThrottled(cn, "hdf: push failed", msg, statePath, cooldown)
		return 0, fmt.Errorf("pushing branch %s: %w", cfg.Branch, err)
	}

	// Persist this cycle's results under the state lock, without clobbering
	// anything other processes wrote during the (long) fetch/push window.
	return interval, config.UpdateState(statePath, mergeSyncResults(&loadedSnapshot, state))
}

// mergeSyncResults returns an UpdateState callback that writes a sync cycle's
// computed results with per-field compare-and-swap semantics: a field is only
// written if its on-disk value still matches what this cycle loaded at the
// start. That way a concurrent writer — e.g. `hdf promote` recording the just-
// pushed main SHA via recordMainCommit — wins over this cycle's stale view,
// and the daemon does not re-notify the user about their own promote.
func mergeSyncResults(loaded, computed *config.State) func(*config.State) error {
	return func(s *config.State) error {
		s.LastSync = time.Now()
		if s.LastMainCommit == loaded.LastMainCommit {
			s.LastMainCommit = computed.LastMainCommit
		}
		if s.LastNotifiedAt.Equal(loaded.LastNotifiedAt) {
			s.LastNotifiedAt = computed.LastNotifiedAt
		}
		return nil
	}
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
		_ = n.Send("hdf", "New commits on main — run 'hdf changes-pull' to review")
	}
	state.LastMainCommit = mainSHA
}

// loadSharedSettings reads SharedSettings from origin/main and returns the
// parsed result. A missing or empty file is treated as "not yet configured"
// and returns defaults with no error. A malformed file is a hard error.
func loadSharedSettings(r *repo.Repo) (*config.SharedSettings, error) {
	ssBytes, err := r.ReadFileFromRemoteBranch("origin", "main", config.SharedSettingsFile)
	if err != nil {
		return nil, fmt.Errorf("reading shared settings from origin/main: %w", err)
	}
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
	variant, res := f.ResolveVariant(cfg.Branch)
	if res == config.VariantNoBranchMatch {
		return 0 // no variant for this branch — not managed here, not drift
	}

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

	registryHash := f.Hash
	if res == config.VariantMatch {
		registryHash = variant.Hash
	}
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

const contextLines = 3

type diffEntry struct {
	op   diffmatchpatch.Operation
	text string
}

func diffToEntries(diffs []diffmatchpatch.Diff) []diffEntry {
	var entries []diffEntry
	for _, d := range diffs {
		for _, text := range strings.Split(strings.TrimSuffix(d.Text, "\n"), "\n") {
			entries = append(entries, diffEntry{d.Type, text})
		}
	}
	return entries
}

func markIncluded(entries []diffEntry) []bool {
	n := len(entries)
	include := make([]bool, n)
	for i, e := range entries {
		if e.op != diffmatchpatch.DiffEqual {
			for j := max(0, i-contextLines); j < min(n, i+contextLines+1); j++ {
				include[j] = true
			}
		}
	}
	return include
}

func hunkLineCounts(entries []diffEntry, start, end int) (oldCount, newCount int) {
	for k := start; k < end; k++ {
		if entries[k].op != diffmatchpatch.DiffInsert {
			oldCount++
		}
		if entries[k].op != diffmatchpatch.DiffDelete {
			newCount++
		}
	}
	return oldCount, newCount
}

func writeHunk(sb *strings.Builder, entries []diffEntry, start, end int, oldLine, newLine *int) {
	oldCount, newCount := hunkLineCounts(entries, start, end)
	fmt.Fprintf(sb, "@@ -%d,%d +%d,%d @@\n", *oldLine, oldCount, *newLine, newCount)
	for k := start; k < end; k++ {
		switch entries[k].op {
		case diffmatchpatch.DiffInsert:
			sb.WriteByte('+')
			*newLine++
		case diffmatchpatch.DiffDelete:
			sb.WriteByte('-')
			*oldLine++
		case diffmatchpatch.DiffEqual:
			sb.WriteByte(' ')
			*oldLine++
			*newLine++
		}
		sb.WriteString(entries[k].text)
		sb.WriteByte('\n')
	}
}

// GenerateUnifiedDiff returns a unified diff between committed and disk content
// with @@ hunk headers and at most contextLines lines of surrounding context.
// Uses the diffmatchpatch engine. Returns "" when the content is identical.
func GenerateUnifiedDiff(committed, disk string) string {
	if committed == disk {
		return ""
	}
	dmp := diffmatchpatch.New()
	a, b, lineMap := dmp.DiffLinesToChars(committed, disk)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, lineMap)

	entries := diffToEntries(diffs)
	include := markIncluded(entries)
	n := len(entries)

	var sb strings.Builder
	oldLine, newLine := 1, 1
	i := 0
	for i < n {
		if !include[i] {
			if entries[i].op != diffmatchpatch.DiffInsert {
				oldLine++
			}
			if entries[i].op != diffmatchpatch.DiffDelete {
				newLine++
			}
			i++
			continue
		}
		hunkStart := i
		for i < n && include[i] {
			i++
		}
		writeHunk(&sb, entries, hunkStart, i, &oldLine, &newLine)
	}
	return sb.String()
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
