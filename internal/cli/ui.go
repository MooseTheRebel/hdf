package cli

import (
	"fmt"
	"hdf/plugin"
	"io"
	"os"
	"runtime"
	"strings"
)

// builtInGUI is a GUI compiled into this binary (see WithUI).
type builtInGUI struct {
	name string // its plugin.GUIs name, e.g. "vanilla"
	path string // this binary, for messages
	ui   plugin.UI
}

// builtIn is the GUI compiled into this binary, or nil for plain hdf.
var builtIn *builtInGUI

// Option configures Execute.
type Option func()

// WithUI compiles the GUI plugin name's ui into this binary: run without a
// command, the binary opens that GUI directly instead of looking for a GUI
// plugin, making it a complete, standalone hdf — CLI, daemon and GUI. The
// GUI plugin binaries use it when run directly rather than by hdf.
func WithUI(name string, ui plugin.UI) Option {
	return func() {
		path, _ := os.Executable()
		builtIn = &builtInGUI{name: name, path: path, ui: ui}
	}
}

// launchUI is what running hdf without a command does: open the GUI —
// the built-in one (builtIn, nil for plain hdf) or else the installed GUI
// plugin — or, with no GUI at all (the headless, CLI-only install), print
// a welcome message to w. Only one GUI may be installed at a time; with
// more, it fails and asks the user to uninstall the extras. find and
// launch are plugin.Find and plugin.Launch, passed in so tests don't spawn
// processes.
func launchUI(w io.Writer, builtIn *builtInGUI, find func(name string) (string, bool), launch func(path string, args []string) error) error {
	var installed []string

	paths := map[string]string{}

	for _, name := range plugin.GUIs {
		path, ok := find(name)
		if builtIn != nil && name == builtIn.name {
			path, ok = builtIn.path, true
		}

		if ok {
			installed = append(installed, name)
			paths[name] = path
		}
	}

	switch {
	case len(installed) > 1:
		return guiConflictError(installed, paths)
	case builtIn != nil:
		return builtIn.ui.Launch(nil)
	case len(installed) == 1:
		return launch(paths[installed[0]], nil)
	default:
		_, err := io.WriteString(w, welcomeText(runtime.GOOS, runtime.GOARCH))

		return err
	}
}

// guiConflictError reports that more than one GUI is installed.
func guiConflictError(installed []string, paths map[string]string) error {
	var b strings.Builder

	b.WriteString("more than one hdf GUI is installed, but only one is allowed at a time.\nPlease uninstall all but one of:")

	for _, name := range installed {
		fmt.Fprintf(&b, "\n  %-16s %s", plugin.BinaryName(name), installLocation(paths[name]))
	}

	return fmt.Errorf("%s", b.String())
}

// installLocation is what a user would remove to uninstall the GUI at
// path: the .app bundle for a macOS app, otherwise the binary itself.
func installLocation(path string) string {
	if i := strings.Index(path, ".app/Contents/MacOS/"); i >= 0 {
		return path[:i+len(".app")]
	}

	return path
}

// welcomeText is what plain hdf shows when run without a command and no
// GUI is installed, for a user on goos/goarch.
func welcomeText(goos, goarch string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "hdf %s — your dotfiles, synced across your machines.\n\n", version)
	b.WriteString("No GUI is installed, so hdf runs as a command-line tool.\n")
	b.WriteString("  Get started:   hdf init\n")
	b.WriteString("  All commands:  hdf --help\n\n")

	b.WriteString(guiDownloadHint(goos, goarch))

	return b.String()
}

// guiDownloadHint tells a user on goos/goarch how to get a GUI.
func guiDownloadHint(goos, goarch string) string {
	if !plugin.GUIAvailableFor(goos, goarch) {
		return fmt.Sprintf("hdf's GUI isn't available for %s yet.\n", platformName(goos))
	}

	names := make([]string, len(plugin.GUIs))
	for i, name := range plugin.GUIs {
		names[i] = plugin.BinaryName(name)
	}

	variants := strings.Join(names, " or ")

	if goos == "darwin" {
		// The macOS GUIs are apps, each a complete hdf on its own.
		return fmt.Sprintf("For the GUI, download the hdf app (%s, only one)\nfrom %s, unzip it, and open it.\n",
			variants, releasesURL())
	}

	return fmt.Sprintf("To add a GUI, download one of %s (only one)\nfrom %s and extract it next to hdf.\n",
		variants, releasesURL())
}

// withoutPSN drops "-psn_..." process-serial-number arguments, which macOS
// has passed to apps opened from Finder (e.g. on first launch from a
// quarantined location); cobra would reject them as unknown flags.
func withoutPSN(args []string) []string {
	kept := make([]string, 0, len(args))
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-psn_") {
			kept = append(kept, arg)
		}
	}

	return kept
}

// releasesURL is this version's release page, where its GUIs can be
// downloaded — or the latest release's, for development builds.
func releasesURL() string {
	if version == "dev" {
		return "https://github.com/MooseTheRebel/hdf/releases/latest"
	}

	return "https://github.com/MooseTheRebel/hdf/releases/tag/v" + version
}

// platformName is goos as users know it.
func platformName(goos string) string {
	switch goos {
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	default:
		return goos
	}
}
