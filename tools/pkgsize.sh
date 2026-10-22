#!/bin/sh
set -eu
limit=${1:-4000}
go list -f '{{.ImportPath}} {{.Dir}} {{join .GoFiles " "}}' ./modules/... ./editor/... ./app/... |
    while read -r pkg dir files; do
        n=0
        for f in $files; do
            n=$((n + $(wc -l < "$dir/$f")))
        done
        mark=""
        [ "$n" -gt "$limit" ] && mark="  split candidate (18.5)"
        echo "$n $pkg$mark"
    done | sort -rn
