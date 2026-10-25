---
id: aego/physics
summary: Запросы лучом, отрезком и на пересечение к 2D-миру физики, поиск слоёв и гравитация мира.
---

Запросы работают, только пока идёт физика: во время игры и в `aego run`. Вне её каждая функция останавливает скрипт с `AES6010`. Функции `Overlap*` и `RaycastAll` заполняют переданный вами срез, не больше его длины, и возвращают, сколько элементов записали. `Layer` и `LayerMask` принимают слой, объявленный в **Настройках проекта → Физика**, и останавливают скрипт при неизвестном имени; маска `-1` означает все слои. В песочнице документации нет мира физики, поэтому пример выполняет все запросы в `OnCollisionEnter`. Этот метод вызывается, когда у узла есть «Тело 2D» и коллайдер и он касается другого тела. См. [Физика 2D](ref:physics).

## Пример

```aes run
package demo

import (
    "aego/log"
    "aego/physics"
)

script Grenade {
    @export var Radius float = 2

    var hits []NodeRef
    var exploded bool

    func Ready() {
        for i := 0; i < 16; i++ {
            hits = append(hits, nil)
        }
        log.Info("waiting for a collision; this node needs a Rigid Body 2D and a collider")
    }

    func OnCollisionEnter(other NodeRef, contact physics.Contact) {
        if exploded {
            return
        }
        exploded = true
        log.Info("hit " + other.Name() + " with vertical speed " + string(contact.RelativeVelocity.Y))
        n := physics.OverlapCircle(contact.Point, Radius, hits)
        for i := 0; i < n; i++ {
            wall := physics.Linecast(contact.Point, hits[i].WorldPosition(), -1)
            if !wall.Hit {
                log.Info("blast reaches " + hits[i].Name())
            }
        }
        down := physics.Raycast(node.WorldPosition(), Vec2{0, 1}, 5)
        if down.Hit {
            log.Info("ground " + string(down.Distance) + " units below, gravity " + string(physics.Gravity().Y))
        }
    }
}
```
