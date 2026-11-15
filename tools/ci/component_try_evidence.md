# Component Try evidence

`make component-try` runs the public component contracts against native Try,
Chromium and Firefox. Run it in a protected snapshot: its `site` and npm
prerequisites build generated output. Set `COMPONENT_TRY_REPORT` and
`COMPONENT_TRY_EVIDENCE` to fresh paths when retaining multiple runs. The runner
does not replace existing report or evidence paths, and forces dummy SDL audio.

The report binds actual observations and Game View RGBA to the component
registry, contract identity, browser versions, renderer details, and the tested
WASM. `tools/ci/component_try_evidence.py verify` checks before/action/reset,
declared visual and numerical assertions, dependencies, the exact source
inventory, and the final gate exit code. It does not run a browser or replace
the original test run. An absent runtime, malformed evidence, failed test or
changed source is a failure rather than coverage.

`report.test.exit` records the Go suite's exit; `report.exit` remains `RUNNING`
until source and evidence verification finish and then records the enclosing
gate's exit. A process terminated before that point cannot leave a final `0`
merely because the Go suite completed. The internal `--during-gate` verifier
records `SUITE_VERIFIED`, not a completed gate. The separate negative-control
directory contains intentionally wrong expectations and must not be mixed with
the passing observations.

The inventory covers tracked and non-ignored untracked worktree paths. It is a
content fingerprint, not a Git commit or signature, and does not cover ignored
files, the index, or changes made and reverted between observations. Software
renderers, virtual displays and synthetic input are recorded as such; they do
not prove physical OS, display or hardware GPU acceptance. Evidence is only
trustworthy from the protected runner that performed the work.
