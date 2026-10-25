---
id: aego/builtin
summary: Методы встроенного типа `NodeRef`: иерархия, теги, включение и 2D- и 3D-трансформации.
---

Каждый скрипт обращается к своему узлу через неявное поле `node`, а к другим узлам — через поля `NodeRef`, `Find` и `Child`. Трансформации заданы относительно родителя, читаются и записываются по значению; 2D-метод на 3D-узле вызывает ошибку. О компонентах и ссылках на скрипты см. [Узлы, компоненты и ссылки](ref:language/nodes-components-and-references).

## Пример

```aes run
package demo

import (
    "aego/log"
    "aego/math"
)

script Door {
    @export var OpenAngle float = 90

    var frames int

    func Ready() {
        log.Info("node " + node.Name() + ", 2D: " + string(node.Space() == 0) + ", top level: " + string(node.Parent() == nil))
        node.AddTag("Interactable.Door")
        log.Info("is interactable: " + string(node.HasTag("Interactable")))
        node.SetScale(Vec2{-1, 1})
        t := node.Transform2D()
        t.Position.X += 2
        node.SetTransform2D(t)
        log.Info("moved to " + string(node.Position().X) + ", mirrored: " + string(node.Scale().X < 0))
    }

    func Update(dt float) {
        frames++
        target := math.Deg2Rad(OpenAngle)
        node.SetRotation(math.Min(node.Rotation() + 2 * dt, target))
        if frames % 30 == 0 {
            log.Info("angle " + string(math.Round(math.Rad2Deg(node.Rotation()))) + " deg")
        }
        if frames == 100 {
            removed := node.RemoveTag("Interactable.Door")
            node.SetEnabled(false)
            log.Info("tag removed: " + string(removed) + ", enabled: " + string(node.Enabled()))
        }
    }
}
```

---
id: aego/math
summary: Математика чисел с плавающей точкой, целых чисел, векторов и поворотов: тригонометрия, ограничение, интерполяция, функции для Vec2 и Vec3 и кватернионы.
---

Перегрузки нет: у вариантов для Vec3 есть суффикс `3` (`Length3`, `Dot3`), у целочисленных — суффикс `Int` (`MinInt`, `ClampInt`). Углы задаются в радианах; `Deg2Rad` и `Rad2Deg` переводят их. Векторная арифметика (`v + w`, `v * s`) входит в язык и не требует импорта.

## Пример

```aes run
package demo

import (
    "aego/log"
    "aego/math"
)

script Orbit {
    @export var Radius float = 3
    @export var Center Vec2 = Vec2{1, 1}

    var angle float
    var frames int

    func Ready() {
        to := Vec2{3, 4}
        log.Info("length " + string(math.Length(to)) + ", heading " + string(math.Round(math.Rad2Deg(math.Angle(to)))) + " deg")
        log.Info("unit " + string(math.Normalize(to).X) + ", " + string(math.Normalize(to).Y))
        log.Info("clamp 12 to 0..10: " + string(math.Clamp(12, 0, 10)))
        log.Info("ammo after pickup: " + string(math.MinInt(25 + 10, 30)))
    }

    func Update(dt float) {
        angle += math.Tau / 2 * dt
        frames++
        if frames % 30 == 0 {
            p := Center + Vec2{math.Cos(angle), math.Sin(angle)} * Radius
            node.SetPosition(p)
            log.Info("frame " + string(frames) + ": at " + string(math.Round(p.X * 100) / 100) + ", " + string(math.Round(p.Y * 100) / 100) + ", " + string(math.Dist(p, Center)) + " from center")
        }
    }
}
```

---
id: aego/log
summary: Пишет сообщения в Консоль на четырёх уровнях: Debug, Info, Warn и Error.
---

Собирайте сообщения через `+` и `string(x)`. Ни одна из функций не останавливает скрипт; `Error` нужен для проблем, после которых скрипт может продолжить работу. Сообщения Debug скрыты в Консоли редактора, если он не запущен с `-debug`; вывод «Запустить» ниже показывает все уровни.

## Пример

```aes run
package demo

import "aego/log"

script Health {
    @export var Max int = 3

    var hp int
    var frames int

    func Ready() {
        hp = Max
        log.Info(node.Name() + " starts with " + string(hp) + " hp")
        log.Debug("Debug lines are hidden in the editor unless it runs with -debug")
    }

    func Update(dt float) {
        frames++
        if frames % 30 != 0 || hp == 0 {
            return
        }
        hp--
        if hp == 0 {
            log.Error("died on frame " + string(frames))
        } else if hp == 1 {
            log.Warn("one hit left")
        } else {
            log.Info("hit, " + string(hp) + " hp left")
        }
    }
}
```

---
id: aego/rand
summary: Случайные числа из генератора мира с зерном, поэтому повтор с тем же зерном даёт те же числа.
---

`Int(min, max)` и `Range(min, max)` включают `min` и не включают `max`. Все скрипты мира берут числа из одного генератора, поэтому числа, которые получает скрипт, зависят от порядка вызовов в кадре. Песочница документации всегда запускается с зерном 1.

## Пример

```aes run
package demo

import (
    "aego/log"
    "aego/rand"
)

script Loot {
    @export var DropChance float = 0.5

    func Ready() {
        log.Info("seed " + string(rand.Seed()))
        for i := 0; i < 5; i++ {
            roll := rand.Int(1, 7)
            if rand.Float() < DropChance {
                gold := rand.Int(5, 20)
                log.Info("chest " + string(i) + ": rolled " + string(roll) + ", drops " + string(gold) + " gold")
            } else {
                log.Info("chest " + string(i) + ": rolled " + string(roll) + ", empty")
            }
        }
        delay := rand.Range(0.5, 1.5)
        log.Info("next spawn in " + string(delay) + " s, facing left: " + string(rand.Bool()))
    }
}
```

---
id: aego/time
summary: Читает часы мира и управляет ими: время кадра, прошедшее игровое время, счётчики кадров и тиков, пауза и масштаб времени.
---

`Delta` и `Elapsed` считают игровое время, которое следует масштабу времени и стоит на паузе; `Unscaled` и `UnscaledElapsed` считают реальное время. `SetPaused` и `SetScale` меняют часы идущей игры; в песочнице документации нет часов мира, поэтому их вызов там останавливает скрипт, и пример только читает часы. См. [Время](ref:time).

## Пример

```aes run
package demo

import (
    "aego/log"
    "aego/math"
    "aego/time"
)

script Bob {
    @export var Amplitude float = 0.5

    func Update(dt float) {
        y := math.Sin(time.Elapsed() * 3) * Amplitude
        node.SetPosition(Vec2{0, y})
        if time.Frame() % 30 == 0 {
            log.Info("frame " + string(time.Frame()) + ", elapsed " + string(time.Elapsed()) + " s, delta " + string(time.Delta()))
        }
        if time.Frame() == 60 {
            log.Info("scale " + string(time.Scale()) + ", paused " + string(time.IsPaused()) + ", fixed step " + string(time.Fixed()))
            log.Info("real time since start " + string(time.UnscaledElapsed()) + " s")
        }
    }
}
```

---
id: aego/scene
summary: Находит, создаёт и уничтожает узлы идущей сцены: поиск по пути, id, тегу, компоненту и области, экземпляры префабов и пулы.
---

`Find` принимает путь от корня сцены. Функции `With*` и `InArea` заполняют переданный вами срез, не больше его длины, и возвращают, сколько узлов они записали. Новые и уничтоженные узлы вступают в силу в конце фазы. `Spawn`, `SpawnAt`, `Pool` и `Despawn` входят в модуль «Создание объектов», который песочница документации не загружает; `Instantiate` нужен префаб, назначенный в Инспекторе, поэтому пример пропускает его, когда поле пустое.

## Пример

```aes run
package demo

import (
    "aego/log"
    "aego/scene"
)

script Crate {
    func Ready() {
        node.SetPosition(Vec2{1, 0})
        node.AddTag("Breakable")
    }
}

script Level {
    @export var Coin AssetRef<Prefab>

    var found []NodeRef
    var frames int

    func Ready() {
        for i := 0; i < 8; i++ {
            found = append(found, nil)
        }
        if Coin != nil {
            coin := scene.Instantiate(Coin, node)
            coin.SetPosition(Vec2{0, -2})
        } else {
            log.Info("Coin prefab is not set, nothing to spawn")
        }
    }

    func Update(dt float) {
        frames++
        if frames == 1 {
            n := scene.WithComponent("SpriteRenderer", found)
            log.Info(string(n) + " nodes with a SpriteRenderer")
            n = scene.WithTag("Breakable", found)
            log.Info(string(n) + " breakable, first is " + found[0].Name())
            n = scene.InArea(Rect{0.5, -0.5, 1, 1}, -1, found)
            log.Info(string(n) + " node in the area right of the origin")
        }
        if frames == 60 {
            crate := scene.Find("Crate")
            if crate != nil {
                log.Info("destroying " + crate.Name())
                scene.Destroy(crate)
            }
        }
        if frames == 61 {
            log.Info("Crate still found: " + string(scene.Find("Crate") != nil))
        }
    }
}
```

---
id: aego/curve
summary: Вычисляет готовые функции сглаживания и поля-кривые компонентов.
---

`Ease` принимает готовую функцию, например `curve.OutCubic`, и время от 0 до 1. `Eval`, `EvalClamped` и `Duration` принимают `Curve`, которая берётся из поля-кривой компонента (`fader.Alpha`); в песочнице документации нет компонента с полем-кривой, поэтому пример использует `Ease`. См. [Кривые](ref:curves).

## Пример

```aes run
package demo

import (
    "aego/curve"
    "aego/log"
    "aego/math"
)

script Slide {
    @export var From Vec2 = Vec2{0, 0}
    @export var To Vec2 = Vec2{4, 0}
    @export var Duration float = 1.5

    var age float

    func Ready() {
        log.Info("at t=0.5: linear " + string(curve.Ease(curve.Linear, 0.5)) + ", in quad " + string(curve.Ease(curve.InQuad, 0.5)) + ", out cubic " + string(curve.Ease(curve.OutCubic, 0.5)))
        log.Info("out back overshoots: " + string(curve.Ease(curve.OutBack, 0.8)))
    }

    func Update(dt float) {
        if age >= Duration {
            return
        }
        age += dt
        k := curve.Ease(curve.InOutSine, age / Duration)
        node.SetPosition(math.LerpVec2(From, To, k))
        if age >= Duration {
            log.Info("arrived at x=" + string(node.Position().X))
        } else if int(age * 60) % 15 == 0 {
            log.Info("x=" + string(node.Position().X))
        }
    }
}
```

---
id: aego/testing
summary: Проверки для тестов скриптов: проваленная проверка останавливает тест и сообщает файл и строку.
---

Тестовые функции — это функции пакета с именами `TestXxx` в файлах `*_test.aes`, и `aego test` запускает их без окна. Песочница документации не запускает `aego test`, поэтому пример вызывает свою тестовую функцию из `Ready`; в проекте перенесите `TestDamage` в файл `_test.aes` и удалите скрипт.

## Пример

```aes run
package demo

import (
    "aego/log"
    "aego/math"
    "aego/testing"
)

func damage(hp int, hit int) int {
    return math.MaxInt(hp - hit, 0)
}

func label(hp int) string {
    if hp == 0 {
        return "dead"
    }
    return "alive"
}

func TestDamage() {
    testing.Equal(damage(10, 3), 7)
    testing.Equal(damage(2, 5), 0)
    testing.EqualString(label(damage(2, 5)), "dead")
    testing.EqualFloat(math.Sqrt(2), 1.4142, 0.001)
    testing.True(damage(10, 0) == 10, "a zero hit changes nothing")
    testing.False(damage(1, 1) > 0, "the last hit kills")
}

script Runner {
    func Ready() {
        TestDamage()
        log.Info("TestDamage passed")
    }
}
```

---
id: events
---

Объявите метод в скрипте; модуль, которому принадлежит событие, вызывает его, когда событие происходит. События триггеров ниже приходят из «Физики 2D» и требуют тела с коллайдером-триггером, которого в песочнице документации нет, поэтому пример вызывает тот же метод `Collect`, который вызывает `OnTriggerEnter`.

## Пример

```aes run
package demo

import "aego/log"

script Coin {
    @export var Value int = 10

    var taken bool

    func Ready() {
        log.Info("coin ready")
    }

    func OnTriggerEnter(other NodeRef) {
        if other.HasTag("Player") {
            Collect(other)
        }
    }

    func OnTriggerExit(other NodeRef) {
        log.Info(other.Name() + " left the coin")
    }

    func Collect(by NodeRef) {
        if taken {
            return
        }
        taken = true
        log.Info(by.Name() + " picked up " + string(Value) + " coins")
        node.Destroy()
    }
}

script Player {
    var frames int

    func Ready() {
        node.AddTag("Player")
    }

    func Update(dt float) {
        frames++
        if frames == 30 {
            coin := node.Find("/Coin")
            if coin != nil {
                coin.GetScript<Coin>().Collect(node)
            }
        }
        if frames == 31 {
            log.Info("coin left in the scene: " + string(node.Find("/Coin") != nil))
        }
    }
}
```
