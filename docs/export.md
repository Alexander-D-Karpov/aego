# Export

`aego export` builds a game that runs on a machine without Go, without the engine sources and without the project.

```
aego export [dir] --target host|linux|windows|darwin[/amd64|/arm64] --development|--release --out dir
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
```

Settings, saves, logs and crash bundles live in `UserConfigDir/aego/<module>/profiles/<name>/`, never next to the game.

## Toolchains

The export needs Go and the SDL build for the target in `.cache/native/<os>_<arch>` of the engine tree.

| From → to | Setup |
|---|---|
| host | `make bootstrap` |
| linux → windows | `CC=x86_64-w64-mingw32-gcc GOOS=windows make bootstrap`, then `aego export --target windows --cc x86_64-w64-mingw32-gcc`, or `zig cc -target x86_64-windows-gnu` |
| linux → darwin | osxcross: `CC=o64-clang GOOS=darwin make bootstrap`, `--cc o64-clang` |
| any → web | Stage 9.5c |

`CC_<OS>_<ARCH>` (for example `CC_WINDOWS_AMD64`) replaces `--cc`.

## Not in v1

No `.aepak`, no pruning of unreferenced assets (every imported asset except presets is exported), no stripping of embedded engine resources, no web target, no installers or notarization.

## Editor app on macOS

`make editor-app` builds `.cache/Aego Editor.app` with the native menu bar: File, Edit, Scene, Play, Assets, Window and Help move from the window into the system bar. Create stays in the Scene View "+" and the Hierarchy menu. Shortcuts are shown in the menu and still go to the editor, so text fields keep their own keys.
