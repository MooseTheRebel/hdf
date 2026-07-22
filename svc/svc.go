// Package svc installs and controls hdf's sync daemon as a per-user
// background service (launchd on macOS, systemd on Linux) via
// github.com/kardianos/service.
package svc

import (
	"context"
	"errors"
	"fmt"
	"hdf/config"
	"hdf/daemon"
	"hdf/eventlog"
	"os"
	"time"

	kservice "github.com/kardianos/service"
)

// ServiceName is the reverse-DNS identifier used for the launchd plist /
// systemd unit.
const ServiceName = "com.moosetherebel.hdf"

// RunSubcommand is the cobra subcommand the installed service re-invokes
// the binary with ("hdf daemon run").
const RunSubcommand = "run"

func buildConfig() *kservice.Config {
	return &kservice.Config{
		Name:        ServiceName,
		DisplayName: "hdf sync daemon",
		Description: "Syncs dotfiles in the background",
		Arguments:   []string{"daemon", RunSubcommand},
		Option: kservice.KeyValue{
			"UserService": true,
			"RunAtLoad":   true,
		},
	}
}

// program adapts daemon.Run to kservice.Interface.
type program struct {
	cfgPath string
	cancel  context.CancelFunc
	done    chan struct{}
}

// runFn is a seam over daemon.Run for tests.
var runFn = daemon.Run

// exitFn is a seam over os.Exit for tests.
var exitFn = os.Exit

// statePathFn is a seam over config.DefaultStatePath for tests.
var statePathFn = config.DefaultStatePath

// runDaemonLoop runs the sync loop and, if it exits with anything other
// than a graceful Stop, exits the process. Without this, an unexpected
// daemon.Run error (e.g. the dotfiles repo disappearing) would leave the
// goroutine dead while s.Run() keeps blocking on the signal channel, so
// the OS service manager keeps reporting "running" and never restarts it.
//
// A graceful Stop is detected two ways: the error wraps context.Canceled,
// or ctx.Err() is already non-nil (the context was canceled even though
// the returned error doesn't wrap it — e.g. a platform-specific "use of
// closed network connection" from an operation cut short mid-flight).
func runDaemonLoop(ctx context.Context, cfgPath string) {
	err := runFn(ctx, cfgPath)
	if err != nil && !errors.Is(err, context.Canceled) && ctx.Err() == nil {
		msg := fmt.Sprintf("hdf daemon exited unexpectedly: %v", err)
		fmt.Fprintf(os.Stderr, "%s\n", msg)
		statePath := statePathFn()
		_ = config.SetPendingCrash(statePath, msg)
		_ = eventlog.Append(eventlog.PathFor(statePath), "daemon_crash", err.Error())
		exitFn(1)
	}
}

// Start is called by the OS service manager. It must not block.
func (p *program) Start(s kservice.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})
	go func() {
		defer close(p.done)
		runDaemonLoop(ctx, p.cfgPath)
	}()
	return nil
}

// Stop is called by the OS service manager. It must return within a few seconds.
func (p *program) Stop(s kservice.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	if p.done != nil {
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-p.done:
		case <-timer.C:
		}
	}
	return nil
}

// newService is a seam for tests to substitute a fake kservice.Service
// instead of touching a real OS service manager.
var newService = kservice.New

func buildService(cfgPath string) (kservice.Service, error) {
	return newService(&program{cfgPath: cfgPath}, buildConfig())
}

// Run runs the sync daemon via the service runner, blocking until stopped
// by an OS signal (SIGINT/SIGTERM) or, when running under a service
// manager, a stop request — either path calls program.Stop for a graceful
// shutdown instead of an abrupt kill mid-sync.
func Run(cfgPath string) error {
	s, err := buildService(cfgPath)
	if err != nil {
		return fmt.Errorf("building service: %w", err)
	}
	return s.Run()
}

// Install installs hdf's sync daemon as a per-user background service and
// starts it immediately.
func Install(cfgPath string) error {
	s, err := buildService(cfgPath)
	if err != nil {
		return fmt.Errorf("building service: %w", err)
	}
	if err := s.Install(); err != nil {
		return fmt.Errorf("installing service: %w", err)
	}
	if err := s.Start(); err != nil {
		return fmt.Errorf("starting service: %w", err)
	}
	return nil
}

// Uninstall stops and removes the installed service. A failure to stop
// (e.g. it wasn't running) does not prevent uninstall, and uninstalling an
// already-uninstalled service is treated as success (idempotent).
func Uninstall(cfgPath string) error {
	s, err := buildService(cfgPath)
	if err != nil {
		return fmt.Errorf("building service: %w", err)
	}
	_ = s.Stop()
	if err := s.Uninstall(); err != nil && !errors.Is(err, kservice.ErrNotInstalled) {
		return fmt.Errorf("uninstalling service: %w", err)
	}
	return nil
}

// Start starts an already-installed service.
func Start(cfgPath string) error {
	s, err := buildService(cfgPath)
	if err != nil {
		return fmt.Errorf("building service: %w", err)
	}
	if err := s.Start(); err != nil {
		return fmt.Errorf("starting service: %w", err)
	}
	return nil
}

// Stop stops an already-installed service.
func Stop(cfgPath string) error {
	s, err := buildService(cfgPath)
	if err != nil {
		return fmt.Errorf("building service: %w", err)
	}
	if err := s.Stop(); err != nil {
		return fmt.Errorf("stopping service: %w", err)
	}
	return nil
}

// Status reports whether the service is "not installed", "stopped", or
// "running". "not installed" is an expected state, not an error.
func Status(cfgPath string) (string, error) {
	s, err := buildService(cfgPath)
	if err != nil {
		return "", fmt.Errorf("building service: %w", err)
	}
	status, err := s.Status()
	if errors.Is(err, kservice.ErrNotInstalled) {
		return "not installed", nil
	}
	if err != nil {
		return "", fmt.Errorf("checking status: %w", err)
	}
	switch status {
	case kservice.StatusRunning:
		return "running", nil
	case kservice.StatusStopped:
		return "stopped", nil
	case kservice.StatusUnknown:
		return "unknown", nil
	default:
		return "unknown", nil
	}
}
