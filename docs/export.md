# Export

`aego export` builds a game that runs on a machine without Go, without the engine sources and without the project.

```
aego export [dir] --target host|linux|windows|darwin[/amd64|/arm64]|web --development|--release --out dir
aego export --check --release        pre-export checks only
aego export --json                   report as JSON
```

In the editor: File → Export… opens the Export panel (target, profile, Check, Export, Reveal in File Manager). Progress is in Task Center, the report stays in the panel. Project Health lists what would block a release export.

## What is built

| Part | Content |
|---|---|
| binary | `cmd/aego-runtime` (or the generated main of a native project) built with `-tags aego_runtime`: no editor, pipeline, compiler, workspace or docs. SDL3 is linked statically. |
| `data/manifest.aetext` | engine version, profile, target, the `game` block (name, module, version, start scene, window, `defaults`, project sections) and the asset index |
| `data/artifacts/` | imported artifacts named by content hash, `<sha>.<kind>`; no `.aes`, `.aescene`, `.meta`, `project.aego` |
| `export.aetext` | Export Manifest: versions, target, profile, modules, binary and its size, libraries with licenses, start scene, data sizes by kind, content hash, build time |
| `licenses/` | SDL3 license |

The runtime reads only the manifest: `vfs/exportfs` opens a file only when the manifest lists it with the same size.

| Target | Layout |
|---|---|
| linux | `game`, `data/`, `game.desktop`, `icon.png` |
| windows | `game.exe` with icon, version info and a PerMonitorV2 manifest; release has no console |
| darwin | `Game.app/Contents/{Info.plist, MacOS/game, Resources/data, Resources/icon.icns}`, ad-hoc signed when `codesign` exists |

## Profiles

- **development**: logs to stderr, debug draw on, unstripped binary.
- **release**: `-trimpath -s -w`; `debug { enabled=false }`; the log goes to `<profile>/logs/game.log`; tracing only with `--trace`; missing asset references block the export.

## Project fields

```
project version=1 {
  gameVersion="1.2.3"
  icon="branding/icon.png"
}
```

The icon must be a PNG of at least 16×16; 1024×1024 square is recommended.

## Checks (D288)

| Code | Meaning | Severity |
|---|---|---|
| EXP001 | no start scene, or it is not an imported scene | error |
| EXP002 | scripts do not compile | error |
| EXP003 | an asset failed to import | error |
| EXP004 | an asset references a missing asset | error in release, warning in development |
| EXP005 | the icon is missing or not a PNG | error |
| EXP006 | duplicate `.meta` ids | error |
| EXP007 | a scene does not load | error |
| EXP008 | a scene uses a component no module registers | warning |

Asset validators (DATxxx and module validators) run as well. A blocked export exits with code 2.

## Running the game

```
game [--data dir] [--profile name] [--windowed|--fullscreen] [--scene id|path]
     [--log file] [--trace file] [--replay file] [--headless --frames n] [--seed n]
     [--observe-checkpoint node-id]
```

For a replay check, pass the stable 32-hex-digit ID of a node with `gameplay.Checkpoint2D`.
The optional observer subscribes to its `Reached(by)` signal. On exit stdout contains a
JSON object with `node`, signal `count`, last simulation `tick` and the activating node's
`by` ID. Before the first signal, count and tick are zero and `by` is absent.
In a web release, set `?observeCheckpoint=<node-id>` and read the same object from
`JSON.parse(aegoGame.stats()).checkpoint`.

Settings, saves, logs and crash bundles live in `UserConfigDir/aego/<module>/profiles/<name>/`, never next to the game.

## Toolchains

The export needs Go and the SDL build for the target in `.cache/native/<os>_<arch>` of the engine tree.

| From → to | Setup |
|---|---|
| host | `make bootstrap` |
| linux → windows | `CC=x86_64-w64-mingw32-gcc GOOS=windows make bootstrap`, then `aego export --target windows --cc x86_64-w64-mingw32-gcc`, or `zig cc -target x86_64-windows-gnu` |
| linux → darwin | osxcross: `CC=o64-clang GOOS=darwin make bootstrap`, `--cc o64-clang` |
| any → web | nothing beyond Go: the build is GOOS=js GOARCH=wasm without cgo |

`CC_<OS>_<ARCH>` (for example `CC_WINDOWS_AMD64`) replaces `--cc`.

## Not in v1

No `.aepak`, no pruning of unreferenced assets (every imported asset except presets is exported), no stripping of embedded engine resources, no installers or notarization.

## Web (D295)

`aego export --target web` builds the same runtime for `js/wasm` and writes a folder that any static server can host:

| File | Content |
|---|---|
| `index.html` | a full-window canvas, the loader with progress and errors, a fullscreen button |
| `loader.js` | fetches `data/files.json`, then `game.wasm`, the manifest and every artifact (six at a time), and starts the game |
| `wasm_exec.js` | the Go WebAssembly support script of the toolchain that built the game, its license in `licenses/Go.txt` |
| `game.wasm` | `cmd/aego-runtime` with `-tags aego_runtime`; release adds `-trimpath -s -w` |
| `data/` | `manifest.aetext`, `artifacts/` and `files.json` with every file and its size |
| `export.aetext` | as for desktop targets, plus `binaryGzip` |

The Go license is copied from `GOROOT/LICENSE`. For a distribution that installs Go in `<prefix>/lib/go` and its license in `<prefix>/share/licenses/go/LICENSE`, the exporter uses that packaged license instead. If neither file exists, the license field is omitted from `export.aetext`; install a complete Go toolchain before distributing the export.

Every file that compresses by more than 10% gets a `.gz` copy next to it, and a `.br` copy when `brotli` is in PATH; serve them with `gzip_static`/`brotli_static` or the equivalent, and serve `.wasm` as `application/wasm`. Artifacts are named by content hash and can be cached forever; `index.html`, `game.wasm`, the manifest and `files.json` carry the export hash in their query strings.

The game reads its data from memory: the loader hands the bytes to the runtime, which mounts them with `vfs/memfs` after checking every file against the manifest. Settings and saves go to `localStorage` through `platform.UserFiles()` (a few MB per site, writes past the quota fail with an error); there are no log files and no crash bundles, the log goes to the browser console. Fullscreen and pointer lock need a click or key press first. URL parameters `?scene=`, `?seed=`, `?profile=` and `?debug=1` replace the command-line flags.

Checks: EXP010 warns about an asset kind the web runtime cannot load (none yet), EXP011 warns that a native module must build without cgo. The target budget is 8 MB gzip for `game.wasm`; `make site` exports the platformer to `site/demo/` and fails above it.

Sound plays through an AudioWorklet and starts after the player's first click or key press; nothing else is needed from the host. When `index.html` is served with `Cross-Origin-Opener-Policy: same-origin` and `Cross-Origin-Embedder-Policy: require-corp`, the page is cross-origin isolated and the audio output uses a SharedArrayBuffer ring, which can use a smaller reserve than the message transport. With COEP every resource the page loads must be same-origin or carry CORP or CORS headers, so add the two headers only when the game loads nothing from other origins. The export does not set them; the engine's own site sends them for `/play/` (`deploy/nginx.conf`).

## Editor app on macOS

`make editor-app` builds `.cache/Aego Editor.app` with the native menu bar: File, Edit, Scene, Play, Assets, Window and Help move from the window into the system bar. Create stays in the Scene View "+" and the Hierarchy menu. Shortcuts are shown in the menu and still go to the editor, so text fields keep their own keys.

## Materials, lighting and surface dependencies

Strict material checks validate current sources/includes against a frozen imported snapshot even while the editor can display last-good. Export includes prepared AESH4/AEMT1, semantic AETX3 textures, AERG4 normal/emission frame mappings and every transitive normal/emission/cookie/height-derived dependency. The runtime compiles prepared shader stages with its graphics driver; it does not import source, expand includes, generate normals or require glslang.

Missing explicit NormalMap/EmissionMap references and broken animated surface-channel pairs block strict export while preserving their IDs for repair. Atlas normal materials are validated with their own region metadata. Frozen hashes include the actual atlas page pixels, so a stale region descriptor cannot hide a changed page. Cookie semantics are checked from prepared CPU metadata without inventing a GPU-ready preview in a headless export.

`make site` exports the canonical `examples/platformer` to `site/demo`, including its lighting sources and updated surface assets. Compare the export content hash and artifact manifest, not only the iframe HTML. Preparing this folder is not a public deploy. Hardware/OS/browser combinations not exercised by a build remain unverified independently of a successful export.
