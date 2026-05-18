package main

import (
	"bufio"
	"embed"
	"fmt"
	"hdf/config"
	"hdf/daemon"
	"hdf/link"
	"hdf/repo"
	"io"
	"math/rand"
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
		return runInit(cmd.InOrStdin(), config.DefaultPath(), config.DefaultStatePath(), "")
	},
}

// localPathToFileURL converts an absolute local path to a git-compatible
// file:// URL. On Windows, drive-letter paths (e.g. C:\foo) become
// file:///C:/foo; on Unix /foo becomes file:///foo.
func localPathToFileURL(absPath string) string {
	p := strings.ReplaceAll(absPath, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "file://" + p
}

// isRemoteURL reports whether s looks like a remote git URL.
// "file://" is intentionally excluded — users never type it; hdf adds it.
func isRemoteURL(s string) bool {
	return strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "git@") ||
		strings.HasPrefix(s, "ssh://") ||
		strings.HasPrefix(s, "git://")
}

const branchNameChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// branchName returns the machine hostname, falling back to "host-<4 random
// ASCII letters>" if the hostname is unavailable or empty.
func branchName() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		b := make([]byte, 4)
		for i := range b {
			b[i] = branchNameChars[rand.Intn(len(branchNameChars))]
		}
		return "host-" + string(b)
	}
	return h
}

// runInit runs the interactive init wizard.
// cloneDir overrides the destination for remote clones (empty → ~/.local/share/hdf/repo).
func runInit(stdin io.Reader, cfgPath, statePath, cloneDir string) error {
	reader := bufio.NewReader(stdin)

	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("hdf is already initialized (%s).\nEdit that file to change settings, or delete it to run hdf init again", cfgPath)
	}

	fmt.Println("How do you want to store your dot files?")
	fmt.Println("  1) Local directory  (create or open a git repo on this machine)")
	fmt.Println("  2) Remote repository  (clone from GitHub, GitLab, etc.)")
	fmt.Print("\nChoice [1]: ")
	choiceStr, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	choice := strings.TrimSpace(choiceStr)
	if choice == "" {
		choice = "1"
		fmt.Println("  No selection made — defaulting to option 1: Local directory.")
	}

	var r *repo.Repo
	var repoPath, gitURL string

	switch choice {
	case "1":
		// Stage A — working copy (local_dotfiles_dir)
		const defaultLocalPath = "~/.local/share/hdf/repo"
		fmt.Printf("Local working copy path [%s]: ", defaultLocalPath)
		pathInput, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading input: %w", err)
		}
		input := strings.TrimSpace(pathInput)
		if input == "" {
			input = defaultLocalPath
		}
		expanded := config.ExpandPath(input)

		// Relative paths are ambiguous: resolve to absolute and ask the user
		// to confirm before creating anything on disk.
		if !filepath.IsAbs(expanded) {
			abs, err := filepath.Abs(expanded)
			if err != nil {
				return fmt.Errorf("resolving path: %w", err)
			}
			expanded = abs
			fmt.Printf("  → Resolved to: %s\n", expanded)
			fmt.Print("  Confirm? [y/N]: ")
			confirmStr, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("reading confirmation: %w", err)
			}
			if strings.ToLower(strings.TrimSpace(confirmStr)) != "y" {
				return fmt.Errorf("aborted")
			}
		}

		repoPath = expanded
		if err := os.MkdirAll(expanded, 0o755); err != nil {
			return fmt.Errorf("creating repo directory: %w", err)
		}
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

		// Stage B — push target (git_push_target)
		const defaultBarePath = "~/.local/share/hdf/repo-bare"
		fmt.Println()
		fmt.Println("Where should changes be pushed?")
		fmt.Println("  Enter a remote URL (e.g. git@github.com:you/dotfiles.git)")
		fmt.Println("  or a local path for a bare repo (e.g. ~/dotfiles-bare)")
		fmt.Printf("Push target [%s]: ", defaultBarePath)
		pushInput, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading push target: %w", err)
		}
		pushRaw := strings.TrimSpace(pushInput)
		if pushRaw == "" {
			pushRaw = defaultBarePath
		}

		if isRemoteURL(pushRaw) {
			gitURL = pushRaw
		} else {
			pushExpanded := config.ExpandPath(pushRaw)
			if !filepath.IsAbs(pushExpanded) {
				abs, err := filepath.Abs(pushExpanded)
				if err != nil {
					return fmt.Errorf("resolving push target path: %w", err)
				}
				pushExpanded = abs
				fmt.Printf("  → Resolved to: %s\n", pushExpanded)
				fmt.Print("  Confirm? [y/N]: ")
				confirmStr, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("reading confirmation: %w", err)
				}
				if strings.ToLower(strings.TrimSpace(confirmStr)) != "y" {
					return fmt.Errorf("aborted")
				}
			}
			if pushExpanded == repoPath {
				return fmt.Errorf("push target must differ from working copy (%s)", repoPath)
			}
			if err := os.MkdirAll(pushExpanded, 0o755); err != nil {
				return fmt.Errorf("creating bare repo directory: %w", err)
			}
			_, created, err := repo.InitOrOpenBare(pushExpanded)
			if err != nil {
				return fmt.Errorf("initializing bare repo: %w", err)
			}
			if created {
				fmt.Printf("Initialized bare repository at %s\n", pushExpanded)
			} else {
				fmt.Printf("Opened existing bare repository at %s\n", pushExpanded)
			}
			gitURL = localPathToFileURL(pushExpanded)
		}

		if err := r.AddRemote("origin", gitURL); err != nil {
			return fmt.Errorf("wiring origin remote: %w", err)
		}
	case "2":
		fmt.Print("Remote git URL: ")
		urlInput, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading input: %w", err)
		}
		gitURL = strings.TrimSpace(urlInput)
		if gitURL == "" {
			return fmt.Errorf("remote git URL cannot be empty")
		}
		repoPath = cloneDir
		if repoPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			repoPath = filepath.Join(home, ".local", "share", "hdf", "repo")
		}
		if err := os.MkdirAll(repoPath, 0o755); err != nil {
			return fmt.Errorf("creating local repo directory: %w", err)
		}
		r, err = repo.Clone(gitURL, repoPath)
		if err != nil {
			return fmt.Errorf("cloning repo: %w", err)
		}
		fmt.Printf("Cloned repository to %s\n", repoPath)
	default:
		return fmt.Errorf("invalid choice %q: enter 1 or 2", choice)
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

	hostname := branchName()
	if err := r.CreateAndCheckoutBranch(hostname); err != nil {
		fmt.Printf("Branch %q already exists, continuing.\n", hostname)
	} else {
		fmt.Printf("Created and checked out branch: %s\n", hostname)
	}

	cfg := &config.Config{
		GitPushTarget:    gitURL,
		LocalDotfilesDir: repoPath,
		Branch:           hostname,
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	state := &config.State{LastCommit: headSHA}
	if err := config.SaveState(statePath, state); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}
	fmt.Printf("Config saved to %s\n", cfgPath)
	fmt.Println("\nhdf initialized. Use 'hdf enroll <path>' to start managing dot files.")
	return nil
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

		hash, err := link.Enroll(expanded, cfg.LocalDotfilesDir)
		if err != nil {
			return fmt.Errorf("enrolling %s: %w", filePath, err)
		}

		r, err := repo.Open(cfg.LocalDotfilesDir)
		if err != nil {
			return fmt.Errorf("opening repo: %w", err)
		}

		repoFilePath, err := link.RepoPathFor(expanded, cfg.LocalDotfilesDir)
		if err != nil {
			return fmt.Errorf("computing repo path: %w", err)
		}
		relName, err := filepath.Rel(cfg.LocalDotfilesDir, repoFilePath)
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
			if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(expanded, home) {
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
		state, err := config.LoadState(statePath)
		if err != nil {
			return fmt.Errorf("loading state: %w", err)
		}
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
			repoFile, err := link.RepoPathFor(expanded, cfg.LocalDotfilesDir)
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

		r, err := repo.Open(cfg.LocalDotfilesDir)
		if err != nil {
			return fmt.Errorf("opening repo: %w", err)
		}
		branch, _ := r.CurrentBranch()
		state, _ := config.LoadState(config.DefaultStatePath())

		fmt.Printf("Git push target:    %s\n", cfg.GitPushTarget)
		fmt.Printf("Local dotfiles dir: %s\n", cfg.LocalDotfilesDir)
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
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Silence Cobra's built-in error/usage printing on RunE failures so we
	// control the format ourselves and avoid duplicate output.
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true

	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(enrollCmd)
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(daemonCmd)
}
