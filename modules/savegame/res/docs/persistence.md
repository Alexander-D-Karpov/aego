---
id: save
icon: panel.saves
group: gameplay
title: Saving games
order: 6
---
# Saving games

A save holds what you mark as persistent, not the whole scene. Loading a save reloads its scene from the project and then applies the save on top.

| Mark | What is saved |
|---|---|
| `save.Persist` component | the node's transform and enabled flag |
| `sdk.Persist("field", …)` on a component type | those fields of every such component |
| `@persist` on a script field | that field of every script instance |
| `OnSave()` / `OnLoad()` | values the script writes with `save.Put*` |

Besides that a save remembers which scene nodes were destroyed and every object made with `scene.Spawn`: its prefab, parent, pose and persistent data.

    @persist
    var Coins int

    func OnSave() {
      save.PutVec2("checkpoint", lastCheckpoint)
    }

    func OnLoad() {
      lastCheckpoint = save.GetVec2("checkpoint", node.Position())
    }

| Function | |
|---|---|
| `save.Save(slot)` | write the game to a slot at the end of the frame |
| `save.Load(slot)` | reload the saved scene and restore the slot |
| `save.Checkpoint()` | save to the slot `checkpoint` |
| `save.Autosave(seconds)` | save to `autosave` every given number of real seconds |
| `save.Exists(slot)`, `save.Delete(slot)` | |
| `save.SetGlobalInt(key, v)`, `save.GlobalInt(key, def)` | game-wide progress carried by every save; also Float, Bool, String |

Slot names use letters, digits, `-` and `_`. Saves live next to the player settings, in `saves/` of the profile.

---
id: save/load
icon: file.open
parent: save
order: 1
title: Loading order
---
# Loading order

1. The scene of the save is loaded fresh.
2. Scene nodes the save lists as destroyed are destroyed.
3. Spawned objects are spawned again with their persistent ids.
4. Transforms, component fields and `@persist` fields are restored.
5. Scripts run `Ready`, then `OnLoad`.

Fields restored in step 4 are already set when `Ready` runs. A field that is not marked comes from the scene, as on a fresh start.

References between saved nodes survive: a `NodeRef` to a scene node is saved by its id, a reference into a spawned object by the object's persistent id and the path inside it.

---
id: save/go
icon: code.brackets
parent: save
order: 2
title: Modules
---
# Modules

A module saves its own state with a section:

    r.SaveSection("mygame.quests", questSection{})

A section has `Version`, `Save(world, w)`, `Load(world, r)` and `Migrate(node, from, to)`, which runs when a save was written by another version of the section. Sections of modules that are not loaded are kept and written back unchanged.
