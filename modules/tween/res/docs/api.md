---
id: aego/tween
summary: Animates a property of a node to a target value over time with an easing.
---

A tween returns a handle: chain `Delay`, `Loop`, `PingPong` or `Unscaled` right after creating it, `yield` it in a coroutine to wait for the end, or connect to `Completed`. A new tween on the same property replaces the running one. In a project, attach it to a node with a Sprite Renderer. See [Tweens](ref:tween).

## Example

```aes run
package demo

import (
    "aego/curve"
    "aego/log"
    "aego/tween"
)

script Pickup {
    var pulse tween.Tween

    func Ready() {
        pulse = tween.ToVec2(node, "transform.scale", Vec2{1.2, 1.2}, 0.25, curve.InOutSine).PingPong(-1)
        start Fly()
    }

    func Fly() coroutine {
        yield tween.Position(node, Vec2{3, -1}, 0.5, curve.OutCubic)
        log.Info("arrived at " + string(node.Position().X) + ", " + string(node.Position().Y))
        fade := tween.ToFloat(node, "SpriteRenderer.Tint.a", 0, 0.5, curve.Linear).Delay(0.25)
        fade.Completed.Connect(OnFaded)
        yield fade
        pulse.Stop()
        log.Info("pulse running: " + string(pulse.Running()))
    }

    func OnFaded(finished bool) {
        log.Info("fade finished: " + string(finished))
    }
}
```

---
id: aego/timer
summary: Timers in game or real time: one-shot, repeating and cooldowns.
---

A timer belongs to the node of the script that made it and is cancelled when that node is destroyed. `yield t` waits for the next fire, and `Fired` runs on every fire. See [Timers](ref:tween/timers).

## Example

```aes run
package demo

import (
    "aego/log"
    "aego/timer"
)

script Turret {
    var reload timer.Timer
    var waves timer.Timer
    var shots int

    func Ready() {
        reload = timer.Cooldown(0.3)
        waves = timer.New(0.5).Repeat(3)
        waves.Fired.Connect(OnWave)
        start Countdown()
    }

    func Update(dt float) {
        if reload.Use() {
            shots++
        }
    }

    func OnWave(count int) {
        log.Info("wave " + string(count) + ", shots so far " + string(shots) + ", reload ready in " + string(reload.Remaining()))
    }

    func Countdown() coroutine {
        yield timer.Start(1.8)
        log.Info("waves done: " + string(waves.Done()) + ", fired " + string(waves.Count()) + " times")
    }
}
```

---
id: aego/action
summary: Runs tweens, timers and other actions one after another or at the same time.
---

Build the action in one frame: `Sequence` or `Parallel`, then `Then`, `ThenTimer`, `ThenAction` and `Wait`. It starts at the next tween update; `yield` it to wait for the end. In a project, attach it to a node with a Sprite Renderer. See [Actions](ref:tween/actions).

## Example

```aes run
package demo

import (
    "aego/action"
    "aego/curve"
    "aego/log"
    "aego/timer"
    "aego/tween"
)

script Door {
    func Ready() {
        start Open()
    }

    func Open() coroutine {
        shake := action.Parallel().
            Then(tween.ToFloat(node, "transform.rotation", 0.1, 0.1, curve.OutQuad).PingPong(4)).
            ThenTimer(timer.Start(0.3))
        a := action.Sequence().
            ThenAction(shake).
            Wait(0.2).
            Then(tween.Position(node, Vec2{0, -2}, 0.5, curve.InOutCubic))
        a.Completed.Connect(OnOpened)
        log.Info("opening, running: " + string(a.Running()))
        yield a
        log.Info("door at y=" + string(node.Position().Y) + ", done: " + string(a.Done()))
    }

    func OnOpened(finished bool) {
        log.Info("action finished: " + string(finished))
    }
}
```
