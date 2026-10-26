#!/bin/sh
# Builds the Windows installer from placeholder files to check aego.nsi without a Windows build.
set -eu
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
for f in aego.exe aego-editor.exe VERSION README.txt; do
	printf 'placeholder\n' > "$tmp/$f"
done
(cd "$root" && go run ./tools/brandres -ico "$tmp/aego.ico")
makensis -V2 -DVERSION=check -DSRC="$tmp" -DOUT="$tmp/setup.exe" -DICON="$tmp/aego.ico" "$root/tools/release/windows/aego.nsi"
test -s "$tmp/setup.exe"
# with wine, install silently, check the installer kept a copy of itself for Restore Previous Version and that uninstalling removes it (D362)
if command -v wine >/dev/null 2>&1; then
	export WINEPREFIX="$tmp/wine" WINEDEBUG=-all
	cp "$tmp/setup.exe" "$tmp/aego-check-setup.exe"
	wine "$tmp/aego-check-setup.exe" /S >/dev/null 2>&1 || true
	dir="$WINEPREFIX/drive_c/users/$(id -un)/AppData/Local/Programs/Aego"
	cmp -s "$tmp/setup.exe" "$dir/setup.exe" || { echo "nsis-check: the installer did not keep setup.exe" >&2; exit 1; }
	wine "$dir/uninstall.exe" /S >/dev/null 2>&1 || true
	sleep 3
	[ ! -e "$dir/setup.exe" ] || { echo "nsis-check: the uninstaller left setup.exe" >&2; exit 1; }
fi
echo "nsis-check: ok"
