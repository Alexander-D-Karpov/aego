#!/bin/sh
# Packs the Linux build into an AppImage whose launch opens the Aego launcher.
# Usage: tools/release/appimage.sh <stage dir with bin/aego> <out.AppImage>
set -eu
src=${1:?usage: appimage.sh <stage dir> <out.AppImage>}
out=${2:?usage: appimage.sh <stage dir> <out.AppImage>}
root=$(cd "$(dirname "$0")/../.." && pwd)
tool=${APPIMAGETOOL:-$(command -v appimagetool || true)}
if [ -z "$tool" ]; then
	tool="$root/.cache/tools/appimagetool"
	if [ ! -x "$tool" ]; then
		mkdir -p "$(dirname "$tool")"
		curl -fsSL -o "$tool" "${APPIMAGETOOL_URL:-https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-x86_64.AppImage}"
		chmod +x "$tool"
	fi
fi
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
app="$work/Aego.AppDir"
mkdir -p "$app/usr/bin" "$app/usr/share/aego"
cp "$src/bin/aego" "$app/usr/bin/aego"
cp "$src/VERSION" "$src/README.txt" "$app/usr/share/aego/"
cp "$root/app/brand/png/aego-256.png" "$app/aego.png"
cat > "$app/AppRun" <<'RUN'
#!/bin/sh
here=$(dirname "$(readlink -f "$0")")
exec "$here/usr/bin/aego" edit "$@"
RUN
chmod +x "$app/AppRun"
cat > "$app/aego.desktop" <<'DESKTOP'
[Desktop Entry]
Type=Application
Name=Aego
Comment=Game engine and editor
Exec=aego %f
Icon=aego
Categories=Development;IDE;
MimeType=application/x-aego-project;application/x-aego-scene;
Terminal=false
DESKTOP
APPIMAGE_EXTRACT_AND_RUN=1 ARCH=x86_64 "$tool" --no-appstream "$app" "$out"
echo "appimage: built $out"
