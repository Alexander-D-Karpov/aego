# Aego Script — lexical decisions

Notes on deliberate choices in the lexer (`script/lexer`). These are design
decisions, not bugs: the lexer rejects the following on purpose and reports a
diagnostic that points at the intended form.

## Identifiers are ASCII-only

An identifier is `[A-Za-z_][A-Za-z0-9_]*`. A non-ASCII rune where an identifier
could start is rejected with `AES1001` (`UnexpectedChar`): "identifiers may use
ASCII letters only".

Rationale: keeping identifiers ASCII avoids Unicode confusables (visually
identical names that are distinct code points) and NFC-vs-NFD normalization
questions in the resolver, and keeps `isLetter`/`isIdentPart` to a byte compare.
Non-ASCII text is still fully supported *inside string literals*.

## No character literals

`'a'` is not a token. A `'` starting a token is consumed up to the closing `'`
(or newline) and reported with `AES1001`: "character literals are not supported,
use a string". Use a one-rune string (`"a"`) instead.

Rationale: the language has no distinct `char`/`rune` scalar separate from
`string`, so a dedicated syntax would only invite confusion with strings.

## Float literals need digits after the point

`1.0` is a float; `1.` is not. After the integer part, a `.` followed by a digit
starts a fraction. A `.` followed by a non-letter (end of input, operator,
whitespace) is a malformed float and is reported with `AES1005`
(`MalformedNumber`): "float literal needs digits after the point, write 1.0".

A `.` followed by a letter is left alone — `1.x` lexes as `Int '.' Ident`, so
method-call / selector syntax on an integer still parses. The parser, not the
lexer, decides what that means.

## No bitwise operators

A single `&` or `|` is illegal (`AES1001`): "operator is not supported, Aego
Script has no bitwise operators". Only `&&` and `||` exist. The language targets
gameplay scripting, where bitwise arithmetic is rare and error-prone next to the
logical operators it resembles.

## Identifier interning

`lexer.New` takes a `*source.Interner` so identical names across every file in a
project share one backing string (see `source.Manager.Interner`). The resolver
can then compare names by string identity instead of re-hashing each file's
copies. An `Interner` is single-goroutine; lex a project's files on one
goroutine, or give each its own.

# Aego Script — package and semantic rules

Decisions enforced by the resolver and checker (`script/resolver`,
`script/checker`), not the lexer.

## Package name equals its directory

A package is a directory under `scripts/`; its path is `project.Module` plus the
relative directory. Every file's `package` clause must name the package after
that directory's base (`game/enemy` → `package enemy`). A mismatch is reported
with `AES4001` (`PackageName`). One name, one mental model, and `aego new`
generates directories that already satisfy it. This is also why a module path is
`[a-z0-9_/-]+` with no dots — a qualified name (`module.Name`) is split on the
last dot.

## Vectors use composite literals, not call syntax

Construct engine value types with a composite literal: `Color{1, 0, 0, 1}`,
`Vec2{x, y}`. There is no `Color(1, 0, 0, 1)` call form — `Color(x)` parses as a
one-argument conversion, so the multi-argument call would be an error. Keeping a
single construction form avoids two spellings for one thing.

## `len` counts bytes; strings are UTF-8

Strings are immutable UTF-8. `len(s)` is the number of **bytes**, not runes, and
both constant folding and the runtime agree on that. There is no string indexing
in v1.

## A slice element is at most 32 slots

The GC marks slice elements through a 32-bit mask (one bit per element slot), so
a slice whose element occupies more than 32 `Value` slots cannot be represented
and is a compile error (`AES2032`): "slice element … is N slots, the limit is
32". Scalars, vectors, refs and small structs are well under it; this only bites
a slice of a very large struct.

## Attributes attach to scripts and script fields only

The parser accepts attributes syntactically before a `func` (forward
compatibility for `@rpc`/`@tool` in a later stage), but in v1 the checker rejects
them with `AES3005` (`AttrTarget`): "attributes on functions are not supported
yet". Attributes are meaningful only on a `script` type and its fields.

# Aego Script — standard library, bindings and tests

## Standard library v1

aego/math: Pi, Tau; Floor Ceil Round Trunc Sqrt Sin Cos Tan Asin Acos Exp Log Abs Sign Min Max Clamp Lerp Atan2 Pow Deg2Rad Rad2Deg; AbsInt MinInt MaxInt ClampInt; Vec2: Length Normalize Dot Cross Dist Rotate Angle LerpVec2; Vec3: Length3 Normalize3 Dot3 Cross3 Dist3 LerpVec3; Quat: QuatEuler QuatAxisAngle QuatMul QuatRotate QuatSlerp QuatLookDir. There is no overloading, so the Vec3 variants carry a 3 suffix and the int variants an Int suffix.

aego/log: Debug Info Warn Error(string). Build messages with + and string(x).

aego/rand: Float() in [0,1), Bool(), Int(min, max) in [min, max), Range(min, max float), Seed(). The source is the world's seeded generator, so a replay with the same seed gives the same numbers.

aego/time: Delta() Fixed() Elapsed() Frame().

aego/testing (only under aego test): True(cond, msg) False(cond, msg) Equal(got, want int) EqualFloat(got, want, eps float) EqualString(got, want string) Fail(msg). A failed assertion stops the test like t.Fatal and reports the script file and line.

## Native bindings

Native Go exposes functions through script/bind: bind.Func0..Func3 for functions with a result, bind.Proc0..Proc3 without one, bind.Const and bind.Enum. The signature is derived from the Go types; allowed types are bool, builtin ints, float32/float64, string, mathx.Vec2/Vec3/Color/Quat/Rect, scene.Transform2D/Transform3D, scene.NodeHandle (a NodeRef), asset.Ref[Texture|Scene|Prefab|Mesh] and Go integer types registered with bind.Enum. Structs and slices cannot cross the boundary. A binding reports a script-visible error with ctx.Fail; a panic becomes fault AES6002. The registry hash covers every module, name and signature and keys the compile cache.

## Tests

aego test [dir] compiles every package under scripts/, including *_test.aes files, and runs each package-level func TestXxx() declared in a _test file. Flags: -run substring, -v, -budget per-test instruction budget, -seed for aego/rand. A test that loops forever ends with AES6001 and the next test still runs.

## Enum fields of native components

In v1 an `enum` field of a Go component is exposed to scripts as a plain `int`: you write `sr.Blend = 2`, not `sr.Blend = Blend.Additive`. It works, but loses the names. In a later stage `fieldABI` will hand back `abi.EnumOf(fd.EnumName)` and `bind` will register the component's enum as a host enum (as it already does for script enums), so the named form becomes available.


# Aego Script — language reference (v1)

## Grammar

    File        = "package" ident ";" { Import ";" } { TopDecl ";" } .
    Import      = "import" ( string | "(" { string ";" } ")" ) .
    TopDecl     = ConstDecl | TypeDecl | EnumDecl | FuncDecl | ScriptDecl .
    ConstDecl   = "const" ident [ Type ] "=" Expr .
    TypeDecl    = "type" ident "struct" "{" { ident Type ";" } "}" .
    EnumDecl    = "enum" ident "{" { ident ";" } "}" .
    FuncDecl    = "func" ident "(" [ ident Type { "," ident Type } ] ")" [ Type ] Block .
    ScriptDecl  = { Attr } "script" ident "{" { ScriptMember ";" } "}" .
    ScriptMember= { Attr } "var" ident Type [ "=" Expr ] | FuncDecl .
    Attr        = "@" ident [ "(" [ AttrArg { "," AttrArg } ] ")" ] .
    AttrArg     = [ ident "=" ] Literal .
    Type        = ident [ "." ident ] [ "<" Type { "," Type } ">" ] | "[" "]" Type .
    Stmt        = "var" ident Type [ "=" Expr ] | ident ":=" Expr
                | LValue ( "=" | "+=" | "-=" | "*=" | "/=" | "%=" ) Expr | LValue ( "++" | "--" )
                | Expr | If | For | Switch | "return" [ Expr ] | "break" | "continue" | Block .
    If          = "if" Expr Block [ "else" ( If | Block ) ] .
    For         = "for" [ Expr | [ Simple ] ";" [ Expr ] ";" [ Simple ] ] Block .
    Switch      = "switch" Expr "{" { ( "case" Expr { "," Expr } | "default" ) ":" { Stmt ";" } } "}" .

Precedence from high to low: unary `-` `!`; `* / %`; `+ -`; `== != < <= > >=`; `&&`; `||`.
A line break after `>` that closes type arguments ends the line, so `var S AssetRef<Texture>` needs no `;`.

## Types

| Type | Notes |
|---|---|
| `bool`, `int` (int64), `float` (float32), `string` | no implicit conversion; `int(x)`, `float(x)`, `string(x)` |
| `Vec2 Vec3 Color Quat Rect` | value types, lanes `.X .Y .Z .W` / `.R .G .B .A` / `.X .Y .W .H`; `Quat` is not comparable |
| `Transform2D{Position, Rotation, Scale}`, `Transform3D{Position, Rotation, Scale}` | value structs, returned and set by copy |
| `NodeRef`, `AssetRef<Texture/Scene/Prefab/Mesh>`, `ComponentRef<T>` | may be `nil`; field access through a nil ref faults |
| `struct`, `enum` | package level; structs copy on assignment; enum members `State.Idle`, switches must be exhaustive or have `default` |
| `[]T` | `len`, `append`, `s[i]`, `s[i] = v`; bounds checked; element ≤ 32 slots |

## Scripts

A `script` is attached to a node. Every script has an implicit `node NodeRef`. Lifecycle methods:

| Method | When |
|---|---|
| `func Ready()` | once, after every instance of the scene exists and refs resolved, in scene pre-order |
| `func FixedUpdate(dt float)` | every fixed step, before physics, in instance creation order |
| `func Update(dt float)` | every frame, in instance creation order |

Fields are runtime state. Only `@export` fields are saved in the scene and edited in the Inspector. Play never writes back to the edit scene.

## Attributes

| Attribute | Target | Meaning |
|---|---|---|
| `@export` / `@export(min=, max=, step=, unit=, display=, tip=, category=)` | field | expose to scene and Inspector |
| `@former("Old")` | script or field | old names still load; the scene is rewritten on save |
| `@hide` | script | not offered in Add Script |

Exportable types: `bool int float string Vec2 Vec3 Color Quat Rect`, enums, `NodeRef`, `AssetRef<T>`. Unknown properties in a scene are kept and re-saved with a warning.

## Components

`node.Get<SpriteRenderer>()` returns `ComponentRef<SpriteRenderer>` or `nil`, `node.Has<T>()` returns `bool`. A short name works when it is unique, otherwise write the full name (`engine.SpriteRenderer`). Field names are the schema names with the first letter upper-cased (`sr.FlipX`, `hp.Current`). Native components registered by `native/` code are available the same way.

## Standard library additions

aego/scene: `Find(path) NodeRef`, `Instantiate(AssetRef<Prefab>, parent NodeRef) NodeRef`, `Destroy(NodeRef)`.

NodeRef methods: `IsAlive Name Parent Child(name) Find(path) Destroy Enabled SetEnabled Space`, 2D: `Transform2D SetTransform2D Position SetPosition WorldPosition Rotation SetRotation Scale SetScale`, 3D: `Transform3D SetTransform3D Position3 SetPosition3 Rotation3 SetRotation3`. 2D methods on a 3D node fault and the other way round.

aego/input: `Down Pressed Released(Key) bool`, `MouseDown MousePressed(Button) bool`, `MousePos() Vec2`, `Axis(name) float`. Keys are `input.Key.Space` or the constant `input.KeySpace`; digits are `Digit1`. Axes come from `project.aego`:

    input {
      axes {
        horizontal { negative=A positive=D altNegative=Left altPositive=Right }
      }
    }

## Native bindings

Project Go code in `native/` adds script functions through the sdk:

    m := sdk.ScriptModule(r, "mygame/combat")
    sdk.Proc2(m, "Damage", func(c *sdk.ScriptCtx, target sdk.NodeHandle, amount int64) { ... })

Scripts `import "mygame/combat"`. Host modules and script packages share one namespace; `aego/` is reserved.
`c.Fail(msg)` stops the calling script instance with a fault, a Go panic becomes fault `AES6002`.

## Runtime faults

| Code | Meaning |
|---|---|
| AES6001 | instruction budget exceeded (default 100k per callback) |
| AES6002 | native binding panicked |
| AES6003 | call depth over 256 |
| AES6004 | nil reference |
| AES6005 | index out of range |
| AES6006 | integer division by zero |
| AES6008 | `missing return` reached |
| AES6010 | host function reported an error |

A fault stops only the faulted instance; the rest of the scene keeps running. The Console shows the script stack, double-click opens the file.

## Packages and files

Scripts live under `scripts/`. The package path is `project.module + "/" + directory`, the package clause must equal the directory name. A package that fails to compile keeps its last good build, its files are marked invalid and Play is blocked until they compile. `*_test.aes` files are only compiled by `aego test`.

## CLI

| Command | |
|---|---|
| `aego check [dir]` | parse every document and type-check scripts |
| `aego test [dir] [-run name] [-v]` | run `TestXxx()` functions from `*_test.aes` headless |
| `aego dump script <pkg|Type> [--func name]` | bytecode with `; file:line source` comments |
| `aego run [dir]` | refuses to start while a script package has errors |

`aego check`, `aego test` and `aego dump` know the standard library and engine components; bindings from `native/` exist only inside the built project binary, so check such scripts with `aego run --headless`.

## Limits

255 registers per function, 32767 instructions per function, 32 slots per slice element, 255 fields per component, call depth 256.
