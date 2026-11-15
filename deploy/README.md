# Website deployment

This directory contains deployment configuration for Aego's documentation site, playground, and web demos. It includes the container and nginx setup, a CI-gated deployment watcher, and edge-site and WebAssembly synchronization.

## Pinned Asset Studio release

`Dockerfile` deliberately fails without a previously built **release** artifact
in `studio-artifact/` in the build context, its trusted
`ARTSTUDIO_MANIFEST_SHA256`, and the full 40-character `AEGO_COMMIT`. It checks
the artifact bytes, commit, Studio source subset, and exact public origins both
before and after building the site. The generated guides point to that release;
the image does not build a second local `/studio`.

The production Studio origin is `https://aeditor.akarpov.ru/`, with versioned
editor and guide links rooted at the same hostname. The dedicated origin and
edge vhost examples are `aeditor.akarpov.ru.conf`,
`edge/aeditor-static.conf`, and `edge/aeditor-edge.conf`; only one edge
serving strategy should be installed. These files do not deploy the site.

The official site build also requires production example packaging with the
reviewed MIT terms in `docs/try-examples-license.txt`; the license notice is
included in each complete example archive. Local preview builds may instead
report `NOASSERTION` and are not production artifacts.

Prepare a dedicated build context outside the working checkout from the exact
clean commit, add only the checked artifact at `studio-artifact/`, and verify
the entire context against that commit. The source-subset check in the image
does **not** establish that every other site source file matches the commit.
Treat these checks as provenance checks, not release-gate evidence. Test the
exact artifact and completed image together on isolated HTTPS using the real
production origins; do not rebuild them after the final gates. Stage both with
an independently verified rollback before any production change.

`deploy.sh` is not a safe pinned-release procedure: it force-checks out a ref,
builds from the server checkout, and prunes images. Do not run it against a
checkout or Docker installation containing data that must be preserved. The
watcher only checks its configured CI statuses, not all Studio release gates.
