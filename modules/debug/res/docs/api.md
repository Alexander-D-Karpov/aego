---
id: aego/debug
summary: Draws lines, arrows, circles, rectangles, crosses and labels over the game for debugging.
---

Shapes last one frame unless `Pen` gives them a lifetime, and belong to a channel that the Debug Draw panel turns on and off. In the documentation sandbox nothing is drawn; the example logs what it draws. See [Debug Draw](ref:debug).

## Example

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
