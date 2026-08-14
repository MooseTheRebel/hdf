package cli

import (
	"errors"
	"fmt"
	"hdf/config"
	"hdf/daemon"
	"hdf/link"
	"hdf/repo"
	"log"
	"maps"
	"sync"
)

// PreservedFile is a registry entry origin/main holds that this machine has
// never pulled — promote preserves it automatically, no decision needed.
type PreservedFile struct {
	Path string `json:"path"`
}

// DivergedFile is a registry entry whose content differs between this
// machine's branch and origin/main in a way this machine has never seen,
// awaiting an explicit overwrite-or-keep decision.
type DivergedFile struct {
	Path string `json:"path"`
	Diff string `json:"diff"` // unified diff: main's content (old) vs this machine's (new)
}

// PromoteStartInfo is returned by starting a promote operation.
type PromoteStartInfo struct {
	Preserved []PreservedFile `json:"preserved"`
	Diverged  []DivergedFile  `json:"diverged"`
}

// PromoteResult is returned after FinishPromote.
type PromoteResult struct {
	Message string `json:"message"`
}

// pendingPromote carries state across computePromoteStart ->
// computeResolveDivergedFile -> computeFinishPromote. Not JSON-exposed —
// App-internal only.
//
// mu guards preferTheirs: App.ResolveDivergedFile releases App.mu before
// calling computeResolveDivergedFile (so a slow git/state operation for one
// file doesn't block the whole App), which otherwise leaves the shared map
// open to a concurrent-write panic if two resolutions overlap (e.g. a
// double-click). computeFinishPromote also reads preferTheirs while
// building MergeOpts, so it takes the same lock.
type pendingPromote struct {
	repo         *repo.Repo
	cfg          *config.Config
	statePath    string
	mu           sync.Mutex
	preferTheirs map[string]bool
	diverged     []unseenIncoming
}

// computePromoteStartFn, computeResolveDivergedFileFn, and
// computeFinishPromoteFn are indirections over their respective functions
// so App's tests can substitute fakes without touching a real
// repo/filesystem, matching the link/enroll/init seam convention.
var (
	computePromoteStartFn        = computePromoteStart
	computeResolveDivergedFileFn = computeResolveDivergedFile
	computeFinishPromoteFn       = computeFinishPromote
)

// computePromoteStart validates the repo is clean and has a remote, fetches,
// and collects unseen incoming content from origin/main — splitting it into
// files that are simply preserved (no decision needed) and files that
// diverge and need an explicit decision, except those already resolved via
// state.DeclinedOverwrites, which are folded in silently exactly like the
// CLI does (no re-prompt). This is the GUI-oriented counterpart to
// runPromote's setup, fetch, and unseen-incoming-collection phase.
func computePromoteStart(cfgPath, statePath, homeDir string) (*PromoteStartInfo, *pendingPromote, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, nil, fmt.Errorf("loading config: %w", err)
	}
	if cfg.GitPushTarget == "" {
		return nil, nil, fmt.Errorf("cannot promote: no remote configured — promotion has no effect without a shared repository")
	}
	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return nil, nil, fmt.Errorf("opening repo: %w", err)
	}
	if err := ensureOnMachineBranch(r, cfg); err != nil {
		return nil, nil, err
	}
	clean, err := r.IsCleanForPromote()
	if err != nil {
		return nil, nil, fmt.Errorf("checking status: %w", err)
	}
	if !clean {
		return nil, nil, fmt.Errorf("uncommitted changes in the dotfiles repository — run 'hdf changes-push <file>' first")
	}
	if err := r.Fetch(); err != nil {
		return nil, nil, fmt.Errorf("fetching before promote: %w", err)
	}
	unseen, err := collectUnseenIncoming(r, cfg, homeDir)
	if err != nil {
		return nil, nil, fmt.Errorf("checking incoming: %w", err)
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		state = &config.State{}
	}

	info := &PromoteStartInfo{Preserved: []PreservedFile{}, Diverged: []DivergedFile{}}
	pending := &pendingPromote{
		repo:         r,
		cfg:          cfg,
		statePath:    statePath,
		preferTheirs: make(map[string]bool),
	}
	for _, u := range unseen {
		if u.branchBytes == nil {
			info.Preserved = append(info.Preserved, PreservedFile{Path: u.tildePath})
			continue
		}
		mainHash := link.HashBytes(u.mainBytes)
		if state.DeclinedOverwrites[u.relPath] == mainHash {
			// Already reviewed this exact main content and chose to keep it.
			pending.preferTheirs[u.relPath] = true
			continue
		}
		info.Diverged = append(info.Diverged, DivergedFile{
			Path: u.tildePath,
			Diff: daemon.GenerateUnifiedDiff(string(u.branchBytes), string(u.mainBytes)),
		})
		pending.diverged = append(pending.diverged, u)
	}
	return info, pending, nil
}

// computeResolveDivergedFile records the decision for the pending diverged
// file at index: keepMine=true overwrites main with this machine's version
// (clearing any remembered decline); keepMine=false keeps main's version
// (recording the decline so future promotes don't re-prompt for this exact
// content, mirroring reviewUnseenIncoming's persistence).
func computeResolveDivergedFile(p *pendingPromote, index int, keepMine bool) error {
	if index < 0 || index >= len(p.diverged) {
		return fmt.Errorf("resolve diverged file: index %d out of range (0..%d)", index, len(p.diverged)-1)
	}
	item := p.diverged[index]
	if keepMine {
		if err := recordDecline(p.statePath, item.relPath, "", false); err != nil {
			return fmt.Errorf("updating state: %w", err)
		}
		return nil
	}
	p.mu.Lock()
	p.preferTheirs[item.relPath] = true
	p.mu.Unlock()
	mainHash := link.HashBytes(item.mainBytes)
	if err := recordDecline(p.statePath, item.relPath, mainHash, true); err != nil {
		return fmt.Errorf("updating state: %w", err)
	}
	return nil
}

// computeFinishPromote syncs local main to origin, merges the machine
// branch into main, and pushes both — the GUI-oriented counterpart to the
// tail of runPromote.
func computeFinishPromote(p *pendingPromote) (*PromoteResult, error) {
	if err := p.repo.SyncLocalMain("origin"); err != nil {
		return nil, fmt.Errorf("syncing local main to origin: %w", err)
	}
	p.mu.Lock()
	preferTheirs := maps.Clone(p.preferTheirs)
	p.mu.Unlock()
	mergeOpts := &repo.MergeOpts{
		PreferTheirs:   preferTheirs,
		ContentMergers: map[string]repo.ContentMerger{managedTOMLPath: registryUnionMerger},
	}
	if err := p.repo.MergeIntoBranch("main", mergeOpts); err != nil {
		return nil, fmt.Errorf("promoting: %w", err)
	}
	return computePushPromoted(p.repo, p.cfg, p.statePath)
}

// computePushPromoted pushes the machine branch and the merged main,
// rolling local main back (Guard 3) when another machine promoted in the
// race window — the GUI-oriented counterpart to pushPromoted.
func computePushPromoted(r *repo.Repo, cfg *config.Config, statePath string) (*PromoteResult, error) {
	if err := r.Push(cfg.Branch); err != nil {
		return nil, fmt.Errorf("pushing %s: %w", cfg.Branch, err)
	}
	if err := r.Push("main"); err != nil {
		if errors.Is(err, repo.ErrNonFastForwardUpdate) {
			if rollbackErr := r.ResetBranchToRemote("main", "origin"); rollbackErr != nil {
				return nil, fmt.Errorf("promote failed and rollback of local main failed: %w (original: %w)", rollbackErr, err)
			}
			return nil, fmt.Errorf("cannot promote: another machine promoted while you were working — run 'hdf changes-pull' and try again")
		}
		return nil, fmt.Errorf("pushing main: %w", err)
	}
	if mainSHA, shaErr := r.BranchSHA("main"); shaErr == nil {
		if stateErr := recordMainCommit(statePath, mainSHA); stateErr != nil {
			log.Printf("[WARN] computePushPromoted: could not update state file: %v", stateErr)
		}
	}
	return &PromoteResult{Message: fmt.Sprintf("Promoted %s → main and pushed to origin.", cfg.Branch)}, nil
}
