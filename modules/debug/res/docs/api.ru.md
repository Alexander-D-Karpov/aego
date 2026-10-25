---
id: aego/debug
summary: Рисует поверх игры линии, стрелки, окружности, прямоугольники, крестики и подписи для отладки.
---

Фигуры живут один кадр, если `Pen` не задаёт им время жизни. Каждая фигура принадлежит каналу, который панель «Отладочная отрисовка» включает и выключает. В песочнице документации ничего не рисуется; пример пишет в лог, что он рисует. См. [Отладочная отрисовка](ref:debug).

## Пример

```aes run
package demo

import (
    "aego/debug"
    "aego/log"
)

script Patrol {
    @export var Goal Vec2 = Vec2{4, 0}
    @export var Speed float = 2

    var frames int

    func Ready() {
        debug.Pen("ai.goal", -1)
        debug.Cross(Goal, 0.5, Color{1, 0, 1, 1})
        debug.Rect(Rect{-1, -1, 6, 2}, Color{1, 1, 1, 1})
    }

    func Update(dt float) {
        p := node.WorldPosition()
        step := Vec2{Speed * dt, 0}
        if p.X < Goal.X {
            node.SetPosition(node.Position() + step)
        }
        debug.Pen("ai.path", 0)
        debug.Arrow(p, Goal, Color{0, 1, 0, 1})
        debug.Circle(p, 0.5, Color{0, 0.6, 1, 1})
        debug.Text(p, "x " + string(p.X), Color{1, 1, 1, 1})
        frames++
        if frames % 30 == 0 {
            log.Info("drawing at x=" + string(p.X))
        }
        if frames == 90 {
            debug.Clear("ai")
            log.Info("cleared the ai channels")
        }
    }
}
```
