//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// hdfBin is the path to the compiled binary used by all e2e tests.
// It is set once in TestMain before any test runs.
var hdfBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "hdf-e2e-*")
	if err != nil {
		panic("MkdirTemp: " + err.Error())
	}

	bin := filepath.Join(dir, "hdf")
	// Build the root package by module path so this works from the e2e
	// subdirectory (and anywhere else inside the module).
	if out, err := exec.Command("go", "build", "-o", bin, "hdf").CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		panic("go build failed:\n" + string(out))
	}
	hdfBin = bin

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// runHDF invokes the binary with args, the given HOME override, and optional
// stdin content. It returns stdout, stderr, and the process exit code.
func runHDF(t *testing.T, home, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(hdfBin, args...)
	// Inherit the current environment but override HOME so hdf's config path
	// resolves into a temp directory rather than the real user's home.
	env := []string{}
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "HOME=") && !strings.HasPrefix(e, "USERPROFILE=") {
			env = append(env, e)
		}
	}
	env = append(env, "HOME="+home, "USERPROFILE="+home)
	cmd.Env = env
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return outBuf.String(), errBuf.String(), exitErr.ExitCode()
		}
	}
	return outBuf.String(), errBuf.String(), 0
}

// initEnv runs `hdf init` with a local working copy and bare push target.
// It fatals the test if init does not succeed.
func initEnv(t *testing.T, home, workDir, bareDir string) {
	t.Helper()
	stdin := "1\n" + workDir + "\n" + bareDir + "\n"
	_, stderr, code := runHDF(t, home, stdin, "init")
	if code != 0 {
		t.Fatalf("init failed (code %d): %s", code, stderr)
	}
}

// TestE2EConfigNoInit verifies that `hdf config` with no config file exits 0
// and tells the user to run `hdf init`.
func TestE2EConfigNoInit(t *testing.T) {
	home := t.TempDir()
	stdout, _, code := runHDF(t, home, "", "config")
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "hdf init") {
		t.Errorf("stdout %q should mention 'hdf init'", stdout)
	}
}

// TestE2EEnrollFileNotFound verifies that enrolling a missing file exits
// non-zero and reports "file not found".
func TestE2EEnrollFileNotFound(t *testing.T) {
	home, workDir, bareDir := t.TempDir(), t.TempDir(), t.TempDir()
	initEnv(t, home, workDir, bareDir)

	_, stderr, code := runHDF(t, home, "", "enroll", "~/.no-such-file")
	if code == 0 {
		t.Error("expected non-zero exit code for missing file")
	}
	if !strings.Contains(stderr, "file not found") {
		t.Errorf("stderr %q should contain 'file not found'", stderr)
	}
}

// TestE2EEnrollOutsideHome verifies that enrolling a file outside the home
// directory exits non-zero and reports the outside-home error.
func TestE2EEnrollOutsideHome(t *testing.T) {
	home, workDir, bareDir := t.TempDir(), t.TempDir(), t.TempDir()
	initEnv(t, home, workDir, bareDir)

	// outsideDir is a sibling of home — guaranteed to be outside it.
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runHDF(t, home, "", "enroll", outsideFile)
	if code == 0 {
		t.Error("expected non-zero exit code for path outside home")
	}
	if !strings.Contains(stderr, "outside the home directory") {
		t.Errorf("stderr %q should contain 'outside the home directory'", stderr)
	}
}

// TestE2EEnrollHomeDirItself verifies that passing the home directory as the
// enroll target exits non-zero and reports "home directory itself", not the
// generic "outside the home directory" message.
func TestE2EEnrollHomeDirItself(t *testing.T) {
	home, workDir, bareDir := t.TempDir(), t.TempDir(), t.TempDir()
	initEnv(t, home, workDir, bareDir)

	_, stderr, code := runHDF(t, home, "", "enroll", home)
	if code == 0 {
		t.Error("expected non-zero exit code when enrolling the home directory itself")
	}
	if !strings.Contains(stderr, "home directory itself") {
		t.Errorf("stderr %q should contain 'home directory itself'", stderr)
	}
}

// TestE2EEnrollPermissionDenied verifies that a permission error from os.Stat
// exits non-zero and is reported as "cannot access", not "file not found".
func TestE2EEnrollPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses DAC — permission test not meaningful")
	}
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workDir, bareDir := t.TempDir(), t.TempDir()
	initEnv(t, home, workDir, bareDir)

	lockedDir := filepath.Join(home, ".locked")
	if err := os.Mkdir(lockedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(lockedDir, "secret")
	if err := os.WriteFile(secret, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(lockedDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(lockedDir, 0o755) }) //nolint:gosec // restoring test directory

	_, stderr, code := runHDF(t, home, "", "enroll", secret)
	if code == 0 {
		t.Error("expected non-zero exit code for permission denied")
	}
	if strings.Contains(stderr, "file not found") {
		t.Errorf("permission error mislabelled as 'file not found': %s", stderr)
	}
	if !strings.Contains(stderr, "cannot access") {
		t.Errorf("stderr %q should contain 'cannot access'", stderr)
	}
}

// TestE2EInitAlreadyInitialized verifies that running `hdf init` when already
// initialized exits non-zero and reports "already initialized".
func TestE2EInitAlreadyInitialized(t *testing.T) {
	home, workDir, bareDir := t.TempDir(), t.TempDir(), t.TempDir()
	initEnv(t, home, workDir, bareDir)

	stdin := "1\n" + workDir + "\n" + bareDir + "\n"
	_, stderr, code := runHDF(t, home, stdin, "init")
	if code == 0 {
		t.Error("second init: expected non-zero exit code")
	}
	if !strings.Contains(stderr, "already initialized") {
		t.Errorf("stderr %q should contain 'already initialized'", stderr)
	}
}

// TestE2EEnrollIdempotent verifies that re-enrolling an already-managed,
// unchanged file exits 0 and reports "already managed and unchanged" rather
// than silently creating an empty commit.
func TestE2EEnrollIdempotent(t *testing.T) {
	home, workDir, bareDir := t.TempDir(), t.TempDir(), t.TempDir()
	initEnv(t, home, workDir, bareDir)

	dotfile := filepath.Join(home, ".testrc")
	if err := os.WriteFile(dotfile, []byte("config\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, code := runHDF(t, home, "", "enroll", "~/.testrc"); code != 0 {
		t.Fatalf("first enroll: exit code %d", code)
	}

	stdout, _, code := runHDF(t, home, "", "enroll", "~/.testrc")
	if code != 0 {
		t.Errorf("second enroll: exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "already managed and unchanged") {
		t.Errorf("stdout %q should contain 'already managed and unchanged'", stdout)
	}
}

// TestE2EMigrationFailureExitsNonZero verifies that a migration error causes
// a non-zero exit rather than being silently swallowed. The trigger: a legacy
// config with Files is present but the .hdf directory in the repo is replaced
// by a regular file, so SaveRegistry cannot create managed.toml inside it.
func TestE2EMigrationFailureExitsNonZero(t *testing.T) {
	home, workDir, bareDir := t.TempDir(), t.TempDir(), t.TempDir()
	initEnv(t, home, workDir, bareDir)

	// Remove managed.toml and replace the .hdf directory with a file so that
	// SaveRegistry fails when migration attempts to write managed.toml.
	hdfDir := filepath.Join(workDir, ".hdf")
	if err := os.RemoveAll(hdfDir); err != nil {
		t.Fatal(err)
	}
	// Write a plain file where the .hdf directory was — os.MkdirAll will fail.
	if err := os.WriteFile(hdfDir, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a legacy config entry so migration is not short-circuited by the
	// "no Files to migrate" early return.
	cfgPath := filepath.Join(home, ".config", "hdf", "config.toml")
	existing, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	legacy := string(existing) + "\n[[files]]\npath = \"~/.bashrc\"\nhash = \"sha256:abc\"\n"
	if err := os.WriteFile(cfgPath, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, code := runHDF(t, home, "", "status")
	if code == 0 {
		t.Error("expected non-zero exit when migration fails")
	}
}
