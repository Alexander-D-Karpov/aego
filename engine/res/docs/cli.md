---
id: cli
title: The aego command
group: ref-cli
order: 1
---

# The aego command

Project-oriented commands take the project directory as their first argument and look for `project.aego` upwards from the current directory when it is left out. Commands report errors with a non-zero status, so they can run in CI.

## aego examples

Manages local copies of data-only remote example projects without Git. `aego examples help` lists the operations. For example:

```text
aego examples clone --url https://example.org/example.json --approve-origin https://example.org
aego examples recent
aego examples reopen --id LOCAL_COPY_ID
aego examples check --id LOCAL_COPY_ID --approve-origin https://example.org
```

Cloning requires an explicitly approved exact HTTPS origin, checks the actual engine version and built-in modules, and verifies the manifest, archive, paths, sizes and SHA-256 hashes. `open` first reuses a local copy, including offline edits; `clone` creates a new copy; `check` reports updates without overwriting files. Results include the local directory and entry scene. These commands do not run the project or open the editor: invoke `aego edit <directory>` separately after reviewing the example. `link` and `inspect` only generate or parse a link; they do not register a protocol handler or grant trust.

The local store currently rejects writable Windows directories because its private-directory check does not yet validate Windows ACLs. Windows storage support is not yet available; link inspection remains data-only.

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

Check opens the asset index read-only unless `--ensure-meta` is given; a script outside `scripts/` is reported as an import error.

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
| `--release-json` | also pack the export into one archive and write the signed `release.json` its copies update from |
| `--channel stable\|nightly` | channel of that release |
| `--base-url url` | where the archive is downloaded from, `update.url/<channel>` by default |
| `--key-file file` | private key that signs `release.json`, from `aego release keygen` |

A game updates itself when `project.aego` has `update { url="https://…" publicKey="…" }`; upload the archive, `release.json` and `release.json.sig` to `<url>/<channel>/`. See [aego/update](ref:aego/update).

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

## aego version

`aego version` prints the engine version and the commit the binary was built from, such as `aego 0.11.0-dev (6151233e0586, 2026-10-25T13:17:32Z)`. A build from a checkout with uncommitted changes adds `-dirty` to the commit.

## aego update

`aego update [status|check|apply|restore] [--channel stable|nightly]` manages updates of an installed copy: `status` shows the version, the mode and how Aego was installed, `check` looks for a newer version, `apply` downloads and installs it for the next start, `restore` goes back to the version before the last update. See [Updates](ref:install).

## aego release

`aego release keygen [--out base]` makes the Ed25519 pair that signs releases: `<base>.key` is the private key, readable only by you and never committed; `<base>.pub` is the public key for `project.aego` or the build. Without `--out` the pair goes to `keys/release` in the user config folder. `aego release manifest --version v --channel c --base-url url [--notes file] [--min v] [--key-file file] [--out dir] <files>…` writes `release.json` for the files and signs it into `release.json.sig`.

## aego mcp

Serves the project over MCP on stdio, for tools such as Claude Code; see [Working with AI agents](ref:agents).

| Flag | Meaning |
|---|---|
| `--watch` | send resource updates when project files change |
| `--interval d` | how often `--watch` polls, 500ms by default |
| `--project dir` | project directory |

## Editing from the command line

These commands read and change scenes, prefabs, data assets and references without the editor. They use the same operations as the editor, and every change can be undone by applying its inverse.

- Paths are relative to the project root. A node is a path from the root (`Player/Sprite`) or `#<id>`. A value is written in scene syntax.
- Every command takes `--json` (a stable machine-readable report), `--quiet`, `--strict` (warnings fail too) and `--project dir`.
- Commands that write take `--dry-run`, which reports the changes and touches nothing.
- The exit status is 0 on success, 1 on an error and 2 when the result has error-level problems.
- While the editor has the project open, commands that create assets refuse to run. The editor picks up edits of existing files.

<!-- agent:begin -->
| Command | Meaning |
|---|---|
| `aego anim add-event <file> <time> <name> [--payload v] [--mode v] [--dry-run]` | add an event; the name must be declared in Project Settings → Animation |
| `aego anim expand-frames <file> <target> [--dry-run]` | turn a frames block into sprite tracks with step keys, so its frames can be edited one by one |
| `aego anim get <file>` | length, loop, tracks with their keys, frames blocks and events of a clip |
| `aego anim move-event <file> <index> <time> [--dry-run]` | move an event to another time |
| `aego anim move-key <file> <target> <prop> <time> <to> [--dry-run]` | move a key to another time |
| `aego anim remove-event <file> <index> [--dry-run]` | remove an event |
| `aego anim remove-key <file> <target> <prop> <time> [--dry-run]` | remove a key; the last key removes its track |
| `aego anim remove-track <file> <target> <prop> [--dry-run]` | remove a track with all its keys |
| `aego anim set <file> <attr> [<value>] [--dry-run]` | set length, loop or fps of a clip; no value clears loop or fps |
| `aego anim set-key <file> <target> <prop> <time> <value> [--kind v] [--interp v] [--tangent v] [--dry-run]` | set a key; a new track needs --kind |
| `aego art apply <file> [--revision v] [--label v] [--operations v] [--dry-run]` | Propose one atomic graphics edit at the revision returned by art.read. Agent panel edits require approval; CLI and external agents follow the editor's confirmation setting. All use the document's common Undo. Name new entities with as and refer to them as $name later. |
| `aego art capture <file> [--frame v] [--channel v] [--out path]` | Capture unsaved CPU pixels with document/frame/channel identity and revision. MCP returns a PNG image; the CLI writes it only with --out to a new file. |
| `aego art read <file> [--section v] [--offset v] [--limit v]` | Read the current art document, not its last saved file. Lists are paginated; revision is a decimal string for art.apply. |
| `aego asset delete <asset> [--force] [--dry-run]` | delete an asset; refuses while it is referenced unless --force |
| `aego asset import-variant [--variant v] [--path v] [--replace] [--dry-run]` | Import an inspected generated PNG variant from this editor conversation as a stable project source asset |
| `aego asset list [--kind v]` | assets with their ids and kinds |
| `aego asset move <asset> <to> [--dry-run]` | move an asset with its .meta; the id and every reference stay |
| `aego asset paint <path> [--ops v] [--width v] [--height v] [--source v] [--palette v] [--replace] [--dry-run]` | Edit a source PNG, create a transparent canvas, or paint on a copy of another source PNG. A single reversible batch; returns image pixels. Coordinates are top-left source pixels. |
| `aego asset pixels <path> [--grid v] [--palette v] [--scale v] [--flipbook] [--replace] [--dry-run]` | draw a sprite as text: each character of grid is a pixel colored by palette; lines like idle: start frames of one size, which become an atlas with one sprite per frame and a looping flipbook clip. Answers with a preview image |
| `aego asset rename <asset> <name> [--dry-run]` | rename an asset in its folder |
| `aego asset svg <path> [--svg v] [--width v] [--height v] [--replace] [--dry-run]` | draw a texture from SVG: path, rect, circle, ellipse, line, polyline, polygon, fill, stroke, opacity and transform; no filters, gradients or text. For tiles, UI and item icons. Answers with a preview image |
| `aego ctrl add-param <file> <name> <kind> [--default v] [--dry-run]` | add a parameter |
| `aego ctrl add-state <file> <name> [--clip v] [--speed v] [--loop v] [--pos v] [--dry-run]` | add a state |
| `aego ctrl add-transition <file> <from> <to> [--duration v] [--exit v] [--interrupt] [--cond v] [--dry-run]` | add a transition; from may be any |
| `aego ctrl entry <file> <name> [--dry-run]` | set the state the animator starts in |
| `aego ctrl get <file>` | parameters, states and transitions of a controller |
| `aego ctrl remove-param <file> <name> [--dry-run]` | remove a parameter; conditions on it become problems |
| `aego ctrl remove-state <file> <name> [--dry-run]` | remove a state and the transitions into and out of it |
| `aego ctrl remove-transition <file> <index> [--dry-run]` | remove a transition |
| `aego ctrl set-state <file> <name> <attr> [<value>] [--dry-run]` | set clip, speed, loop, events or pos of a state |
| `aego ctrl set-transition <file> <index> <attr> [<value>] [--dry-run]` | set to, duration, exitTime or interrupt of a transition |
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
| `aego material apply <file> [--tx v] [--expected v] [--dry-run]` | apply a material transaction atomically, using the same mutations and serialization as Inspector |
| `aego material assign <file> <node> <material> [--dry-run]` | assign a compatible material to SpriteRenderer or Mesh2D through the scene mutation service |
| `aego material create <file> <shader> [--dry-run]` | create a material inheriting defaults from a shader |
| `aego material get <file>` | read explicit/inherited parameters, unknown data, variants and source revision from the current document |
| `aego material reset <file> <param> [--expected v] [--dry-run]` | remove an explicit parameter and inherit the shader default |
| `aego material set <file> <param> <value> [--expected v] [--dry-run]` | set a typed explicit parameter as one semantic mutation |
| `aego material validate <file>` | validate material values, shader and texture references without changing the source |
| `aego material variants <file> [--keywords v] [--expected v] [--dry-run]` | select an allowed combination of comma-separated keywords; an empty string selects no keywords |
| `aego postfx get <file> <node>` | read the authored camera stack including stable instance IDs from the live scene |
| `aego postfx set <file> <node> <instance> <field> <value> [--dry-run]` | set an effect switch or typed numeric parameter by persistent instance ID in one scene edit |
| `aego prefab apply <file> <node> [--level v] [--owner v] [--field v] [--nearest] [--dry-run]` | write instance overrides into the prefab file |
| `aego prefab create <file> <node> <out> [--dry-run]` | turn a node into a prefab file and link the node to it |
| `aego prefab instantiate <file> <prefab> [--parent v] [--name v] [--index v] [--dry-run]` | add an instance of a prefab |
| `aego prefab overrides <file> <node>` | list the overrides of the instance a node belongs to |
| `aego prefab revert <file> <node> [--level v] [--owner v] [--field v] [--dry-run]` | drop instance overrides |
| `aego prefab unpack <file> <node> [--deep] [--dry-run]` | turn an instance into plain nodes |
| `aego project get [<section>]` | module sections of project.aego (physics, …) with their current values |
| `aego project input-action <name> [--type v] [--bind v] [--context v] [--active] [--dry-run]` | Create/update one action and add it to a context. Binding strings use the project input syntax. Only an empty initial stack is initialized. |
| `aego project main-scene <file> [--dry-run]` | Set an imported scene as the startup scene by project-relative path. |
| `aego project set <section> <field> [<value>] [--reset] [--dry-run]` | set a field of a module section in project.aego, for example physics gravity [0, -15] |
| `aego project validate` | missing assets and nodes, import errors, prefabs and scenes that do not load |
| `aego refs find <asset>` | files and fields that reference an asset |
| `aego refs fix <from> [<to>] [--dry-run]` | point every reference to one asset at another (none clears them); originals go to .aego/recovery |
| `aego refs missing` | references to assets and nodes that do not exist |
| `aego scene add-comp <file> <node> <comp> [--preset v] [--no-required] [--dry-run]` | add a component with its defaults and the components it requires, or attach a script by its type name |
| `aego scene add-node <file> <name> [--parent v] [--space v] [--index v] [--dry-run]` | add an empty node |
| `aego scene apply <file> [--tx v] [--tx-file v] [--dry-run]` | apply a transaction (tx version=1 { op … }) in one step |
| `aego scene get <file> <node> [<path>]` | a node with its transform and every saved field, or one field |
| `aego scene move <file> <node> <parent> [--index v] [--keep-world] [--dry-run]` | move a node under another parent |
| `aego scene new <file> [--dry-run]` | create an empty scene |
| `aego scene remove-comp <file> <node> <comp> [--dry-run]` | remove a component |
| `aego scene remove-node <file> <node> [--dry-run]` | remove a node with its children |
| `aego scene rename <file> <node> <name> [--dry-run]` | rename a node |
| `aego scene reset <file> <node> <path> [--dry-run]` | reset a field to its default, a transform part to identity |
| `aego scene run [<file>] [--frames v] [--seed v] [--watch v] [--input v] [--assert v] [--camera v] [--shot v] [--scale v]` | run a scene headless and report the log, the state hash and where the watched nodes went |
| `aego scene set <file> <node> <path> <value> [--dry-run]` | set a field |
| `aego scene tree <file> [--node v]` | node tree with IDs, components, scripts and prefab links |
| `aego scene validate <file>` | load problems and references to nodes the scene does not have |
| `aego schema list [--kind v]` | component and data types |
| `aego schema show <type> [--lang v]` | fields of a type: kind, range, unit, default, flags and docs |
| `aego script create <file> [--text v] [--dry-run]` | create a script file; it fails when the file exists, so change an existing script with script.edit |
| `aego script edit <file> [--old v] [--new v] [--dry-run]` | replace text in a script; old must match exactly once, so quote enough lines around the change |
| `aego script read <file> [--from v] [--to v]` | text of a script, as the editor has it, with unsaved changes; from and to read a part of a long one |
| `aego script write <file> [--text v] [--dry-run]` | replace the whole text of a script; for a small change script.edit is safer |
| `aego shader create <file> [--dry-run]` | create a portable sprite shader through the registered asset importer |
| `aego shader schema <shader>` | declared parameters, typed defaults, domain and allowed variants; checks current source and unsaved includes |
| `aego shader validate <shader>` | validate current source with the registered importer; reports schema/reference validation independently from GPU linking |
| `aego texture apply <file> [--tx v] [--expected v] [--dry-run]` | atomic typed texture metadata transaction; same validation and Undo rules as Inspector |
| `aego texture channel <file> <channel> <asset> [--frame v] [--expected v] [--dry-run]` | assign a normal/height/emission source or frame by stable AssetID; unresolved IDs are retained with diagnostics |
| `aego texture channel-reset <file> <channel> [--frame v] [--expected v] [--dry-run]` | remove the explicit channel mapping without deleting source assets |
| `aego texture get <file>` | read current dirty metadata, stable frames, source size, roles and channel references |
| `aego texture reset <file> <setting> [--expected v] [--dry-run]` | remove an explicit import setting and inherit the importer default |
| `aego texture schema` | registered import roles/settings and source-frame channel contract |
| `aego texture set <file> <setting> <value> [--expected v] [--dry-run]` | set an import setting using the shared semantic mutation service |
| `aego texture validate <file>` | check source metadata and prepare a private import candidate without changing cache or GPU resources |
<!-- agent:end -->

## aego docs

| Command | Meaning |
|---|---|
| `aego docs export [--out file]` | write the documentation registry as JSON |
| `aego docs check [--strict]` | broken links, missing groups and orders, undocumented bindings, Script API modules and script guides without a runnable example |
| `aego docs build [--out dir] [--playground=false] [--repo url]` | generate the static site with the playground |
| `aego docs serve [--addr host:port]` | build, serve on localhost and rebuild on changes |

`--project dir` documents a project's own scripts, `--engine dir` points at the engine source tree.
