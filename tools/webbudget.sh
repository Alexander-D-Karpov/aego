#!/bin/sh
# Fails when a gzipped wasm of the built site is over its budget in MB.
set -eu
site=${1:-site}
bad=0
check() {
    f="$site/$1"
    if [ ! -f "$f" ]; then
        echo "web budget: $f is missing"
        bad=1
        return
    fi
    size=$(wc -c < "$f")
    mb=$(awk -v s="$size" 'BEGIN { printf "%.2f", s / 1048576 }')
    if [ "$size" -gt $(($2 * 1048576)) ]; then
        echo "web budget: $1 is $mb MB, over $2 MB"
        bad=1
    else
        echo "web budget: $1 $mb MB of $2 MB"
    fi
}
# The demo shell is patched after export. A stale static gzip can otherwise
# serve different launch settings depending on the browser's Accept-Encoding.
page="$site/demo/index.html"
if ! gzip -t "$page.gz" || ! gzip -dc "$page.gz" | cmp -s "$page" -; then
    echo "web budget: demo/index.html.gz does not match the published launch HTML"
    bad=1
fi
check demo/game.wasm.gz 8
check play/aego.wasm.gz 8
check try/aego-docs.wasm.gz 10
exit $bad
