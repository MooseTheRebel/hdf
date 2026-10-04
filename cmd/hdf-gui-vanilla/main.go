// Command hdf-gui-vanilla is hdf's vanilla-TypeScript GUI (./frontend),
// shipped as a plugin: bare `hdf` finds this binary (see plugin.Find) and
// launches it. Installing hdf without any GUI plugin gives a headless,
// CLI-only install.
package main

import (
	"embed"
	"hdf/internal/gui"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	gui.Main(assets)
}
