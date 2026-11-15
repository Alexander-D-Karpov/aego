#!/usr/bin/env bash
set -euo pipefail

# Versioned packaging is manual GitHub-hosted only; normal CI uses 0.0.0.
package_version=0.0.0
package_commit=
if [[ -n "${AEGO_CI_PACKAGE_VERSION+x}" || -n "${AEGO_CI_PACKAGE_COMMIT+x}" ]]; then
  package_version="${AEGO_CI_PACKAGE_VERSION:-}"
  package_commit="${AEGO_CI_PACKAGE_COMMIT:-}"
  [[ "${GITHUB_ACTIONS:-}" == true && "${GITHUB_SERVER_URL:-}" == https://github.com &&
     "${RUNNER_ENVIRONMENT:-}" == github-hosted && "${GITHUB_EVENT_NAME:-}" == workflow_dispatch &&
     "${GITHUB_WORKFLOW:-}" == aego-native-artifacts-only && "${RUNNER_OS:-}" == macOS ]] || { echo 'Versioned packaging requires the separate manual, GitHub-hosted artifact workflow on macOS.' >&2; exit 1; }
  [[ "$package_version" =~ ^(0|[1-9][0-9]{0,4})\.(0|[1-9][0-9]{0,4})\.(0|[1-9][0-9]{0,4})$ ]] || { echo 'Invalid stable package version.' >&2; exit 1; }
  IFS=. read -r major minor patch <<< "$package_version"
  (( major <= 65535 && minor <= 65535 && patch <= 65535 )) || { echo 'Package version exceeds the Windows resource limit.' >&2; exit 1; }
  source_version="$(sed -n 's/^var Version = "\([^"]*\)"[[:space:]]*$/\1/p' engine/config.go)"
  [[ "$source_version" == "$package_version" ]] || { echo 'Package version does not match engine.Version.' >&2; exit 1; }
  [[ "$package_commit" =~ ^[0-9a-f]{40}$ && "${GITHUB_SHA:-}" == "$package_commit" && "$(git rev-parse HEAD)" == "$package_commit" ]] || { echo 'Package commit does not match the event and checkout.' >&2; exit 1; }
  case "${RUNNER_ARCH:-}/$(go env GOOS)/$(go env GOARCH)" in
    X64/darwin/amd64|ARM64/darwin/arm64) ;;
    *) echo 'macOS runner and Go target do not match.' >&2; exit 1 ;;
  esac
  tracked_flags="$(git ls-files -v)"
  [[ -n "$tracked_flags" ]] && ! grep -Ev '^H ' <<< "$tracked_flags" >/dev/null || { echo 'Unexpected Git index flags.' >&2; exit 1; }
  source_status="$(git status --porcelain --untracked-files=all)"
  ignored_sources="$(git ls-files --others --ignored --exclude-standard -- . ':(exclude).cache/**')"
  [[ -z "$source_status" && -z "$ignored_sources" ]] || { echo 'Package source is not clean.' >&2; exit 1; }
  export GOFLAGS='' GOWORK=off
fi

root="$PWD/.cache/github-ci"
if [[ -n "$package_commit" ]]; then
  for path in "$PWD/.cache" "$root" "$root/dist" "$root/logs"; do
    [[ ! -L "$path" ]] || { echo "Package output parent is a symlink: $path" >&2; exit 1; }
  done
  for path in "$root/macos" "$root/dist/macos" "$root/logs/macos-libraries.txt"; do
    [[ ! -e "$path" && ! -L "$path" ]] || { echo "Package output already exists: $path" >&2; exit 1; }
  done
fi
mkdir -p "$root/macos" "$root/dist/macos" "$root/logs"
go build -trimpath -o "$root/macos/aego-editor" ./cmd/aego-editor
go run ./tools/appbundle -bin "$root/macos/aego-editor" -out "$root/macos" \
  -version "$package_version" -icon app/brand/png/aego-256.png
app="$root/macos/Aego Editor.app"
plutil -lint "$app/Contents/Info.plist"
if [[ -n "$package_commit" ]]; then
  [[ "$(plutil -extract CFBundleShortVersionString raw -o - "$app/Contents/Info.plist")" == "$package_version" &&
     "$(plutil -extract CFBundleVersion raw -o - "$app/Contents/Info.plist")" == "$package_version" ]] || { echo 'Bundle version does not match the requested source version.' >&2; exit 1; }
fi
codesign --verify --deep --strict --verbose=2 "$app"
otool -L "$app/Contents/MacOS/aego-editor" | tee "$root/logs/macos-libraries.txt"
# SDL is linked statically; a non-system dependency would make the bundle non-portable.
if otool -L "$app/Contents/MacOS/aego-editor" | tail -n +2 | \
  grep -vE '^[[:space:]]*(/System/Library/|/usr/lib/)' ; then
  echo 'app bundle has a non-system dynamic dependency; investigate' >&2
  exit 1
fi
ditto -c -k --sequesterRsrc --keepParent "$app" "$root/dist/macos/Aego-Editor-macos.zip"
(cd "$root/dist/macos" && shasum -a 256 Aego-Editor-macos.zip > SHA256SUMS.txt)
if [[ -n "$package_commit" ]]; then
  printf 'version=%s\ncommit=%s\nstatus=artifact candidate; ad-hoc signed, not notarized\nupdate_key=not injected by this script\n' \
    "$package_version" "$package_commit" > "$root/dist/macos/CI-PROVENANCE.txt"
fi
