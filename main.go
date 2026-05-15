package main

import (
	"embed"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"

	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

var rootCmd = &cobra.Command{
	Use:   "hdf",
	Short: "A Wails application with CLI capabilities",
	Long:  `This is mostly a CLI application, and for some use-cases launches a GUI.`,
	Run: func(cmd *cobra.Command, args []string) {
		// This runs when the command is executed with no subcommands or the default run behavior.
		// Launch the Wails UI here.
		launchGUI([]string{})
	},
}

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage application configuration",
	Long:  `Allows viewing and modifying the application's configuration settings from the CLI.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("config called")
		// Add your configuration logic here
	},
}

// diffCmd represents the diff command
var diffCmd = &cobra.Command{
	Use:   "diff [url]",
	Short: "Display a diff in a window",
	Long:  `Opens a window to display a diff from a URL or file.`,
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		diffURLs := []string{
			"https://github.com/spf13/cobra/commit/10d4b48a79be3d4e89e6c45cb59f4d32a3d2ae19.diff",
			"https://github.com/spf13/cobra/commit/88b30ab89da2d0d0abb153818746c5a2d30eccec.diff",
			"https://github.com/spf13/cobra/commit/346d408fe7d4be00ff9481ea4d43c4abb5e5f77d.diff",
		}

		// If user provides a URL, use it as a single-item list
		if len(args) > 0 {
			diffURLs = []string{args[0]}
		}

		launchGUI(diffURLs)
	},
}

func launchGUI(diffURLs []string) {
	// Create an instance of the app structure
	app := NewApp()
	app.diffURLs = diffURLs

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "home-dawt-files",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func init() {
	// Add subcommands to the main application's root command
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(diffCmd)
}
