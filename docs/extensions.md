# Kernel, modules and extension points

What belongs to the kernel, what belongs to a module, and what depcheck reports when the line is crossed. The rules are in ARCHITECTURE §4; the only place to waive one is `tools/depcheck/exceptions.txt` (D212).

## What is kernel

The kernel (`schema`, `scene`, `render`, `asset`, `engine`, `script`, `ui`, `project`, `contracts`) and the editor core know concepts every module can use, never a particular module:

| Kernel concept | Examples |
|---|---|
| Frame phases and scheduling | `PhasePhysics`, `PhasePostPhysics`, `SystemDesc.Before/After` inside one module |
| Scene data | presentation pose (`SetPresentationLocal`), component defaults, `OnAdd` |
| Script host | host structs, component methods, script events (`module.Registry.ScriptEvent`), `bind.Ctx` |
| Contracts | `contracts/scriptevents`, `contracts/physics2d`, `contracts/camera2d`: interfaces between modules and scripts |
| Editor core | documents, history, selection, `assetops`, `fuzzy`, gizmos, vectors, play, `services` (Frame and the panel services) |
| Editor extension points | `editor/ext`: `SettingsPage`, `Problems`, `Profiler`, `GameOverlay`, `Vectors`, `Overlay`, `ext/inspect` custom inspectors |

## What is a violation

| Code in the kernel or the editor that… | depcheck rule | Do instead |
|---|---|---|
| imports `modules/X` | `kernel-imports-module`, `editor-imports-module` | register through `module.Registry` from the module; compose in `app` (the catalog) |
| imports another module from `modules/X` | `module-imports-module` | depend through a contract in `contracts/`, declare `Info.Requires` |
| names a module's system in an ordering constraint | caught by `Build` (unknown dependency) | order by phases; keep `Before/After` inside the module (D204) |
| writes a module ID, `<ID>.*`, a module component name or a module diagnostic code as a literal | `module-literal` | let the module pass the string through its registration (category, page, section) |
| imports one panel from another | `panel-imports-panel` | move the shared code to `editor/core` or `editor/ext` |
| imports a panel from `editor/ext` or `editor/core` | `editor-ext-imports-panel`, `editor-core-imports-panel` | move the type to `editor/ext` |
| makes `modules/X` import `modules/X/editor` | `module-imports-own-editor` | the editor side imports the module, never the reverse |
| lets a runtime package reach dev-only code | `runtime-reaches-dev` | put the dev part behind `!aego_runtime` or in a dev package |
| lets the playground reach native or pipeline code | `wasm-reaches-forbidden` | keep it out of the playground profile |
| lets a web build reach `net/http`, `crypto/tls`, `crypto/x509` or `archive/*` | `wasm-reaches-std` | move the code behind `//go:build !js` |
| holds or receives `*context.Editor` in `editor/panels/*` | `tests/modulecheck/panels_test.go` | take the services the panel needs in its `Deps` (D216) |

A module can tune another module's behaviour without importing it by providing a value under a contract kind: `r.Provide(camera2d.AccessibilityKind, camera2d.Accessibility{ScreenShake: "...", ReducedMotion: "..."})` hands the camera module the IDs of settings it reads every frame. The consumer reads `ctx.Provided(kind)` in `Init`.

## Proof with a fake module

A rule holds when a test module that is not the real one can use the same extension point. `tests/modulecheck/fakephys` registers a project section, inspector hints, a script event, component hooks, a settings page, a panel, a custom inspector, a gizmo, bounds, overlays, a vector layer and a profiler section without importing `modules/physics`; the guard test there checks that the kernel and the editor do not import or name any module.

## Writing a module

A project module is written against `aego/sdk` only. `examples/native-settings` is a complete one: a project section, a component with a validator and a game-view overlay. `tests/modulecheck/fakephys` uses every extension point and runs in the editor in place of physics; it is the working reference for the full list. The engine-side names behind each facade are in D205–D211.

### Lifecycle

`Register` → schema freeze → `Init(ctx)` → `AttachWorld(ctx)` / `DetachWorld(ctx)` per edit or play world → `Shutdown()`. `Shutdown` runs in reverse `Init` order; a failing `Init` shuts down the modules initialized before it. `DetachWorld` is called for every module whose `AttachWorld` was called, including a failing one, so it must tolerate a partial attach. Per-world state lives in world services, not in the module struct (D206).

### Project settings sections

A module owns one block of `project.aego`, named after the module ID or `<id>.<name>`:

```go
sdk.ProjectSection(r, sdk.SectionDesc[Settings]{
    ID:       "weather",
    Title:    "Weather",
    Order:    400,
    Default:  func() Settings { return Settings{Strength: 1} },
    Validate: func(s *Settings) error { ... },
})
```

- With `Decode`/`Encode` left empty and the type registered with `sdk.RegisterStruct`, the schema codec reads the block and Project Settings draws a page from the fields.
- `Version` and `Migrate(node, from, to)` upgrade old blocks; migration works on a clone, the file is rewritten only on save.
- `Former` lists old block heads (`render2d` reads `render {}`).
- `Names(v, list)` returns named lists for inspector hints (below).
- `sdk.Section[T](ctx, id)` gives the current value. Project Settings edits a draft and replaces the value after `project.aego` is written, so a module reads the section when it needs it instead of caching the pointer from `Init`.

Blocks without a registered section stay in the file untouched. `aego check` validates the sections of the built-in modules; sections of native modules are checked when the project runs.

### Inspector hints

Schema options on the component registration, never code in the Inspector:

| Option | Effect |
|---|---|
| `sdk.ChoicesFrom(field, section, list)` | integer field picked from a named list of a section (`Names`) |
| `sdk.Bitset(field, section, list)` | integer field edited as a mask over that list; all listed bits set is stored as -1 |
| `sdk.Runtime(fields...)` | shown only during Play, read-only |

Component groups in the Inspector follow module order from the manifest, then registration order (D208).

### Component hooks

```go
sdk.Component[Wind](r, sdk.Name("weather.Wind")).
    OnAdd(func(ctx sdk.AddContext, w *Wind) { ... }).
    OnRemoving(func(ctx sdk.AddContext, w *Wind) { ... }).
    OnChanged(func(cs *sdk.ChangeSet) { ... }).
    Validate(func(ctx sdk.ValidateContext, w *Wind) []sdk.Diagnostic { ... })
```

- `OnRemoving` gets the old value before the store frees it (editor remove and undo of add).
- `OnChanged` runs after every committed transaction, Undo and Redo with the deduplicated `ChangeSet`; add and remove of one component in a batch cancel out.
- `Validate` reports problems of one component; the editor runs it over open edit scenes.
- Hooks make no structural changes outside pending. Runtime code keeps comparing values itself (D185, D209).

### Diagnostics

`engine/problems` collects `sdk.Diagnostic{Owner, Code, Severity, Message, Subject, BlocksPlay}` from sources with a revision; the service rebuilds the list only when a revision changes. `sdk.Diagnostics(ctx)` in `Init` returns the service; a module adds runtime diagnostics as a `sdk.ProblemSource` from `AttachWorld` and removes it in `DetachWorld`. The Problems panel shows them with the owner as category; an error with `BlocksPlay` stops Play (D210).

Every code a module reports is listed in `Info.Codes` and has a page `res/docs/diagnostics/<CODE>.md` with front matter `code`, `title`, `severity` (`error`, `warning` or `info`) and either `example: <CODE>.aescene`, a scene next to the page that produces the code (with `<CODE>.aes` next to it when a script has to act, as for PHY007), `scope: asset` for codes that come from importing or checking an asset, or `scope: play` for codes that only an edit during Play produces (PHY006). `aego docs check` fails on a declared code without a page, a page whose code is not declared, a component code without an example, and an example that does not produce its code after `module.ValidateScene` and 60 headless frames. The pages appear under Reference → Diagnostics next to the script codes (D146, D316).

### Contracts between modules

A module never imports another module. What two modules share lives in `contracts/<name>` as plain types, a service ID and an interface; the provider registers the service in its world, the consumer looks it up with `engine.ServiceOf` and works without it.

- **Provisions.** A value handed over at load time, keyed by a kind. `camera2d.AccessibilityKind` (`"camera2d.accessibility"`) carries the IDs of the Screen Shake and Reduce Motion settings: the settings module provides them with `r.Provide(camera2d.AccessibilityKind, camera2d.Accessibility{...})`, the camera module reads `ctx.Provided(camera2d.AccessibilityKind)` in `Init` and scales shake by them every frame.
- **`physics2d.Queries`** (`physics2d.ServiceID`). Ray, point, circle and box queries; shape casts `CastCircle`, `CastBox`, `CastCapsule` and `CastBody`, which sweeps every non-trigger collider of a body and returns the first hit with its fraction, normal and, when the shapes already overlap, `Initial` and `Depth`; `Penetration` lists the overlaps of a body for depenetration; `BodyPose`, `BodyVelocity`, `MoveKinematic` (moves a kinematic body so contacts see its velocity), `Teleport`, `ApplyImpulse` and `Gravity`. `QueryFilter` selects layers and triggers; `SkipDynamic` ignores dynamic bodies, and `ExcludeNode` and `ExcludeBody` skip the caller's own collider or body. The gameplay controller is written only against this interface.
- **`scriptcall.Caller`** (`scriptcall.ServiceID`). `Method(node, name, params)` finds a plain method by name in the scripts on a node and returns a `Ref`; `Call(&ref, args)` runs it. A hot reload starts a new `Generation`, which makes every older `Ref` stale: look the method up again when `Ref.Gen` differs. The State Machine finds its hooks this way.

### Resources

Module icons are named `<owner>.*` or `cat.<owner>`; a clash with the engine atlas or another module is a build error. Built-in modules list their icon sources through `IconDir()`. Docs registered with `Registry.Docs` carry the module as owner and are left out of runtime builds (D211).

### Shader resources and material tools

Shader and material assets use the existing asset pipeline and module lifecycle. A module registers its components, script bindings and property adapters in `Register`. Its optional `Init(*module.InitContext)` receives the shared asset resolver as `ctx.Assets` and the VFS as `ctx.Engine.VFS()`; per-world material state belongs to the world attached through `AttachWorld`. The Render2D implementation registers its material components and bindings in [`registerMaterials`](../modules/render2d/material_module.go). There is no separate material registration phase in the kernel.

Keep project shader resources under a module-owned directory, for example `assets/weather/`, with stable `.meta` AssetIDs. Shared project GLSL helpers go under `assets/shaders/include/weather/` and are included as `#include "weather/noise.glsl"`. The `engine/` include prefix is reserved for engine helpers under `engine://shaders/include/`. Explicit `./` and `../` includes instead resolve against the current source/include, within `assets/` or `engine://shaders/`; they never fall back to the shared root. Adding a VFS mount in `Init` does not add another shader namespace or make the mount available to an earlier import. Module-owned VFS mounts use their own scheme; shader dependencies must be available to the development pipeline before import.

[`app.CoreImporters`](../app/importers.go) registers the standard shader, GLSL include and material importers. A module using `.aeshader` and `.aemat` needs no additional importer. A new source format uses `sdk.RegisterImporter` in a `!aego_runtime` file; this adds an owner-scoped `Registry.Dev` hook consumed by `app.DevImporters`. Both user and built-in materials produce prepared AESH/AEMT artifacts. Runtime resolves their AssetIDs and creates device resources from those artifacts, without importing `shaderx` or parsing authoring files. The six engine materials are generated by [`tools/genmaterials`](../tools/genmaterials/main.go), listed in the common built-in asset registry and loaded through the same runtime material path. Module assets do not allocate IDs from the engine's built-in range.

Material editor integration uses the normal editor hook, independently of `Init`. In engine assembly, [`hooked.Register`](../app/assembly/hooked.go) attaches the Render2D adapter through `Registry.Editor`; project modules use `sdk.EditorHook` in their `aego_editor` files. The adapter in [`materialHooks`](../modules/render2d/editor/materials.go) registers:

| Extension point | Render2D registration |
|---|---|
| Asset inspectors | `AssetInspector("shader", ...)`, `AssetInspector("material", ...)` |
| Node inspector | `Inspector("engine.MaterialOverride", ...)` |
| Searchable creation actions | `CreateAction` with `render2d.shader`, `render2d.material`, `render2d.material-from-shader` |
| Preview and diagnostics lifecycle | `Tick` callbacks and the existing Problems service |

Other modules use their own component names and action keys. The generic Inspector dispatches registered adapters; it contains no branch for MaterialOverride. Material source and parameter edits share the existing asset document/history and mutation infrastructure. Preview uses the real renderer in a separate session. This keeps module UI, source import and runtime rendering at their existing boundaries. The complete source and ABI contract is in [materials-shaders.md](materials-shaders.md).

### Profiling

Every system already shows up in the Profiler flame graph and in exported traces under its system ID. For finer zones inside a system, register a zone once in `Init` and open it around the work:

```go
var gust sdk.TraceZone

func (m *Weather) Init(ctx *sdk.InitContext) error {
    gust = ctx.Engine.Trace().Zone("weather.gust")
    return nil
}

func (s *windSystem) Run(ctx *sdk.Context) {
    z := sdk.TracerOf(ctx).Begin(gust)
    s.applyGusts(ctx)
    z.End()
}
```

Zones nest, cost no allocations and are no-ops while tracing is off; every method is safe on a nil tracer (D215).

### Editor extensions

Editor code lives in files tagged `aego_editor` and is attached with `sdk.EditorHook`. The hook runs once when the editor starts; everything it registers is removed together when one of its callbacks panics (D72).

```go
//go:build aego_editor

func registerEditor(r *sdk.Registry) {
    sdk.EditorHook(r, func(e *sdk.EditorRegistry) error {
        ed := e.Editor()
        if err := e.Panel(sdk.PanelDesc{Panel: &questPanel{ed: ed}, Near: sdk.PanelInspector}); err != nil {
            return err
        }
        return e.Command(sdk.CommandDesc{ID: "game.heal_all", Name: "Heal All", Run: func() error { return heal(ed) }})
    })
}

type questPanel struct{ ed *sdk.Editor }

func (*questPanel) ID() string    { return "game.quest" }
func (*questPanel) Title() string { return "Quest" }

func (q *questPanel) Draw(f *sdk.Frame, r mathx.Rect) {
    ui := f.UI
    if q.ed.Doc() == nil {
        widgets.LabelDim(ui, "No scene open")
        return
    }
    ...
}
```

| Extension point | Registration |
|---|---|
| Panel | `Panel(sdk.PanelDesc{Panel, Slot, Near, Order, Hidden})`; draws with `Draw(*sdk.Frame, r)` |
| Custom inspector | `Inspector("game.Health", sdk.InspectorFunc(fn))`; `fn` gets the editor, the selected nodes and the default field drawer |
| Command | `Command(sdk.CommandDesc{...})`; appears in its category menu and in Search Everywhere |
| Gizmo, bounds | `Gizmo(type, g)`, `Bounds(type, fn)` for picking and framing |
| Overlays | `Overlay(fn)` in the Scene View, `GameOverlay(fn)` also in the Game View |
| Settings page, profiler section, vector layer | `SettingsPage`, `Profiler`, `Vectors` |

A panel gets a fresh `Frame` every draw (`UI`, drag and drop, time, focus). Anything else it needs it captures at registration through `e.Editor()`. Built-in panels follow a stricter rule and never see the editor (D216). `PanelDesc.Slot` puts a panel into a column of the default layout; `Near` opens it next to a panel on demand; without `Order` extension panels follow the built-ins in the Window menu (D217).

## Try it scenes

A module can give any of its components a live example on the documentation site: put a scene in `res/docs/try/<Component>.aescene`, named after the last part of the component ID (`try/Body2D.aescene` for `physics.Body2D`). The page of that component gets a Try it block that runs the scene in the browser with the Inspector, selecting the first node that has the component; a guide page can show a scene with `try: try/name.aescene` and `try_select: module.Component` in its front matter.

Try scenes run on the docs runtime catalog (core, input, render2d, render3d, scripting, physics, debug, tween, cameras, gameplay, spawn, savegame and audio) with builtin assets only. A script `<Component>.aes` and prefabs `<Component>.<name>.aeprefab` (with their `.meta`) next to the scene are compiled and mounted with it. `aego docs check` loads each one, runs it for 60 frames and fails on a load error, a warning or a fault, and on a scene of a module outside the catalog. Keep the scene to what the component needs, put the documented node first, and add a camera.

Every visible component needs one. A component registered `Hidden`, which only carries state for a service, has none and is left out of Create, Add Component, Try it and the docs. The rules for engine code, including these, are in [contributing.md](contributing.md).
