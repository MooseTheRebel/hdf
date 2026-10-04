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
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Main is a GUI binary's entire main, for the GUI plugin name (e.g.
// "vanilla") with the given frontend assets. Started by hdf, it serves as
// that GUI plugin. Run directly — from a launcher, a terminal, or `wails
// dev` — it's a complete hdf with this GUI built in: no command opens the
// GUI, and every hdf command works too.
func Main(name string, assets fs.FS) {
	g := gui{name: name, assets: assets}

	switch {
	case plugin.IsPluginProcess():
		plugin.Serve(g)
	case generatingBindings:
		// `wails build`/`wails generate module` run the binary just to
		// generate frontend bindings; skip hdf's startup work (config
		// migration, crash prompt) against the developer's real home.
		if err := g.Launch(nil); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		cli.Execute(cli.WithUI(name, g))
	}
}

type gui struct {
	name   string // its plugin.GUIs name, e.g. "vanilla"
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
		// GTK otherwise names the window after however the binary was
		// started — "hdf" via a Linux GUI package's /usr/bin/hdf symlink —
		// and it wouldn't match the package's hdf-gui-<name>.desktop entry,
		// losing its taskbar icon.
		Linux:     &linux.Options{ProgramName: plugin.BinaryName(g.name)},
		OnStartup: startup,
		Bind: []any{
			app,
		},
	})
}
