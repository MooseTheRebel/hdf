package daemon

import (
	"fmt"
	"hdf/config"
	"hdf/link"
	"hdf/notify"
	"hdf/repo"
	"os"
	"time"
)

const syncInterval = 30 * time.Minute

// Run starts the hdf sync daemon, which syncs on a fixed interval indefinitely.
func Run(cfgPath string) error {
	// Pre-flight: catch permanent configuration errors before entering the loop
	// so the daemon fails fast with a clear message rather than spamming warnings.
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

	fmt.Fprintf(os.Stderr, "hdf daemon started (sync every %s)\n", syncInterval)
	statePath := config.DefaultStatePath()
	for {
		if err := Sync(cfgPath, statePath); err != nil {
			fmt.Fprintf(os.Stderr, "sync error: %v\n", err)
		}
		time.Sleep(syncInterval)
	}
}

// Sync performs one sync cycle. Exported for testing.
func Sync(cfgPath, statePath string) error {
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

	// 1. Fetch from remote (non-fatal if no remote or no network).
	if err := r.Fetch(); err != nil {
		fmt.Fprintf(os.Stderr, "fetch warning: %v\n", err)
	}

	// 2. Check if main has advanced past our tracked commit.
	if state.LastCommit != "" {
		behind, err := r.HasNewCommitsOnMain(state.LastCommit)
		if err == nil && behind {
			_ = notify.Send("hdf", "New commits on main — merge into your branch")
		}
	}

	// 3. Compare on-disk hashes against stored hashes.
	for _, f := range cfg.Files {
		expanded := config.ExpandPath(f.Path)
		hash, err := link.HashFile(expanded)
		if err != nil {
			continue
		}
		if hash != f.Hash {
			_ = notify.Send("hdf",
				fmt.Sprintf("Local file changed but not committed — run: hdf enroll %s", f.Path))
		}
	}

	// 4. Check if the machine's branch has commits not in main.
	unpushed, err := r.HasUnpushedCommits(cfg.Branch, "main")
	if err == nil && unpushed {
		_ = notify.Send("hdf", "Unpushed changes — push your branch and merge into main")
	}

	// 5. Persist only the lightweight state (not the full user config).
	state.LastSync = time.Now()
	return config.SaveState(statePath, state)
}
