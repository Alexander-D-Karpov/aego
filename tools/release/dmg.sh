#!/bin/sh
# Builds a drag-to-Applications disk image from the editor app on macOS; the app is ad-hoc signed.
set -eu
app=${1:?usage: dmg.sh <Aego.app> <out.dmg>}
out=${2:?usage: dmg.sh <Aego.app> <out.dmg>}
[ "$(uname -s)" = Darwin ] || { echo "dmg.sh: needs macOS (hdiutil)" >&2; exit 1; }
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
cp -R "$app" "$stage/Aego.app"
ln -s /Applications "$stage/Applications"
mkdir -p "$(dirname "$out")"
hdiutil create -quiet -volname Aego -srcfolder "$stage" -ov -format UDZO "$out"
echo "dmg: built $out"
