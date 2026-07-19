package cli

import (
	"bufio"
	crand "crypto/rand"
	"embed"
	"errors"
	"fmt"
	"hdf/config"
	"hdf/daemon"
	"hdf/link"
	"hdf/repo"
	"hdf/svc"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// assets holds the embedded frontend, provided by package main via Execute.
// The embed directive itself must stay next to frontend/ in the repository
// root because embed paths cannot reference parent directories.
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
// HDF_BRANCH overrides the hostname — used in e2e tests to assign unique
// branch names to nodes running on the same machine.
func branchName() string {
	if override := os.Getenv("HDF_BRANCH"); override != "" {
		if sanitized := sanitizeBranchName(override); sanitized != "" {
			return sanitized
		}
	}
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

	hostname, err := setupMachineBranch(reader, r, gitURL)
	if err != nil {
		return err
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

// setupMachineBranch picks this machine's branch name (resolving remote
// collisions interactively when a remote is configured) and checks it out,
// creating it unless it was adopted from the remote.
func setupMachineBranch(reader *bufio.Reader, r *repo.Repo, gitURL string) (string, error) {
	hostname := branchName()
	adopted := false
	if gitURL != "" {
		var err error
		hostname, adopted, err = resolveBranchCollision(reader, r, hostname)
		if err != nil {
			return "", err
		}
	}
	if !adopted {
		if err := r.CreateAndCheckoutBranch(hostname); err != nil {
			fmt.Printf("Branch %q already exists, continuing.\n", hostname)
		} else {
			fmt.Printf("Created and checked out branch: %s\n", hostname)
		}
	}
	return hostname, nil
}

// resolveBranchCollision checks whether the remote already has a branch with
// this machine's name. That is ambiguous: it could be this machine
// re-initializing after a reinstall, or a different machine that happens to
// share the hostname — so the user decides. Returns the branch name to use and
// whether it was already checked out (adopted from the remote).
func resolveBranchCollision(reader *bufio.Reader, r *repo.Repo, branch string) (string, bool, error) {
	if err := r.Fetch(); err != nil {
		fmt.Printf("Warning: could not fetch from remote to check for an existing %q branch: %v\n", branch, err)
		return branch, false, nil
	}
	has, err := r.RemoteHasBranch("origin", branch)
	if err != nil {
		return "", false, fmt.Errorf("checking remote for branch %q: %w", branch, err)
	}
	if !has {
		return branch, false, nil
	}
	fmt.Printf("A branch named %q already exists on the remote.\n", branch)
	fmt.Println("  1) Reuse it (this machine was previously initialized)")
	fmt.Println("  2) Create a unique branch name (a different machine uses this name)")
	fmt.Print("Choice [1]: ")
	ans, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", false, fmt.Errorf("reading input: %w", err)
	}
	if strings.TrimSpace(ans) == "2" {
		unique := branch + "-" + randomBranchSuffix()
		fmt.Printf("Using unique branch name: %s\n", unique)
		return unique, false, nil
	}
	if err := r.CheckoutTrackingBranch(branch, "origin"); err != nil {
		fmt.Printf("Could not adopt remote branch %q (%v); creating it locally.\n", branch, err)
		return branch, false, nil
	}
	fmt.Printf("Reusing existing remote branch: %s\n", branch)
	return branch, true, nil
}

// randomBranchSuffix returns a short random letter suffix for de-duplicating
// branch names across machines that share a hostname.
func randomBranchSuffix() string {
	b := make([]byte, 4)
	if _, err := crand.Read(b); err != nil {
		return "x"
	}
	for i := range b {
		b[i] = branchNameChars[int(b[i])%len(branchNameChars)]
	}
	return string(b)
}

// ensureOnMachineBranch refuses to proceed when the repo has a different
// branch checked out than this machine's configured branch. Commands that
// commit (changes-push, changes-pull accepts, promote) operate on HEAD, so a
// manual git checkout in the dotfiles repo would otherwise route commits to
// the wrong branch — later discarded by promote's SyncLocalMain.
func ensureOnMachineBranch(r *repo.Repo, cfg *config.Config) error {
	cur, err := r.CurrentBranch()
	if err != nil {
		return fmt.Errorf("determining current branch: %w", err)
	}
	if cur != cfg.Branch {
		return fmt.Errorf(
			"dotfiles repo has branch %q checked out, but this machine's branch is %q — run 'git -C %s checkout %s' first",
			cur, cfg.Branch, cfg.LocalDotfilesDir, cfg.Branch)
	}
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
// entry for tildeFile, and commits the updated registry directly to main
// without touching the working tree. File content reaches main only via promote.
func updateMainRegistry(r *repo.Repo, tildeFile, filePath string) error {
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

// unseenIncoming is a registered file whose origin/main content this machine
// has never held on its branch — either not on the branch at all (branchBytes
// nil: another machine's promote this machine hasn't pulled) or diverged from
// content the branch never carried (a newer promote this machine would revert).
type unseenIncoming struct {
	tildePath   string
	relPath     string
	mainBytes   []byte
	branchBytes []byte
}

// collectUnseenIncoming lists registered files where origin/main holds content
// that has never appeared at that path in the machine branch's history. Files
// whose main content this machine produced or previously accepted are skipped,
// so the routine edit → changes-push → promote cycle stays non-interactive.
func collectUnseenIncoming(r *repo.Repo, cfg *config.Config, homeDir string) ([]unseenIncoming, error) {
	regBytes, err := r.ReadFileFromRemoteBranch("origin", "main", managedTOMLPath)
	if err != nil {
		return nil, fmt.Errorf("reading remote registry: %w", err)
	}
	if len(regBytes) == 0 {
		return nil, nil
	}
	reg, err := config.RegistryFromBytes(regBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing remote registry: %w", err)
	}
	if reg == nil {
		return nil, nil
	}
	var pending []unseenIncoming
	for _, f := range reg.Files {
		item, err := unseenIncomingForFile(r, cfg, homeDir, f)
		if err != nil {
			return nil, err
		}
		if item != nil {
			pending = append(pending, *item)
		}
	}
	return pending, nil
}

// unseenIncomingForFile checks one registered file and returns a non-nil
// unseenIncoming when origin/main holds content for it that has never appeared
// at that path in the machine branch's history.
func unseenIncomingForFile(r *repo.Repo, cfg *config.Config, homeDir string, f config.ManagedFile) (*unseenIncoming, error) {
	relSlash, ok := repoRelPathForManagedFile(cfg, homeDir, f)
	if !ok {
		return nil, nil
	}
	mainBytes, err := r.ReadFileFromRemoteBranch("origin", "main", relSlash)
	if err != nil {
		return nil, fmt.Errorf("reading remote file %s: %w", relSlash, err)
	}
	if mainBytes == nil {
		return nil, nil // enrolled but not yet promoted by any machine
	}
	branchBytes, err := r.ReadFileFromBranch(cfg.Branch, relSlash)
	if err != nil {
		return nil, fmt.Errorf("reading local file %s: %w", relSlash, err)
	}
	if branchBytes != nil && string(branchBytes) == string(mainBytes) {
		return nil, nil // synced
	}
	seen, err := r.BranchHistoryHasFileContent(cfg.Branch, relSlash, mainBytes)
	if err != nil {
		return nil, fmt.Errorf("checking branch history for %s: %w", relSlash, err)
	}
	if seen {
		return nil, nil // main holds content this machine produced or accepted before
	}
	return &unseenIncoming{
		tildePath:   f.Path,
		relPath:     relSlash,
		mainBytes:   mainBytes,
		branchBytes: branchBytes,
	}, nil
}

// repoRelPathForManagedFile resolves a registry entry to its repo-relative
// slash path, using the variant-specific path when the file has variants.
// ok is false when the entry does not resolve to a path for this machine.
func repoRelPathForManagedFile(cfg *config.Config, homeDir string, f config.ManagedFile) (string, bool) {
	expanded := config.ExpandPathIn(f.Path, homeDir)
	var repoPath string
	var err error
	if len(f.Variants) > 0 {
		repoPath, err = resolveRepoPath(f, cfg.Branch, cfg.LocalDotfilesDir)
	} else {
		repoPath, err = link.RepoPathForHome(expanded, cfg.LocalDotfilesDir, homeDir)
	}
	if err != nil || repoPath == "" {
		return "", false
	}
	rel, err := filepath.Rel(cfg.LocalDotfilesDir, repoPath)
	if err != nil {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// registryUnionMerger merges conflicting managed.toml blobs during promote by
// unioning entries by path. This machine's entry wins on conflict, but foreign
// entries (files enrolled by other machines this branch hasn't heard of yet)
// and foreign per-branch variants are preserved instead of being clobbered by
// an older wholesale copy of the registry.
func registryUnionMerger(ours, theirs []byte) ([]byte, error) {
	oursReg, err := config.RegistryFromBytes(ours)
	if err != nil {
		return nil, fmt.Errorf("parsing our registry: %w", err)
	}
	theirsReg, err := config.RegistryFromBytes(theirs)
	if err != nil {
		return nil, fmt.Errorf("parsing main's registry: %w", err)
	}
	byPath := make(map[string]config.ManagedFile)
	if theirsReg != nil {
		for _, f := range theirsReg.Files {
			byPath[f.Path] = f
		}
	}
	if oursReg != nil {
		for _, f := range oursReg.Files {
			if existing, ok := byPath[f.Path]; ok {
				f.Variants = unionVariants(f.Variants, existing.Variants)
			}
			byPath[f.Path] = f
		}
	}
	merged := &config.Registry{Files: make([]config.ManagedFile, 0, len(byPath))}
	for _, f := range byPath {
		merged.Files = append(merged.Files, f)
	}
	sort.Slice(merged.Files, func(i, j int) bool {
		return merged.Files[i].Path < merged.Files[j].Path
	})
	return config.RegistryToBytes(merged)
}

// unionVariants merges two variant lists by branch; ours wins when both sides
// carry a variant for the same branch. The result is sorted by branch.
func unionVariants(ours, theirs []config.Variant) []config.Variant {
	byBranch := make(map[string]config.Variant, len(ours)+len(theirs))
	for _, v := range theirs {
		byBranch[v.Branch] = v
	}
	for _, v := range ours {
		byBranch[v.Branch] = v
	}
	merged := make([]config.Variant, 0, len(byBranch))
	for _, v := range byBranch {
		merged = append(merged, v)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].Branch < merged[j].Branch })
	return merged
}

// errPromoteUnreviewed is the refusal returned when promote cannot get an
// explicit answer about unseen incoming content (closed stdin or decline).
func errPromoteUnreviewed() error {
	return fmt.Errorf("cannot promote: main has changes you haven't reviewed — run 'hdf changes-pull' first")
}

// reviewUnseenIncoming walks the user through every unseen incoming file and
// returns the per-path merge overrides for files where main's version should
// win. Files absent from the machine branch need only aggregate consent (the
// merge preserves them); diverged files each get an explicit overwrite prompt.
func reviewUnseenIncoming(pending []unseenIncoming, reader *bufio.Reader, statePath string) (map[string]bool, error) {
	preferTheirs := make(map[string]bool)

	var preserved []unseenIncoming
	var diverged []unseenIncoming
	for _, p := range pending {
		if p.branchBytes == nil {
			preserved = append(preserved, p)
		} else {
			diverged = append(diverged, p)
		}
	}

	if len(preserved) > 0 {
		fmt.Println("main has file(s) promoted by other machines that you haven't pulled:")
		for _, p := range preserved {
			fmt.Printf("  - %s (will be preserved by promote)\n", p.tildePath)
		}
		fmt.Print("Continue promoting? [y/N]: ")
		ans, err := readPromptAnswer(reader)
		if err != nil || !isYes(ans) {
			return nil, errPromoteUnreviewed()
		}
	}

	state, err := config.LoadState(statePath)
	if err != nil {
		state = &config.State{}
	}
	for _, p := range diverged {
		mainHash := link.HashBytes(p.mainBytes)
		if state.DeclinedOverwrites[p.relPath] == mainHash {
			// The user already reviewed exactly this main content and chose to
			// keep it — honor that decision without re-prompting.
			preferTheirs[p.relPath] = true
			fmt.Printf("Keeping main's version of %s (previously declined overwrite).\n", p.tildePath)
			continue
		}
		fmt.Printf("\nmain has a newer version of %s that this machine has never had:\n", p.tildePath)
		printDiff(daemon.GenerateUnifiedDiff(string(p.branchBytes), string(p.mainBytes)))
		fmt.Printf("Overwrite main's newer version of %s with yours? [y/N]: ", p.tildePath)
		ans, err := readPromptAnswer(reader)
		if err != nil {
			return nil, errPromoteUnreviewed()
		}
		if isYes(ans) {
			if updateErr := recordDecline(statePath, p.relPath, "", false); updateErr != nil {
				fmt.Printf("Warning: could not update state file: %v\n", updateErr)
			}
			continue
		}
		preferTheirs[p.relPath] = true
		fmt.Printf("Keeping main's version of %s.\n", p.tildePath)
		if updateErr := recordDecline(statePath, p.relPath, mainHash, true); updateErr != nil {
			fmt.Printf("Warning: could not update state file: %v\n", updateErr)
		}
	}
	return preferTheirs, nil
}

// recordDecline persists (or clears, when add is false) the remembered
// decline for relPath under the state lock.
func recordDecline(statePath, relPath, mainHash string, add bool) error {
	return config.UpdateState(statePath, func(s *config.State) error {
		if !add {
			delete(s.DeclinedOverwrites, relPath)
			return nil
		}
		if s.DeclinedOverwrites == nil {
			s.DeclinedOverwrites = make(map[string]string)
		}
		s.DeclinedOverwrites[relPath] = mainHash
		return nil
	})
}

// readPromptAnswer reads one line from reader, tolerating a final line without
// a trailing newline. A closed stream with no input is an error.
func readPromptAnswer(reader *bufio.Reader) (string, error) {
	ans, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("reading user input: %w", err)
	}
	ans = strings.TrimSpace(ans)
	if errors.Is(err, io.EOF) && ans == "" {
		return "", fmt.Errorf("stdin closed")
	}
	return ans, nil
}

// pushBranches pushes the hostname branch and main when a remote is configured.
// A non-fast-forward rejection on main is tolerated: it means another node
// promoted since our local main was last synced; the machine branch push still
// lands, and the registry entry reaches main on the next promote. The user is
// told, since until then other machines cannot see this enrollment.
func pushBranches(r *repo.Repo, cfg *config.Config) error {
	if cfg.GitPushTarget == "" {
		return nil
	}
	if err := r.Push(cfg.Branch); err != nil {
		return fmt.Errorf("pushing hostname branch: %w", err)
	}
	if err := r.Push("main"); err != nil {
		if !errors.Is(err, repo.ErrNonFastForwardUpdate) {
			return fmt.Errorf("pushing main: %w", err)
		}
		fmt.Println("Note: main has moved on the remote (another machine promoted); " +
			"this enrollment will be registered on main when you next run 'hdf promote' " +
			"(run 'hdf changes-pull' first to review the incoming changes).")
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
	if err := updateMainRegistry(r, tildeFile, filePath); err != nil {
		return err
	}
	if err := pushBranches(r, cfg); err != nil {
		return err
	}
	if err := config.UpdateState(statePath, func(s *config.State) error {
		s.LastCommit = sha
		return nil
	}); err != nil {
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
	if err := ensureOnMachineBranch(r, cfg); err != nil {
		return err
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

// remoteRegistry returns origin/main's registry when it contains files, falling
// back to fallback when the file is absent or empty.  A non-nil error is
// returned when the remote file exists but cannot be parsed.
func remoteRegistry(r *repo.Repo, fallback *config.Registry) (*config.Registry, error) {
	b, err := r.ReadFileFromRemoteBranch("origin", "main", managedTOMLPath)
	if err != nil {
		return nil, fmt.Errorf("reading remote registry: %w", err)
	}
	if len(b) == 0 {
		return fallback, nil
	}
	reg, err := config.RegistryFromBytes(b)
	if err != nil {
		return nil, fmt.Errorf("parsing remote registry: %w", err)
	}
	if reg == nil || len(reg.Files) == 0 {
		return fallback, nil
	}
	return reg, nil
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
	// Prefer the registry from origin/main so files enrolled on other machines
	// are visible even when this machine branch was created before enrollment.
	reg, err = remoteRegistry(r, reg)
	if err != nil {
		return false, err
	}
	anyIncoming := false
	for _, f := range reg.Files {
		expanded := config.ExpandPathIn(f.Path, homeDir)
		var repoFile string
		if len(f.Variants) > 0 {
			repoFile, _ = resolveRepoPath(f, cfg.Branch, cfg.LocalDotfilesDir)
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
		// nil means the file is not yet in origin/main (enrolled but not yet
		// promoted by any machine) — nothing to diff against, skip.
		if mainBytes == nil {
			continue
		}
		branchBytes, err := r.ReadFileFromBranch(cfg.Branch, relPath)
		if err != nil {
			return false, fmt.Errorf("reading %s from branch %s: %w", relPath, cfg.Branch, err)
		}
		if branchBytes != nil && string(mainBytes) == string(branchBytes) {
			continue
		}
		anyIncoming = true
		if err := promptAndMaybeAccept(r, cfg, f, relPath, mainBytes, branchBytes, reader); err != nil {
			return anyIncoming, err
		}
	}
	return anyIncoming, nil
}

// promptAndMaybeAccept shows the diff for one incoming file, prompts the user,
// and accepts or skips it. Returns an error only when stdin is closed or has
// an unexpected read error.
func promptAndMaybeAccept(r *repo.Repo, cfg *config.Config, f config.ManagedFile, relPath string, mainBytes, branchBytes []byte, reader *bufio.Reader) error {
	fmt.Printf("\n--- %s ---\n", f.Path)
	printDiff(daemon.GenerateUnifiedDiff(string(branchBytes), string(mainBytes)))
	fmt.Printf("Accept main's version of %s? [y/N]: ", f.Path)
	ans, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("reading user input: %w", err)
	}
	if errors.Is(err, io.EOF) && strings.TrimSpace(ans) == "" {
		return fmt.Errorf("stdin closed: aborting pull")
	}
	if isYes(strings.TrimSpace(ans)) {
		if err := acceptPromotedFile(r, cfg, relPath, mainBytes, f.Path); err != nil {
			return fmt.Errorf("accepting %s: %w", f.Path, err)
		}
		fmt.Printf("Accepted %s from main.\n", f.Path)
	} else {
		fmt.Printf("Skipped %s — keeping local version.\n", f.Path)
	}
	return nil
}

func acceptPromotedFile(r *repo.Repo, cfg *config.Config, relPath string, mainBytes []byte, tildePath string) (retErr error) {
	staged, err := r.HasStagedChanges()
	if err != nil {
		return fmt.Errorf("checking staged changes: %w", err)
	}
	if staged {
		return fmt.Errorf("index has staged changes unrelated to this accept — commit or unstage them first")
	}

	fullPath := filepath.Join(cfg.LocalDotfilesDir, filepath.FromSlash(relPath))
	regPath := filepath.Join(cfg.LocalDotfilesDir, filepath.FromSlash(managedTOMLPath))

	// Snapshot disk state so we can roll back if any later step fails.
	origFile, origFileErr := os.ReadFile(fullPath)
	origReg, origRegErr := os.ReadFile(regPath)
	defer func() {
		if retErr == nil {
			return
		}
		if origFileErr == nil {
			_ = os.WriteFile(fullPath, origFile, 0o644) //nolint:gosec
		} else if os.IsNotExist(origFileErr) {
			_ = os.Remove(fullPath)
		}
		if origRegErr == nil {
			_ = os.WriteFile(regPath, origReg, 0o644) //nolint:gosec
		}
		_ = r.UnstageAll()
	}()

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	if err := os.WriteFile(fullPath, mainBytes, 0o644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	localReg, err := config.LoadRegistry(cfg.LocalDotfilesDir)
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}
	// Hash the accepted bytes rather than trusting main's registry entry,
	// which may carry a stale or empty stub hash from the enrolling machine.
	upsertRegistryEntry(localReg, tildePath, link.HashBytes(mainBytes))
	if err := config.SaveRegistry(cfg.LocalDotfilesDir, localReg); err != nil {
		return fmt.Errorf("saving registry: %w", err)
	}
	if err := r.StageFile(relPath); err != nil {
		return fmt.Errorf("staging file: %w", err)
	}
	if err := r.StageFile(managedTOMLPath); err != nil {
		return fmt.Errorf("staging registry: %w", err)
	}
	if _, err := r.CommitStaged(fmt.Sprintf("hdf: accept %s from main", relPath)); err != nil {
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
	if err := ensureOnMachineBranch(r, cfg); err != nil {
		return err
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
			if anyIncoming {
				reloaded, err := config.LoadRegistry(cfg.LocalDotfilesDir)
				if err != nil {
					return fmt.Errorf("reloading registry: %w", err)
				}
				reg = reloaded
			} else {
				fmt.Println("Already up to date.")
			}
		}
	}

	for _, f := range reg.Files {
		expanded := config.ExpandPathIn(f.Path, homeDir)
		var repoFile string
		var err error
		if len(f.Variants) > 0 {
			repoFile, err = resolveRepoPath(f, cfg.Branch, cfg.LocalDotfilesDir)
		} else {
			repoFile, err = link.RepoPathForHome(expanded, cfg.LocalDotfilesDir, homeDir)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "link %s: %v\n", f.Path, err)
			continue
		}
		if repoFile == "" {
			fmt.Fprintf(os.Stderr, "link %s: no variant for branch %q — skipping (add a variant for this branch to %s to manage the file here)\n",
				f.Path, cfg.Branch, managedTOMLPath)
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

func runPromote(cfg *config.Config, homeDir string, stdin io.Reader, statePath string) error {
	if cfg.GitPushTarget == "" {
		return fmt.Errorf("cannot promote: no remote configured — promotion has no effect without a shared repository")
	}
	r, err := repo.Open(cfg.LocalDotfilesDir)
	if err != nil {
		return fmt.Errorf("opening repo: %w", err)
	}
	if err := ensureOnMachineBranch(r, cfg); err != nil {
		return err
	}
	clean, err := r.IsCleanForPromote()
	if err != nil {
		return fmt.Errorf("checking status: %w", err)
	}
	if !clean {
		return fmt.Errorf("uncommitted changes in the dotfiles repository — run 'hdf changes-push <file>' first")
	}
	if err := r.Fetch(); err != nil {
		return fmt.Errorf("fetching before promote: %w", err)
	}
	// Guard 2: origin/main holds content this machine has never had — either
	// unpulled promotes (preserved by the merge) or newer versions this promote
	// would overwrite. Each needs explicit consent before proceeding.
	pending, err := collectUnseenIncoming(r, cfg, homeDir)
	if err != nil {
		return fmt.Errorf("checking incoming: %w", err)
	}
	preferTheirs, err := reviewUnseenIncoming(pending, bufio.NewReader(stdin), statePath)
	if err != nil {
		return err
	}
	// Sync local main to origin/main so MergeIntoBranch builds on top of all
	// prior promotions. Without this, Push("main") would be a non-fast-forward
	// when another machine has promoted since this repo was last cloned/fetched.
	if err := r.SyncLocalMain("origin"); err != nil {
		return fmt.Errorf("syncing local main to origin: %w", err)
	}
	fmt.Printf("Merging %s into main...\n", cfg.Branch)
	mergeOpts := &repo.MergeOpts{
		PreferTheirs:   preferTheirs,
		ContentMergers: map[string]repo.ContentMerger{managedTOMLPath: registryUnionMerger},
	}
	if err := r.MergeIntoBranch("main", mergeOpts); err != nil {
		return fmt.Errorf("promoting: %w", err)
	}
	return pushPromoted(r, cfg, statePath)
}

// pushPromoted pushes the machine branch and the merged main, rolling local
// main back (Guard 3) when another machine promoted in the race window, and
// records the new main SHA so the daemon does not notify this machine about
// its own promote.
func pushPromoted(r *repo.Repo, cfg *config.Config, statePath string) error {
	if err := r.Push(cfg.Branch); err != nil {
		return fmt.Errorf("pushing %s: %w", cfg.Branch, err)
	}
	// TODO(future): make this atomic — push the merge commit object directly to
	// origin/main using a compare-and-swap refspec so local main is never
	// advanced until the remote accepts. See design doc 2026-07-05.
	if err := r.Push("main"); err != nil {
		if errors.Is(err, repo.ErrNonFastForwardUpdate) {
			// Guard 3: another machine promoted between Guard 2's fetch and now.
			// Reset local main back to origin/main (MergeIntoBranch only moves a
			// ref, so no working-tree changes need to be undone).
			if rollbackErr := r.ResetBranchToRemote("main", "origin"); rollbackErr != nil {
				return fmt.Errorf("promote failed and rollback of local main failed: %w (original: %w)", rollbackErr, err)
			}
			return fmt.Errorf("cannot promote: another machine promoted while you were working — run 'hdf changes-pull' and try again")
		}
		return fmt.Errorf("pushing main: %w", err)
	}
	// Best-effort: a failure here only costs one redundant notification.
	if mainSHA, shaErr := r.BranchSHA("main"); shaErr == nil {
		if stateErr := recordMainCommit(statePath, mainSHA); stateErr != nil {
			fmt.Printf("Warning: could not update state file: %v\n", stateErr)
		}
	}
	fmt.Printf("Promoted %s → main and pushed to origin.\n", cfg.Branch)
	return nil
}

// recordMainCommit updates state.LastMainCommit under the state lock so the
// daemon does not notify this machine about its own promote.
func recordMainCommit(statePath, mainSHA string) error {
	return config.UpdateState(statePath, func(state *config.State) error {
		state.LastMainCommit = mainSHA
		return nil
	})
}

var promoteCmd = &cobra.Command{
	Use:   "promote",
	Short: "Merge your machine branch into main and push",
	Long: `Merges the current machine branch into main and pushes both to origin.
Content this machine has never reviewed (another machine's promote you haven't
pulled, or a newer version of a file you both changed) is shown first and needs
explicit consent. Run 'hdf changes-pull' to review incoming changes in full.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := config.DefaultPath()
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("hdf is not initialized — run 'hdf init' first (%w)", err)
		}
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home directory: %w", err)
		}
		return runPromote(cfg, homeDir, os.Stdin, config.DefaultStatePath())
	},
}

var linkCmd = &cobra.Command{
	Use:   "changes-pull",
	Short: "Fetch main, review incoming files one by one, and re-create symlinks",
	Long: `Fetches origin/main and walks through each managed file that differs from your
machine branch, showing the diff and asking whether to accept main's version.
Accepting commits main's content to your branch and updates the local registry;
skipping keeps your version. Symlinks are always re-created afterwards.
Skipping is an accepted workflow — run hdf changes-pull again when ready.`,
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
			fmt.Printf("  %-40s %s\n", f.Path, fileStatus(f, cfg.Branch, homeDir))
		}
		return nil
	},
}

// statusNoVariant is the status label for a file that has variants but none
// for this machine's branch.
const statusNoVariant = "no variant for this branch"

// fileStatus returns the status label for one managed file on the given
// branch. A file whose variants have no entry for this branch is not managed
// on this machine, so it gets its own state rather than being misreported as
// drift or as missing.
func fileStatus(f config.ManagedFile, branch, homeDir string) string {
	v, res := f.ResolveVariant(branch)
	if res == config.VariantNoBranchMatch {
		return statusNoVariant
	}
	expectedHash := f.Hash
	if res == config.VariantMatch {
		expectedHash = v.Hash
	}
	expanded := config.ExpandPathIn(f.Path, homeDir)
	currentHash, err := link.HashFile(expanded)
	if err != nil {
		return "missing"
	}
	if currentHash != expectedHash {
		return "CHANGED (uncommitted)"
	}
	return "ok"
}

// svcInstall, svcUninstall, svcStart, svcStop, and svcStatus are indirections
// over the svc package so tests can substitute fakes instead of touching a
// real OS service manager.
var (
	svcInstall   = svc.Install
	svcUninstall = svc.Uninstall
	svcStart     = svc.Start
	svcStop      = svc.Stop
	svcStatus    = svc.Status
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the hdf sync daemon",
	Long:  `Run the hdf sync daemon in the foreground, or install/control it as a per-user background service.`,
}

// svcRun is an indirection over svc.Run so tests can substitute a fake
// instead of touching a real OS service manager.
var svcRun = svc.Run

// runDaemon checks that hdf is initialized before handing off to run, so
// the service doesn't start under OS supervision in a broken state and
// fail silently. It's a var so tests can substitute it without touching
// the real default config path.
var runDaemon = func(cfgPath string, run func(string) error) error {
	if _, err := config.Load(cfgPath); err != nil {
		return fmt.Errorf("hdf is not initialized — run 'hdf init' first (%w)", err)
	}
	return run(cfgPath)
}

var daemonRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the hdf sync daemon in the foreground",
	Long:  `Runs a background loop that syncs every 30 minutes and sends OS notifications when action is needed. Also used internally as the entry point for the installed background service.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDaemon(config.DefaultPath(), svcRun)
	},
}

var daemonInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install and start the hdf sync daemon as a per-user background service",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDaemon(config.DefaultPath(), svcInstall)
	},
}

var daemonUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Stop and remove the installed hdf sync daemon service",
	RunE: func(cmd *cobra.Command, args []string) error {
		return svcUninstall(config.DefaultPath())
	},
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the already-installed hdf sync daemon service",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDaemon(config.DefaultPath(), svcStart)
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the already-installed hdf sync daemon service",
	RunE: func(cmd *cobra.Command, args []string) error {
		return svcStop(config.DefaultPath())
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report whether the hdf sync daemon service is installed/running",
	RunE: func(cmd *cobra.Command, args []string) error {
		status, err := svcStatus(config.DefaultPath())
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), status)
		return err
	},
}

func init() {
	daemonCmd.AddCommand(daemonRunCmd, daemonInstallCmd, daemonUninstallCmd, daemonStartCmd, daemonStopCmd, daemonStatusCmd)
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
func resolveRepoPath(f config.ManagedFile, branch, localDotfilesDir string) (string, error) {
	v, res := f.ResolveVariant(branch)
	if res != config.VariantMatch {
		return "", nil
	}
	// RepoPath comes from the shared registry, which other machines write —
	// treat it as untrusted input and refuse anything that would resolve
	// outside the dotfiles repo (a hostile entry could otherwise make
	// changes-pull write attacker-controlled content to an arbitrary path).
	if filepath.IsAbs(v.RepoPath) {
		return "", fmt.Errorf("variant repo path for %s must be relative, got %q", f.Path, v.RepoPath)
	}
	resolved := filepath.Join(localDotfilesDir, v.RepoPath)
	rel, err := filepath.Rel(localDotfilesDir, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("variant repo path for %s escapes the dotfiles repo: %q", f.Path, v.RepoPath)
	}
	return resolved, nil
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

// version is the release version shown by `hdf --version`. It is injected at
// build time by goreleaser via -ldflags "-X hdf/internal/cli.version=...";
// plain `go build` binaries report "dev".
var version = "dev"

// Execute runs the hdf CLI. frontendAssets is the embedded frontend bundle,
// passed in by package main. Exits the process with status 1 on error.
func Execute(frontendAssets embed.FS) {
	assets = frontendAssets
	rootCmd.Version = version
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
