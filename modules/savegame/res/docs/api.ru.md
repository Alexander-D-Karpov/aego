---
id: aego/save
summary: Сохраняет и загружает игры в именованных слотах, хранит значения узлов в OnSave и OnLoad и ведёт прогресс всей игры.
---

`Save` и `Load` срабатывают в конце кадра. `Put*` вызывают в `OnSave`, а `Get*` — в `OnLoad`; их ключи принадлежат узлу. Значения `SetGlobal*` переносит каждое сохранение. В песочнице документации нет сохранений, поэтому этому примеру нужен проект. См. [Сохранение игр](ref:save).

## Пример

```aes check
package demo

import (
    "aego/log"
    "aego/save"
)

script Checkpoints {
    var reached int
    var frames int

    func Ready() {
        log.Info("runs so far " + string(save.GlobalInt("runs", 0)) + ", slot1 exists: " + string(save.Exists("slot1")))
        save.SetGlobalInt("runs", save.GlobalInt("runs", 0) + 1)
    }

    func Update(dt float) {
        frames++
        if frames == 30 {
            reached = 2
            node.SetPosition(Vec2{5, 0})
            save.Save("slot1")
            log.Info("saved at checkpoint " + string(reached))
        }
        if frames == 31 {
            log.Info("slot1 exists: " + string(save.Exists("slot1")))
        }
    }

    func OnSave() {
        save.PutInt("reached", reached)
        save.PutVec2("respawn", node.Position())
    }

    func OnLoad() {
        reached = save.GetInt("reached", 0)
        node.SetPosition(save.GetVec2("respawn", Vec2{}))
        log.Info("loaded checkpoint " + string(reached))
    }
}
```
