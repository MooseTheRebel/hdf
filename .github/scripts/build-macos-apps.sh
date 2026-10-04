#!/usr/bin/env bash
# Builds hdf's GUIs as macOS apps — universal (Intel and Apple Silicon),
# ad-hoc signed — and zips each one for release:
#
#   <outdir>/hdf-gui-<name>_<version>_darwin_universal.zip
#
# Each app is a complete hdf (see gui.Main): double-clicking it opens the
# GUI. Wails needs cgo, so this runs on macOS (the release workflow's
# publish-macos job), not in goreleaser's Linux job.
#
# Usage, from the repository root (needs Go, Node, jq and the wails CLI):
#   .github/scripts/build-macos-apps.sh <version> <outdir>
set -euo pipefail

version=${1:?usage: $0 <version> <outdir>}
outdir=${2:?usage: $0 <version> <outdir>}
mkdir -p "$outdir" build

# wails turns build/appicon.png (gitignored) into the app icon.
cp cmd/hdf-gui-vanilla/linux/hdf.png build/appicon.png

for gui in vanilla vuejs; do
	dir=cmd/hdf-gui-$gui
	app=build/bin/hdf-gui-$gui.app

	# Stamp the version into the app's Info.plist (from wails.json) and into
	# hdf itself (`hdf --version`), restoring wails.json afterwards.
	cp "$dir/wails.json" "$dir/wails.json.orig"
	trap 'mv "$dir/wails.json.orig" "$dir/wails.json"' EXIT
	jq --arg v "$version" '.info.productVersion = $v' "$dir/wails.json.orig" >"$dir/wails.json"

	rm -rf "$app"
	(cd "$dir" && wails build -platform darwin/universal -ldflags "-X hdf/internal/cli.version=$version")

	mv "$dir/wails.json.orig" "$dir/wails.json"
	trap - EXIT

	# ditto keeps the bundle's symlinks, permissions and signature intact.
	ditto -c -k --keepParent "$app" "$outdir/hdf-gui-${gui}_${version}_darwin_universal.zip"
done

(cd "$outdir" && shasum -a 256 hdf-gui-*_darwin_universal.zip >checksums-macos.txt)
