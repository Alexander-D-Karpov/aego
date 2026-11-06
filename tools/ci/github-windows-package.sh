#!/usr/bin/env bash
set -euo pipefail

root="$PWD/.cache/github-ci"
stage="$root/windows-stage"
portable="$root/dist/portable"
installer="$root/dist/installer"
mkdir -p "$stage" "$portable" "$installer"
printf 'CI build\n%s\n' "$(git rev-parse HEAD)" > "$stage/VERSION"
cp tools/release/README.txt "$stage/README.txt"
go run ./tools/brandres -ico "$root/aego.ico" -syso cmd/aego,cmd/aego-editor -version 0.0.0
go build -trimpath -o "$stage/aego.exe" ./cmd/aego
go build -trimpath -tags aego_editor -ldflags '-H=windowsgui' -o "$stage/aego-editor.exe" ./cmd/aego-editor
objdump -p "$stage/aego.exe" > "$root/logs/windows-cli-pe.txt"
objdump -p "$stage/aego-editor.exe" > "$root/logs/windows-editor-pe.txt"
export AEGO_CI_STAGE="$(cygpath -w "$stage")"
export AEGO_CI_ZIP="$(cygpath -w "$portable/aego-windows-amd64-portable.zip")"
pwsh -NoProfile -Command 'Compress-Archive -Path "$env:AEGO_CI_STAGE\*" -DestinationPath $env:AEGO_CI_ZIP'
makensis -V2 -DVERSION=0.0.0 \
  "-DSRC=$(cygpath -w "$stage")" \
  "-DOUT=$(cygpath -w "$installer/aego-windows-amd64-setup.exe")" \
  "-DICON=$(cygpath -w "$root/aego.ico")" \
  "$(cygpath -w "$PWD/tools/release/windows/aego.nsi")"
(cd "$portable" && sha256sum aego-windows-amd64-portable.zip > SHA256SUMS.txt)
(cd "$installer" && sha256sum aego-windows-amd64-setup.exe > SHA256SUMS.txt)
