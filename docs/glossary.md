# Glossary

One term for one thing. The Russian column holds the terms of the editor translation; the author approves every term.

| Term | Use for | Not | Russian |
|---|---|---|---|
| project | a folder with project.aego | game folder, workspace | проект |
| scene | an .aescene file and what it holds | map | сцена |
| node | an object in a scene | entity, game object | узел |
| component | data and behaviour attached to a node | behaviour, module | компонент |
| script | an .aes file and the type it declares | code file | скрипт |
| prefab | an .aeprefab file and its instances | template | префаб |
| instance | a node made from a prefab | copy, clone | экземпляр |
| asset | a file the project imports | resource | ассет |
| artifact | what importing an asset produces and the game loads | build output | артефакт |
| handle | a value a script holds to refer to something the engine owns, such as a sound or a request | pointer, reference | дескриптор |
| data asset | an .aedata file | config | ассет данных |
| module | a part of the engine: physics, audio | plugin, package | модуль |
| Play | running the game in the editor | preview | играть (команда); во время игры |
| play mode | the state of the editor while Play runs: edits go to a copy | | режим игры |
| edit scene | the scene being edited, outside Play | | редактируемая сцена |
| fixed step | one simulation step at the fixed rate | | фиксированный шаг |
| tick | the count of fixed steps; a time on the timeline is a tick | | тик |
| frame | one drawn image | | кадр |
| world | everything one running game holds: its scene, services and modules | | мир |
| keyframe (rewind) | a saved copy of the game state for rewinding | snapshot | снимок |
| key (animation) | a value at a time in an animation clip | keyframe | ключ |
| clip | an .aeanim file | animation (as a file) | клип |
| controller | an .aectrl file | state machine (as a file) | контроллер |
| signal | a named event a component or script emits | callback | сигнал |
| action | a named input such as Jump or Move, bound to keys and buttons | key binding | действие |
| level | a part of a game the player goes through; it is usually a scene | map, stage | уровень |
| script event | a method the engine calls on scripts, such as OnCollisionEnter | callback, hook | событие скрипта |
| Inspector, Hierarchy, Scene View, Game View | the panels by their titles | | Инспектор, Иерархия, Сцена, Игра |
| workspace | a named arrangement of panels: Design, Programming | layout | раскладка |
| space | whether a scene or node is 2D or 3D | dimension | пространство |
| transform | the position, rotation and scale of a node | | трансформация |
| Watch | the panel that tracks values while the game runs | | Отслеживание |
| Problems, Console | the panels by their titles | | Проблемы, Консоль |
| spawn, despawn | create a node from a prefab while the game runs, remove it | instantiate at runtime | создать, убрать; создание объектов |
| gameplay | the game rules a module provides: health, hitboxes, pickups | | геймплей |
| consume (input) | a context hides its controls from the contexts below | block | перехватывать ввод |
| joint | a constraint between two bodies | | соединение |
| clamp, loop, ping-pong | what a curve does outside its range | | удерживать, повтор, туда-обратно |
| rewind | running the game back to an earlier frame | | перемотка |
| undo, redo | step a change back or forward again | | отменить, повторить |
| hitbox, hurtbox, health | what deals damage, what takes it, how much is left | | хитбокс, хёртбокс, здоровье |
| virtual camera | a camera setup the camera brain blends to | | виртуальная камера |
| follow, shake, zone | what a virtual camera does: track a target, shake, stay inside an area | | слежение, тряска, зона |
| animator, state | the component that plays a controller, and one node of its graph | | аниматор, состояние |
| Try it, Run it | the buttons that run an example on the site and in the editor | | Попробовать, Запустить |
| checkpoint, respawn | where a player comes back after dying, and coming back | | контрольная точка, возрождение |
