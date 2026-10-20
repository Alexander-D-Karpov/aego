---
id: aego/settings
summary: Reads player and project settings, stages and applies changes, sets session overrides and opens the settings screen.
---

`Get*` return the effective value. `Set*` only stage a value; `Apply` makes every staged value take effect at once and `Revert` drops them. `Override*` change a value for this session only and are never saved. Scripts with `OnSettingChanged(id string)` hear about every change. The documentation sandbox does not load the Settings module, so this example needs a project. See [Settings](ref:settings).

## Example

```aes check
package demo

import (
    "aego/log"
    "aego/settings"
)

script Shaker {
    @export var Strength float = 1

    var frames int

    func Ready() {
        log.Info("screen shake " + string(settings.GetFloat("accessibility.screenShake")) + ", reduced motion " + string(settings.GetBool("accessibility.reducedMotion")))
        settings.OverrideFloat("accessibility.screenShake", 0.25)
    }

    func Update(dt float) {
        frames++
        if frames == 30 {
            shake := Strength * settings.GetFloat("accessibility.screenShake")
            log.Info("shaking with " + string(shake))
            settings.ClearOverride("accessibility.screenShake")
        }
    }

    func OnSettingChanged(id string) {
        log.Info(id + " is now " + string(settings.GetFloat(id)))
    }
}
```
