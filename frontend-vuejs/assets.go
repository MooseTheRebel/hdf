// Package frontendvuejs embeds the built Vue.js GUI (dist/) for the
// hdf-gui-vuejs plugin. The embed directive must live here, beside dist/,
// because go:embed paths cannot contain "..".
package frontendvuejs

import "embed"

// Assets is the built frontend, served by the GUI plugin's Wails asset
// server.
//
//go:embed all:dist
var Assets embed.FS
