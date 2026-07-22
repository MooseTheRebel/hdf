# home-dawt-files

## About

A hybrid Wails application that functions as both a CLI tool and a desktop application. The primary use case is
managing a user's `$HOME` directory (dot files).


## Features

- **CLI Interface**: Command-line interface for quick operations
- **GUI Diff Viewer**: Opens a window to display diffs from URLs or files
- **Hybrid Design**: Seamlessly switches between CLI and GUI modes based on the command
- **Viewing diffs**: in a windowed interface while maintaining CLI accessibility.

## Installation

Download the latest release for your platform from the
[GitHub releases page](https://github.com/MooseTheRebel/hdf/releases), extract
it, and put the `hdf` binary somewhere on your `PATH`.

The examples below use `hdf` directly. If you're building from source instead
of using a release, see [Development](#development) for how to set `HDF_CLI`
in place of `hdf`.

## CLI Commands

### init
Initialize hdf. Prompts for a git URL or local path, sets up the repository, and creates a per-machine branch named after your hostname.

```bash
hdf init
```

### changes-push (alias: enroll)
Enroll a dot file. Copies it into the hdf repo, replaces the original with a symlink, commits to your machine branch, and registers it on main.

```bash
hdf changes-push ~/.bashrc
hdf changes-push ~/.vimrc
```

### changes-pull (alias: link)
Fetch main, review each incoming file (accept or skip per file), and re-create all managed symlinks (safe to re-run after cloning on a new machine).

```bash
hdf changes-pull
```

### promote
Merge your machine branch into main and push. Content you have never reviewed (another machine's promote you haven't pulled, or a newer version of a file you both changed) is shown first and needs explicit consent. See [docs/state-machine.md](docs/state-machine.md) for the full state machine, guards, and multi-machine model.

```bash
hdf promote
```

### status
Show managed files, their sync state, and the current branch.

```bash
hdf status
```

### daemon
Run the background sync daemon in the foreground, or install it as a per-user background service. Runs every 30 minutes and sends OS notifications when commits, pushes, or merges are needed.

```bash
hdf daemon run
```

#### Install as a background service

Installs a per-user service (a launchd agent on macOS, a systemd user unit on Linux) that starts the daemon now, restarts it on crash, and starts it again at login/boot. No `sudo` required.

```bash
hdf daemon install     # install and start
hdf daemon status      # "not installed" / "stopped" / "running"
hdf daemon stop        # stop without uninstalling
hdf daemon start       # start an installed-but-stopped service
hdf daemon uninstall   # stop and remove the service
```

### diff
Opens a window to display a diff from a URL.

```bash
hdf diff
hdf diff https://patch-diff.githubusercontent.com/raw/spf13/cobra/pull/2285.diff
```

### config
Show the current hdf configuration file.

```bash
hdf config
```

### report-issue
Package diagnostics — a summary, recent daemon activity, known host branches, and a compressed copy of your dotfiles repo (all branches + HEAD) — into a local `.zip` you can hand to an admin. Nothing is uploaded. Refuses to run if the compressed repo would exceed 4MB.

```bash
hdf report-issue
```

## Development

Building from source instead of using a release binary? Substitute `HDF_CLI`
for `hdf` in the commands above (e.g. `"$HDF_CLI" init` instead of `hdf
init`).

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

Set `HDF_CLI` for your platform:

```bash
# macOS
HDF_CLI="./build/bin/hdf.app/Contents/MacOS/hdf"

# Linux
HDF_CLI="./build/bin/hdf"
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
