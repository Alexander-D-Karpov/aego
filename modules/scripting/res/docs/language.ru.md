---
id: language
title: Aego Script
---
# Aego Script

Aego Script — язык геймплея в Aego. Он статически типизирован и читается как Go. Скрипты прикрепляются к узлам, показывают поля в Инспекторе, читают и пишут компоненты, вызывают функции движка и нативные функции и общаются друг с другом через сигналы.

Эти страницы описывают язык. [Обзор](ref:language/overview) разбирает один скрипт — врага, который патрулирует, — а остальные страницы объясняют каждую часть. Функции движка перечислены в справочнике Script API.

```aes run
package demo

import "aego/log"

script Hello {
    @export var Greeting string = "hello from Aego Script"

    var frames int

    func Ready() {
        log.Info(Greeting + " on node " + node.Name())
    }

    func Update(dt float) {
        frames++
        if frames == 60 {
            log.Info("one second of frames: " + string(frames))
        }
    }
}
```

---
id: language/overview
title: Обзор
---
# Обзор

На этой странице разобран один полный скрипт: враг ходит туда-обратно, ждёт на каждом конце и сообщает другим скриптам, когда разворачивается. Каждая часть ссылается на страницу, которая её объясняет.

```aes run
package enemy

import "aego/log"

enum State {
    Walk
    Wait
}

script Patrol {
    @export(min=0, max=20, unit="u/s")
    var Speed float = 3

    @export(min=0, max=50, unit="u")
    var Distance float = 4

    @export(min=0, max=5, unit="s")
    var Pause float = 0.5

    signal Turned(dir float)

    var state State
    var dir float = 1
    var origin float
    var turns int

    func Ready() {
        origin = node.Position().X
        Turned.Connect(OnTurned)
    }

    func Update(dt float) {
        if state == State.Wait {
            return
        }
        p := node.Position()
        p.X += dir * Speed * dt
        node.SetPosition(p)
        if dir > 0 && p.X >= origin+Distance || dir < 0 && p.X <= origin {
            start Turn()
        }
    }

    func Turn() coroutine {
        state = State.Wait
        yield wait(Pause)
        dir = -dir
        sr := node.Get<SpriteRenderer>()
        if sr != nil {
            sr.FlipX = dir < 0
        }
        state = State.Walk
        Turned.Emit(dir)
    }

    func OnTurned(d float) {
        turns++
        log.Info("turn " + string(turns) + ", walking " + string(d))
    }
}
```

## Части

- `package enemy` и `import` называют пакет скрипта и пакеты движка, которые он использует. См. [Файлы и пакеты](ref:language/files-and-packages).
- `enum State` объявляет два состояния. См. [Типы](ref:language/types).
- `@export` показывает `Speed`, `Distance` и `Pause` в Инспекторе с их диапазоном и единицей измерения. См. [Атрибуты](ref:language/attributes).
- `Ready` выполняется один раз, когда узел входит в игру, `Update` — один раз за кадр. См. [Скрипты и жизненный цикл](ref:language/scripts-and-lifecycle).
- `node` — узел, к которому прикреплён скрипт; `node.Get<SpriteRenderer>()` читает один из его компонентов. См. [Узлы, компоненты и ссылки](ref:language/nodes-components-and-references).
- `Turn` — корутина: `yield wait(Pause)` приостанавливает её на `Pause` секунд, не останавливая игру. См. [Корутины](ref:language/coroutines).
- `signal Turned` позволяет другим скриптам реагировать на разворот. Скрипт подключает к нему собственный обработчик в `Ready` и испускает его в конце `Turn`. См. [Сигналы](ref:language/signals).

## Как использовать

1. Сохраните скрипт как `scripts/enemy/patrol.aes`.
2. Выберите узел со спрайтом, нажмите **Добавить скрипт** в Инспекторе и выберите **Patrol**.
3. Нажмите «Играть». Узел идёт вправо, ждёт, разворачивается и идёт обратно.
4. Пока игра идёт, измените **Speed** или **Distance** в Инспекторе. Узел сразу следует новым значениям.

---
id: language/files-and-packages
title: Файлы и пакеты
---
# Файлы и пакеты

- Исходные файлы имеют расширение `.aes` и лежат в `scripts/` внутри проекта. Любая папка внутри `scripts/` — пакет; файлы одной папки принадлежат одному пакету.
- Путь пакета — `project.module + "/" + directory` (`mygame/player`). Объявление `package` должно совпадать с базовым именем папки (`package player`); несовпадение — `AES4001`.
- Путь модуля использует только `[a-z0-9_/-]`, без точек: полные имена (`mygame/player.State`) разделяются по последней точке.
- `import` работает как в Go; псевдонимов импорта нет. Импорт пакета, который не лежит в `scripts/` и не является модулем хоста, — `AES4002`; циклы — `AES4005`. Неиспользуемые импорты дают предупреждения.
- Модули хоста (`aego/input`, `mygame/combat`) и пакеты скриптов делят одно пространство имён. Папка скриптов, путь пакета которой совпадает с путём модуля хоста, — `AES4010`. Префикс `aego/` зарезервирован за стандартной библиотекой; `module = "aego"` — ошибка проекта.
- Видимость устроена как в Go: идентификаторы с заглавной буквы экспортируются из пакета, всё остальное приватно для пакета. `@export` с видимостью не связан (см. [Атрибуты](ref:language/attributes)).
- Файлы `*_test.aes` компилирует только `aego test`.

Пакет, который не компилируется, сохраняет свою последнюю рабочую сборку. Его файлы помечаются как неверные, и запуск игры заблокирован, пока они снова не скомпилируются.

Запустите: один пакет с экспортируемыми и приватными объявлениями. Песочница компилирует один файл, поэтому для импорта между пакетами скриптов нужен проект.

```aes run
package inventory

import (
    "aego/log"
    "aego/math"
)

// MaxSlots is exported: other packages read it as inventory.MaxSlots.
const MaxSlots int = 8

// stackSize is package-private.
const stackSize int = 20

// Fit is exported and can be called from other packages.
func Fit(count int) int {
    return math.MinInt(count, MaxSlots * stackSize)
}

script Bag {
    @export var Count int = 200

    func Ready() {
        log.Info("package inventory keeps " + string(Fit(Count)) + " of " + string(Count) + " items")
    }
}
```

---
id: language/lexical-structure
title: Лексическая структура
---
# Лексическая структура

- **Идентификаторы** состоят только из ASCII: `[A-Za-z_][A-Za-z0-9_]*`. Не-ASCII буква там, где может начинаться идентификатор, — `AES1001`. Внутри строковых литералов не-ASCII текст полностью поддерживается.
- Ключевые слова: `package import script type struct enum var const func return if else for switch case default break continue true false nil yield signal coroutine`.
- Контекстные слова: `start` — ключевое слово, только когда за ним идут имя и `(`; в остальных местах это обычный идентификатор. `wait`, `frames` и `fixed` особые только сразу после `yield`.
- **Зарезервированы для будущих версий** (их использование — `AES1004`): `await select go chan defer interface map`.
- Литералы: целые `10`, `0x1f`, `1_000`; дробные `1.5`, `.5`, `1e3`, `1_000.5`; строки `"..."` с экранированием `\n \t \" \\ \uXXXX`; `true`, `false`, `nil`. Подчёркивания должны стоять между цифрами.
- Комментарии: `// line` и `/* block */`. Группа комментариев `//` прямо над объявлением — его документация; она показывается при наведении и в автодополнении.
- **Точки с запятой** вставляются в конце строки после идентификатора, литерала, `)`, `]`, `}`, `return`, `break`, `continue`, `++`, `--` или после `>`, закрывающего аргументы типа. Пишите код построчно, как в Go; `}` и следующий за ней `else` должны стоять на одной строке.
- Операторы: `+ - * / % == != < <= > >= && || ! = := += -= *= /= %= ++ --`. Побитовых операторов нет; одиночный `&` или `|` — `AES1001`.

Намеренно неподдерживаемые лексические формы (для каждой есть диагностика, которая называет нужную форму):

| Ввод | Почему | Пишите вместо этого |
|---|---|---|
| `'a'` | нет типа `char` отдельно от `string` | `"a"` |
| `1.` / `1.e3` | у дробного числа должны быть цифры после точки | `1.0` / `1.0e3` |
| `имя := 1` | идентификаторы — ASCII | имена в ASCII |
| `a & b` | нет побитовой арифметики | `&&` / `\|\|` |

Запустите:

```aes run
package demo

import "aego/log"

/* Literals and operators from this page. */
script Lexer {
    func Ready() {
        hex := 0x1f
        big := 1_000_000
        half := .5
        sci := 1e3
        log.Info("ints " + string(hex) + " " + string(big) + ", floats " + string(half) + " " + string(sci))
        log.Info("escapes: tab[\t] quote[\"] backslash[\\] unicode[\u00e9]")
        log.Info("bytes in \"héllo\": " + string(len("héllo")))
        a := 7
        a += 3
        a %= 4
        a++
        log.Info("(7 + 3) % 4 + 1 = " + string(a) + ", both: " + string(a > 2 && !(a == 0)))
    }
}
```

---
id: language/grammar
title: Грамматика
---
# Грамматика

```
File        = "package" ident ";" { Import ";" } { TopDecl ";" } .
Import      = "import" ( string | "(" { string ";" } ")" ) .
TopDecl     = ConstDecl | TypeDecl | EnumDecl | FuncDecl | ScriptDecl .
ConstDecl   = "const" ident [ Type ] "=" Expr .
TypeDecl    = "type" ident "struct" "{" { ident Type ";" } "}" .
EnumDecl    = "enum" ident "{" { ident ";" } "}" .
FuncDecl    = { Attr } "func" ident "(" [ Params ] ")" [ Type | "coroutine" ] Block .
Params      = ident Type { "," ident Type } .
ScriptDecl  = { Attr } "script" ident "{" { ScriptMember ";" } "}" .
ScriptMember= { Attr } "var" ident Type [ "=" Expr ] | FuncDecl | SignalDecl .
SignalDecl  = "signal" ident "(" [ Params ] ")" .
Attr        = "@" ident [ "(" [ AttrArg { "," AttrArg } ] ")" ] .
AttrArg     = [ ident "=" ] ( Literal | "-" Literal ) .
Type        = ident [ "." ident ] [ "<" Type { "," Type } ">" ] | "[" "]" Type .
Block       = "{" { Stmt ";" } "}" .
Stmt        = "var" ident Type [ "=" Expr ]
            | ident ":=" Expr
            | LValue ( "=" | "+=" | "-=" | "*=" | "/=" | "%=" ) Expr
            | LValue ( "++" | "--" )
            | Expr | If | For | Switch
            | "return" [ Expr ] | "break" | "continue" | Block
            | "yield" [ YieldArg ] .
YieldArg    = "wait" "(" Expr ")" | "frames" "(" Expr ")" | "fixed" "(" Expr ")" | Expr .
If          = "if" Expr Block [ "else" ( If | Block ) ] .
For         = "for" [ Expr | [ Simple ] ";" [ Expr ] ";" [ Simple ] ] Block .
Switch      = "switch" Expr "{" { ( "case" Expr { "," Expr } | "default" ) ":" { Stmt ";" } } "}" .
Expr        = Unary { BinOp Unary } .
Unary       = [ "-" | "!" ] Primary | "start" ident "(" [ Expr { "," Expr } ] ")" .
Primary     = Operand { "." ident | "[" Expr "]" | [ "<" Type ">" ] "(" [ Expr { "," Expr } ] ")" } .
Operand     = Literal | ident | "(" Expr ")" | Type "{" [ Elems ] "}" .
```

Приоритет, от высокого к низкому: унарные `-` `!`; `* / %`; `+ -`; `== != < <= > >=`; `&&`; `||`. Аргументы типа в вызове (`Get<T>()`) парсер принимает где угодно, но использовать их могут только встроенные функции; см. [Узлы, компоненты и ссылки](ref:language/nodes-components-and-references).

Составные литералы нельзя писать прямо в заголовке `if`, `for` или `switch`; заключите их в скобки (`if x == (Stats{}) {`).

Запустите:

```aes run
package demo

import "aego/log"

const Limit int = 3

type Point struct {
    X int
    Y int
}

enum Dir {
    Left
    Right
}

func flip(d Dir) Dir {
    switch d {
    case Dir.Left:
        return Dir.Right
    default:
        return Dir.Left
    }
}

script Grammar {
    var steps []Point

    func Ready() {
        d := Dir.Left
        for i := 0; i < Limit; i++ {
            d = flip(d)
            steps = append(steps, Point{X: i, Y: int(d)})
        }
        n := 0
        for n < len(steps) {
            log.Info("step " + string(n) + ": " + string(steps[n].X) + ", " + string(steps[n].Y))
            n++
        }
        if steps[0] == (Point{0, 1}) {
            log.Info("a composite literal in an if head needs parentheses")
        }
    }
}
```

---
id: language/types
title: Типы
---
# Типы

| Тип | Описание |
|---|---|
| `bool` | `true` / `false` |
| `int` | знаковое 64-битное; при переполнении переходит через край |
| `float` | IEEE float32 |
| `string` | неизменяемая UTF-8; конкатенация `+`, сравнение, `len` в **байтах**; без индексации |
| `Vec2 Vec3 Color Quat Rect` | значимые типы движка; компоненты `.X .Y .Z .W`, `.R .G .B .A`, `.X .Y .W .H` |
| `Transform2D{Position Vec2, Rotation float, Scale Vec2}` | структура-значение |
| `Transform3D{Position Vec3, Rotation Quat, Scale Vec3}` | структура-значение |
| `NodeRef` | ссылка на узел; может быть `nil` |
| `AssetRef<Texture \| Scene \| Prefab \| Mesh>` | ссылка на ассет; может быть `nil` |
| `ComponentRef<T>` | ссылка на компонент узла; может быть `nil` |
| `ScriptRef<T>` | ссылка на экземпляр скрипта `T`; может быть `nil`; поля, обычные методы и сигналы `T` |
| `Coroutine` | дескриптор, который возвращает `start`; может быть `nil`; методы `Stop()`, `Running() bool` |
| `Connection` | дескриптор, который возвращает `Connect`; может быть `nil`; методы `Disconnect()`, `Connected() bool` |
| `Curve` | представление поля-кривой компонента только для чтения (`fader.Alpha`); вычисляется через `aego/curve` |
| `tween.Tween`, `timer.Timer`, … | дескрипторы хоста, которые возвращают модули движка и нативные модули; могут быть `nil`; методы задаёт модуль |
| `struct` | уровня пакета, семантика значения, копируется при присваивании |
| `enum` | уровня пакета, на основе `int`, члены `State.Idle` |
| `[]T` | растущий срез в куче VM |

Правила:

- Неявных числовых преобразований нет. Используйте `int(x)`, `float(x)`, `string(x)` (из `bool`, `int`, `float`, `string`), `int(e)` для перечисления и `State(i)` для int → перечисление. Нетипизированные константы подстраиваются под контекст (`var f float = 1` допустимо, `var i int = 1.5` — нет).
- Арифметика `float` везде идёт в float32, включая свёртку констант.
- `==`/`!=` работают со скалярами, строками, перечислениями, ссылками, дескрипторами, векторами, структурами со сравнимыми полями и `Transform*`; `Quat`, `Curve` и срезы несравнимы. `< <= > >=` работают с числами и строками.
- Арифметика векторов: `v + w`, `v - w`, `v * s`, `s * v`, `v / s`, `-v`. Векторы создаются только составными литералами (`Vec2{x, y}`, `Color{1, 0, 0, 1}`), никогда синтаксисом вызова.
- Структура — значение: `a = b` копирует, передача в функцию копирует. Вложенные структуры разрешены, в том числе структуры из других пакетов; структура, которая содержит себя по значению, — `AES2017`.
- Элемент среза может занимать не более 32 слотов значений (`AES2032`); скаляры, векторы, ссылки и небольшие структуры далеко не доходят до этого предела.
- `nil` допустим для `NodeRef`, `AssetRef`, `ComponentRef`, `ScriptRef`, `Coroutine`, `Connection` и дескрипторов хоста. Чтение или запись через nil-ссылку — ошибка времени выполнения (`AES6004`). Вызов метода у nil `Coroutine`, `Connection` или дескриптора хоста ошибкой не является: `Running()` и `Connected()` возвращают `false`, `Done()` возвращает `true`, а `Stop()`, `Cancel()` и `Disconnect()` ничего не делают.

Запустите:

```aes run
package demo

import "aego/log"

type Stats struct {
    Hp    int
    Armor float
}

enum Mood {
    Calm
    Angry
}

script Types {
    var conn Connection

    func Ready() {
        a := Stats{Hp: 10, Armor: 0.5}
        b := a
        b.Hp = 3
        log.Info("structs copy: a.Hp " + string(a.Hp) + ", b.Hp " + string(b.Hp))

        f := 7.9
        log.Info("int(7.9) = " + string(int(f)) + ", float(7) / 2 = " + string(float(7) / 2) + ", 7 / 2 = " + string(7 / 2))

        m := Mood(1)
        log.Info("Mood(1) is Angry: " + string(m == Mood.Angry) + ", int " + string(int(m)))

        v := Vec2{3, 4} * 2 - Vec2{1, 1}
        c := Color{1, 0.5, 0, 1}
        log.Info("vector " + string(v.X) + ", " + string(v.Y) + ", color green " + string(c.G))

        var hits []int
        hits = append(hits, 5)
        hits = append(hits, 7)
        log.Info("slice of " + string(len(hits)) + ", last " + string(hits[len(hits) - 1]))

        log.Info("nil connection connected: " + string(conn.Connected()))
    }
}
```

---
id: language/declarations-and-statements
title: Объявления и инструкции
---
# Объявления и инструкции

- `var x T`, `var x T = e` — явный тип; `x := e` — тип выводится из `e` (нетипизированные константы по умолчанию становятся `int`/`float`); `nil` не может инициализировать `:=`.
- На уровне пакета разрешён только `const`; `var` уровня пакета — `AES1105`. Состояние хранится в полях скриптов и в компонентах.
- У функций фиксированное число параметров и ноль или один результат. Функция, которая объявляет результат, должна заканчиваться завершающей инструкцией (`AES2013`). Рекурсия разрешена до глубины 256.
- `if`, `if … else if … else`; `for cond {}`, `for init; cond; post {}`, `for {}`; `break`, `continue`.
- `switch x { case a, b: … default: … }` — без проваливания. `switch` по перечислению должен покрывать все члены или иметь `default` (`AES2015`); повторяющиеся case — ошибки.
- Составное присваивание `+= -= *= /= %=` и `++`/`--` работают с lvalue типов `int`/`float`. Lvalue — это переменные, поля, компоненты векторов, элементы срезов, поля компонентов и поля, доступные через `ScriptRef`.
- Выражение на уровне инструкции должно быть вызовом; неиспользованное значение — ошибка. Отбросить результат `len`, `append`, преобразования, `Get<T>`, `GetScript<T>` или `Connect` — ошибка. Результат `start` отбрасывать можно.
- Неиспользуемые локальные переменные дают предупреждения.
- Замыканий, лямбд, defer, горутин, указателей и map нет.

Запустите:

```aes run
package demo

import "aego/log"

enum Weapon {
    Sword
    Bow
    Staff
}

func damage(w Weapon) int {
    switch w {
    case Weapon.Sword:
        return 12
    case Weapon.Bow, Weapon.Staff:
        return 8
    }
    return 0
}

func fib(n int) int {
    if n < 2 {
        return n
    }
    return fib(n - 1) + fib(n - 2)
}

script Statements {
    func Ready() {
        var total int
        var scale float = 1.5
        for w := 0; w < 3; w++ {
            total += damage(Weapon(w))
        }
        log.Info("total damage " + string(total) + ", scaled " + string(float(total) * scale))

        i := 0
        for {
            i++
            if i % 2 == 0 {
                continue
            }
            if i > 7 {
                break
            }
            log.Info("odd " + string(i))
        }
        log.Info("fib(15) = " + string(fib(15)))
    }
}
```

---
id: language/scripts-and-lifecycle
title: Скрипты и жизненный цикл
---
# Скрипты и жизненный цикл

```aes
script Enemy {
	@export var Speed float = 120
	var elapsed float

	signal Died()

	func Ready() {}
	func FixedUpdate(dt float) {}
	func Update(dt float) {}
	func Reloaded() {}

	func helper() int { return 1 }
}
```

- `script` — тип с состоянием, который прикрепляется к узлу. Узел содержит не больше одного экземпляра каждого типа скрипта; скрипты разных типов могут быть на одном узле.
- У каждого скрипта есть неявное поле `node NodeRef`. Назвать поле, метод или сигнал `node` — ошибка.
- Методы объявляются внутри скрипта и вызываются из других методов без квалификатора. Функции уровня пакета не видят состояния скрипта и не могут использовать `node`, `start`, `yield` или сигналы.
- Поля, объявленные через `var`, — состояние времени выполнения: они создаются при запуске игры и отбрасываются при остановке. В сцене сохраняются и в Инспекторе показываются только поля `@export`. Игра никогда не записывает изменения обратно в редактируемую сцену.
- Инициализаторы полей должны быть константами (или `nil` для ссылок и дескрипторов).

Методы жизненного цикла распознаются по имени и должны иметь ровно такие сигнатуры:

| Метод | Когда |
|---|---|
| `func Ready()` | один раз, после того как все экземпляры сцены созданы и ссылки разрешены, в прямом порядке обхода сцены |
| `func FixedUpdate(dt float)` | каждый фиксированный шаг, до физики, в порядке создания экземпляров |
| `func Update(dt float)` | каждый кадр, в порядке создания экземпляров |
| `func Reloaded()` | после горячей перезагрузки, для каждого экземпляра, у которого изменился скрипт или интерфейс зависимости; см. [Горячая перезагрузка](ref:language/hot-reload) |

Экземпляры, которые не объявляют метод, для него не обходятся: сцена с десятью тысячами экземпляров без `Update` ничего не стоит в фазе обновления. Методы жизненного цикла не могут быть корутинами (`AES2056`), обработчиками сигналов (`AES2055`) и не вызываются через `ScriptRef` (`AES2059`).

Запустите: песочница прикрепляет каждый скрипт к собственному узлу.

```aes run
package demo

import "aego/log"

script Enemy {
    @export var Speed float = 2

    var fixedSteps int
    var frames int
    var elapsed float

    func Ready() {
        log.Info("Ready on " + node.Name() + ", speed " + string(Speed))
    }

    func FixedUpdate(dt float) {
        fixedSteps++
    }

    func Update(dt float) {
        frames++
        elapsed += dt
        node.SetPosition(node.Position() + Vec2{Speed * dt, 0})
        if frames % 40 == 0 {
            log.Info("frame " + string(frames) + ", fixed steps " + string(fixedSteps) + ", elapsed " + string(elapsed) + ", x " + string(node.Position().X))
        }
    }

    func helper() int {
        return frames
    }
}

script Spawner {
    func Ready() {
        log.Info("Ready on " + node.Name() + "; every Ready runs before the first Update")
    }
}
```

---
id: language/attributes
title: Атрибуты
---
# Атрибуты

| Атрибут | Цель | Значение |
|---|---|---|
| `@export` | поле | сохраняется в сцене, показывается в Инспекторе |
| `@export(min=0, max=100, step=1, unit="px/s", display="Speed", tip="…", category="Movement")` | поле | то же, с подсказками для Инспектора; `min/max/step` только у числовых полей |
| `@former("OldName")` | скрипт или поле | старое имя по-прежнему загружается из сцен; при сохранении сцена переписывается с новым именем; также используется, чтобы сопоставить поля при горячей перезагрузке |
| `@hide` | скрипт | не предлагается в «Добавить скрипт» |
| `@persist` | поле | сохраняется в сохранениях игры |

Типы экспортируемых полей: `bool int float string Vec2 Vec3 Color Quat Rect`, перечисления, `NodeRef`, `AssetRef<T>`, `ComponentRef<T>`. Срезы, структуры, `ScriptRef`, `Coroutine`, `Connection`, `Curve` и дескрипторы хоста экспортировать нельзя (`AES3001`). Экспортированный `ComponentRef<T>` хранится как ссылка на узел. Инспектор предлагает только узлы, у которых есть `T`, а на узле без `T` поле начинается с `nil`.

Значение поля по умолчанию должно быть константой. Составные литералы `Vec2`, `Vec3`, `Color`, `Quat` и `Rect` с константными компонентами считаются константами: `var Jump Vec2 = Vec2{0, -5}`, `var Tint Color = Color{R: 1, A: 1}`. Инспектор показывает их как значение по умолчанию, и сцена, в которой нет этого свойства, начинает с них.

Аргументы атрибутов — литералы (возможно, отрицательные). Атрибуты перед `func` разбираются для совместимости с будущими версиями, но отклоняются (`AES3005`).

Неизвестные свойства в сцене (поле, которое удалили или переименовали без `@former`) сохраняются и записываются обратно с предупреждением. Во время выполнения они не применяются.

## Сохраняемые поля

`@persist` отмечает поле скрипта, которое хранится в сохранениях игры:

```
@persist
var Coins int
```

Сохраняемые поля могут быть bool, int, float, string, вектором или цветом, перечислением, `NodeRef` или `AssetRef`. Другие типы — AES3007; вместо этого сохраните то, на что они указывают, в `OnSave`. `@persist` и `@export` можно сочетать.

`OnSave()` выполняется при сохранении игры и записывает значения через `save.PutInt`, `PutFloat`, `PutBool`, `PutString` и `PutVec2`. `OnLoad()` выполняется после `Ready` при загрузке игры и читает их обратно через `save.GetInt(key, def)` и остальные. Ключи принадлежат узлу, поэтому два скрипта на одном узле делят их.

Запустите: атрибуты меняют то, что Инспектор, сцены и сохранения делают с полем; в песочнице каждое поле начинается со своего значения по умолчанию.

```aes run
package demo

import "aego/log"

enum Team {
    Red
    Blue
}

@former("Mover")
script Walker {
    @export(min=0, max=10, step=0.5, unit="u/s", display="Walk speed", tip="Units per second", category="Movement")
    var Speed float = 2.5

    @export
    @former("Side")
    var Team Team = Team.Blue

    @export var Jump Vec2 = Vec2{0, -5}
    @export var Tint Color = Color{R: 1, A: 1}

    @persist
    var Coins int

    func Ready() {
        log.Info("speed " + string(Speed) + ", team " + string(int(Team)) + ", jump " + string(Jump.Y) + ", tint red " + string(Tint.R))
        Coins += 5
        log.Info("coins " + string(Coins))
    }
}

@hide
script Helper {
    func Ready() {
        log.Info("Helper is attached here but not offered in Add Script")
    }
}
```

---
id: language/nodes-components-and-references
title: Узлы, компоненты и ссылки
---
# Узлы, компоненты и ссылки

## NodeRef

| Метод | |
|---|---|
| `IsAlive() bool`, `Name() string`, `Parent() NodeRef`, `Child(name string) NodeRef`, `Find(path string) NodeRef`, `Destroy()` | иерархия; `Find` принимает `Weapon/Gun`, `../Sibling`, `/Root/Player` |
| `Enabled() bool`, `SetEnabled(bool)`, `Space() int` | состояние; `Space` равно 0 для 2D и 1 для 3D |
| `Transform2D() Transform2D`, `SetTransform2D(t)`, `Position() Vec2`, `SetPosition(Vec2)`, `WorldPosition() Vec2`, `Rotation() float`, `SetRotation(float)`, `Scale() Vec2`, `SetScale(Vec2)` | 2D-узлы |
| `Transform3D() Transform3D`, `SetTransform3D(t)`, `Position3() Vec3`, `SetPosition3(Vec3)`, `Rotation3() Quat`, `SetRotation3(Quat)` | 3D-узлы |
| `Get<T>() ComponentRef<T>`, `Has<T>() bool` | компоненты |
| `GetScript<T>() ScriptRef<T>` (nil, если у узла нет T) | скрипты; см. [ScriptRef](ref:language/scriptref) |

Трансформации читаются и записываются по значению: `t := node.Transform2D(); t.Position.X += 1; node.SetTransform2D(t)`. 2D-метод на 3D-узле (и наоборот) вызывает ошибку. `Destroy` срабатывает в конце фазы; `IsAlive` становится false сразу. `Get<T>()` и `GetScript<T>()` на уничтоженном узле возвращают `nil`, а на ссылке `nil` вызывают ошибку `AES6004`.

## Компоненты

`node.Get<SpriteRenderer>()` возвращает `ComponentRef<SpriteRenderer>` или `nil`. Имена полей — имена из схемы с заглавной первой буквой (`sr.FlipX`, `hp.Current`); полям только для чтения присваивать нельзя. Короткое имя компонента сначала ищется в проекте. Если ровно у одного компонента проекта (из его нативных модулей) такое имя, используется он, а компоненты движка с тем же именем скрываются. Поэтому новый компонент движка никогда не ломает проект, в котором уже есть компонент с таким именем. Если в проекте совпадения нет, имя должно быть уникальным среди компонентов движка. Два компонента проекта с одним именем, или два компонента движка, дают `AES2021` со списком кандидатов; полное имя (`gameplay.Health`, `game.Health`) работает всегда. Поле-перечисление компонента на Go видно как `int`.

У компонентов могут быть и методы: `hp.Damage(5)`, `body.ApplyImpulse(Vec2{0, -300})`. Какие поля и методы видны, решает владелец компонента, когда регистрирует его (см. [Нативные привязки](ref:language/native-bindings)). Сгенерированные заглушки библиотеки (`Ctrl+Click` по имени компонента в редакторе) показывают, что именно доступно.

`ComponentRef` можно безопасно хранить в поле: он проверяется заново при каждом обращении и вызывает ошибку `AES6004`, как только компонент или его узел уничтожен, даже до конца фазы.

## AssetRef

`IsNil() bool`, `Name() string`. Ассеты назначаются в Инспекторе; скрипты передают их в функции движка (`scene.Instantiate`).

Запустите: песочница помещает каждый скрипт на собственный узел верхнего уровня с «Отрисовкой спрайта».

```aes run
package demo

import "aego/log"

script Gun {
    var shots int

    func Fire() {
        shots++
    }
}

script Player {
    var sprite ComponentRef<SpriteRenderer>
    var frames int

    func Ready() {
        sprite = node.Get<SpriteRenderer>()
        log.Info("has sprite: " + string(node.Has<SpriteRenderer>()))
        gun := node.Find("../Gun")
        if gun != nil && gun.IsAlive() {
            log.Info("found " + gun.Name() + " next to " + node.Name())
            gun.GetScript<Gun>().Fire()
        }
        t := node.Transform2D()
        t.Position.X += 1
        node.SetTransform2D(t)
        log.Info("position " + string(node.Position().X) + ", world " + string(node.WorldPosition().X))
    }

    func Update(dt float) {
        frames++
        if frames == 30 {
            sprite.FlipX = true
            sprite.Tint = Color{1, 0, 0, 1}
            log.Info("flipped: " + string(sprite.FlipX) + ", tint red " + string(sprite.Tint.R) + " green " + string(sprite.Tint.G))
        }
        if frames == 60 {
            gun := node.Find("/Gun")
            log.Info("gun has a sprite: " + string(gun.Has<SpriteRenderer>()))
            log.Info("Gun fired " + string(gun.GetScript<Gun>().shots) + " time")
        }
    }
}
```

---
id: language/native-bindings
title: Нативные привязки
---
# Нативные привязки

Код проекта на Go в `native/` предоставляет функции, константы и члены компонентов через SDK.

## Функции

```go
package native

import "aego/sdk"

type Health struct {
	Current float32 `aego:"current"`
	Max     float32 `aego:"max"`
}

func registerCombat(r *sdk.Registry) {
	m := r.ScriptModule("mygame/combat")
	sdk.Proc2(m, "Damage", func(c *sdk.ScriptCtx, target sdk.NodeHandle, amount int64) {
		if h, ok := sdk.Get[Health](c.Scene, target); ok {
			h.Current -= float32(amount)
		}
	}, sdk.ScriptDoc("deals damage to a node with Health"))
}
```

Скрипты делают `import "mygame/combat"` и вызывают `combat.Damage(target, 5)`. Используйте `Func0..Func5` для функций с результатом, `Proc0..Proc3` — без результата, `Const` и `Enum` — для констант и перечислений хоста. Допустимые типы параметров и результатов: `bool`, целые типы Go, `float32/float64`, `string`, `mathx.Vec2/Vec3/Color/Quat/Rect`, `scene.Transform2D/Transform3D`, `scene.NodeHandle` (`NodeRef`), `asset.Ref[…]`, `sdk.ScriptCurve` (параметр `Curve`, который разрешается его методом `Resolve`), целые типы, зарегистрированные через `Enum`, и типы Go, зарегистрированные как дескрипторы. Структуры и срезы не могут пересекать границу. `c.Fail(msg)` останавливает вызывающий экземпляр с `AES6010`; паника в Go становится `AES6002`.

## Поля и методы компонентов

Компонент, зарегистрированный через `sdk.Component[T]`, открывает каждое поле с флагом схемы `script`. Билдер сужает или расширяет этот набор:

```go
package native

import "aego/sdk"

type Health struct {
	Current float32 `aego:"current"`
	Max     float32 `aego:"max"`
}

func registerHealth(r *sdk.Registry) {
	sdk.Component[Health](r, sdk.Name("mygame.Health")).
		Category("Gameplay").
		Script("Current", "Max").
		Method1("Damage", func(c *sdk.ScriptCtx, h *Health, amount int64) bool {
			h.Current -= float32(amount)
			return h.Current <= 0
		}).
		Proc0("Heal", func(c *sdk.ScriptCtx, h *Health) { h.Current = h.Max })
}
```

`Script(names...)` перечисляет видимые поля, а `ScriptNone` полностью скрывает компонент от скриптов. `Method0`…`Method2` и `Proc0`…`Proc2` билдера принимают те же типы параметров, что и функции модуля, плюс указатель на компонент. Они компилируются в типизированные адаптеры и ничего не выделяют при вызове. Скрипты вызывают их у ссылки на компонент: `hp := node.Get<Health>()`, затем `hp.Damage(5)`. Методы не являются модулем, импортировать их нельзя. Вызов через ссылку `nil`, уничтоженный узел или удалённый компонент вызывает ошибку `AES6004`. `ScriptMethod(name, fn any)` принимает произвольную функцию Go через рефлексию. Он нужен для прототипов, пишет предупреждение при регистрации и отклоняется в сборках `aego_runtime`.

Хеш реестра привязок покрывает каждый модуль, имя и сигнатуру и входит в ключ кэша компиляции. Поэтому изменение нативной сигнатуры перекомпилирует скрипты, которые её используют. Изменения в `native/` никогда не подменяются внутри процесса; см. [Горячая перезагрузка](ref:language/hot-reload).

## Дескрипторы

Модуль, которому принадлежат долгоживущие объекты, передаёт их скриптам как дескрипторы:

```go
package native

import "aego/sdk"

// Request is a pending HTTP call; the key 0 is nil.
type Request uint64

func (r Request) Key() uint64 { return uint64(r) }

func RequestOf(key uint64) Request { return Request(key) }

type service struct{ done map[Request]bool }

func (s *service) Done(r Request) bool { return s.done[r] }

func registerNet(r *sdk.Registry, svc *service) {
	m := r.ScriptModule("mygame/net")
	sdk.ScriptHandle(m, "Request", true, RequestOf, Request.Key, sdk.ScriptDoc("Request is a pending HTTP call."))
	hm := m.Registry().HandleMethods("mygame/net.Request")
	sdk.Func1(hm, "Done", func(c *sdk.ScriptCtx, r Request) bool { return svc.Done(r) })
}
```

Для каждого типа дескриптора нужен отдельный тип Go, который преобразуется в ненулевой ключ `uint64` и обратно; ключ `0` — это `nil`. Скрипты видят тип как `net.Request` и вызывают его методы. У дескриптора, зарегистрированного как ожидаемый, должен быть `Done() bool`. Тогда `yield req` приостанавливает корутину, пока модуль не вызовет `Complete(abi.HandleID("mygame/net.Request"), key)` у сервиса мира `scriptwait.ServiceID`. Вызывайте его, когда объект завершается, срабатывает или отменяется.

Запустите: в песочнице нет кода `native/`, но стандартная библиотека и поля компонентов привязаны через тот же SDK, поэтому скрипты вызывают их так же, как ваш собственный модуль.

```aes run
package demo

import (
    "aego/log"
    "aego/math"
    "aego/rand"
)

script Bindings {
    func Ready() {
        log.Info("math.Clamp from Go: " + string(math.Clamp(14, 0, 10)) + ", math.Pi " + string(math.Pi))
        log.Info("rand.Int from Go: " + string(rand.Int(1, 7)))
        sr := node.Get<SpriteRenderer>()
        sr.FlipY = true
        log.Info("SpriteRenderer.FlipY set through a component binding: " + string(sr.FlipY))
    }
}
```

---
id: language/runtime-behaviour
title: Поведение во время выполнения
---
# Поведение во время выполнения

- Ошибка останавливает только экземпляр, который её вызвал (его методы, корутины и обработчики больше не вызываются); остальная сцена продолжает работать. Консоль показывает стек скрипта; двойной щелчок открывает файл.
- У каждого вызова есть бюджет инструкций (по умолчанию 1 000 000); бесконечный цикл заканчивается `AES6001`. У возобновления корутины и у обработчика сигнала свой бюджет.
- Целочисленное деление на ноль вызывает ошибку; `MinInt / -1` даёт `MinInt`, `MinInt % -1` даёт `0`. Преобразование `float → int` насыщается и превращает NaN в `0`, одинаково на всех платформах.
- Строки и срезы собираются сборщиком мусора между вызовами; скрипты, которые не выделяют память, не создают мусора. Приостановленная корутина удерживает свои строки и срезы.

| Код | Значение |
|---|---|
| AES6001 | превышен бюджет инструкций |
| AES6002 | паника в нативной привязке |
| AES6003 | глубина вызовов больше 256 или сигнал, который бесконечно испускает сам себя |
| AES6004 | nil или мёртвая ссылка (`NodeRef`, `ComponentRef`, `ScriptRef`, `Curve`) |
| AES6005 | индекс вне диапазона |
| AES6006 | целочисленное деление на ноль |
| AES6007 | неверный дескриптор строки или среза |
| AES6008 | достигнут `missing return` |
| AES6009 | неразрешённый extern или привязка |
| AES6010 | функция хоста сообщила об ошибке (`c.Fail`) |

Ошибки, которые вызывает сам движок (например, мёртвая ссылка внутри метода доступа компонента), несут свой точный код; только `c.Fail` из нативного кода отображается в `AES6010`.

Запустите:

```aes run
package demo

import (
    "aego/log"
    "aego/math"
)

script Runtime {
    var names []string
    var frames int

    func Ready() {
        log.Info("int(1e20) saturates to " + string(int(1e20)))
        log.Info("int(NaN) is " + string(int(math.Sqrt(-1))))
        minInt := -9223372036854775807
        minInt--
        log.Info("MinInt / -1 = " + string(minInt / -1) + ", MinInt % -1 = " + string(minInt % -1))
        log.Info("-7 / 2 = " + string(-7 / 2) + ", -7 % 2 = " + string(-7 % 2))
    }

    func Update(dt float) {
        frames++
        names = append(names, "frame " + string(frames))
        if frames % 40 == 0 {
            log.Info(string(len(names)) + " strings kept in a field, last \"" + names[len(names) - 1] + "\"")
        }
    }
}
```

---
id: language/limits
title: Ограничения
---
# Ограничения

255 регистров на функцию · 32 767 инструкций на функцию · 32 слота значений на элемент среза · 255 видимых скриптам полей на компонент (проверяется при регистрации компонента) · глубина вызовов 256 · 1 000 000 инструкций на вызов (настраивается).

Запустите:

```aes run
package demo

import "aego/log"

func depth(n int) int {
    if n == 0 {
        return 0
    }
    return 1 + depth(n - 1)
}

script Limits {
    var frames int
    var sum int

    func Ready() {
        log.Info("recursion " + string(depth(200)) + " deep, below the limit of 256")
    }

    func Update(dt float) {
        for i := 0; i < 20_000; i++ {
            sum += i % 7
        }
        frames++
        if frames % 60 == 0 {
            log.Info("frame " + string(frames) + ": 20 000 iterations per frame fit the budget, sum " + string(sum))
        }
    }
}
```

---
id: language/diagnostics
title: Диагностика
---
# Диагностика

У диагностик компилятора есть код, файл, строка и столбец, а часто и примечание:

```
scripts/player/player.aes:17:21 AES2003 cannot use value of type NodeRef as float in assignment
  note: field Speed has type float
```

`AES1xxx` — лексика и синтаксис · `AES2xxx` — типы · `AES3xxx` — атрибуты · `AES4xxx` — пакеты и импорты · `AES5xxx` — привязки · `AES6xxx` — ошибки времени выполнения. После ошибки парсер восстанавливается, поэтому один запуск сообщает обо всех проблемах в файле. `aego check --explain AES2052` выводит подробное описание кода; редактор показывает его при наведении в панели «Проблемы».

Коды, относящиеся к корутинам, сигналам и ссылкам на скрипты:

| Код | Значение |
|---|---|
| AES2050 | `yield` вне корутины |
| AES2051 | корутина вызвана без `start` |
| AES2052 | цель `start` — не корутина этого скрипта |
| AES2053 | корутина — не метод скрипта без результата |
| AES2054 | параметры обработчика не совпадают с сигналом |
| AES2055 | неверный обработчик `Connect` или `Emit` сигнала чужого скрипта |
| AES2056 | метод жизненного цикла объявлен корутиной |
| AES2057 | неверный тип аргумента `yield` |
| AES2058 | цель `ScriptRef` / `GetScript` — не тип скрипта или неоднозначна |
| AES2059 | метод жизненного цикла или корутина вызваны через `ScriptRef` |

Запустите: проверки ниже избегают ошибок времени выполнения AES6004, AES6005 и AES6006.

```aes run
package demo

import "aego/log"

script Guards {
    @export var Target NodeRef

    var scores []int

    func Ready() {
        scores = append(scores, 40)
        scores = append(scores, 15)
        var none []int
        log.Info("average " + string(average(scores)) + ", of none " + string(average(none)))
        log.Info("score 5: " + string(at(5)))
        if Target == nil {
            log.Warn("Target is not set; Target.Name() would stop the script with AES6004")
        }
    }

    func average(list []int) int {
        if len(list) == 0 {
            return 0
        }
        total := 0
        for i := 0; i < len(list); i++ {
            total += list[i]
        }
        return total / len(list)
    }

    func at(i int) int {
        if i < 0 || i >= len(scores) {
            return -1
        }
        return scores[i]
    }
}
```

---
id: language/cli
title: CLI
---
# CLI

| Команда | |
|---|---|
| `aego check [dir] [--manifest file] [--explain code]` | проверяет типы каждого пакета скриптов по привязкам движка и нативным привязкам |
| `aego test [dir] [-run name] [-v] [-budget n] [-seed n]` | запускает функции `TestXxx()` из `*_test.aes` без окна |
| `aego test [dir] --determinism [--frames n] [--every n] [--seed s] [--scene file] [-v]` | дважды запускает игру без окна и сравнивает хеши состояния |
| `aego dump script <pkg\|Type> [--func name] [--project dir]` | байт-код с комментариями `; file:line source`; заголовок перечисляет импорты, extern, перечисления, сигналы, ссылки на скрипты, поля и методы других скриптов и подключения |
| `aego run [dir]` | отказывается запускаться, пока в пакете скриптов есть ошибки |

Все они используют то же рабочее пространство, что редактор и горячая перезагрузка. `check` и `dump` используют манифест привязок. Для проекта с `native/` его записывает в `.aego/cache/script-bindings` небольшая вспомогательная программа, которая только вызывает `Register` модуля. Манифест пересобирается, когда меняются `native/*.go`, `go.mod`/`go.sum` или движок. Ошибка компиляции нативного кода или сбой `Register` дают код выхода 2. `test` в проекте с `native/` собирает бинарник игры и запускает тесты внутри него, потому что тесты вызывают настоящие нативные функции.

Тестовые функции — функции пакета. У них нет сцены, поэтому корутины, сигналы и ссылки на скрипты проверяются в игре, а не в `_test.aes`. Файлы `_test.aes` компилирует только `aego test`; они никогда не попадают в игру и в экспортированные сборки. Тест, который зацикливается навсегда, заканчивается `AES6001`, и запускается следующий тест.

`aego test --determinism` проверяет, что сама игра детерминирована. Команда дважды запускает главную сцену (или `--scene file`) без окна с одним и тем же зерном (`--seed`, по умолчанию 1) и фиксированным кадром 1/60 с на `--frames` кадров (по умолчанию 600). Каждые `--every` кадров (по умолчанию 60) она хеширует время мира, источник случайных чисел и состояние каждого сервиса, который поддерживает снимки перемотки. Если хеши различаются, она выводит первую контрольную точку, где они разошлись, и завершается с кодом 1. `-v` показывает, какие сервисы покрывает хеш, а какие нет (сервисы движка, например загрузчик ассетов и рендерер, не хранят состояния игры). Обычно различие значит, что скрипт или модуль читает системные часы, перебирает map в Go или хранит состояние вне мира.

Запустите: в песочнице нет запуска тестов, поэтому скрипт вызывает тестовую функцию из `Ready`. В проекте поместите `TestCombo` в файл `_test.aes` и запустите `aego test`.

```aes run
package demo

import (
    "aego/log"
    "aego/testing"
)

func combo(hits int) int {
    if hits < 3 {
        return 1
    }
    return hits / 3 + 1
}

func TestCombo() {
    testing.Equal(combo(0), 1)
    testing.Equal(combo(3), 2)
    testing.Equal(combo(9), 4)
}

script Runner {
    func Ready() {
        TestCombo()
        log.Info("ok   TestCombo")
    }
}
```

---
id: language/coroutines
title: Корутины
---
# Корутины

Корутина — метод скрипта, который может приостановиться и продолжить работу в одном из следующих кадров.

```aes
script Player {
	@export var HitTint Color
	var base Color
	var blink Coroutine

	func Hurt() {
		if !blink.Running() {
			blink = start Blink(6)
		}
	}

	func Blink(times int) coroutine {
		sr := node.Get<SpriteRenderer>()
		for i := 0; i < times; i++ {
			if i%2 == 0 {
				sr.Tint = HitTint
			} else {
				sr.Tint = base
			}
			yield wait(0.1)
		}
		sr.Tint = base
	}
}
```

### Объявление

`coroutine` пишется после сигнатуры: `func Name(params) coroutine { … }`. Корутинами могут быть только методы скрипта без результата (`AES2053`). `Ready`, `Update`, `FixedUpdate` и `Reloaded` не могут быть корутинами (`AES2056`); вместо этого запускайте корутину из них.

### Запуск

`start Name(args)` сразу выполняет корутину до её первого `yield` и возвращает `Coroutine`. Запускать можно только корутины того же скрипта (`AES2052`). Вызов корутины как обычной функции — `AES2051`. Если запустить одну корутину дважды, получатся два независимых выполнения.
- **yield** разрешён только внутри корутины (`AES2050`):

| Форма | Возобновляется |
|---|---|
| `yield` | в следующем кадре |
| `yield wait(s)` | через `s` секунд (`float`) игрового времени, не раньше следующего кадра; на паузе время не идёт |
| `yield frames(n)` | через `n` кадров (`int`) |
| `yield fixed(n)` | через `n` фиксированных шагов (`int`), до физики |
| `yield c` | когда корутина `c` заканчивается; возобновляется в том же кадре; если `c` равна `nil`, завершена или отменена — в следующем кадре |
| `yield h` | когда твин или таймер `h` (любой ожидаемый дескриптор хоста) заканчивается, срабатывает или отменяется, в следующем кадре; если `h` равен `nil` или завершён — в следующем кадре; на паузе игры ожидание продолжается |

Неверный тип аргумента или дескриптор, который нельзя ожидать, — `AES2057`.

### Порядок

Корутины, которым пора в одном кадре, возобновляются в порядке запуска. Они выполняются в `script.coroutines`, сразу после `script.update`; ожидания `fixed` выполняются после `script.fixed`. Обходятся только корутины, которым пора; десять тысяч спящих корутин ничего не стоят за кадр.

### Владение

Корутина принадлежит экземпляру, который её запустил. Она отменяется, когда узел экземпляра уничтожен, скрипт удалён или выключен, в экземпляре произошла ошибка или его скрипт изменился при горячей перезагрузке. Ошибка внутри корутины останавливает экземпляр-владелец, как ошибка в `Update`. Отменённая корутина сразу освобождает свой слот ожидания и не висит до момента, когда сработал бы её таймер.

### Дескрипторы

`c.Stop()` отменяет корутину; `c.Running()` равно false, когда корутина завершилась, была отменена или упала с ошибкой. Нулевое значение `Coroutine` — `nil`, оно никогда не выполняется. Дескриптор, полученный до горячей перезагрузки, остаётся допустимым значением, но больше никогда не выполняется.

### Бюджет

У каждого возобновления свой бюджет инструкций; см. [Ограничения](ref:language/limits). Цикл без `yield` внутри корутины всё равно заканчивается `AES6001`.

Запустите:

```aes run
package demo

import "aego/log"

script Blink {
    var blink Coroutine

    func Ready() {
        blink = start Run(3)
    }

    func Run(times int) coroutine {
        for i := 0; i < times; i++ {
            log.Info("blink " + string(i))
            yield wait(0.1)
        }
        log.Info("done")
    }
}
```

---
id: language/signals
title: Сигналы
---
# Сигналы

```aes
script EnemyAI {
	signal Died()

	func Kill() {
		Died.Emit()
		node.Destroy()
	}
}

script PlayerController {
	@export var Enemy NodeRef
	var kills int
	var conn Connection

	func Ready() {
		e := Enemy.GetScript<EnemyAI>()
		if e != nil {
			conn = e.Died.Connect(OnEnemyDied)
		}
	}

	func OnEnemyDied() {
		kills++
	}
}
```

### Объявление

`signal Name(params)` внутри скрипта. У сигналов нет результата. Имя с заглавной буквы видно из других пакетов.

### Emit

`Name.Emit(args)` разрешён только внутри скрипта, который объявляет сигнал (`AES2055`). Обработчики выполняются синхронно, в порядке подключения, до возврата из `Emit`. У каждого обработчика свой бюджет инструкций. Ошибка в обработчике останавливает владельца обработчика, а не того, кто испустил сигнал, и остальные обработчики всё равно выполняются.

### Connect

`Name.Connect(Method)` или `ref.Name.Connect(Method)` регистрирует обычный метод этого скрипта или `ref.Method` другого экземпляра и возвращает `Connection`. Обработчик должен принимать ровно типы параметров сигнала и ничего не возвращать (`AES2054`). Корутины и методы жизненного цикла не могут быть обработчиками (`AES2055`). Если подключить один метод дважды, он вызовется дважды.

### Изменения во время Emit

Подключения, добавленные или удалённые обработчиком, вступают в силу после возврата из текущего `Emit`; выполняющийся `Emit` использует список, с которым начал.

### Время жизни

Когда экземпляр-источник останавливается, все его подключения удаляются. Когда останавливается владелец обработчика, его подключения пропускаются и удаляются. В обоих случаях `Connected()` становится false. `c.Disconnect()` удаляет одно подключение; на дескрипторе `nil` он ничего не делает.

### Рекурсия

Обработчик, который снова испускает сигнал, уходит на один вызов глубже; бесконечное повторное испускание заканчивается `AES6003` на испускающем экземпляре.

### Стоимость

`Emit` ничего не выделяет; сцена с десятью тысячами подключений и тысячей испусканий за кадр не создаёт мусора.

Запустите:

```aes run
package demo

import "aego/log"

script Counter {
    signal Ticked(n int)

    var frames int
    var conn Connection

    func Ready() {
        conn = Ticked.Connect(OnTicked)
    }

    func Update(dt float) {
        frames++
        if frames%30 == 0 {
            Ticked.Emit(frames)
        }
    }

    func OnTicked(n int) {
        log.Info("tick at frame " + string(n))
    }
}
```

## Сигналы движка

Сигналы есть и у компонентов, и у дескрипторов движка. Скрипты подключаются к ним, но никогда их не испускают:

```aes
hp := node.Get<Health>()
conn = hp.Died.Connect(OnDied)
fade = tween.ToFloat(node, "SpriteRenderer.Tint.a", 0, 0.3, curve.Linear).Completed.Connect(OnFaded)
```

Правила те же, что и для сигналов скриптов. Подключения к сигналу компонента заканчиваются, когда его узел уничтожен или компонент удалён. Подключения к сигналу дескриптора заканчиваются после последнего испускания дескриптора (`Completed` у твина, последний `Fired` у одноразового таймера). Подключение к завершённому дескриптору возвращает `Connection`, равный `nil`. `Emit` у сигнала движка — `AES2055`. Параметры сигналов движка — числа, векторы, цвета, перечисления и ссылки, но никогда не строки и не срезы.

Код на Go подписывается через `sdk.Connect1[A](ctx, node, "Health.Died", fn)` или `"script/Enemy.Died"`. Обработчик на Go, который паникует, отключается, и об этом сообщается; он никогда не останавливает скрипт.

---
id: language/scriptref
title: ScriptRef
---
# ScriptRef

`node.GetScript<T>()` возвращает `ScriptRef<T>` скрипта `T` на этом узле или `nil`, если такого скрипта у узла нет. На узле `nil` или уничтоженном узле вызывает ошибку `AES6004`. Если короткое имя неоднозначно (`AES2058`), используйте `pkg.Name`.

Через `ScriptRef` можно читать и записывать поля (из других пакетов — только экспортируемые), вызывать обычные методы и подключаться к сигналам. Методы жизненного цикла и корутины недоступны (`AES2059`): вызовите обычный метод, который делает работу, или подключитесь к сигналу. `ScriptRef` проверяется заново при каждом обращении; как только целевой экземпляр останавливается, обращение вызывает ошибку `AES6004`. Поля `ScriptRef` нельзя экспортировать. `ScriptRef` переживает горячую перезагрузку любой из сторон: поля и методы заново разрешаются по имени.

Запустите:

```aes run
package demo

import "aego/log"

script Health {
    @export var Max int = 10

    var Current int

    signal Died()

    func Ready() {
        Current = Max
    }

    func Damage(n int) {
        Current -= n
        if Current <= 0 {
            Current = 0
            Died.Emit()
        }
    }
}

script Attacker {
    var target ScriptRef<Health>
    var conn Connection
    var frames int

    func Ready() {
        other := node.Find("/Health")
        target = other.GetScript<Health>()
        conn = target.Died.Connect(OnTargetDied)
    }

    func Update(dt float) {
        frames++
        if frames % 20 == 0 && target.Current > 0 {
            target.Damage(4)
            log.Info("hit, target has " + string(target.Current) + " of " + string(target.Max))
        }
    }

    func OnTargetDied() {
        log.Info("target died on frame " + string(frames))
        conn.Disconnect()
    }
}
```

---
id: language/hot-reload
title: Горячая перезагрузка
---
# Горячая перезагрузка

Если сохранить скрипт, пока игра идёт (`aego run`, «Играть» в редакторе), код подменяется без перезапуска. Рабочее пространство собирает из сохранённых файлов новое поколение. В начале следующего кадра (`script.swap`, `PhaseFrameBegin`, стек VM пуст) каждый модуль этого поколения загружается в ту же VM. Затем каждый живой экземпляр переносится в новую раскладку, а динамическая схема заменяется в той же транзакции. Если что-то не удаётся, ничего не меняется и продолжает работать предыдущее поколение. Сохранение, которое не компилируется, никогда не создаёт поколения.

Экземпляры сохраняют свою идентичность: `InstanceID`, узел, строки и срезы в куче остаются. Поля сопоставляются по имени (или `@former`), никогда по слоту:

| Изменение | Результат |
|---|---|
| тот же тип | значение сохраняется |
| новое поле | его значение по умолчанию |
| удалённое поле | отбрасывается |
| несовместимый тип (`float` → `string`) | значение по умолчанию + предупреждение, считается сбросом |
| перечисление | по имени члена; удалённый член сбрасывает поле |
| структура | поле за полем, рекурсивно |
| `[]T` | элемент за элементом; несовместимый тип элемента сбрасывает срез |
| `NodeRef`, `AssetRef`, `ComponentRef`, `ScriptRef` | сохраняются |
| `Coroutine` | сохраняется как значение; выполняется ли она дальше — см. ниже |
| `Connection` | сохраняется; перепривязывается или становится недействительным, см. ниже |
| дескрипторы хоста, `Curve` | сохраняются как значения |

После подмены для каждого скрипта, у которого изменился пакет (или интерфейс зависимости):

1. его **корутины отменяются**: приостановленное состояние корутины принадлежит старому коду; поле `Coroutine` хранит старый, уже не выполняющийся дескриптор;
2. **подключения к сигналам перепривязываются** по имени сигнала, типам параметров и имени обработчика; подключение, у которого пропал сигнал или обработчик или изменилась сигнатура, удаляется, и его `Connection` сообщает `Connected() == false`;
3. выполняется `func Reloaded()`. Используйте его, чтобы перезапустить корутины:

```aes
func Reloaded() {
	if flash > 0 {
		blink = start Blink(6)
	}
}
```

`Ready` повторно не выполняется. Экземпляры, упавшие с ошибкой в изменённом скрипте, снова начинают работать. Экземпляры удалённого скрипта останавливаются. Корутины и подключения неизменённых скриптов не затрагиваются. Консоль выводит одну строку на перезагрузку: `Hot reload N → N+1: X migrated, Y reset, Z coroutines canceled, W handlers disconnected, … ms`; те же числа появляются в разделе «Скрипты» Профилировщика. Изменения в `native/` никогда не подменяются внутри процесса: супервизор пересобирает игру и перезапускает её.

Запустите: песочница никогда не перезагружает код, поэтому `Reloaded` здесь не выполняется. В `aego run` или в игре из редактора измените `Beat` и сохраните: корутина отменится, и `Reloaded` запустит её снова.

```aes run
package demo

import "aego/log"

script Pulse {
    @export var Period float = 0.5

    var pulse Coroutine
    var beats int

    func Ready() {
        pulse = start Beat()
    }

    func Reloaded() {
        if !pulse.Running() {
            pulse = start Beat()
        }
    }

    func Beat() coroutine {
        for {
            beats++
            log.Info("beat " + string(beats))
            yield wait(Period)
        }
    }
}
```

---
id: language/editor
title: Редактор
---
# Редактор

Редактор кода, навигация, автодополнение, подсказки при наведении, заглушки библиотеки, панель «Проблемы» и разделы Профилировщика для скриптов описаны в разделе [Редактор](ref:editor). Несохранённые правки анализируются, но никогда не выполняются. Автосохранение записывает их через секунду после последнего нажатия клавиши, при потере фокуса и перед запуском игры.

Запустите: в редакторе кода наведите курсор на `Turret`, `Range` или `Rate`, чтобы увидеть комментарии над ними; Инспектор группирует поля по `category`.

```aes run
package demo

import "aego/log"

// Turret aims at the nearest target. This comment is shown on hover.
script Turret {
    // Range is how far the turret sees, in world units.
    @export(min=0, max=20, unit="u", category="Aim")
    var Range float = 8

    // Rate is shots per second.
    @export(min=0.1, max=10, category="Fire")
    var Rate float = 2

    var cooldown float
    var shots int

    func Update(dt float) {
        cooldown -= dt
        if cooldown <= 0 {
            cooldown = 1 / Rate
            shots++
            if shots % 2 == 0 {
                log.Info("shot " + string(shots) + " at range " + string(Range))
            }
        }
    }
}
```

---
id: language/data-assets
title: Ассеты данных
---
# Ассеты данных

Тип данных, зарегистрированный нативным модулем, доступен скриптам по короткому имени.

```
@export
var Stats AssetRef<EnemyStats>

@export
var Loot TableRef<LootEntry>
```

`Stats.Speed` читает поле записи, на которую указывает ссылка. Каждое чтение видит текущую запись, поэтому настройка вживую в редакторе доходит до скрипта при следующем чтении. Присваивать полям через ссылку нельзя (AES2007). `Stats.Copy()` возвращает запись как значение-структуру типа `data.EnemyStats`, которое скрипт может свободно менять.

У ссылки на таблицу есть `Row(id string)`, `Has(id string) bool` и `Len() int`. `Row` возвращает копию строки.

Чтение через nil-ссылку или запрос несуществующей строки останавливает скрипт с AES6004.

Запустите: `Stats.Copy()` возвращает структуру вроде `EnemyStats` ниже. Песочница не регистрирует типов данных, поэтому программа сама строит структуру.

```aes run
package demo

import "aego/log"

type EnemyStats struct {
    Speed float
    Hp    int
}

script Enemy {
    var stats EnemyStats

    func Ready() {
        stats = EnemyStats{Speed: 3, Hp: 20}
        boss := stats
        boss.Hp *= 5
        log.Info("enemy " + string(stats.Hp) + " hp, boss copy " + string(boss.Hp) + " hp")
    }

    func Update(dt float) {
        node.SetPosition(node.Position() + Vec2{stats.Speed * dt, 0})
        if node.Position().X >= 3 && stats.Speed > 0 {
            stats.Speed = 0
            log.Info("stopped at x " + string(node.Position().X))
        }
    }
}
```
