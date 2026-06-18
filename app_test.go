package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetDiffContent_HTTPErrors(t *testing.T) {
	cases := []struct {
		name          string
		statusCode    int
		responseBody  string
		wantErr       bool
		wantErrSubstr string
		wantContent   string
	}{
		{
			name:          "404 returns error, not body",
			statusCode:    http.StatusNotFound,
			responseBody:  "<html>Not Found</html>",
			wantErr:       true,
			wantErrSubstr: "HTTP 404",
		},
		{
			name:          "500 returns error, not body",
			statusCode:    http.StatusInternalServerError,
			responseBody:  "internal server error",
			wantErr:       true,
			wantErrSubstr: "HTTP 500",
		},
		{
			name:         "200 returns diff body",
			statusCode:   http.StatusOK,
			responseBody: "diff --git a/foo b/foo",
			wantErr:      false,
			wantContent:  "diff --git a/foo b/foo",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.responseBody))
			}))
			defer srv.Close()

			app := &App{
				diffURLs:     []string{srv.URL},
				currentIndex: 0,
				ctx:          context.Background(),
			}

			got, err := app.GetDiffContent()

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.wantErrSubstr) {
					t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErrSubstr)
				}
				if got != "" {
					t.Errorf("expected empty string on error, got %q", got)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tc.wantContent {
					t.Errorf("GetDiffContent() = %q, want %q", got, tc.wantContent)
				}
			}
		})
	}
}

func TestIsInitialized(t *testing.T) {
	validTOML := `git_push_target = "file:///tmp/bare"
local_dotfiles_dir = "/tmp/repo"
branch = "test-host"
`
	cases := []struct {
		name    string
		setup   func(t *testing.T) string
		wantOk  bool
		wantErr bool
	}{
		{
			name: "missing config — not initialized, no error",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "no-such-config.toml")
			},
			wantOk:  false,
			wantErr: false,
		},
		{
			name: "valid config — initialized, no error",
			setup: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "config.toml")
				if err := os.WriteFile(p, []byte(validTOML), 0o644); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantOk:  true,
			wantErr: false,
		},
		{
			name: "corrupted config — not initialized, returns error",
			setup: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "config.toml")
				if err := os.WriteFile(p, []byte("not valid toml [\x00\x01"), 0o644); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantOk:  false,
			wantErr: true,
		},
		{
			name: "empty config file — initialized (all fields zero-valued)",
			setup: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "config.toml")
				if err := os.WriteFile(p, []byte(""), 0o644); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantOk:  true,
			wantErr: false,
		},
		{
			name: "unreadable config file — not initialized, returns error",
			setup: func(t *testing.T) string {
				if os.Getuid() == 0 {
					t.Skip("root bypasses DAC — permission test not meaningful")
				}
				p := filepath.Join(t.TempDir(), "config.toml")
				if err := os.WriteFile(p, []byte(validTOML), 0o000); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantOk:  false,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.setup(t)
			ok, err := isInitialized(path)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok != tc.wantOk {
				t.Errorf("isInitialized = %v, want %v", ok, tc.wantOk)
			}
		})
	}
}

// TestGetDiffContent_NoDiffs verifies the HasDiff()==false path: GetDiffContent
// must return ("", nil) without panicking when no diff URLs are queued.
func TestGetDiffContent_NoDiffs(t *testing.T) {
	app := &App{
		diffURLs:     []string{},
		currentIndex: 0,
		ctx:          context.Background(),
	}
	got, err := app.GetDiffContent()
	if err != nil {
		t.Fatalf("expected no error with empty diffURLs, got: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string with no diffs, got: %q", got)
	}
}

// TestHasDiff_FalseWhenEmpty verifies HasDiff returns false for an empty slice
// and true once URLs are present.
func TestHasDiff_FalseWhenEmpty(t *testing.T) {
	app := &App{}
	if app.HasDiff() {
		t.Error("HasDiff() = true for zero-value App, want false")
	}

	app.diffURLs = []string{"http://example.com/diff1"}
	if !app.HasDiff() {
		t.Error("HasDiff() = false after adding a URL, want true")
	}
}
