package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Find locates the executable for GUI plugin name ("hdf-gui-<name>"),
// returning its path and whether one was found. It looks next to the
// running hdf executable first, so a plugin shipped alongside hdf wins over
// one elsewhere, then on $PATH.
func Find(name string) (string, bool) {
	exeDir := ""

	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}

		exeDir = filepath.Dir(exe)
	}

	return find(name, exeDir, runtime.GOOS, isExecutable, exec.LookPath)
}

// find is Find with its environment passed in, for testing. Next to hdf it
// checks the bare binary, then (on macOS) inside the .app bundle that
// `wails build` produces.
func find(name, exeDir, goos string, isExec func(string) bool, lookPath func(string) (string, error)) (string, bool) {
	bin := BinaryName(name)
	if goos == "windows" {
		bin += ".exe"
	}

	if exeDir != "" {
		candidates := []string{filepath.Join(exeDir, bin)}
		if goos == "darwin" {
			candidates = append(candidates, filepath.Join(exeDir, bin+".app", "Contents", "MacOS", bin))
		}

		for _, c := range candidates {
			if isExec(c) {
				return c, true
			}
		}
	}

	if p, err := lookPath(bin); err == nil {
		return p, true
	}

	return "", false
}

// BinaryName is the executable name of GUI plugin name, without any
// platform suffix.
func BinaryName(name string) string {
	return "hdf-gui-" + name
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}

	return runtime.GOOS == "windows" || info.Mode()&0o111 != 0
}
