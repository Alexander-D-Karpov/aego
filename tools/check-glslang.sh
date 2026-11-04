#!/usr/bin/env bash
set -euo pipefail
AEGO_CHECK_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AEGO_CHECK_EXPECTED="$(tr -d '\r\n' < "$AEGO_CHECK_ROOT/tools/glslang.version")"
if ! command -v glslangValidator >/dev/null 2>&1; then
  echo "glslangValidator $AEGO_CHECK_EXPECTED is required for portable shader validation" >&2
  exit 1
fi
AEGO_CHECK_VERSION="$(glslangValidator --version | sed -n 's/^Glslang Version: [0-9]*:\([0-9.]*\).*/\1/p')"
if [[ "$AEGO_CHECK_VERSION" != "$AEGO_CHECK_EXPECTED" ]]; then
  echo "glslangValidator version mismatch: need $AEGO_CHECK_EXPECTED, found $AEGO_CHECK_VERSION" >&2
  echo "Install the pinned frontend, or explicitly review and update tools/glslang.version and generated shader artifacts." >&2
  exit 1
fi
echo "glslangValidator $AEGO_CHECK_VERSION (pinned reference frontend)"
