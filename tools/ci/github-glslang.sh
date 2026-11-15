#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
VERSION="$(tr -d '\r\n' < "$ROOT/tools/glslang.version")"
# The source archive is pinned independently of the tag on the download URL.
[[ "$VERSION" == "15.1.0" ]] || { echo "Update the glslang source pin for $VERSION" >&2; exit 1; }
SHA256=4bdcd8cdb330313f0d4deed7be527b0ac1c115ff272e492853a6e98add61b4bc
WORK="${AEGO_GLSLANG_WORK:-$ROOT/.cache/github-ci/glslang}"
mkdir -p "$WORK/src"
ARCHIVE="$WORK/glslang-$VERSION.tar.gz"
if [[ ! -f "$ARCHIVE" ]]; then
  curl -fsSL "https://codeload.github.com/KhronosGroup/glslang/tar.gz/refs/tags/$VERSION" -o "$ARCHIVE"
fi
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL="$(sha256sum "$ARCHIVE" | awk '{print $1}')"
else
  ACTUAL="$(shasum -a 256 "$ARCHIVE" | awk '{print $1}')"
fi
[[ "$ACTUAL" == "$SHA256" ]] || { echo "glslang source checksum mismatch" >&2; exit 1; }
tar -xzf "$ARCHIVE" -C "$WORK/src" --strip-components=1

# Only the GLSL/ESSL reference frontend and linker are needed for validation.
cmake -S "$WORK/src" -B "$WORK/build" -G Ninja \
  -DCMAKE_BUILD_TYPE=Release -DGLSLANG_TESTS=OFF -DGLSLANG_ENABLE_INSTALL=OFF \
  -DENABLE_SPIRV=OFF -DENABLE_HLSL=OFF -DENABLE_OPT=OFF
cmake --build "$WORK/build" --target glslang-standalone --parallel 2
BIN="$WORK/build/StandAlone"
PATH="$BIN:$PATH" bash "$ROOT/tools/check-glslang.sh"
if [[ -n "${GITHUB_PATH:-}" ]]; then
  if [[ "${RUNNER_OS:-}" == "Windows" ]]; then
    BIN="$(cygpath -w "$BIN")"
  fi
  printf '%s\n' "$BIN" >> "$GITHUB_PATH"
fi
