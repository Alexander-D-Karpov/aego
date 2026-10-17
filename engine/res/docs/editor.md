---
id: editor
title: Editor
order: 2
---
# Editor

## Scripts during Play

- **Autosave and reload.** With File → Autosave Scripts on (default), a script is saved one second after the last keystroke, and when the editor window loses focus. Each save that compiles is hot-reloaded into the running Play session at the start of the next frame. A save that does not compile leaves the running code untouched. The Problems panel and the red banner above the views show what is broken, and Play cannot be started again until it compiles.
- **Banner.** While a script package has errors, a banner over the Scene and Game views says how many and which package; clicking it opens the first error. The script editor's top bar shows `Scripts: build N · ok | · K errors | · latest failed`.
- **Console.** Every reload prints `Hot reload N → N+1: X migrated, Y reset, Z coroutines canceled, W handlers disconnected, … ms`. Warnings about reset fields and removed scripts follow it on the `script` channel.
- **Profiler.** During Play the Game section shows *coroutines* (active per interval) and *signal emits* graphs. The script table lists active and sleeping coroutines, resumes per frame and total cancelled, signal connections, emits and handler calls per frame, plus per-script instances, calls, time and faults.
- **Restarting coroutines.** Coroutines of a changed script are cancelled by a reload; declare `func Reloaded()` to start them again. Signal connections survive when the signal and the handler keep their names and parameter types.

## Event Monitor

Window → Event Monitor lists what happened in the running game: script signals, engine events (collisions, triggers), script faults, nodes created and destroyed, scene changes. Every row carries the world frame and the fixed tick, the same numbers as the profiler; clicking a frame in the profiler's strip filters the monitor to it. Click a row to select its node. Engine events are listed for nodes with a handler; with *Selected nodes* on, every event of the selected nodes is listed, handler or not. Export writes the filtered rows to `.aego/user/events.log`.

*Break on* pauses Play when a listed kind matches the text after it, for example `OnTriggerEnter`. The pause lands on the tick of the event: the rest of that frame does not run.

## Watch

Window → Watch shows live values: `Player.PlayerController.Velocity`, `Player.PlayerController.Velocity.y`, `Enemy.EnemyAI.Speed`, `time.Scale`, `time.Tick`. The node part is a path from the scene root, then a component or script name, then a field; `.x .y .z .w` pick one lane. Ctrl+Alt+W in the Inspector adds the field under the mouse. A row with a comparison, like `Player.Health.Hits >= 3`, pauses Play on the tick it becomes true. *HUD* shows the rows over the Game View. The list is saved with the editor state.

## Debug Draw

Window → Debug Draw lists the debug channels of the running game with their counts. Click a channel to hide or show it, or use a preset. Physics channels follow the Physics menu.

## Crash Reports

When a panel, an extension, a native script binding or the play world panics, the editor writes a bundle to `.aego/crash/<time>/`: `info.aetext` (versions, OS, GPU, modules, message), `go.stack`, `script.stack` for script faults, `trace.json` (the last 240 frames, open in ui.perfetto.dev), `events.log`, `scene.aescene` (the play scene, unless `engine { crashScene=false }`) and `log.txt`. Paths under your home folder are written as `~`. Unsaved scenes and scripts are saved to recovery at the same moment. `engine { spikeMs=50 }` writes the same bundle, marked *spike*, for frames slower than 50 ms. Window → Crash Reports lists the bundles; click one to open the script line or copy the folder.
