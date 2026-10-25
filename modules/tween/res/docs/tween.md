---
id: tween
icon: play.start
group: gameplay
title: Tweens and timers
order: 3
---
# Tweens and timers

A tween moves one property of a node from its current value to a target over time; a timer counts down and fires. Both run during Play and in `aego run`, belong to the play world and allocate nothing per frame once created.

    import (
      "aego/curve"
      "aego/timer"
      "aego/tween"
    )

    func Hurt() coroutine {
      yield tween.ToColor(node, "SpriteRenderer.Tint", HitTint, 0.1, curve.OutQuad)
      yield tween.ToColor(node, "SpriteRenderer.Tint", Color{1, 1, 1, 1}, 0.3, curve.InQuad)
    }

---
id: tween/paths
icon: view.vectors
parent: tween
order: 1
title: Property paths
---
# Property paths

A tween names its property by a path, resolved once when the tween starts.

| Path | Property |
|---|---|
| `SpriteRenderer.Tint` | a component field; the short component name works when it is unique, otherwise write `render2d.SpriteRenderer.Tint` |
| `SpriteRenderer.Tint.a` | one lane of a vector or color: `x y z w` or `r g b a` |
| `transform.position`, `transform.rotation`, `transform.scale` | the local transform, 2D or 3D |
| `script/Player.Speed`, `script/mygame.player.Player.Speed` | an exported or private field of a script on the node |

Field names are accepted as in the schema (`tint`) or with an upper-case first letter (`Tint`). Values that tween: `float` and `int` (rounded), `Vec2`, `Vec3`, `Color`, `Rect`, `Quat` (shortest arc). Strings, enums, references, read-only fields and lists do not. A wrong path or a value of the wrong type stops the calling script with `AES6010` and returns `nil`.

Tweening the transform of a dynamic physics body is a teleport at the next physics step; move bodies with velocity instead.

---
id: tween/scripts
icon: file.script
parent: tween
order: 2
title: Tweens in scripts
---
# Tweens in scripts

A tween changes one property of a node from its current value to a target over a duration. Scripts start tweens with the functions of `aego/tween` and chain options on the result.

```aes
tween.ToFloat(node, "SpriteRenderer.Tint.a", 0, 0.5, curve.Linear).Delay(1).PingPong(-1)
```

- `ToFloat`, `ToVec2`, `ToVec3`, `ToColor` and `ToQuat` animate a property by its path. `Position` moves a 2D node.
- The easing is a preset from `aego/curve`, such as `curve.OutQuad` or `curve.OutBack`. `Curve` uses a curve field instead.
- Options such as `Delay`, `Loop`, `PingPong` and `Unscaled` go right after the call.
- `Completed` is a signal that fires once when the tween ends.

Every function and option is listed on the [aego/tween](ref:aego/tween) page.

## Rules

- When the last leg ends, the property gets exactly the end value: `to`, or the start after an even number of ping-pong legs.
- A new tween on the same property of the same node stops the running one.
- A tween stops when its node, its component or its script is gone.

---
id: tween/timers
icon: panel.profiler
parent: tween
order: 3
title: Timers
---
# Timers

    var attack timer.Timer

    func Ready() {
      attack = timer.Cooldown(0.4)
    }

    func Update(dt float) {
      if input.Pressed("Fire") && attack.Use() {
        shoot()
      }
    }

| Function | |
|---|---|
| `timer.Start(s) Timer` | a one-shot timer; it frees itself when it fires |
| `timer.New(s) Timer` | a timer that stays after it ends, so `Restart` can run it again |
| `timer.Cooldown(s) Timer` | starts ready; `Use()` takes it and starts the wait |

| Method | |
|---|---|
| `Repeat(n) Timer` | fire `n` times, `-1` until `Cancel` |
| `Unscaled() Timer` | count real time |
| `Pause()`, `Resume()`, `Restart() bool`, `Cancel()` | control |
| `Done() bool`, `Remaining() float`, `Count() int` | state |
| `Ready() bool`, `Use() bool` | cooldowns |
| `Fired(count int)` | signal, on every fire |

A timer made by a script belongs to that script's node and is cancelled when the node is destroyed. `Restart` on a one-shot timer that already fired does nothing: its slot is gone. Use `timer.New` for timers you restart.

---
id: tween/waiting
icon: status.busy
parent: tween
order: 4
title: Waiting in coroutines
---
# Waiting in coroutines

`yield` waits for a tween or a timer:

    func Die() coroutine {
      yield tween.ToFloat(node, "SpriteRenderer.Tint.a", 0, 0.2, curve.OutQuad)
      node.Destroy()
    }

    func Spawner() coroutine {
      t := timer.New(2).Repeat(-1)
      for {
        yield t
        spawn()
      }
    }

How waits resume:

- `yield t` resumes on the frame after the tween ends or is stopped, or after the timer fires or is cancelled.
- A repeating timer wakes the coroutine on every fire.
- A `nil` or finished handle resumes on the next frame.
- Woken coroutines run with the other game-time coroutines, so they wait while the game is paused even when the tween or timer itself is unscaled. In a pause menu, wait with `yield waitUnscaled(s)`.

There is no polling: a coroutine parked on a handle costs nothing until the handle wakes it.

---
id: tween/actions
icon: panel.tasks
parent: tween
order: 5
title: Actions
---
# Actions

An action runs tweens and timers in order or together:

    yield action.Sequence().
      Then(tween.Position(node, Vec2{4, 0}, 0.5, curve.OutCubic)).
      Wait(0.2).
      Then(tween.ToFloat(node, "SpriteRenderer.Tint.a", 0, 0.3, curve.Linear))

| Function or method | |
|---|---|
| `action.Sequence()`, `action.Parallel()` | a new action; it starts at the next tween update |
| `Then(tween)`, `ThenTimer(timer)`, `ThenAction(action)`, `Wait(s)` | steps, added before the action starts |
| `Stop()`, `Done()`, `Running()` | control and state |
| `Completed(finished bool)` | signal when the action ends |

A tween or timer passed to `Then` must be created in the same frame and belong to no other action; it waits for its turn. A step that ends early counts as done. `Stop` stops every unfinished step and ends the action with `finished=false`. An action is stopped when the node of the script that made it dies. A coroutine waits for an action with `yield a`; an action cannot wait for a coroutine.

---
id: tween/go
icon: code.brackets
parent: tween
order: 6
title: Go
---
# Go

    tw := sdk.TweensOf(ctx)
    h, err := tw.ToVec2(node, "transform.scale", sdk.Vec2{X: 1.2, Y: 1.2}, 0.1, sdk.TweenOptions{Ease: curve.OutBack, Legs: 2, PingPong: true})
    t, _ := tw.StartTimer(1.5, sdk.TimerOptions{Repeat: -1, Owner: node})

`TweenOptions` has `Ease`, `Curve`, `Delay`, `Legs`, `PingPong` and `Unscaled`. `TimerOptions` has `Repeat`, `Unscaled`, `Keep` and `Owner`. Handles are values; a stale handle reads as done.

The systems `tween.game` (PostUpdate, stops while paused) and `tween.unscaled` (PostUpdate, always runs) advance the pools. A module that hands out its own awaitable handles registers them with `sdk.ScriptHandle` and calls `scriptwait.Waker.Complete` when an object ends (see Native bindings in the language reference).

`OnCompleted(h, func(finished bool))` and `OnFired(h, func(count int))` connect Go code to the same signals and return a `Connection` for `Disconnect`.
