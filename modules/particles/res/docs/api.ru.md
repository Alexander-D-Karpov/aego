---
id: aego/particles
summary: Включает и выключает декоративные частицы в текущем PlayWorld.
---

Сравните один и тот же маршрут с частицами и без них.

## Example

```aes check
package demo

import "aego/particles"

script Effects {
    func Ready() {
        particles.SetEnabled(true)
    }
}
```
