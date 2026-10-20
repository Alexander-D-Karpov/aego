#!/bin/sh
# Builds the Linux release archive and publishes it as a Gitea release.
# Usage: tools/release/release.sh [--dry-run] <vX.Y.Z|nightly>
# Needs GITEA_TOKEN, and GITEA_API (https://host/api/v1/repos/owner/name) unless --dry-run.
set -eu

dry=0
if [ "${1:-}" = "--dry-run" ]; then
	dry=1
	shift
fi
tag="${1:?usage: release.sh [--dry-run] <vX.Y.Z|nightly>}"
case "$tag" in
v[0-9]*) pre=false; name="Aego ${tag#v}" ;;
nightly) pre=true; name="Nightly" ;;
*) echo "release: tag must be vX.Y.Z or nightly" >&2; exit 2 ;;
esac

root=$(cd "$(dirname "$0")/../.." && pwd)
cd "$root"
commit=$(git rev-parse HEAD)
short=$(git rev-parse --short HEAD)
version=$(sed -n 's/^const Version = "\(.*\)"$/\1/p' engine/config.go)
if [ "$pre" = false ] && [ "v$version" != "$tag" ]; then
	echo "release: tag $tag does not match engine.Version $version" >&2
	exit 1
fi

label="$version"
[ "$tag" = nightly ] && label="nightly"
dist="$root/.cache/dist"
rm -rf "$dist" && mkdir -p "$dist"

sum() {
	(cd "$dist" && sha256sum "$1" > "$1.sha256")
}

stage() {
	mkdir -p "$1/bin"
	printf '%s\n%s\n' "$tag" "$commit" > "$1/VERSION"
	cp tools/release/README.txt "$1/README.txt"
}

# Linux: the host build.
base="aego-$label-linux-amd64"
stage "$dist/$base"
CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o "$dist/$base/bin/aego" ./cmd/aego
tar -C "$dist" -czf "$dist/$base.tar.gz" "$base"
sum "$base.tar.gz"
echo "release: built $base.tar.gz"

# Windows: cross-built with mingw-w64 and packed by NSIS.
wcc=x86_64-w64-mingw32-gcc
if command -v "$wcc" >/dev/null 2>&1 && command -v makensis >/dev/null 2>&1; then
	CC=$wcc GOOS=windows GOARCH=amd64 bash tools/bootstrap-native.sh
	win="aego-$label-windows-amd64"
	stage "$dist/$win"
	export CGO_ENABLED=1 CC=$wcc GOOS=windows GOARCH=amd64
	go build -trimpath -ldflags "-s -w" -o "$dist/$win/aego.exe" ./cmd/aego
	go build -trimpath -tags aego_editor -ldflags "-s -w -H=windowsgui" -o "$dist/$win/aego-editor.exe" ./cmd/aego-editor
	unset CGO_ENABLED CC GOOS GOARCH
	rmdir "$dist/$win/bin"
	(cd "$dist" && zip -qr "$win.zip" "$win")
	sum "$win.zip"
	makensis -V2 -DVERSION="$label" -DSRC="$dist/$win" -DOUT="$dist/$win-setup.exe" tools/release/windows/aego.nsi
	sum "$win-setup.exe"
	echo "release: built $win.zip and $win-setup.exe"
elif [ "${REQUIRE_WINDOWS:-0}" = 1 ]; then
	echo "release: $wcc and makensis are needed for the Windows build" >&2
	exit 1
fi

body="Built from $short."
[ "$tag" = nightly ] && body="Rolling build of master, currently $short. Install with: curl -fsSL https://aego.akarpov.ru/install.sh | sh -s -- --nightly"

if [ "$dry" = 1 ]; then
	echo "release: dry run, would publish $name ($tag, prerelease=$pre) at $commit with:"
	ls -l "$dist"/aego-*.*
	exit 0
fi

: "${GITEA_TOKEN:?GITEA_TOKEN is not set}"
: "${GITEA_API:?GITEA_API is not set}"
api() {
	method=$1
	shift
	curl -fsS -X "$method" -H "Authorization: token $GITEA_TOKEN" "$@"
}
release_id() {
	api GET "$GITEA_API/releases/tags/$1" 2>/dev/null | sed -n 's/^{"id":\([0-9]*\),.*/\1/p'
}

id=$(release_id "$tag" || true)
if [ "$tag" = nightly ]; then
	[ -n "$id" ] && api DELETE "$GITEA_API/releases/$id" >/dev/null
	api DELETE "$GITEA_API/tags/nightly" >/dev/null 2>&1 || true
	id=""
fi
if [ -z "$id" ]; then
	json=$(printf '{"tag_name":"%s","target_commitish":"%s","name":"%s","body":"%s","prerelease":%s}' "$tag" "$commit" "$name" "$body" "$pre")
	id=$(api POST "$GITEA_API/releases" -H "Content-Type: application/json" -d "$json" | sed -n 's/^{"id":\([0-9]*\),.*/\1/p')
fi
[ -n "$id" ] || { echo "release: could not create release $tag" >&2; exit 1; }
for f in "$dist"/aego-*.tar.gz "$dist"/aego-*.zip "$dist"/aego-*-setup.exe "$dist"/aego-*.sha256; do
	[ -f "$f" ] || continue
	n=$(basename "$f")
	old=$(api GET "$GITEA_API/releases/$id/assets" | tr '{' '\n' | sed -n "s/^\"id\":\([0-9]*\),\"name\":\"$n\".*/\1/p")
	[ -n "$old" ] && api DELETE "$GITEA_API/releases/$id/assets/$old" >/dev/null
	api POST "$GITEA_API/releases/$id/assets?name=$n" -F "attachment=@$f" >/dev/null
	echo "release: uploaded $n"
done
echo "release: published $tag"
