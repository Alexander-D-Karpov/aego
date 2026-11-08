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
: "${AEGO_NATIVE_SCENARIO_RUNNER:?Existing executable for actual native UI/game scenario; API-only tests are insufficient}"
: "${AEGO_COMPONENT_TRY_RUNNER:?Executable for all registry components in native/web; must fail below 100 percent}"
: "${AEGO_DOCS_INTEGRATION_RUNNER:?Executable that builds/tests the existing Aego documentation with this pinned artifact}"
# The built-in runner compares the pinned WASM against native Go after every
# replay operation. A caller may supply another real runner, never a stub.
: "${AEGO_WASM_PARITY_RUNNER:=$root/tools/artstudio/release/tests/parity_e2e.py}"
for runner in "$AEGO_NATIVE_SCENARIO_RUNNER" "$AEGO_COMPONENT_TRY_RUNNER" "$AEGO_WASM_PARITY_RUNNER" "$AEGO_DOCS_INTEGRATION_RUNNER"; do
  [[ -f "$runner" && -x "$runner" ]] || { printf 'Required executable missing: %s\n' "$runner" >&2; exit 2; }
done
[[ ! -e "$ARTSTUDIO_EVIDENCE" ]] || { echo 'Evidence directory already exists' >&2; exit 2; }
mkdir -p -- "$ARTSTUDIO_EVIDENCE"
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
run_gate core bash -c 'go test -race -count=1 ./art ./tools/artstudio/host ./tools/artstudio/release && go vet ./art ./tools/artstudio/host ./tools/artstudio/release && node --test tools/artstudio/release/tests/*.test.cjs'
run_gate native-build bash tools/stage125-check.sh native
run_gate native-user-scenario "$AEGO_NATIVE_SCENARIO_RUNNER"
run_gate wasm-parity "$AEGO_WASM_PARITY_RUNNER"
# Build and verify the actual integrated documentation before browsers use it.
run_gate docs "$AEGO_DOCS_INTEGRATION_RUNNER"
for browser in chromium firefox; do
  executable=()
  if [[ "$browser" == chromium && -n "${ARTSTUDIO_CHROMIUM_EXECUTABLE:-}" ]]; then executable=(--executable "$ARTSTUDIO_CHROMIUM_EXECUTABLE"); fi
  run_gate "$browser" python3 tools/artstudio/release/tests/browser_e2e.py \
    --artifact "$ARTSTUDIO_ARTIFACT" --app-url "$ARTSTUDIO_APP_URL" --docs-url "$ARTSTUDIO_DOCS_URL" \
    --docs-prefix asset-studio --browser "$browser" --evidence "$ARTSTUDIO_EVIDENCE/$browser-browser" "${executable[@]}"
done
# Both browser runs exercise those docs; artifact bytes are checked separately.
run_gate http-bytes go run ./tools/artstudio/release check-http -dir "$ARTSTUDIO_ARTIFACT" -url "$ARTSTUDIO_APP_URL"
run_gate component-try "$AEGO_COMPONENT_TRY_RUNNER"
run_gate budgets python3 tools/artstudio/release/tests/budgets.py --artifact "$ARTSTUDIO_ARTIFACT"
run_gate regression make ci
run_gate source-check go run ./tools/artstudio/release check-source -dir "$ARTSTUDIO_ARTIFACT" -root "$root"
python3 tools/artstudio/release/tests/gate_report.py --artifact "$ARTSTUDIO_ARTIFACT" --evidence "$ARTSTUDIO_EVIDENCE"
printf '%s\n' 'Required gates passed for this exact artifact. Deployment remains an explicit, separate action.'
