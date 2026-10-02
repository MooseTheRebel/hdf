package cli

import (
	"fmt"
	"hdf/plugin"

	"github.com/spf13/cobra"
)

// launchUI is what bare `hdf` runs: it hands off to a GUI plugin, or
// prints help when none is installed — the headless, CLI-only install.
// With gui set (`hdf --gui <name>`) it launches that GUI or fails;
// otherwise it launches the first of plugin.GUIs that's installed. find and
// launch are plugin.Find and plugin.Launch, passed in so tests don't spawn
// processes.
func launchUI(cmd *cobra.Command, gui string, find func(name string) (string, bool), launch func(path string, args []string) error) error {
	if gui != "" {
		path, ok := find(gui)
		if !ok {
			return fmt.Errorf("GUI %q is not installed (looked for %s next to hdf and on $PATH)", gui, plugin.BinaryName(gui))
		}
		return launch(path, nil)
	}
	for _, name := range plugin.GUIs {
		if path, ok := find(name); ok {
			return launch(path, nil)
		}
	}
	return cmd.Help()
}
