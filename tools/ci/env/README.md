# Prepared Linux CI environment

The Gitea Linux jobs use a digest-pinned image built from these inputs. The
image and workspace checks do not imply that full CI or release gates passed.

The base and toolchain images, Python wheels, SDL source and web dependency
inputs are pinned in `lock.json` and the Dockerfile. `preflight.py` verifies
versions, real browser resolution, SDL linkage and the exact locked input set;
it never installs dependencies. Updating an input requires reviewing it and
updating the lock. A changed or newly added SDL patch is rejected.
The image also requires `zstd` and `zstdmt`, matching the runner's cache
compression rather than silently creating incompatible cache versions.

To prepare a build context, use a **new absolute path outside the checkout**:

```sh
python3 -B tools/ci/env/build_context.py --out /path/to/new/context
```

Only the listed inputs are copied. Build the context with resource limits and a
new, non-conflicting image tag; do not overwrite or prune existing images. A
successful image build runs an image preflight plus real Chromium and headed
Firefox smoke tests. Before updating the workflow, verify a workspace on the
actual runner, publish the image, pin its digest, test cache storage, and compare
all mandatory checks. No runner restart, publish or workflow switch is performed
by this context builder.

For a **fresh, protected checkout inside the candidate image**,
`tools/ci/prepare_ci_workspace.py --root "$PWD"` attaches the prepared Node tree
and copies the SDL prefix. It refuses to replace conflicting existing paths.
Follow it with `tools/ci/env/preflight.py --root "$PWD"`; these steps do not
install dependencies. `AEGO_CI_PREPARED=1` makes the shared `web-deps` target
verify that Node tree instead of running npm, and makes `lint` use the pinned
Staticcheck binary. Without that explicit mode the existing local install and
`go run` behavior remains.
