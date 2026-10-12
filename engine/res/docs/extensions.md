---
id: extensions
title: Native extensions
order: 3
---
# Native extensions

A project extends the engine with Go code in `native/`. It imports only `aego/sdk` and implements `sdk.Module`:

    type Module struct{}

    func (Module) Info() sdk.ModuleInfo { return sdk.ModuleInfo{ID: "mygame", Requires: []string{"render2d"}} }
    func (Module) Register(r *sdk.Registry) error { ... }

Engine subsystems (`render2d`, `render3d`, `script`) register through the same registry, so anything they can do a project can do.

## Components

Everything about a component is declared in one chain and stored in its schema descriptor:

    sdk.Component[Spinner](r, sdk.Name("game.Spinner")).
        Category("Game").                 // Inspector section and Add Component group
        Display("Spinner").               // card title, defaults to the humanized type name
        Icon("cat.game").                 // icon id from the engine atlas or from r.Icons
        Default(func() Spinner { ... }).  // values used by Add Component, Reset and templates
        Requires("engine.SpriteRenderer").// Add Component is disabled until the node has it; templates add it
        SceneUnique().                    // at most one per scene
        Script("Speed").                  // fields visible to scripts; all fields when omitted, none with ScriptNone()
        Proc0("Reset", fn, sdk.ScriptDoc("...")).
        Template("Spinner", setup).       // Create menu entry under the module's submenu; "" uses the type name
        Err()

Script methods are declared with typed adapters: `Method0..Method2` for functions with a result (`func(*sdk.ScriptCtx, *Spinner, args...) R`) and `Proc0..Proc2` without one. Scripts call them as functions of the module `<module id>/<type>`: `import "mygame/spinner"`, then `spinner.Reset(node)`. `ScriptMethod(name, fn)` with an untyped function still works in development builds through reflection, logs a warning and fails in runtime builds.

A component is added with one file in the module and one line in its `Register`, as `registerSpinner(r)` in `examples/native-rotator`.

Registration errors are collected and reported together when the project starts, with the module id in every message.

## Systems

    r.System(sdk.SystemDesc{ID: "mygame.spin", Phase: sdk.PhaseUpdate, System: spinSystem{}})

System ids must start with the module id. `FrameBegin`, `PollEvents` and `FrameEnd` are reserved for the engine.

## Templates without a component

    r.Template(sdk.TemplateDesc{ID: "mygame.enemy", Name: "Enemy", Category: "Actors", Setup: buildEnemy})

`Space` selects the node space (`sdk.SpaceAny` creates a node in the scene's default space, `sdk.Kind3D` a 3D node).

## Script bindings

    m := r.ScriptModule("mygame/combat")
    sdk.Proc2(m, "Damage", func(c *sdk.ScriptCtx, target sdk.NodeHandle, amount int64) { ... })

## Importers

`sdk.RegisterImporter(r, imp)` adds an asset importer. It exists only in development builds.

## Editor tools

Editor code lives in files tagged `//go:build aego_editor` and is attached with a hook:

    func registerEditor(r *sdk.Registry) {
        sdk.EditorHook(r, func(e *sdk.EditorRegistry) error {
            e.Inspector("mygame.Health", sdk.InspectorFunc(drawHealth))
            e.Panel(&questPanel{})
            return e.Command(sdk.CommandDesc{...})
        })
    }

Provide an empty `registerEditor` in a `//go:build !aego_editor` file. `EditorRegistry` also offers `Gizmo`, `Bounds` (picking and framing shape of a component), `Overlay` (scene view drawing) and `AssetDrop` (what a dropped asset creates). A panic in any of them disables the extension and keeps the editor running.

## Icons

`r.Icons(fsys, "mygame.")` adds `<id>@<size>x<scale>.png` files. Every id used by `Icon` must exist in the engine atlas or in a registered source.
