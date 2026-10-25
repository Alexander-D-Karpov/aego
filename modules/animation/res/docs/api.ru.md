---
id: aego/anim
summary: Имена и id событий анимации для OnAnimEvent и сигнала Event «Проигрывателя анимации».
---

Когда клип анимации доходит до события, скрипты на узле проигрывателя получают `OnAnimEvent(id int, payload float)`. События объявляются по имени в **Настройках проекта → Анимация**; `anim.EventName` и `anim.EventID` превращают id в имена и обратно. У неизвестного id имя `""`, у неизвестного имени id `-1`. См. [Анимация](ref:animation).

## Пример

```aes run
package demo

import (
	"aego/anim"
	"aego/log"
)

script Listener {
	var hit int

	func Ready() {
		hit = anim.EventID("Hit")
		log.Info("Hit has the id " + string(hit) + ", id 0 is called '" + anim.EventName(0) + "'")
	}

	func OnAnimEvent(id int, payload float) {
		if id == hit {
			log.Info("hit for " + string(payload))
		}
	}
}
```
