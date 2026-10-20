---
id: aego/save
summary: Saves and loads games in named slots, stores per-node values in OnSave and OnLoad, and keeps game-wide progress.
---

`Save` and `Load` take effect at the end of the frame. `Put*` belong in `OnSave` and `Get*` in `OnLoad`; their keys belong to the node. `SetGlobal*` values are carried by every save. The documentation sandbox has no saves, so this example needs a project. See [Saving games](ref:save).

## Example

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
