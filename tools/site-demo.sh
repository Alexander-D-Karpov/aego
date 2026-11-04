#!/bin/sh
# Build the canonical platformer into a fresh directory. A failed export must
# not replace the last good local site or leave old, unreferenced artifacts.
set -eu
out=$1
shift
parent=$(dirname "$out")
stage=$(mktemp -d "$parent/.site-demo.XXXXXX")
cleanup() {
    if [ -d "$stage/previous" ] && [ ! -e "$out" ]; then
        mv "$stage/previous" "$out"
    fi
    rm -rf "$stage"
}
trap cleanup 0
trap 'exit 1' HUP INT TERM
"$@" --out "$stage/new"

# The site's launcher starts muted; the downloadable game keeps its settings.
# Generate every representation from the same final HTML. gzip -n omits the
# source name and time; rename completed files on the same filesystem.
page="$stage/new/index.html"
sed '/<html /{ /data-sound="off"/!s/<html lang="en"/<html lang="en" data-sound="off"/; }' "$page" > "$stage/index.html"
grep -q '<html .*data-sound="off"' "$stage/index.html"
gzip -n -9 -c "$stage/index.html" > "$stage/index.html.gz"
if [ -f "$page.br" ]; then
    brotli -q 11 -c "$stage/index.html" > "$stage/index.html.br"
    mv "$stage/index.html.br" "$page.br"
fi
mv "$stage/index.html.gz" "$page.gz"
mv "$stage/index.html" "$page"
if [ -e "$out" ]; then
    mv "$out" "$stage/previous"
fi
mv "$stage/new" "$out"
