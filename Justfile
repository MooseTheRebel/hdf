bin := if os() == "macos" { "build/bin/hdf.app/Contents/MacOS/hdf" } else if os() == "windows" { "build/bin/hdf.exe" } else { "build/bin/hdf" }

export PATH := env_var('HOME') + "/go/bin:/usr/local/go/bin:" + env_var('PATH')

# Install Go dependencies and build the binary
install path="":
    #!/usr/bin/env bash
    set -euo pipefail
    go mod download
    wails build
    if [ "{{path}}" = "true" ]; then
        echo "Adding hdf to /usr/local/bin..."
        cp {{bin}} /usr/local/bin/hdf
        echo "Done."
    fi

# Run in live development mode (hot reload)
dev:
    wails dev

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

# Run the config command
config:
    {{bin}} config

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
