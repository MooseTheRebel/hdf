package svc

import (
	"context"
	"errors"
	"slices"
	"testing"

	kservice "github.com/kardianos/service"
)

// fakeService implements kservice.Service and records calls for tests,
// without touching any real OS service manager.
type fakeService struct {
	calls []string

	installErr   error
	uninstallErr error
	startErr     error
	stopErr      error
	runErr       error
	statusVal    kservice.Status
	statusErr    error
}

func (f *fakeService) Run() error {
	f.calls = append(f.calls, "run")
	return f.runErr
}

func (f *fakeService) Start() error {
	f.calls = append(f.calls, "start")
	return f.startErr
}

func (f *fakeService) Stop() error {
	f.calls = append(f.calls, "stop")
	return f.stopErr
}
func (f *fakeService) Restart() error { return nil }
func (f *fakeService) Install() error {
	f.calls = append(f.calls, "install")
	return f.installErr
}

func (f *fakeService) Uninstall() error {
	f.calls = append(f.calls, "uninstall")
	return f.uninstallErr
}
func (f *fakeService) Logger(errs chan<- error) (kservice.Logger, error)       { return nil, nil }
func (f *fakeService) SystemLogger(errs chan<- error) (kservice.Logger, error) { return nil, nil }
func (f *fakeService) String() string                                          { return "hdf" }
func (f *fakeService) Platform() string                                        { return "test" }
func (f *fakeService) Status() (kservice.Status, error) {
	return f.statusVal, f.statusErr
}

// withFakeService overrides newService for the duration of a test.
func withFakeService(t *testing.T, fake *fakeService) {
	t.Helper()
	orig := newService
	newService = func(kservice.Interface, *kservice.Config) (kservice.Service, error) {
		return fake, nil
	}
	t.Cleanup(func() { newService = orig })
}

func TestBuildConfig_Fields(t *testing.T) {
	cfg := buildConfig()

	if cfg.Name != "com.moosetherebel.hdf" {
		t.Errorf("Name = %q, want %q", cfg.Name, "com.moosetherebel.hdf")
	}
	wantArgs := []string{"daemon", RunSubcommand}
	if len(cfg.Arguments) != len(wantArgs) {
		t.Fatalf("Arguments = %v, want %v", cfg.Arguments, wantArgs)
	}
	for i, a := range wantArgs {
		if cfg.Arguments[i] != a {
			t.Errorf("Arguments[%d] = %q, want %q", i, cfg.Arguments[i], a)
		}
	}
	if userService, _ := cfg.Option["UserService"].(bool); !userService {
		t.Errorf("Option[UserService] = %v, want true", cfg.Option["UserService"])
	}
	if runAtLoad, _ := cfg.Option["RunAtLoad"].(bool); !runAtLoad {
		t.Errorf("Option[RunAtLoad] = %v, want true", cfg.Option["RunAtLoad"])
	}
}

func TestInstallUninstall(t *testing.T) {
	cases := []struct {
		name        string
		fake        *fakeService
		call        func(cfgPath string) error
		wantErr     bool
		wantCalls   []string // exact expected call sequence, checked when non-nil
		mustHave    string   // a call that must still happen despite an earlier error
		mustNotHave string   // a call that must not happen when the sequence aborts early
	}{
		{
			name:      "Install installs then starts",
			fake:      &fakeService{},
			call:      Install,
			wantCalls: []string{"install", "start"},
		},
		{
			name:        "Install propagates install error without starting",
			fake:        &fakeService{installErr: errors.New("already installed")},
			call:        Install,
			wantErr:     true,
			mustNotHave: "start",
		},
		{
			name:      "Uninstall stops then uninstalls",
			fake:      &fakeService{},
			call:      Uninstall,
			wantCalls: []string{"stop", "uninstall"},
		},
		{
			name:     "Uninstall ignores stop error and still uninstalls",
			fake:     &fakeService{stopErr: errors.New("not running")},
			call:     Uninstall,
			mustHave: "uninstall",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withFakeService(t, tc.fake)

			err := tc.call("/cfg")
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
			if tc.wantCalls != nil && !slices.Equal(tc.fake.calls, tc.wantCalls) {
				t.Errorf("calls = %v, want %v", tc.fake.calls, tc.wantCalls)
			}
			if tc.mustHave != "" && !slices.Contains(tc.fake.calls, tc.mustHave) {
				t.Errorf("expected calls to include %q, got %v", tc.mustHave, tc.fake.calls)
			}
			if tc.mustNotHave != "" && slices.Contains(tc.fake.calls, tc.mustNotHave) {
				t.Errorf("expected calls not to include %q, got %v", tc.mustNotHave, tc.fake.calls)
			}
		})
	}
}

func TestStartStop_Delegate(t *testing.T) {
	cases := []struct {
		name string
		fake *fakeService
		call func(cfgPath string) error
	}{
		{name: "Start", fake: &fakeService{startErr: errors.New("boom")}, call: Start},
		{name: "Stop", fake: &fakeService{stopErr: errors.New("boom")}, call: Stop},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withFakeService(t, tc.fake)
			if err := tc.call("/cfg"); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestStatus(t *testing.T) {
	cases := []struct {
		name      string
		statusVal kservice.Status
		statusErr error
		want      string
		wantErr   bool
	}{
		{name: "service is running", statusVal: kservice.StatusRunning, want: "running"},
		{name: "service is stopped", statusVal: kservice.StatusStopped, want: "stopped"},
		{name: "not installed is not an error", statusErr: kservice.ErrNotInstalled, want: "not installed"},
		{name: "other error propagates", statusErr: errors.New("dbus unreachable"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeService{statusVal: tc.statusVal, statusErr: tc.statusErr}
			withFakeService(t, fake)

			got, err := Status("/cfg")
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Status() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("Status() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRun(t *testing.T) {
	cases := []struct {
		name    string
		fake    *fakeService
		wantErr bool
	}{
		{name: "delegates to service Run", fake: &fakeService{}},
		{name: "propagates error", fake: &fakeService{runErr: errors.New("boom")}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withFakeService(t, tc.fake)

			err := Run("/cfg")
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error = %v, want nil", err)
			}
			if !slices.Equal(tc.fake.calls, []string{"run"}) {
				t.Errorf("calls = %v, want [run]", tc.fake.calls)
			}
		})
	}
}

func TestRunDaemonLoop(t *testing.T) {
	cases := []struct {
		name      string
		runErr    error
		ctxCancel bool // cancel ctx before calling runDaemonLoop
		wantExit  bool
	}{
		{name: "exits process on unexpected error", runErr: errors.New("repo gone"), wantExit: true},
		{name: "does not exit on graceful context.Canceled", runErr: context.Canceled},
		{
			// Covers a daemon.Run error that doesn't wrap context.Canceled
			// (e.g. a platform-specific "use of closed network connection"
			// from an in-flight operation cut short by Stop), but where the
			// context itself shows the shutdown was already requested —
			// that should still be treated as graceful, not a crash.
			name:      "does not exit when context already canceled",
			runErr:    errors.New("use of closed network connection"),
			ctxCancel: true,
		},
		{name: "does not exit on nil error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			origRun, origExit := runFn, exitFn
			defer func() { runFn, exitFn = origRun, origExit }()

			runFn = func(context.Context, string) error { return tc.runErr }
			var gotCode int
			exitCalled := false
			exitFn = func(code int) {
				exitCalled = true
				gotCode = code
			}

			ctx, cancel := context.WithCancel(context.Background())
			if tc.ctxCancel {
				cancel()
			} else {
				defer cancel()
			}

			runDaemonLoop(ctx, "/cfg")

			if tc.wantExit != exitCalled {
				t.Errorf("exitFn called = %v, want %v", exitCalled, tc.wantExit)
			}
			if tc.wantExit && gotCode != 1 {
				t.Errorf("exit code = %d, want 1", gotCode)
			}
		})
	}
}
