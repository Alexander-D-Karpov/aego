#!/bin/sh
# Fails when the checkout root holds executables or test binaries, or, in CI, when git sees files the run left behind.
set -eu
cd "$(dirname "$0")/.."
bad=""
for f in * .[!.]*; do
	[ -f "$f" ] || continue
	case "$f" in
	*.test) bad="$bad $f" ;;
	*)
		if [ -x "$f" ] && ! git ls-files --error-unmatch -- "$f" >/dev/null 2>&1; then
			bad="$bad $f"
		fi
		;;
	esac
done
if [ -n "$bad" ]; then
	echo "build output in the root:$bad" >&2
	echo "binaries go to .cache/bin: go build -o .cache/bin/<name> ./<pkg>, go test -c -o .cache/bin/<name>.test ./<pkg>" >&2
	exit 1
fi
if [ -n "${CI:-}" ]; then
	left=$(git status --porcelain)
	if [ -n "$left" ]; then
		echo "the run left changes in the checkout:" >&2
		echo "$left" >&2
		exit 1
	fi
fi
