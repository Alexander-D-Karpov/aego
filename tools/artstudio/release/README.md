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
Passing a subset of tests does not authorize deployment. Production staging,
activation and rollback remain separate explicit operations after all gates pass.
