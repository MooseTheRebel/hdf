// Command hdf-gui-vuejs is hdf's Vue.js GUI (./frontend), shipped as a
// plugin alongside or instead of hdf-gui-vanilla: bare `hdf` launches it
// when it's the only GUI installed, or when run as `hdf --gui vuejs`.
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
