#!/bin/sh
# WebGL tests use a separate X server, never the user's active desktop.
# Firefox can lack WebGL2 in headless mode; an unavailable Xvfb is a failure,
# not permission to omit Firefox or silently use a different rendering path.
set -eu

if [ "$#" -eq 0 ]; then
    echo 'usage: private-xvfb.sh command [args...]' >&2
    exit 2
fi
if ! command -v xvfb-run >/dev/null 2>&1; then
    echo 'private WebGL browser tests require xvfb-run and xauth' >&2
    exit 1
fi

exec xvfb-run -a -s '-screen 0 1600x1000x24 -nolisten tcp' \
    env AEGO_PRIVATE_XVFB=1 "$@"
