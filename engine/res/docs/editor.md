---
id: editor
icon: panel.inspector
group: editor
title: Editor
order: 1
---
# Editor

![The Design workspace: Hierarchy, Scene View, Inspector, Assets and Console](shots/editor.png)

## Workspaces and Layout

![The Programming workspace: scripts open next to the Scene View](shots/workspace-programming.png)

A workspace is a named layout: which panels are docked where and whether the toolbar is shown. The editor ships with four:

| Workspace | For | Differs from Design |
|---|---|---|
| Design | building scenes | the default layout |
| Programming | writing scripts | no Hierarchy, Game View or debug panels; scripts open next to the Scene View |
| Art | sprites and levels | Assets along the bottom, no Console or debug panels |
| Audio | sound work, grows with the audio tools | no Hierarchy, Game View or debug panels |

Switch with the selector at the right of the toolbar or with `Ctrl+1` … `Ctrl+4`, which work from any panel, the code editor included. Rearrange panels freely: every workspace remembers its own layout, and documents, selection and Play are shared between them. **Window → Save as Workspace…** keeps the current arrangement under a new name; your workspaces can be renamed and deleted, the built-in ones can only be reset with **Window → Reset Layout**. Modules may add workspaces of their own.

Workspaces are stored in `.aego/user/workspaces.aetext`, the current one in `.aego/user/editor.aetext`. A layout saved by an older editor becomes the Design workspace.

### Maximize and full screen

- `Shift+Space`, a double click on a tab or **Maximize** in the tab menu fills the window with one panel; repeat to bring the layout back.
- `F11` runs the game in full screen: the editor around it disappears and the game gets all input. `Shift+Esc` returns input to the editor and leaves full screen while the game keeps running; Stop leaves it too.
- **Zen Mode** in the command palette gives the active script the whole window without the path bar, breadcrumbs and minimap.

### Undo History

**Window → Undo History** lists the edits of the active scene, one row per action. Click a row to undo or redo up to it; rows after the current one are the ones Redo would bring back. During Play the list shows the Play history, which is dropped on Stop.

## Theme

The editor has three themes: **System**, **Dark** and **Light**. System, the default, follows the light or dark look of the OS and switches when the OS does; when the OS does not say, the editor is dark. Pick one in **Window → Theme** or in the command palette (**Theme: System**, **Theme: Dark**, **Theme: Light**), or with the **Theme** box at the bottom of the launcher. The change applies on the next frame, without a restart.

![The Design workspace in the light theme](shots/editor-light.png)

The Scene View, the Game View, previews and thumbnails keep the same colors in both themes, so a scene looks the same whatever the editor around it. The choice is yours, not the project's: it is saved in `prefs.aetext` in the editor's config directory, next to the list of recent projects, and applies to every project and to the launcher. `AEGO_EDITOR_THEME=system|dark|light` overrides it for one run.

## Projects, Scenes and Previews

The start screen has three tabs. **Recent** shows your projects with a picture of their startup scene. **New Project** shows the templates with a preview and the files they contain; pick a name, a folder, a module path and the workspace the editor opens with. **Examples** copies one of the engine examples into a folder of your choice.

The **Scenes** panel lists every scene of the project with a preview. The startup scene carries a play mark; right-click a scene to open it, make it the startup scene, rename, duplicate or delete it, or show it in Assets.

Assets shows a preview for every file: textures on a checkerboard, scenes and prefabs as small renders, scripts with their first lines, data assets with their type. The **S M L** buttons change the tile size. Previews are cached in `.aego/cache/thumbs` and rebuilt when the file or anything it uses changes; deleting that folder is always safe.

## Editing Many Nodes

Select several nodes and the Inspector shows the components they share. A field whose values differ shows "—"; hover its name to see the values. Any change goes to every selected node and is one Undo step. Clicking, typing or pasting on more than ten nodes asks first.

Component menus copy and paste values, and the Node card copies all components at once. Paste onto a node without the component adds it.

**Presets.** "Save as Preset…" in a component or script menu writes `assets/presets/<name>.aepreset`. "Apply Preset" lists the presets of that type and applies one to every selected node.

**Reset.** Right-click a field: "Reset to Default", or "Reset to Prefab Value" on a prefab override. In data assets with a base: "Reset to Base Value".

**Lock.** The padlock at the top keeps the Inspector on the current nodes while you select others.

**Favorites.** Add assets from their menu, nodes from the Hierarchy menu, and actions with Ctrl+D in the command palette. They show in the Favorites panel and first in the palette.

**Selection history.** `Alt+←` and `Alt+→` step through previous selections, switching scenes when needed.

**Isolation.** `Shift+H` shows only the selected nodes and their children in the Scene View; press it again or "Show All" to leave.

## Keeping Play Changes

When you stop Play after changing something in the Inspector or with a gizmo, the editor lists those changes, already checked, with the value you set (even if the game moved the node afterwards). Stopping without edits shows nothing. **Show all differences** adds everything else the game changed (positions, velocities, counters), unchecked and marked "from gameplay"; the editor remembers the choice. "Apply to scene" copies the checked values as one Undo step. Window → Play can turn the list off.

## Assets and Project Health

**Find References** (`Shift+F12` or the asset menu) lists every scene, prefab, data asset and the project file that uses an asset, with the node, component and field. Click a line to open it. "Replace with…" points every reference at another asset of the same kind; "Clear References" sets them to None. Open documents change through Undo; files that are not open are rewritten on disk and their originals are copied to `.aego/recovery/refs-<time>/`.

**Safe Delete.** Deleting an asset that is still used lists who uses it: show the references to replace them first, delete anyway (the references become missing), or cancel.

**Missing References** lists references to assets that no longer exist, grouped by asset, with the path it had when it was deleted. Use the suggested asset with the same name, pick another, or clear. Missing node references inside scenes and prefabs can be selected or cleared. References from scripts by string path are not detected.

**Batch Rename** works on several assets (asset menu) or nodes (Hierarchy menu): a pattern with `{name}`, `{n}`, `{n:03}`, `{parent}`, `{ext}`, find and replace (optionally a regular expression), and case. The preview marks names that clash; renaming assets keeps their ids, so nothing that uses them breaks.

**Diff.** "Compare with HEAD" in the menu of a scene, prefab or data asset shows what changed since the last commit, or since an earlier commit chosen in the panel: added, removed, moved and renamed nodes, components and fields. Click a node change to select the node. The project must be inside a git repository.

**Tasks.** Imports, previews, reference updates, diffs and checks run in the background. The status bar shows how many are running; click it for the Tasks panel, where any task can be cancelled and failed ones show their error.

**Project Health** checks import errors, problems, missing references, unused assets, scenes nothing loads, names that differ only in case, textures larger than 4096 px and scenes without a camera. Every scene, prefab and script, the startup scene and `assets/presets` count as used. The panel also shows how long the current scene took to open, its node count and how many of its assets are missing.

## Scripts during Play

![A script open in the code editor next to the scene](shots/script.png)

- **Autosave and reload.** With File → Autosave Scripts on (default), a script is saved one second after the last keystroke, and when the editor window loses focus. Each save that compiles is hot-reloaded into the running Play session at the start of the next frame. A save that does not compile leaves the running code untouched. The Problems panel and the red banner above the views show what is broken, and Play cannot be started again until it compiles.
- **Banner.** While a script package has errors, a banner over the Scene and Game views says how many and which package; clicking it opens the first error. The script editor's top bar shows `Scripts: build N · ok | · K errors | · latest failed`.
- **Console.** Every reload prints `Hot reload N → N+1: X migrated, Y reset, Z coroutines canceled, W handlers disconnected, … ms`. Warnings about reset fields and removed scripts follow it on the `script` channel.
- **Profiler.** During Play the Game section shows *coroutines* (active per interval) and *signal emits* graphs. The script table lists active and sleeping coroutines, resumes per frame and total cancelled, signal connections, emits and handler calls per frame, plus per-script instances, calls, time and faults.
- **Restarting coroutines.** Coroutines of a changed script are cancelled by a reload; declare `func Reloaded()` to start them again. Signal connections survive when the signal and the handler keep their names and parameter types.

## Rewinding Play

![Play mode: the Game View with the timeline under it](shots/play.png)

While a game runs in the editor, every frame is recorded: its time step, time scale, pause and the input the game saw. The bar at the bottom of the Game view shows the recorded frames; drag it to scrub through them, or press Ctrl+Left and Ctrl+Right to move one frame back or forward. The Game view follows the handle while you drag, so a jump plays back and forth under the cursor.

The editor keeps keyframes as it plays: copies of the scene, the scripts and every module. How often depends on how heavy the game is: often enough that rewinding replays about 1.5 ms of game, but no more than every 30 ticks and no less than every 300. Older history keeps fewer keyframes: one per 300 ticks after two minutes, one per 1200 after ten. While you drag the timeline back and forth, the editor keeps a few extra keyframes near the cursor, so dragging stays smooth; they are dropped when the game runs again. When keyframes are off, dragging back moves the cursor and rewinds when you release. Rewinding restores the nearest keyframe before the chosen frame and plays the recording from there, without drawing; the game then waits paused at that frame. When a module keeps state that cannot be copied, the bar says "keyframes off" with its name, and rewinding restarts the game from the scene as it was when Play started. Saves made by the game during playback are not written to disk again. Pressing Play or Step from there starts a new timeline and forgets the frames after it. While the editor holds the game paused, nothing runs and nothing is recorded; a pause the game makes itself, such as a pause menu, keeps running and recording.

An edit in the running scene ends the recording at the frame you are on; frames before it can still be rewound to. An edit made before the first frame becomes the new start of the recording. An asset or script reload, a tuned data asset or a changed setting would make the replay run with the new values, so they drop the whole recording. When the history reaches its memory limit, the oldest keyframes are dropped and the bar starts later; without keyframes the limit ends the recording. A module that declares itself non-deterministic turns rewinding off, and the bar says which one.

Selections, watches and the event monitor follow the rewound game: node ids stay the same, and events from playback are hidden unless Show replayed is on in the event monitor.

## Event Monitor

Window → Event Monitor lists what happened in the running game: script signals, engine events (collisions, triggers), script faults, nodes created and destroyed, scene changes, audio and gameplay events (state changes, damage, deaths, respawns, pickups). Every row carries the world frame and the fixed tick, the same numbers as the profiler; clicking a frame in the profiler's strip filters the monitor to it. Click a row to select its node. Engine events are listed for nodes with a handler; with *Selected nodes* on, every event of the selected nodes is listed, handler or not. Export writes the filtered rows to `.aego/user/events.log`.

*Break on* pauses Play when a listed kind matches the text after it, for example `OnTriggerEnter`. The pause lands on the tick of the event: the rest of that frame does not run.

## Watch

Window → Watch shows live values: `Player.PlayerController.Velocity`, `Player.PlayerController.Velocity.y`, `Enemy.EnemyAI.Speed`, `time.Scale`, `time.Tick`. The node part is a path from the scene root, then a component or script name, then a field; `.x .y .z .w` pick one lane. Ctrl+Alt+W in the Inspector adds the field under the mouse. A row with a comparison, like `Player.Health.Hits >= 3`, pauses Play on the tick it becomes true. *HUD* shows the rows over the Game View. The list is saved with the editor state.

## Debug Draw

Window → Debug Draw lists the debug channels of the running game with their counts. Click a channel to hide or show it, or use a preset. Physics channels follow the Physics menu.

## Crash Reports

When a panel, an extension, a native script binding or the play world panics, the editor writes a bundle to `.aego/crash/<time>/`: `info.aetext` (versions, OS, GPU, modules, message), `go.stack`, `script.stack` for script faults, `trace.json` (the last 240 frames, open in ui.perfetto.dev), `events.log`, `scene.aescene` (the play scene, unless `engine { crashScene=false }`) and `log.txt`. Paths under your home folder are written as `~`. Unsaved scenes and scripts are saved to recovery at the same moment. `engine { spikeMs=50 }` writes the same bundle, marked *spike*, for frames slower than 50 ms. Window → Crash Reports lists the bundles; click one to open the script line or copy the folder.

---
id: editor/add-component
icon: ui.plus
parent: editor
order: 1
title: Add Component and Create
---
# Add Component and Create

![The palette: "col" lists the colliders, the right pane describes the highlighted one](shots/add-palette.png)

One palette adds components and creates nodes. Ctrl+Shift+A, or Add Component in the Inspector, adds to the selected nodes; Ctrl+Shift+N, Create… in the Hierarchy or Scene View menus, or the + buttons create a node. Create Empty is Alt+Shift+N.

The search field takes the keys at once, so Ctrl+Shift+A, `mot`, Enter adds Motion 2D without the mouse. The query matches the start of the name (`mot` → Motion 2D), initials (`vc` → Virtual Camera 2D, `cf2` → Camera Follow 2D), the start of any word (`fol`), then any part of the name (`box` → Box Collider 2D, then Hitbox 2D and Hurtbox 2D). Within each kind of match, items you add more often come first.

| Key | Does |
|---|---|
| ↑ ↓, PgUp PgDn, Home End | move the highlight |
| Enter | adds the highlighted item, the first one by default |
| Shift+Enter | adds it and keeps the palette open |
| Tab | switches the alternative of a requirement |
| Ctrl+D | stars or unstars the item |
| Esc | closes |

With an empty query the palette lists the eight items used last, your starred items, then the categories. Categories are folded, except those you used; the palette remembers which are open. The state is kept in `.aego/user/palette.aetext`.

The right pane shows the component's documentation and what comes with it:

- **Also adds** lists the components it requires; they are added first, in the same step.
- **Needs one of** appears when any of several components will do. The first is picked; Tab or a click picks another.
- **Also add…** check boxes offer components that usually go with it, such as a collider for Motion 2D. They start unchecked.
- Items that cannot be added are grey, with the reason: already on the node, needs a 2D node, one per scene, or two components that drive the same node.

With several nodes selected, the component goes on every node it fits, and the pane names the nodes it skips. Everything one pick adds is one Undo step. Project scripts are listed under Scripts with their package on the right.

---
id: editor/agent
icon: panel.agent
parent: editor
order: 2
title: The Agent panel
---
# The Agent panel

The Agent tab, next to the Inspector, talks to Claude Code about the open project. It answers questions, reads the scenes and the documentation, and proposes changes that you apply. The model works through the same tools as `aego` and [`aego mcp`](ref:agents), on the editor's open documents: it sees unsaved edits, and what it changes in an open scene becomes an Undo step there instead of a write to disk.

**Setting up.** Install Claude Code, the `claude` command, and sign in once by running `claude` in a terminal. The panel looks for `claude` on your PATH; when it is not there, the panel says so and takes the path of the program. Check Again looks once more. The path is kept with the project's editor state. Claude Code only gets the aego tools: no shell, no file access of its own.

**Asking.** Type in the field at the bottom and press Enter. Answers are formatted text with code blocks; each tool call is a line you can open for its arguments and result. Stop ends the answer and rejects whatever still waits. Type `@` to pick a node of the open scene or an asset: a node goes in as `@Name (#id in scenes/main.aescene)`, an asset as its path, both the way the tools address them. Right-click a node in the Hierarchy and pick **Ask Agent…**, or a problem in Problems and pick **Ask Agent to Fix**, to start a message about it; the text waits in the field until you send it.

**Changes.** Every change the model wants becomes a card with what it would do: added and removed nodes, fields with the value before and after, and the notes and problems of a dry run. **Apply** makes the change, on the scene as it is now; **Reject** tells the model no. A card nobody answers is rejected after ten minutes. With **Auto-apply** on, cards apply at once, except removals, moves, deletes and reference fixes: those always wait for you and carry a warning sign. **Apply All**, at the top while the model is answering, applies the waiting card and the rest of that answer's cards the same way, and stops when the answer ends. The model is asked to send the edits of one scene together, so a new node with its components and fields is one card and one Undo step.

Each applied change is one Undo step of the scene you edit, named `Agent: …`; a change to a project setting, such as gravity through `project.set`, is a step there too, and Undo puts the old `project.aego` back. **Undo** on a card works while its step is still the last one; after other edits, use Edit → Undo or the Undo History. Changes made during Play go to the edited scene, not to the running game.

**Sessions.** Each conversation is written to `.aego/user/agent/<date-time>.jsonl`, one line per message, tool call, result and card state. The list at the top opens earlier sessions read-only; a new message always starts from Current session.
