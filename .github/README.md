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

To show native GitHub results on the Gitea commit, create the GitHub Actions
secret `GITEA_STATUS_TOKEN` containing a Gitea API token with repository write
permission. Prefer a non-admin bot that has access only to `sanspie/aego`:
Gitea's repository-write token is broader than commit statuses. The reporter
publishes pending and final statuses with links to the real GitHub jobs only
for trusted `master` pushes or manual runs, never for pull requests. If the
secret is missing, CI still runs but no Gitea statuses are published.

Keep GitHub Actions disabled through the first mirror sync: historical tags
can contain older workflows. Before enabling Actions, verify mirrored refs and
restrict the allowed workflows so historical release jobs cannot run. Use a
separate repository-scoped credential for the mirror; do not put it in a URL.
Automatic nightly and stable publication is paused until a separately reviewed
release pipeline enforces the necessary gates and preserves existing releases.
