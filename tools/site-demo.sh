#!/bin/sh
# Build the canonical platformer fresh; a failed export must not replace the last good site or leave stale artifacts.
set -eu
out=$1
shift
parent=$(dirname "$out")
stage=$(mktemp -d "$parent/.site-demo.XXXXXX")
stage=$(cd "$stage" && pwd -P)
cleanup() {
    if [ -d "$stage/previous" ] && [ ! -e "$out" ]; then
        mv "$stage/previous" "$out"
    fi
    rm -rf "$stage"
}
trap cleanup 0
trap 'exit 1' HUP INT TERM
if [ "${1:-}" = --fix-checkpoint-gap ]; then
    shift
    cli=$1
    verb=$2
    project=$3
    shift 3
    [ "$verb" = export ] || { echo 'expected export command' >&2; exit 1; }
    python3 -B tools/site_demo_prepare.py "$project" "$stage/project"
    engine=$(pwd -P)
    (cd "$stage" && GOWORK=off go work init "$engine" "$stage/project")
    GOWORK="$stage/go.work" "$cli" "$verb" "$stage/project" "$@" --out "$stage/new"
else
    "$@" --out "$stage/new"
fi

# Mute only the site's launcher; generate all representations from the final HTML, omit gzip name/time, and rename on the same filesystem.
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
