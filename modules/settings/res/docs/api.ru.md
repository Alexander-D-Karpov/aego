---
id: aego/settings
summary: Читает настройки игрока и проекта, готовит и применяет изменения, задаёт переопределения на сеанс и открывает экран настроек.
---

`Get*` возвращают действующее значение. `Set*` только готовят значение; `Apply` вводит в действие все подготовленные значения разом, а `Revert` отбрасывает их. `Override*` меняют значение только на этот сеанс и никогда не сохраняются. Скрипты с `OnSettingChanged(id string)` узнают о каждом изменении. Песочница документации не загружает модуль настроек, поэтому этому примеру нужен проект. См. [Настройки](ref:settings).

## Пример

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
