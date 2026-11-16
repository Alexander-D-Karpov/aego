#!/usr/bin/env bash
set -euo pipefail

# Versioned packaging is manual GitHub-hosted only; normal CI uses 0.0.0.
package_version=0.0.0
package_commit=
update_key=
update_fingerprint=
if [[ -n "${AEGO_CI_PACKAGE_VERSION+x}" || -n "${AEGO_CI_PACKAGE_COMMIT+x}" ]]; then
  package_version="${AEGO_CI_PACKAGE_VERSION:-}"
  package_commit="${AEGO_CI_PACKAGE_COMMIT:-}"
  [[ "${GITHUB_ACTIONS:-}" == true && "${GITHUB_SERVER_URL:-}" == https://github.com &&
     "${RUNNER_ENVIRONMENT:-}" == github-hosted && "${GITHUB_EVENT_NAME:-}" == workflow_dispatch &&
     "${GITHUB_WORKFLOW:-}" == aego-native-artifacts-only && "${RUNNER_OS:-}" == Windows ]] || { echo 'Versioned packaging requires the separate manual, GitHub-hosted artifact workflow on Windows.' >&2; exit 1; }
  [[ "$package_version" =~ ^(0|[1-9][0-9]{0,4})\.(0|[1-9][0-9]{0,4})\.(0|[1-9][0-9]{0,4})$ ]] || { echo 'Invalid stable package version.' >&2; exit 1; }
  IFS=. read -r major minor patch <<< "$package_version"
  (( major <= 65535 && minor <= 65535 && patch <= 65535 )) || { echo 'Package version exceeds the Windows resource limit.' >&2; exit 1; }
  source_version="$(sed -n 's/^var Version = "\([^"]*\)"[[:space:]]*$/\1/p' engine/config.go)"
  [[ "$source_version" == "$package_version" ]] || { echo 'Package version does not match engine.Version.' >&2; exit 1; }
  [[ "$package_commit" =~ ^[0-9a-f]{40}$ && "${GITHUB_SHA:-}" == "$package_commit" && "$(git rev-parse HEAD)" == "$package_commit" ]] || { echo 'Package commit does not match the event and checkout.' >&2; exit 1; }
  [[ "${RUNNER_ARCH:-}/$(go env GOOS)/$(go env GOARCH)" == X64/windows/amd64 ]] || { echo 'Windows runner and Go target do not match.' >&2; exit 1; }
  tracked_flags="$(git ls-files -v)"
  [[ -n "$tracked_flags" ]] && ! grep -Ev '^H ' <<< "$tracked_flags" >/dev/null || { echo 'Unexpected Git index flags.' >&2; exit 1; }
  source_status="$(git status --porcelain --untracked-files=all)"
  ignored_sources="$(git ls-files --others --ignored --exclude-standard -- . ':(exclude).cache/**')"
  [[ -z "$source_status" && -z "$ignored_sources" ]] || { echo 'Package source is not clean.' >&2; exit 1; }
  export GOFLAGS='' GOWORK=off
  update_key="$(py -3 -B tools/ci/release_update_key.py --value)"
  [[ "$update_key" =~ ^[A-Za-z0-9+/]{43}=$ ]] || { echo 'Invalid trusted update key.' >&2; exit 1; }
  update_fingerprint="$(AEGO_CI_UPDATE_PUBLIC_KEY="$update_key" py -3 -B tools/ci/release_update_key.py --fingerprint)"
fi

root="$PWD/.cache/github-ci"
stage="$root/windows-stage"
portable="$root/dist/portable"
installer="$root/dist/installer"
if [[ -n "$package_commit" ]]; then
  for path in "$PWD/.cache" "$root" "$root/dist" "$root/logs"; do
    [[ ! -L "$path" ]] || { echo "Package output parent is a symlink: $path" >&2; exit 1; }
  done
  for path in "$stage" "$portable" "$installer" "$root/aego.ico" \
    "$root/logs/windows-cli-pe.txt" "$root/logs/windows-editor-pe.txt" \
    cmd/aego/rsrc_windows_amd64.syso cmd/aego-editor/rsrc_windows_amd64.syso; do
    [[ ! -e "$path" && ! -L "$path" ]] || { echo "Package output already exists: $path" >&2; exit 1; }
  done
fi
mkdir -p "$stage" "$portable" "$installer" "$root/logs"
if [[ -n "$package_commit" ]]; then
  printf '%s\n%s\n' "$package_version" "$package_commit" > "$stage/VERSION"
else
  printf 'CI build\n%s\n' "$(git rev-parse HEAD)" > "$stage/VERSION"
fi
cp tools/release/README.txt "$stage/README.txt"
go run ./tools/brandres -ico "$root/aego.ico" -syso cmd/aego,cmd/aego-editor -version "$package_version"
update_ldflags=
if [[ -n "$package_commit" ]]; then
  update_ldflags="-X=aego/update.PublicKey=$update_key"
  go build -trimpath -ldflags "$update_ldflags" -o "$stage/aego.exe" ./cmd/aego
else
  go build -trimpath -o "$stage/aego.exe" ./cmd/aego
fi
go build -trimpath -tags aego_editor -ldflags "-H=windowsgui${update_ldflags:+ $update_ldflags}" -o "$stage/aego-editor.exe" ./cmd/aego-editor
if [[ -n "$package_commit" ]]; then
  cli_check="$(cygpath -w "$stage/aego.exe")"
  editor_check="$(cygpath -w "$stage/aego-editor.exe")"
  [[ -n "$cli_check" && -n "$editor_check" ]] || { echo 'Windows update check path is empty.' >&2; exit 1; }
  AEGO_CI_UPDATE_PUBLIC_KEY="$update_key" py -3 -B tools/ci/release_update_key.py --smoke "$cli_check"
  AEGO_CI_UPDATE_PUBLIC_KEY="$update_key" py -3 -B tools/ci/release_update_key.py --check-binary "$editor_check"
fi
objdump -p "$stage/aego.exe" > "$root/logs/windows-cli-pe.txt"
objdump -p "$stage/aego-editor.exe" > "$root/logs/windows-editor-pe.txt"
AEGO_CI_STAGE="$(cygpath -w "$stage")"
AEGO_CI_ZIP="$(cygpath -w "$portable/aego-windows-amd64-portable.zip")"
nsis_out="$(cygpath -w "$installer/aego-windows-amd64-setup.exe")"
nsis_icon="$(cygpath -w "$root/aego.ico")"
nsis_script="$(cygpath -w "$PWD/tools/release/windows/aego.nsi")"
[[ -n "$AEGO_CI_STAGE" && -n "$AEGO_CI_ZIP" && -n "$nsis_out" && -n "$nsis_icon" && -n "$nsis_script" ]] || {
  echo 'Windows path conversion returned an empty path.' >&2
  exit 1
}
export AEGO_CI_STAGE AEGO_CI_ZIP
pwsh -NoProfile -Command 'Compress-Archive -Path "$env:AEGO_CI_STAGE\*" -DestinationPath $env:AEGO_CI_ZIP'
makensis -V2 "-DVERSION=$package_version" \
  "-DSRC=$AEGO_CI_STAGE" \
  "-DOUT=$nsis_out" \
  "-DICON=$nsis_icon" \
  "$nsis_script"
(cd "$portable" && sha256sum aego-windows-amd64-portable.zip > SHA256SUMS.txt)
(cd "$installer" && sha256sum aego-windows-amd64-setup.exe > SHA256SUMS.txt)
if [[ -n "$package_commit" ]]; then
  for dir in "$portable" "$installer"; do
    printf 'version=%s\ncommit=%s\nstatus=unsigned artifact candidate\nupdate_key=embedded\nupdate_key_sha256=%s\n' \
      "$package_version" "$package_commit" "$update_fingerprint" > "$dir/CI-PROVENANCE.txt"
  done
fi
