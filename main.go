package main

import (
	"bufio"
	crand "crypto/rand"
	"embed"
	"fmt"
	"hdf/config"
	"hdf/daemon"
	"hdf/link"
	"hdf/repo"
	"io"
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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := config.DefaultPath()
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return nil // no config yet (e.g. before hdf init), skip migration
		}
		// Best-effort: errors are ignored so a stale or partial config
		// doesn't block subcommands that would surface a clearer message.
		_ = config.MigrateFilesToRegistry(cfgPath, cfg.LocalDotfilesDir)
		return nil
	},
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

// resolveRepoPath returns the absolute repo file path for a managed file on
// the current branch. For variant files it looks up the matching variant;
// returns "" (no error) when the file has variants but none match the branch.
func resolveRepoPath(f config.ManagedFile, branch, repoDir, expandedPath string) (string, error) {
	if len(f.Variants) > 0 {
		for _, v := range f.Variants {
			if v.Branch == branch {
				return filepath.Join(repoDir, v.RepoPath), nil
			}
		}
		return "", nil // variant file, no match for this branch
	}
	return link.RepoPathFor(expandedPath, repoDir)
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

// isYes returns true when the user typed "y" or "yes" (case-insensitive).
func isYes(s string) bool {
	c := strings.ToLower(strings.TrimSpace(s))
	return c == "y" || c == "yes"
}

// sanitizeBranchName replaces any character that is not an ASCII letter,
// digit, or hyphen with a hyphen, then strips leading/trailing hyphens.
func sanitizeBranchName(s string) string {
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, s)
	return strings.Trim(s, "-")
}

// branchName returns the sanitized machine hostname, falling back to
// "host-<4 random ASCII letters>" if the hostname is unavailable or empty.
func branchName() string {
	h, err := os.Hostname()
	if err == nil && h != "" {
		if sanitized := sanitizeBranchName(h); sanitized != "" {
			return sanitized
		}
	}
	b := make([]byte, 4)
	if _, err := crand.Read(b); err != nil {
		return "host-unknown"
	}
	for i := range b {
		b[i] = branchNameChars[int(b[i])%len(branchNameChars)]
	}
	return "host-" + string(b)
}

// setupLocalRepo runs the two-stage local wizard (working copy + push target).
// Returns the opened repo, resolved paths, and the git push URL.
func setupLocalRepo(reader *bufio.Reader) (*repo.Repo, string, string, error) {
	// Stage A — working copy (local_dotfiles_dir)
	const defaultLocalPath = "~/.local/share/hdf/repo"
	fmt.Printf("Local working copy path [%s]: ", defaultLocalPath)
	pathInput, err := reader.ReadString('\n')
	if err != nil {
		return nil, "", "", fmt.Errorf("reading input: %w", err)
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
			return nil, "", "", fmt.Errorf("resolving path: %w", err)
		}
		expanded = abs
		fmt.Printf("  → Resolved to: %s\n", expanded)
		fmt.Print("  Confirm? [y/N]: ")
		confirmStr, err := reader.ReadString('\n')
		if err != nil {
			return nil, "", "", fmt.Errorf("reading confirmation: %w", err)
		}
		if !isYes(confirmStr) {
			return nil, "", "", fmt.Errorf("aborted")
		}
	}

	repoPath := expanded
	if err := os.MkdirAll(expanded, 0o755); err != nil {
		return nil, "", "", fmt.Errorf("creating repo directory: %w", err)
	}
	r, err := repo.Open(expanded)
	if err != nil {
		r, err = repo.Init(expanded)
		if err != nil {
			return nil, "", "", fmt.Errorf("initializing repo: %w", err)
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
		return nil, "", "", fmt.Errorf("reading push target: %w", err)
	}
	pushRaw := strings.TrimSpace(pushInput)
	if pushRaw == "" {
		pushRaw = defaultBarePath
	}

	var gitURL string
	if isRemoteURL(pushRaw) {
		gitURL = pushRaw
	} else {
		pushExpanded := config.ExpandPath(pushRaw)
		if !filepath.IsAbs(pushExpanded) {
			abs, err := filepath.Abs(pushExpanded)
			if err != nil {
				return nil, "", "", fmt.Errorf("resolving push target path: %w", err)
			}
			pushExpanded = abs
			fmt.Printf("  → Resolved to: %s\n", pushExpanded)
			fmt.Print("  Confirm? [y/N]: ")
			confirmStr, err := reader.ReadString('\n')
			if err != nil {
				return nil, "", "", fmt.Errorf("reading confirmation: %w", err)
			}
			if !isYes(confirmStr) {
				return nil, "", "", fmt.Errorf("aborted")
			}
		}
		if pushExpanded == repoPath {
			return nil, "", "", fmt.Errorf("push target must differ from working copy (%s)", repoPath)
		}
		if err := os.MkdirAll(pushExpanded, 0o755); err != nil {
			return nil, "", "", fmt.Errorf("creating bare repo directory: %w", err)
		}
		_, created, err := repo.InitOrOpenBare(pushExpanded)
		if err != nil {
			return nil, "", "", fmt.Errorf("initializing bare repo: %w", err)
		}
		if created {
			fmt.Printf("Initialized bare repository at %s\n", pushExpanded)
		} else {
			fmt.Printf("Opened existing bare repository at %s\n", pushExpanded)
		}
		gitURL = localPathToFileURL(pushExpanded)
	}

	if err := r.AddRemote("origin", gitURL); err != nil {
		return nil, "", "", fmt.Errorf("wiring origin remote: %w", err)
	}
	return r, repoPath, gitURL, nil
}

// setupRemoteRepo clones a remote repository into cloneDir (or the default
// path when cloneDir is empty). Returns the cloned repo, local path, and URL.
func setupRemoteRepo(reader *bufio.Reader, cloneDir string) (*repo.Repo, string, string, error) {
	fmt.Print("Remote git URL: ")
	urlInput, err := reader.ReadString('\n')
	if err != nil {
		return nil, "", "", fmt.Errorf("reading input: %w", err)
	}
	gitURL := strings.TrimSpace(urlInput)
	if gitURL == "" {
		return nil, "", "", fmt.Errorf("remote git URL cannot be empty")
	}
	repoPath := cloneDir
	if repoPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, "", "", err
		}
		repoPath = filepath.Join(home, ".local", "share", "hdf", "repo")
	}
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		return nil, "", "", fmt.Errorf("creating local repo directory: %w", err)
	}
	r, err := repo.Clone(gitURL, repoPath)
	if err != nil {
		return nil, "", "", fmt.Errorf("cloning repo: %w", err)
	}
	fmt.Printf("Cloned repository to %s\n", repoPath)
	return r, repoPath, gitURL, nil
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
		var err error
		r, repoPath, gitURL, err = setupLocalRepo(reader)
		if err != nil {
			return err
		}
	case "2":
		var err error
		r, repoPath, gitURL, err = setupRemoteRepo(reader, cloneDir)
		if err != nil {
			return err
		}
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

// upsertRegistryEntry updates an existing entry's hash in reg, or appends a
// new one. hash is set to "" when called for the main-branch stub.
func upsertRegistryEntry(reg *config.Registry, tildeFile, hash string) {
	for i, f := range reg.Files {
		if f.Path == tildeFile {
			reg.Files[i].Hash = hash
			return
		}
	}
	reg.Files = append(reg.Files, config.ManagedFile{Path: tildeFile, Hash: hash})
}

// updateMainRegistry reads the registry from the main branch, upserts an
// empty stub for tildeFile, and commits the updated registry (plus an empty
// placeholder blob for slashRel) directly to main without touching the
// working tree.
func updateMainRegistry(r *repo.Repo, tildeFile, slashRel, filePath string) error {
	mainRegBytes, err := r.ReadFileFromBranch("main", ".hdf/managed.toml")
	if err != nil {
		return fmt.Errorf("reading main registry: %w", err)
	}
	var mainReg *config.Registry
	if len(mainRegBytes) > 0 {
		mainReg, err = config.RegistryFromBytes(mainRegBytes)
		if err != nil {
			return fmt.Errorf("parsing main registry: %w", err)
		}
	} else {
		mainReg = &config.Registry{}
	}

	upsertRegistryEntry(mainReg, tildeFile, "")

	mainRegBytes, err = config.RegistryToBytes(mainReg)
	if err != nil {
		return fmt.Errorf("serialising main registry: %w", err)
	}
	if _, err := r.CommitFilesToBranch("main", []repo.BranchFile{
		{RepoRelPath: slashRel, Content: []byte{}},
		{RepoRelPath: ".hdf/managed.toml", Content: mainRegBytes},
	}, fmt.Sprintf("hdf: register %s baseline", filePath)); err != nil {
		return fmt.Errorf("registering main baseline: %w", err)
	}
	return nil
}

// runEnroll copies filePath into the hdf repo, commits it to the hostname
// branch, registers an empty stub in main, and pushes both branches.
// homeDir is used as the home directory for path resolution; callers should
// pass os.UserHomeDir() in production and a temp dir in tests.
func runEnroll(filePath, homeDir string, cfg *config.Config, statePath string) error {
	// Expand ~/... using the provided homeDir.
	expanded := filePath
	if strings.HasPrefix(filePath, "~/") {
		expanded = filepath.Join(homeDir, filePath[2:])
	}

	if _, err := os.Stat(expanded); err != nil {
		return fmt.Errorf("file not found: %s", expanded)
	}

	// Normalise to ~/... form early — needed for the ignored-paths guard.
	tildeFile := filePath
	if !strings.HasPrefix(filePath, "~/") {
		if rel, err := filepath.Rel(homeDir, expanded); err == nil && !strings.HasPrefix(rel, "..") {
			tildeFile = "~/" + filepath.ToSlash(rel)
		}
	}

	// Open the repo before touching the filesystem so we can read shared
	// settings and fail fast on a missing/invalid repo.
	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return fmt.Errorf("opening repo: %w", err)
	}

	// Guard: reject files that match the fleet-wide ignored-paths list.
	// Fall back to package defaults when shared settings are unavailable
	// (e.g. before the first sync has fetched origin/main).
	ignoredPaths := config.DefaultIgnoredPaths
	if ssBytes, _ := r.ReadFileFromRemoteBranch("origin", "main", config.SharedSettingsFile); len(ssBytes) > 0 {
		if ss, err := config.SharedSettingsFromBytes(ssBytes); err == nil {
			ss.ApplyDefaults()
			ignoredPaths = ss.IgnoredPaths
		}
	}
	if config.IsIgnored(tildeFile, ignoredPaths) {
		return fmt.Errorf("%s matches an ignored path — edit %s on the main branch to override",
			filePath, config.SharedSettingsFile)
	}

	hash, err := link.EnrollInHome(expanded, cfg.LocalDotfilesDir, homeDir)
	if err != nil {
		return fmt.Errorf("enrolling %s: %w", filePath, err)
	}

	repoFilePath, err := link.RepoPathForHome(expanded, cfg.LocalDotfilesDir, homeDir)
	if err != nil {
		return fmt.Errorf("computing repo path: %w", err)
	}
	relName, err := filepath.Rel(cfg.LocalDotfilesDir, repoFilePath)
	if err != nil {
		return fmt.Errorf("computing relative path: %w", err)
	}

	// Update hostname branch registry with the real content hash.
	reg, err := config.LoadRegistry(cfg.LocalDotfilesDir)
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}
	upsertRegistryEntry(reg, tildeFile, hash)
	if err := config.SaveRegistry(cfg.LocalDotfilesDir, reg); err != nil {
		return fmt.Errorf("saving registry: %w", err)
	}

	// Commit file + registry to hostname branch.
	if err := r.StageFile(relName); err != nil {
		return fmt.Errorf("staging file: %w", err)
	}
	if err := r.StageFile(".hdf/managed.toml"); err != nil {
		return fmt.Errorf("staging registry: %w", err)
	}
	sha, err := r.CommitStaged(fmt.Sprintf("hdf: enroll %s", filePath))
	if err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	// Register an empty stub in main so other machines can discover the file.
	slashRel := filepath.ToSlash(relName)
	if err := updateMainRegistry(r, tildeFile, slashRel, filePath); err != nil {
		return err
	}

	// Push hostname branch then main if a remote is configured.
	if cfg.GitPushTarget != "" {
		if err := r.Push(cfg.Branch); err != nil {
			return fmt.Errorf("pushing hostname branch: %w", err)
		}
		if err := r.Push("main"); err != nil {
			return fmt.Errorf("pushing main: %w", err)
		}
	}

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
}

var enrollCmd = &cobra.Command{
	Use:   "enroll <path>",
	Short: "Enroll a dot file under hdf management",
	Long:  `Copies the file into the hdf repo, replaces it with a symlink, and commits.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(config.DefaultPath())
		if err != nil {
			return fmt.Errorf("loading config (run 'hdf init' first): %w", err)
		}
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home directory: %w", err)
		}
		return runEnroll(args[0], homeDir, cfg, config.DefaultStatePath())
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
		reg, err := config.LoadRegistry(cfg.LocalDotfilesDir)
		if err != nil {
			return fmt.Errorf("loading registry: %w", err)
		}
		for _, f := range reg.Files {
			expanded := config.ExpandPath(f.Path)
			repoFile, err := resolveRepoPath(f, cfg.Branch, cfg.LocalDotfilesDir, expanded)
			if err != nil {
				fmt.Fprintf(os.Stderr, "link %s: %v\n", f.Path, err)
				continue
			}
			if repoFile == "" {
				fmt.Fprintf(os.Stderr, "link %s: no variant for branch %q — run: hdf enroll --variant %s\n",
					f.Path, cfg.Branch, f.Path)
				continue
			}
			if err := link.Link(expanded, repoFile); err != nil {
				fmt.Fprintf(os.Stderr, "link %s: %v\n", f.Path, err)
				continue
			}
			fmt.Printf("linked %s\n", f.Path)
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

		reg, err := config.LoadRegistry(cfg.LocalDotfilesDir)
		if err != nil {
			return fmt.Errorf("loading registry: %w", err)
		}

		fmt.Printf("Git push target:    %s\n", cfg.GitPushTarget)
		fmt.Printf("Local dotfiles dir: %s\n", cfg.LocalDotfilesDir)
		fmt.Printf("Branch:      %s\n", branch)
		fmt.Printf("Last commit: %s\n", state.LastCommit)
		fmt.Printf("Last sync:   %s\n", state.LastSync.Format("2006-01-02 15:04:05"))
		fmt.Printf("\nManaged files (%d):\n", len(reg.Files))

		for _, f := range reg.Files {
			expanded := config.ExpandPath(f.Path)
			expectedHash := f.Hash
			for _, v := range f.Variants {
				if v.Branch == cfg.Branch {
					expectedHash = v.Hash
					break
				}
			}
			currentHash, err := link.HashFile(expanded)
			status := "ok"
			if err != nil {
				status = "missing"
			} else if currentHash != expectedHash {
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
