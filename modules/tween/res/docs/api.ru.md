---
id: aego/tween
summary: Анимирует свойство узла до целевого значения за заданное время с функцией сглаживания.
---

Твин возвращает дескриптор. Сразу после создания к нему можно дописать `Delay`, `Loop`, `PingPong` или `Unscaled`. Чтобы дождаться конца, сделайте ему `yield` в корутине или подключитесь к `Completed`. Новый твин на том же свойстве заменяет запущенный. В проекте прикрепите скрипт к узлу с «Отрисовкой спрайта». См. [Твины](ref:tween).

## Пример

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
summary: Таймеры в игровом или реальном времени: одноразовые, повторяющиеся и перезарядки.
---

Таймер принадлежит узлу скрипта, который его создал, и отменяется, когда этот узел уничтожается. `yield t` ждёт следующего срабатывания, а `Fired` вызывается при каждом срабатывании. См. [Таймеры](ref:tween/timers).

## Пример

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
summary: Запускает твины, таймеры и другие действия друг за другом или одновременно.
---

Соберите действие за один кадр: `Sequence` или `Parallel`, затем `Then`, `ThenTimer`, `ThenAction` и `Wait`. Оно начинается при следующем обновлении твинов. Чтобы дождаться конца, сделайте ему `yield`. В проекте прикрепите скрипт к узлу с «Отрисовкой спрайта». См. [Действия](ref:tween/actions).

## Пример

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
