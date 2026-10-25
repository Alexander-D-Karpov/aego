---
id: aego/camera
summary: Выбирает виртуальные камеры и плавно переходит между ними, трясёт вид и переводит координаты между экраном и миром.
---

Виртуальные камеры — это компоненты; этот пакет переключается между ними и добавляет тряску. Тряска растёт с травмой, которая затухает со временем и масштабируется настройкой доступности. В песочнице документации ничего не рисуется; пример пишет в журнал, как затухает травма. См. [Камеры 2D](ref:camera).

## Пример

```aes run
package demo

import (
    "aego/camera"
    "aego/log"
)

script Quake {
    @export var Strength float = 0.6

    var frames int

    func Ready() {
        camera.Shake(Strength)
        log.Info("trauma " + string(camera.Trauma()))
    }

    func Update(dt float) {
        frames++
        if frames % 20 == 0 {
            log.Info("trauma " + string(camera.Trauma()))
        }
        if frames == 60 {
            camera.Shake(0.3)
            log.Info("shaken again, trauma " + string(camera.Trauma()))
        }
    }
}
```
