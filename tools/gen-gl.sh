#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/gpu/gl33/gl"
WORK="$ROOT/.cache/glow"

mkdir -p "$WORK"
GOBIN="$WORK/bin" go install github.com/go-gl/glow@latest
export PATH="$WORK/bin:$PATH"

if [[ ! -d "$WORK/xml" ]]; then
  git clone --depth 1 https://github.com/go-gl/glow "$WORK/glow-src"
  cp -r "$WORK/glow-src/xml" "$WORK/xml"
  cp -r "$WORK/glow-src/tmpl" "$WORK/tmpl"
fi

rm -rf "$OUT"
mkdir -p "$OUT"
glow generate \
  -out="$OUT" \
  -api=gl \
  -version=3.3 \
  -profile=core \
  -xml="$WORK/xml" \
  -tmpl="$WORK/tmpl"

gofmt -w "$OUT"
echo "generated into $OUT"
