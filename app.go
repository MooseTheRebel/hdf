package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx          context.Context
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

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}

// GetDiffContent returns the diff content to display
func (a *App) GetDiffContent() string {
	if len(a.diffURLs) == 0 || a.currentIndex >= len(a.diffURLs) {
		return ""
	}

	currentURL := a.diffURLs[a.currentIndex]
	resp, err := http.Get(currentURL)
	if err != nil {
		return fmt.Sprintf("Error fetching diff: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("Error reading diff: %v", err)
	}

	return string(body)
}

// HasDiff returns true if a diff URL is set
func (a *App) HasDiff() bool {
	return len(a.diffURLs) > 0
}

// GetCurrentIndex returns the current diff index
func (a *App) GetCurrentIndex() int {
	return a.currentIndex
}

// GetTotalDiffs returns the total number of diffs
func (a *App) GetTotalDiffs() int {
	return len(a.diffURLs)
}

// NextDiff moves to the next diff
func (a *App) NextDiff() {
	if a.currentIndex < len(a.diffURLs)-1 {
		a.currentIndex++
	}
}

// PreviousDiff moves to the previous diff
func (a *App) PreviousDiff() {
	if a.currentIndex > 0 {
		a.currentIndex--
	}
}

// CloseWindow closes the application window
func (a *App) CloseWindow() {
	runtime.Quit(a.ctx)
}
