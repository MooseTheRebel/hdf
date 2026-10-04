// Command reportplugin is a UI plugin for plugin's tests: Launch writes a
// line to stdout and to stderr, then either panics (when args[0] is
// "panic") or returns an error carrying plugin.HostExecutable(), so tests
// can check what hdf passes to and shows from a plugin.
package main

import (
	"fmt"
	"os"

	"hdf/plugin"
)

type ui struct{}

func (ui) Launch(args []string) error {
	fmt.Fprintln(os.Stdout, "plugin-stdout")
	fmt.Fprintln(os.Stderr, "plugin-stderr")
	if len(args) > 0 && args[0] == "panic" {
		panic("plugin-panic")
	}
	return fmt.Errorf("host=%s", plugin.HostExecutable())
}

func main() {
	plugin.Serve(ui{})
}
