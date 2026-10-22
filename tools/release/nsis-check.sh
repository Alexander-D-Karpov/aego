#!/bin/sh
# Builds the Windows installer from placeholder files to check aego.nsi without a Windows build.
set -eu
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
for f in aego.exe aego-editor.exe VERSION README.txt; do
	printf 'placeholder\n' > "$tmp/$f"
done
(cd "$root" && go run ./tools/brandres -ico "$tmp/aego.ico")
makensis -V2 -DVERSION=check -DSRC="$tmp" -DOUT="$tmp/setup.exe" -DICON="$tmp/aego.ico" "$root/tools/release/windows/aego.nsi"
test -s "$tmp/setup.exe"
echo "nsis-check: ok"
