---
id: time
title: Time
order: 3
---
# Time

Every world has its own clock with several domains:

- **Real**: wall time between frames, cut to `maxFrameDelta` (0.1 s by default) so a stalled frame or a minimized window cannot produce a huge step.
- **Game**: Real × Scale. It is 0 while paused. `time.Delta()` and `time.Elapsed()` read it.
- **Fixed**: a constant step (`fixedHz`, 60 by default). Game time fills an accumulator; each full step runs FixedUpdate and physics. At most `maxFixedSteps` (4) steps run per frame; extra steps are dropped and counted in the profiler as *dropped ticks*.
- **Editor**: the editor's own time. It never pauses with the game.

Each world counts its frames and fixed steps. `time.Frame()` and `time.Tick()` return the same numbers that the profiler and the Event Monitor show.

## Pause, step and scale

- **Pause** stops Game and Fixed time. Rendering, overlays and the editor keep running.
- **Step Frame** runs one Update and the fixed steps the accumulator holds for that frame, zero or one.
- **Step Fixed** runs exactly one fixed step (FixedUpdate, physics, collision events) and no Update.
- **Time Scale** from 0 to 8 changes how much game time passes per real second. The fixed step keeps its length, so physics behaves the same at 0.25 as at 1, only slower.

Time spent paused never piles up: resuming after a long pause runs one ordinary frame.

## Project settings

The `engine` block of `project.aego` sets the limits. A missing field keeps the default.

    engine {
      fixedHz=60.0
      maxFrameDelta=0.1
      maxFixedSteps=4
    }

## Systems

A Go system chooses what it does while paused with `Pause` in its `SystemDesc`: `PauseNormal` (stops), `PauseAlways` (runs every frame) or `PauseWhenPaused` (runs only while paused). The pause mode applies to the fixed and update phases; input and rendering always run.

## Scripts

    @pause("always")
    script PauseMenu {
      func Update(dt float) {
        if input.Pressed(input.KeyEscape) {
          time.SetPaused(!time.IsPaused())
        }
      }
    }

- `@pause("normal")` (the default): Update stops while paused.
- `@pause("always")`: Update runs every frame, paused or not, and `dt` is real time. Use it for menus and HUD.
- `@pause("whenPaused")`: Update runs only while paused.

FixedUpdate always follows fixed steps. `yield wait(s)` counts game seconds and stands still while paused; `yield waitUnscaled(s)` counts real seconds and keeps running while paused. `frames(n)` counts game frames, `fixed(n)` counts fixed steps.
