---
id: language/builtins
title: Ключевые слова и встроенные средства
---

# Ключевые слова и встроенные средства

Все ключевые слова, встроенные типы и встроенные функции Aego Script. Подсказка при наведении в редакторе скриптов показывает тот же текст.

«Запустить» показывает `append`, `len`, `start`, `yield frames`, `signal`, `Emit`, `Connect` и `Disconnect` в одном скрипте.

```aes run
package demo

import "aego/log"

script Builtins {
    signal Counted(n int)

    var words []string
    var conn Connection
    var count Coroutine

    func Ready() {
        words = append(words, "yield")
        words = append(words, "start")
        words = append(words, "signal")
        conn = Counted.Connect(OnCounted)
        count = start Count()
        log.Info(string(len(words)) + " words, running: " + string(count.Running()))
    }

    func Count() coroutine {
        for i := 0; i < len(words); i++ {
            Counted.Emit(i)
            yield frames(10)
        }
        conn.Disconnect()
        log.Info("connected after Disconnect: " + string(conn.Connected()))
    }

    func OnCounted(n int) {
        log.Info("word " + string(n) + ": " + words[n] + " (" + string(len(words[n])) + " bytes)")
    }
}
```

---
id: keyword/package
title: package
---

package называет пакет этого файла. Имя должно совпадать с именем каталога в scripts/; все файлы одного каталога используют одно имя пакета.

---
id: keyword/import
title: import
---

import делает пакет скриптов или модуль движка доступным по последнему элементу его пути.

```aes
import (
	"aego/input"
	"mygame/combat"
)
```

---
id: keyword/script
title: script
---

script объявляет тип с состоянием, который прикрепляется к узлам сцены. Узел содержит не больше одного экземпляра каждого типа скрипта.
Поля var — состояние во время работы; поля @export сохраняются в сцене и показываются в «Инспекторе».

```aes
script Mover {
	@export var Speed float = 200
	func Update(dt float) {
	}
}
```

---
id: keyword/type
title: type
---

type объявляет структуру. Структуры — значения: присваивание и передача их копируют.

```aes
type Stats struct {
	Hp    int
	Armor float
}
```

---
id: keyword/struct
title: struct
---

struct перечисляет поля объявления типа. Поля могут быть скалярами, векторами, ссылками, перечислениями, срезами или другими структурами.

---
id: keyword/enum
title: enum
---

enum объявляет именованные целые константы, которые используются как Name.Member. switch по перечислению должен покрывать все его элементы или иметь default.

```aes
enum State {
	Idle
	Run
}
```

---
id: keyword/var
title: var
---

var объявляет переменную с явным типом и необязательным значением. Внутри скрипта он объявляет поле.

```aes
var hits []int
var speed float = 1.5
```

---
id: keyword/const
title: const
---

const объявляет константу времени компиляции типа bool, int, float, string или перечисления. Пакеты содержат только константы и никогда — изменяемые переменные.

```aes
const MaxSpeed float = 600
```

---
id: keyword/func
title: func
---

func объявляет функцию. Функции пакета не видят состояния скрипта; функции внутри скрипта — методы, они могут использовать node и поля.

```aes
func clamp01(x float) float {
	return math.Clamp(x, 0, 1)
}
```

---
id: keyword/return
title: return
---

return завершает функцию, со значением, если функция объявляет результат. Такая функция должна возвращать значение на каждом пути.

---
id: keyword/if
title: if
---

if выполняет блок, когда условие типа bool истинно. Скобки вокруг условия не ставятся.

```aes
if hp <= 0 {
	die()
} else if hp < 10 {
	flee()
}
```

---
id: keyword/else
title: else
---

else выполняется, когда условие предыдущего if ложно. Он должен стоять на строке закрывающей скобки: } else {

---
id: keyword/for
title: for
---

for — единственный цикл: for cond {}, for init; cond; post {} или for {} для бесконечного цикла.

```aes
for i := 0; i < len(items); i++ {
}
```

---
id: keyword/switch
title: switch
---

switch сравнивает значение с каждым case и выполняет первое совпадение. Сквозного перехода (fallthrough) нет.

```aes
switch state {
case State.Idle:
case State.Run, State.Walk:
default:
}
```

---
id: keyword/case
title: case
---

case перечисляет одно или несколько значений для ветки switch.

---
id: keyword/default
title: default
---

default выполняется, когда ни один case в switch не совпал.

---
id: keyword/break
title: break
---

break выходит из самого внутреннего цикла for или switch.

---
id: keyword/continue
title: continue
---

continue переходит к следующей итерации самого внутреннего цикла for.

---
id: keyword/true
title: true
---

true — константа bool со значением «да».

---
id: keyword/false
title: false
---

false — константа bool со значением «нет».

---
id: keyword/nil
title: nil
---

nil — пустая ссылка. nil могут быть только NodeRef, AssetRef и ComponentRef; чтение через nil завершается ошибкой AES6004.

```aes
if Target != nil {
}
```

---
id: keyword/export-attr
title: @export
---

@export сохраняет поле в сцене и показывает его в «Инспекторе».
Аргументы: min, max, step (только для чисел), unit, display, tip, category.

```aes
@export(min=0, max=600, unit="px/s")
var Speed float = 220
```

---
id: keyword/former-attr
title: @former
---

@former перечисляет старые имена, чтобы сцены, сохранённые до переименования, по-прежнему загружались; при сохранении сцена переписывается с новым именем.

```aes
@former("Velocity")
@export var Speed float
```

---
id: keyword/hide-attr
title: @hide
---

@hide убирает скрипт из меню «Добавить скрипт». Уже прикреплённые экземпляры продолжают работать.

---
id: keyword/ready
title: Ready
---

Ready выполняется один раз, после того как все экземпляры в сцене созданы и ссылки разрешены, в прямом порядке обхода сцены.

```aes
func Ready() {
}
```

---
id: keyword/update
title: Update
---

Update выполняется каждый кадр. dt — время кадра в секундах.

```aes
func Update(dt float) {
}
```

---
id: keyword/fixedupdate
title: FixedUpdate
---

FixedUpdate выполняется на каждом фиксированном шаге, перед физикой. dt — фиксированный шаг, по умолчанию 1/60.

```aes
func FixedUpdate(dt float) {
}
```

---
id: keyword/reloaded
title: Reloaded
---

Reloaded выполняется после горячей перезагрузки для каждого экземпляра, у которого изменился скрипт или зависимость. Ready повторно не выполняется.

```aes
func Reloaded() {
}
```

---
id: keyword/node
title: node
---

node — NodeRef узла, к которому прикреплён этот экземпляр. Он есть в каждом методе скрипта.

```aes
t := node.Transform2D()
```

---
id: keyword/bool
title: bool
---

bool — это true или false. Условия if и for должны иметь тип bool.

---
id: keyword/int
title: int
---

int — знаковое 64-битное целое, которое при переполнении переходит через край. Преобразуйте через float(i) и int(f); неявного преобразования нет.

---
id: keyword/float
title: float
---

float — 32-битное число IEEE. Преобразование float → int насыщается и превращает NaN в 0.

---
id: keyword/string
title: string
---

string — неизменяемый текст в UTF-8. + склеивает строки, len считает байты, string(x) преобразует bool, int и float.

---
id: keyword/vec2
title: Vec2
---

Vec2 — 2D-вектор с компонентами X и Y. Создавайте его составным литералом; он поддерживает +, -, а также * и / на float.

```aes
v := Vec2{1, 0} * Speed
```

---
id: keyword/vec3
title: Vec3
---

Vec3 — 3D-вектор с компонентами X, Y и Z.

```aes
up := Vec3{0, 1, 0}
```

---
id: keyword/color
title: Color
---

У Color есть компоненты R, G, B и A в линейном пространстве, от 0 до 1.

```aes
red := Color{1, 0, 0, 1}
```

---
id: keyword/quat
title: Quat
---

Quat — 3D-поворот с компонентами X, Y, Z и W. Создавайте его через math.QuatEuler или math.QuatAxisAngle. Quat нельзя сравнивать через ==.

---
id: keyword/rect
title: Rect
---

У Rect есть компоненты X, Y, W и H.

---
id: keyword/transform2d
title: Transform2D
---

Transform2D содержит Position Vec2, Rotation float в радианах (по часовой стрелке) и Scale Vec2. Это значение: прочитайте его, измените и запишите обратно.

```aes
t := node.Transform2D()
t.Position.X += 1
node.SetTransform2D(t)
```

---
id: keyword/transform3d
title: Transform3D
---

Transform3D содержит Position Vec3, Rotation Quat и Scale Vec3.

```aes
t := node.Transform3D()
t.Position.Y += 1
node.SetTransform3D(t)
```

---
id: keyword/noderef
title: NodeRef
---

NodeRef ссылается на узел сцены и может быть nil. Экспортируемые поля NodeRef задаются перетаскиванием узла на поле в «Инспекторе».

```aes
@export var Target NodeRef
```

---
id: keyword/assetref
title: AssetRef
---

AssetRef<Texture | Scene | Prefab | Mesh> ссылается на ассет и может быть nil. Назначьте его в «Инспекторе».

```aes
@export var Bullet AssetRef<Prefab>
```

---
id: keyword/componentref
title: ComponentRef
---

ComponentRef<T> ссылается на компонент узла и проверяется заново при каждом обращении.

```aes
sr := node.Get<SpriteRenderer>()
if sr != nil {
	sr.FlipX = true
}
```

---
id: keyword/len
title: len
---

len возвращает число элементов среза или число байтов строки.

```aes
n := len(items)
```

---
id: keyword/append
title: append
---

append возвращает срез с v, добавленным в конец. Присвойте результат обратно.

```aes
hits = append(hits, 5)
```

---
id: keyword/coroutine
title: coroutine
---

coroutine после сигнатуры метода делает метод корутиной: она может приостановиться через yield и продолжиться в одном из следующих кадров. Корутинами могут быть только методы скрипта без результата, и не методы жизненного цикла. Запускайте корутину через start Name(args).

```aes
func Blink() coroutine {
	yield wait(0.1)
}
```

---
id: keyword/start
title: start
---

start выполняет метод-корутину этого скрипта до первого yield и возвращает её дескриптор Coroutine. Корутина принадлежит экземпляру и отменяется, когда экземпляр останавливается.

```aes
blink = start Blink()
```

---
id: keyword/yield
title: yield
---

yield приостанавливает текущую корутину:
```text
  yield            next frame
  yield wait(s)    after s seconds of game time
  yield frames(n)  after n frames
  yield fixed(n)   after n fixed steps
  yield c          when coroutine c ends
```
yield разрешён только внутри корутины (AES2050).

---
id: keyword/yield-wait
title: yield wait
---

yield wait(seconds float) продолжает выполнение, когда пройдёт заданное игровое время, но не раньше следующего кадра.

```aes
yield wait(0.25)
```

---
id: keyword/yield-frames
title: yield frames
---

yield frames(n int) продолжает выполнение через n кадров; frames(1) — то же, что yield без аргументов.

```aes
yield frames(3)
```

---
id: keyword/yield-fixed
title: yield fixed
---

yield fixed(n int) продолжает выполнение через n фиксированных шагов, перед физикой; используйте его для движения, которое должно идти в такт с FixedUpdate.

```aes
yield fixed(1)
```

---
id: keyword/signal
title: signal
---

signal объявляет событие скрипта. Вызвать Emit для него может только сам скрипт; другие скрипты подключаются к нему через Connect по ScriptRef.

```aes
signal Died(reason string)
```

---
id: keyword/emit
title: Emit
---

Emit синхронно вызывает каждый подключённый обработчик в порядке подключения. Подключения, добавленные или удалённые обработчиком, вступают в силу после возврата из этого Emit.

```aes
Died.Emit("fall")
```

---
id: keyword/connect
title: Connect
---

Connect регистрирует метод с теми же параметрами, что у сигнала, и возвращает Connection. Обработчик должен быть обычным методом: не корутиной и не методом жизненного цикла.

```aes
conn = enemy.Died.Connect(OnEnemyDied)
```

---
id: keyword/disconnect
title: Disconnect
---

Disconnect удаляет обработчик; повторный вызов безвреден.

```aes
conn.Disconnect()
```

---
id: keyword/connected
title: Connected
---

Connected сообщает, активно ли ещё подключение. Он становится false после Disconnect или когда останавливается источник либо владелец обработчика.

---
id: keyword/stop
title: Stop
---

Stop отменяет корутину; она больше не выполняется. Остановка завершённой корутины безвредна.

```aes
blink.Stop()
```

---
id: keyword/running
title: Running
---

Running сообщает, жива ли ещё корутина (не завершена, не отменена и не упала с ошибкой).

```aes
if !blink.Running() {
	blink = start Blink()
}
```

---
id: keyword/coroutine-type
title: Coroutine
---

Coroutine — дескриптор, который возвращает start. Нулевое значение — nil, и оно никогда не выполняется.

---
id: keyword/connection
title: Connection
---

Connection — дескриптор, который возвращает Connect. Храните его в поле, чтобы позже вызвать Disconnect.

---
id: keyword/scriptref
title: ScriptRef
---

ScriptRef<T> ссылается на экземпляр скрипта на узле. Через него вы читаете и пишете поля, вызываете обычные методы и подключаетесь к сигналам T. Методы жизненного цикла и корутины недоступны (AES2059). Ссылка становится недействительной, когда экземпляр останавливается; тогда обращение завершается ошибкой AES6004.

```aes
var enemy ScriptRef<EnemyAI>
```

---
id: keyword/getscript
title: GetScript
---

GetScript<T>() возвращает экземпляр скрипта T на узле или nil, если его там нет. Используйте pkg.Name, когда короткое имя неоднозначно.

```aes
e := Target.GetScript<EnemyAI>()
if e != nil {
	e.Died.Connect(OnEnemyDied)
}
```

---
id: keyword/copy
title: Copy
---

Возвращает копию ассета данных в виде структуры, которой владеет скрипт. Изменение копии не меняет ассет.

```aes
s := Stats.Copy()
```

---
id: keyword/row
title: Row
---

Возвращает строку таблицы с заданным id. Если строки нет, скрипт останавливается с AES6004; сначала проверьте через Has.

```aes
entry := Loot.Row("goblin")
```

---
id: keyword/has
title: Has
---

Сообщает, есть ли в таблице строка с заданным id.

---
id: keyword/table-len
title: Len
---

Возвращает число строк в таблице.
