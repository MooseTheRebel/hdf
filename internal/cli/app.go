package cli

import (
	"context"
	"errors"
	"fmt"
	"hdf/config"
	"hdf/daemon"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx           context.Context
	mu            sync.Mutex
	diffURLs      []string
	currentIndex  int
	linkPending   []pendingIncomingFile
	enrollPending *pendingEnroll
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		currentIndex: 0,
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// IsInitialized reports whether hdf has been configured on this machine.
// It returns (false, nil) when the config file is absent and (false, err)
// when the file exists but is corrupted, so the UI can distinguish the two.
func (a *App) IsInitialized() (bool, error) {
	return isInitialized(config.DefaultPath())
}

// GetStatus returns hdf's current status — config summary, branch, last
// sync, and each managed file's state — the GUI's equivalent of `hdf
// status`.
func (a *App) GetStatus() (*StatusInfo, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}
	return computeStatus(config.DefaultPath(), config.DefaultStatePath(), homeDir)
}

// GetConfig returns hdf's current config file — path and raw contents (or
// an indication it doesn't exist yet) — the GUI's equivalent of `hdf
// config`.
func (a *App) GetConfig() (*ConfigInfo, error) {
	return computeConfigInfo(config.DefaultPath())
}

// GetDaemonStatus reports whether the hdf sync daemon service is
// installed/running — the GUI's equivalent of `hdf daemon status`.
func (a *App) GetDaemonStatus() (string, error) {
	return svcStatus(config.DefaultPath())
}

// InstallDaemon installs and starts the hdf sync daemon as a per-user
// background service — the GUI's equivalent of `hdf daemon install`.
func (a *App) InstallDaemon() error {
	return runDaemon(config.DefaultPath(), svcInstall)
}

// UninstallDaemon stops and removes the installed hdf sync daemon service —
// the GUI's equivalent of `hdf daemon uninstall`.
func (a *App) UninstallDaemon() error {
	return svcUninstall(config.DefaultPath())
}

// StartDaemon starts the already-installed hdf sync daemon service — the
// GUI's equivalent of `hdf daemon start`.
func (a *App) StartDaemon() error {
	return runDaemon(config.DefaultPath(), svcStart)
}

// StopDaemon stops the already-installed hdf sync daemon service — the
// GUI's equivalent of `hdf daemon stop`.
func (a *App) StopDaemon() error {
	return svcStop(config.DefaultPath())
}

// GetPendingWarnings returns and clears any daemon-recorded warnings — the
// GUI's equivalent of the check `hdf link` runs before proceeding. Matches
// daemon.PendingWarnings' take-and-clear semantics: calling this consumes
// the warnings even if the caller then cancels.
func (a *App) GetPendingWarnings() ([]string, error) {
	return daemon.PendingWarnings(config.DefaultStatePath())
}

// StartLink begins a link operation: computes pending incoming file diffs
// (unless noFetch) and stores them in App state for AcceptIncomingFile to
// consume by index. Call FinishLink to complete the operation once all
// incoming files have been reviewed (or immediately, if IncomingFiles is
// empty) — the GUI's equivalent of `hdf link`.
func (a *App) StartLink(noFetch bool) (*LinkStartInfo, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}
	info, pending, err := computeLinkStartFn(config.DefaultPath(), homeDir, noFetch)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.linkPending = pending
	a.mu.Unlock()
	return info, nil
}

// AcceptIncomingFile accepts main's version of the pending incoming file at
// index, as computed by the most recent StartLink call.
func (a *App) AcceptIncomingFile(index int) error {
	a.mu.Lock()
	if index < 0 || index >= len(a.linkPending) {
		a.mu.Unlock()
		return fmt.Errorf("accept incoming file: index %d out of range (0..%d)", index, len(a.linkPending)-1)
	}
	item := a.linkPending[index]
	a.mu.Unlock()
	return acceptIncomingFileFn(config.DefaultPath(), item)
}

// FinishLink re-creates symlinks for all managed files and clears the link
// session state started by StartLink.
func (a *App) FinishLink() ([]LinkedFile, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}
	results, err := computeRelinkFn(config.DefaultPath(), homeDir)
	a.mu.Lock()
	a.linkPending = nil
	a.mu.Unlock()
	return results, err
}

// PickFileToEnroll opens a native "choose file" dialog rooted at the user's
// home directory and returns the selected path, or "" if the user
// cancelled. Not unit-tested — it drives a real OS dialog, the same
// untestable-by-design shape as CloseWindow.
func (a *App) PickFileToEnroll() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Select a file to enroll",
		DefaultDirectory: homeDir,
		ShowHiddenFiles:  true,
	})
}

// StartEnroll begins enrolling path: computes the diff against any
// currently committed version and stores the pending decision in App state
// for ConfirmEnroll to apply — the GUI's equivalent of `hdf enroll`/`hdf
// changes-push`'s setup and diff-preview phase.
func (a *App) StartEnroll(path string) (*EnrollStartInfo, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}
	info, pending, err := computeEnrollStartFn(config.DefaultPath(), homeDir, path)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.enrollPending = pending
	a.mu.Unlock()
	return info, nil
}

// ConfirmEnroll applies the enroll started by the most recent StartEnroll
// call: copies the file into the repo, commits, and pushes.
func (a *App) ConfirmEnroll() (*EnrollResult, error) {
	a.mu.Lock()
	pending := a.enrollPending
	a.mu.Unlock()
	if pending == nil {
		return nil, fmt.Errorf("confirm enroll: no pending enroll — call StartEnroll first")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}
	result, err := computeApplyEnrollFn(config.DefaultPath(), homeDir, config.DefaultStatePath(), *pending)
	a.mu.Lock()
	a.enrollPending = nil
	a.mu.Unlock()
	return result, err
}

func isInitialized(path string) (bool, error) {
	_, err := config.Load(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// fetchDiff fetches raw unified-diff content from url.
// Non-2xx responses are logged and returned as errors so callers and the
// daemon log can surface them without silently discarding the failure.
func fetchDiff(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching diff: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[WARN] fetchDiff: HTTP %d from %s", resp.StatusCode, url)
		return "", fmt.Errorf("fetching diff from %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading diff: %w", err)
	}
	return string(body), nil
}

// GetDiffContent returns the diff content for the current index.
// Returns ("", nil) when there are no diffs to display (HasDiff() == false),
// which is the safe no-op path when the wails window is opened without diffs.
func (a *App) GetDiffContent() (string, error) {
	a.mu.Lock()
	if len(a.diffURLs) == 0 || a.currentIndex >= len(a.diffURLs) {
		a.mu.Unlock()
		return "", nil
	}
	currentURL := a.diffURLs[a.currentIndex]
	a.mu.Unlock()

	parentCtx := a.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	return fetchDiff(parentCtx, currentURL)
}

// HasDiff returns true if one or more diff URLs are queued for display.
func (a *App) HasDiff() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.diffURLs) > 0
}

// GetCurrentIndex returns the current diff index
func (a *App) GetCurrentIndex() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.currentIndex
}

// GetTotalDiffs returns the total number of diffs
func (a *App) GetTotalDiffs() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.diffURLs)
}

// NextDiff moves to the next diff
func (a *App) NextDiff() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.currentIndex < len(a.diffURLs)-1 {
		a.currentIndex++
	}
}

// PreviousDiff moves to the previous diff
func (a *App) PreviousDiff() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.currentIndex > 0 {
		a.currentIndex--
	}
}

// CloseWindow closes the application window.
// Note: this must only be called from the wails GUI path; the daemon process
// must never call into this function.
func (a *App) CloseWindow() {
	runtime.Quit(a.ctx)
}
