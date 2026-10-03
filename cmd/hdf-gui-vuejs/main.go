// Command hdf-gui-vuejs is hdf's Vue.js GUI (frontend-vuejs/), shipped as a
// plugin alongside or instead of hdf-gui-vanilla: bare `hdf` launches it
// when it's the only GUI installed, or when run as `hdf --gui vuejs`.
package main

import (
	frontendvuejs "hdf/frontend-vuejs"
	"hdf/internal/gui"
)

func main() {
	gui.Main(frontendvuejs.Assets)
}
