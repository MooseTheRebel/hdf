package plugin

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHostExecutable(t *testing.T) {
	t.Setenv(HostExecutableEnv, "/usr/local/bin/hdf")

	t.Setenv(Handshake.MagicCookieKey, "")

	if got := HostExecutable(); got != "" {
		t.Errorf("outside a plugin: HostExecutable() = %q, want empty", got)
	}

	t.Setenv(Handshake.MagicCookieKey, Handshake.MagicCookieValue)

	if got := HostExecutable(); got != "/usr/local/bin/hdf" {
		t.Errorf("inside a plugin: HostExecutable() = %q, want /usr/local/bin/hdf", got)
	}
}

// syncBuffer is a bytes.Buffer safe for go-plugin's concurrent copiers.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// waitFor polls until buf contains want, since go-plugin copies a plugin's
// output asynchronously.
func waitFor(t *testing.T, name string, buf *syncBuffer, want string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(buf.String(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("%s = %q, want it to contain %q", name, buf.String(), want)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func buildTestPlugin(t *testing.T) string {
	t.Helper()

	if testing.Short() {
		t.Skip("builds a plugin binary")
	}

	bin := filepath.Join(t.TempDir(), "reportplugin")

	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin, "./testdata/reportplugin") //nolint:gosec // bin is this test's own temp path
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building test plugin: %v\n%s", err, out)
	}

	return bin
}

func TestLaunch_PassesHostExecutableAndShowsOutput(t *testing.T) {
	bin := buildTestPlugin(t)
	// A stale inherited value must not win over hdf's own path.
	t.Setenv(HostExecutableEnv, "/stale/hdf")

	var stdout, stderr syncBuffer

	err := launch(bin, nil, &stdout, &stderr)

	exe, _ := os.Executable()
	if err == nil || err.Error() != "host="+exe {
		t.Errorf("Launch error = %v, want host=%s", err, exe)
	}

	waitFor(t, "stdout", &stdout, "plugin-stdout")
	waitFor(t, "stderr", &stderr, "plugin-stderr")

	// A normal shutdown must not surface go-plugin's own chatter.
	if got := stderr.String(); strings.Contains(got, "plugin:") || strings.Contains(got, "@level") {
		t.Errorf("stderr has go-plugin noise:\n%s", got)
	}
}

func TestLaunch_ShowsPluginPanicOnce(t *testing.T) {
	bin := buildTestPlugin(t)

	var stdout, stderr syncBuffer
	err := launch(bin, []string{"panic"}, &stdout, &stderr)
	if err == nil {
		t.Error("Launch error = nil, want an error after the plugin panicked")
	}

	waitFor(t, "stderr", &stderr, "panic: plugin-panic")
	time.Sleep(100 * time.Millisecond)

	if n := strings.Count(stderr.String(), "plugin-panic"); n != 1 {
		t.Errorf("panic shown %d times, want once:\n%s", n, stderr.String())
	}
}
