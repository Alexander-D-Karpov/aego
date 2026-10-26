---
id: aego/update
summary: Находит более новую версию игры на её сервере обновлений, скачивает её и перезапускает игру на ней.
---

Игра обновляет себя, если в `project.aego` указаны её сервер обновлений и ключ, которым подписаны её релизы:

```
update {
  url="https://example.com/mygame/updates"
  publicKey="<публичный ключ из aego release keygen>"
}
```

На сервере по папке на канал, `stable/release.json` и `nightly/release.json`, у каждого свой `release.json.sig`. `aego export --release-json` пишет оба файла рядом с архивом экспорта. Игра проверяет обновления при запуске; в режиме «Автоматически» она скачивает новую версию в фоне, а в окне настроек появляются **Обновления** и **Канал обновлений**. Без верной подписи и контрольной суммы ничего не устанавливается. В песочнице нет сервера обновлений, поэтому для этого примера нужен проект.

## Пример

```aes check
package demo

import (
    "aego/log"
    "aego/update"
)

script UpdateBanner {
    var shown bool

    func Update(dt float) {
        if !shown && update.State() == "ready" {
            shown = true
            log.Info("version " + update.Version() + " is ready: " + update.Notes())
        }
    }

    func Restart() {
        if update.State() == "ready" {
            update.Apply()
        }
    }
}
```
