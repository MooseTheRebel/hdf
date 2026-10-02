// Command hdf-gui-vanilla is hdf's vanilla-TypeScript GUI, shipped as a
// plugin: bare `hdf` finds this binary (see plugin.Find) and launches it. Installing hdf
// without it gives a headless, CLI-only install.
//
// Run directly (e.g. by `wails dev`) it opens the GUI standalone, without
// the plugin handshake.
package main

import (
	"context"
	"fmt"
	"hdf/frontend"
	"hdf/internal/cli"
	"hdf/plugin"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type gui struct{}

func (gui) Launch(args []string) error {
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
			Assets: frontend.Assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        startup,
		Bind: []interface{}{
			app,
		},
	})
}

func main() {
	if plugin.IsPluginProcess() {
		plugin.Serve(gui{})
		return
	}
	if err := (gui{}).Launch(nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
