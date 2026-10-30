bin := if os() == "windows" { "build/bin/hdf.exe" } else { "build/bin/hdf" }
# The GUI plugin hdf launches when run with no subcommand. `wails build`
# produces an app bundle on macOS; hdf finds the plugin either way.
gui_plugin := if os() == "macos" { "build/bin/hdf-gui-vanilla.app" } else if os() == "windows" { "build/bin/hdf-gui-vanilla.exe" } else { "build/bin/hdf-gui-vanilla" }

export PATH := env_var('HOME') + "/go/bin:/usr/local/go/bin:" + env_var('PATH')

# Build hdf and the GUI plugin
build: build-cli build-gui

# Build hdf alone: the headless, CLI-only install (no cgo, no frontend)
build-cli:
    CGO_ENABLED=0 go build -o {{bin}} .

# Build the vanilla GUI plugin (needs cgo and the frontend toolchain)
build-gui:
    cd cmd/hdf-gui-vanilla && wails build

# Build the Vue.js GUI plugin. It's a complete hdf, so run it directly
# (build/bin/hdf-gui-vuejs[.app]); with the vanilla GUI also in build/bin,
# build/bin/hdf refuses to open either, since only one GUI may be installed.
build-gui-vuejs:
    cd cmd/hdf-gui-vuejs && wails build

# Install Go dependencies and build hdf and the GUI plugin
install path="":
    #!/usr/bin/env bash
    set -euo pipefail
    go mod download
    just build
    if [ "{{path}}" = "true" ]; then
        echo "Adding hdf and its GUI plugin to /usr/local/bin..."
        cp {{bin}} /usr/local/bin/hdf
        rm -rf "/usr/local/bin/$(basename {{gui_plugin}})"
        cp -R {{gui_plugin}} /usr/local/bin/
        echo "Done."
    fi

# Run the GUI in live development mode (hot reload)
dev:
    cd cmd/hdf-gui-vanilla && wails dev

# Run the Vue.js GUI in live development mode (hot reload)
dev-vuejs:
    cd cmd/hdf-gui-vuejs && wails dev

# Open a diff viewer window (optionally pass a diff URL)
diff url="":
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -n "{{url}}" ]; then
        {{bin}} diff "{{url}}"
    else
        {{bin}} diff
    fi

# Run all Go tests
test:
    go test ./...

# Run the plain-language multi-machine scenarios in e2e/features (tags="@bug"
# or tags="~@wip" picks a subset; a subset skips the unwalked-edge check)
bdd tags="":
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -n "{{tags}}" ]; then export HDF_BDD_TAGS="{{tags}}"; fi
    go test -tags e2e ./e2e -run TestFeatures -count=1 -v

# Measure code coverage across unit, e2e and BDD tests (hdf processes the
# e2e tests start included) and write build/coverage/: coverage.html,
# coverage.out, and state-graph.md (which state transitions were walked)
cover:
    #!/usr/bin/env bash
    set -euo pipefail
    out="$PWD/build/coverage"
    rm -rf "$out"
    mkdir -p "$out/unit" "$out/e2e" "$out/merged"
    go test -cover -coverpkg=hdf/... ./... -args -test.gocoverdir="$out/unit"
    HDF_E2E_COVERDIR="$out/e2e" go test -tags e2e ./e2e -count=1
    go tool covdata merge -i="$out/unit,$out/e2e" -o "$out/merged"
    go tool covdata textfmt -i="$out/unit" -o "$out/unit.out"
    go tool covdata textfmt -i="$out/merged" -o "$out/coverage.out"
    go tool cover -html="$out/coverage.out" -o "$out/coverage.html"
    echo "Unit tests only:   $(go tool cover -func="$out/unit.out" | tail -1 | awk '{print $NF}')"
    echo "Unit + e2e + BDD:  $(go tool cover -func="$out/coverage.out" | tail -1 | awk '{print $NF}')"
    echo "Report: $out/coverage.html"
    echo "State graph: $out/state-graph.md"

# Run the config command
config:
    {{bin}} config

# Initialize hdf (wizard: prompts for git URL)
init: _check
    {{bin}} init

# Enroll a dot file under hdf management
enroll file: _check
    {{bin}} enroll "{{file}}"

# Re-create all managed symlinks
link: _check
    {{bin}} link

# Show managed files and sync state
status: _check
    {{bin}} status

# Start the hdf sync daemon (foreground)
daemon: _check
    {{bin}} daemon

# Demo commands
demo: _check
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Testing hdf CLI commands..."
    echo ""
    echo "1. Testing 'diff' command with default URL:"
    {{bin}} diff &
    echo "   Started in background (PID: $!)"
    echo ""
    echo "2. Testing 'config' command:"
    {{bin}} config
    echo ""

_check:
    @test -f {{bin}} || (echo "Error: hdf binary not found at {{bin}}" && echo "Run 'just install' first." && exit 1)
