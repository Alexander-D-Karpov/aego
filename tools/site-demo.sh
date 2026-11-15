#!/bin/sh
# Build the canonical platformer fresh; a failed export must not replace the last good site or leave stale artifacts.
set -eu
out=$1
shift
if [ -L "$out" ] || { [ -e "$out" ] && [ ! -d "$out" ]; }; then
    echo "site-demo: output must be a directory, not a file or symlink: $out" >&2
    exit 1
fi
parent=$(dirname "$out")
stage=$(mktemp -d "$parent/.site-demo.XXXXXX")
stage=$(cd "$stage" && pwd -P)
published=0
cleanup() {
    if [ "$published" = 0 ] && { [ -e "$stage/previous" ] || [ -L "$stage/previous" ]; }; then
        if [ ! -e "$out" ] && [ ! -L "$out" ]; then
            mv "$stage/previous" "$out" || true
        fi
        if [ -e "$stage/previous" ] || [ -L "$stage/previous" ]; then
            printf 'site-demo: previous output retained at %s\n' "$stage/previous" >&2
            return
        fi
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
# A marker detects mv nesting if the destination reappears during publication.
marker=$(mktemp "$stage/new/.aego-demo-publish.XXXXXXXX")
marker=${marker##*/}
if [ -L "$out" ] || { [ -e "$out" ] && [ ! -d "$out" ]; }; then
    echo "site-demo: output changed to a file or symlink: $out" >&2
    exit 1
fi
if [ -d "$out" ]; then
    mv "$out" "$stage/previous"
fi
[ ! -e "$out" ] && [ ! -L "$out" ] || { echo "site-demo: output appeared during publication: $out" >&2; exit 1; }
mv "$stage/new" "$out"
[ -f "$out/$marker" ] && [ ! -L "$out/$marker" ] || { echo "site-demo: output changed during publication: $out" >&2; exit 1; }
rm "$out/$marker"
published=1
