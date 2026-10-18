---
id: language
title: Aego Script
order: 1
---
# Aego Script

Aego Script is the gameplay language of the Aego engine. It is statically typed, uses Go-like syntax, compiles to a register bytecode and runs inside the engine's VM. Scripts attach to scene nodes, expose fields to the Inspector, read and write engine components, call functions exposed by the engine or by a project's native Go code, and talk to each other through signals and script references.

This document is the normative reference for language version 2 (Stages 5–6). Language version 1 modules are no longer loaded; projects are recompiled on open.

---
id: language/overview
parent: language
order: 1
title: Overview
---
# Overview

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
		if Target != nil && input.KeyPressed(input.KeySpace) {
			combat.Damage(Target, 5)
		}
	}
}
```

Attach `PlayerController` to a node in the editor, set `Speed` and `Target` in the Inspector, press Play.

---
id: language/files-and-packages
parent: language
order: 2
title: Files and packages
---
# Files and packages

- Source files use the `.aes` extension and live under `scripts/` in the project. Any directory under `scripts/` is a package; files in one directory belong to one package.
- The package path is `project.module + "/" + directory` (`mygame/player`). The `package` clause must equal the directory's base name (`package player`); a mismatch is `AES4001`.
- A module path uses `[a-z0-9_/-]` only, no dots: qualified names (`mygame/player.State`) are split on the last dot.
- `import` works as in Go; there are no import aliases. Importing a package that is not under `scripts/` and not a host module is `AES4002`; cycles are `AES4005`. Unused imports are warnings.
- Host modules (`aego/input`, `mygame/combat`) and script packages share one namespace. A script directory whose package path equals a host module path is `AES4010`. The `aego/` prefix is reserved for the standard library; `module = "aego"` is a project error.
- Visibility follows Go: identifiers starting with an uppercase letter are exported from the package; everything else is package-private. `@export` is unrelated to visibility (see §8).
- `*_test.aes` files are compiled only by `aego test`.

A package that fails to compile keeps its last good build; its files are marked invalid and Play is blocked until they compile again.

---
id: language/lexical-structure
parent: language
order: 3
title: Lexical structure
---
# Lexical structure

- **Identifiers** are ASCII only: `[A-Za-z_][A-Za-z0-9_]*`. A non-ASCII letter where an identifier could start is `AES1001`. Non-ASCII text is fully supported inside string literals.
- **Keywords**: `package import script type struct enum var const func return if else for switch case default break continue true false nil yield signal coroutine`.
- **Contextual**: `start` is a keyword only when it is followed by a name and `(`; elsewhere it is an ordinary identifier. `wait`, `frames` and `fixed` are special only right after `yield`.
- **Reserved for future versions** (using them is `AES1004`): `await select go chan defer interface map`.
- **Literals**: integers `10`, `0x1f`, `1_000`; floats `1.5`, `.5`, `1e3`, `1_000.5`; strings `"..."` with escapes `\n \t \" \\ \uXXXX`; `true`, `false`, `nil`. Underscores must separate digits.
- **Comments**: `// line` and `/* block */`. A `//` comment group directly above a declaration is its documentation and is shown on hover and in completion.
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
id: language/grammar
parent: language
order: 4
title: Grammar
---
# Grammar

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

Precedence, high to low: unary `-` `!`; `* / %`; `+ -`; `== != < <= > >=`; `&&`; `||`. Type arguments in a call (`Get<T>()`) are accepted by the parser anywhere, but only intrinsics may use them (§9).

Composite literals are not allowed directly in the head of `if`, `for` or `switch`; parenthesize them (`if x == (Stats{}) {`).

---
id: language/types
parent: language
order: 5
title: Types
---
# Types

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
| `ScriptRef<T>` | reference to an instance of script `T`; may be `nil`; fields, plain methods and signals of `T` |
| `Coroutine` | handle returned by `start`; may be `nil`; methods `Stop()`, `Running() bool` |
| `Connection` | handle returned by `Connect`; may be `nil`; methods `Disconnect()`, `Connected() bool` |
| `Curve` | read-only view of a curve field of a component (`fader.Alpha`); evaluated with `aego/curve` |
| `tween.Tween`, `timer.Timer`, … | host handles returned by engine and native modules; may be `nil`; methods come from the module |
| `struct` | package-level, value semantics, copied on assignment |
| `enum` | package-level, `int`-backed, members `State.Idle` |
| `[]T` | growable slice on the VM heap |

Rules:

- No implicit numeric conversion. Use `int(x)`, `float(x)`, `string(x)` (from `bool`, `int`, `float`, `string`), `int(e)` for an enum and `State(i)` for int → enum. Untyped constants adapt to the context (`var f float = 1` is fine, `var i int = 1.5` is not).
- `float` arithmetic is float32 everywhere, including constant folding.
- `==`/`!=` work on scalars, strings, enums, references, handles, vectors, structs with comparable fields, and `Transform*`; `Quat`, `Curve` and slices are not comparable. `< <= > >=` work on numbers and strings.
- Vector arithmetic: `v + w`, `v - w`, `v * s`, `s * v`, `v / s`, `-v`; vectors are constructed only with composite literals (`Vec2{x, y}`, `Color{1, 0, 0, 1}`), never with call syntax.
- A struct is a value: `a = b` copies; passing to a function copies. Nested structs are allowed, including structs from other packages; a struct that contains itself by value is `AES2017`.
- A slice element may occupy at most 32 value slots (`AES2032`); scalars, vectors, refs and small structs are far below it.
- `nil` is valid for `NodeRef`, `AssetRef`, `ComponentRef`, `ScriptRef`, `Coroutine`, `Connection` and host handles. Reading or writing through a nil reference is a runtime fault (`AES6004`); calling a method on a nil `Coroutine`, `Connection` or host handle is not: `Running()` and `Connected()` return `false`, `Done()` returns `true`, `Stop()`, `Cancel()` and `Disconnect()` do nothing.

---
id: language/declarations-and-statements
parent: language
order: 6
title: Declarations and statements
---
# Declarations and statements

- `var x T`, `var x T = e` — explicit type; `x := e` — inferred from `e` (untyped constants default to `int`/`float`); `nil` cannot initialise `:=`.
- Package-level `const` only; package-level `var` is `AES1105`. State lives in script fields and components.
- Functions have fixed arity and zero or one result. A function that declares a result must end in a terminating statement (`AES2013`). Recursion is allowed up to depth 256.
- `if`, `if … else if … else`; `for cond {}`, `for init; cond; post {}`, `for {}`; `break`, `continue`.
- `switch x { case a, b: … default: … }` — no fallthrough; a `switch` on an enum must cover every member or have a `default` (`AES2015`); duplicate cases are errors.
- Compound assignment `+= -= *= /= %=` and `++`/`--` on `int`/`float` lvalues; lvalues are variables, fields, lanes, slice elements, component fields and fields reached through a `ScriptRef`.
- A statement-level expression must be a call; an unused value is an error. Discarding the result of `len`, `append`, a conversion, `Get<T>`, `GetScript<T>` or `Connect` is an error. Discarding the result of `start` is allowed.
- Unused local variables are warnings.
- There are no closures, lambdas, defer, goroutines, pointers or maps.

---
id: language/scripts-and-lifecycle
parent: language
order: 7
title: Scripts and lifecycle
---
# Scripts and lifecycle

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

- A `script` is a stateful type that attaches to a node. A node holds at most one instance of each script type; different script types may share a node.
- Every script has an implicit field `node NodeRef`. Naming a field, method or signal `node` is an error.
- Methods are declared inside the script and called unqualified from other methods. Package-level functions see no script state and cannot use `node`, `start`, `yield` or signals.
- Fields declared with `var` are runtime state: they are created when Play starts and discarded on Stop. Only `@export` fields are saved in the scene and shown in the Inspector. Play never writes back to the edit scene.
- Field initialisers must be constants (or `nil` for references and handles).

Lifecycle methods are recognised by name and must have exactly these signatures:

| Method | When |
|---|---|
| `func Ready()` | once, after every instance in the scene exists and references are resolved, in scene pre-order |
| `func FixedUpdate(dt float)` | every fixed step, before physics, in instance creation order |
| `func Update(dt float)` | every frame, in instance creation order |
| `func Reloaded()` | after a hot reload, for every instance whose script or a dependency's interface changed (§19) |

Instances that do not declare a method are not visited for it; a scene with ten thousand instances that never declare `Update` costs nothing in the update phase. Lifecycle methods cannot be coroutines (`AES2056`), cannot be signal handlers (`AES2055`) and cannot be called through a `ScriptRef` (`AES2059`).

---
id: language/attributes
parent: language
order: 8
title: Attributes
---
# Attributes

| Attribute | Target | Meaning |
|---|---|---|
| `@export` | field | saved in the scene, shown in the Inspector |
| `@export(min=0, max=100, step=1, unit="px/s", display="Speed", tip="…", category="Movement")` | field | same, with Inspector hints; `min/max/step` only on numeric fields |
| `@former("OldName")` | script or field | the old name still loads from scenes; the scene is rewritten with the new name on save; also used to match fields across a hot reload |
| `@hide` | script | not offered in Add Script |

Exportable field types: `bool int float string Vec2 Vec3 Color Quat Rect`, enums, `NodeRef`, `AssetRef<T>`, `ComponentRef<T>`. Slices, structs, `ScriptRef`, `Coroutine`, `Connection`, `Curve` and host handles cannot be exported (`AES3001`). An exported `ComponentRef<T>` is stored as a node reference; the Inspector only offers nodes that have `T`, and a node without `T` starts the field as `nil`.

A field default must be a constant. Composite literals of `Vec2`, `Vec3`, `Color`, `Quat` and `Rect` whose components are constants count as constants: `var Jump Vec2 = Vec2{0, -5}`, `var Tint Color = Color{R: 1, A: 1}`. The Inspector shows them as the default, and a scene that omits the property starts with them.

Attribute arguments are literals (optionally negative). Attributes before `func` are parsed for forward compatibility but rejected (`AES3005`).

Unknown properties found in a scene (a field that was removed or renamed without `@former`) are preserved and re-saved with a warning; they are not applied at runtime.

---
id: language/nodes-components-and-references
parent: language
order: 9
title: Nodes, components and references
---
# Nodes, components and references

## NodeRef

| Method | |
|---|---|
| `IsAlive() bool`, `Name() string`, `Parent() NodeRef`, `Child(name string) NodeRef`, `Find(path string) NodeRef`, `Destroy()` | hierarchy; `Find` accepts `Weapon/Gun`, `../Sibling`, `/Root/Player` |
| `Enabled() bool`, `SetEnabled(bool)`, `Space() int` | state; `Space` is 0 for 2D, 1 for 3D |
| `Transform2D() Transform2D`, `SetTransform2D(t)`, `Position() Vec2`, `SetPosition(Vec2)`, `WorldPosition() Vec2`, `Rotation() float`, `SetRotation(float)`, `Scale() Vec2`, `SetScale(Vec2)` | 2D nodes |
| `Transform3D() Transform3D`, `SetTransform3D(t)`, `Position3() Vec3`, `SetPosition3(Vec3)`, `Rotation3() Quat`, `SetRotation3(Quat)` | 3D nodes |
| `Get<T>() ComponentRef<T>`, `Has<T>() bool` | components |
| `GetScript<T>() ScriptRef<T>`, `HasScript<T>() bool` | scripts (§18) |

Transforms are read and written by value: `t := node.Transform2D(); t.Position.X += 1; node.SetTransform2D(t)`. A 2D method on a 3D node (and the reverse) faults. `Destroy` takes effect at the end of the phase; `IsAlive` is false immediately. `Get<T>()` and `GetScript<T>()` on a destroyed node return `nil`; on a `nil` reference they fault with `AES6004`.

## Components

`node.Get<SpriteRenderer>()` returns a `ComponentRef<SpriteRenderer>` or `nil`. Field names are the schema names with an upper-cased first letter (`sr.FlipX`, `hp.Current`); read-only fields cannot be assigned. A short component name works when it is unique among registered components; otherwise write the full name (`render2d.SpriteRenderer`). An enum field of a Go component is exposed as `int`.

Components may also expose methods: `hp.Damage(5)`, `body.ApplyImpulse(Vec2{0, -300})`. Which fields and methods are visible is decided by the component's owner when it registers the component (§11); the generated library stubs (`Ctrl+Click` on a component name in the editor) show exactly what is available.

A `ComponentRef` is safe to keep in a field: it is re-validated on every access and faults with `AES6004` as soon as the component or its node is destroyed, even before the end of the phase.

## AssetRef

`IsNil() bool`, `Name() string`. Assets are assigned in the Inspector; scripts pass them to engine functions (`scene.Instantiate`).

---
id: language/native-bindings
parent: language
order: 11
title: Native bindings
---
# Native bindings

Project Go code in `native/` exposes functions, constants and component members through the SDK.

## Functions

```go
m := r.ScriptModule("mygame/combat")
sdk.Proc2(m, "Damage", func(c *sdk.ScriptCtx, target sdk.NodeHandle, amount int64) {
	if h, ok := sdk.Get[Health](c.Scene, target); ok {
		h.Current -= float32(amount)
	}
}, sdk.ScriptDoc("deals damage to a node with Health"))
```

Scripts `import "mygame/combat"` and call `combat.Damage(target, 5)`. Use `Func0..Func5` for functions with a result, `Proc0..Proc3` without, `Const` and `Enum` for constants and host enums. Allowed parameter and result types: `bool`, Go integer types, `float32/float64`, `string`, `mathx.Vec2/Vec3/Color/Quat/Rect`, `scene.Transform2D/Transform3D`, `scene.NodeHandle` (a `NodeRef`), `asset.Ref[…]`, `sdk.ScriptCurve` (a `Curve` parameter, resolved with `Resolve()`), integer types registered with `Enum` and Go types registered as handles. Structs and slices cannot cross the boundary. `c.Fail(msg)` stops the calling instance with `AES6010`; a Go panic becomes `AES6002`.

## Component fields and methods

A component registered through `sdk.Component[T]` exposes every field marked with the `script` schema flag by default. The builder narrows or extends that:

```go
b := sdk.Component[Health](r, schema.Category("Gameplay"))
b.Script("Current", "Max")
sdk.Method1[Health, int64, bool](b, "Damage", func(c *sdk.ScriptCtx, h *Health, amount int64) bool {
	h.Current -= float32(amount)
	return h.Current <= 0
})
sdk.Proc0[Health](b, "Heal", func(c *sdk.ScriptCtx, h *Health) { h.Current = h.Max })
```

`Script(names...)` lists the visible fields, `ScriptNone()` hides the component from scripts entirely. `Method0..Method3` and `Proc0..Proc3` on a builder take the same parameter types as module functions plus the component pointer; they are compiled to typed adapters and allocate nothing per call. Scripts call them on the component reference: `hp := node.Get<Health>()` then `hp.Damage(5)`. The methods are not a module and cannot be imported. A call through a `nil` reference, a destroyed node or a removed component faults with `AES6004`. `ScriptMethod(name, fn any)` accepts an arbitrary Go function through reflection: it is for prototyping, logs a warning at registration and is rejected in `aego_runtime` builds.

The binding registry's hash covers every module, name and signature and is part of the compile cache key, so changing a native signature recompiles the scripts that use it. Changes in `native/` are never swapped in-process (§19).

## Handles

A module that owns long-lived objects hands them to scripts as handles:

```go
m := r.ScriptModule("mygame/net")
sdk.ScriptHandle(m, "Request", true, RequestOf, Request.Key, sdk.ScriptDoc("Request is a pending HTTP call."))
hm := m.Registry().HandleMethods("mygame/net.Request")
sdk.Func1(hm, "Done", func(c *sdk.ScriptCtx, r Request) bool { return svc.Done(r) })
```

The Go type must be distinct for every handle type and convert to and from a non-zero `uint64` key; key `0` is `nil`. Scripts see the type as `net.Request` and call its methods. A handle registered as awaitable must have `Done() bool`; `yield req` then parks the coroutine until the module calls `Complete(abi.HandleID("mygame/net.Request"), key)` on the world service `scriptwait.ServiceID`. Call it when the object ends, fires or is cancelled.

---
id: language/runtime-behaviour
parent: language
order: 12
title: Runtime behaviour
---
# Runtime behaviour

- A fault stops only the instance that caused it (its methods, coroutines and handlers are no longer called); the rest of the scene keeps running. The Console shows the script stack; double-click opens the file.
- Each callback has an instruction budget (default 1 000 000); an infinite loop ends with `AES6001`. A coroutine resume and a signal handler each have their own budget.
- Integer division by zero faults; `MinInt / -1` yields `MinInt`, `MinInt % -1` yields `0`. `float → int` conversion saturates and maps NaN to `0`, identically on every platform.
- Strings and slices are garbage-collected between callbacks; scripts that do not allocate produce no garbage. A suspended coroutine keeps its strings and slices alive.

| Code | Meaning |
|---|---|
| AES6001 | instruction budget exceeded |
| AES6002 | native binding panicked |
| AES6003 | call depth over 256, or a signal that re-emits itself without end |
| AES6004 | nil or dead reference (`NodeRef`, `ComponentRef`, `ScriptRef`, `Curve`) |
| AES6005 | index out of range |
| AES6006 | integer division by zero |
| AES6007 | invalid string or slice handle |
| AES6008 | `missing return` reached |
| AES6009 | unresolved extern or binding |
| AES6010 | host function reported an error (`c.Fail`) |

Faults raised by the engine itself (a dead reference inside a component accessor, for instance) carry their exact code; only `c.Fail` from native code maps to `AES6010`.

---
id: language/limits
parent: language
order: 13
title: Limits
---
# Limits

255 registers per function · 32 767 instructions per function · 32 value slots per slice element · 255 script-visible fields per component (checked when the component is registered) · call depth 256 · 1 000 000 instructions per callback (configurable).

---
id: language/diagnostics
parent: language
order: 14
title: Diagnostics
---
# Diagnostics

Compiler diagnostics have a code, file, line and column, and often a note:

```
scripts/player/player.aes:17:21 AES2003 cannot use value of type NodeRef as float in assignment
  note: field Speed has type float
```

`AES1xxx` lexical/syntax · `AES2xxx` types · `AES3xxx` attributes · `AES4xxx` packages and imports · `AES5xxx` bindings · `AES6xxx` runtime faults. The parser recovers after an error, so one run reports every problem in a file. `aego check --explain AES2052` prints the long description of a code; the editor shows it on hover in the Problems panel.

Codes specific to coroutines, signals and script references:

| Code | Meaning |
|---|---|
| AES2050 | `yield` outside a coroutine |
| AES2051 | coroutine called without `start` |
| AES2052 | `start` target is not a coroutine of this script |
| AES2053 | coroutine is not a script method without a result |
| AES2054 | handler parameters do not match the signal |
| AES2055 | bad `Connect` handler, or `Emit` on another script's signal |
| AES2056 | lifecycle method declared as a coroutine |
| AES2057 | wrong `yield` argument type |
| AES2058 | `ScriptRef` / `GetScript` target is not a script type, or is ambiguous |
| AES2059 | lifecycle method or coroutine called through a `ScriptRef` |

---
id: language/cli
parent: language
order: 15
title: CLI
---
# CLI

| Command | |
|---|---|
| `aego check [dir] [--manifest file] [--explain code]` | type-check every script package against engine and native bindings |
| `aego test [dir] [-run name] [-v] [-budget n] [-seed n]` | run `TestXxx()` functions from `*_test.aes` headless |
| `aego dump script <pkg\|Type> [--func name] [--project dir]` | bytecode with `; file:line source` comments; the header lists imports, externs, enums, signals, script refs, cross-script fields and methods, and connections |
| `aego run [dir]` | refuses to start while a script package has errors |

All of them use the same workspace as the editor and hot reload. `check` and `dump` use the bindings manifest. For a project with `native/`, a small helper that only calls the module's `Register` writes it to `.aego/cache/script-bindings`; it is rebuilt when `native/*.go`, `go.mod`/`go.sum` or the engine change. A native compile error or a failing `Register` is exit code 2. `test` on a project with `native/` builds the game binary and runs the tests inside it, because tests call the real native functions.

Test functions are package functions; they have no scene, so coroutines, signals and script references are tested in play, not in `_test.aes`. `_test.aes` files are compiled only by `aego test`; they never enter Play or exported builds. A test that loops forever ends with `AES6001` and the next test runs.

---
id: language/coroutines
parent: language
order: 16
title: Coroutines
---
# Coroutines

A coroutine is a script method that can pause and continue on a later frame.

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

- **Declaration.** `coroutine` follows the signature: `func Name(params) coroutine { … }`. Only script methods without a result can be coroutines (`AES2053`). `Ready`, `Update`, `FixedUpdate` and `Reloaded` cannot be coroutines (`AES2056`); start one from them instead.
- **Starting.** `start Name(args)` runs the coroutine at once, up to its first `yield`, and returns a `Coroutine`. Only coroutines of the same script can be started (`AES2052`). Calling a coroutine like a plain function is `AES2051`. Starting the same coroutine twice gives two independent runs.
- **yield** is allowed only inside a coroutine (`AES2050`):

| Form | Resumes |
|---|---|
| `yield` | on the next frame |
| `yield wait(s)` | after `s` seconds (`float`) of game time, at the earliest on the next frame; paused Play does not advance it |
| `yield frames(n)` | after `n` frames (`int`) |
| `yield fixed(n)` | after `n` fixed steps (`int`), before physics |
| `yield c` | when coroutine `c` ends; it resumes in the same frame; a `nil`, finished or cancelled `c` resumes on the next frame |
| `yield h` | when the tween or timer `h` (any awaitable host handle) ends, fires or is cancelled, on the next frame; a `nil` or finished `h` resumes on the next frame; it waits while the game is paused |

A wrong argument type, or a handle that cannot be awaited, is `AES2057`.
- **Order.** Coroutines that are due in one frame resume in the order they were started. They run in `script.coroutines`, right after `script.update`; `fixed` waits run after `script.fixed`. Only coroutines that are due are visited; ten thousand sleeping coroutines cost nothing per frame.
- **Ownership.** A coroutine belongs to the instance that started it. It is cancelled when the instance's node is destroyed, the script is removed or disabled, the instance faults, or its script changes in a hot reload. A fault inside a coroutine stops the owning instance, like a fault in `Update`. A cancelled coroutine releases its wait slot immediately; it does not linger until its timer would have fired.
- **Handles.** `c.Stop()` cancels; `c.Running()` is false once the coroutine finished, was cancelled or faulted. The zero `Coroutine` is `nil` and never running. A handle from before a hot reload stays valid as a value but is never running again.
- **Budget.** Each resume has its own instruction budget (§12). A loop without `yield` inside a coroutine still ends with `AES6001`.

Run it:

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
parent: language
order: 17
title: Signals
---
# Signals

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

- **Declaration.** `signal Name(params)` inside a script. Signals have no result. An uppercase name is visible from other packages.
- **Emit.** `Name.Emit(args)` is allowed only inside the script that declares the signal (`AES2055`). Handlers run synchronously, in connection order, before `Emit` returns. Each handler has its own instruction budget; a fault in a handler stops the handler's owner, not the emitter, and the remaining handlers still run.
- **Connect.** `Name.Connect(Method)` or `ref.Name.Connect(Method)` registers a plain method of this script, or `ref.Method` of another instance, and returns a `Connection`. The handler must take exactly the signal's parameter types and return nothing (`AES2054`). Coroutines and lifecycle methods cannot be handlers (`AES2055`). Connecting the same method twice calls it twice.
- **Changes during Emit.** Connections added or removed by a handler take effect after the current `Emit` returns; the running `Emit` uses the list it started with.
- **Lifetime.** When the source instance stops, all its connections are removed. When a handler's owner stops, its connections are skipped and removed. `Connected()` becomes false in both cases. `c.Disconnect()` removes one connection; on a `nil` handle it does nothing.
- **Recursion.** A handler that emits again goes one call deeper; endless re-emitting ends with `AES6003` on the emitting instance.
- **Cost.** `Emit` allocates nothing; a scene with ten thousand connections and a thousand emits per frame produces no garbage.

Run it:

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

## Engine signals

Components and engine handles have signals too. Scripts connect to them and never emit them:

```aes
hp := node.Get<Health>()
conn = hp.Died.Connect(OnDied)
fade = tween.ToFloat(node, "SpriteRenderer.Tint.a", 0, 0.3, curve.Linear).Completed.Connect(OnFaded)
```

The same rules apply as for script signals. A component signal's connections end when its node is destroyed or the component removed; a handle signal's connections end after the handle's last emit (a tween's `Completed`, the last `Fired` of a one-shot timer). Connecting to a finished handle returns a `nil` `Connection`. `Emit` on an engine signal is `AES2055`. Parameters of engine signals are numbers, vectors, colors, enums and references, never strings or slices.

Go code subscribes with `sdk.Connect1[A](ctx, node, "Health.Died", fn)` or `"script/Enemy.Died"`. A Go handler that panics is disconnected and reported; it never stops a script.

---
id: language/scriptref
parent: language
order: 18
title: ScriptRef
---
# ScriptRef

`node.GetScript<T>()` returns the `ScriptRef<T>` of script `T` on that node, or `nil` when the node has none. On a `nil` or destroyed node it faults with `AES6004`. Use `pkg.Name` when the short name is ambiguous (`AES2058`).

Through a `ScriptRef` you can read and write fields (exported ones from other packages), call plain methods and connect to signals. Lifecycle methods and coroutines are not reachable (`AES2059`); call a plain method that does the work, or connect to a signal. A `ScriptRef` is re-validated on every access; once the target instance stops, access faults with `AES6004`. `ScriptRef` fields cannot be exported. A `ScriptRef` survives a hot reload of either side: fields and methods are re-resolved by name.

---
id: language/hot-reload
parent: language
order: 19
title: Hot reload
---
# Hot reload

Saving a script while the game runs (`aego run`, Play in the editor) swaps the code without restarting. The workspace builds a new generation from the saved files. At the start of the next frame (`script.swap`, `PhaseFrameBegin`, the VM stack is empty), every module of that generation is loaded into the same VM. Every live instance is then moved into the new layout, and the dynamic schema is replaced in the same transaction. If anything fails, nothing changes and the previous generation keeps running. A save that does not compile never produces a generation.

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
| `NodeRef`, `AssetRef`, `ComponentRef`, `ScriptRef` | kept |
| `Coroutine` | kept as a value; see below for whether it is still running |
| `Connection` | kept; rebound or invalidated, see below |
| host handles, `Curve` | kept as values |

After the swap, for every script whose package (or a dependency's interface) changed:

1. its **coroutines are cancelled**: a coroutine's suspended state belongs to the old code; a `Coroutine` field keeps the old, no longer running handle;
2. **signal connections are rebound** by signal name, parameter types and handler name; a connection whose signal or handler is gone, or whose signature changed, is removed and its `Connection` reports `Connected() == false`;
3. `func Reloaded()` runs. Use it to restart coroutines:

```aes
func Reloaded() {
	if flash > 0 {
		blink = start Blink(6)
	}
}
```

`Ready` does not run again. Instances that faulted in a changed script resume running. Instances of a removed script stop. Coroutines and connections of unchanged scripts are untouched. The Console prints one line per reload: `Hot reload N → N+1: X migrated, Y reset, Z coroutines canceled, W handlers disconnected, … ms`; the same numbers appear in the Profiler's Scripts section. Changes in `native/` are never swapped in-process: the supervisor rebuilds and restarts.

---
id: language/editor
parent: language
order: 20
title: Editor
---
# Editor

The editor's code editor, navigation, completion, hover, library stubs, Problems panel and Profiler sections for scripts are described in `docs/editor.md` (§ Scripts). Unsaved edits are analysed but never run; autosave writes them one second after the last keystroke, on focus loss and before Play.

---
id: language/data-assets
parent: language
order: 21
title: Data assets
---
# Data assets

A data type registered by a native module is available to scripts by its short name.

```
@export
var Stats AssetRef<EnemyStats>

@export
var Loot TableRef<LootEntry>
```

`Stats.Speed` reads a field of the record the reference points to; every read sees the current record, so live tuning in the editor reaches the script on the next read. Fields cannot be assigned through a reference (AES2007). `Stats.Copy()` returns the record as a struct value of type `data.EnemyStats` that the script may change freely.

A table reference has `Row(id string)`, `Has(id string) bool` and `Len() int`. `Row` returns a copy of the row.

Reading through a nil reference or asking for a missing row stops the script with AES6004.
