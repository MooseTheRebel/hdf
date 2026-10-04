package plugin

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	goplugin "github.com/hashicorp/go-plugin"
)

func TestFind(t *testing.T) {
	const exeDir, linux = "/opt/hdf", "linux"
	notOnPath := func(string) (string, error) { return "", exec.ErrNotFound }
	onPath := func(bin string) (string, error) { return "/usr/bin/" + bin, nil }
	only := func(want string) func(string) bool {
		return func(p string) bool { return p == want }
	}
	none := func(string) bool { return false }

	tests := []struct {
		name     string
		exeDir   string
		goos     string
		isExec   func(string) bool
		lookPath func(string) (string, error)
		wantPath string
		wantOK   bool
	}{
		{
			name:     "next to hdf",
			exeDir:   exeDir,
			goos:     linux,
			isExec:   only(filepath.Join(exeDir, "hdf-gui-vanilla")),
			lookPath: onPath,
			wantPath: filepath.Join(exeDir, "hdf-gui-vanilla"),
			wantOK:   true,
		},
		{
			name:     "inside macOS app bundle",
			exeDir:   exeDir,
			goos:     "darwin",
			isExec:   only(filepath.Join(exeDir, "hdf-gui-vanilla.app", "Contents", "MacOS", "hdf-gui-vanilla")),
			lookPath: notOnPath,
			wantPath: filepath.Join(exeDir, "hdf-gui-vanilla.app", "Contents", "MacOS", "hdf-gui-vanilla"),
			wantOK:   true,
		},
		{
			name:     "app bundle ignored off macOS",
			exeDir:   exeDir,
			goos:     linux,
			isExec:   only(filepath.Join(exeDir, "hdf-gui-vanilla.app", "Contents", "MacOS", "hdf-gui-vanilla")),
			lookPath: notOnPath,
			wantOK:   false,
		},
		{
			name:     "falls back to PATH",
			exeDir:   exeDir,
			goos:     linux,
			isExec:   none,
			lookPath: onPath,
			wantPath: "/usr/bin/hdf-gui-vanilla",
			wantOK:   true,
		},
		{
			name:     "windows adds .exe",
			exeDir:   "",
			goos:     "windows",
			isExec:   none,
			lookPath: onPath,
			wantPath: "/usr/bin/hdf-gui-vanilla.exe",
			wantOK:   true,
		},
		{
			name:     "headless: no plugin anywhere",
			exeDir:   exeDir,
			goos:     linux,
			isExec:   none,
			lookPath: notOnPath,
			wantOK:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := find("vanilla", tt.exeDir, tt.goos, tt.isExec, tt.lookPath)
			if ok != tt.wantOK || got != tt.wantPath {
				t.Errorf("find() = (%q, %v), want (%q, %v)", got, ok, tt.wantPath, tt.wantOK)
			}
		})
	}
}

type fakeUI struct {
	gotArgs []string
	err     error
}

func (f *fakeUI) Launch(args []string) error {
	f.gotArgs = args
	return f.err
}

func dispenseUI(t *testing.T, impl UI) UI {
	t.Helper()
	client, _ := goplugin.TestPluginRPCConn(t, goplugin.PluginSet{UIName: &uiPlugin{Impl: impl}}, nil)
	t.Cleanup(func() { _ = client.Close() })
	raw, err := client.Dispense(UIName)
	if err != nil {
		t.Fatalf("Dispense: %v", err)
	}
	return raw.(UI)
}

func TestLaunchOverRPC(t *testing.T) {
	impl := &fakeUI{}
	ui := dispenseUI(t, impl)
	if err := ui.Launch([]string{"a", "b"}); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if !reflect.DeepEqual(impl.gotArgs, []string{"a", "b"}) {
		t.Errorf("plugin got args %v, want [a b]", impl.gotArgs)
	}
}

func TestMainThreadUIRunsLaunchOnServingGoroutine(t *testing.T) {
	reqs := make(chan launchRequest)
	impl := &fakeUI{err: errors.New("closed")}
	ui := dispenseUI(t, mainThreadUI{reqs: reqs})
	result := make(chan error, 1)
	go func() {
		result <- ui.Launch([]string{"x"})
		close(reqs)
	}()

	runLaunches(impl, reqs) // serves the call on this goroutine until reqs closes

	if err := <-result; err == nil || err.Error() != "closed" {
		t.Errorf("Launch error = %v, want %q", err, "closed")
	}
	if !reflect.DeepEqual(impl.gotArgs, []string{"x"}) {
		t.Errorf("impl got args %v, want [x]", impl.gotArgs)
	}
}

func TestLaunchOverRPCReturnsPluginError(t *testing.T) {
	ui := dispenseUI(t, &fakeUI{err: errors.New("no display")})
	err := ui.Launch(nil)
	if err == nil || err.Error() != "no display" {
		t.Errorf("Launch error = %v, want %q", err, "no display")
	}
}

func TestDropDebugLines(t *testing.T) {
	var out bytes.Buffer
	w := dropDebugLines{w: &out}
	for _, line := range []string{
		"2026/10/03 21:42:14 [DEBUG] plugin: plugin server: accept unix /tmp/x: use of closed network connection\n",
		"2026/10/03 21:42:14 [TRACE] noise\n",
		"2026/10/03 21:42:14 [WARN] fetchDiff: HTTP 500 from https://example.com\n",
		"2026/10/03 21:42:14 [ERR] plugin: plugin server: accept failed\n",
	} {
		if n, err := w.Write([]byte(line)); n != len(line) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", line, n, err)
		}
	}
	want := "2026/10/03 21:42:14 [WARN] fetchDiff: HTTP 500 from https://example.com\n" +
		"2026/10/03 21:42:14 [ERR] plugin: plugin server: accept failed\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}
