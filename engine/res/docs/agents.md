---
id: agents
icon: panel.console
title: Working with AI agents
group: tools
order: 3
---

# Working with AI agents

`aego mcp` serves a project to an AI agent over the Model Context Protocol. The agent edits scenes, prefabs and data assets through the same operations the editor uses, reads the documentation and the type schema, and runs scenes headless to check its work. Project editing commands can also be used from the CLI; operations that need a live view or an open document are supplied by the editor.

## Connecting

### Inside the editor

Open the **Agent** panel and choose **Claude Code** or **Codex** in its provider row. The selected provider, CLI path and model override are saved in the editor's project preferences. Existing projects keep Claude Code as their default.

For Codex, install the official CLI (version 0.166.0 or newer) and sign in once in a terminal:

```sh
npm install -g @openai/codex
codex login
```

The editor uses that CLI's existing authentication. In **Agent settings**, choose a model from the list or type to filter it; Codex models and their supported reasoning levels come from the installed CLI. The list does not guarantee that your account has access. You can also type a custom model ID, choose a custom reasoning level, or select **CLI default** to use the CLI configuration. Claude Code offers its usual model aliases. A missing executable or sign-in error is shown in the panel; after installing or signing in, use **Check again** to refresh. The path field accepts an executable name or a full path; press Enter to apply it.

Both providers use the same project tools, change proposals, questions and Undo history. Changes to open documents remain unsaved until you save them. **Stop** cancels the current turn and rejects pending proposals. Switch provider or change its settings while idle: this starts a fresh conversation, while the previous transcript remains available in session history under its provider name.

### Verify the open scene

In Agent Settings → Diagnostics, check the provider, active project, document and last tool call. Reconnect starts a new conversation binding. Explain what this scene does sends a bounded snapshot of the current editor document, including its selection, revision and script references; it does not require Auto-apply. On Codex the panel checks the selected thread's MCP catalog and probes the bridge through that thread. A listed tool and a completed read are reported separately.

Use `asset_view` for the Assets preview, source pixels or the imported region, and `asset_contact_sheet` for a folder, frame sequence or explicit list of assets. `editor_view_capture` reads the current Game or Scene View; for reproducible frames, use `scene_run` with the same scene, camera, input, seed and shot, then pass both capture IDs to `scene_compare_frames`. PNGs appear in the tool card. A model without image input can still use text tools.

`asset_paint` edits source pixels in one reversible operation. External image generation, when an adapter is installed, requires separate consent with the references and a total spend limit. Use `project_export_game` and `project_run_export` to check a saved release in its own output folder. Auto-apply does not grant permission to run native code or publish anything.

### From an external CLI

Claude Code, from the project directory:

    claude mcp add aego -- aego mcp --project . --watch

or a `.mcp.json` next to `project.aego`:

    {"mcpServers": {"aego": {"command": "aego", "args": ["mcp", "--watch"]}}}

For an external Codex session started in the project directory:

```sh
codex mcp add aego -- aego mcp --project . --watch
```

Other clients take the same command. The server speaks MCP over stdio; logs go to stderr. A project with native code is served by its own binary, so its components are known.

## Tools

Each tool has the name of its command with underscores (`scene set` is `scene_set`) and the same arguments; `aego scene help` lists them, and the CLI reference has the full table. `docs_search` and `docs_read` read the documentation. Write tools take `dry-run`, which reports the changes and writes nothing. The script tools answer with what the compiler says about the new text, the same `AES` errors as `aego check`. In the editor the agent also has `ask_user`, which puts a question with options in the Agent panel and waits for the answer.

## Resources

| URI | Content |
|---|---|
| `aego://project/manifest` | `project.aego` |
| `aego://project/problems` | what `project_validate` reports |
| `aego://docs/index`, `aego://docs/<id>` | documentation |
| `aego://schema/index`, `aego://schema/<type>` | types and their fields |
| `aego://file/<path>` | a scene, prefab, data asset or table as text |

With `--watch` the server polls the project and sends `resources/updated` for subscribed resources when files change, including changes made in the editor.

## A typical session

1. `scene_tree` and `schema_show` to see what is there and what a component takes.
2. Changes with `scene_set`, `scene_add_node`, `scene_add_comp`, `prefab_instantiate`, or several at once with `scene_apply`; `dry-run` first when unsure. `scene_add_comp` also attaches a script by its type name (`EnemyAI`), and the script's exported fields are read and set like component fields (`EnemyAI.PointA`). A node reference field takes a node path such as `Root/PatrolA1`.
3. `scene_validate`, then `scene_run` with a number of frames: the log has the warnings and errors, and the state hash shows whether two runs behave the same. `watch` names nodes whose positions the report samples through the run, to check that something moves. In the editor `scene_run` plays the scene as the editor has it, unsaved changes included, in a separate process.
4. Play it like a player would and check the outcome in the same run:
   - `input` presses actions by frame: `30 press Jump; 10-60 hold Right; 90 release Right; 0-40 axis Move 1`. The names are the input actions of `project.aego`.
   - `assert` checks values, at a frame or at any frame of the run: `@120 Player.transform.position.y < 3; EnemyLeft.Health.hp == 0; Enemy.script/Enemy.hp ~= 2.5; exists Ledge; @60 gone Coin`. Paths are the ones `scene_set` takes; `~=` compares numbers within 0.01. The report says `ok` or `failed` with the value found and the frame; a failed check fails the run, and the CLI exits with 2.
   - `shot` captures up to three frames: `shot=30,90`. The answer has the images scaled to 640 pixels wide (`scale` changes it) and the paths of the full frames under `.aego/cache/runs/`. Screenshots need a display: on a server, start the agent or `aego mcp` under `xvfb-run -a` (Debian and Ubuntu: `apt install xvfb`), or leave out `shot`. Without either, the run stops with that advice.

```sh
aego scene run scenes/main.aescene --frames 120 --input "1-80 axis Move 1; 40 press Jump" --assert "Player.transform.position.x > -10" --shot 60
```

## Drawing sprites

An agent draws the art it needs instead of building characters from colored rectangles. Both tools write a PNG under `assets/`, import it and answer with a preview image, so the agent can look and draw again with `replace`; `dry-run` only shows the preview.

- `asset_pixels` takes pixel art as text: each character of `grid` is a pixel, `palette` maps characters to colors, `.` is transparent. Lines such as `idle:` and `run1:` start frames of one size; they become one atlas with a sprite per frame (`hero_idle`, `hero_run1`) and a looping flipbook clip `hero.aeanim`, ready for an Animation Player. `scale` repeats each pixel; textures are filtered as pixels.

```
asset_pixels path=assets/sprites/hero.png scale=4
palette: {"r": "#d0312d", "s": "#f4c7a1", "b": "#2b4fd8"}
grid:
idle:
..rrrr..
.rrrrrr.
.ssbss..
run1:
..rrrr..
.rrrrrr.
..sbbs..
```

- `asset_svg` draws an SVG document at `width`×`height`: `path`, `rect`, `circle`, `ellipse`, `line`, `polyline`, `polygon`, groups, `fill`, `stroke`, opacity and `transform`. Filters, gradients and text are not drawn; the answer names what was skipped. For tiles, UI and item icons.

A project keeps its drawings in one style with an `art` block in `project.aego`:

```
art {
  palette=["#1a1c2c", "#5d275d", "#b13e53", "#ef7d57", "#ffcd75", "#a7f070"]
  pixelsPerUnit=16
}
```

Without a `palette` argument, `asset_pixels` uses this palette with `0`–`9` and `a`–`z` for its colors; `pixelsPerUnit` is art pixels per world unit, so a drawing is scaled up to the render `pixelsPerUnit`. **Project Health** lists pixel-art sprites that use colors outside the palette.

## With the editor open

The editor and the agent can work on the same project. While the editor is open, `aego` commands and `aego mcp` go through it: the editor writes the address of its proxy in `.aego/editor.lock`, and the CLI sends each command there.

- A change is an Undo step of the open scene, named after where it came from: `via CLI: Rename`, or `via agent: <client>: Add Ledge` for an agent connected with `aego mcp`. The file changes when you save, as with your own edits.
- **Window → Confirm Changes from Outside Agents** (`editor.proxy.confirm`, off by default) makes these changes wait in the Agent panel like its own agent's.
- Follow mode shows where an outside agent works, the same as the panel's agent.
- Commands that create assets work: the editor creates them. `prefab_create` on an open scene with unsaved changes makes the prefab and links the node as an Undo step; the scene stays unsaved.
- When the editor does not answer, the CLI works on the files itself. A scene, prefab or data asset changed on disk reloads in the editor when it has no unsaved changes; with unsaved changes the editor asks whether to reload or keep its version.
