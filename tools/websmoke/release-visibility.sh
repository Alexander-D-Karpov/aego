#!/bin/sh
# The browser and keyboard automation must never use the user's desktop.
set -eu
cd "$(dirname "$0")"
command -v xdotool >/dev/null || { echo 'xdotool is required' >&2; exit 1; }
exec xvfb-run -a -s '-screen 0 1600x1000x24 -nolisten tcp' env AEGO_PRIVATE_XVFB=1 node release-visibility.mjs "$@"
