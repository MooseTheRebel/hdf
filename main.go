package main

import (
	"embed"
	"hdf/internal/cli"
)

// The embed directive must live here: go:embed paths cannot contain "..",
// and frontend/ sits in the repository root. All CLI logic lives in
// internal/cli/ and GUI code is located in frontend/ .
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	cli.Execute(assets)
}
