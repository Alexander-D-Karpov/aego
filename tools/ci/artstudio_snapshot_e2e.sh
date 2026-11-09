#!/usr/bin/env bash
# Actual HTTP/browser checks on one snapshot from the current tree. This is a
# PR/development check, not the strict release gate or production deployment.
# ARTSTUDIO_SNAPSHOT_DIR must name a new evidence directory outside the repo.
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)"
cd "$root"
: "${ARTSTUDIO_SNAPSHOT_DIR:?Set an absolute, new evidence directory outside the repository}"
python="${ARTSTUDIO_PYTHON:-python3}"
app_port="${ARTSTUDIO_APP_PORT:-8097}"
docs_port="${ARTSTUDIO_DOCS_PORT:-8098}"
for port in "$app_port" "$docs_port"; do
  [[ "$port" =~ ^[0-9]{4,5}$ ]] && (( 10#$port >= 1024 && 10#$port <= 65535 )) || {
    printf 'Invalid unprivileged port: %s\n' "$port" >&2; exit 2;
  }
done
[[ "$app_port" != "$docs_port" ]] || { echo 'App and docs require different ports' >&2; exit 2; }
command -v "$python" >/dev/null
"$python" -c 'import playwright.sync_api'

# Create only the explicitly selected new directory; never clean an existing
# directory or place generated artifacts in the user's checkout.
dir="$("$python" - "$root" "$ARTSTUDIO_SNAPSHOT_DIR" <<'PY'
import os, sys
from pathlib import Path
root = Path(sys.argv[1]).resolve(strict=True)
requested = Path(sys.argv[2])
if not requested.is_absolute() or requested.name in ('', '.', '..'):
    raise SystemExit('evidence directory must be a new absolute path')
parent = requested.parent.resolve(strict=True)
target = parent / requested.name
if target == root or root in target.parents:
    raise SystemExit('evidence directory must be outside the repository')
os.mkdir(target, 0o700)
print(target)
PY
)"
artifact="$dir/artifact"
site="$dir/site"
app_origin="http://127.0.0.1:$app_port"
docs_origin="http://127.0.0.1:$docs_port"
build_id="ci-snapshot-$(git rev-parse --short=12 HEAD)-$$"
app_pid=''
docs_pid=''
cleanup() {
  for pid in "$app_pid" "$docs_pid"; do
    if [[ -n "$pid" ]]; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
}
trap cleanup EXIT
run_gate() {
  local name="$1"; shift
  local code
  set +e
  "$@" >"$dir/$name.log" 2>&1
  code=$?
  set -e
  printf '%s\n' "$code" >"$dir/$name.exit"
  printf '%s: exit %s (%s)\n' "$name" "$code" "$dir/$name.log"
  if [[ "$code" != 0 ]]; then cat "$dir/$name.log"; fi
  return "$code"
}

git rev-parse HEAD >"$dir/head.txt"
git status --short --branch >"$dir/worktree-status.txt"
go version >"$dir/go-version.txt"
"$python" -m playwright --version >"$dir/playwright-version.txt"
run_gate worktree-before "$python" tools/ci/worktree_manifest.py --root "$root" --output "$dir/worktree-before.json"

run_gate core make studio-web-test
run_gate build-tool go build -o "$dir/artstudio-release" ./tools/artstudio/release
run_gate build-snapshot "$dir/artstudio-release" build -root "$root" -out "$artifact" \
  -id "$build_id" -app-origin "$app_origin" -docs-origin "$docs_origin"
manifest_sha="$(sha256sum "$artifact/manifest.json" | cut -d' ' -f1)"
printf '%s\n' "$manifest_sha" >"$dir/manifest.sha256"
run_gate verify "$dir/artstudio-release" verify -dir "$artifact" -sha256 "$manifest_sha"
run_gate check-source "$dir/artstudio-release" check-source -root "$root" -dir "$artifact"
run_gate build-cli go build -o "$dir/aego" ./cmd/aego
run_gate docs env ARTSTUDIO_ARTIFACT="$artifact" ARTSTUDIO_MANIFEST_SHA256="$manifest_sha" \
  "$dir/aego" docs build --out "$site" --engine "$root"

"$dir/artstudio-release" serve -dir "$artifact" -addr "127.0.0.1:$app_port" -docs-addr= >"$dir/app-server.log" 2>&1 &
app_pid=$!
"$python" tools/ci/artstudio_docs_server.py --site "$site" --port "$docs_port" \
  --app-origin "$app_origin" >"$dir/docs-server.log" 2>&1 &
docs_pid=$!

run_gate server-ready "$python" - "$artifact/manifest.json" "$app_origin" "$docs_origin" <<'PY'
import sys, time, urllib.request
from pathlib import Path
expected = Path(sys.argv[1]).read_bytes()
for attempt in range(60):
    try:
        with urllib.request.urlopen(sys.argv[2] + '/manifest.json', timeout=2) as r:
            if r.read() != expected:
                raise RuntimeError('served app is not the newly built artifact')
        with urllib.request.urlopen(sys.argv[3] + '/asset-studio/ru/first-sprite/', timeout=2) as r:
            if 'frame-src ' + sys.argv[2] not in r.headers.get('Content-Security-Policy', ''):
                raise RuntimeError('docs CSP does not pin the app origin')
        print('Both loopback servers are ready with the expected artifact and CSP')
        break
    except (OSError, RuntimeError):
        if attempt == 59:
            raise
        time.sleep(0.5)
PY

run_gate http "$dir/artstudio-release" check-http -dir "$artifact" -url "$app_origin/" -sha256 "$manifest_sha"
run_gate http-versioned "$dir/artstudio-release" check-http -dir "$artifact" -url "$app_origin/r/$build_id/" -sha256 "$manifest_sha"
run_gate budgets "$python" tools/artstudio/release/tests/budgets.py --artifact "$artifact"
chromium_args=()
firefox_args=()
if [[ -n "${ARTSTUDIO_CHROMIUM_EXECUTABLE:-}" ]]; then chromium_args=(--executable "$ARTSTUDIO_CHROMIUM_EXECUTABLE"); fi
if [[ -n "${ARTSTUDIO_FIREFOX_EXECUTABLE:-}" ]]; then firefox_args=(--executable "$ARTSTUDIO_FIREFOX_EXECUTABLE"); fi
for browser in chromium firefox; do
  args=()
  if [[ "$browser" == chromium ]]; then args=("${chromium_args[@]}"); else args=("${firefox_args[@]}"); fi
  run_gate "$browser" env -u GOMAXPROCS "$python" tools/artstudio/release/tests/browser_e2e.py \
    --artifact "$artifact" --app-url "$app_origin/" --docs-url "$docs_origin/" \
    --docs-prefix asset-studio --browser "$browser" --evidence "$dir/$browser-browser" "${args[@]}"
  run_gate "$browser-gpu" env -u GOMAXPROCS ARTSTUDIO_PYTHON="$python" ARTSTUDIO_GPU_BROWSER="$browser" \
    bash tools/stage125-check.sh artstudio-gpu --artifact "$artifact" --app-url "$app_origin/" \
    --evidence "$dir/$browser-gpu-browser" "${args[@]}"
done
run_gate parity env -u GOMAXPROCS ARTSTUDIO_ARTIFACT="$artifact" ARTSTUDIO_APP_URL="$app_origin/" \
  ARTSTUDIO_EVIDENCE="$dir" "$python" tools/artstudio/release/tests/parity_e2e.py
run_gate final-source-check "$dir/artstudio-release" check-source -root "$root" -dir "$artifact"
run_gate final-verify "$dir/artstudio-release" verify -dir "$artifact" -sha256 "$manifest_sha"
run_gate worktree-after "$python" tools/ci/worktree_manifest.py --root "$root" --output "$dir/worktree-after.json"
run_gate worktree-unchanged "$python" - "$dir/worktree-before.json" "$dir/worktree-after.json" <<'PY'
import json, sys
with open(sys.argv[1], encoding='utf-8') as f:
    before = json.load(f)
with open(sys.argv[2], encoding='utf-8') as f:
    after = json.load(f)
if before['head'] != after['head'] or before['worktreeSHA256'] != after['worktreeSHA256']:
    raise SystemExit('worktree changed during snapshot checks; results do not identify one tree')
print('Identical tracked/non-ignored worktree inventories before and after:', before['worktreeSHA256'])
PY
printf 'Snapshot browser/docs checks passed; evidence: %s\n' "$dir"
printf '%s\n' 'This is not the full Stage 12.5 gate, a production release, or deployment authorization.'
