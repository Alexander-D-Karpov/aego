---
id: spawn
title: Spawning and pools
order: 26
---
# Spawning and pools

`scene.Spawn` places a copy of a prefab; `scene.Despawn` returns it to a pool, and the next `Spawn` of the same prefab reuses it instead of building a new one.

    @export var Bullet AssetRef<Prefab>

    func Ready() {
      scene.Pool(Bullet, 32)
    }

    func Fire() {
      b := scene.Spawn(Bullet, node.WorldPosition())
    }

| Function | |
|---|---|
| `scene.Spawn(prefab, pos) NodeRef` | under the scene root at a 2D position |
| `scene.SpawnAt(prefab, parent, pos, rot) NodeRef` | under a parent with a local position and rotation |
| `scene.Despawn(node)` | back to the pool; a node that did not come from a pool is destroyed |
| `scene.Pool(prefab, n)` | build `n` copies now; their `Ready` runs now |

---
id: spawn/lifecycle
parent: spawn
order: 1
title: Lifecycle
---
# Lifecycle

- A new copy runs `Ready`, then `OnSpawned()`.
- A pooled copy gets the values it had when it was built: component fields, child poses and enabled flags, and every script field back to its default and its prefab value. `Ready` does not run again, `OnSpawned()` does.
- `Despawn` runs `OnDespawned()`, turns the copy off at once and parks it at the end of the frame: its coroutines and signal connections end, its scripts stop, and it moves under a disabled `Pool` node.
- Components and children added while the copy was out stay; the pool resets values, not structure.
- A physics body keeps its velocity across a respawn. Set it in `OnSpawned`.
- `Spawn` sets the pose of 2D copies only; 3D copies keep the prefab's pose.
