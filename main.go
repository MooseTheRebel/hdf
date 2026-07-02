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
		return config.MigrateFilesToRegistry(cfgPath, cfg.LocalDotfilesDir)
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
		cfgPath := config.DefaultPath()
		statePath := config.DefaultStatePath()
		return runInit(os.Stdin, cfgPath, statePath, "")
	},
}

// isYes reports whether s is an affirmative response.
func isYes(s string) bool {
	l := strings.ToLower(strings.TrimSpace(s))
	return l == "y" || l == "yes"
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

const branchNameChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

const managedTOMLPath = ".hdf/managed.toml"

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

// branchName returns a sanitised hostname suitable for use as a git branch name.
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

// isRemoteURL reports whether s looks like a remote git URL.
// "file://" is intentionally excluded — users never type it; hdf adds it.
func isRemoteURL(s string) bool {
	return strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "git@") ||
		strings.HasPrefix(s, "ssh://") ||
		strings.HasPrefix(s, "git://")
}

// resolveAndConfirmPath expands raw to an absolute path and, when it was
// relative, asks the user to confirm. Returns an error containing "aborted"
// when the user declines.
func resolveAndConfirmPath(reader *bufio.Reader, raw string) (string, error) {
	expanded := config.ExpandPath(raw)
	if filepath.IsAbs(expanded) {
		return expanded, nil
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}
	fmt.Printf("  → Resolved to: %s\n", abs)
	fmt.Print("  Confirm? [y/N]: ")
	answer, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading confirmation: %w", err)
	}
	if !isYes(strings.TrimSpace(answer)) {
		return "", fmt.Errorf("aborted")
	}
	return abs, nil
}

func setupLocalRepo(reader *bufio.Reader) (*repo.Repo, string, string, error) {
	home, _ := os.UserHomeDir()
	defaultPath := filepath.Join(home, ".local", "share", "hdf", "repo")
	fmt.Printf("Local repo path [%s]: ", defaultPath)
	pathStr, err := reader.ReadString('\n')
	if err != nil {
		return nil, "", "", fmt.Errorf("reading input: %w", err)
	}
	raw := strings.TrimSpace(pathStr)
	if raw == "" {
		raw = defaultPath
	}
	repoPath, err := resolveAndConfirmPath(reader, raw)
	if err != nil {
		return nil, "", "", err
	}

	fmt.Print("Push target path or remote URL (leave blank to skip): ")
	pushStr, err := reader.ReadString('\n')
	if err != nil {
		return nil, "", "", fmt.Errorf("reading push target: %w", err)
	}
	pushRaw := strings.TrimSpace(pushStr)

	r, err := repo.InitOrOpen(repoPath)
	if err != nil {
		return nil, "", "", fmt.Errorf("initialising repo at %s: %w", repoPath, err)
	}

	if pushRaw == "" {
		return r, repoPath, "", nil
	}

	var gitURL string
	if isRemoteURL(pushRaw) {
		gitURL = pushRaw
	} else {
		pushPath, err := resolveAndConfirmPath(reader, pushRaw)
		if err != nil {
			return nil, "", "", err
		}
		resolvedPush := pushPath
		if rp, err := filepath.EvalSymlinks(pushPath); err == nil {
			resolvedPush = rp
		}
		resolvedRepo := repoPath
		if rr, err := filepath.EvalSymlinks(repoPath); err == nil {
			resolvedRepo = rr
		}
		if pushPath == repoPath || resolvedPush == resolvedRepo {
			return nil, "", "", fmt.Errorf("push target and working copy must differ")
		}
		if _, _, err := repo.InitOrOpenBare(pushPath); err != nil {
			return nil, "", "", fmt.Errorf("initialising bare repo at %s: %w", pushPath, err)
		}
		gitURL = localPathToFileURL(pushPath)
	}

	if err := r.AddRemote("origin", gitURL); err != nil {
		return nil, "", "", fmt.Errorf("adding remote: %w", err)
	}
	return r, repoPath, gitURL, nil
}

func setupRemoteRepo(reader *bufio.Reader, cloneDir string) (*repo.Repo, string, string, error) {
	fmt.Print("Remote repository URL: ")
	urlStr, err := reader.ReadString('\n')
	if err != nil {
		return nil, "", "", fmt.Errorf("reading input: %w", err)
	}
	gitURL := strings.TrimSpace(urlStr)
	if gitURL == "" {
		return nil, "", "", fmt.Errorf("remote git URL cannot be empty")
	}

	dest := cloneDir
	if dest == "" {
		home, _ := os.UserHomeDir()
		dest = filepath.Join(home, ".local", "share", "hdf", "repo")
	}
	fmt.Printf("Cloning %s into %s...\n", gitURL, dest)
	r, err := repo.Clone(gitURL, dest)
	if err != nil {
		return nil, "", "", fmt.Errorf("cloning %s: %w", gitURL, err)
	}
	return r, dest, gitURL, nil
}

func ensureInitialCommit(r *repo.Repo, repoPath string) (string, error) {
	headSHA, err := r.HeadSHA()
	if err == nil {
		return headSHA, nil
	}
	keepFile := filepath.Join(repoPath, ".hdf", ".gitkeep")
	if err := os.MkdirAll(filepath.Dir(keepFile), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(keepFile, []byte(""), 0o644); err != nil {
		return "", err
	}
	headSHA, err = r.CommitFile(".hdf/.gitkeep", "hdf: initial commit")
	if err != nil {
		return "", fmt.Errorf("creating initial commit: %w", err)
	}
	return headSHA, nil
}

// runInit runs the interactive init wizard.
// cloneDir overrides the destination for remote clones (empty -> ~/.local/share/hdf/repo).
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
	headSHA, err := ensureInitialCommit(r, repoPath)
	if err != nil {
		return err
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
	state := &config.State{LastCommit: headSHA, LastMainCommit: headSHA}
	if err := config.SaveState(statePath, state); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}
	fmt.Printf("Config saved to %s\n", cfgPath)
	fmt.Println("\nhdf initialized. Use 'hdf changes-push <path>' to start managing dot files.")
	return nil
}

// registryContains reports whether reg already has an entry for tildeFile
// with exactly the given hash.
func registryContains(reg *config.Registry, tildeFile, hash string) bool {
	for _, f := range reg.Files {
		if f.Path == tildeFile && f.Hash == hash {
			return true
		}
	}
	return false
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
	mainRegBytes, err := r.ReadFileFromBranch("main", managedTOMLPath)
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
		{RepoRelPath: managedTOMLPath, Content: mainRegBytes},
	}, fmt.Sprintf("hdf: register %s baseline", filePath)); err != nil {
		return fmt.Errorf("registering main baseline: %w", err)
	}
	return nil
}

// expandAndValidate expands filePath relative to homeDir, verifies the file
// exists, and returns both the absolute path and its ~/... normalised form.
func expandAndValidate(filePath, homeDir string) (expanded, tildeFile string, err error) {
	expanded = filePath
	if strings.HasPrefix(filePath, "~/") {
		expanded = filepath.Join(homeDir, filePath[2:])
	} else if !filepath.IsAbs(expanded) {
		abs, err := filepath.Abs(expanded)
		if err != nil {
			return "", "", fmt.Errorf("resolving absolute path: %w", err)
		}
		expanded = abs
	}
	if _, err := os.Stat(expanded); err != nil {
		if os.IsNotExist(err) {
			return "", "", fmt.Errorf("file not found: %s", expanded)
		}
		return "", "", fmt.Errorf("cannot access %s: %w", expanded, err)
	}
	resolvedHome := homeDir
	if rh, err := filepath.EvalSymlinks(homeDir); err == nil {
		resolvedHome = rh
	}
	resolvedExpanded := expanded
	dir, file := filepath.Split(expanded)
	if rd, err := filepath.EvalSymlinks(dir); err == nil {
		resolvedExpanded = filepath.Join(rd, file)
	}
	rel, err := filepath.Rel(resolvedHome, resolvedExpanded)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", "", fmt.Errorf("path %s is outside the home directory and cannot be managed", expanded)
	}
	if rel == "." {
		return "", "", fmt.Errorf("path %s is the home directory itself and cannot be managed", expanded)
	}
	tildeFile = "~/" + filepath.ToSlash(rel)
	return expanded, tildeFile, nil
}

// ignoredPathsFromRemote returns the fleet-wide ignored-paths list from
// SharedSettings on origin/main, or the package defaults when the file is absent.
func ignoredPathsFromRemote(r *repo.Repo) ([]string, error) {
	ssBytes, err := r.ReadFileFromRemoteBranch("origin", "main", config.SharedSettingsFile)
	if err != nil {
		return nil, fmt.Errorf("reading shared settings from origin/main: %w", err)
	}
	if len(ssBytes) == 0 {
		return config.DefaultIgnoredPaths, nil
	}
	ss, err := config.SharedSettingsFromBytes(ssBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing shared settings: %w", err)
	}
	ss.ApplyDefaults()
	return ss.IgnoredPaths, nil
}

// stageAndCommit stages relName and the managed registry, then commits.
func stageAndCommit(r *repo.Repo, relName, filePath string) (string, error) {
	if err := r.StageFile(relName); err != nil {
		return "", fmt.Errorf("staging file: %w", err)
	}
	if err := r.StageFile(managedTOMLPath); err != nil {
		return "", fmt.Errorf("staging registry: %w", err)
	}
	sha, err := r.CommitStaged(fmt.Sprintf("hdf: enroll %s", filePath))
	if err != nil {
		return "", fmt.Errorf("committing: %w", err)
	}
	return sha, nil
}

// pushBranches pushes the hostname branch and main when a remote is configured.
func pushBranches(r *repo.Repo, cfg *config.Config) error {
	if cfg.GitPushTarget == "" {
		return nil
	}
	if err := r.Push(cfg.Branch); err != nil {
		return fmt.Errorf("pushing hostname branch: %w", err)
	}
	if err := r.Push("main"); err != nil {
		return fmt.Errorf("pushing main: %w", err)
	}
	return nil
}

// showEnrollDiff prints the diff between committed and disk content and
// optionally prompts for confirmation. Returns an error when the user aborts.
// reader must be the single bufio.Reader for this command invocation.
func showEnrollDiff(committed, disk []byte, filePath string, reader *bufio.Reader, yes bool) error {
	if committed == nil {
		fmt.Printf("new file: %s\n", filePath)
		return nil
	}
	diff := daemon.GenerateUnifiedDiff(string(committed), string(disk))
	if diff == "" {
		return nil
	}
	fmt.Printf("changes to %s:\n", filePath)
	printDiff(diff)
	if yes {
		return nil
	}
	fmt.Print("Enroll these changes? [Y/n]: ")
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)
	if answer != "" && !isYes(answer) {
		return fmt.Errorf("aborted")
	}
	return nil
}

// applyEnroll copies expanded into the repo, commits it, updates the main
// registry, pushes, and records the new commit SHA in state.
func applyEnroll(r *repo.Repo, expanded, tildeFile, relName, filePath, homeDir string, cfg *config.Config, statePath string) error {
	hash, err := link.EnrollInHome(expanded, cfg.LocalDotfilesDir, homeDir)
	if err != nil {
		return fmt.Errorf("enrolling %s: %w", filePath, err)
	}
	reg, err := config.LoadRegistry(cfg.LocalDotfilesDir)
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}
	if registryContains(reg, tildeFile, hash) {
		fmt.Printf("%s is already managed and unchanged\n", filePath)
		return nil
	}
	upsertRegistryEntry(reg, tildeFile, hash)
	if err := config.SaveRegistry(cfg.LocalDotfilesDir, reg); err != nil {
		return fmt.Errorf("saving registry: %w", err)
	}
	sha, err := stageAndCommit(r, relName, filePath)
	if err != nil {
		return err
	}
	if err := updateMainRegistry(r, tildeFile, filepath.ToSlash(relName), filePath); err != nil {
		return err
	}
	if err := pushBranches(r, cfg); err != nil {
		return err
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

func runEnroll(filePath, homeDir string, cfg *config.Config, statePath string, stdin io.Reader, yes bool) error {
	expanded, tildeFile, err := expandAndValidate(filePath, homeDir)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(expanded); err == nil && fi.IsDir() {
		return fmt.Errorf("%s is a directory; hdf only supports managing individual files", filePath)
	}

	reader := bufio.NewReader(stdin)

	// Surface any daemon warnings before proceeding.
	if err := promptPendingWarnings(statePath, reader); err != nil {
		return err
	}

	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return fmt.Errorf("opening repo: %w", err)
	}
	ignoredPaths, err := ignoredPathsFromRemote(r)
	if err != nil {
		return err
	}
	if config.IsIgnored(tildeFile, ignoredPaths) {
		return fmt.Errorf("%s matches an ignored path — edit %s on the main branch to override",
			filePath, config.SharedSettingsFile)
	}

	// Compute the repo-relative path early so we can show a diff before
	// modifying anything on disk.
	repoFilePath, err := link.RepoPathForHome(expanded, cfg.LocalDotfilesDir, homeDir)
	if err != nil {
		return fmt.Errorf("computing repo path: %w", err)
	}
	relName, err := filepath.Rel(cfg.LocalDotfilesDir, repoFilePath)
	if err != nil {
		return fmt.Errorf("computing relative path: %w", err)
	}
	committedBytes, err := r.ReadFileFromBranch(cfg.Branch, filepath.ToSlash(relName))
	if err != nil {
		return fmt.Errorf("reading committed version of %s: %w", relName, err)
	}
	diskBytes, err := os.ReadFile(expanded)
	if err != nil {
		return fmt.Errorf("reading %s: %w", expanded, err)
	}
	if err := showEnrollDiff(committedBytes, diskBytes, filePath, reader, yes); err != nil {
		return err
	}
	return applyEnroll(r, expanded, tildeFile, relName, filePath, homeDir, cfg, statePath)
}

var enrollCmd = &cobra.Command{
	Use:   "changes-push <path>",
	Short: "Commit a dot file to your machine branch and push to remote",
	Long: `Shows a diff of changes, copies the file into the hdf repo, replaces it with a symlink,
commits to your machine branch, and pushes that branch to the remote.
Does not merge into main — that is a deliberate step done via hdf changes-pull after review.`,
	Args:    cobra.ExactArgs(1),
	Aliases: []string{"enroll"},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(config.DefaultPath())
		if err != nil {
			return fmt.Errorf("loading config (run 'hdf init' first): %w", err)
		}
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home directory: %w", err)
		}
		return runEnroll(args[0], homeDir, cfg, config.DefaultStatePath(), os.Stdin, enrollYes)
	},
}

// fetchAndShowIncoming fetches from remote, prints a colored diff for every
// managed file that differs between origin/main and the current branch, and
// returns true when at least one file has incoming changes.
func fetchAndShowIncoming(r *repo.Repo, cfg *config.Config, reg *config.Registry, homeDir string, reader *bufio.Reader) (bool, error) {
	if err := r.Fetch(); err != nil {
		return false, fmt.Errorf("fetching from remote: %w", err)
	}
	// Short-circuit: if origin/main has no commits that aren't already in HEAD
	// (e.g. HEAD is ahead of or equal to main), there is nothing to show.
	// This also avoids false positives when the local branch has diverged and
	// a per-file diff would compare stale main content against newer local content.
	hasIncoming, err := r.HasIncomingCommits()
	if err != nil {
		return false, fmt.Errorf("checking incoming commits: %w", err)
	}
	if !hasIncoming {
		return false, nil
	}
	anyIncoming := false
	for _, f := range reg.Files {
		expanded := config.ExpandPathIn(f.Path, homeDir)
		var repoFile string
		if len(f.Variants) > 0 {
			repoFile, _ = resolveRepoPath(f, cfg.Branch, cfg.LocalDotfilesDir, expanded)
		} else {
			repoFile, _ = link.RepoPathForHome(expanded, cfg.LocalDotfilesDir, homeDir)
		}
		if repoFile == "" {
			continue
		}
		relPath, err := filepath.Rel(cfg.LocalDotfilesDir, repoFile)
		if err != nil {
			continue
		}
		relPath = filepath.ToSlash(relPath)
		mainBytes, err := r.ReadFileFromRemoteBranch("origin", "main", relPath)
		if err != nil {
			return false, fmt.Errorf("reading %s from origin/main: %w", relPath, err)
		}
		// len==0 means main holds an enrollment placeholder committed by
		// changes-push when the file was first registered. The real content
		// lives only on the machine branch at this point, so there is nothing
		// meaningful to diff against — skip to avoid a false "incoming change".
		if len(mainBytes) == 0 {
			continue
		}
		branchBytes, err := r.ReadFileFromBranch(cfg.Branch, relPath)
		if err != nil {
			return false, fmt.Errorf("reading %s from branch %s: %w", relPath, cfg.Branch, err)
		}
		if string(mainBytes) == string(branchBytes) {
			continue
		}
		anyIncoming = true
		fmt.Printf("\n--- %s ---\n", f.Path)
		printDiff(daemon.GenerateUnifiedDiff(string(branchBytes), string(mainBytes)))
		fmt.Printf("Accept main's version of %s? [y/N]: ", f.Path)
		ans, _ := reader.ReadString('\n')
		if isYes(strings.TrimSpace(ans)) {
			if err := acceptPromotedFile(r, cfg, relPath, mainBytes); err != nil {
				fmt.Fprintf(os.Stderr, "accepting %s: %v\n", f.Path, err)
			} else {
				fmt.Printf("Accepted %s from main.\n", f.Path)
			}
		} else {
			fmt.Printf("Skipped %s — keeping local version.\n", f.Path)
		}
	}
	return anyIncoming, nil
}

func acceptPromotedFile(r *repo.Repo, cfg *config.Config, relPath string, mainBytes []byte) error {
	fullPath := filepath.Join(cfg.LocalDotfilesDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	if err := os.WriteFile(fullPath, mainBytes, 0o644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	if _, err := r.CommitFile(relPath, fmt.Sprintf("hdf: accept %s from main", relPath)); err != nil {
		return fmt.Errorf("committing: %w", err)
	}
	return nil
}

// runLink fetches from remote, shows incoming diffs, merges main, and
// re-creates symlinks for all managed files. Pass noFetch=true to skip the
// network operations and only re-create symlinks (offline / test use).
// homeDir is the user's home directory; pass os.UserHomeDir() in production
// and a temp dir in tests.
func runLink(homeDir string, cfg *config.Config, noFetch bool, stdin io.Reader, statePath string) error {
	reader := bufio.NewReader(stdin)

	// Surface any daemon warnings before proceeding.
	if err := promptPendingWarnings(statePath, reader); err != nil {
		return err
	}

	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return fmt.Errorf("opening repo: %w", err)
	}
	reg, err := config.LoadRegistry(cfg.LocalDotfilesDir)
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}
	if !noFetch {
		if r.RemoteURL() == "" {
			fmt.Println("No remote configured; skipping fetch.")
		} else {
			fmt.Println("Fetching from remote...")
			anyIncoming, err := fetchAndShowIncoming(r, cfg, reg, homeDir, reader)
			if err != nil {
				return err
			}
			if !anyIncoming {
				fmt.Println("Already up to date.")
			}
		}
	}

	for _, f := range reg.Files {
		expanded := config.ExpandPathIn(f.Path, homeDir)
		var repoFile string
		var err error
		if len(f.Variants) > 0 {
			repoFile, err = resolveRepoPath(f, cfg.Branch, cfg.LocalDotfilesDir, expanded)
		} else {
			repoFile, err = link.RepoPathForHome(expanded, cfg.LocalDotfilesDir, homeDir)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "link %s: %v\n", f.Path, err)
			continue
		}
		if repoFile == "" {
			fmt.Fprintf(os.Stderr, "link %s: no variant for branch %q — run: hdf changes-push --variant %s\n",
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
}

func runPromote(cfg *config.Config) error {
	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return fmt.Errorf("opening repo: %w", err)
	}
	clean, err := r.IsCleanForPromote()
	if err != nil {
		return fmt.Errorf("checking status: %w", err)
	}
	if !clean {
		return fmt.Errorf("uncommitted changes in the dotfiles repository — run 'hdf changes-push <file>' first")
	}
	hasIncoming, err := r.HasIncomingCommits()
	if err == nil && hasIncoming {
		fmt.Fprintln(os.Stderr, "warning: main has commits you haven't pulled — promoting anyway")
	}
	fmt.Printf("Merging %s into main...\n", cfg.Branch)
	if err := r.MergeIntoBranch("main"); err != nil {
		return fmt.Errorf("promoting: %w", err)
	}
	if err := pushBranches(r, cfg); err != nil {
		return err
	}
	fmt.Printf("Promoted %s → main and pushed.\n", cfg.Branch)
	return nil
}

var promoteCmd = &cobra.Command{
	Use:   "promote",
	Short: "Merge your machine branch into main and push",
	Long:  "Merges the current machine branch into main (fast-forward) and pushes both to origin.\nRun 'hdf changes-pull' first if main has diverged.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := config.DefaultPath()
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("hdf is not initialized — run 'hdf init' first (%w)", err)
		}
		return runPromote(cfg)
	},
}

var linkCmd = &cobra.Command{
	Use:   "changes-pull",
	Short: "Fetch main, show incoming diffs, optionally merge, and re-create symlinks",
	Long: `Fetches origin/main, shows an incoming diff, then prompts to merge now or delay.
Symlinks are always re-created from the current branch state regardless of the choice.
Delaying is an accepted workflow — run hdf changes-pull again when ready to merge.`,
	Aliases: []string{"link"},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := config.DefaultPath()
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config (run 'hdf init' first): %w", err)
		}
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home directory: %w", err)
		}
		return runLink(homeDir, cfg, linkNoFetch, os.Stdin, config.DefaultStatePath())
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
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home directory: %w", err)
		}

		fmt.Printf("\nManaged files (%d):\n", len(reg.Files))

		for _, f := range reg.Files {
			expanded := config.ExpandPathIn(f.Path, homeDir)
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
		return daemon.Run(cmd.Context(), cfgPath)
	},
}

// printDiff writes ANSI-colored unified diff content to stdout.
func printDiff(content string) {
	const (
		reset = "\033[0m"
		bold  = "\033[1m"
		red   = "\033[31m"
		green = "\033[32m"
		cyan  = "\033[36m"
	)
	for _, line := range strings.Split(strings.TrimSuffix(content, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "diff "),
			strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "--- "),
			strings.HasPrefix(line, "+++ "):
			fmt.Printf("%s%s%s\n", bold, line, reset)
		case strings.HasPrefix(line, "@@"):
			fmt.Printf("%s%s%s\n", cyan, line, reset)
		case strings.HasPrefix(line, "+"):
			fmt.Printf("%s%s%s\n", green, line, reset)
		case strings.HasPrefix(line, "-"):
			fmt.Printf("%s%s%s\n", red, line, reset)
		default:
			fmt.Println(line)
		}
	}
}

// promptPendingWarnings checks for daemon warnings persisted to statePath since
// the last push/pull and prompts the user before continuing. Returns an error
// if the user declines, which causes the calling command to abort cleanly.
// reader must be the single bufio.Reader for this command invocation so that
// subsequent prompts in the same call chain read from the same buffer.
func promptPendingWarnings(statePath string, reader *bufio.Reader) error {
	warnings, err := daemon.PendingWarnings(statePath)
	if err != nil {
		return fmt.Errorf("reading pending warnings: %w", err)
	}
	if len(warnings) == 0 {
		return nil
	}
	fmt.Fprintln(os.Stderr, "Warning: The hdf daemon has recorded the following warnings:")
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "   * %s\n", w)
	}
	fmt.Fprint(os.Stderr, "Continue anyway? [y/N]: ")
	answer, _ := reader.ReadString('\n')
	if !isYes(strings.TrimSpace(answer)) {
		return fmt.Errorf("aborted — address the warnings above before continuing")
	}
	return nil
}

// resolveRepoPath returns the repo file path for a managed file with variants,
// choosing the variant matching branch, or empty string if no variant matches.
func resolveRepoPath(f config.ManagedFile, branch, localDotfilesDir, expanded string) (string, error) {
	for _, v := range f.Variants {
		if v.Branch == branch {
			return filepath.Join(localDotfilesDir, v.RepoPath), nil
		}
	}
	return "", nil
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

var (
	enrollYes   bool
	linkNoFetch bool
)

func init() {
	// Silence Cobra's built-in error/usage printing on RunE failures so we
	// control the format ourselves and avoid duplicate output.
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true

	enrollCmd.Flags().BoolVarP(&enrollYes, "yes", "y", false, "Skip the diff confirmation prompt")
	linkCmd.Flags().BoolVar(&linkNoFetch, "no-fetch", false, "Skip fetch and merge from remote; only re-create symlinks")

	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(enrollCmd)
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(promoteCmd)
}
