#!/bin/bash
# Test script for CLI commands

HDF_CLI="./build/bin/hdf.app/Contents/MacOS/hdf"

if [ ! -f "$HDF_CLI" ]; then
    echo "Error: hdf not found at $HDF_CLI"
    echo "Please run 'wails build' first to build the application"
    exit 1
fi

echo "Testing hdf CLI commands..."
echo ""

echo "1. Testing 'diff' command with default URL:"
"$HDF_CLI" diff &
echo "   Started in background (PID: $!)"
echo ""

echo "2. Testing 'config' command:"
"$HDF_CLI" config
echo ""
