---
id: editor
title: Editor
order: 2
---
# Editor

## Workspaces and Layout

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

When you stop Play after changing something, the editor lists what differs from the scene. Fields you changed in the Inspector during Play are already checked; everything else (positions, velocities, counters) changed by the game itself and stays unchecked. "Apply to scene" copies the checked values as one Undo step. Window → Play chooses whether the list shows when there are changes, always, or never.

## Assets and Project Health

**Find References** (`Shift+F12` or the asset menu) lists every scene, prefab, data asset and the project file that uses an asset, with the node, component and field. Click a line to open it. "Replace with…" points every reference at another asset of the same kind; "Clear References" sets them to None. Open documents change through Undo; files that are not open are rewritten on disk and their originals are copied to `.aego/recovery/refs-<time>/`.

**Safe Delete.** Deleting an asset that is still used lists who uses it: show the references to replace them first, delete anyway (the references become missing), or cancel.

**Missing References** lists references to assets that no longer exist, grouped by asset, with the path it had when it was deleted. Use the suggested asset with the same name, pick another, or clear. Missing node references inside scenes and prefabs can be selected or cleared. References from scripts by string path are not detected.

**Batch Rename** works on several assets (asset menu) or nodes (Hierarchy menu): a pattern with `{name}`, `{n}`, `{n:03}`, `{parent}`, `{ext}`, find and replace (optionally a regular expression), and case. The preview marks names that clash; renaming assets keeps their ids, so nothing that uses them breaks.

**Diff.** "Compare with HEAD" in the menu of a scene, prefab or data asset shows what changed since the last commit, or since an earlier commit chosen in the panel: added, removed, moved and renamed nodes, components and fields. Click a node change to select the node. The project must be inside a git repository.

**Tasks.** Imports, previews, reference updates, diffs and checks run in the background. The status bar shows how many are running; click it for the Tasks panel, where any task can be cancelled and failed ones show their error.

**Project Health** checks import errors, problems, missing references, unused assets, scenes nothing loads, names that differ only in case, textures larger than 4096 px and scenes without a camera. Every scene, prefab and script, the startup scene and `assets/presets` count as used. The panel also shows how long the current scene took to open, its node count and how many of its assets are missing.

## Scripts during Play

- **Autosave and reload.** With File → Autosave Scripts on (default), a script is saved one second after the last keystroke, and when the editor window loses focus. Each save that compiles is hot-reloaded into the running Play session at the start of the next frame. A save that does not compile leaves the running code untouched. The Problems panel and the red banner above the views show what is broken, and Play cannot be started again until it compiles.
- **Banner.** While a script package has errors, a banner over the Scene and Game views says how many and which package; clicking it opens the first error. The script editor's top bar shows `Scripts: build N · ok | · K errors | · latest failed`.
- **Console.** Every reload prints `Hot reload N → N+1: X migrated, Y reset, Z coroutines canceled, W handlers disconnected, … ms`. Warnings about reset fields and removed scripts follow it on the `script` channel.
- **Profiler.** During Play the Game section shows *coroutines* (active per interval) and *signal emits* graphs. The script table lists active and sleeping coroutines, resumes per frame and total cancelled, signal connections, emits and handler calls per frame, plus per-script instances, calls, time and faults.
- **Restarting coroutines.** Coroutines of a changed script are cancelled by a reload; declare `func Reloaded()` to start them again. Signal connections survive when the signal and the handler keep their names and parameter types.

## Rewinding Play

While a game runs in the editor, every frame is recorded: its time step, time scale, pause and the input the game saw. The bar at the bottom of the Game view shows the recorded frames; drag it to scrub through them, or press Ctrl+Left and Ctrl+Right to move one frame back or forward. The Game view follows the handle while you drag, so a jump plays back and forth under the cursor.

Every 300 ticks the editor also keeps a keyframe: a copy of the scene, the scripts and the state of every module. Rewinding restores the nearest keyframe before the chosen frame and plays the recording from there, without drawing; the game then waits paused at that frame. When a module keeps state that cannot be copied, the bar says "keyframes off" with its name, and rewinding restarts the game from the scene as it was when Play started. Saves made by the game during playback are not written to disk again. Pressing Play or Step from there starts a new timeline and forgets the frames after it. While the editor holds the game paused, nothing runs and nothing is recorded; a pause the game makes itself, such as a pause menu, keeps running and recording.

An edit in the running scene ends the recording at the frame you are on; frames before it can still be rewound to. An edit made before the first frame becomes the new start of the recording. An asset or script reload, a tuned data asset or a changed setting would make the replay run with the new values, so they drop the whole recording. When the history reaches its memory limit, the oldest keyframes are dropped and the bar starts later; without keyframes the limit ends the recording. A module that declares itself non-deterministic turns rewinding off, and the bar says which one.

Selections, watches and the event monitor follow the rewound game: node ids stay the same, and events from playback are hidden unless Show replayed is on in the event monitor.

## Event Monitor

Window → Event Monitor lists what happened in the running game: script signals, engine events (collisions, triggers), script faults, nodes created and destroyed, scene changes. Every row carries the world frame and the fixed tick, the same numbers as the profiler; clicking a frame in the profiler's strip filters the monitor to it. Click a row to select its node. Engine events are listed for nodes with a handler; with *Selected nodes* on, every event of the selected nodes is listed, handler or not. Export writes the filtered rows to `.aego/user/events.log`.

*Break on* pauses Play when a listed kind matches the text after it, for example `OnTriggerEnter`. The pause lands on the tick of the event: the rest of that frame does not run.

## Watch

Window → Watch shows live values: `Player.PlayerController.Velocity`, `Player.PlayerController.Velocity.y`, `Enemy.EnemyAI.Speed`, `time.Scale`, `time.Tick`. The node part is a path from the scene root, then a component or script name, then a field; `.x .y .z .w` pick one lane. Ctrl+Alt+W in the Inspector adds the field under the mouse. A row with a comparison, like `Player.Health.Hits >= 3`, pauses Play on the tick it becomes true. *HUD* shows the rows over the Game View. The list is saved with the editor state.

## Debug Draw

Window → Debug Draw lists the debug channels of the running game with their counts. Click a channel to hide or show it, or use a preset. Physics channels follow the Physics menu.

## Crash Reports

When a panel, an extension, a native script binding or the play world panics, the editor writes a bundle to `.aego/crash/<time>/`: `info.aetext` (versions, OS, GPU, modules, message), `go.stack`, `script.stack` for script faults, `trace.json` (the last 240 frames, open in ui.perfetto.dev), `events.log`, `scene.aescene` (the play scene, unless `engine { crashScene=false }`) and `log.txt`. Paths under your home folder are written as `~`. Unsaved scenes and scripts are saved to recovery at the same moment. `engine { spikeMs=50 }` writes the same bundle, marked *spike*, for frames slower than 50 ms. Window → Crash Reports lists the bundles; click one to open the script line or copy the folder.
