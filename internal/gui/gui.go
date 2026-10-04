// Package gui runs hdf's Wails GUI. It's shared by the GUI plugin binaries
// (cmd/hdf-gui-*), each of which supplies its own frontend's assets. Only
// those binaries may import it: it links Wails, which needs cgo, and the
// hdf binary must stay cgo-free.
package gui

import (
	"context"
	"fmt"
	"hdf/internal/cli"
	"hdf/plugin"
	"io/fs"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Main is a GUI plugin binary's entire main: started by hdf it serves the
// plugin; run directly (e.g. by `wails dev`) it opens the GUI standalone,
// without the plugin handshake.
func Main(assets fs.FS) {
	g := gui{assets: assets}
	if plugin.IsPluginProcess() {
		plugin.Serve(g)

		return
	}

	err := g.Launch(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

type gui struct {
	assets fs.FS
}

func (g gui) Launch(args []string) error {
	app, startup := cli.NewGUIApp(cli.Runtime{
		OpenFileDialog: func(ctx context.Context, title, defaultDir string) (string, error) {
			return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
				Title:            title,
				DefaultDirectory: defaultDir,
				ShowHiddenFiles:  true,
			})
		},
		OpenDirectoryDialog: func(ctx context.Context, title, defaultDir string) (string, error) {
			return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{
				Title:            title,
				DefaultDirectory: defaultDir,
			})
		},
		Quit: runtime.Quit,
	}, args)

	return wails.Run(&options.App{
		Title:  "home-dawt-files",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: g.assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        startup,
		Bind: []any{
			app,
		},
	})
}
