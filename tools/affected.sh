#!/bin/sh
set -eu
base=${1:-origin/master}
mod=$(go list -m)
dirs=$( { git diff --name-only "$base"...HEAD; git diff --name-only; git diff --name-only --cached; git ls-files --others --exclude-standard; } | grep '\.go$' | while read -r f; do dirname "$f"; done | sort -u)
[ -n "$dirs" ] || exit 0
want=""
for d in $dirs; do
    [ -d "$d" ] && want="$want $mod/$d"
done
[ -n "$want" ] || exit 0
go list -f '{{.ImportPath}}{{range .Deps}} {{.}}{{end}}{{range .TestImports}} {{.}}{{end}}{{range .XTestImports}} {{.}}{{end}}' ./... 2>/dev/null |
    awk -v want="$want" 'BEGIN { n = split(want, w, " "); for (i = 1; i <= n; i++) set[w[i]] = 1 } { for (i = 1; i <= NF; i++) if ($i in set) { print $1; next } }' |
    sort -u
