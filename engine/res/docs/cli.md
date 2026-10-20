---
id: cli
title: The aego command
group: ref-cli
order: 1
---

# The aego command

Every command takes the project directory as its first argument and looks for `project.aego` upwards from the current directory when it is left out. Commands print diagnostics with file, line and code and exit with a non-zero status on any error, so they run unchanged in CI.

## aego new

Creates a project from a template or an engine example and checks it.

| Flag | Meaning |
|---|---|
| `--template empty\|2d\|2.5d\|3d` | starting contents, `2d` by default |
| `--module name` | module path of the scripts, the directory name by default |
| `--example name` | copy an engine example instead of a template |
| `--native` | add a native Go module under `native/` |
| `--engine dir` | engine source tree, needed for native projects and examples |

## aego edit

Opens the project in the editor. Without a project it opens the launcher. Native projects run under a supervisor that rebuilds and restarts the editor when Go code changes.

| Flag | Meaning |
|---|---|
| `--profile name` | player settings profile used by Play |
| `--debug` | verbose logging and GPU debug output |
| `--cpuprofile file` | write a CPU profile |
| `--hidden`, `--headless`, `--frames n` | for tests: hidden window, no window, stop after n frames |

## aego run

Imports assets and runs the main scene. Assets and scripts reload on save; native code is rebuilt and the game restarts with its state.

| Flag | Meaning |
|---|---|
| `--scene file` | run this scene instead of the main scene |
| `--profile name` | player settings profile |
| `--trace file` | write a Chrome trace of the last frames on exit |
| `--headless`, `--frames n` | run without a window, stop after n frames |
| `--debug` | verbose logging and GPU debug output |

## aego check

Parses every document, audits `.meta` sidecars, checks project sections and assets and type-checks every script package. Exit 1 on problems, 2 when native code does not build.

| Flag | Meaning |
|---|---|
| `--explain` | print the reference entry of every reported diagnostic code |
| `--ensure-meta` | write missing `.meta` files instead of reporting them |
| `--manifest file` | check against this bindings manifest instead of building one |

## aego test

Runs `*_test.aes` script tests headless, or with `--determinism` runs the game twice and compares state hashes.

| Flag | Meaning |
|---|---|
| `-run name` | only tests whose name contains this |
| `-v` | list passing tests too; with `--determinism`, the services the hash covers |
| `-budget n` | instruction budget per test |
| `-seed n` | seed of `aego/rand`, and of both runs with `--determinism` |
| `--determinism` | compare two runs instead of running tests |
| `--frames n`, `--every n`, `--scene file` | length of the runs, checkpoint spacing, scene to run |

## aego export

Builds a distributable game that runs without Go and without the project sources.

| Flag | Meaning |
|---|---|
| `--target host\|linux\|windows\|darwin` | target platform, `host` by default |
| `--release`, `--development` | build profile |
| `--out dir` | output directory |
| `--check` | run the pre-export checks only |
| `--json` | print the build report as JSON |
| `--cc compiler` | C compiler for a cross target |

## aego build

Builds the native binary of a project with a `native/` module and prints its path.

## aego bootstrap

Regenerates `.aego/generated` for the native module without building it.

## aego import

Reimports every asset; `--force` ignores the artifact cache.

## aego icons

`aego icons check` compares icon use with the icon sources and fails on missing or unused icons; `aego icons list [--json] [--out file]` prints the manifest.

## aego dump

`aego dump script <package|Type> [--func name]` disassembles a compiled script package with its source lines.

## aego docs

| Command | Meaning |
|---|---|
| `aego docs export [--out file]` | write the documentation registry as JSON |
| `aego docs check [--strict]` | broken links, missing groups and orders, undocumented bindings |
| `aego docs build [--out dir] [--playground=false] [--repo url]` | generate the static site with the playground |
| `aego docs serve [--addr host:port]` | build, serve on localhost and rebuild on changes |

`--project dir` documents a project's own scripts, `--engine dir` points at the engine source tree.
