#!/bin/sh
# Packs the Linux build into an AppImage whose launch opens the Aego launcher.
set -eu
src=${1:?usage: appimage.sh <stage dir> <out.AppImage>}
out=${2:?usage: appimage.sh <stage dir> <out.AppImage>}
root=$(cd "$(dirname "$0")/../.." && pwd)
# Explicit or PATH tools are caller-managed; downloads and cached tools require a pinned hash.
tool=${APPIMAGETOOL:-$(command -v appimagetool || true)}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
valid_sha256() {
	case "$1" in ''|*[!0123456789abcdef]*) return 1 ;; esac
	[ "${#1}" -eq 64 ]
}
sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		value=$(sha256sum "$1")
	elif command -v shasum >/dev/null 2>&1; then
		value=$(shasum -a 256 "$1")
	else
		echo 'appimage: sha256sum or shasum is required' >&2; return 1
	fi
	printf '%s\n' "${value%% *}"
}
if [ -z "$tool" ]; then
	want=${APPIMAGETOOL_SHA256:-e8c1169c4af66c3184d7ee110713d4be04befe671fa358e2c1591ee840553b03}
	valid_sha256 "$want" || { echo 'appimage: invalid appimagetool SHA-256' >&2; exit 1; }
	cache="$root/.cache/tools/appimagetool"
	downloaded=0
	if [ -e "$cache" ] || [ -L "$cache" ]; then
		[ -f "$cache" ] && [ ! -L "$cache" ] && [ -x "$cache" ] || { echo 'appimage: cached appimagetool is not a regular executable' >&2; exit 1; }
		tool=$cache
	else
		tool="$work/appimagetool"
		curl -fsSL -o "$tool" "${APPIMAGETOOL_URL:-https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-x86_64.AppImage}"
		downloaded=1
	fi
	got=$(sha256_of "$tool")
	[ "$got" = "$want" ] || { echo 'appimage: appimagetool checksum mismatch; refusing to execute' >&2; exit 1; }
	[ "$downloaded" = 0 ] || chmod +x "$tool"
fi
# Always pin the executable AppImage runtime separately.
runtime_sha=${APPIMAGE_RUNTIME_SHA256:-617aa77314d57c8b8e4fae9c3f0b35af7b30d5529cb69e98f64f92510ed98a46}
valid_sha256 "$runtime_sha" || { echo 'appimage: invalid runtime SHA-256' >&2; exit 1; }
runtime=${APPIMAGE_RUNTIME:-}
if [ -n "$runtime" ]; then
	[ -f "$runtime" ] || { echo 'appimage: runtime file does not exist' >&2; exit 1; }
else
	runtime="$work/runtime-x86_64"
	curl -fsSL -o "$runtime" "${APPIMAGE_RUNTIME_URL:-https://github.com/AppImage/type2-runtime/releases/download/continuous/runtime-x86_64}"
fi
[ "$(sha256_of "$runtime")" = "$runtime_sha" ] || { echo 'appimage: runtime checksum mismatch; refusing to build' >&2; exit 1; }
app="$work/Aego.AppDir"
mkdir -p "$app/usr/bin" "$app/usr/share/aego"
cp "$src/bin/aego" "$app/usr/bin/aego"
cp "$src/VERSION" "$src/README.txt" "$app/usr/share/aego/"
cp "$root/app/brand/png/aego-256.png" "$app/aego.png"
cat > "$app/AppRun" <<'RUN'
#!/bin/sh
here=$(dirname "$(readlink -f "$0")")
case "${1:-}" in
	aego:*) exec "$here/usr/bin/aego" edit --example-link "$@" ;;
	file:*) exec "$here/usr/bin/aego" edit --file-link "$@" ;;
esac
exec "$here/usr/bin/aego" edit "$@"
RUN
chmod +x "$app/AppRun"
cat > "$app/aego.desktop" <<'DESKTOP'
[Desktop Entry]
Type=Application
Name=Aego
Comment=Game engine and editor
Exec=aego %u
Icon=aego
Categories=Development;IDE;
MimeType=application/x-aego-project;application/x-aego-scene;x-scheme-handler/aego;
Terminal=false
DESKTOP
APPIMAGE_EXTRACT_AND_RUN=1 ARCH=x86_64 "$tool" --no-appstream --runtime-file "$runtime" "$app" "$out"
echo "appimage: built $out"
