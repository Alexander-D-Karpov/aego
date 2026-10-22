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
check demo/game.wasm.gz 8
check play/aego.wasm.gz 8
check try/aego-docs.wasm.gz 10
exit $bad
