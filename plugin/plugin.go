// Package plugin defines hdf's plugin contract, built on
// hashicorp/go-plugin: plugins are separate executables that hdf launches
// as subprocesses and talks to over net/rpc. Nothing here needs cgo, so the
// hdf binary stays statically buildable even though a GUI plugin (Wails)
// does need cgo.
//
// A GUI plugin binary is named "hdf-gui-<name>" (see GUIs and Find) and
// calls Serve from its main. hdf runs it via Launch.
package plugin

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/rpc"
	"os"
	"os/exec"

	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
)

// GUIs are the GUI plugins hdf knows, in the order bare `hdf` tries them:
// "vanilla" is hdf-gui-vanilla (cmd/hdf-gui-vanilla, vanilla TypeScript),
// "vuejs" is hdf-gui-vuejs (cmd/hdf-gui-vuejs, planned). With none
// installed, hdf is a headless, CLI-only tool.
var GUIs = []string{"vanilla", "vuejs"}

// UIName is the name every GUI plugin serves its UI under over RPC. It's
// the same for all GUIs; which GUI runs is decided by which binary hdf
// starts.
const UIName = "ui"

// Handshake guards against running a plugin binary directly or pairing hdf
// with a plugin built for an incompatible protocol. Bump ProtocolVersion on
// any breaking change to the UI interface.
var Handshake = goplugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "HDF_PLUGIN",
	MagicCookieValue: "a6c1f0f4-hdf-ui",
}

// UI is the interface a UI plugin implements. Launch runs the UI and blocks
// until it exits. The UI plugin does its own hdf work by importing hdf's
// packages directly — the RPC boundary only carries the launch itself.
type UI interface {
	Launch(args []string) error
}

// HostExecutableEnv is the environment variable hdf sets on a plugin's
// process to hdf's own executable path (see HostExecutable).
const HostExecutableEnv = "HDF_HOST_EXECUTABLE"

// IsPluginProcess reports whether this process was started by hdf as a
// plugin (as opposed to run directly, e.g. by `wails dev`).
func IsPluginProcess() bool {
	return os.Getenv(Handshake.MagicCookieKey) == Handshake.MagicCookieValue
}

// HostExecutable returns the path of the hdf executable that launched this
// plugin, or "" when this process isn't a plugin started by hdf. Anything a
// plugin sets up to run hdf later — e.g. the sync daemon service, which
// re-runs `hdf daemon run` — must use this rather than os.Executable(),
// which is the plugin itself.
func HostExecutable() string {
	if !IsPluginProcess() {
		return ""
	}
	return os.Getenv(HostExecutableEnv)
}

// Serve serves impl as hdf's UI plugin. Call it from a plugin binary's main
// goroutine when IsPluginProcess is true; it returns once hdf disconnects.
//
// impl.Launch runs on the calling (main) goroutine rather than on
// go-plugin's RPC goroutine, because GUI toolkits — Cocoa in particular —
// only allow windows on the main thread.
func Serve(impl UI) {
	// hdf shows everything a plugin writes to stderr, so keep go-plugin's
	// routine chatter out of it: its own logger defaults to Trace, and its
	// net/rpc server logs "[DEBUG] ... use of closed network connection"
	// through the standard logger on every normal shutdown.
	log.SetOutput(dropDebugLines{w: os.Stderr})
	logger := hclog.New(&hclog.LoggerOptions{Name: "plugin", Level: hclog.Warn, Output: os.Stderr})

	reqs := make(chan launchRequest)
	go func() {
		goplugin.Serve(&goplugin.ServeConfig{
			HandshakeConfig: Handshake,
			Plugins:         goplugin.PluginSet{UIName: &uiPlugin{Impl: mainThreadUI{reqs: reqs}}},
			Logger:          logger,
		})
		close(reqs)
	}()
	runLaunches(impl, reqs)
}

// dropDebugLines passes standard-logger output through except "[DEBUG]"
// and "[TRACE]" lines (hashicorp's level-prefix convention). The standard
// logger writes each message in a single Write call.
type dropDebugLines struct {
	w io.Writer
}

func (d dropDebugLines) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("[DEBUG] ")) || bytes.Contains(p, []byte("[TRACE] ")) {
		return len(p), nil
	}
	return d.w.Write(p)
}

type launchRequest struct {
	args []string
	done chan error
}

// mainThreadUI forwards Launch calls from the RPC goroutine to runLaunches.
type mainThreadUI struct {
	reqs chan<- launchRequest
}

func (m mainThreadUI) Launch(args []string) error {
	done := make(chan error)
	m.reqs <- launchRequest{args: args, done: done}
	return <-done
}

// runLaunches serves requests sent by mainThreadUI on the calling goroutine
// until reqs is closed.
func runLaunches(impl UI, reqs <-chan launchRequest) {
	for req := range reqs {
		req.done <- impl.Launch(req.args)
	}
}

// Launch starts the plugin executable at path, calls its UI.Launch with
// args, and stops the plugin once Launch returns. Everything the plugin
// prints reaches hdf's own stdout and stderr.
func Launch(path string, args []string) error {
	return launch(path, args, os.Stdout, os.Stderr)
}

// launch is Launch with the plugin's output destinations passed in, for
// testing.
func launch(path string, args []string, stdout, stderr io.Writer) error {
	// Build the plugin's environment here, rather than letting go-plugin
	// append the host's after ours, so HostExecutableEnv always wins over
	// any inherited value.
	cmd := exec.CommandContext(context.Background(), path)
	cmd.Env = os.Environ()
	if exe, err := os.Executable(); err == nil {
		cmd.Env = append(cmd.Env, HostExecutableEnv+"="+exe)
	}

	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  Handshake,
		Plugins:          goplugin.PluginSet{UIName: &uiPlugin{}},
		Cmd:              cmd,
		SkipHostEnv:      true,
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolNetRPC},
		// go-plugin discards a plugin's output by default. SyncStdout and
		// SyncStderr carry what it writes via os.Stdout/os.Stderr; Stderr
		// carries its raw stderr, including Go panics and native crashes
		// (e.g. a Cocoa exception). go-plugin's own log would repeat
		// panics and adds nothing the returned error doesn't, so it's
		// silenced.
		SyncStdout: stdout,
		SyncStderr: stderr,
		Stderr:     stderr,
		Logger:     hclog.NewNullLogger(),
	})
	defer client.Kill()

	rpcClient, err := client.Client()
	if err != nil {
		return fmt.Errorf("starting plugin %s: %w", path, err)
	}
	raw, err := rpcClient.Dispense(UIName)
	if err != nil {
		return fmt.Errorf("loading plugin %s: %w", path, err)
	}
	return raw.(UI).Launch(args)
}

// uiPlugin adapts UI to go-plugin's net/rpc plugin interface. Impl is only
// set on the plugin side.
type uiPlugin struct {
	Impl UI
}

func (p *uiPlugin) Server(*goplugin.MuxBroker) (any, error) {
	return &uiRPCServer{impl: p.Impl}, nil
}

func (p *uiPlugin) Client(_ *goplugin.MuxBroker, c *rpc.Client) (any, error) {
	return &uiRPCClient{client: c}, nil
}

// uiRPCClient is the host-side stub for UI.
type uiRPCClient struct {
	client *rpc.Client
}

func (c *uiRPCClient) Launch(args []string) error {
	var errMsg string
	if err := c.client.Call("Plugin.Launch", args, &errMsg); err != nil {
		return err
	}
	if errMsg != "" {
		return fmt.Errorf("%s", errMsg)
	}
	return nil
}

// uiRPCServer is the plugin-side net/rpc receiver for UI. Errors travel back
// as a string because net/rpc can't serialize arbitrary error values.
type uiRPCServer struct {
	impl UI
}

func (s *uiRPCServer) Launch(args []string, errMsg *string) error {
	if err := s.impl.Launch(args); err != nil {
		*errMsg = err.Error()
	}
	return nil
}
