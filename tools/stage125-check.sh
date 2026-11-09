#!/usr/bin/env bash
# Explicitly separated checks: focused package tests are not Stage 12.5 acceptance.
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
mode="${1:-core}"
case "$mode" in core|native|try|ci|component-try|artstudio-native|artstudio-gpu|artstudio-docs) ;; *) printf 'Usage: %s [core|native|try|ci|component-try|artstudio-native|artstudio-gpu|artstudio-docs]\n' "$0" >&2; exit 2;; esac
if (($#)); then shift; fi
export GOFLAGS="${GOFLAGS:+$GOFLAGS }-mod=readonly"
export SDL_AUDIO_DRIVER=dummy SDL_AUDIODRIVER=dummy

# Reusable acceptance runners live here rather than in untracked shell files.
# Their evidence directories must be fresh; run these only in a protected tree.
if [[ "$mode" == component-try ]]; then
  (($# == 0)) || { echo 'component-try does not accept extra arguments' >&2; exit 2; }
  report="${AEGO_DEMOCHECK_REPORT:-$root/.cache/component-try.json}"
  evidence="${AEGO_DEMOCHECK_EVIDENCE:-$root/.cache/component-try-evidence}"
  for target in "$report" "$report.exit" "$report.test.exit" "$report.log" "$evidence" "$evidence-negative"; do
    [[ ! -e "$target" && ! -L "$target" ]] || { printf 'Use a fresh evidence path: %s\n' "$target" >&2; exit 2; }
  done
  mkdir -p -- "$(dirname -- "$report")" "$(dirname -- "$evidence")"
  mkdir -- "$evidence"
  (
    set -C
    : > "$report"
    printf '%s\n' RUNNING > "$report.exit"
    printf '%s\n' RUNNING > "$report.test.exit"
    : > "$report.log"
  )
  trap 'status=$?; printf "%s\n" "$status" > "$report.exit"' EXIT
  snapshot=(python3 -B "$root/tools/ci/component_try_evidence.py" snapshot --root "$root")
  if [[ "${AEGO_COMPONENT_TRY_ALLOW_UNBORN:-0}" == 1 ]]; then snapshot+=(--allow-unborn); fi
  "${snapshot[@]}" --output "$evidence/source-before.json"
  export AEGO_DEMOCHECK_REPORT="$report" AEGO_DEMOCHECK_EVIDENCE="$evidence" AEGO_DEMOCHECK_ENFORCE=1
  export SDL_VIDEO_DRIVER="${SDL_VIDEODRIVER:-offscreen}"
  set +e
  go test -tags aego_editor,aego_gltest -count=1 -timeout=45m -run '^TestComponentContractsAcrossNativeAndChromium$' -v ./app/docsview 2>&1 | tee "$report.log"
  code=$?
  set -e
  printf '%s\n' "$code" > "$report.test.exit"
  "${snapshot[@]}" --output "$evidence/source-after.json"
  if ((code != 0)); then exit "$code"; fi
  verify=(python3 -B "$root/tools/ci/component_try_evidence.py" verify --during-gate --root "$root" --report "$report" --evidence "$evidence" --wasm "${AEGO_TRY_SITE:?Set AEGO_TRY_SITE to the tested site}/try/aego-docs.wasm" --output "$evidence/verified.json")
  if [[ "${AEGO_COMPONENT_TRY_ALLOW_UNBORN:-0}" == 1 ]]; then verify+=(--allow-unborn); fi
  "${verify[@]}"
  exit 0
fi

if [[ "$mode" == artstudio-native ]]; then
  (($# == 0)) || { echo 'artstudio-native does not accept extra arguments' >&2; exit 2; }
  [[ -z "${AEGO_ART_INTEROP_RETURN:-}" ]] || { echo 'The producer must create its own source' >&2; exit 2; }
  [[ "$(go env GOOS)" == linux && "$(go env CGO_ENABLED)" == 1 ]] || { echo 'Requires Linux and CGO' >&2; exit 2; }
  if [[ -n "${AEGO_NATIVE_SCENARIO_EVIDENCE:-}" ]]; then
    evidence="$AEGO_NATIVE_SCENARIO_EVIDENCE"
  elif [[ -n "${ARTSTUDIO_EVIDENCE:-}" ]]; then
    evidence="$ARTSTUDIO_EVIDENCE/native-sdl"
  else
    mkdir -p -- "$root/.cache/stage125"
    evidence="$(mktemp -d "$root/.cache/stage125/native-sdl.XXXXXXXX")"
  fi
  if [[ ! -d "$evidence" ]]; then
    [[ ! -e "$evidence" && ! -L "$evidence" ]] || { echo 'Evidence path already exists' >&2; exit 2; }
    mkdir -- "$evidence"
  elif [[ -n "${AEGO_NATIVE_SCENARIO_EVIDENCE:-}${ARTSTUDIO_EVIDENCE:-}" ]]; then
    echo 'Use a new native evidence directory' >&2; exit 2
  fi
  evidence="$(cd -- "$evidence" && pwd -P)"
  printf '%s\n' RUNNING > "$evidence/exit"
  trap 'status=$?; printf "%s\n" "$status" > "$evidence/exit"' EXIT
  export SDL_VIDEODRIVER="${SDL_VIDEODRIVER:-offscreen}"
  export SDL_VIDEO_DRIVER="$SDL_VIDEODRIVER"
  export LIBGL_ALWAYS_SOFTWARE="${LIBGL_ALWAYS_SOFTWARE:-1}" GALLIUM_DRIVER="${GALLIUM_DRIVER:-llvmpipe}"
  export AEGO_PLATFORMER_ART_SHOTS="$evidence/player" AEGO_ART_WORKFLOW_SHOTS="$evidence/authoring"
  export AEGO_ART_SHADOW_SHOTS="$evidence/shadow" AEGO_VFX_SHOTS="$evidence/particle"
  export AEGO_VFX_WORKSPACE_SHOTS="$evidence/vfx" AEGO_ART_EXTENDED_SHOTS="$evidence/tools"
  export AEGO_ART_WORKSPACE_SHOTS="$evidence/workspaces" AEGO_ART_SURFACE_SHOTS="$evidence/surface"
  export AEGO_ASE_IMPORT_SHOTS="$evidence/aseprite" AEGO_ART_AGENT_SHOTS="$evidence/agent"
  export AEGO_ART_INTEROP_OUT="$evidence/interoperability" AEGO_EFFECT_TEMPLATE_SHOTS="$evidence/effect-templates"
  export AEGO_ART_PROMPT_SHOTS="$evidence/source-prompt" AEGO_WELCOME_LOCALE_SHOTS="$evidence/welcome-locale"
  export AEGO_ART_CONTROLS_SHOTS="$evidence/authoring-controls" AEGO_REMOTE_LAUNCH_SHOTS="$evidence/remote-launcher"
  snapshot=(python3 -B "$root/tools/ci/component_try_evidence.py" snapshot --root "$root")
  if [[ "${AEGO_NATIVE_ALLOW_UNBORN:-0}" == 1 ]]; then snapshot+=(--allow-unborn); fi
  "${snapshot[@]}" --output "$evidence/source-before.json"
  {
    go version
    go env GOOS GOARCH CGO_ENABLED
    printf 'SDL_VIDEODRIVER=%s\nSDL_VIDEO_DRIVER=%s\nSDL_AUDIO_DRIVER=%s\nLIBGL_ALWAYS_SOFTWARE=%s\nGALLIUM_DRIVER=%s\n' "$SDL_VIDEODRIVER" "$SDL_VIDEO_DRIVER" "$SDL_AUDIO_DRIVER" "$LIBGL_ALWAYS_SOFTWARE" "$GALLIUM_DRIVER"
    cat third_party/sdl/version.txt
  } > "$evidence/environment.txt"
  tests=(
    TestCreatePixelArtNativeEnterWaitsForConfirmation
    TestInitializeBlankTextArtInNativeEditor
    TestAsepriteImportNativeEditor
    TestPlatformerEditSourceArtNative
    TestLinkedSourcePromptNativeLayout
    TestWelcomeLocaleNativeStartup
    TestArtSceneAuthoringWorkflowNative
    TestArtWorkingAlphaShadowNative
    TestAuthoredFlipbookParticleGameViewNative
    TestVFXCanvasAndParticlePresetNative
    TestArtEffectTemplateNative
    TestArtExtendedToolsNative
    TestArtFillPixelPerfectNative
    TestArtColorModesRemapNative
    TestArtSelectionTransformNative
    TestArtOnionTagsDurationNative
    TestArtWorkspaceContinuityNative
    TestArtSurfaceEmissionBloomNative
    TestArtAgentNativeAssignGameViewUndo
    TestImagePreviewLitPixelsAndLifetime
    TestLinkedPlatformerArtExportDesktopAndWeb
    TestRemoteLauncherClonesOverLocalTLS
    TestRemoteLauncherNativeConfirmation
    TestRemoteProtocolLauncherNativeConfirmation
  )
  printf '%s\n' "${tests[@]}" > "$evidence/required-tests.txt"
  selector="^($(IFS='|'; printf '%s' "${tests[*]}"))$"
  command=(go test -p 1 -json -count=1 -tags aego_gltest -run "$selector" ./app/edit ./modules/render2d/editor ./app/export)
  printf '%q ' "${command[@]}" > "$evidence/command.txt"
  printf '\n' >> "$evidence/command.txt"
  set +e
  "${command[@]}" 2>&1 | tee "$evidence/events.jsonl"
  test_status=$?
  set -e
  printf '%s\n' "$test_status" > "$evidence/tests.exit"
  "${snapshot[@]}" --output "$evidence/source-after.json"
  python3 -B - "$evidence" <<'PY'
import json
from pathlib import Path
import sys

path = Path(sys.argv[1])
required = path.joinpath('required-tests.txt').read_text().splitlines()
results, output, skipped = {}, [], []
for line in path.joinpath('events.jsonl').read_text().splitlines():
    try:
        event = json.loads(line)
    except json.JSONDecodeError:
        output.append(line + '\n')
        continue
    if event.get('Output'):
        output.append(event['Output'])
    name, action = event.get('Test', ''), event.get('Action')
    if name and action in ('pass', 'fail', 'skip'):
        results.setdefault(name, []).append(action)
        if action == 'skip':
            skipped.append(name)
before = json.loads(path.joinpath('source-before.json').read_text())
after = json.loads(path.joinpath('source-after.json').read_text())
same_source = before == after
missing = [name for name in required if results.get(name) != ['pass']]
summary = {
    'schema': 1,
    'runner': 'Linux SDL / GL33 with synthetic input; see environment.txt for driver and audio',
    'sourceFingerprint': before['worktreeSHA256'], 'head': before.get('head'),
    'sourceUnchanged': same_source,
    'required': {name: results.get(name, ['NOT_RUN'])[0].upper() for name in required},
    'skipped': skipped, 'testExit': int(path.joinpath('tests.exit').read_text()),
}
summary['passed'] = not missing and not skipped and same_source and summary['testExit'] == 0
path.joinpath('native.log').write_text(''.join(output))
path.joinpath('summary.json').write_text(json.dumps(summary, indent=2) + '\n')
print(json.dumps(summary, indent=2))
if not summary['passed']:
    raise SystemExit(1)
PY
  exit "$test_status"
fi

if [[ "$mode" == artstudio-gpu ]]; then
  python="${ARTSTUDIO_PYTHON:-python3}"
  browser="${ARTSTUDIO_GPU_BROWSER:?Set chromium or firefox}"
  case "$browser" in chromium|firefox) ;; *) echo 'Invalid GPU browser' >&2; exit 2;; esac
  command=("$python" "$root/tools/artstudio/release/tests/gpu_e2e.py" --browser "$browser" "$@")
  if [[ "$browser" == firefox && "$(uname -s)" == Linux ]]; then
    command+=(--headed)
    if [[ -z "${DISPLAY:-}" ]]; then
      command -v xvfb-run >/dev/null || { echo 'Firefox GPU acceptance requires DISPLAY or xvfb-run' >&2; exit 2; }
      exec xvfb-run -a -s '-screen 0 1440x1050x24' "${command[@]}"
    fi
  fi
  exec "${command[@]}"
fi

if [[ "$mode" == artstudio-docs ]]; then
  (($# == 0)) || { echo 'artstudio-docs does not accept extra arguments' >&2; exit 2; }
  : "${ARTSTUDIO_ARTIFACT:?Provide the tested standalone artifact}"
  : "${ARTSTUDIO_EVIDENCE:?Provide the release evidence directory}"
  site="${ARTSTUDIO_DOCS_SITE:-$ARTSTUDIO_EVIDENCE/docs-site}"
  builder="$ARTSTUDIO_EVIDENCE/docs-builder"
  for target in "$site" "$builder"; do
    [[ ! -e "$target" && ! -L "$target" ]] || { printf 'Output already exists: %s\n' "$target" >&2; exit 2; }
  done
  mkdir -- "$builder"
  builder="$(cd -- "$builder" && pwd -P)"
  manifest_sha="$(python3 - "$ARTSTUDIO_ARTIFACT/manifest.json" <<'PY'
import hashlib, sys
from pathlib import Path
print(hashlib.sha256(Path(sys.argv[1]).read_bytes()).hexdigest())
PY
)"
  go run ./tools/artstudio/release verify -dir "$ARTSTUDIO_ARTIFACT" -sha256 "$manifest_sha"
  go build -o "$builder/aego" ./cmd/aego
  "$builder/aego" docs check --strict --lang ru
  extra=()
  if [[ -n "${AEGO_EXAMPLE_LICENSE:-}" ]]; then extra+=(--examples-license "$AEGO_EXAMPLE_LICENSE"); fi
  if [[ ${AEGO_EXAMPLES_URL+x} ]]; then extra+=(--examples-url "$AEGO_EXAMPLES_URL"); fi
  if [[ -n "${AEGO_EXAMPLES_VERSION:-}" ]]; then extra+=(--examples-version "$AEGO_EXAMPLES_VERSION"); fi
  if python3 - "$ARTSTUDIO_ARTIFACT/manifest.json" <<'PY'
import json, sys
from pathlib import Path
raise SystemExit(0 if json.loads(Path(sys.argv[1]).read_text())['release'] else 1)
PY
  then
    : "${AEGO_EXAMPLE_LICENSE:?Production examples require their reviewed source license}"
    extra+=(--examples-production)
  fi
  ARTSTUDIO_MANIFEST_SHA256="$manifest_sha" "$builder/aego" docs build --out "$site" --engine "$root" "${extra[@]}"
  "$builder/aego" docs export --out "$builder/docs-registry.json"
  (
    cd tools/gendocs
    AEGO_DOCS_EXPORT="$builder/docs-registry.json" go test -count=1 ./...
  )
  go test -count=1 ./app/docstool
  go run ./tools/artstudio/release integrate-docs -dir "$ARTSTUDIO_ARTIFACT" -out "$site" -sha256 "$manifest_sha"
  printf 'Built actual documentation for browser verification at %s: %s\n' "${ARTSTUDIO_DOCS_URL:-the configured docs URL}" "$site"
  exit 0
fi

go version
if [[ "$mode" == try ]]; then
  : "${AEGO_TRY_SITE:?Set AEGO_TRY_SITE to a freshly built documentation site}"
  [[ -f "$AEGO_TRY_SITE/index.html" && -f "$AEGO_TRY_SITE/try/aego-docs.wasm" ]] || { printf '%s\n' 'Try site or actual docs WASM is missing' >&2; exit 1; }
  [[ "$(go env GOOS)" == linux && "$(go env CGO_ENABLED)" == 1 ]] || { printf '%s\n' 'Native OpenGL component gate needs Linux and CGO' >&2; exit 1; }
  go test -tags 'aego_editor aego_gltest' -list '^TestComponentContractsAcrossNativeAndChromium$' ./app/docsview | grep -x 'TestComponentContractsAcrossNativeAndChromium'
  AEGO_DEMOCHECK_ENFORCE=1 go test -timeout 50m -tags 'aego_editor aego_gltest' -count=1 -run '^TestComponentContractsAcrossNativeAndChromium$' -v ./app/docsview
  exit 0
fi
go test -race -count=1 ./art ./project/remoteexample ./tools/remoteexample ./tools/artstudio ./tools/artstudio/host ./engine/docs/democheck
go vet ./art ./project/remoteexample ./tools/remoteexample ./tools/artstudio ./tools/artstudio/host ./engine/docs/democheck
bash tools/artstudio/build.sh
if [[ "$mode" == core ]]; then
  printf '%s\n' 'Core/transport/contract tests and WASM build passed. Native, browser and component acceptance NOT covered by this command.'
  exit 0
fi
for file in app/importers.go editor/core/document/document.go engine/docs/registry.go; do
  [[ -f "$file" ]] || { printf 'Full engine source missing: %s\n' "$file" >&2; exit 1; }
done
go test -race -count=1 ./editor/core/artedit ./editor/core/context ./asset/pipeline ./editor/core/document ./editor/core/history ./editor/core/workspace ./asset/importers/artsource ./app/agent ./app/edit/agentbridge ./engine/docs ./engine/i18n
go test -count=1 ./modules/render2d/editor ./editor/ext ./editor/shell
go test -count=1 -run '^TestArt(SceneAuthoringWorkflowHeadless|AgentUsesLiveDocumentApprovalAndHistory)$' ./app/edit
go vet ./editor/core/artedit ./editor/core/context ./asset/pipeline ./asset/importers/artsource ./app/agent ./app/edit ./app/edit/agentbridge ./modules/render2d/editor ./editor/shell ./engine/docs
if [[ "$mode" == ci ]]; then
  make ci
  "$0" try
fi
printf '%s\n' 'Requested code checks completed. AS01-AS28, actual browser/UI scenarios and real all-component DemoContract probes still require their own evidence.'
