#!/usr/bin/env bash
set -euo pipefail

root="$PWD/.cache/github-ci"
mkdir -p "$root/macos" "$root/dist/macos"
go build -trimpath -o "$root/macos/aego-editor" ./cmd/aego-editor
go run ./tools/appbundle -bin "$root/macos/aego-editor" -out "$root/macos" \
  -version 0.0.0 -icon app/brand/png/aego-256.png
app="$root/macos/Aego Editor.app"
plutil -lint "$app/Contents/Info.plist"
codesign --verify --deep --strict --verbose=2 "$app"
otool -L "$app/Contents/MacOS/aego-editor" | tee "$root/logs/macos-libraries.txt"
# SDL is linked statically; a non-system dependency would make the bundle non-portable.
if otool -L "$app/Contents/MacOS/aego-editor" | tail -n +2 | \
  grep -vE '^[[:space:]]*(/System/Library/|/usr/lib/)' ; then
  echo 'app bundle has a non-system dynamic dependency; investigate' >&2
  exit 1
fi
ditto -c -k --sequesterRsrc --keepParent "$app" "$root/dist/macos/Aego-Editor-macos.zip"
(cd "$root/dist/macos" && shasum -a 256 Aego-Editor-macos.zip > SHA256SUMS.txt)
