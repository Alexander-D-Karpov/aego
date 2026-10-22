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

# sum adds a file to SHA256SUMS.txt; Gitea accepts only a few attachment types, .txt among them.
sum() {
	(cd "$dist" && sha256sum "$1" >> SHA256SUMS.txt)
}

stage() {
	mkdir -p "$1/bin" "$1/share"
	printf '%s\n%s\n' "$tag" "$commit" > "$1/VERSION"
	cp tools/release/README.txt "$1/README.txt"
	cp app/brand/png/aego-256.png "$1/share/aego.png"
}

# Linux: the host build.
base="aego-$label-linux-amd64"
stage "$dist/$base"
CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o "$dist/$base/bin/aego" ./cmd/aego
tar -C "$dist" -czf "$dist/$base.tar.gz" "$base"
sum "$base.tar.gz"
echo "release: built $base.tar.gz"
if [ "${SKIP_APPIMAGE:-0}" != 1 ]; then
	app="Aego-$label-x86_64.AppImage"
	sh tools/release/appimage.sh "$dist/$base" "$dist/$app"
	sum "$app"
fi

# Windows: cross-built with mingw-w64 and packed by NSIS.
wcc=x86_64-w64-mingw32-gcc
if command -v "$wcc" >/dev/null 2>&1 && command -v makensis >/dev/null 2>&1; then
	CC=$wcc GOOS=windows GOARCH=amd64 bash tools/bootstrap-native.sh
	win="aego-$label-windows-amd64"
	stage "$dist/$win"
	trap 'rm -f cmd/aego/rsrc_windows_*.syso cmd/aego-editor/rsrc_windows_*.syso' EXIT
	go run ./tools/brandres -ico "$dist/aego.ico" -syso cmd/aego,cmd/aego-editor -version "$label"
	export CGO_ENABLED=1 CC=$wcc GOOS=windows GOARCH=amd64
	go build -trimpath -ldflags "-s -w" -o "$dist/$win/aego.exe" ./cmd/aego
	go build -trimpath -tags aego_editor -ldflags "-s -w -H=windowsgui" -o "$dist/$win/aego-editor.exe" ./cmd/aego-editor
	unset CGO_ENABLED CC GOOS GOARCH
	rm -f cmd/aego/rsrc_windows_*.syso cmd/aego-editor/rsrc_windows_*.syso
	rmdir "$dist/$win/bin"
	rm -rf "$dist/$win/share"
	(cd "$dist" && zip -qr "$win.zip" "$win")
	sum "$win.zip"
	mkdir -p "$dist/setup"
	makensis -V2 -DVERSION="$label" -DSRC="$dist/$win" -DOUT="$dist/setup/$win-setup.exe" -DICON="$dist/aego.ico" tools/release/windows/aego.nsi
	cp "$dist/setup/$win-setup.exe" "$dist/$win-setup.exe"
	sum "$win-setup.exe"
	(cd "$dist/setup" && zip -q "../$win-setup.zip" "$win-setup.exe")
	sum "$win-setup.zip"
	echo "release: built $win.zip and $win-setup.zip"
elif [ "${REQUIRE_WINDOWS:-0}" = 1 ]; then
	echo "release: $wcc and makensis are needed for the Windows build" >&2
	exit 1
fi

body="Built from $short."
# tools/release/notes/<tag>.md, when present, is the text of the release page.
notes="$(dirname "$0")/notes/$tag.md"
[ -f "$notes" ] && body="$(cat "$notes")

$body"
[ "$tag" = nightly ] && body="Rolling build of master, currently $short. Install with: curl -fsSL https://aego.akarpov.ru/install.sh | sh -s -- --nightly"

if [ "$dry" = 1 ]; then
	echo "release: dry run, would publish $name ($tag, prerelease=$pre) at $commit with:"
	ls -l "$dist"/aego-*.* "$dist"/Aego-*.* "$dist"/SHA256SUMS.txt 2>/dev/null || true
	cat "$dist"/SHA256SUMS.txt
	exit 0
fi

: "${GITEA_TOKEN:?GITEA_TOKEN is not set}"
: "${GITEA_API:?GITEA_API is not set}"
# GITEA_RESOLVE=host:443:ip sends API calls straight to the Gitea server, past a slow proxy.
api() {
	method=$1
	shift
	set -- -fsS -m 1800 -X "$method" -H "Authorization: token $GITEA_TOKEN" "$@"
	[ -n "${GITEA_RESOLVE:-}" ] && set -- --resolve "$GITEA_RESOLVE" "$@"
	curl "$@"
}
jstr() {
	python3 -c 'import json, sys; sys.stdout.write(json.dumps(sys.stdin.read()))'
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
	json=$(printf '{"tag_name":"%s","target_commitish":"%s","name":"%s","body":%s,"prerelease":%s}' "$tag" "$commit" "$name" "$(printf '%s' "$body" | jstr)" "$pre")
	id=$(api POST "$GITEA_API/releases" -H "Content-Type: application/json" -d "$json" | sed -n 's/^{"id":\([0-9]*\),.*/\1/p')
else
	api PATCH "$GITEA_API/releases/$id" -H "Content-Type: application/json" -d "{\"body\":$(printf '%s' "$body" | jstr)}" >/dev/null || true
fi
[ -n "$id" ] || { echo "release: could not create release $tag" >&2; exit 1; }
failed=""
for f in "$dist"/aego-*.tar.gz "$dist"/SHA256SUMS.txt "$dist"/aego-*.zip "$dist"/aego-*-setup.exe "$dist"/Aego-*.AppImage; do
	[ -f "$f" ] || continue
	n=$(basename "$f")
	old=$(api GET "$GITEA_API/releases/$id/assets" | tr '{' '\n' | sed -n "s/^\"id\":\([0-9]*\),\"name\":\"$n\".*/\1/p")
	[ -n "$old" ] && api DELETE "$GITEA_API/releases/$id/assets/$old" >/dev/null
	ok=0
	for attempt in 1 2 3 4 5; do
		code=$(curl -sS -m 1800 ${GITEA_RESOLVE:+--resolve "$GITEA_RESOLVE"} -o "$dist/upload.out" -w '%{http_code}' \
			-H "Authorization: token $GITEA_TOKEN" -F "attachment=@$f" "$GITEA_API/releases/$id/assets?name=$n" || echo 000)
		if [ "$code" = 201 ]; then
			ok=1
			break
		fi
		echo "release: upload of $n failed with HTTP $code, attempt $attempt: $(head -c 400 "$dist/upload.out" 2>/dev/null)" >&2
		sleep 10
		stale=$(api GET "$GITEA_API/releases/$id/assets" | tr '{' '\n' | sed -n "s/^\"id\":\([0-9]*\),\"name\":\"$n\".*/\1/p")
		[ -n "$stale" ] && api DELETE "$GITEA_API/releases/$id/assets/$stale" >/dev/null
	done
	if [ "$ok" != 1 ]; then
		why=$(printf 'Upload of %s failed with HTTP %s: %s' "$n" "$code" "$(head -c 300 "$dist/upload.out" 2>/dev/null | tr -d '"\\\n\r')")
		api PATCH "$GITEA_API/releases/$id" -H "Content-Type: application/json" -d "{\"body\":$(printf '%s %s' "$body" "$why" | jstr)}" >/dev/null || true
		echo "release: $why" >&2
		failed="$failed $n"
		continue
	fi
	echo "release: uploaded $n"
done
if [ -n "$failed" ]; then
	echo "release: published $tag without$failed" >&2
	exit 1
fi
echo "release: published $tag"
