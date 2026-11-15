---
id: quickstart
icon: play.start
group: start
title: Your first project
summary: Create a 2D project, attach a script, save your work and run an exported game.
order: 1
style_allow: [agent]
---
# Your first project

Create a project with a sprite you control, then export a folder you can run outside the editor. You need a desktop Aego build; the embedded browser playground is not the project editor. Start with [Install Aego](ref:install) if it is not installed.

## 1. Create a project

Open Aego. Click **New Project**, pick **2D**, name it `hello`, choose where it lives and click **Create**. The editor opens the project's main scene: a camera and a sprite.

## 2. Look around

Hierarchy on the left lists the scene's nodes. The Inspector on the right shows the selected node's components and their fields. The middle is the Scene view — drag to pan, scroll to zoom, click a node to select it. Press **Play** (or Ctrl+P): the Game view shows what a player sees. Press **Stop** to return.

## 3. Make it move

In Assets, right-click the `scripts/game` folder and choose **New → Script…**. Name it `Spinner` and click **Create**. The script editor opens with an empty script; give it an `Update`:

```aes check
package game

script Spinner {
	func Update(dt float) {
		node.SetRotation(node.Rotation() + dt)
	}
}
```

Save (Ctrl+S). Select the sprite, click **Add Component** in the Inspector, type `spinner` and choose **Spinner**. Press Play: the sprite turns. Change `dt` to `dt * 3` while the game runs and save — it spins faster without restarting. That's hot reload; it works for every script.

## 4. Add control

Replace the script with input:

```aes check
package game

import "aego/input"

script Spinner {
	func Update(dt float) {
		p := node.Position()
		p.X += input.Axis("Move") * 5 * dt
		node.SetPosition(p)
	}
}
```

`Move` is an input action — the arrows and A/D by default, a gamepad stick if one is plugged in. Actions are defined in **Project Settings → Input**; add your own there.

## Save your work

Save the script with Ctrl+S, then focus the scene document and save it too. Scripts live in `scripts/`, scenes in `scenes/`, and imported source files in `assets/`. Keep the source files and their `.meta` files together: metadata contains the stable asset identities used by scene references. Stopping Play does not turn temporary game state into saved scene edits. Export builds the saved project; it is not a replacement for saving documents.

Check: stop Play, close and reopen the scene, then press Play again. The sprite should still have Spinner attached and respond to Move.

## 5. Rewind it

While playing, press F6 to pause and drag the timeline at the bottom of the Game view. The game runs backwards to the frame you choose and forwards again — every frame, exactly as it happened. Press F7 to step one fixed tick.

## 6. Export it

**File → Export…** opens the Export panel. Pick **Web** as the target, choose an output folder and click **Export**. Click **Run**: the panel serves the folder locally and your game opens in the browser. Pick **Linux**, **Windows** or **macOS** for a native build; a system other than your own needs Go and the engine source, see [Install Aego](ref:install).

Copy the entire web output folder, rather than only its HTML or WASM file. Serve it over HTTP; opening `index.html` with `file:` does not reproduce a deployed web build. Verify movement in the served export, outside the editor. See the [export command and flags](ref:cli) for `--target web` and build checks.

## Where to go next

- [Platformer from scratch](ref:platformer) — a character, a camera, a pit, coins and an enemy, step by step.
- [Editor guide](ref:editor) — every panel and shortcut.
- [Aego Script](ref:language) — the language in one read.
- [Working with agents](ref:agents) — let an AI agent build and check scenes with you.
