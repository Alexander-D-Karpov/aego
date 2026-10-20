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

## aego mcp

Serves the project to an AI agent over MCP on stdio; see [Working with AI agents](ref:agents).

| Flag | Meaning |
|---|---|
| `--watch` | send resource updates when project files change |
| `--interval d` | how often `--watch` polls, 500ms by default |
| `--project dir` | project directory |

## Editing from the command line

These commands read and change scenes, prefabs, data assets and references without the editor, through the same operations the editor uses, and every change can be undone by applying its inverse. Paths are relative to the project root; a node is a path from the root (`Player/Sprite`) or `#<id>`; a value is written in scene syntax. Every command takes `--json` (a stable machine-readable report), `--quiet`, `--strict` (warnings fail too) and `--project dir`; commands that write take `--dry-run`, which reports the changes and touches nothing. The exit status is 0 on success, 1 on an error and 2 when the result has error-level problems. While the editor has the project open, commands that create assets refuse to run; edits of existing files are picked up by the editor.

<!-- agent:begin -->
| Command | Meaning |
|---|---|
| `aego asset delete <asset> [--force] [--dry-run]` | delete an asset; refuses while it is referenced unless --force |
| `aego asset list [--kind v]` | assets with their ids and kinds |
| `aego asset move <asset> <to> [--dry-run]` | move an asset with its .meta; the id and every reference stay |
| `aego asset rename <asset> <name> [--dry-run]` | rename an asset in its folder |
| `aego data add-row <file> <id> [--dry-run]` | add a row with defaults |
| `aego data base <file> [<asset>] [--dry-run]` | set or clear the base asset of a data asset |
| `aego data export <file> <csv> [--dry-run]` | write the rows of a table to a csv file |
| `aego data get <file> [--row v]` | fields set in a data asset or a table row; omitted fields use the base or the type default |
| `aego data import <file> <csv> [--dry-run]` | replace the rows of a table with a csv file |
| `aego data remove-row <file> <id> [--dry-run]` | remove a row |
| `aego data rename-row <file> <id> <name> [--dry-run]` | change a row id |
| `aego data reset <file> <path> [--row v] [--dry-run]` | remove a field so it falls back to the base or the default |
| `aego data rows <file>` | row ids of a table |
| `aego data set <file> <path> <value> [--row v] [--dry-run]` | set a field |
| `aego prefab apply <file> <node> [--level v] [--owner v] [--field v] [--nearest] [--dry-run]` | write instance overrides into the prefab file |
| `aego prefab create <file> <node> <out> [--dry-run]` | turn a node into a prefab file and link the node to it |
| `aego prefab instantiate <file> <prefab> [--parent v] [--name v] [--index v] [--dry-run]` | add an instance of a prefab |
| `aego prefab overrides <file> <node>` | list the overrides of the instance a node belongs to |
| `aego prefab revert <file> <node> [--level v] [--owner v] [--field v] [--dry-run]` | drop instance overrides |
| `aego prefab unpack <file> <node> [--deep] [--dry-run]` | turn an instance into plain nodes |
| `aego project validate` | missing assets and nodes, import errors, prefabs and scenes that do not load |
| `aego refs find <asset>` | files and fields that reference an asset |
| `aego refs fix <from> [<to>] [--dry-run]` | point every reference to one asset at another (none clears them); originals go to .aego/recovery |
| `aego refs missing` | references to assets and nodes that do not exist |
| `aego scene add-comp <file> <node> <comp> [--dry-run]` | add a component with its defaults |
| `aego scene add-node <file> <name> [--parent v] [--space v] [--index v] [--dry-run]` | add an empty node |
| `aego scene apply <file> [--tx v] [--tx-file v] [--dry-run]` | apply a transaction (tx version=1 { op … }) in one step |
| `aego scene get <file> <node> [<path>]` | a node with its transform and every saved field, or one field |
| `aego scene move <file> <node> <parent> [--index v] [--keep-world] [--dry-run]` | move a node under another parent |
| `aego scene new <file> [--dry-run]` | create an empty scene |
| `aego scene remove-comp <file> <node> <comp> [--dry-run]` | remove a component |
| `aego scene remove-node <file> <node> [--dry-run]` | remove a node with its children |
| `aego scene rename <file> <node> <name> [--dry-run]` | rename a node |
| `aego scene reset <file> <node> <path> [--dry-run]` | reset a field to its default, a transform part to identity |
| `aego scene run [<file>] [--frames v] [--seed v]` | run a scene headless and report the log and the state hash |
| `aego scene set <file> <node> <path> <value> [--dry-run]` | set a field |
| `aego scene tree <file>` | node tree with IDs, components, scripts and prefab links |
| `aego scene validate <file>` | load problems and references to nodes the scene does not have |
| `aego schema list [--kind v]` | component and data types |
| `aego schema show <type>` | fields of a type: kind, range, unit, default, flags and docs |
<!-- agent:end -->

## aego docs

| Command | Meaning |
|---|---|
| `aego docs export [--out file]` | write the documentation registry as JSON |
| `aego docs check [--strict]` | broken links, missing groups and orders, undocumented bindings, Script API modules and script guides without a runnable example |
| `aego docs build [--out dir] [--playground=false] [--repo url]` | generate the static site with the playground |
| `aego docs serve [--addr host:port]` | build, serve on localhost and rebuild on changes |

`--project dir` documents a project's own scripts, `--engine dir` points at the engine source tree.
