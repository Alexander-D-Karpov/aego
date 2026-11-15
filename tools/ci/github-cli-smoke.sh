#!/usr/bin/env bash
set -euo pipefail

root="$PWD/.cache/github-ci"
mkdir -p "$root/bin"
exe="$root/bin/aego"
if [[ "$(go env GOOS)" == windows ]]; then exe="$exe.exe"; fi
go build -trimpath -o "$exe" ./cmd/aego
"$exe" version
"$exe" new "$root/project"
"$exe" import "$root/project"
"$exe" check "$root/project"
"$exe" run "$root/project" --headless --frames 60
"$exe" check examples/platformer
"$exe" run examples/platformer --headless --frames 120
