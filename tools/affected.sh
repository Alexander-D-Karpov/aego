#!/bin/sh
# The package list is a local hint, not a substitute for the full CI gate.
set -eu
exec python3 -B "$(dirname -- "$0")/ci/affected_packages.py" "$@"
