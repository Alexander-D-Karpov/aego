#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION_FILE="$ROOT/third_party/sdl/version.txt"
SDL_VERSION="$(awk -F= '$1=="version"{print $2}' "$VERSION_FILE" | tr -d '\r')"
SDL_SHA256="$(awk -F= '$1=="sha256"{print $2}' "$VERSION_FILE" | tr -d '\r')"

GOOS="${GOOS:-$(go env GOOS)}"
GOARCH="${GOARCH:-$(go env GOARCH)}"
TARGET="${GOOS}_${GOARCH}"
PREFIX="$ROOT/.cache/native/$TARGET"
STAMP="$PREFIX/.sdl-$SDL_VERSION"

if [[ -f "$STAMP" && "${FORCE:-0}" != "1" ]]; then
  echo "SDL $SDL_VERSION already installed in $PREFIX"
  exit 0
fi

DOWNLOAD_DIR="$ROOT/.cache/download"
WORK="$ROOT/.cache/build/sdl-$SDL_VERSION-$TARGET"
TARBALL="$DOWNLOAD_DIR/SDL3-$SDL_VERSION.tar.gz"
URL="https://github.com/libsdl-org/SDL/releases/download/release-$SDL_VERSION/SDL3-$SDL_VERSION.tar.gz"

mkdir -p "$DOWNLOAD_DIR" "$WORK"

if [[ ! -f "$TARBALL" ]]; then
  echo "downloading $URL"
  curl -fsSL "$URL" -o "$TARBALL.part"
  mv "$TARBALL.part" "$TARBALL"
fi

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

ACTUAL="$(sha256_of "$TARBALL")"
if [[ -z "$SDL_SHA256" ]]; then
  echo "warning: sha256 is not pinned in $VERSION_FILE, got $ACTUAL"
elif [[ "$ACTUAL" != "$SDL_SHA256" ]]; then
  echo "sha256 mismatch for $TARBALL: expected $SDL_SHA256, got $ACTUAL" >&2
  rm -f "$TARBALL"
  exit 1
fi

rm -rf "$WORK/src" "$WORK/build"
mkdir -p "$WORK/src"
tar -xzf "$TARBALL" -C "$WORK/src" --strip-components=1

shopt -s nullglob
for p in "$ROOT"/third_party/sdl/patches/*.patch; do
  echo "applying $(basename "$p")"
  patch -d "$WORK/src" -p1 < "$p"
done
shopt -u nullglob

CMAKE_ARGS=(
  -DCMAKE_BUILD_TYPE=Release
  -DCMAKE_INSTALL_LIBDIR=lib
  -DCMAKE_POSITION_INDEPENDENT_CODE=ON
  -DSDL_SHARED=OFF
  -DSDL_STATIC=ON
  -DSDL_TEST_LIBRARY=OFF
  -DSDL_TESTS=OFF
  -DSDL_EXAMPLES=OFF
  -DSDL_GPU=OFF
  -DSDL_RENDER=OFF
  -DSDL_CAMERA=OFF
)

GENERATOR=()
if command -v ninja >/dev/null 2>&1; then
  GENERATOR=(-G Ninja)
fi

case "$GOOS" in
  darwin)
    case "$GOARCH" in
      amd64) OSX_ARCH=x86_64 ;;
      arm64) OSX_ARCH=arm64 ;;
      *) echo "unsupported darwin arch $GOARCH" >&2; exit 1 ;;
    esac
    CMAKE_ARGS+=(-DCMAKE_OSX_ARCHITECTURES="$OSX_ARCH" -DCMAKE_OSX_DEPLOYMENT_TARGET=11.0)
    ;;
  windows)
    export CC="${CC:-gcc}"
    if [[ ${#GENERATOR[@]} -eq 0 ]]; then
      GENERATOR=(-G "MinGW Makefiles")
    fi
    ;;
  linux)
    ;;
  *)
    echo "unsupported GOOS $GOOS" >&2
    exit 1
    ;;
esac

cmake -S "$WORK/src" -B "$WORK/build" "${GENERATOR[@]}" "${CMAKE_ARGS[@]}"
cmake --build "$WORK/build" --config Release --parallel
rm -rf "$PREFIX"
cmake --install "$WORK/build" --prefix "$PREFIX" --config Release
touch "$STAMP"
echo "SDL $SDL_VERSION installed into $PREFIX"