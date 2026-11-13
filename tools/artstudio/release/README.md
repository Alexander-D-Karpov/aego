# Asset Studio release integration

This builder packages the existing `tools/artstudio/wasm` and Go Art Core. It
does not replace the desktop editor, the existing Try runtime, or the normal
documentation build. A snapshot is not a production release.

The manifest records two distinct engine versions. `aegoVersion` is the compatible
release base (currently `0.12.1`); `engineSourceVersion` is read from the exact
packaged `engine/config.go` (currently `0.12.5-dev`). Older manifests without the
additive source-version field remain valid for rollback. The eight effect
starters are listed separately as `templates`; the twelve tutorial `examples`
remain unchanged. Both native and web creation use the shared Art Core template
catalog, and GPU previews use the corresponding existing particle preset.


The native entry point uses that same `art` package and document operations:

```sh
go build -o .cache/bin/aego ./cmd/aego
.cache/bin/aego edit examples/platformer
```

Select Player and choose SpriteRenderer → Edit Sprite. For its original PNG,
confirm the linked editable source before painting. The native dependency and
SDL setup is the normal Aego build setup; this command does not install it.

Build and verify a new local artifact in a directory that does not yet exist:

```sh
make studio-web-build ARTSTUDIO_ID=local-01 ARTSTUDIO_OUT=/absolute/new/output
make studio-web-verify ARTSTUDIO_OUT=/absolute/new/output
go run ./tools/artstudio/release check-source -dir /absolute/new/output
go run ./tools/artstudio/release serve -dir /absolute/new/output
```

The server prints the standalone and documentation preview URLs. It is a
development static server; the distributable directory needs no Go server in
production. `/editor/`, `/embed/` and `/examples/<id>/` use the same resources.
Release builds additionally require a clean Git checkout without ignored or
untracked files inside the Studio source inputs, hidden Git index entries, or an
active Go workspace. The builder and `check-source` compare those input bytes
with the pinned commit; ordinary snapshot builds retain their working-tree
behavior. These checks are not atomic or a substitute for an isolated checkout
and controlled CI environment. Ignored outputs outside the inputs are allowed.
The core WASM is limited to 8 MiB raw / 3 MiB gzip. GPU Preview is loaded only
when selected, from a separate 32 MiB raw / 10 MiB gzip budget in the same
manifest. It uses the existing renderer, materials, alpha shadows, Bloom and
CPU ParticleSystem2D in an isolated world. It excludes desktop editor, pipeline
importers and unused game modules. It is not linked into the regular Demo,
Playground or component Try entry points.

Height mode visualizes the data channel; it does not implement parallax.
Particle flipbooks use the runtime's uniform FrameFPS derived from average
duration; ordinary Art animation respects each frame's duration. Neither web
preview nor export assigns assets to a local game project that is not open.

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
CSP must explicitly permit the exact app origin and `blob:` in `frame-src`:
Firefox applies that directive to downloads from a sandboxed child iframe.
The iframe allows local dialog forms while `form-action 'none'` still prevents
network form submissions. Do not add wildcard origins or relax script rules.
A local test
server's policy does not establish the policy of a production deployment.

`make studio-web-test` runs the scoped tests. `make studio-web-ci` calls the
strict release gate and requires its real native, browser, documentation,
parity, full component Try and regression runners; missing runners are errors.
The supplied defaults are `tools/stage125-check.sh artstudio-native` (synthetic
SDL input through the real editor and GL/game scenarios) and
`tools/stage125-check.sh artstudio-docs` (the ordinary registry/site builder
with the pinned artifact). The latter creates a new
`$ARTSTUDIO_EVIDENCE/docs-site`, or the explicit `ARTSTUDIO_DOCS_SITE`; arrange
for the CI docs host at `ARTSTUDIO_DOCS_URL` to serve that directory. Production
example packages additionally require `AEGO_EXAMPLE_LICENSE` and
`AEGO_EXAMPLE_LICENSE_FILE` with the reviewed source license and its full terms;
dependency licenses do not establish the examples' license.
For a staging or alternate HTTPS host, set `AEGO_EXAMPLES_URL` to its exact
documentation root ending in `/`; `AEGO_EXAMPLES_VERSION` selects the catalog
version. An explicitly empty URL disables packages only for non-production
builds. Leaving the variable unset retains gendocs' canonical default. The
address and catalog contents still require independent deployment checks.
The release and primary Gitea gates also run `make remote-examples-check`:
Linux GIO dispatches the installed protocol handler in an isolated XDG profile
into the actual editor confirmation window. This does not establish Windows
or macOS protocol registration; those remain separate platform checks.
By default its parity runner replays `tools/artstudio/tests/parity.json` on
native Go and the served pinned WASM in Chromium and Firefox, checking every
frame's albedo, normal, height and emission channels and the document topology
after each operation. It records the intentionally
different initial dirty state of a new web document. The runner requires the
Python Playwright environment and both browser executables. Set
`ARTSTUDIO_PYTHON` to that environment's Python when it is not the default
`python3`; the gate uses it for its built-in Python checks. An alternative
real executable may be supplied with `AEGO_WASM_PARITY_RUNNER` and remains
responsible for selecting its own interpreter. The parity runner does not
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
both on loopback, and runs the browser, WebGL and parity suites. Optional
`ARTSTUDIO_APP_PORT`, `ARTSTUDIO_DOCS_PORT`, `ARTSTUDIO_PYTHON`,
`ARTSTUDIO_CHROMIUM_EXECUTABLE` and `ARTSTUDIO_FIREFOX_EXECUTABLE` select the
local environment. It records tracked and non-ignored worktree inventories
before and after the run; this is not a Git commit or proof of ignored files.
The workflow uploads the snapshot and diagnostic evidence with a pinned
Gitea-compatible artifact action. The remote runner and artifact service must
still be checked by an actual CI run. This snapshot job never creates a release
gate report or deploys anything.

For a separately isolated HTTPS fixture, the release browser runners support
`AEGO_STUDIO_FIXTURE_CA`, `AEGO_STUDIO_FIXTURE_CA_SHA256`, and
`AEGO_STUDIO_FIXTURE_PROFILE_ROOT`. Set all three or none: the CA is a single
PEM file, the SHA-256 must come from the fixture setup, and the profile root is
an existing private directory. The runners require loopback-resolved HTTPS
origins and `certutil` from `libnss3-tools`, and seed a fresh browser-owned NSS
database for each context. They do not add a system certificate or disable TLS
verification. The caller must independently isolate and verify the network,
configure trust for the other HTTPS clients, and demonstrate rejection of an
unrelated CA; checking DNS addresses alone is not a network boundary. This mode
is test infrastructure, not evidence of production TLS or a passed release
gate. With the variables unset the ordinary browser launch is unchanged.

The GPU E2E gate performs real HTTP/WASM hashing, lighting and alpha-shadow
checks, authored normal/emission updates, Bloom, Flash/Dissolve, CPU particle
flipbooks, actual context loss/restore, DPR resize and 30 document close/open
resource cycles. On Linux Firefox runs headed with a real X display (or
`xvfb-run`); an unavailable WebGL2 environment fails this gate. Screenshots,
renderer/browser versions, resource counters and the exact manifest hash are
saved with the result. A virtual display, SwiftShader or Mesa software renderer
does not establish physical-display or hardware-GPU support. Headless pixel-core
tests alone do not establish GPU support.

`make ci` includes the full public-component runtime coverage gate. The release gate
reuses its evidence only after verifying the unchanged source inventory, the
served Try WASM hash, every component/environment/action/reset observation and
the actual exit code. It does not rerun a second full component suite alongside
the first. An interrupted run or a report from another tree is rejected.
