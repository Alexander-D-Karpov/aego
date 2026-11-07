#!/usr/bin/env bash
# Explicitly separated checks: focused package tests are not Stage 12.5 acceptance.
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
mode="${1:-core}"
case "$mode" in core|native|try|ci) ;; *) printf 'Usage: %s [core|native|try|ci]\n' "$0" >&2; exit 2;; esac
export GOFLAGS="${GOFLAGS:+$GOFLAGS }-mod=readonly"
go version
if [[ "$mode" == try ]]; then
  : "${AEGO_TRY_SITE:?Set AEGO_TRY_SITE to a freshly built documentation site}"
  [[ -f "$AEGO_TRY_SITE/index.html" && -f "$AEGO_TRY_SITE/try/aego-docs.wasm" ]] || { printf '%s\n' 'Try site or actual docs WASM is missing' >&2; exit 1; }
  [[ "$(go env GOOS)" == linux && "$(go env CGO_ENABLED)" == 1 ]] || { printf '%s\n' 'Native OpenGL component gate needs Linux and CGO' >&2; exit 1; }
  go test -tags 'aego_editor aego_gltest' -list '^TestComponentContractsAcrossNativeAndChromium$' ./app/docsview | grep -x 'TestComponentContractsAcrossNativeAndChromium'
  AEGO_DEMOCHECK_ENFORCE=1 go test -tags 'aego_editor aego_gltest' -count=1 -run '^TestComponentContractsAcrossNativeAndChromium$' -v ./app/docsview
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
