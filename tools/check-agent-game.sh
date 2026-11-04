#!/bin/sh
set -eu
engine=$(pwd)
cli=${AEGO_CLI:-$engine/.cache/bin/aego}
output=${1:?usage: sh tools/check-agent-game.sh NEW_OUTPUT_DIRECTORY}
mkdir "$output"
output=$(cd "$output" && pwd)
AEGO_BELLRUN_OUT="$output/source" go test -count=1 -tags aego_nonative ./tests/agent -run '^TestAuthorBellRunFromEmpty$'
"$cli" export "$output/source" --engine "$engine" --target host --development --out "$output/native"
native=$(sed -n 's/^  binary="\([^"]*\)"$/\1/p' "$output/native/export.aetext")
test -n "$native"
"$output/native/$native" --headless --frames 185 --seed 41 --replay "$output/source/route.aeinput"
"$cli" export "$output/source" --engine "$engine" --target web --release --out "$output/web"
node tools/websmoke/agent-game.mjs "$output/web" "$output/source/route.aeinput" "$output/web-captures"
