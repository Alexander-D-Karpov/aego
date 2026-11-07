#!/usr/bin/env bash
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"
out="${1:-.cache/artstudio}"
mkdir -p -- "$out"
GOOS=js GOARCH=wasm go build -buildvcs=false -trimpath -o "$out/artstudio.wasm" ./tools/artstudio/wasm
wasm_exec="$(go env GOROOT)/lib/wasm/wasm_exec.js"
[[ -f "$wasm_exec" ]] || wasm_exec="$(go env GOROOT)/misc/wasm/wasm_exec.js"
cp -- "$wasm_exec" "$out/wasm_exec.js"
cp -- tools/artstudio/web/index.html tools/artstudio/web/studio.css tools/artstudio/web/studio.js tools/artstudio/web/recovery.js "$out/"
printf 'Built %s\n' "$out"
wc -c "$out/artstudio.wasm"
