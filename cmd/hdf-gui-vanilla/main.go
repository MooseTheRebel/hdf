// Command hdf-gui-vanilla is hdf's vanilla-TypeScript GUI (./frontend), an
// alternative to hdf-gui-vuejs (only one may be installed). Next to hdf,
// it's the GUI plugin `hdf` opens; run directly, it's a complete hdf with
// this GUI built in (see gui.Main).
package main

import (
	"embed"
	"hdf/internal/gui"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	gui.Main("vanilla", assets)
}
