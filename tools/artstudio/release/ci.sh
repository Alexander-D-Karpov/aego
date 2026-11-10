#!/usr/bin/env bash
# Called by the existing trusted Aego CI. Does not deploy or change releases.
# All gates are mandatory; absent native/registry runners are errors, not skips.
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$root"
: "${ARTSTUDIO_ARTIFACT:?Build one exact artifact first; provide its absolute directory}"
: "${ARTSTUDIO_EVIDENCE:?Use a new evidence directory outside the artifact}"
: "${ARTSTUDIO_APP_URL:?Exact pinned HTTP(S) artifact URL for browser tests}"
: "${ARTSTUDIO_DOCS_URL:?HTTP(S) URL serving the same build guide integration}"
# This gate can open real game scenes; never send their audio to a physical
# output. Browser processes additionally request their own muted audio modes.
export SDL_AUDIO_DRIVER=dummy SDL_AUDIODRIVER=dummy
export PYTHONDONTWRITEBYTECODE=1
: "${AEGO_NATIVE_SCENARIO_EVIDENCE:=$ARTSTUDIO_EVIDENCE/native-sdl}"
export AEGO_NATIVE_SCENARIO_EVIDENCE
native_runner=(bash "$root/tools/stage125-check.sh" artstudio-native)
docs_runner=(bash "$root/tools/stage125-check.sh" artstudio-docs)
if [[ -n "${AEGO_NATIVE_SCENARIO_RUNNER:-}" ]]; then native_runner=("$AEGO_NATIVE_SCENARIO_RUNNER"); fi
if [[ -n "${AEGO_DOCS_INTEGRATION_RUNNER:-}" ]]; then docs_runner=("$AEGO_DOCS_INTEGRATION_RUNNER"); fi
# The built-in runner compares the pinned WASM against native Go after every
# replay operation. A caller may supply another real runner, never a stub.
: "${AEGO_WASM_PARITY_RUNNER:=$root/tools/artstudio/release/tests/parity_e2e.py}"
for runner in "${AEGO_NATIVE_SCENARIO_RUNNER:-}" "${AEGO_COMPONENT_TRY_RUNNER:-}" "$AEGO_WASM_PARITY_RUNNER" "${AEGO_DOCS_INTEGRATION_RUNNER:-}"; do
  [[ -z "$runner" || ( -f "$runner" && -x "$runner" ) ]] || { printf 'Required executable missing: %s\n' "$runner" >&2; exit 2; }
done
# The artifact is immutable; evidence must not create files inside it. Resolve
# the existing parent, but allocate the final output directory exclusively.
ARTSTUDIO_ARTIFACT="$(cd -- "$ARTSTUDIO_ARTIFACT" && pwd -P)"
ARTSTUDIO_EVIDENCE="$(python3 -B - "$ARTSTUDIO_ARTIFACT" "$ARTSTUDIO_EVIDENCE" <<'PY'
from pathlib import Path
import sys
artifact = Path(sys.argv[1]).resolve(strict=True)
requested = Path(sys.argv[2]).absolute()
evidence = requested.parent.resolve(strict=True) / requested.name
if evidence == artifact or artifact in evidence.parents:
    raise SystemExit('Evidence must be outside the artifact')
print(evidence)
PY
)"
export ARTSTUDIO_ARTIFACT ARTSTUDIO_EVIDENCE
[[ ! -e "$ARTSTUDIO_EVIDENCE" && ! -L "$ARTSTUDIO_EVIDENCE" ]] || { echo 'Evidence directory already exists' >&2; exit 2; }
mkdir -- "$ARTSTUDIO_EVIDENCE"
run_gate() {
  local name="$1"; shift
  set +e
  "$@" >"$ARTSTUDIO_EVIDENCE/$name.log" 2>&1
  local status=$?
  set -e
  printf '%s\n' "$status" >"$ARTSTUDIO_EVIDENCE/$name.exit"
  cat "$ARTSTUDIO_EVIDENCE/$name.log"
  [[ "$status" == 0 ]]
}
run_gate core bash -c 'go test -race -count=1 ./art ./tools/artstudio/host ./tools/artstudio/gpupreview ./tools/artstudio/release && go vet ./art ./tools/artstudio/host ./tools/artstudio/gpupreview ./tools/artstudio/release && node --test tools/artstudio/release/tests/*.test.cjs'
run_gate native-build bash tools/stage125-check.sh native
run_gate native-user-scenario "${native_runner[@]}"
interop_executables=()
if [[ -n "${ARTSTUDIO_CHROMIUM_EXECUTABLE:-}" ]]; then interop_executables+=(--chromium-executable "$ARTSTUDIO_CHROMIUM_EXECUTABLE"); fi
if [[ -n "${ARTSTUDIO_FIREFOX_EXECUTABLE:-}" ]]; then interop_executables+=(--firefox-executable "$ARTSTUDIO_FIREFOX_EXECUTABLE"); fi
run_gate file-interoperability python3 tools/artstudio/release/tests/file_interop_e2e.py \
  --artifact "$ARTSTUDIO_ARTIFACT" --app-url "$ARTSTUDIO_APP_URL" \
  --native-evidence "$AEGO_NATIVE_SCENARIO_EVIDENCE" \
  --evidence "$ARTSTUDIO_EVIDENCE/file-interop" "${interop_executables[@]}"
run_gate remote-examples make remote-examples-check "REMOTE_EXAMPLES_OUT=$ARTSTUDIO_EVIDENCE/remote-protocol"
run_gate wasm-parity "$AEGO_WASM_PARITY_RUNNER"
# Build and verify the actual integrated documentation before browsers use it.
run_gate docs "${docs_runner[@]}"
wasm="$(python3 -B - "$ARTSTUDIO_ARTIFACT" <<'PY'
import json, sys
from pathlib import Path
root = Path(sys.argv[1]).resolve(strict=True)
path = Path(json.loads((root / 'manifest.json').read_text())['wasm']['path'])
if path.is_absolute() or '..' in path.parts:
    raise SystemExit('invalid artifact WASM path')
wasm = (root / path).resolve(strict=True)
if root not in wasm.parents or not wasm.is_file():
    raise SystemExit('artifact WASM is not a file inside the artifact')
print(wasm)
PY
)"
for browser in chromium firefox; do
  executable=()
  if [[ "$browser" == chromium && -n "${ARTSTUDIO_CHROMIUM_EXECUTABLE:-}" ]]; then executable=(--executable "$ARTSTUDIO_CHROMIUM_EXECUTABLE"); fi
  if [[ "$browser" == firefox && -n "${ARTSTUDIO_FIREFOX_EXECUTABLE:-}" ]]; then executable=(--executable "$ARTSTUDIO_FIREFOX_EXECUTABLE"); fi
  run_gate "$browser" python3 tools/artstudio/release/tests/browser_e2e.py \
    --artifact "$ARTSTUDIO_ARTIFACT" --app-url "$ARTSTUDIO_APP_URL" --docs-url "$ARTSTUDIO_DOCS_URL" \
    --docs-prefix asset-studio --browser "$browser" --evidence "$ARTSTUDIO_EVIDENCE/$browser-browser" "${executable[@]}"
  file_args=()
  if [[ "$browser" == chromium && -n "${ARTSTUDIO_CHROMIUM_EXECUTABLE:-}" ]]; then file_args=(--chromium "$ARTSTUDIO_CHROMIUM_EXECUTABLE"); fi
  if [[ "$browser" == firefox && -n "${ARTSTUDIO_FIREFOX_EXECUTABLE:-}" ]]; then file_args=(--firefox "$ARTSTUDIO_FIREFOX_EXECUTABLE"); fi
  run_gate "$browser-workspace-files" python3 -B tools/artstudio/tests/workspace_files_browser.py \
    --wasm "$wasm" --engine "$browser" "${file_args[@]}"
  run_gate "$browser-pending-fields" python3 -B tools/artstudio/tests/pending_fields_browser.py \
    --wasm "$wasm" --engine "$browser" "${file_args[@]}"
  run_gate "$browser-gpu" env ARTSTUDIO_GPU_BROWSER="$browser" bash tools/stage125-check.sh artstudio-gpu \
    --artifact "$ARTSTUDIO_ARTIFACT" --app-url "$ARTSTUDIO_APP_URL" \
    --evidence "$ARTSTUDIO_EVIDENCE/$browser-gpu-browser" "${executable[@]}"
done
# Both browser runs exercise those docs; artifact bytes are checked separately.
run_gate http-bytes go run ./tools/artstudio/release check-http -dir "$ARTSTUDIO_ARTIFACT" -url "$ARTSTUDIO_APP_URL"
run_gate budgets python3 tools/artstudio/release/tests/budgets.py --artifact "$ARTSTUDIO_ARTIFACT"
# make ci already executes the complete registry once. Preserve its evidence in
# this fresh release directory, then independently verify it on the same tree.
: "${COMPONENT_TRY_SITE:=$root/site}"
: "${COMPONENT_TRY_REPORT:=$ARTSTUDIO_EVIDENCE/component-try-report.json}"
: "${COMPONENT_TRY_EVIDENCE:=$ARTSTUDIO_EVIDENCE/component-try-observations}"
export COMPONENT_TRY_SITE COMPONENT_TRY_REPORT COMPONENT_TRY_EVIDENCE
run_gate regression make ci
if [[ -z "${AEGO_COMPONENT_TRY_RUNNER:-}" ]]; then
  run_gate component-try python3 -B tools/ci/component_try_evidence.py verify --root "$root" \
    --report "$COMPONENT_TRY_REPORT" --evidence "$COMPONENT_TRY_EVIDENCE" \
    --wasm "$COMPONENT_TRY_SITE/try/aego-docs.wasm" --output "$ARTSTUDIO_EVIDENCE/component-try-verified.json"
else
  run_gate component-try "$AEGO_COMPONENT_TRY_RUNNER"
fi
run_gate source-check go run ./tools/artstudio/release check-source -dir "$ARTSTUDIO_ARTIFACT" -root "$root"
python3 tools/artstudio/release/tests/gate_report.py --artifact "$ARTSTUDIO_ARTIFACT" --evidence "$ARTSTUDIO_EVIDENCE"
printf '%s\n' 'Required gates passed for this exact artifact. Deployment remains an explicit, separate action.'
