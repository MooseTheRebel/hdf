package main

import (
	"context"
	"errors"
	"fmt"
	"hdf/config"
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
	ctx          context.Context
	mu           sync.Mutex
	diffURLs     []string
	currentIndex int
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
