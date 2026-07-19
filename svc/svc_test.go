package svc

import (
	"context"
	"errors"
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

func TestInstall_InstallsThenStarts(t *testing.T) {
	fake := &fakeService{}
	withFakeService(t, fake)

	if err := Install("/cfg"); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}
	want := []string{"install", "start"}
	if len(fake.calls) != len(want) || fake.calls[0] != want[0] || fake.calls[1] != want[1] {
		t.Errorf("calls = %v, want %v", fake.calls, want)
	}
}

func TestInstall_PropagatesInstallError(t *testing.T) {
	fake := &fakeService{installErr: errors.New("already installed")}
	withFakeService(t, fake)

	err := Install("/cfg")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	for _, c := range fake.calls {
		if c == "start" {
			t.Errorf("Start should not be called when Install fails, calls = %v", fake.calls)
		}
	}
}

func TestUninstall_StopsThenUninstalls(t *testing.T) {
	fake := &fakeService{}
	withFakeService(t, fake)

	if err := Uninstall("/cfg"); err != nil {
		t.Fatalf("Uninstall() error = %v, want nil", err)
	}
	want := []string{"stop", "uninstall"}
	if len(fake.calls) != len(want) || fake.calls[0] != want[0] || fake.calls[1] != want[1] {
		t.Errorf("calls = %v, want %v", fake.calls, want)
	}
}

func TestUninstall_IgnoresStopErrorAndStillUninstalls(t *testing.T) {
	fake := &fakeService{stopErr: errors.New("not running")}
	withFakeService(t, fake)

	if err := Uninstall("/cfg"); err != nil {
		t.Fatalf("Uninstall() error = %v, want nil", err)
	}
	found := false
	for _, c := range fake.calls {
		if c == "uninstall" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Uninstall to be called despite Stop error, calls = %v", fake.calls)
	}
}

func TestStart_Delegates(t *testing.T) {
	fake := &fakeService{startErr: errors.New("boom")}
	withFakeService(t, fake)

	if err := Start("/cfg"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStop_Delegates(t *testing.T) {
	fake := &fakeService{stopErr: errors.New("boom")}
	withFakeService(t, fake)

	if err := Stop("/cfg"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStatus_Running(t *testing.T) {
	fake := &fakeService{statusVal: kservice.StatusRunning}
	withFakeService(t, fake)

	got, err := Status("/cfg")
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	if got != "running" {
		t.Errorf("Status() = %q, want %q", got, "running")
	}
}

func TestStatus_Stopped(t *testing.T) {
	fake := &fakeService{statusVal: kservice.StatusStopped}
	withFakeService(t, fake)

	got, err := Status("/cfg")
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	if got != "stopped" {
		t.Errorf("Status() = %q, want %q", got, "stopped")
	}
}

func TestStatus_NotInstalled(t *testing.T) {
	fake := &fakeService{statusErr: kservice.ErrNotInstalled}
	withFakeService(t, fake)

	got, err := Status("/cfg")
	if err != nil {
		t.Fatalf("Status() error = %v, want nil (not-installed is not a command error)", err)
	}
	if got != "not installed" {
		t.Errorf("Status() = %q, want %q", got, "not installed")
	}
}

func TestStatus_OtherErrorPropagates(t *testing.T) {
	fake := &fakeService{statusErr: errors.New("dbus unreachable")}
	withFakeService(t, fake)

	_, err := Status("/cfg")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRun_DelegatesToServiceRun(t *testing.T) {
	fake := &fakeService{}
	withFakeService(t, fake)

	if err := Run("/cfg"); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if len(fake.calls) != 1 || fake.calls[0] != "run" {
		t.Errorf("calls = %v, want [run]", fake.calls)
	}
}

func TestRun_PropagatesError(t *testing.T) {
	fake := &fakeService{runErr: errors.New("boom")}
	withFakeService(t, fake)

	if err := Run("/cfg"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRunDaemonLoop_ExitsProcessOnUnexpectedError(t *testing.T) {
	origRun, origExit := runFn, exitFn
	defer func() { runFn, exitFn = origRun, origExit }()

	runFn = func(context.Context, string) error { return errors.New("repo gone") }
	var gotCode int
	exitCalled := false
	exitFn = func(code int) {
		exitCalled = true
		gotCode = code
	}

	runDaemonLoop(context.Background(), "/cfg")

	if !exitCalled {
		t.Fatal("expected exitFn to be called on unexpected daemon error")
	}
	if gotCode != 1 {
		t.Errorf("exit code = %d, want 1", gotCode)
	}
}

func TestRunDaemonLoop_DoesNotExitOnContextCanceled(t *testing.T) {
	origRun, origExit := runFn, exitFn
	defer func() { runFn, exitFn = origRun, origExit }()

	runFn = func(context.Context, string) error { return context.Canceled }
	exitCalled := false
	exitFn = func(int) { exitCalled = true }

	runDaemonLoop(context.Background(), "/cfg")

	if exitCalled {
		t.Error("expected exitFn not to be called on graceful context.Canceled")
	}
}

func TestRunDaemonLoop_DoesNotExitOnNilError(t *testing.T) {
	origRun, origExit := runFn, exitFn
	defer func() { runFn, exitFn = origRun, origExit }()

	runFn = func(context.Context, string) error { return nil }
	exitCalled := false
	exitFn = func(int) { exitCalled = true }

	runDaemonLoop(context.Background(), "/cfg")

	if exitCalled {
		t.Error("expected exitFn not to be called on nil error")
	}
}
