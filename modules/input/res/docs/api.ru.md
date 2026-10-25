---
id: aego/input
summary: Читает действия, отдельные клавиши, мышь и геймпады, управляет контекстами ввода и даёт игрокам переназначать управление.
---

Используйте действия (`Pressed("Jump")`, `Axis2("Move")`), а не отдельные клавиши: действия назначаются в проекте, игроки могут их переназначить, и они работают с любым устройством. Варианты с `P` читают одного игрока. В песочнице документации нет устройств, поэтому пример читает нулевые значения и сообщает, что устройство не использовалось; в игре тот же код двигает узел. См. [Ввод](ref:input).

## Пример

```aes run
package demo

import (
    "aego/input"
    "aego/log"
)

script Player {
    @export var Speed float = 4

    var jumps int
    var frames int

    func Update(dt float) {
        move := input.Axis2("Move")
        node.SetPosition(node.Position() + move * Speed * dt)
        if input.Pressed("Jump") {
            jumps++
        }
        if input.Down(input.KeyF1) || input.MousePressed(input.MouseLeft) {
            log.Info("cursor at " + string(input.MouseWorld().X) + ", " + string(input.MouseWorld().Y))
        }
        frames++
        if frames % 60 == 0 {
            device := input.ActiveDevice()
            if device < 0 {
                log.Info("no input yet; move " + string(move.X) + ", " + string(move.Y) + ", jumps " + string(jumps))
            } else {
                log.Info("device " + string(device) + ", jump is " + input.Glyph("Jump"))
            }
        }
    }
}
```
