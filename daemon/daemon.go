package daemon

import (
	"fmt"
	"os"
	"time"

	"hdf/config"
	"hdf/link"
	"hdf/notify"
	"hdf/repo"
)

const syncInterval = 30 * time.Minute

func Run(cfgPath string) error {
	fmt.Fprintf(os.Stderr, "hdf daemon started (sync every %s)\n", syncInterval)
	for {
		if err := Sync(cfgPath); err != nil {
			fmt.Fprintf(os.Stderr, "sync error: %v\n", err)
		}
		time.Sleep(syncInterval)
	}
}

// Sync performs one sync cycle. Exported for testing.
func Sync(cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	r, err := repo.Open(cfg.RepoPath)
	if err != nil {
		return fmt.Errorf("opening repo at %s: %w", cfg.RepoPath, err)
	}

	// 1. Fetch from remote (non-fatal if no remote or no network).
	if err := r.Fetch(); err != nil {
		fmt.Fprintf(os.Stderr, "fetch warning: %v\n", err)
	}

	// 2. Check if main has advanced past our tracked commit.
	if cfg.LastCommit != "" {
		behind, err := r.HasNewCommitsOnMain(cfg.LastCommit)
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

	// 4. Check if hostname branch has commits not in main.
	hostname, _ := os.Hostname()
	unpushed, err := r.HasUnpushedCommits(hostname, "main")
	if err == nil && unpushed {
		_ = notify.Send("hdf", "Unpushed changes — push your branch and merge into main")
	}

	// 5. Update last sync timestamp.
	cfg.LastSync = time.Now()
	return config.Save(cfgPath, cfg)
}
