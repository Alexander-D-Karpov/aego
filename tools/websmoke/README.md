# Browser checks

Run these scripts from `tools/websmoke` after `npm ci`. Set `CHROMIUM` to an installed Chromium executable. The existing `make web-smoke` builds the site and runs the playground, runtime demo, Try, audio, theme and runtime benchmark gates. Tests do not publish the site.

## Material contract and exported game

`make web-materials` builds the prepared-material fixture and checks actual WebGL2 rendering, typed overrides, source-over/Additive alpha, variants, sampler and block bindings, reload, resource counts and context loss during a pending reload. Missing WebGL2 is a failure, not a successful skip. The JSON states whether `KHR_parallel_shader_compile` is available; an absent extension does not prove the extension-supported path.

To run an already built fixture in two browsers sequentially:

```sh
node materials.mjs --site=../../.cache/web-materials --browsers=chromium,firefox --report=../../.cache/web-materials/report.json
```

`FIREFOX` selects a Firefox executable. Some Linux software-rendered Firefox installations require a display, for example `xvfb-run -a node ...`. `PLAYWRIGHT_MODULE` may select an explicitly installed compatible Playwright module; browser and driver versions are included in the reports.

To verify a completed `examples/materials` web export:

```sh
node export-materials.mjs --root=../../.cache/export-materials-web --browsers=chromium,firefox --report=../../.cache/export-materials-web.json
```

The runner serves only that export directory over local static HTTP. It checks real frames and visible Flash/Dissolve changes, dispatches an actual `WEBGL_lose_context` loss and waits for restoration, then checks animation again. It also rejects requests outside the prepared export files. Captured images are compared in memory; the report contains hashes, counts, versions, failures and the export hash.

## Optional after-only traffic measurements

The normal runtime benchmark records engine bridge counters and an allocation profile. Use the existing gate, without changing its limits:

```sh
node bench.mjs --site=../../site --replay=../../tests/integration/testdata/platformer_run.aeinput --out=../../.cache/web-bench.json --baseline=../../.cache/web-bench.base.json --browsers=chromium --runs=1 --gate
```

The playground and Try runners can additionally intercept WebGL2 calls before wasm initialization:

```sh
node smoke.mjs --site=../../site --traffic=true --report=../../.cache/web-playground-traffic.json
node try.mjs --site=../../site --traffic=true --report=../../.cache/web-try-traffic.json
```

The optional probe reports calls, timer-query calls, actual draw calls, buffer uploads and transferred buffer bytes per completed engine frame. Numeric `bufferData` allocation sizes are not counted as bytes transferred. All draws to that canvas, including presentation, are counted, so this may exceed the renderer's sprite draw count. It records frame-interval and callback-work quantiles; those timings include interception overhead and any host contention. These measurements have no implicit before baseline or frame-rate improvement claim. The optional probe adds no new performance threshold.

For a paired performance conclusion, retain the same browser, driver, build flags, scene, viewport, DPR, replay and measurement windows. Run software GPU workloads sequentially. Do not infer a before/after speedup from these three different profile scenes.

## Screenshot stability

Playground and Try canvas captures align only the CSS origin to a physical pixel. This avoids a fractional document position expanding an otherwise exact 351-pixel drawing buffer into a 352-pixel screenshot. The capture does not resize the canvas, resample its pixels or relax the image threshold.

The Try motion check first verifies that real keyboard input moves the player. Its final UI screenshot then uses an immediately reset and paused scene, with zero simulation frames and the declared initial player pose. This keeps camera follow and wall-clock physics out of the image comparison while preserving the input assertion. `try.mjs --shot=/absolute/path/candidate.png` saves that candidate for review without changing its reference.

Changes to reference images require review of the actual differences. The measurement options do not update goldens or budgets.

## Stage 12 standalone release checks

Run these commands from `tools/websmoke` against a fresh web release of `examples/platformer`. Supply absolute output paths. Each run records the browser version, renderer, WASM and data manifest hashes and exits nonzero on failure. The finish check replays the canonical seed-42 input, observes the Player's actual checkpoint signal on tick 1810 and verifies its count at frame 2400. The restore case measures independent real WebGL loss, restoration, subsequent frames and PostFX execution.

```sh
node platformer-lighting.mjs --scenario=finish --root=/tmp/aego-release/web --out=/tmp/aego-release/check-finish
node platformer-lighting.mjs --scenario=restore --root=/tmp/aego-release/web --out=/tmp/aego-release/check-restore
sh release-visibility.sh --root=/tmp/aego-release/web --out=/tmp/aego-release/check-visibility
```

Visibility runs the normal wall clock in full headed Chromium and Firefox on a new Xvfb display. Install `xvfb-run`, `xauth` and `xdotool`, and use a full Chromium executable (a headless shell cannot test browser tabs). Browser-chrome shortcuts background the real game tab for at least two seconds. The report contains both trusted visibility events, simulation ticks, the resumed frame and the change in clamped time against the existing benchmark gate. Use separate output directories to retain each case.

For the separate topdown release run `node platformer-lighting.mjs --scenario=topdown --root=/tmp/aego-topdown/web --out=/tmp/aego-topdown/check`. It checks the exported module identity, rendered pixels, input events, ticking and draw submission. `TestTopdownExample` exercises collection, chase, damage and return to patrol in the integration runtime.

On desktop pass `--observe-checkpoint <node-id>` to the standalone game alongside `--headless --frames 2400 --seed 42 --replay <recording>` for a JSON summary of the count, last activating node and simulation tick. On web `?observeCheckpoint=<node-id>` enables the same read-only observation in `aegoGame.stats()`.

### Platformer PostFX timing

Run the unchanged demo smoke once against a private site with the candidate export under
`demo/`. Run timing probes separately from builds, CI and browser fixtures. For five
alternating on/off pairs on one immutable web release, use the following command
from the repository root. The result directory must be new.

```sh
python3 tools/websmoke/postfx-pair.py /path/to/export /path/to/new-results
```

Each probe uses seed 42 and the canonical platformer lighting replay, observes the
actual WASM and data manifest responses, verifies the player's camera stack setting,
warms up for 120 frames and measures 600 frame intervals without the frame limiter.
Each run records FPS and interval percentiles, GPU timing when available, scratch
acquisitions, physical passes and shadow-cache counters. The regular 26–32 FPS gate
still runs through `demo.mjs`. For the hardware series, add `--gpu=hard --expect-renderer=Iris`.

For all five particle-golden differences in one diagnostic run, pass
`--collect-all=true` to `particle-goldens.mjs`. It saves expected/actual/difference
images and returns a failing exit code if any original threshold fails.

For residual lighting measurements use `lighting-controls.py ENGINE CLI NEW_DIRECTORY`.
It clones only the canonical example, builds production, particles off, shadow
budget zero, unlit while retaining HDR resolve, and direct unlit using the same
game binary. Measure each with `bench.mjs --root=EXPORT --bool=render2d.postProcessing:0`
and the same replay, seed and interval window as the main pair. Send reports as
diagnostics. The genuine stock production export is the acceptance target.
For a separate pre- and post-mobile-tab observation, run `postfx-tab.mjs` with
the arguments for the demo smoke plus `--diagnose-tab=/path/to/tab.json`.
