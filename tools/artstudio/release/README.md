# Asset Studio release integration

This builder packages the existing `tools/artstudio/wasm` and Go Art Core. It
does not replace the desktop editor, the existing Try runtime, or the normal
documentation build. A snapshot is not a production release.

Build and verify a new local artifact in a directory that does not yet exist:

```sh
make studio-web-build ARTSTUDIO_ID=local-01 ARTSTUDIO_OUT=/absolute/new/output
make studio-web-verify ARTSTUDIO_OUT=/absolute/new/output
go run ./tools/artstudio/release check-source -dir /absolute/new/output
go run ./tools/artstudio/release serve -dir /absolute/new/output
```

The artifact's app and docs origins must match the actual preview hosts. For
real documentation integration, build into a **fresh** site directory with the
same artifact and, preferably, the independently trusted manifest SHA:

```sh
ARTSTUDIO_ARTIFACT=/absolute/new/output \
ARTSTUDIO_MANIFEST_SHA256=<manifest-sha256> \
  go run ./cmd/aego docs build --out /absolute/new/site --engine "$PWD"
```

This validates the artifact, installs its localized guides and their small host
assets into the site, and links the exact `/r/<buildID>/` editor. No editor WASM
is copied into the docs site. Existing differing files and symlink destinations
are rejected, not overwritten. Without `ARTSTUDIO_ARTIFACT`, the existing local
`studio/` documentation build continues to work unchanged. The docs server's
CSP must explicitly permit the exact app origin in `frame-src`; a local test
server's policy does not establish the policy of a production deployment.

`make studio-web-test` runs the scoped tests. `make studio-web-ci` calls the
strict release gate and requires its real native, browser, documentation,
parity, full component Try and regression runners; missing runners are errors.
By default its parity runner replays `tools/artstudio/tests/parity.json` on
native Go and the served pinned WASM in Chromium and Firefox, checking every
frame and the document topology after each operation. It records the intentionally
different initial dirty state of a new web document. The runner requires the
Python Playwright environment and both browser executables; an alternative
real executable may be supplied with `AEGO_WASM_PARITY_RUNNER`. It does not
replace the separate native UI scenario or test physical input and OS save.
Passing a subset of tests does not authorize deployment. Production staging,
activation and rollback remain separate explicit operations after all gates pass.

For the same non-production snapshot checks used by the existing Gitea workflow,
install Python Playwright 1.64.1 and its Chromium and Firefox browsers, then run:

```sh
ARTSTUDIO_SNAPSHOT_DIR=/absolute/new/evidence/directory \
  bash tools/ci/artstudio_snapshot_e2e.sh
```

The evidence directory must not exist and must be outside the checkout. The
runner builds one pinned artifact and the actual documentation site, serves
both on loopback, and runs the browser and parity suites. Optional
`ARTSTUDIO_APP_PORT`, `ARTSTUDIO_DOCS_PORT`, `ARTSTUDIO_PYTHON`,
`ARTSTUDIO_CHROMIUM_EXECUTABLE` and `ARTSTUDIO_FIREFOX_EXECUTABLE` select the
local environment. It records tracked and non-ignored worktree inventories
before and after the run; this is not a Git commit or proof of ignored files.
The workflow uploads the snapshot and diagnostic evidence with a pinned
Gitea-compatible artifact action. The remote runner and artifact service must
still be checked by an actual CI run. This snapshot job never creates a release
gate report or deploys anything.
