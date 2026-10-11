# Aego Script — Language Reference (v1)

Aego Script is the gameplay language of the Aego engine. It is statically typed, uses Go-like syntax, compiles to a register bytecode and runs inside the engine's VM. Scripts attach to scene nodes, expose fields to the Inspector, read and write engine components, and call functions exposed by the engine or by a project's native Go code.

This document is the normative reference for v1 (Stage 5). Features planned for the next stage are listed at the end and are not available yet.

---

## 1. Overview

```aes
package player

import (
	"aego/input"
	"aego/log"
	"aego/math"
	"mygame/combat"
)

enum State {
	Idle
	Run
}

type Stats struct {
	Hp    int
	Armor float
}

const MaxSpeed float = 600

script PlayerController {
	@export(min=0, max=600, unit="px/s")
	var Speed float = 220

	@export
	var Target NodeRef

	var state State
	var stats Stats
	var hits []int

	func Ready() {
		stats = Stats{Hp: 10, Armor: 0.5}
		log.Info("ready")
	}

	func Update(dt float) {
		x := input.Axis("horizontal")
		if x != 0 {
			state = State.Run
		} else {
			state = State.Idle
		}
		t := node.Transform2D()
		t.Position.X += math.Clamp(x*Speed, -MaxSpeed, MaxSpeed) * dt
		node.SetTransform2D(t)

		sr := node.Get<SpriteRenderer>()
		if sr != nil {
			sr.FlipX = x < 0
		}
		if Target != nil && input.Pressed(input.KeySpace) {
			combat.Damage(Target, 5)
		}
	}
}
```

Attach `PlayerController` to a node in the editor, set `Speed` and `Target` in the Inspector, press Play.

---

## 2. Files and packages

- Source files use the `.aes` extension and live under `scripts/` in the project. Any directory under `scripts/` is a package; files in one directory belong to one package.
- The package path is `project.module + "/" + directory` (`mygame/player`). The `package` clause must equal the directory's base name (`package player`); a mismatch is `AES4001`.
- A module path uses `[a-z0-9_/-]` only, no dots: qualified names (`mygame/player.State`) are split on the last dot.
- `import` works as in Go; there are no import aliases. Importing a package that is not under `scripts/` and not a host module is `AES4002`; cycles are `AES4003`. Unused imports are warnings.
- Host modules (`aego/input`, `mygame/combat`) and script packages share one namespace. The `aego/` prefix is reserved for the standard library; `module = "aego"` is a project error.
- Visibility follows Go: identifiers starting with an uppercase letter are exported from the package; everything else is package-private. `@export` is unrelated to visibility (see §8).
- `*_test.aes` files are compiled only by `aego test`.

A package that fails to compile keeps its last good build; its files are marked invalid and Play is blocked until they compile again.

---

## 3. Lexical structure

- **Identifiers** are ASCII only: `[A-Za-z_][A-Za-z0-9_]*`. A non-ASCII letter where an identifier could start is `AES1001`. Non-ASCII text is fully supported inside string literals.
- **Keywords**: `package import script type struct enum var const func return if else for switch case default break continue true false nil`.
- **Reserved for future versions** (using them is `AES1004`): `yield signal coroutine await select go chan defer interface map`.
- **Literals**: integers `10`, `0x1f`, `1_000`; floats `1.5`, `.5`, `1e3`, `1_000.5`; strings `"..."` with escapes `\n \t \" \\ \uXXXX`; `true`, `false`, `nil`. Underscores must separate digits.
- **Comments**: `// line` and `/* block */`. The source formats of the engine (`.aescene`, `project.aego`) have no comments; scripts do.
- **Semicolons** are inserted at a line end after an identifier, literal, `)`, `]`, `}`, `return`, `break`, `continue`, `++`, `--`, or after a `>` that closes type arguments. Write code line by line as in Go; `}` followed by `else` must be on the same line.
- **Operators**: `+ - * / % == != < <= > >= && || ! = := += -= *= /= %= ++ --`. There are no bitwise operators; a lone `&` or `|` is `AES1001`.

Deliberately unsupported lexical forms (each has a diagnostic that names the intended form):

| Input | Why | Write instead |
|---|---|---|
| `'a'` | no `char` type separate from `string` | `"a"` |
| `1.` / `1.e3` | a float needs digits after the point | `1.0` / `1.0e3` |
| `имя := 1` | identifiers are ASCII | ASCII names |
| `a & b` | no bitwise arithmetic | `&&` / `\|\|` |

---

## 4. Grammar

```
File        = "package" ident ";" { Import ";" } { TopDecl ";" } .
Import      = "import" ( string | "(" { string ";" } ")" ) .
TopDecl     = ConstDecl | TypeDecl | EnumDecl | FuncDecl | ScriptDecl .
ConstDecl   = "const" ident [ Type ] "=" Expr .
TypeDecl    = "type" ident "struct" "{" { ident Type ";" } "}" .
EnumDecl    = "enum" ident "{" { ident ";" } "}" .
FuncDecl    = { Attr } "func" ident "(" [ ident Type { "," ident Type } ] ")" [ Type ] Block .
ScriptDecl  = { Attr } "script" ident "{" { ScriptMember ";" } "}" .
ScriptMember= { Attr } "var" ident Type [ "=" Expr ] | FuncDecl .
Attr        = "@" ident [ "(" [ AttrArg { "," AttrArg } ] ")" ] .
AttrArg     = [ ident "=" ] ( Literal | "-" Literal ) .
Type        = ident [ "." ident ] [ "<" Type { "," Type } ">" ] | "[" "]" Type .
Block       = "{" { Stmt ";" } "}" .
Stmt        = "var" ident Type [ "=" Expr ]
            | ident ":=" Expr
            | LValue ( "=" | "+=" | "-=" | "*=" | "/=" | "%=" ) Expr
            | LValue ( "++" | "--" )
            | Expr | If | For | Switch
            | "return" [ Expr ] | "break" | "continue" | Block .
If          = "if" Expr Block [ "else" ( If | Block ) ] .
For         = "for" [ Expr | [ Simple ] ";" [ Expr ] ";" [ Simple ] ] Block .
Switch      = "switch" Expr "{" { ( "case" Expr { "," Expr } | "default" ) ":" { Stmt ";" } } "}" .
Expr        = Unary { BinOp Unary } .
Unary       = [ "-" | "!" ] Primary .
Primary     = Operand { "." ident | "[" Expr "]" | [ "<" Type ">" ] "(" [ Expr { "," Expr } ] ")" } .
Operand     = Literal | ident | "(" Expr ")" | Type "{" [ Elems ] "}" .
```

Precedence, high to low: unary `-` `!`; `* / %`; `+ -`; `== != < <= > >=`; `&&`; `||`. Type arguments in a call (`Get<T>()`) are accepted by the parser anywhere, but only intrinsics may use them (§9).

Composite literals are not allowed directly in the head of `if`, `for` or `switch`; parenthesize them (`if x == (Stats{}) {`).

---

## 5. Types

| Type | Description |
|---|---|
| `bool` | `true` / `false` |
| `int` | signed 64-bit; wraps on overflow |
| `float` | IEEE float32 |
| `string` | immutable UTF-8; `+` concatenation, comparison, `len` in **bytes**; no indexing |
| `Vec2 Vec3 Color Quat Rect` | engine value types; lanes `.X .Y .Z .W`, `.R .G .B .A`, `.X .Y .W .H` |
| `Transform2D{Position Vec2, Rotation float, Scale Vec2}` | value struct |
| `Transform3D{Position Vec3, Rotation Quat, Scale Vec3}` | value struct |
| `NodeRef` | reference to a node; may be `nil` |
| `AssetRef<Texture \| Scene \| Prefab \| Mesh>` | reference to an asset; may be `nil` |
| `ComponentRef<T>` | reference to a component on a node; may be `nil` |
| `struct` | package-level, value semantics, copied on assignment |
| `enum` | package-level, `int`-backed, members `State.Idle` |
| `[]T` | growable slice on the VM heap |

Rules:

- No implicit numeric conversion. Use `int(x)`, `float(x)`, `string(x)` (from `bool`, `int`, `float`, `string`), `int(e)` for an enum and `State(i)` for int → enum. Untyped constants adapt to the context (`var f float = 1` is fine, `var i int = 1.5` is not).
- `float` arithmetic is float32 everywhere, including constant folding.
- `==`/`!=` work on scalars, strings, enums, references, vectors, structs with comparable fields, and `Transform*`; `Quat` and slices are not comparable. `< <= > >=` work on numbers and strings.
- Vector arithmetic: `v + w`, `v - w`, `v * s`, `s * v`, `v / s`, `-v`; vectors are constructed only with composite literals (`Vec2{x, y}`, `Color{1, 0, 0, 1}`), never with call syntax.
- A struct is a value: `a = b` copies; passing to a function copies. Nested structs are allowed; a struct that contains itself by value is `AES2007`.
- A slice element may occupy at most 32 value slots (`AES2032`); scalars, vectors, refs and small structs are far below it.
- `nil` is only for `NodeRef`, `AssetRef`, `ComponentRef`. Reading or writing through a nil reference is a runtime fault.

---

## 6. Declarations and statements

- `var x T`, `var x T = e` — explicit type; `x := e` — inferred from `e` (untyped constants default to `int`/`float`); `nil` cannot initialise `:=`.
- Package-level `const` only; package-level `var` is `AES4004`. State lives in script fields and components.
- Functions have fixed arity and zero or one result. A function that declares a result must end in a terminating statement (`AES2040`). Recursion is allowed up to depth 256.
- `if`, `if … else if … else`; `for cond {}`, `for init; cond; post {}`, `for {}`; `break`, `continue`.
- `switch x { case a, b: … default: … }` — no fallthrough; a `switch` on an enum must cover every member or have a `default` (`AES2041`); duplicate cases are errors.
- Compound assignment `+= -= *= /= %=` and `++`/`--` on `int`/`float` lvalues; lvalues are variables, fields, lanes, slice elements and component fields.
- A statement-level expression must be a call; an unused value is an error. Discarding the result of `len`, `append`, a conversion or `Get<T>` is an error.
- Unused local variables are warnings.
- There are no closures, lambdas, defer, goroutines, pointers or maps.

---

## 7. Scripts and lifecycle

```aes
script Enemy {
	@export var Speed float = 120
	var elapsed float

	func Ready() {}
	func FixedUpdate(dt float) {}
	func Update(dt float) {}

	func helper() int { return 1 }
}
```

- A `script` is a stateful type that attaches to a node. A node holds at most one instance of each script type; different script types may share a node.
- Every script has an implicit field `node NodeRef`. Naming a field or method `node` is an error.
- Methods are declared inside the script and called unqualified from other methods. Package-level functions see no script state and cannot use `node`.
- Fields declared with `var` are runtime state: they are created when Play starts and discarded on Stop. Only `@export` fields are saved in the scene and shown in the Inspector. Play never writes back to the edit scene.
- Field initialisers must be constants (or `nil` for references).

Lifecycle methods are recognised by name and must have exactly these signatures:

| Method | When |
|---|---|
| `func Ready()` | once, after every instance in the scene exists and references are resolved, in scene pre-order |
| `func FixedUpdate(dt float)` | every fixed step, before physics, in instance creation order |
| `func Update(dt float)` | every frame, in instance creation order |

Instances that do not declare a method are not visited for it.

---

## 8. Attributes

| Attribute | Target | Meaning |
|---|---|---|
| `@export` | field | saved in the scene, shown in the Inspector |
| `@export(min=0, max=100, step=1, unit="px/s", display="Speed", tip="…", category="Movement")` | field | same, with Inspector hints; `min/max/step` only on numeric fields |
| `@former("OldName")` | script or field | the old name still loads from scenes; the scene is rewritten with the new name on save |
| `@hide` | script | not offered in Add Script |

Exportable field types: `bool int float string Vec2 Vec3 Color Quat Rect`, enums, `NodeRef`, `AssetRef<T>`. Slices, structs and `ComponentRef` cannot be exported (`AES3001`).

Attribute arguments are literals (optionally negative). Attributes before `func` are parsed for forward compatibility but rejected in v1 (`AES3005`).

Unknown properties found in a scene (a field that was removed or renamed without `@former`) are preserved and re-saved with a warning; they are not applied at runtime.

---

## 9. Nodes, components and references

### NodeRef

| Method | |
|---|---|
| `IsAlive() bool`, `Name() string`, `Parent() NodeRef`, `Child(name string) NodeRef`, `Find(path string) NodeRef`, `Destroy()` | hierarchy; `Find` accepts `Weapon/Gun`, `../Sibling`, `/Root/Player` |
| `Enabled() bool`, `SetEnabled(bool)`, `Space() int` | state; `Space` is 0 for 2D, 1 for 3D |
| `Transform2D() Transform2D`, `SetTransform2D(t)`, `Position() Vec2`, `SetPosition(Vec2)`, `WorldPosition() Vec2`, `Rotation() float`, `SetRotation(float)`, `Scale() Vec2`, `SetScale(Vec2)` | 2D nodes |
| `Transform3D() Transform3D`, `SetTransform3D(t)`, `Position3() Vec3`, `SetPosition3(Vec3)`, `Rotation3() Quat`, `SetRotation3(Quat)` | 3D nodes |
| `Get<T>() ComponentRef<T>`, `Has<T>() bool` | components |

Transforms are read and written by value: `t := node.Transform2D(); t.Position.X += 1; node.SetTransform2D(t)`. A 2D method on a 3D node (and the reverse) faults. `Destroy` takes effect at the end of the phase; `IsAlive` is false immediately. `Get<T>()` on a destroyed node returns `nil`; on a `nil` reference it faults.

### Components

`node.Get<SpriteRenderer>()` returns a `ComponentRef<SpriteRenderer>` or `nil`. Field names are the schema names with an upper-cased first letter (`sr.FlipX`, `hp.Current`); read-only fields cannot be assigned. A short component name works when it is unique among registered components; otherwise write the full name (`engine.SpriteRenderer`). Components registered by the project's native Go code are available the same way. In v1 an enum field of a Go component is exposed as `int`.

A `ComponentRef` is safe to keep in a field: it is re-validated on every access and faults with `AES6004` if the component or node is gone.

### AssetRef

`IsNil() bool`, `Name() string`. Assets are assigned in the Inspector; scripts pass them to engine functions (`scene.Instantiate`).

---

## 10. Standard library

`aego/math` — `Pi Tau`; `Floor Ceil Round Trunc Sqrt Sin Cos Tan Asin Acos Exp Log Abs Sign Min Max Clamp Lerp Atan2 Pow Deg2Rad Rad2Deg`; `AbsInt MinInt MaxInt ClampInt`; Vec2: `Length Normalize Dot Cross Dist Rotate Angle LerpVec2`; Vec3: `Length3 Normalize3 Dot3 Cross3 Dist3 LerpVec3`; Quat: `QuatEuler QuatAxisAngle QuatMul QuatRotate QuatSlerp QuatLookDir`. There is no overloading: Vec3 variants carry a `3` suffix, int variants an `Int` suffix.

`aego/log` — `Debug Info Warn Error(msg string)`. Build messages with `+` and `string(x)`.

`aego/rand` — `Float() float` in `[0,1)`, `Bool() bool`, `Int(min, max int) int` in `[min, max)`, `Range(min, max float) float`, `Seed() int`. The source is the world's seeded generator; a replay with the same seed reproduces the numbers.

`aego/time` — `Delta() float`, `Fixed() float`, `Elapsed() float`, `Frame() int`.

`aego/input` — `Down(k) Pressed(k) Released(k) bool`, `MouseDown(b) MousePressed(b) bool`, `MousePos() Vec2`, `Axis(name string) float`. Keys are `input.Key.Space` or the constants `input.KeySpace`, digits `input.KeyDigit1`; buttons `input.MouseLeft`. Axes are declared in `project.aego`:

```
input {
  axes {
    horizontal { negative=A positive=D altNegative=Left altPositive=Right }
    vertical   { negative=S positive=W altNegative=Down altPositive=Up }
  }
}
```

`aego/scene` — `Find(path string) NodeRef` from the scene root, `Instantiate(prefab AssetRef<Prefab>, parent NodeRef) NodeRef`, `Destroy(NodeRef)`. New nodes appear at the end of the phase; their scripts receive `Ready` before the next `Update`.

`aego/testing` — available only under `aego test`: `True(cond bool, msg string)`, `False(…)`, `Equal(got, want int)`, `EqualFloat(got, want, eps float)`, `EqualString(got, want string)`, `Fail(msg string)`. A failed assertion stops the test and reports the file and line.

---

## 11. Native bindings

Project Go code in `native/` exposes functions through the SDK:

```go
m := sdk.ScriptModule(r, "mygame/combat")
sdk.Proc2(m, "Damage", func(c *sdk.ScriptCtx, target sdk.NodeHandle, amount int64) {
	if h, ok := scene.Get[Health](c.Scene, target); ok {
		h.Current -= float32(amount)
	}
})
```

Scripts `import "mygame/combat"` and call `combat.Damage(target, 5)`. Use `Func0..Func3` for functions with a result, `Proc0..Proc3` without, `Const` and `Enum` for constants and host enums. Allowed parameter and result types: `bool`, Go integer types, `float32/float64`, `string`, `mathx.Vec2/Vec3/Color/Quat/Rect`, `scene.Transform2D/Transform3D`, `scene.NodeHandle` (a `NodeRef`), `asset.Ref[…]`, and integer types registered with `Enum`. Structs and slices cannot cross the boundary. `c.Fail(msg)` stops the calling instance with `AES6010`; a Go panic becomes `AES6002`.

The binding registry's hash covers every module, name and signature and is part of the compile cache key, so changing a native signature recompiles the scripts that use it.

---

## 12. Runtime behaviour

- A fault stops only the instance that caused it (its methods are no longer called); the rest of the scene keeps running. The Console shows the script stack; double-click opens the file.
- Each callback has an instruction budget (default 100 000); an infinite loop ends with `AES6001`.
- Integer division by zero faults; `MinInt / -1` yields `MinInt`, `MinInt % -1` yields `0`. `float → int` conversion saturates and maps NaN to `0`, identically on every platform.
- Strings and slices are garbage-collected between callbacks; scripts that do not allocate produce no garbage.

| Code | Meaning |
|---|---|
| AES6001 | instruction budget exceeded |
| AES6002 | native binding panicked |
| AES6003 | call depth over 256 |
| AES6004 | nil or dead reference |
| AES6005 | index out of range |
| AES6006 | integer division by zero |
| AES6007 | invalid string or slice handle |
| AES6008 | `missing return` reached |
| AES6009 | unresolved extern or binding |
| AES6010 | host function reported an error |

---

## 13. Limits

255 registers per function · 32 767 instructions per function · 32 value slots per slice element · 255 fields per component · call depth 256 · 100 000 instructions per callback (configurable).

---

## 14. Diagnostics

Compiler diagnostics have a code, file, line and column, and often a note:

```
scripts/player/player.aes:17:21 AES2103 cannot use NodeRef as float in assignment
  note: field Speed has type float
```

`AES1xxx` lexical/syntax · `AES2xxx` types · `AES3xxx` attributes · `AES4xxx` packages and imports · `AES5xxx` bindings · `AES6xxx` runtime faults. The parser recovers after an error, so one run reports every problem in a file.

---

## 15. CLI

| Command | |
|---|---|
| `aego check [dir] [--manifest file]` | parse every document and type-check scripts against engine and native bindings |
| `aego test [dir] [-run name] [-v]` | run `TestXxx()` functions from `*_test.aes` headless |
| `aego dump script <pkg\|Type> [--func name]` | bytecode with `; file:line source` comments |
| `aego run [dir]` | refuses to start while a script package has errors |

`aego check` on a project with `native/` builds a small helper that only calls the module's `Register` and writes a bindings manifest (`.aego/cache/script-bindings`). It is rebuilt when `native/*.go`, `go.mod`/`go.sum` or the engine change. A native compile error or a failing `Register` is exit code 2. `--manifest` checks against a given manifest file instead.

Native functions can carry documentation for hover and completion:

```go
sdk.Proc2(m, "Damage", damage, sdk.ScriptDoc("deals damage to a node with Health"))
```

A `//` comment group directly above a declaration is its documentation.

---

## 16. Hot reload

Saving a script while the game runs (`aego run`, Play in the editor) swaps the code without restarting. The workspace builds a new generation from the saved files; at the start of the next frame (`script.swap`, `PhaseFrameBegin`, the VM stack is empty) every module of that generation is loaded into the same VM, every live instance is moved into the new layout and the dynamic schema is replaced in the same transaction. If anything fails, nothing changes and the previous generation keeps running. A save that does not compile never produces a generation.

Instances keep their identity: `InstanceID`, node, heap strings and slices survive. Fields are matched by name (or `@former`), never by slot:

| Change | Result |
|---|---|
| same type | value kept |
| new field | its default |
| removed field | dropped |
| incompatible type (`float` → `string`) | default + warning, counted as reset |
| enum | by member name; a removed member resets the field |
| struct | field by field, recursively |
| `[]T` | element by element; incompatible element type resets the slice |
| `NodeRef`, `AssetRef`, `ComponentRef` | kept, refs stay valid in the running scene |

`Ready` does not run again. A script may declare `func Reloaded()`; it runs after the swap for every instance of a script whose package (or a dependency's interface) changed. Instances that faulted in a changed script resume running. Instances of a removed script stop. Changes in `native/` are never swapped in-process: the supervisor rebuilds and restarts.

The editor has a built-in code editor: double-click a `.aes` file, or a script problem in the Console. Diagnostics, hover (F12 goes to the definition, Shift+F12 lists references), completion (Ctrl+Space, also after `.`, `@`, `Get<`), find/replace (Ctrl+F, Ctrl+H), Ctrl+/ toggles comments, Tab/Shift+Tab indent. Unsaved edits are analyzed but never run.

---

## 17. Planned (Stage 6)

Not available in v1; the keywords are reserved so adding them breaks nothing.

- **Coroutines**: `func Blink() coroutine { … yield wait(0.1) … }`, started with `start Blink()`, owned by the instance and cancelled with it.
- **Signals**: `signal Died(reason string)`, `Died.Emit("fall")`, `Died.Connect(OnDied)`; cross-script via `node.GetScript<T>()` and `ScriptRef<T>`.
