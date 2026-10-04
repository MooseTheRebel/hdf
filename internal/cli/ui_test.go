package cli

import (
	"bufio"
	"bytes"
	"errors"
	"hdf/config"
	"path/filepath"
	"strings"
	"testing"
)

// fakeUI records whether it was launched.
type fakeUI struct {
	launched bool
	err      error
}

func (f *fakeUI) Launch([]string) error {
	f.launched = true

	return f.err
}

func TestLaunchUI(t *testing.T) {
	const (
		vanilla, vuejs         = "vanilla", "vuejs"
		vanillaPath, vuejsPath = "/opt/hdf/hdf-gui-vanilla", "/usr/bin/hdf-gui-vuejs"
		selfPath               = "/home/u/Downloads/hdf-gui-vanilla"
	)

	launchErr := errors.New("plugin failed")

	tests := []struct {
		name         string
		builtIn      string // name of the GUI built into this binary, "" for plain hdf
		installed    []string
		launchErr    error
		wantLaunch   string // plugin path launched, "" for none
		wantBuiltIn  bool   // built-in GUI launched
		wantErr      []string
		wantWelcome  bool
		wantNoOutput bool
	}{
		{name: "plain hdf, no GUI: welcome", wantWelcome: true},
		{name: "plain hdf launches the installed GUI", installed: []string{vanilla}, wantLaunch: vanillaPath, wantNoOutput: true},
		{name: "plain hdf launches vuejs when it's the one installed", installed: []string{vuejs}, wantLaunch: vuejsPath, wantNoOutput: true},
		{name: "plugin error is returned", installed: []string{vanilla}, launchErr: launchErr, wantLaunch: vanillaPath, wantErr: []string{"plugin failed"}},
		{
			name: "both GUIs installed: error naming both", installed: []string{vanilla, vuejs},
			wantErr: []string{"only one is allowed", "uninstall", "hdf-gui-vanilla", vanillaPath, "hdf-gui-vuejs", vuejsPath},
		},
		{name: "built-in GUI opens directly", builtIn: vanilla, wantBuiltIn: true, wantNoOutput: true},
		{name: "built-in GUI finding itself is fine", builtIn: vanilla, installed: []string{vanilla}, wantBuiltIn: true},
		{
			name: "built-in GUI with the other GUI installed: error", builtIn: vanilla, installed: []string{vuejs},
			wantErr: []string{"only one is allowed", selfPath, vuejsPath},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := map[string]string{vanilla: vanillaPath, vuejs: vuejsPath}
			find := func(name string) (string, bool) {
				for _, n := range tt.installed {
					if n == name {
						return paths[name], true
					}
				}

				return "", false
			}

			launched := ""
			launch := func(path string, _ []string) error {
				launched = path

				return tt.launchErr
			}

			var built *builtInGUI

			ui := &fakeUI{}
			if tt.builtIn != "" {
				built = &builtInGUI{name: tt.builtIn, path: selfPath, ui: ui}
			}

			var out bytes.Buffer

			err := launchUI(&out, built, find, launch)

			if len(tt.wantErr) == 0 && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}

			for _, want := range tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("err = %v, want it to contain %q", err, want)
				}
			}

			if launched != tt.wantLaunch {
				t.Errorf("launched plugin %q, want %q", launched, tt.wantLaunch)
			}

			if ui.launched != tt.wantBuiltIn {
				t.Errorf("built-in GUI launched = %v, want %v", ui.launched, tt.wantBuiltIn)
			}

			if got := strings.Contains(out.String(), "hdf init"); got != tt.wantWelcome {
				t.Errorf("welcome printed = %v, want %v (output %q)", got, tt.wantWelcome, out.String())
			}

			if tt.wantNoOutput && out.Len() != 0 {
				t.Errorf("output = %q, want none", out.String())
			}
		})
	}
}

func TestWelcomeText(t *testing.T) {
	origVersion := version
	defer func() { version = origVersion }()

	version = "0.3.1"

	linux := welcomeText("linux", "amd64")
	for _, want := range []string{"hdf 0.3.1", "hdf init", "hdf --help", "hdf-gui-vanilla or hdf-gui-vuejs (only one)", "https://github.com/MooseTheRebel/hdf/releases/tag/v0.3.1"} {
		if !strings.Contains(linux, want) {
			t.Errorf("linux/amd64 welcome missing %q:\n%s", want, linux)
		}
	}

	mac := welcomeText("darwin", "arm64")
	if !strings.Contains(mac, "hdf's GUI isn't available for macOS yet.") {
		t.Errorf("darwin/arm64 welcome should say there's no GUI yet:\n%s", mac)
	}

	if strings.Contains(mac, "download") {
		t.Errorf("darwin/arm64 welcome must not suggest a download that won't run:\n%s", mac)
	}
}

func TestReleasesURL(t *testing.T) {
	origVersion := version
	defer func() { version = origVersion }()

	version = "dev"
	if got := releasesURL(); got != "https://github.com/MooseTheRebel/hdf/releases/latest" {
		t.Errorf("dev releasesURL() = %q", got)
	}

	version = "0.3.1"
	if got := releasesURL(); got != "https://github.com/MooseTheRebel/hdf/releases/tag/v0.3.1" {
		t.Errorf("0.3.1 releasesURL() = %q", got)
	}
}

// TestPromptPendingCrashIfInteractive verifies a pending crash is kept,
// not silently consumed, when there's no terminal to answer the prompt.
func TestPromptPendingCrashIfInteractive(t *testing.T) {
	const crash = "panic: crash for this test"

	statePath := filepath.Join(t.TempDir(), "state.toml")
	if err := config.SetPendingCrash(statePath, crash); err != nil {
		t.Fatal(err)
	}

	if err := promptPendingCrashIfInteractive(statePath, false, strings.NewReader("")); err != nil {
		t.Fatalf("non-interactive: %v", err)
	}

	msg, err := config.TakePendingCrash(statePath)
	if err != nil || msg != crash {
		t.Fatalf("after non-interactive run, pending crash = %q, %v; want it kept", msg, err)
	}

	// Interactive and declined: consumed, as before.
	if err := config.SetPendingCrash(statePath, crash); err != nil {
		t.Fatal(err)
	}

	if err := promptPendingCrash(statePath, bufio.NewReader(strings.NewReader("n\n"))); err != nil {
		t.Fatal(err)
	}

	if msg, _ := config.TakePendingCrash(statePath); msg != "" {
		t.Errorf("after declining interactively, pending crash = %q, want consumed", msg)
	}
}

func TestInstallLocation(t *testing.T) {
	for path, want := range map[string]string{
		"/Applications/hdf-gui-vuejs.app/Contents/MacOS/hdf-gui-vuejs": "/Applications/hdf-gui-vuejs.app",
		"/usr/bin/hdf-gui-vanilla":                                     "/usr/bin/hdf-gui-vanilla",
	} {
		if got := installLocation(path); got != want {
			t.Errorf("installLocation(%q) = %q, want %q", path, got, want)
		}
	}
}

// TestRootHasNoGUIFlag: with one GUI allowed at a time there's nothing to
// choose, so --gui is gone.
func TestRootHasNoGUIFlag(t *testing.T) {
	if rootCmd.Flags().Lookup("gui") != nil {
		t.Error("rootCmd still defines --gui")
	}
}
