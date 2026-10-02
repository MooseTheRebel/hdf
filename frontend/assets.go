// Package frontend embeds the built GUI (dist/) for the UI plugin. The
// embed directive must live here, beside dist/, because go:embed paths
// cannot contain "..".
package frontend

import "embed"

// Assets is the built frontend, served by the UI plugin's Wails asset server.
//
//go:embed all:dist
var Assets embed.FS
