# Materials WebGL2 integration

`make web-materials CHROMIUM=/path/to/chromium` prepares the native test fixtures,
builds a standalone Go/wasm harness, and runs actual WebGL2 in Chromium. `make ci`
includes this target. It fails when WebGL2 or `WEBGL_lose_context` is unavailable;
that environment is not counted as a passed GPU check.

The wasm program embeds only prepared AESH artifacts. The native fixture generator
is `go run ./tools/genwebmaterials/main.go`; the intentionally invalid `broken`
fixture exercises driver failure and last-good retention. Source parsing and the
reference validator are not dependencies of the wasm program.

The runner writes JSON and PNG images to `.cache/web-materials`. The fixed CPU
reference images in `tests/materialgolden` describe linear RGBA8 premultiplied
composition independently of the shader implementation. Entire images, including
transparent holes, edges, checkerboards, and dissolve endpoints, must match within
2 channel values. There is no update-golden option. The native GL suite compares
against the same references.

The checks include all six builtin materials; exact typed int32/bool instance
values; active std140 fields; equivalent batching with overrides; Mesh2D typed
lanes; trimmed atlas geometry, original-source local UV and both flips;
interleaved Sprite/Mesh/Sprite painter order; two camera viewports sharing a
target; failed shader reload; context loss during a submitted but uncommitted
real driver compile; restoration and migration; and repeated resource retirement.
Focused contract checks also verify optimized-out parameter retention, distinct
sRGB/data sampling, sampler-slot and UBO remapping after relink, and separate
time/instance values for two preview worlds submitted through one renderer.
The tiny completion gate makes pending compilation reproducible on fast drivers:
it delays consuming the actual GPU result, without simulating GPU compilation or
context events. Both loss and restore wait for the browser events. Before asking
for restore, the harness yields to a new JavaScript task so the browser has
finished dispatching the cancelable loss event. Observing Go's lost flag while
the `js.FuncOf` callback is still on the JS stack is not that boundary.

Additional browsers are explicit:

```sh
FIREFOX=/path/to/firefox make web-materials WEB_MATERIAL_BROWSERS=chromium,firefox
```

Firefox needs a graphics-capable display on configurations where headless WebGL2
is disabled. For example, run the command through `xvfb-run -a`. The browser and
Playwright versions must match; `PLAYWRIGHT_MODULE=/absolute/path/index.mjs`
selects an already installed matching runtime without changing the project's
package lock. The report records the browser version, renderer, extension
availability, checks, image hashes, and context restoration count.

A report with `parallelShaderCompile: false` verifies the supported fallback. It
does not claim that a driver exposing `KHR_parallel_shader_compile` was tested.
