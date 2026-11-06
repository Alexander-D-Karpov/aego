# CI workflows

Gitea is the source of truth. The Linux workflow runs there; optional Gitea
platform runners remain controlled by `PLATFORM_RUNNERS`.

`github-native.yml` is for the GitHub mirror. It builds and tests on native
Windows, macOS (arm64 and amd64), and Linux runners, and uploads unsigned CI
artifacts and diagnostic logs. The Windows installer smoke is restricted to an
ephemeral GitHub-hosted runner. macOS bundles are ad-hoc signed, not notarized.
These jobs are not evidence of platform support until they actually pass on
their respective runners.

On Gitea, the GitHub-only matrix routes to the Linux runner before being
skipped. This prevents Windows and macOS entries from waiting indefinitely for
runners that Gitea does not have. On GitHub, the actual native labels are used.
macOS and Windows build the pinned GLSL reference validator from verified
source, rather than depending on a changing package-manager version.

Keep GitHub Actions disabled through the first mirror sync: historical tags
can contain older workflows. Before enabling Actions, verify mirrored refs and
restrict the allowed workflows so historical release jobs cannot run. Use a
separate repository-scoped credential for the mirror; do not put it in a URL.
Automatic nightly and stable publication is paused until a separately reviewed
release pipeline enforces the necessary gates and preserves existing releases.
