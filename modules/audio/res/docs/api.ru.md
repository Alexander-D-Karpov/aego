---
id: aego/audio.PlayOptions.Bus
---
Выбирает шину воспроизведения. Здесь Master означает шину из настроек клипа, а не принудительное направление в главную шину.

---
id: aego/audio.PlayOptions.FadeIn
---
Задаёт длительность плавного появления звука в секундах; ноль запускает без плавного перехода.

---
id: aego/audio.PlayOptions.Loop
---
Повторяет клип до явной остановки вместо завершения после одного воспроизведения.

---
id: aego/audio.PlayOptions.Pan
---
Задаёт стереобаланс от −1 (слева) через 0 (по центру) до 1 (справа).

---
id: aego/audio.PlayOptions.Pitch
---
Одновременно меняет скорость и высоту звука; 1 сохраняет исходную скорость, а 0 в этих параметрах также выбирает 1.

---
id: aego/audio.PlayOptions.Priority
---
Задаёт приоритет голоса с ограничением диапазоном 0–255. При замене голоса микшер предпочитает звуки с большим приоритетом.

---
id: aego/audio.PlayOptions.Unscaled
---
При true оставляет звук играть во время паузы игры; false связывает его с игровой паузой.

---
id: aego/audio.PlayOptions.Volume
---
Умножает усиление клипа; 1 сохраняет его, а 0 в этих параметрах также выбирает 1, а не выключает звук.

---
id: aego/data.AudioEvent.Bus
---
Направляет голоса, запущенные этим событием, в выбранную звуковую шину.

---
id: aego/data.AudioEvent.Cooldown
---
Задаёт минимальный интервал между принятыми запусками события в секундах; ноль отключает ограничение интервала.

---
id: aego/data.AudioEvent.Loop
---
Повторяет каждый запущенный голос до его остановки, в том числе при выборе варианта клипа.

---
id: aego/data.AudioEvent.MaxDistance
---
Задаёт внешнюю дистанцию затухания в единицах мира. Для пространственного звука она должна превышать MinDistance.

---
id: aego/data.AudioEvent.MaxVoices
---
Ограничивает число одновременных голосов этого события; ноль отключает отдельный лимит события. Общий лимит микшера сохраняется.

---
id: aego/data.AudioEvent.MinDistance
---
Задаёт внутреннюю дистанцию в единицах мира, до которой пространственное затухание по расстоянию ещё не начинается.

---
id: aego/data.AudioEvent.PanWidth
---
Масштабирует пространственное стереопанорамирование от 0 (по центру) до 1 (полная ширина); не меняет затухание по расстоянию.

---
id: aego/data.AudioEvent.PauseWith
---
Выбирает Game для паузы голосов вместе с игрой или Never для продолжения воспроизведения во время игровой паузы.

---
id: aego/data.AudioEvent.Pitch
---
Задаёт общую скорость и высоту звука от 0.25 до 4; 1 сохраняет исходную скорость клипа.

---
id: aego/data.AudioEvent.PitchRand
---
Задаёт долю случайного изменения высоты от 0 до 1. Каждый запуск умножает Pitch на случайное число между 1−PitchRand и 1+PitchRand.

---
id: aego/data.AudioEvent.Priority
---
Задаёт приоритет голоса в микшере от 0 до 255; при нехватке голосов предпочтение получают звуки с большим приоритетом.

---
id: aego/data.AudioEvent.Spatial
---
Включает затухание по расстоянию и панорамирование относительно слушателя вместо непространственного воспроизведения.

---
id: aego/data.AudioEvent.Steal
---
Выбирает, отклонять ли новый запуск при достижении MaxVoices или заменять существующий голос по возрасту либо громкости.

---
id: aego/data.AudioEvent.Volume
---
Задаёт исходное линейное усиление от 0 до 2 до случайного изменения и усилений клипа и шины; ноль означает тишину.

---
id: aego/data.AudioEvent.VolumeRand
---
Задаёт долю случайного уменьшения усиления от 0 до 1. При запуске усиление лежит между Volume×(1−VolumeRand) и Volume.

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
