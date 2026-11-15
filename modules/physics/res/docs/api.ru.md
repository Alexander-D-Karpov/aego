---
id: aego/physics.Contact.Point
---
Хранит точку контакта в мировых координатах и единицах мира, усреднённую при нескольких точках контакта. Событие выхода сохраняет последнюю известную точку.

---
id: aego/physics.Contact.Normal
---
Направлена от другого тела внутрь тела, получающего событие столкновения; противоположный получатель получает обратное направление.

---
id: aego/physics.Contact.NormalImpulse
---
Хранит суммарный нормальный импульс контакта в единицах масса×единицы-мира/секунда. При выходе из контакта равен нулю.

---
id: aego/physics.Contact.RelativeVelocity
---
Хранит скорость этого тела относительно другого в точке Point, в единицах мира в секунду, с учётом вращения в этой точке.

---
id: aego/physics.Contact.SelfCollider
---
Ссылается на узел коллайдера получателя; он может отличаться от узла твёрдого тела, если коллайдер находится на дочернем узле.

---
id: aego/physics.Contact.OtherCollider
---
Ссылается на узел коллайдера другой стороны, не обязательно на узел её тела. Проверяйте действительность ссылки, если узел мог быть уничтожен.

---
id: aego/physics.RayHit.Hit
---
Сообщает, нашёл ли запрос коллайдер. Проверяйте перед использованием остальных полей: при промахе возвращается пустой результат.

---
id: aego/physics.RayHit.Point
---
Хранит позицию попадания в мировых координатах и единицах мира, когда Hit равен true.

---
id: aego/physics.RayHit.Normal
---
Хранит нормаль поверхности в точке попадания в мировом пространстве, когда Hit равен true.

---
id: aego/physics.RayHit.Distance
---
Хранит расстояние от начала запроса до попадания в единицах мира, а не нормированную долю длины запроса.

---
id: aego/physics.RayHit.Node
---
Ссылается на узел твёрдого тела, которому принадлежит найденная форма; Collider указывает узел самой формы.

---
id: aego/physics.RayHit.Collider
---
Ссылается на узел с формой коллайдера, в которую попал запрос; он может быть дочерним относительно тела из Node.

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
