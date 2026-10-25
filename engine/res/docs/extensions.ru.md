---
id: extensions
title: Нативные расширения
---
# Нативные расширения

Проект расширяет движок кодом на Go в `native/`. Он импортирует только `aego/sdk` и реализует `sdk.Module`:

    type Module struct{}

    func (Module) Info() sdk.ModuleInfo { return sdk.ModuleInfo{ID: "mygame", Requires: []string{"render2d"}} }
    func (Module) Register(r *sdk.Registry) error { ... }

Подсистемы движка (`render2d`, `render3d`, `script`) регистрируются через тот же реестр, поэтому проект может всё, что могут они.

## Компоненты

Всё о компоненте объявляется одной цепочкой и хранится в его дескрипторе схемы:

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

Методы для скриптов объявляются типизированными адаптерами: `Method0..Method2` для функций с результатом (`func(*sdk.ScriptCtx, *Spinner, args...) R`) и `Proc0..Proc2` для функций без него. Скрипты вызывают их как функции модуля `<module id>/<type>`: `import "mygame/spinner"`, затем `spinner.Reset(node)`. `ScriptMethod(name, fn)` с нетипизированной функцией по-прежнему работает в сборках для разработки через рефлексию, пишет предупреждение и не работает в сборках для выпуска.

Компонент добавляется одним файлом в модуле и одной строкой в его `Register`, как `registerSpinner(r)` в `examples/native-rotator`.

Ошибки регистрации собираются и выводятся вместе при запуске проекта, с id модуля в каждом сообщении.

## Системы

    r.System(sdk.SystemDesc{ID: "mygame.spin", Phase: sdk.PhaseUpdate, System: spinSystem{}})

Id систем должны начинаться с id модуля. `FrameBegin`, `PollEvents` и `FrameEnd` зарезервированы за движком.

## Шаблоны без компонента

    r.Template(sdk.TemplateDesc{ID: "mygame.enemy", Name: "Enemy", Category: "Actors", Setup: buildEnemy})

`Space` выбирает пространство узла (`sdk.SpaceAny` создаёт узел в пространстве сцены по умолчанию, `sdk.Kind3D` — 3D-узел).

## Привязки для скриптов

    m := r.ScriptModule("mygame/combat")
    sdk.Proc2(m, "Damage", func(c *sdk.ScriptCtx, target sdk.NodeHandle, amount int64) { ... })

## Импортёры

`sdk.RegisterImporter(r, imp)` добавляет импортёр ассетов. Он есть только в сборках для разработки.

## Инструменты редактора

Код редактора лежит в файлах с тегом `//go:build aego_editor` и подключается хуком:

    func registerEditor(r *sdk.Registry) {
        sdk.EditorHook(r, func(e *sdk.EditorRegistry) error {
            e.Inspector("mygame.Health", sdk.InspectorFunc(drawHealth))
            e.Panel(&questPanel{})
            return e.Command(sdk.CommandDesc{...})
        })
    }

Положите пустую `registerEditor` в файл с `//go:build !aego_editor`. `EditorRegistry` также предлагает `Gizmo`, `Bounds` (форма компонента для выбора и кадрирования), `Overlay` (рисование на панели «Сцена») и `AssetDrop` (что создаёт брошенный ассет). Паника в любом из них отключает расширение, а редактор продолжает работать.

## Иконки

`r.Icons(fsys, "mygame.")` добавляет файлы `<id>@<size>x<scale>.png`. Каждый id, который использует `Icon`, должен быть в атласе движка или в зарегистрированном источнике.
