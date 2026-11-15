#!/usr/bin/env bash
# Focused gates for this authoring slice. Does not claim to replace Aego's CI.
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"
go test -race -count=1 ./art ./tools/artstudio ./tools/artstudio/host
go vet ./art ./tools/artstudio ./tools/artstudio/host
bash tools/artstudio/build.sh
go build -o .cache/artstudio-host ./tools/artstudio
printf '%s\n' 'Focused Go / WASM gates passed. Browser and full engine CI still need separate execution.'
