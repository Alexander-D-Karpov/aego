---
id: language/builtins
icon: code.block
parent: language
order: 100
title: Keywords and builtins
---

# Keywords and builtins

Every keyword, builtin type and builtin function of Aego Script. Hover in the script editor shows the same text.

Run it: `append`, `len`, `start`, `yield frames`, `signal`, `Emit`, `Connect` and `Disconnect` in one script.

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
parent: language/builtins
order: 1
title: package
keys: ["package"]
---

package names the package of this file. It must equal the directory name under scripts/; all files in one directory share it.

---
id: keyword/import
parent: language/builtins
order: 2
title: import
keys: ["import"]
---

import makes a script package or an engine module available by the last element of its path.

```aes
import (
	"aego/input"
	"mygame/combat"
)
```

---
id: keyword/script
parent: language/builtins
order: 3
title: script
keys: ["script"]
---

script declares a stateful type that attaches to scene nodes. A node holds at most one instance of each script type.
var fields are runtime state; @export fields are saved in the scene and shown in the Inspector.

```aes
script Mover {
	@export var Speed float = 200
	func Update(dt float) {
	}
}
```

---
id: keyword/type
parent: language/builtins
order: 4
title: type
keys: ["type"]
---

type declares a struct. Structs are values: assignment and passing copy them.

```aes
type Stats struct {
	Hp    int
	Armor float
}
```

---
id: keyword/struct
parent: language/builtins
order: 5
title: struct
keys: ["struct"]
---

struct lists the fields of a type declaration. Fields may be scalars, vectors, references, enums, slices or other structs.

---
id: keyword/enum
parent: language/builtins
order: 6
title: enum
keys: ["enum"]
---

enum declares named int constants used as Name.Member. A switch on an enum must cover every member or have a default.

```aes
enum State {
	Idle
	Run
}
```

---
id: keyword/var
parent: language/builtins
order: 7
title: var
keys: ["var"]
---

var declares a variable with an explicit type and an optional value. Inside a script it declares a field.

```aes
var hits []int
var speed float = 1.5
```

---
id: keyword/const
parent: language/builtins
order: 8
title: const
keys: ["const"]
---

const declares a compile-time constant of type bool, int, float, string or enum. Packages hold constants only, never mutable variables.

```aes
const MaxSpeed float = 600
```

---
id: keyword/func
parent: language/builtins
order: 9
title: func
keys: ["func"]
---

func declares a function. Package functions see no script state; functions inside a script are methods and can use node and the fields.

```aes
func clamp01(x float) float {
	return math.Clamp(x, 0, 1)
}
```

---
id: keyword/return
parent: language/builtins
order: 10
title: return
keys: ["return"]
---

return ends the function, with a value when the function declares a result. Such a function must return on every path.

---
id: keyword/if
parent: language/builtins
order: 11
title: if
keys: ["if"]
---

if runs a block when the bool condition is true. No parentheses around the condition.

```aes
if hp <= 0 {
	die()
} else if hp < 10 {
	flee()
}
```

---
id: keyword/else
parent: language/builtins
order: 12
title: else
keys: ["else"]
---

else runs when the preceding if condition is false. It must stay on the line of the closing brace: } else {

---
id: keyword/for
parent: language/builtins
order: 13
title: for
keys: ["for"]
---

for is the only loop: for cond {}, for init; cond; post {} or for {} for an endless loop.

```aes
for i := 0; i < len(items); i++ {
}
```

---
id: keyword/switch
parent: language/builtins
order: 14
title: switch
keys: ["switch"]
---

switch compares a value with each case and runs the first match. There is no fallthrough.

```aes
switch state {
case State.Idle:
case State.Run, State.Walk:
default:
}
```

---
id: keyword/case
parent: language/builtins
order: 15
title: case
keys: ["case"]
---

case lists one or more values for a switch branch.

---
id: keyword/default
parent: language/builtins
order: 16
title: default
keys: ["default"]
---

default runs when no case of the switch matches.

---
id: keyword/break
parent: language/builtins
order: 17
title: break
keys: ["break"]
---

break leaves the innermost for loop or switch.

---
id: keyword/continue
parent: language/builtins
order: 18
title: continue
keys: ["continue"]
---

continue jumps to the next iteration of the innermost for loop.

---
id: keyword/true
parent: language/builtins
order: 19
title: true
keys: ["true"]
---

true is the bool constant for yes.

---
id: keyword/false
parent: language/builtins
order: 20
title: false
keys: ["false"]
---

false is the bool constant for no.

---
id: keyword/nil
parent: language/builtins
order: 21
title: nil
keys: ["nil"]
---

nil is the empty reference. Only NodeRef, AssetRef and ComponentRef can be nil; reading through nil faults with AES6004.

```aes
if Target != nil {
}
```

---
id: keyword/export-attr
parent: language/builtins
order: 22
title: @export
keys: ["@export"]
---

@export saves the field in the scene and shows it in the Inspector.
Arguments: min, max, step (numbers only), unit, display, tip, category.

```aes
@export(min=0, max=600, unit="px/s")
var Speed float = 220
```

---
id: keyword/former-attr
parent: language/builtins
order: 23
title: @former
keys: ["@former"]
---

@former lists old names so scenes saved before a rename still load; the scene is rewritten with the new name on save.

```aes
@former("Velocity")
@export var Speed float
```

---
id: keyword/hide-attr
parent: language/builtins
order: 24
title: @hide
keys: ["@hide"]
---

@hide keeps the script out of the Add Script menu. Existing attachments keep working.

---
id: keyword/ready
parent: language/builtins
order: 25
title: Ready
keys: ["Ready"]
---

Ready runs once after every instance in the scene exists and references are resolved, in scene pre-order.

```aes
func Ready() {
}
```

---
id: keyword/update
parent: language/builtins
order: 26
title: Update
keys: ["Update"]
---

Update runs every frame. dt is the frame time in seconds.

```aes
func Update(dt float) {
}
```

---
id: keyword/fixedupdate
parent: language/builtins
order: 27
title: FixedUpdate
keys: ["FixedUpdate"]
---

FixedUpdate runs every fixed step, before physics. dt is the fixed step, 1/60 by default.

```aes
func FixedUpdate(dt float) {
}
```

---
id: keyword/reloaded
parent: language/builtins
order: 28
title: Reloaded
keys: ["Reloaded"]
---

Reloaded runs after a hot reload for every instance whose script or a dependency changed. Ready does not run again.

```aes
func Reloaded() {
}
```

---
id: keyword/node
parent: language/builtins
order: 29
title: node
keys: ["node"]
---

node is the NodeRef of the node this instance is attached to. It exists in every script method.

```aes
t := node.Transform2D()
```

---
id: keyword/bool
parent: language/builtins
order: 30
title: bool
keys: ["bool"]
---

bool is true or false. Conditions of if and for must be bool.

---
id: keyword/int
parent: language/builtins
order: 31
title: int
keys: ["int"]
---

int is a signed 64-bit integer that wraps on overflow. Convert with float(i) and int(f); there is no implicit conversion.

---
id: keyword/float
parent: language/builtins
order: 32
title: float
keys: ["float"]
---

float is a 32-bit IEEE number. float → int conversion saturates and maps NaN to 0.

---
id: keyword/string
parent: language/builtins
order: 33
title: string
keys: ["string"]
---

string is immutable UTF-8 text. + concatenates, len counts bytes, string(x) converts bool, int and float.

---
id: keyword/vec2
parent: language/builtins
order: 34
title: Vec2
keys: ["Vec2"]
---

Vec2 is a 2D vector with lanes X and Y. Build it with a composite literal; it supports +, -, and * or / by a float.

```aes
v := Vec2{1, 0} * Speed
```

---
id: keyword/vec3
parent: language/builtins
order: 35
title: Vec3
keys: ["Vec3"]
---

Vec3 is a 3D vector with lanes X, Y and Z.

```aes
up := Vec3{0, 1, 0}
```

---
id: keyword/color
parent: language/builtins
order: 36
title: Color
keys: ["Color"]
---

Color has lanes R, G, B and A in linear space, from 0 to 1.

```aes
red := Color{1, 0, 0, 1}
```

---
id: keyword/quat
parent: language/builtins
order: 37
title: Quat
keys: ["Quat"]
---

Quat is a 3D rotation with lanes X, Y, Z and W. Build it with math.QuatEuler or math.QuatAxisAngle. Quats cannot be compared with ==.

---
id: keyword/rect
parent: language/builtins
order: 38
title: Rect
keys: ["Rect"]
---

Rect has lanes X, Y, W and H.

---
id: keyword/transform2d
parent: language/builtins
order: 39
title: Transform2D
keys: ["Transform2D"]
---

Transform2D holds Position Vec2, Rotation float in radians (clockwise) and Scale Vec2. It is a value: read it, change it, write it back.

```aes
t := node.Transform2D()
t.Position.X += 1
node.SetTransform2D(t)
```

---
id: keyword/transform3d
parent: language/builtins
order: 40
title: Transform3D
keys: ["Transform3D"]
---

Transform3D holds Position Vec3, Rotation Quat and Scale Vec3.

```aes
t := node.Transform3D()
t.Position.Y += 1
node.SetTransform3D(t)
```

---
id: keyword/noderef
parent: language/builtins
order: 41
title: NodeRef
keys: ["NodeRef"]
---

NodeRef references a scene node and may be nil. Exported NodeRef fields are set by dragging a node onto the field in the Inspector.

```aes
@export var Target NodeRef
```

---
id: keyword/assetref
parent: language/builtins
order: 42
title: AssetRef
keys: ["AssetRef"]
---

AssetRef<Texture | Scene | Prefab | Mesh> references an asset and may be nil. Assign it in the Inspector.

```aes
@export var Bullet AssetRef<Prefab>
```

---
id: keyword/componentref
parent: language/builtins
order: 43
title: ComponentRef
keys: ["ComponentRef"]
---

ComponentRef<T> references a component on a node and is re-validated on every access.

```aes
sr := node.Get<SpriteRenderer>()
if sr != nil {
	sr.FlipX = true
}
```

---
id: keyword/len
parent: language/builtins
order: 44
title: len
keys: ["len"]
---

len returns the number of elements of a slice or the number of bytes of a string.

```aes
n := len(items)
```

---
id: keyword/append
parent: language/builtins
order: 45
title: append
keys: ["append"]
---

append returns the slice with v added at the end. Assign the result back.

```aes
hits = append(hits, 5)
```

---
id: keyword/coroutine
parent: language/builtins
order: 46
title: coroutine
keys: ["coroutine"]
---

coroutine after a method signature makes the method a coroutine: it can pause with yield and continue on a later frame. Only script methods without a result can be coroutines, and lifecycle methods cannot. Run one with start Name(args).

```aes
func Blink() coroutine {
	yield wait(0.1)
}
```

---
id: keyword/start
parent: language/builtins
order: 47
title: start
keys: ["start"]
---

start runs a coroutine method of this script up to its first yield and returns its Coroutine handle. The coroutine belongs to the instance and is cancelled when the instance stops.

```aes
blink = start Blink()
```

---
id: keyword/yield
parent: language/builtins
order: 48
title: yield
keys: ["yield"]
---

yield pauses the current coroutine:
```text
  yield            next frame
  yield wait(s)    after s seconds of game time
  yield frames(n)  after n frames
  yield fixed(n)   after n fixed steps
  yield c          when coroutine c ends
```
yield is allowed only inside a coroutine (AES2050).

---
id: keyword/yield-wait
parent: language/builtins
order: 49
title: yield wait
keys: ["yield wait"]
---

yield wait(seconds float) resumes after the given game time has passed, at the earliest on the next frame.

```aes
yield wait(0.25)
```

---
id: keyword/yield-frames
parent: language/builtins
order: 50
title: yield frames
keys: ["yield frames"]
---

yield frames(n int) resumes after n frames; frames(1) is the same as a bare yield.

```aes
yield frames(3)
```

---
id: keyword/yield-fixed
parent: language/builtins
order: 51
title: yield fixed
keys: ["yield fixed"]
---

yield fixed(n int) resumes after n fixed steps, before physics; use it for movement that must stay in step with FixedUpdate.

```aes
yield fixed(1)
```

---
id: keyword/signal
parent: language/builtins
order: 52
title: signal
keys: ["signal"]
---

signal declares an event of the script. Only the script itself can Emit it; other scripts Connect to it through a ScriptRef.

```aes
signal Died(reason string)
```

---
id: keyword/emit
parent: language/builtins
order: 53
title: Emit
keys: ["Emit"]
---

Emit calls every connected handler synchronously, in connection order. Connections added or removed by a handler apply after this Emit returns.

```aes
Died.Emit("fall")
```

---
id: keyword/connect
parent: language/builtins
order: 54
title: Connect
keys: ["Connect"]
---

Connect registers a method with the same parameters as the signal and returns a Connection. The handler must be a plain method: not a coroutine and not a lifecycle method.

```aes
conn = enemy.Died.Connect(OnEnemyDied)
```

---
id: keyword/disconnect
parent: language/builtins
order: 55
title: Disconnect
keys: ["Disconnect"]
---

Disconnect removes the handler; calling it twice is harmless.

```aes
conn.Disconnect()
```

---
id: keyword/connected
parent: language/builtins
order: 56
title: Connected
keys: ["Connected"]
---

Connected reports whether the connection is still active. It becomes false after Disconnect or when the source or the handler owner stops.

---
id: keyword/stop
parent: language/builtins
order: 57
title: Stop
keys: ["Stop"]
---

Stop cancels the coroutine; it does not run again. Stopping a finished coroutine is harmless.

```aes
blink.Stop()
```

---
id: keyword/running
parent: language/builtins
order: 58
title: Running
keys: ["Running"]
---

Running reports whether the coroutine is still alive (not finished, cancelled or faulted).

```aes
if !blink.Running() {
	blink = start Blink()
}
```

---
id: keyword/coroutine-type
parent: language/builtins
order: 59
title: Coroutine
keys: ["Coroutine"]
---

Coroutine is a handle returned by start. The zero value is nil and is never running.

---
id: keyword/connection
parent: language/builtins
order: 60
title: Connection
keys: ["Connection"]
---

Connection is a handle returned by Connect. Keep it in a field to Disconnect later.

---
id: keyword/scriptref
parent: language/builtins
order: 61
title: ScriptRef
keys: ["ScriptRef"]
---

ScriptRef<T> references a script instance on a node. Through it you read and write fields, call plain methods and connect to signals of T. Lifecycle methods and coroutines are not reachable (AES2059). It becomes invalid when the instance stops; access then faults with AES6004.

```aes
var enemy ScriptRef<EnemyAI>
```

---
id: keyword/getscript
parent: language/builtins
order: 62
title: GetScript
keys: ["GetScript"]
---

GetScript<T>() returns the instance of script T on the node, or nil when the node has none. Use pkg.Name when the short name is ambiguous.

```aes
e := Target.GetScript<EnemyAI>()
if e != nil {
	e.Died.Connect(OnEnemyDied)
}
```

---
id: keyword/copy
parent: language/builtins
order: 63
title: Copy
keys: ["Copy"]
---

Returns a copy of a data asset as a struct the script owns. Changing the copy does not change the asset.

```aes
s := Stats.Copy()
```

---
id: keyword/row
parent: language/builtins
order: 64
title: Row
keys: ["Row"]
---

Returns the row of a table with the given id. A missing row stops the script with AES6004; check with Has first.

```aes
entry := Loot.Row("goblin")
```

---
id: keyword/has
parent: language/builtins
order: 65
title: Has
keys: ["Has"]
---

Reports whether a table has a row with the given id.

---
id: keyword/table-len
parent: language/builtins
order: 66
title: Len
keys: ["Len"]
---

Returns the number of rows in a table.
