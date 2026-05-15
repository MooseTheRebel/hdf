package main

import (
	"bufio"
	"embed"
	"fmt"
	"hdf/config"
	"hdf/daemon"
	"hdf/link"
	"hdf/repo"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

var rootCmd = &cobra.Command{
	Use:   "hdf",
	Short: "home-dawt-files: manage your $HOME dot files",
	Long:  `hdf manages dot files by symlinking them from $HOME into a git-backed repository.`,
	Run: func(cmd *cobra.Command, args []string) {
		launchGUI([]string{})
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show the current hdf configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := config.DefaultPath()
		fmt.Printf("Config file: %s\n\n", cfgPath)
		data, err := os.ReadFile(cfgPath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No config found. Run 'hdf init' to get started.")
				return nil
			}
			return err
		}
		fmt.Print(string(data))
		return nil
	},
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize hdf and set up your dot file repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		reader := bufio.NewReader(os.Stdin)

		fmt.Print("Enter git URL or local path for your dot file repository: ")
		gitURL, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading input: %w", err)
		}
		gitURL = strings.TrimSpace(gitURL)
		if gitURL == "" {
			return fmt.Errorf("git URL cannot be empty")
		}

		cfgPath := config.DefaultPath()
		expanded := config.ExpandPath(gitURL)

		var r *repo.Repo
		var repoPath string

		isLocal := strings.HasPrefix(gitURL, "/") || strings.HasPrefix(gitURL, "~/") || strings.HasPrefix(gitURL, ".")
		if isLocal {
			if err := os.MkdirAll(expanded, 0o755); err != nil {
				return fmt.Errorf("creating repo directory: %w", err)
			}
			var err error
			r, err = repo.Open(expanded)
			if err != nil {
				r, err = repo.Init(expanded)
				if err != nil {
					return fmt.Errorf("initializing repo: %w", err)
				}
				fmt.Printf("Initialized new git repository at %s\n", expanded)
			} else {
				fmt.Printf("Opened existing repository at %s\n", expanded)
			}
			repoPath = expanded
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			repoPath = filepath.Join(home, ".local", "share", "hdf", "repo")
			if err := os.MkdirAll(repoPath, 0o755); err != nil {
				return fmt.Errorf("creating local repo directory: %w", err)
			}
			r, err = repo.Clone(gitURL, repoPath)
			if err != nil {
				return fmt.Errorf("cloning repo: %w", err)
			}
			fmt.Printf("Cloned repository to %s\n", repoPath)
		}

		// Ensure there is at least one commit so branching works.
		headSHA, err := r.HeadSHA()
		if err != nil {
			keepFile := filepath.Join(repoPath, ".hdf", ".gitkeep")
			if err := os.MkdirAll(filepath.Dir(keepFile), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(keepFile, []byte(""), 0o644); err != nil {
				return err
			}
			headSHA, err = r.CommitFile(".hdf/.gitkeep", "hdf: initial commit")
			if err != nil {
				return fmt.Errorf("creating initial commit: %w", err)
			}
		}

		hostname, _ := os.Hostname()
		if err := r.CreateAndCheckoutBranch(hostname); err != nil {
			fmt.Printf("Branch %q already exists, continuing.\n", hostname)
		} else {
			fmt.Printf("Created and checked out branch: %s\n", hostname)
		}

		cfg := &config.Config{
			GitURL:   gitURL,
			RepoPath: repoPath,
		}
		if err := config.Save(cfgPath, cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		state := &config.State{LastCommit: headSHA}
		if err := config.SaveState(config.DefaultStatePath(), state); err != nil {
			return fmt.Errorf("saving state: %w", err)
		}
		fmt.Printf("Config saved to %s\n", cfgPath)
		fmt.Println("\nhdf initialized. Use 'hdf enroll <path>' to start managing dot files.")
		return nil
	},
}

var enrollCmd = &cobra.Command{
	Use:   "enroll <path>",
	Short: "Enroll a dot file under hdf management",
	Long:  `Copies the file into the hdf repo, replaces it with a symlink, and commits.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := config.DefaultPath()
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config (run 'hdf init' first): %w", err)
		}

		filePath := args[0]
		expanded := config.ExpandPath(filePath)

		if _, err := os.Stat(expanded); err != nil {
			return fmt.Errorf("file not found: %s", expanded)
		}

		hash, err := link.Enroll(expanded, cfg.RepoPath)
		if err != nil {
			return fmt.Errorf("enrolling %s: %w", filePath, err)
		}

		r, err := repo.Open(cfg.RepoPath)
		if err != nil {
			return fmt.Errorf("opening repo: %w", err)
		}

		repoFilePath, err := link.RepoPathFor(expanded, cfg.RepoPath)
		if err != nil {
			return fmt.Errorf("computing repo path: %w", err)
		}
		relName, err := filepath.Rel(cfg.RepoPath, repoFilePath)
		if err != nil {
			return fmt.Errorf("computing relative path: %w", err)
		}
		sha, err := r.CommitFile(relName, fmt.Sprintf("hdf: enroll %s", filePath))
		if err != nil {
			return fmt.Errorf("committing: %w", err)
		}

		// Normalise the stored path to ~/… form for portability.
		tildeFile := filePath
		if !strings.HasPrefix(filePath, "~/") {
			home, _ := os.UserHomeDir()
			if strings.HasPrefix(expanded, home) {
				tildeFile = "~" + expanded[len(home):]
			}
		}
		for i, f := range cfg.Files {
			if f.Path == tildeFile {
				cfg.Files[i].Hash = hash
				goto save
			}
		}
		cfg.Files = append(cfg.Files, config.ManagedFile{Path: tildeFile, Hash: hash})

	save:
		if err := config.Save(cfgPath, cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		statePath := config.DefaultStatePath()
		state, _ := config.LoadState(statePath)
		state.LastCommit = sha
		if err := config.SaveState(statePath, state); err != nil {
			return fmt.Errorf("saving state: %w", err)
		}
		fmt.Printf("Enrolled %s (commit %s)\n", filePath, sha[:8])
		return nil
	},
}

var linkCmd = &cobra.Command{
	Use:   "link",
	Short: "Re-create symlinks for all managed files",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := config.DefaultPath()
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config (run 'hdf init' first): %w", err)
		}
		for _, f := range cfg.Files {
			expanded := config.ExpandPath(f.Path)
			repoFile, err := link.RepoPathFor(expanded, cfg.RepoPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "link %s: %v\n", f.Path, err)
				continue
			}
			if err := link.Link(expanded, repoFile); err != nil {
				fmt.Fprintf(os.Stderr, "link %s: %v\n", f.Path, err)
				continue
			}
			fmt.Printf("linked %s → %s\n", f.Path, repoFile)
		}
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show managed files and sync state",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := config.DefaultPath()
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config (run 'hdf init' first): %w", err)
		}

		r, err := repo.Open(cfg.RepoPath)
		if err != nil {
			return fmt.Errorf("opening repo: %w", err)
		}
		branch, _ := r.CurrentBranch()
		state, _ := config.LoadState(config.DefaultStatePath())

		fmt.Printf("Git URL:     %s\n", cfg.GitURL)
		fmt.Printf("Repo path:   %s\n", cfg.RepoPath)
		fmt.Printf("Branch:      %s\n", branch)
		fmt.Printf("Last commit: %s\n", state.LastCommit)
		fmt.Printf("Last sync:   %s\n", state.LastSync.Format("2006-01-02 15:04:05"))
		fmt.Printf("\nManaged files (%d):\n", len(cfg.Files))

		for _, f := range cfg.Files {
			expanded := config.ExpandPath(f.Path)
			currentHash, err := link.HashFile(expanded)
			status := "ok"
			if err != nil {
				status = "missing"
			} else if currentHash != f.Hash {
				status = "CHANGED (uncommitted)"
			}
			fmt.Printf("  %-40s %s\n", f.Path, status)
		}
		return nil
	},
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Start the hdf sync daemon",
	Long:  `Runs a background loop that syncs every 30 minutes and sends OS notifications when action is needed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := config.DefaultPath()
		return daemon.Run(cfgPath)
	},
}

var diffCmd = &cobra.Command{
	Use:   "diff [url]",
	Short: "Display a diff in a window",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		diffURLs := []string{
			"https://github.com/spf13/cobra/commit/10d4b48a79be3d4e89e6c45cb59f4d32a3d2ae19.diff",
			"https://github.com/spf13/cobra/commit/88b30ab89da2d0d0abb153818746c5a2d30eccec.diff",
			"https://github.com/spf13/cobra/commit/346d408fe7d4be00ff9481ea4d43c4abb5e5f77d.diff",
		}
		if len(args) > 0 {
			diffURLs = []string{args[0]}
		}
		launchGUI(diffURLs)
	},
}

func launchGUI(diffURLs []string) {
	app := NewApp()
	app.diffURLs = diffURLs

	err := wails.Run(&options.App{
		Title:  "home-dawt-files",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func init() {
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(enrollCmd)
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(daemonCmd)
}
