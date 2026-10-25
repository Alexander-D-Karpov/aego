---
id: language/standard-library
title: Стандартная библиотека
---
# Стандартная библиотека

Стандартная библиотека — это набор пакетов `aego/…`, которые может импортировать любой скрипт. У каждого пакета есть своя страница в справочнике Script API со всеми функциями; здесь сказано, для чего нужен каждый из них.

| Пакет | Для чего |
|---|---|
| [`aego/math`](ref:aego/math) | числа, векторы и повороты |
| [`aego/log`](ref:aego/log) | сообщения в «Консоли» |
| [`aego/rand`](ref:aego/rand) | случайные числа из зерна игры |
| [`aego/time`](ref:aego/time) | время кадра, фиксированный шаг и прошедшее время |
| [`aego/input`](ref:aego/input) | действия, контексты и клавиши напрямую; см. [Ввод](ref:input) |
| [`aego/settings`](ref:aego/settings) | настройки игрока и проекта; см. [Настройки](ref:settings) |
| [`aego/scene`](ref:aego/scene) | поиск, создание и удаление узлов |
| [`aego/testing`](ref:aego/testing) | проверки в тестах скриптов |

Модули движка добавляют свои пакеты, например `aego/physics` и `aego/audio`. Они перечислены в том же справочнике.

## Что стоит знать

- Перегрузки нет. У вариантов для Vec3 суффикс `3` (`Dot3`), у вариантов для int — суффикс `Int` (`ClampInt`).
- Собирайте сообщения журнала через `+` и `string(x)`.
- `aego/rand` берёт числа из генератора мира с зерном, поэтому повтор с тем же зерном даёт те же числа.
- Узлы, созданные через `scene.Instantiate`, появляются в конце фазы. Их скрипты получают `Ready` до следующего `Update`.
- `aego/testing` доступен только под `aego test`. Проваленная проверка останавливает тест и сообщает файл и строку.

Запустите:

```aes run
package demo

import (
    "aego/log"
    "aego/math"
    "aego/rand"
    "aego/scene"
    "aego/time"
)

script Wander {
    @export var Speed float = 2

    var dir Vec2

    func Ready() {
        dir = math.Normalize(Vec2{rand.Range(-1, 1), rand.Range(-1, 1)})
        log.Info("found myself: " + string(scene.Find("Wander") == node))
    }

    func Update(dt float) {
        node.SetPosition(node.Position() + dir * Speed * dt)
        if time.Frame() % 40 == 0 {
            p := node.Position()
            log.Info("t=" + string(time.Elapsed()) + " s, " + string(math.Length(p)) + " units from the start")
        }
    }
}
```
