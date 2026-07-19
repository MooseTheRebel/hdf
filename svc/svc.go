// Package svc installs and controls hdf's sync daemon as a per-user
// background service (launchd on macOS, systemd on Linux) via
// github.com/kardianos/service.
package svc

import (
	"context"
	"errors"
	"fmt"
	"hdf/daemon"
	"time"

	kservice "github.com/kardianos/service"
)

// ServiceName is the reverse-DNS identifier used for the launchd plist /
// systemd unit.
const ServiceName = "com.moosetherebel.hdf"

func buildConfig() *kservice.Config {
	return &kservice.Config{
		Name:        ServiceName,
		DisplayName: "hdf sync daemon",
		Description: "Syncs dotfiles in the background",
		Arguments:   []string{"daemon", "run"},
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

// Start is called by the OS service manager. It must not block.
func (p *program) Start(s kservice.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})
	go func() {
		defer close(p.done)
		_ = daemon.Run(ctx, p.cfgPath)
	}()
	return nil
}

// Stop is called by the OS service manager. It must return within a few seconds.
func (p *program) Stop(s kservice.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	if p.done != nil {
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
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
// (e.g. it wasn't running) does not prevent uninstall.
func Uninstall(cfgPath string) error {
	s, err := buildService(cfgPath)
	if err != nil {
		return fmt.Errorf("building service: %w", err)
	}
	_ = s.Stop()
	if err := s.Uninstall(); err != nil {
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
