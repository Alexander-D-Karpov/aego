# Editor

## Scripts during Play

- **Autosave and reload.** With File → Autosave Scripts on (default), a script is saved one second after the last keystroke, and when the editor window loses focus. Each save that compiles is hot-reloaded into the running Play session at the start of the next frame. A save that does not compile leaves the running code untouched. The Problems panel and the red banner above the views show what is broken, and Play cannot be started again until it compiles.
- **Banner.** While a script package has errors, a banner over the Scene and Game views says how many and which package; clicking it opens the first error. The script editor's top bar shows `Scripts: build N · ok | · K errors | · latest failed`.
- **Console.** Every reload prints `Hot reload N → N+1: X migrated, Y reset, Z coroutines canceled, W handlers disconnected, … ms`. Warnings about reset fields and removed scripts follow it on the `script` channel.
- **Profiler.** During Play the Game section shows *coroutines* (active per interval) and *signal emits* graphs. The script table lists active and sleeping coroutines, resumes per frame and total cancelled, signal connections, emits and handler calls per frame, plus per-script instances, calls, time and faults.
- **Restarting coroutines.** Coroutines of a changed script are cancelled by a reload; declare `func Reloaded()` to start them again. Signal connections survive when the signal and the handler keep their names and parameter types.
