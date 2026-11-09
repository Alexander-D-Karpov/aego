#!/bin/sh
# Installs the Aego editor and CLI from the latest Gitea release.
set -eu

api="${AEGO_API:-https://git.akarpov.ru/api/v1/repos/sanspie/aego}"
docs="https://aego.akarpov.ru/install.html"
prefix="${AEGO_PREFIX:-$HOME/.local}"
which="latest"
deps=1
desktop=1

while [ $# -gt 0 ]; do
	case "$1" in
	--nightly) which="tags/nightly" ;;
	--version) which="tags/${2:?--version needs a tag}"; shift ;;
	--prefix) prefix="${2:?--prefix needs a directory}"; shift ;;
	--no-deps) deps=0 ;;
	--no-desktop) desktop=0 ;;
	-h | --help)
		echo "usage: install.sh [--nightly | --version vX.Y.Z] [--prefix DIR] [--no-deps] [--no-desktop]"
		exit 0 ;;
	*) echo "install.sh: unknown option $1" >&2; exit 2 ;;
	esac
	shift
done

say() { printf '%s\n' "$*" >&2; }
need() { command -v "$1" >/dev/null 2>&1 || { say "install.sh: $1 is required"; exit 1; }; }
need curl
need tar

os=$(uname -s)
arch=$(uname -m)
case "$os/$arch" in
Linux/x86_64 | Linux/amd64) target="linux-amd64" ;;
*)
	say "There is no prebuilt Aego for $os/$arch yet."
	say "Build it from source instead: $docs#build-from-source"
	exit 1 ;;
esac

json=$(curl -fsSL "$api/releases/$which" 2>/dev/null) || json=""
if [ -z "$json" ] && [ "$which" = latest ]; then
	say "No stable release yet; installing the nightly build."
	which="tags/nightly"
	json=$(curl -fsSL "$api/releases/$which" 2>/dev/null) || json=""
fi
if [ -z "$json" ]; then
	say "install.sh: no Aego release is published yet (looked for ${which#tags/})."
	say "Build it from source instead: $docs#build-from-source"
	exit 1
fi
url=$(printf '%s' "$json" | tr ',' '\n' | sed -n "s/.*\"browser_download_url\":\"\([^\"]*-$target\.tar\.gz\)\".*/\1/p" | head -n 1)
[ -n "$url" ] || { say "install.sh: release $which has no $target archive"; exit 1; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
file="$tmp/$(basename "$url")"
sums=$(printf '%s' "$json" | tr ',' '\n' | sed -n 's/.*"browser_download_url":"\([^"]*\/SHA256SUMS\.txt\)".*/\1/p' | head -n 1)
[ -n "$sums" ] || { say "install.sh: the release has no SHA256SUMS.txt"; exit 1; }
curl -fsSL -o "$tmp/SHA256SUMS.txt" "$sums?t=$(date +%s)"
want=$(grep " $(basename "$url")\$" "$tmp/SHA256SUMS.txt" | cut -d' ' -f1)
[ -n "$want" ] || { say "install.sh: SHA256SUMS.txt does not list $(basename "$url")"; exit 1; }
# the checksum in the URL keeps caching proxies from serving an earlier build under the same name
say "Downloading $url"
curl -fSL --progress-bar -o "$file" "$url?v=$want"
if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$file" | cut -d' ' -f1)
else
	got=$(shasum -a 256 "$file" | cut -d' ' -f1)
fi
[ "$want" = "$got" ] || { say "install.sh: checksum mismatch, refusing to install"; exit 1; }

tar -C "$tmp" -xzf "$file"
dir=$(find "$tmp" -mindepth 1 -maxdepth 1 -type d -name 'aego-*' | head -n 1)
# share/aego holds one folder per version; current and previous point at them, so the editor can update itself and go back
root="$prefix/share/aego"
mkdir -p "$prefix/bin" "$root"
if [ -d "$root/bin" ]; then
	rm -rf "$root.legacy" && mv "$root" "$root.legacy" && mkdir -p "$root" && mv "$root.legacy" "$root/legacy"
	ln -sfn legacy "$root/previous"
fi
ver=$(head -n 1 "$dir/VERSION" | sed 's/^v//')
[ "$ver" = nightly ] && ver="nightly-$(sed -n 2p "$dir/VERSION" | cut -c1-7)"
rm -rf "$root/$ver"
mv "$dir" "$root/$ver"
old=$(readlink "$root/current" 2>/dev/null || true)
ln -sfn "$ver" "$root/current"
[ -n "$old" ] && [ "$old" != "$ver" ] && ln -sfn "$old" "$root/previous"
ln -sf "$root/current/bin/aego" "$prefix/bin/aego"

# Runtime libraries: OpenGL with Mesa drivers, X11 and Wayland for the window, xkbcommon for keyboards.
libs="libGL.so.1 libEGL.so.1 libX11.so.6 libXext.so.6 libXrandr.so.2 libXcursor.so.1 libXfixes.so.3 libXi.so.6 libXss.so.1 libxkbcommon.so.0 libwayland-client.so.0 libwayland-egl.so.1 libwayland-cursor.so.0"

have_lib() {
	ldconfig -p 2>/dev/null | grep -q "[[:space:]]$1 " && return 0
	for d in /usr/lib /usr/lib64 /usr/lib/x86_64-linux-gnu /lib/x86_64-linux-gnu; do
		[ -e "$d/$1" ] && return 0
	done
	return 1
}

pkg_for() {
	case "$1:$2" in
	apt:libGL.so.1) echo "libgl1 libgl1-mesa-dri" ;;
	apt:libEGL.so.1) echo "libegl1" ;;
	apt:libX11.so.6) echo "libx11-6" ;;
	apt:libXext.so.6) echo "libxext6" ;;
	apt:libXrandr.so.2) echo "libxrandr2" ;;
	apt:libXcursor.so.1) echo "libxcursor1" ;;
	apt:libXfixes.so.3) echo "libxfixes3" ;;
	apt:libXi.so.6) echo "libxi6" ;;
	apt:libXss.so.1) echo "libxss1" ;;
	apt:libxkbcommon.so.0) echo "libxkbcommon0" ;;
	apt:libwayland-*) echo "libwayland-client0 libwayland-egl1 libwayland-cursor0" ;;
	dnf:libGL.so.1) echo "mesa-libGL mesa-dri-drivers" ;;
	dnf:libEGL.so.1) echo "mesa-libEGL" ;;
	dnf:libX11.so.6) echo "libX11" ;;
	dnf:libXext.so.6) echo "libXext" ;;
	dnf:libXrandr.so.2) echo "libXrandr" ;;
	dnf:libXcursor.so.1) echo "libXcursor" ;;
	dnf:libXfixes.so.3) echo "libXfixes" ;;
	dnf:libXi.so.6) echo "libXi" ;;
	dnf:libXss.so.1) echo "libXScrnSaver" ;;
	dnf:libxkbcommon.so.0) echo "libxkbcommon" ;;
	dnf:libwayland-*) echo "libwayland-client libwayland-egl libwayland-cursor" ;;
	pacman:libGL.so.1 | pacman:libEGL.so.1) echo "libglvnd mesa" ;;
	pacman:libX11.so.6) echo "libx11" ;;
	pacman:libXext.so.6) echo "libxext" ;;
	pacman:libXrandr.so.2) echo "libxrandr" ;;
	pacman:libXcursor.so.1) echo "libxcursor" ;;
	pacman:libXfixes.so.3) echo "libxfixes" ;;
	pacman:libXi.so.6) echo "libxi" ;;
	pacman:libXss.so.1) echo "libxss" ;;
	pacman:libxkbcommon.so.0) echo "libxkbcommon" ;;
	pacman:libwayland-*) echo "wayland" ;;
	zypper:libGL.so.1) echo "Mesa-libGL1 Mesa-dri" ;;
	zypper:libEGL.so.1) echo "Mesa-libEGL1" ;;
	zypper:libX11.so.6) echo "libX11-6" ;;
	zypper:libXext.so.6) echo "libXext6" ;;
	zypper:libXrandr.so.2) echo "libXrandr2" ;;
	zypper:libXcursor.so.1) echo "libXcursor1" ;;
	zypper:libXfixes.so.3) echo "libXfixes3" ;;
	zypper:libXi.so.6) echo "libXi6" ;;
	zypper:libXss.so.1) echo "libXss1" ;;
	zypper:libxkbcommon.so.0) echo "libxkbcommon0" ;;
	zypper:libwayland-*) echo "libwayland-client0 libwayland-egl1 libwayland-cursor0" ;;
	esac
}

install_deps() {
	missing=""
	for l in $libs; do
		have_lib "$l" || missing="$missing $l"
	done
	[ -n "$missing" ] || return 0
	pm=""
	for m in apt-get dnf pacman zypper; do
		if command -v "$m" >/dev/null 2>&1; then pm=$m; break; fi
	done
	[ "$pm" = apt-get ] && pm=apt
	pkgs=""
	for l in $missing; do
		for p in $(pkg_for "$pm" "$l"); do
			case " $pkgs " in *" $p "*) ;; *) pkgs="$pkgs $p" ;; esac
		done
	done
	say "Missing system libraries:$missing"
	if [ -z "$pm" ] || [ -z "$pkgs" ]; then
		say "Install them with your package manager: OpenGL (Mesa), X11 and Wayland client libraries."
		return 0
	fi
	run=""
	if [ "$(id -u)" != 0 ]; then
		if command -v sudo >/dev/null 2>&1; then run="sudo"; else
			say "Run as root to install:$pkgs"
			return 0
		fi
	fi
	in=/dev/null
	if (exec </dev/tty) 2>/dev/null; then in=/dev/tty; fi
	say "Installing:$pkgs"
	case "$pm" in
	apt) $run apt-get update -qq <"$in" && $run apt-get install -y $pkgs <"$in" ;;
	dnf) $run dnf install -y $pkgs <"$in" ;;
	pacman) $run pacman -S --needed --noconfirm $pkgs <"$in" ;;
	zypper) $run zypper --non-interactive install $pkgs <"$in" ;;
	esac || say "The packages could not be installed; install them by hand:$pkgs"
}

check_gl() {
	command -v glxinfo >/dev/null 2>&1 || return 0
	[ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ] || return 0
	v=$(glxinfo 2>/dev/null | sed -n 's/^OpenGL core profile version string: \([0-9]*\)\.\([0-9]*\).*/\1 \2/p' | head -n 1)
	[ -n "$v" ] || return 0
	set -- $v
	if [ "$1" -lt 3 ] || { [ "$1" -eq 3 ] && [ "$2" -lt 3 ]; }; then
		say "This system offers OpenGL $1.$2; Aego needs 3.3. Update the GPU driver or Mesa, or run with LIBGL_ALWAYS_SOFTWARE=1."
	fi
}

check_glibc() {
	v=$(ldd --version 2>/dev/null | head -n 1 | sed -n 's/.* \([0-9]*\)\.\([0-9]*\)$/\1 \2/p')
	[ -n "$v" ] || return 0
	set -- $v
	if [ "$1" -lt 2 ] || { [ "$1" -eq 2 ] && [ "$2" -lt 39 ]; }; then
		say "This system has glibc $1.$2; the prebuilt Aego needs 2.39 or newer (Ubuntu 24.04, Debian 13, Fedora 40). Build from source instead: $docs#build-from-source"
	fi
}

install_desktop() {
	# Both desktop entries contain the prefix in Exec. Refuse ambiguous field
	# codes, quoting or control characters rather than emit a broken command.
	case "$prefix" in
	*\"*|*\\*|*\$*|*\`*|*%*)
		printf '%s\n' 'install.sh: desktop registration skipped for an unsafe Exec path' >&2
		return 0 ;;
	esac
	# grep treats newlines as record separators, so count control bytes instead.
	if [ "$(printf '%s' "$prefix" | LC_ALL=C tr -cd '\001-\037\177' | wc -c)" -ne 0 ]; then
		printf '%s\n' 'install.sh: desktop registration skipped for an Exec path containing control characters' >&2
		return 0
	fi
	data="${XDG_DATA_HOME:-$HOME/.local/share}"
	mkdir -p "$data/applications" "$data/icons/hicolor/256x256/apps" "$data/mime/packages"
	[ -f "$root/current/share/aego.png" ] && cp "$root/current/share/aego.png" "$data/icons/hicolor/256x256/apps/aego.png"
	cat > "$data/applications/aego.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=Aego
Comment=Game engine and editor
Exec="$prefix/bin/aego" edit %f
Icon=aego
Categories=Development;IDE;
MimeType=application/x-aego-project;application/x-aego-scene;
Terminal=false
EOF
	# Keep file associations separate from the single untrusted protocol URL.
	protocol_ready=0
	protocol_tmp=$(mktemp "$data/applications/.aego-example.XXXXXXXX")
	cat > "$protocol_tmp" <<EOF
[Desktop Entry]
Type=Application
Name=Aego
Comment=Game engine and editor
Exec="$prefix/bin/aego" edit --example-link %u
Icon=aego
Categories=Development;IDE;
MimeType=x-scheme-handler/aego;
NoDisplay=true
Terminal=false
EOF
	protocol_file="$data/applications/aego-example.desktop"
	if [ -L "$protocol_file" ]; then
		printf '%s\n' 'install.sh: existing protocol desktop symlink preserved' >&2
	elif [ -e "$protocol_file" ]; then
		if [ -f "$protocol_file" ] && cmp -s "$protocol_tmp" "$protocol_file"; then
			protocol_ready=1
		else
			printf '%s\n' 'install.sh: existing protocol desktop file preserved' >&2
		fi
	elif ln "$protocol_tmp" "$protocol_file" 2>/dev/null; then
		# Hard-link publication fails instead of replacing a newly appeared file.
		protocol_ready=1
	else
		printf '%s\n' 'install.sh: could not safely create protocol desktop file' >&2
	fi
	rm -f "$protocol_tmp"
	cat > "$data/mime/packages/aego.xml" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<mime-info xmlns="http://www.freedesktop.org/standards/shared-mime-info">
  <mime-type type="application/x-aego-project">
    <comment>Aego project</comment>
    <glob pattern="project.aego"/>
    <glob pattern="*.aego"/>
  </mime-type>
  <mime-type type="application/x-aego-scene">
    <comment>Aego scene</comment>
    <glob pattern="*.aescene"/>
    <glob pattern="*.aeprefab"/>
  </mime-type>
</mime-info>
EOF
	command -v update-mime-database >/dev/null 2>&1 && update-mime-database "$data/mime" >/dev/null 2>&1 || true
	command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$data/applications" >/dev/null 2>&1 || true
	command -v xdg-mime >/dev/null 2>&1 && xdg-mime default aego.desktop application/x-aego-project application/x-aego-scene >/dev/null 2>&1 || true
	if [ "$protocol_ready" = 1 ] && command -v xdg-mime >/dev/null 2>&1; then
		current=$(xdg-mime query default x-scheme-handler/aego 2>/dev/null || true)
		case "$current" in
		"" | aego-example.desktop) xdg-mime default aego-example.desktop x-scheme-handler/aego >/dev/null 2>&1 || true ;;
		*) printf '%s\n' "install.sh: existing aego: handler preserved: $current" >&2 ;;
		esac
	fi
}

check_glibc
if [ "$deps" = 1 ]; then
	install_deps
	check_gl
fi
[ "$desktop" = 1 ] && install_desktop

say "Installed $(head -n 1 "$root/current/VERSION") to $root/$ver"
case ":$PATH:" in
*":$prefix/bin:"*) ;;
*) say "Add $prefix/bin to PATH, for example: export PATH=\"$prefix/bin:\$PATH\"" ;;
esac
say "Start with: aego new mygame && aego edit mygame"
