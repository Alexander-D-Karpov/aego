---
id: aego/audio
summary: Играет клипы, звуковые события и музыку и задаёт громкость шин на сеанс.
---

Звук — это представление: он никогда не меняет состояние игры, а перемотка и повтор идут без звука. Ждите игровую логику таймером, а не `Sound`. Движок поставляет четыре клипа для тестов и набросков: `audio.Builtin("Beep")`, `Click`, `Hum` и `Melody`; последние два повторяются без шва. Пример играет гул и музыку, затем плавно гасит оба. См. [Звук](ref:audio).

## Пример

```aes run
package demo

import (
    "aego/audio"
    "aego/log"
)

script Chime {
    var tone audio.Sound
    var frames int

    func Ready() {
        tone = audio.PlayWith(audio.Builtin("Hum"), audio.PlayOptions{Volume: 0.5, Pitch: 1.5, Loop: true})
        audio.Music(audio.Builtin("Melody"), 0.5)
        audio.SetBusVolume(audio.Bus.Music, 0.6)
        log.Info("the hum plays: " + string(tone.Playing()))
    }

    func Update(dt float) {
        frames++
        if frames == 60 {
            tone.StopFade(0.2)
            audio.StopMusic(0.5)
            log.Info("the hum fades out: " + string(!tone.Playing()))
        }
    }
}
```

---
id: aego/data
summary: Типы ассетов данных, которые скрипты читают через AssetRef и TableRef.
---

Тип данных, зарегистрированный модулем или проектом, появляется здесь как структура. Поле скрипта `AssetRef<T>` хранит запись, `TableRef<T>` — таблицу; поля читаются через ссылку. Типы данных модулей несут имя модуля, например `AudioEvent`, поэтому не сталкиваются с собственными типами игры. См. [Ассеты данных](ref:data).

## Пример

```aes run
package demo

import (
    "aego/audio"
    "aego/log"
)

script Jumper {
    @export var Jump AssetRef<AudioEvent>

    func Ready() {
        if Jump == nil {
            log.Info("no jump event set")
            return
        }
        log.Info("jump volume " + string(Jump.Volume))
        audio.PlayEvent(Jump)
    }
}
```
