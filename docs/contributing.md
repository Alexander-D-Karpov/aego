# Contributing

## Build and run

```sh
bash tools/bootstrap-native.sh
make cli
.cache/bin/aego edit examples/platformer
```

`make res` regenerates icons, previews and ES shaders; `make golden-update` rewrites golden images after an intended UI change.

## Check levels

| Level | When | Command |
|---|---|---|
| 1 | after an edit | `make quick`: build and short tests of the packages your change touches and everything that imports them |
| 2 | before a commit | `make affected`, plus `make check-camera`, `check-audio`, `check-gameplay`, `check-mutate`, `check-editor`, `check-web` or `check-docs` for the areas you touched, and `make golden` after UI changes |
| 3 | before a push or a tag | `make ci` |

`tools/affected.sh [base]` lists the affected packages (default base `origin/master`, uncommitted files included). The CI server always runs the full `make ci`.

## Rules for engine code

- **Modules talk through contracts.** A module never imports another module: shared types and services live in `contracts/` (`physics2d`, `camera2d`, `scriptcall`, `signals`, `debugdraw`, …). `tools/depcheck` enforces the import rules; an exception goes into `tools/depcheck/exceptions.txt` with its file and reason.
- **Kernel stays neutral.** Kernel, editor and `sdk` never name a built-in module, its components or its diagnostic codes in string literals; names come from the module catalog.
- **Every component has a Try it scene.** It lives at `res/docs/try/<Component>.aescene` (with `<Component>.aes` and prefabs next to it when needed), shows the component's own effect and runs without warnings; `aego docs check` fails without it.
- **Internal state is not a component.** Controller state lives in unexported fields of the component it belongs to or in a world service. A component that exists only to carry state for a service is registered `Hidden`: it is not in Create, Add Component, Try it or the docs.
- **Requirements add, they do not block.** Declare `Requires`; Add Component, `scene add-comp`, Paste and presets add the missing components in the same undo step.
- **Every diagnostic code has a page with an example.** Component codes (`CAM`, `AUD`, `GMP`, `PHY`, …) also have an example scene that produces the code; `aego docs check` fails without one. Asset codes say `scope: asset`, codes that only an edit during Play produces say `scope: play`.
- **Nodes are labelled, not pathed.** UI shows nodes with `props.NodeLabel` (icon and name, the path in the tooltip).
- **Editor edits go through history.** Every edit is a history command, normally `commands.Mutation`, so Undo, the Play changes diff and agents see it.
- **Hot paths do not allocate.** Systems, mixers and controllers allocate nothing per frame; add `testing.AllocsPerRun` checks and a benchmark next to new hot code.

## Commits

One commit per sub-stage, named `Stage <n><x>: <what>`; decisions go into `ARCHITECTURE.md` as `D<number>` in the same commit.
