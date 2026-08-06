package cli

import (
	"context"
	"errors"
	"hdf/config"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Shared fixture paths for App-layer seam tests below (link/enroll session
// state tests) — kept as constants so goconst doesn't flag their repeated
// literal use across independent test functions.
const (
	tildeA           = "~/a.txt"
	tildeB           = "~/b.txt"
	relPathATxt      = "a.txt"
	sharedHostBranch = "shared-host"
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

// TestGetDiffContentLargeResponseTruncatedAt1MB verifies that a response body
// larger than 1 MB is capped at exactly 1<<20 bytes, preventing OOM from
// arbitrarily large remote files.
func TestGetDiffContentLargeResponseTruncatedAt1MB(t *testing.T) {
	const limit = 1 << 20 // 1 MB
	large := make([]byte, limit+512)
	for i := range large {
		large[i] = 'x'
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(large)
	}))
	defer srv.Close()

	app := &App{
		diffURLs:     []string{srv.URL},
		currentIndex: 0,
		ctx:          context.Background(),
	}

	got, err := app.GetDiffContent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) > limit {
		t.Errorf("GetDiffContent() returned %d bytes, want at most %d", len(got), limit)
	}
}

// TestDaemonActionMethods_DelegateToSvcFuncs verifies that the App's daemon
// management methods delegate to their respective svc func var with the
// default config path and surface errors. InstallDaemon and StartDaemon
// also go through runDaemon's preflight check, so it's mocked through for
// those two, mirroring TestDaemonServiceCmds_DelegateToSvcFuncs for the
// cobra commands.
func TestDaemonActionMethods_DelegateToSvcFuncs(t *testing.T) {
	cases := []struct {
		name         string
		method       func(*App) error
		svcFunc      *func(string) error
		viaRunDaemon bool
	}{
		{name: daemonSubcmdInstall, method: (*App).InstallDaemon, svcFunc: &svcInstall, viaRunDaemon: true},
		{name: daemonSubcmdUninstall, method: (*App).UninstallDaemon, svcFunc: &svcUninstall},
		{name: daemonSubcmdStart, method: (*App).StartDaemon, svcFunc: &svcStart, viaRunDaemon: true},
		{name: daemonSubcmdStop, method: (*App).StopDaemon, svcFunc: &svcStop},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.viaRunDaemon {
				origRunDaemon := runDaemon
				defer func() { runDaemon = origRunDaemon }()
				runDaemon = func(cfgPath string, run func(string) error) error { return run(cfgPath) }
			}

			origFunc := *tc.svcFunc
			defer func() { *tc.svcFunc = origFunc }()

			var gotCfgPath string
			*tc.svcFunc = func(cfgPath string) error {
				gotCfgPath = cfgPath
				return nil
			}
			app := &App{}
			if err := tc.method(app); err != nil {
				t.Fatalf("%s() error = %v, want nil", tc.name, err)
			}
			if gotCfgPath != config.DefaultPath() {
				t.Errorf("cfgPath = %q, want %q", gotCfgPath, config.DefaultPath())
			}

			*tc.svcFunc = func(string) error { return errors.New("boom") }
			if err := tc.method(app); err == nil {
				t.Fatalf("expected %s() error to propagate, got nil", tc.name)
			}
		})
	}
}

// TestAppStartLink_StoresPendingForAcceptIncomingFile verifies that
// App.StartLink stashes the pending incoming files returned by
// computeLinkStartFn, that AcceptIncomingFile looks up the right one by
// index and forwards it to acceptIncomingFileFn, and that an out-of-range
// index is rejected without calling acceptIncomingFileFn at all.
func TestAppStartLink_StoresPendingForAcceptIncomingFile(t *testing.T) {
	origStart := computeLinkStartFn
	origAccept := acceptIncomingFileFn
	defer func() {
		computeLinkStartFn = origStart
		acceptIncomingFileFn = origAccept
	}()

	wantPending := []pendingIncomingFile{
		{relPath: relPathATxt, tildePath: tildeA, mainBytes: []byte("a")},
		{relPath: "b.txt", tildePath: tildeB, mainBytes: []byte("b")},
	}
	computeLinkStartFn = func(cfgPath, homeDir string, noFetch bool) (*LinkStartInfo, []pendingIncomingFile, error) {
		return &LinkStartInfo{IncomingFiles: []IncomingFile{{Path: tildeA}, {Path: tildeB}}}, wantPending, nil
	}

	app := &App{}
	info, err := app.StartLink(false)
	if err != nil {
		t.Fatalf("StartLink: %v", err)
	}
	if len(info.IncomingFiles) != 2 {
		t.Fatalf("IncomingFiles = %v, want 2 entries", info.IncomingFiles)
	}

	var gotItem pendingIncomingFile
	var called bool
	acceptIncomingFileFn = func(cfgPath string, item pendingIncomingFile) error {
		called = true
		gotItem = item
		return nil
	}
	if err := app.AcceptIncomingFile(1); err != nil {
		t.Fatalf("AcceptIncomingFile(1): %v", err)
	}
	if !called {
		t.Fatal("acceptIncomingFileFn was not called")
	}
	if gotItem.relPath != wantPending[1].relPath || gotItem.tildePath != wantPending[1].tildePath || string(gotItem.mainBytes) != string(wantPending[1].mainBytes) {
		t.Errorf("acceptIncomingFileFn got %+v, want %+v", gotItem, wantPending[1])
	}

	called = false
	if err := app.AcceptIncomingFile(5); err == nil {
		t.Fatal("expected error for out-of-range index, got nil")
	}
	if called {
		t.Error("acceptIncomingFileFn should not be called for an out-of-range index")
	}
}

// TestAppFinishLink_ClearsPendingState verifies that App.FinishLink returns
// computeRelinkFn's results and clears the pending-incoming-files state
// left over from a prior StartLink, so a stale accept can't be replayed
// against a new link session.
func TestAppFinishLink_ClearsPendingState(t *testing.T) {
	origRelink := computeRelinkFn
	defer func() { computeRelinkFn = origRelink }()

	computeRelinkFn = func(cfgPath, homeDir string) ([]LinkedFile, error) {
		return []LinkedFile{{Path: tildeA}}, nil
	}

	app := &App{linkPending: []pendingIncomingFile{{relPath: relPathATxt}}}
	results, err := app.FinishLink()
	if err != nil {
		t.Fatalf("FinishLink: %v", err)
	}
	if len(results) != 1 || results[0].Path != tildeA {
		t.Errorf("results = %v, want one entry for %s", results, tildeA)
	}
	if len(app.linkPending) != 0 {
		t.Errorf("linkPending = %v, want cleared after FinishLink", app.linkPending)
	}
}

// TestAppStartLink_FailureInvalidatesPriorPending verifies that a StartLink
// call which fails (e.g. a remote-fetch error) clears any linkPending left
// over from an earlier successful StartLink, rather than leaving it
// applicable via AcceptIncomingFile. Without this, a failed "start a new
// link session" leaves the previous session's stale incoming-file snapshot
// executable at the Wails boundary.
func TestAppStartLink_FailureInvalidatesPriorPending(t *testing.T) {
	origStart := computeLinkStartFn
	origAccept := acceptIncomingFileFn
	defer func() {
		computeLinkStartFn = origStart
		acceptIncomingFileFn = origAccept
	}()

	app := &App{linkPending: []pendingIncomingFile{{relPath: relPathATxt, tildePath: tildeA}}}

	computeLinkStartFn = func(cfgPath, homeDir string, noFetch bool) (*LinkStartInfo, []pendingIncomingFile, error) {
		return nil, nil, errors.New("fetch failed")
	}
	if _, err := app.StartLink(false); err == nil {
		t.Fatal("expected StartLink to propagate the computeLinkStartFn error, got nil")
	}

	var called bool
	acceptIncomingFileFn = func(cfgPath string, item pendingIncomingFile) error {
		called = true
		return nil
	}
	if err := app.AcceptIncomingFile(0); err == nil {
		t.Fatal("expected AcceptIncomingFile to reject the stale session after a failed StartLink, got nil")
	}
	if called {
		t.Error("acceptIncomingFileFn should not be called against a pending session from before a failed StartLink")
	}
}

// TestAppStartEnroll_StoresPendingForConfirmEnroll verifies that
// App.StartEnroll stashes the pendingEnroll returned by
// computeEnrollStartFn, and that ConfirmEnroll forwards it to
// computeApplyEnrollFn and clears the stored state afterward.
func TestAppStartEnroll_StoresPendingForConfirmEnroll(t *testing.T) {
	origStart := computeEnrollStartFn
	origApply := computeApplyEnrollFn
	defer func() {
		computeEnrollStartFn = origStart
		computeApplyEnrollFn = origApply
	}()

	wantPending := &pendingEnroll{tildeFile: tildeA, relName: relPathATxt, filePath: "/home/a.txt"}
	computeEnrollStartFn = func(cfgPath, homeDir, filePath string) (*EnrollStartInfo, *pendingEnroll, error) {
		return &EnrollStartInfo{Path: tildeA, IsNewFile: true}, wantPending, nil
	}

	app := &App{}
	info, err := app.StartEnroll("/home/a.txt")
	if err != nil {
		t.Fatalf("StartEnroll: %v", err)
	}
	if info.Path != tildeA {
		t.Errorf("Path = %q, want %q", info.Path, tildeA)
	}

	var gotPending pendingEnroll
	var called bool
	computeApplyEnrollFn = func(cfgPath, homeDir, statePath string, p pendingEnroll) (*EnrollResult, error) {
		called = true
		gotPending = p
		return &EnrollResult{Message: "Enrolled ~/a.txt (commit abc12345)"}, nil
	}
	result, err := app.ConfirmEnroll()
	if err != nil {
		t.Fatalf("ConfirmEnroll: %v", err)
	}
	if !called {
		t.Fatal("computeApplyEnrollFn was not called")
	}
	if gotPending != *wantPending {
		t.Errorf("computeApplyEnrollFn got %+v, want %+v", gotPending, *wantPending)
	}
	if result.Message != "Enrolled ~/a.txt (commit abc12345)" {
		t.Errorf("Message = %q, want the enrolled message", result.Message)
	}
	if app.enrollPending != nil {
		t.Errorf("enrollPending = %+v, want nil after ConfirmEnroll", app.enrollPending)
	}
}

// TestAppConfirmEnroll_WithoutStartEnrollReturnsError verifies that
// ConfirmEnroll rejects being called before StartEnroll has populated
// pending state, rather than silently applying a zero-value pendingEnroll.
func TestAppConfirmEnroll_WithoutStartEnrollReturnsError(t *testing.T) {
	origApply := computeApplyEnrollFn
	defer func() { computeApplyEnrollFn = origApply }()

	var called bool
	computeApplyEnrollFn = func(cfgPath, homeDir, statePath string, p pendingEnroll) (*EnrollResult, error) {
		called = true
		return &EnrollResult{}, nil
	}

	app := &App{}
	if _, err := app.ConfirmEnroll(); err == nil {
		t.Fatal("expected error when ConfirmEnroll is called without a prior StartEnroll, got nil")
	}
	if called {
		t.Error("computeApplyEnrollFn should not be called without pending enroll state")
	}
}

// TestAppStartEnroll_FailureInvalidatesPriorPending verifies that a
// StartEnroll call which fails clears any enrollPending left over from an
// earlier successful StartEnroll, rather than leaving it confirmable via
// ConfirmEnroll. Without this, a failed "start a new enroll" leaves the
// previous session's stale pending enroll applicable at the Wails boundary.
func TestAppStartEnroll_FailureInvalidatesPriorPending(t *testing.T) {
	origStart := computeEnrollStartFn
	origApply := computeApplyEnrollFn
	defer func() {
		computeEnrollStartFn = origStart
		computeApplyEnrollFn = origApply
	}()

	app := &App{enrollPending: &pendingEnroll{tildeFile: tildeA, relName: relPathATxt}}

	computeEnrollStartFn = func(cfgPath, homeDir, filePath string) (*EnrollStartInfo, *pendingEnroll, error) {
		return nil, nil, errors.New("path is a directory")
	}
	if _, err := app.StartEnroll("/some/dir"); err == nil {
		t.Fatal("expected StartEnroll to propagate the computeEnrollStartFn error, got nil")
	}

	var called bool
	computeApplyEnrollFn = func(cfgPath, homeDir, statePath string, p pendingEnroll) (*EnrollResult, error) {
		called = true
		return &EnrollResult{}, nil
	}
	if _, err := app.ConfirmEnroll(); err == nil {
		t.Fatal("expected ConfirmEnroll to reject the stale session after a failed StartEnroll, got nil")
	}
	if called {
		t.Error("computeApplyEnrollFn should not be called against a pending session from before a failed StartEnroll")
	}
}

// TestAppStartInitLocal_StoresPendingForResolveAndFinish verifies that
// App.StartInitLocal stashes the pendingInit returned by
// computeInitLocalStartFn, that ResolveBranchCollision and FinishInit
// forward it to their respective seams, and that FinishInit clears the
// stored state afterward.
func TestAppStartInitLocal_StoresPendingForResolveAndFinish(t *testing.T) {
	origLocal := computeInitLocalStartFn
	origResolve := computeResolveBranchCollisionFn
	origFinish := computeFinishInitFn
	defer func() {
		computeInitLocalStartFn = origLocal
		computeResolveBranchCollisionFn = origResolve
		computeFinishInitFn = origFinish
	}()

	wantPending := &pendingInit{repoPath: "/repo", branch: sharedHostBranch}
	computeInitLocalStartFn = func(cfgPath, repoPath, pushTarget string) (*InitStartInfo, *pendingInit, error) {
		return &InitStartInfo{Collision: &InitBranchCollision{Branch: sharedHostBranch}}, wantPending, nil
	}

	app := &App{}
	info, err := app.StartInitLocal("/repo", "")
	if err != nil {
		t.Fatalf("StartInitLocal: %v", err)
	}
	if info.Collision == nil || info.Collision.Branch != sharedHostBranch {
		t.Errorf("Collision = %+v, want branch shared-host", info.Collision)
	}

	var gotUnique bool
	var resolveCalled bool
	computeResolveBranchCollisionFn = func(p *pendingInit, useUnique bool) error {
		resolveCalled = true
		gotUnique = useUnique
		if p != wantPending {
			t.Errorf("computeResolveBranchCollisionFn got %+v, want the stored pendingInit", p)
		}
		return nil
	}
	if err := app.ResolveBranchCollision(true); err != nil {
		t.Fatalf("ResolveBranchCollision: %v", err)
	}
	if !resolveCalled || !gotUnique {
		t.Error("computeResolveBranchCollisionFn was not called with useUnique=true")
	}

	var finishCalled bool
	computeFinishInitFn = func(cfgPath, statePath string, p *pendingInit) (*InitResult, error) {
		finishCalled = true
		if p != wantPending {
			t.Errorf("computeFinishInitFn got %+v, want the stored pendingInit", p)
		}
		return &InitResult{Message: "hdf initialized (branch shared-host)"}, nil
	}
	result, err := app.FinishInit()
	if err != nil {
		t.Fatalf("FinishInit: %v", err)
	}
	if !finishCalled {
		t.Fatal("computeFinishInitFn was not called")
	}
	if result.Message != "hdf initialized (branch shared-host)" {
		t.Errorf("Message = %q, want the init result message", result.Message)
	}
	if app.initPending != nil {
		t.Errorf("initPending = %+v, want nil after FinishInit", app.initPending)
	}
}

// TestAppResolveBranchCollision_WithoutStartInitReturnsError verifies that
// ResolveBranchCollision rejects being called before StartInitLocal/Remote
// has populated pending state.
func TestAppResolveBranchCollision_WithoutStartInitReturnsError(t *testing.T) {
	origResolve := computeResolveBranchCollisionFn
	defer func() { computeResolveBranchCollisionFn = origResolve }()

	var called bool
	computeResolveBranchCollisionFn = func(p *pendingInit, useUnique bool) error {
		called = true
		return nil
	}

	app := &App{}
	if err := app.ResolveBranchCollision(true); err == nil {
		t.Fatal("expected error when ResolveBranchCollision is called without a prior StartInit*, got nil")
	}
	if called {
		t.Error("computeResolveBranchCollisionFn should not be called without pending init state")
	}
}

// TestAppFinishInit_WithoutStartInitReturnsError verifies that FinishInit
// rejects being called before StartInitLocal/Remote has populated pending
// state, rather than applying a nil pendingInit.
func TestAppFinishInit_WithoutStartInitReturnsError(t *testing.T) {
	origFinish := computeFinishInitFn
	defer func() { computeFinishInitFn = origFinish }()

	var called bool
	computeFinishInitFn = func(cfgPath, statePath string, p *pendingInit) (*InitResult, error) {
		called = true
		return &InitResult{}, nil
	}

	app := &App{}
	if _, err := app.FinishInit(); err == nil {
		t.Fatal("expected error when FinishInit is called without a prior StartInit*, got nil")
	}
	if called {
		t.Error("computeFinishInitFn should not be called without pending init state")
	}
}

// TestAppStartInitLocal_FailureInvalidatesPriorPending verifies that a
// StartInitLocal call which fails clears any initPending left over from an
// earlier successful StartInit*, rather than leaving it applicable via
// ResolveBranchCollision/FinishInit — the same session-invalidation fix
// applied to StartLink/StartEnroll after code review.
func TestAppStartInitLocal_FailureInvalidatesPriorPending(t *testing.T) {
	origLocal := computeInitLocalStartFn
	origFinish := computeFinishInitFn
	defer func() {
		computeInitLocalStartFn = origLocal
		computeFinishInitFn = origFinish
	}()

	app := &App{initPending: &pendingInit{repoPath: "/repo", branch: sharedHostBranch}}

	computeInitLocalStartFn = func(cfgPath, repoPath, pushTarget string) (*InitStartInfo, *pendingInit, error) {
		return nil, nil, errors.New("hdf is already initialized")
	}
	if _, err := app.StartInitLocal("/repo", ""); err == nil {
		t.Fatal("expected StartInitLocal to propagate the computeInitLocalStartFn error, got nil")
	}

	var called bool
	computeFinishInitFn = func(cfgPath, statePath string, p *pendingInit) (*InitResult, error) {
		called = true
		return &InitResult{}, nil
	}
	if _, err := app.FinishInit(); err == nil {
		t.Fatal("expected FinishInit to reject the stale session after a failed StartInitLocal, got nil")
	}
	if called {
		t.Error("computeFinishInitFn should not be called against a pending session from before a failed StartInitLocal")
	}
}

// TestAppStartPromote_StoresPendingForResolveAndFinish verifies that
// App.StartPromote stashes the pendingPromote returned by
// computePromoteStartFn, that ResolveDivergedFile and FinishPromote forward
// it to their respective seams, and that FinishPromote clears the stored
// state afterward.
func TestAppStartPromote_StoresPendingForResolveAndFinish(t *testing.T) {
	origStart := computePromoteStartFn
	origResolve := computeResolveDivergedFileFn
	origFinish := computeFinishPromoteFn
	defer func() {
		computePromoteStartFn = origStart
		computeResolveDivergedFileFn = origResolve
		computeFinishPromoteFn = origFinish
	}()

	wantPending := &pendingPromote{preferTheirs: map[string]bool{}}
	computePromoteStartFn = func(cfgPath, statePath, homeDir string) (*PromoteStartInfo, *pendingPromote, error) {
		return &PromoteStartInfo{Diverged: []DivergedFile{{Path: tildeA}}}, wantPending, nil
	}

	app := &App{}
	info, err := app.StartPromote()
	if err != nil {
		t.Fatalf("StartPromote: %v", err)
	}
	if len(info.Diverged) != 1 || info.Diverged[0].Path != tildeA {
		t.Errorf("Diverged = %+v, want one entry for %s", info.Diverged, tildeA)
	}

	var gotIndex int
	var gotKeepMine bool
	var resolveCalled bool
	computeResolveDivergedFileFn = func(p *pendingPromote, index int, keepMine bool) error {
		resolveCalled = true
		gotIndex = index
		gotKeepMine = keepMine
		if p != wantPending {
			t.Errorf("computeResolveDivergedFileFn got %+v, want the stored pendingPromote", p)
		}
		return nil
	}
	if err := app.ResolveDivergedFile(0, true); err != nil {
		t.Fatalf("ResolveDivergedFile: %v", err)
	}
	if !resolveCalled || gotIndex != 0 || !gotKeepMine {
		t.Error("computeResolveDivergedFileFn was not called with index=0, keepMine=true")
	}

	var finishCalled bool
	computeFinishPromoteFn = func(p *pendingPromote) (*PromoteResult, error) {
		finishCalled = true
		if p != wantPending {
			t.Errorf("computeFinishPromoteFn got %+v, want the stored pendingPromote", p)
		}
		return &PromoteResult{Message: "Promoted machine → main and pushed to origin."}, nil
	}
	result, err := app.FinishPromote()
	if err != nil {
		t.Fatalf("FinishPromote: %v", err)
	}
	if !finishCalled {
		t.Fatal("computeFinishPromoteFn was not called")
	}
	if result.Message != "Promoted machine → main and pushed to origin." {
		t.Errorf("Message = %q, want the promote result message", result.Message)
	}
	if app.promotePending != nil {
		t.Errorf("promotePending = %+v, want nil after FinishPromote", app.promotePending)
	}
}

// TestAppResolveDivergedFile_WithoutStartPromoteReturnsError verifies that
// ResolveDivergedFile rejects being called before StartPromote has
// populated pending state.
func TestAppResolveDivergedFile_WithoutStartPromoteReturnsError(t *testing.T) {
	origResolve := computeResolveDivergedFileFn
	defer func() { computeResolveDivergedFileFn = origResolve }()

	var called bool
	computeResolveDivergedFileFn = func(p *pendingPromote, index int, keepMine bool) error {
		called = true
		return nil
	}

	app := &App{}
	if err := app.ResolveDivergedFile(0, true); err == nil {
		t.Fatal("expected error when ResolveDivergedFile is called without a prior StartPromote, got nil")
	}
	if called {
		t.Error("computeResolveDivergedFileFn should not be called without pending promote state")
	}
}

// TestAppFinishPromote_WithoutStartPromoteReturnsError verifies that
// FinishPromote rejects being called before StartPromote has populated
// pending state, rather than applying a nil pendingPromote.
func TestAppFinishPromote_WithoutStartPromoteReturnsError(t *testing.T) {
	origFinish := computeFinishPromoteFn
	defer func() { computeFinishPromoteFn = origFinish }()

	var called bool
	computeFinishPromoteFn = func(p *pendingPromote) (*PromoteResult, error) {
		called = true
		return &PromoteResult{}, nil
	}

	app := &App{}
	if _, err := app.FinishPromote(); err == nil {
		t.Fatal("expected error when FinishPromote is called without a prior StartPromote, got nil")
	}
	if called {
		t.Error("computeFinishPromoteFn should not be called without pending promote state")
	}
}

// TestAppStartPromote_FailureInvalidatesPriorPending verifies that a
// StartPromote call which fails clears any promotePending left over from an
// earlier successful StartPromote, rather than leaving it applicable via
// ResolveDivergedFile/FinishPromote.
func TestAppStartPromote_FailureInvalidatesPriorPending(t *testing.T) {
	origStart := computePromoteStartFn
	origFinish := computeFinishPromoteFn
	defer func() {
		computePromoteStartFn = origStart
		computeFinishPromoteFn = origFinish
	}()

	app := &App{promotePending: &pendingPromote{preferTheirs: map[string]bool{}}}

	computePromoteStartFn = func(cfgPath, statePath, homeDir string) (*PromoteStartInfo, *pendingPromote, error) {
		return nil, nil, errors.New("uncommitted changes")
	}
	if _, err := app.StartPromote(); err == nil {
		t.Fatal("expected StartPromote to propagate the computePromoteStartFn error, got nil")
	}

	var called bool
	computeFinishPromoteFn = func(p *pendingPromote) (*PromoteResult, error) {
		called = true
		return &PromoteResult{}, nil
	}
	if _, err := app.FinishPromote(); err == nil {
		t.Fatal("expected FinishPromote to reject the stale session after a failed StartPromote, got nil")
	}
	if called {
		t.Error("computeFinishPromoteFn should not be called against a pending session from before a failed StartPromote")
	}
}
