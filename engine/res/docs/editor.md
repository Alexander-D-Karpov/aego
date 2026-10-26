---
id: editor
icon: panel.inspector
group: editor
title: Editor
order: 1
---
# Editor

The editor is where you build scenes, write scripts and run the game. This page tours its parts: workspaces, the theme, editing many nodes, Play and the tools that watch a running game.

![The Design workspace: Hierarchy, Scene View, Inspector, Assets and Console](shots/editor.png)

## Workspaces and layout

A workspace is a named layout: which panels are docked where and whether the toolbar is shown. The editor ships with four.

![The Programming workspace: scripts open next to the Scene View](shots/workspace-programming.png)

| Workspace | For | Differs from Design |
|---|---|---|
| Design | building scenes | the default layout |
| Programming | writing scripts | no Hierarchy, Game View or debug panels; scripts open next to the Scene View |
| Art | sprites and levels | Assets along the bottom, no Console or debug panels |
| Audio | sound work | no Hierarchy, Game View or debug panels |

- Switch with the selector at the right of the toolbar, or with `Ctrl+1` … `Ctrl+4` from any panel, the code editor included.
- Rearrange panels freely. Every workspace remembers its own layout. Documents, the selection and Play are shared between workspaces.
- **Window → Save as Workspace…** keeps the current layout under a new name. You can rename and delete your own workspaces. The built-in ones can only be reset, with **Window → Reset Layout**.
- Modules can add workspaces of their own.

### Maximize and full screen

- `Shift+Space`, a double click on a tab or **Maximize** in the tab menu fills the window with one panel. Repeat to bring the layout back.
- `F11` runs the game in full screen, with all input going to the game. `Shift+Esc` returns input to the editor and leaves full screen while the game keeps running. Stop leaves it too.
- **Zen Mode** in the command palette gives the active script the whole window.

### Undo History

**Window → Undo History** lists the edits of the active scene, one row per action. Click a row to undo or redo up to it. During Play the list shows the Play history, which is dropped on Stop.

## Theme

The editor has three themes: **System**, **Dark** and **Light**. System, the default, follows the light or dark look of the OS; when the OS does not say, the editor is dark.

![The Design workspace in the light theme](shots/editor-light.png)

- Pick a theme in **Window → Theme**, in the command palette (**Theme: System**, **Theme: Dark**, **Theme: Light**) or with the **Theme** box at the bottom of the launcher. It changes at once, without a restart.
- The Scene View, the Game View, previews and thumbnails keep the same colors in both themes, so a scene looks the same in either.
- The theme is your choice, not the project's: it applies to every project and to the launcher. `AEGO_EDITOR_THEME=system|dark|light` overrides it for one run.

## Projects, scenes and previews

The launcher has three tabs:

- **Recent** shows your projects with a picture of their startup scene.
- **New Project** shows the templates with a preview and the files they contain. Pick a name, a folder, a module path and the workspace the editor opens with.
- **Examples** copies one of the engine examples into a folder of your choice.

The **Scenes** panel lists every scene of the project with a preview. The startup scene carries a play mark. Right-click a scene to open it, make it the startup scene, rename, duplicate or delete it, or show it in Assets.

Assets shows a preview for every file: textures on a checkerboard, scenes and prefabs as small renders, scripts with their first lines and data assets with their type. The **S M L** buttons change the tile size. Previews are rebuilt when the file or anything it uses changes.

## Editing many nodes

Select several nodes and the Inspector shows the components they share. A field whose values differ shows "—"; hover its name to see the values. A change goes to every selected node as one Undo step. Clicking, typing or pasting on more than ten nodes asks first.

Component menus copy and paste values, and the Node card copies all components at once. Pasting onto a node without the component adds it.

### Presets

**Save as Preset…** in a component or script menu writes `assets/presets/<name>.aepreset`. **Apply Preset** lists the presets of that type and applies one to every selected node.

### Reset

Right-click a field and pick **Reset to Default**, or **Reset to Prefab Value** on a prefab override. Data assets with a base offer **Reset to Base Value**.

### Lock

The padlock at the top of the Inspector keeps it on the current nodes while you select others.

### Favorites

Add assets from their menu, nodes from the Hierarchy menu and actions with `Ctrl+D` in the command palette. Favorites show in the Favorites panel and first in the palette.

### Selection history

`Alt+←` and `Alt+→` step through earlier selections, switching scenes when needed.

### Isolation

`Shift+H` shows only the selected nodes and their children in the Scene View. Press it again, or pick **Show All**, to leave.

## Keeping Play changes

When you stop Play after changing values in the Inspector or with a gizmo, the editor lists those changes, already checked. Each one shows the value you set, even if the game moved the node afterwards. Stopping without edits shows nothing.

- **Show all differences** adds everything else the game changed, such as positions and counters. These rows start unchecked and are marked "from gameplay". The editor remembers this choice.
- **Apply to scene** copies the checked values into the scene as one Undo step.
- **Play → Never Show Play Changes** turns the list off; **Always Show Play Changes** shows it after every stop.

## Assets and project health

### Find References

**Find References** (`Shift+F12` or the asset menu) lists every scene, prefab and data asset, and the project file, that use an asset, with the node, component and field. Click a line to open it.

- **Replace with…** points every reference at another asset of the same kind.
- **Clear References** sets them to None.

Open documents change through Undo. Files that are not open are rewritten on disk, and their originals are kept in `.aego/recovery/`.

### Safe Delete

Deleting an asset that is still used lists what uses it. Show the references to replace them first, delete anyway, which leaves the references missing, or cancel.

### Missing References

**Missing References** lists references to assets that no longer exist, grouped by asset, with the path each one had. Use the suggested asset with the same name, pick another, or clear the reference. Missing node references inside scenes and prefabs can be selected or cleared. References that scripts make by a string path are not found.

### Batch Rename

**Batch Rename** works on several assets (asset menu) or nodes (Hierarchy menu). It takes a pattern with `{name}`, `{n}`, `{n:03}`, `{parent}` and `{ext}`, find and replace (optionally with a regular expression) and case. The preview marks names that clash. Renamed assets keep their ids, so nothing that uses them breaks.

### Diff

**Compare with HEAD** in the menu of a scene, prefab or data asset shows what changed since the last commit, or since an earlier commit you pick. It lists added, removed, moved and renamed nodes, components and fields. Click a node change to select the node. The project must be in a git repository.

### Tasks

Imports, previews, reference updates, diffs and checks run in the background. The status bar shows how many are running. Click it for the Tasks panel, where you can cancel a task and read the error of a failed one.

### Project Health

**Project Health** checks import errors, problems, missing references, unused assets, scenes nothing loads, names that differ only in case, textures larger than 4096 px, scenes without a camera and, when `project.aego` has an `art` palette, pixel-art sprites drawn in other colors. Every scene, prefab and script, the startup scene and `assets/presets` count as used. The panel also shows how long the current scene took to open, its node count and how many of its assets are missing.

## Scripts during Play

![A script open in the code editor next to the scene](shots/script.png)

With **File → Autosave Scripts** on, the default, a script is saved a second after you stop typing and when the editor loses focus. Each save that compiles is loaded into the running game at the next frame. A save that does not compile leaves the running code as it was.

- While a script package has errors, a banner over the Scene View and the Game View says how many and where. Click it to open the first error. Play cannot start again until the scripts compile.
- The Console reports each reload, with warnings about fields that were reset and scripts that were removed.
- During Play the Profiler shows coroutines and signal emits, and a table of scripts with their calls, time and faults.

[Hot reload](ref:language/hot-reload) explains what survives a reload and how to restart coroutines.

## Rewinding Play

![Play mode: the Game View with the timeline under it](shots/play.png)

While a game runs in the editor, every frame is recorded: its time step, time scale, pause and the input the game saw. The bar at the bottom of the Game View shows the recorded frames.

- Drag the bar to scrub through them. The Game View follows the handle while you drag.
- `Ctrl+Left` and `Ctrl+Right` move one frame back or forward.
- After a rewind the game waits paused at that frame. Play or Step starts a new timeline from there and forgets the frames after it.

The editor keeps copies of the game along the timeline, so rewinding is quick even in long sessions. When a module keeps state that cannot be copied, the bar says "keyframes off" with its name. Rewinding then restarts the game from the scene as it was when Play started, and dragging back moves the cursor and rewinds when you release.

While the editor holds the game paused, nothing runs and nothing is recorded. A pause the game makes itself, such as a pause menu, keeps running and recording. Saves the game makes during playback are not written to disk again.

### What ends the recording

- An edit in the running scene ends the recording at the frame you are on. Earlier frames can still be rewound to. An edit made before the first frame becomes the new start.
- An asset or script reload, a tuned data asset or a changed setting drops the whole recording, because a replay would run with the new values.
- When the history reaches its memory limit, the oldest part is dropped and the bar starts later.
- A module that declares itself non-deterministic turns rewinding off, and the bar names it.

Selections, watches and the Event Monitor follow the rewound game. Node ids stay the same, and events from playback are hidden unless **Show replayed** is on in the Event Monitor.

## Event Monitor

**Window → Event Monitor** lists what happened in the running game: script signals, collisions and triggers, script faults, nodes created and destroyed, scene changes, audio and gameplay events such as damage, deaths and pickups.

- Every row carries the frame and the fixed step, the same numbers as the Profiler. Clicking a frame in the Profiler's strip filters the monitor to it.
- Click a row to select its node.
- Engine events are listed for nodes with a handler. With **Selected nodes** on, every event of the selected nodes is listed.
- **Export** writes the filtered rows to `.aego/user/events.log`.
- **Break on** pauses Play when an event of a listed kind matches the text after it, for example `OnTriggerEnter`. The pause lands on the fixed step of the event.

## Watch

**Window → Watch** shows live values such as `Player.PlayerController.Velocity`, `Enemy.EnemyAI.Speed` or `time.Tick`.

- A row is a node path from the scene root, then a component or script name, then a field. `.x .y .z .w` pick one part of a vector.
- `Ctrl+Alt+W` in the Inspector adds the field under the mouse.
- A row with a comparison, such as `Player.Health.Hits >= 3`, pauses Play on the fixed step it becomes true.
- **HUD** shows the rows over the Game View.

## Debug Draw

**Window → Debug Draw** lists the debug channels of the running game with their counts. Click a channel to hide or show it, or use a preset. Physics channels follow the Physics menu.

## Crash Reports

When a panel, an extension, a native script binding or the running game fails, the editor saves the open scenes and scripts to recovery and writes a report to `.aego/crash/<time>/`. The report holds the versions, OS, GPU and modules, the stacks, the last frames as a trace you can open in ui.perfetto.dev, recent events, the play scene and the log. Paths under your home folder are written as `~`.

- `engine { crashScene=false }` in the project leaves the play scene out of the report.
- `engine { spikeMs=50 }` writes the same report, marked *spike*, for frames slower than 50 ms.
- **Window → Crash Reports** lists the reports. Click one to open the script line or copy the folder.

---
id: editor/add-component
icon: ui.plus
parent: editor
order: 1
title: Add Component and Create
---
# Add Component and Create

One palette adds components and creates nodes. It finds an item from a few typed letters and shows what comes with it before you add it.

![The palette: "col" lists the colliders, the right pane describes the highlighted one](shots/add-palette.png)

- `Ctrl+Shift+A`, or **Add Component** in the Inspector, adds to the selected nodes.
- `Ctrl+Shift+N`, **Create…** in the Hierarchy or Scene View menus, or the **+** buttons create a node. **Create Empty** is `Alt+Shift+N`.

## Search

The search field takes the keys at once, so `Ctrl+Shift+A`, `mot`, `Enter` adds Motion 2D without the mouse. The query matches, in this order:

1. the start of the name: `mot` finds Motion 2D;
2. initials: `vc` finds Virtual Camera 2D, `cf2` finds Camera Follow 2D;
3. the start of any word: `fol`;
4. any part of the name: `box` finds Box Collider 2D, then Hitbox 2D and Hurtbox 2D.

Within each kind of match, items you add more often come first.

| Key | Does |
|---|---|
| ↑ ↓, PgUp PgDn, Home End | move the highlight |
| Enter | adds the highlighted item, the first one by default |
| Shift+Enter | adds it and keeps the palette open |
| Tab | switches the alternative of a requirement |
| Ctrl+D | stars or unstars the item |
| Esc | closes |

With an empty query the palette lists the eight items used last, your starred items, then the categories. Categories are folded, except those you used, and the palette remembers which are open.

## What comes with a component

The right pane shows the component's documentation and what comes with it:

- **Also adds** lists the components it requires. They are added first, in the same step.
- **Needs one of** appears when any of several components will do. The first is picked; `Tab` or a click picks another.
- **Also add…** check boxes offer components that usually go with it, such as a collider for Motion 2D. They start unchecked.
- Items that cannot be added are grey, with the reason: already on the node, needs a 2D node, one per scene, or two components that drive the same node.

With several nodes selected, the component goes on every node it fits, and the pane names the nodes it skips. Everything one pick adds is one Undo step. Project scripts are listed under Scripts with their package on the right.

---
id: editor/agent
icon: panel.agent
parent: editor
order: 2
title: The Agent panel
style_allow: [agent]
---
# The Agent panel

The Agent tab, next to the Inspector, talks to Claude Code about the open project. It answers questions, reads the scenes, the scripts and the documentation, and proposes changes that you apply.

The model uses the same tools as `aego` and [`aego mcp`](ref:agents), on the editor's open documents. It sees unsaved edits, and what it changes in an open scene or script becomes an Undo step there instead of a write to disk.

## Setting up

1. Install Claude Code, the `claude` command.
2. Sign in once by running `claude` in a terminal.

The panel looks for `claude` on your PATH. When it is not there, the panel says so and takes the path of the program; **Check Again** looks once more. Claude Code only gets the aego tools: no shell and no file access of its own.

## Asking

Type in the field at the bottom and press `Enter`. While the agent answers, **Send** turns into **Stop**, which ends the answer and rejects whatever still waits. Under the field the panel shows what the last answer cost and how long it took.

- Type `@` to pick a node of the open scene or an asset. It goes in as a chip; the agent gets the node's id and scene, or the asset's path.
- Right-click a node in the Hierarchy and pick **Ask Agent…**, or a problem in Problems and pick **Ask Agent to Fix**. The text waits in the field until you send it.
- With an empty transcript, the panel offers three examples; a click sends one.

## Steps

Each tool call is a row: an icon, what the agent did and what it found, such as **Read scene main.aescene · 48 nodes**. Steps that only read fold into one row, **Looked around · 9 steps**; open it to see each step, and open a step to see its result laid out: search results, a node tree, fields with their values, assets, components by category, a log. A click on a result goes to it: a page of the documentation, a node, an asset.

While the agent works, the last row says what it is doing, **Reading the project…** or **Preparing a change…**. A step that failed shows its error with **Retry** and **Ask Agent to Fix**.

## Questions

When there is more than one way to do something, the agent asks with a card of options, the one it recommends first. Click an option or press its number. Type an answer of your own in **Something else…** and press **Answer**; when the agent lets you pick several options, tick them first. The agent waits for the answer and goes on with it.

## Changes

Every change the agent wants becomes a card, grouped by node: new nodes and components, and each field with its value before and after, by the names the Inspector uses.

- **Apply** makes the change, on the scene as it is now. **Skip** tells the agent no. **Dry-run** shows everything the dry run reported, with its notes and problems. A card nobody answers is skipped after ten minutes.
- With **Auto-apply** on, cards apply at once, except removals, moves, deletes and reference fixes. Those always wait for you and carry a warning sign.
- When several cards wait, **Apply All** above the first one applies them and the rest of that answer's cards the same way. It stops when the answer ends.
- An applied card folds to one line with a check mark; open it for **Undo** and **Select**, which selects the nodes it changed.

The agent is asked to send the edits of one scene together, so a new node with its components and fields is one card.

## Following the agent

The eye button in the header, **Window → Follow the Agent** or `Ctrl+Alt+F` turns following on. While it is on, the editor shows each step where it happens:

![Following the agent: its marker on Player in the Hierarchy and the Scene View, its own selection outline, and the Inspector at Motion 2D](shots/agent-follow.png)

- A scene step opens the scene, selects the node with an outline of its own in the agent's colour, frames it in the Scene View and shows it in the Inspector at the component. Your own selection stays as it was, and **Delete** and **Copy** still act on it.
- A changed field lights up for two seconds, a new node or component lights whole, and a removed node fades where it was.
- A script step opens the script at its line. The lines a waiting card would change show their new text as ghost lines; once applied, they stay marked in the gutter for a while.
- A run or a check brings Problems or the Console forward, with the new entries lit.

The agent's avatar marks where it works: on the node in the Hierarchy, over it in the Scene View, on the component in the Inspector and on the lines in the code editor. The row under the header says where it is; a click goes there.

Any click, key or scroll of yours outside the Agent panel pauses following, so the editor never moves under your hands. **Resume Following** goes on at once; after 30 seconds without input it goes on by itself.

When you edit a field the agent is about to change, the Inspector says so, and your change is made again after the agent's, as its own Undo step.

The dots above the message field are the steps of the session, coloured by what they did. Point at one for what it changed, click it to go there, and right-click a change for **Undo to Here**.

## Scripts

The agent reads scripts and changes them with cards too. An open script changes in its buffer, as one Undo step of the code editor, and is saved like your own edits; a closed one changes on disk. A new script is a card as well, and **Undo** on it removes the file.

## Undo

Each applied change is one Undo step of the scene you edit, named `Agent: …`. A change to a project setting, such as gravity, is a step too, and Undo puts the old setting back.

- **Undo** on a card works while its step is still the last one. After other edits, use **Edit → Undo** or the Undo History.
- Changes made during Play go to the running game, like your own edits in Play: the agent's changes show in **Play changes** when you stop, where you keep or drop them.

## Sessions

The header shows the session by its first message, and what the agent does: idle, thinking, working or waiting for you. The menu next to **Auto-apply** has **New Session**, **History…**, **Plan Only** and **Stop**.

- **Plan Only** keeps the agent from changing anything: it describes the changes and their order, and you turn the plan into changes when you turn it off.
- Each conversation is written to `.aego/user/agent/<date-time>.jsonl`, one line per message, tool call, result and card state. **History…**, or a click on the session name, opens an earlier session read-only. A new message always goes to the current session.

## Context

The gauge in the header shows how much of the model's context window the session uses; it turns yellow past 80 % and near the point where the agent compacts its history. A click opens the layout:

![The Context popup: the window as one bar, the parts of the session with their tokens, the Scene part open with Drop and Pin](shots/agent-context.png)

- The total is what the model read for its last answer, as the CLI reports it. The parts are the editor's estimate of what it put there, scaled to that total: **Messages** (yours and the agent's), **Scene** (scene trees and nodes), **Code** (scripts the agent read), **Docs**, **Project & schema**, **Tool results** (runs, checks, changes), the editor's **Instructions** and **Tool definitions**, and the CLI's own **System prompt**. **Compaction buffer** is the room the CLI keeps; the history is compacted when the context reaches it, and the transcript says so.
- A part opens to its contributions, largest first. **Drop** leaves one out: the next message starts the session again with a shortened history, where your messages and the agent's answers stay, results become one-line summaries and dropped ones are gone. **Pin** keeps a result in full in that history.
- **Send with every message** adds the open scene and the selection to each message, so the agent knows what you look at.
- **Read limits** keep large reads small: a scene tree up to a number of nodes and levels, with the rest counted, a documentation search up to a number of results, and a script longer than the limit read in parts. They are kept in `prefs.aetext`.
- **New Session with Summary** asks the agent for a summary of the session, starts a new one and sends the summary, with the names of the nodes the session changed, with your next message.

