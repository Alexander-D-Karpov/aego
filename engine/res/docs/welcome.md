---
id: welcome
group: editor
editor_only: true
title: Getting started with Aego
order: 0
---
# Getting started with Aego

Six short steps through the editor: create a node, attach a script, press Play, tweak a value live, hot-reload a script and fix an error from Problems. The Getting Started panel ticks them off as you go.

---
id: welcome/node
parent: welcome
order: 1
title: Create a node
---
Right-click in the Hierarchy and pick Create, or press Ctrl+Shift+N.

---
id: welcome/attach
parent: welcome
order: 2
title: Attach the Spinner script
---
Select the node and press Add Script in the Inspector, or drag scripts/hello/spinner.aes onto it.

---
id: welcome/play
parent: welcome
order: 3
title: Press Play
---
Ctrl+P or the play button in the toolbar. The scene runs in a copy; Stop throws the copy away.

---
id: welcome/tweak
parent: welcome
order: 4
title: Change Speed in the Inspector while playing
---
Values change live. Right-click a component to copy play-mode values back to the edit scene.

---
id: welcome/reload
parent: welcome
order: 5
title: Edit the script and save while playing
---
Autosave writes the file a second after you stop typing; hot reload swaps the code without restarting.

---
id: welcome/error
parent: welcome
order: 6
title: Open a script error from Problems
---
Break a line on purpose, then click the problem: the editor jumps to it and explains the fix.

---
id: welcome/done
parent: welcome
order: 99
title: All done
---
All done. The Documentation panel covers the whole language, the standard library, every engine module and hot reload; press F1 on any symbol in a script to open its entry.
