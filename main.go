package main

import "hdf/internal/cli"

// All CLI logic lives in internal/cli/. The GUI is a separate plugin binary
// (cmd/hdf-gui-vanilla), so this binary builds without cgo or the frontend.
func main() {
	cli.Execute()
}
