# home-dawt-files

## About

A hybrid Wails application that functions as both a CLI tool and a desktop application. The primary use case is
managing a user's `$HOME` directory (dot files).


## Features

- **CLI Interface**: Command-line interface for quick operations
- **GUI Diff Viewer**: Opens a window to display diffs from URLs or files
- **Hybrid Design**: Seamlessly switches between CLI and GUI modes based on the command
- **Viewing diffs**: in a windowed interface while maintaining CLI accessibility.

## CLI Commands

Build first, then set `HDF_CLI` for your platform:

```bash
just install

# macOS
HDF_CLI="./build/bin/hdf.app/Contents/MacOS/hdf"

# Linux
HDF_CLI="./build/bin/hdf"
```

### init
Initialize hdf. Prompts for a git URL or local path, sets up the repository, and creates a per-machine branch named after your hostname.

```bash
"$HDF_CLI" init
```

### changes-push (alias: enroll)
Enroll a dot file. Copies it into the hdf repo, replaces the original with a symlink, commits to your machine branch, and registers it on main.

```bash
"$HDF_CLI" changes-push ~/.bashrc
"$HDF_CLI" changes-push ~/.vimrc
```

### changes-pull (alias: link)
Fetch main, review each incoming file (accept or skip per file), and re-create all managed symlinks (safe to re-run after cloning on a new machine).

```bash
"$HDF_CLI" changes-pull
```

### promote
Merge your machine branch into main and push. Content you have never reviewed (another machine's promote you haven't pulled, or a newer version of a file you both changed) is shown first and needs explicit consent. See [docs/state-machine.md](docs/state-machine.md) for the full state machine, guards, and multi-machine model.

```bash
"$HDF_CLI" promote
```

### status
Show managed files, their sync state, and the current branch.

```bash
"$HDF_CLI" status
```

### daemon
Start the background sync daemon. Runs every 30 minutes and sends OS notifications when commits, pushes, or merges are needed.

```bash
"$HDF_CLI" daemon
```

### diff
Opens a window to display a diff from a URL.

```bash
"$HDF_CLI" diff
"$HDF_CLI" diff https://patch-diff.githubusercontent.com/raw/spf13/cobra/pull/2285.diff
```

### config
Show the current hdf configuration file.

```bash
"$HDF_CLI" config
```

## Development

### Prerequisites

Install [just](https://github.com/casey/just) and [wails](https://wails.io):

```bash
brew install just
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### Install

```bash
# Build and install locally
just install

# Build and add hdf to /usr/local/bin
just install --path
```

### Live Development

```bash
just dev
```

This runs a Vite development server with hot reload for frontend changes. A dev server at http://localhost:34115 also exposes Go methods directly in the browser.

### Testing CLI

```bash
just demo
```

### Running Go Tests

```bash
just test
```
