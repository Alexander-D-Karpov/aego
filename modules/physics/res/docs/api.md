---
id: aego/physics.Contact.Point
---
Stores the world-space contact point in world units, averaged when the manifold has several points. Exit events retain the last known point.

---
id: aego/physics.Contact.Normal
---
Points from the other body into the body receiving the collision callback; the opposite receiver gets the negated direction.

---
id: aego/physics.Contact.NormalImpulse
---
Stores the summed normal impulse at the contact, in mass×world-units/second. An exit event reports zero.

---
id: aego/physics.Contact.RelativeVelocity
---
Stores this body's velocity relative to the other at Point, in world units per second, including angular motion at that point.

---
id: aego/physics.Contact.SelfCollider
---
References the receiving side's collider node; it can differ from the rigid-body node when the collider is a child.

---
id: aego/physics.Contact.OtherCollider
---
References the other side's collider node, not necessarily its rigid-body node. Check validity if that node may have been destroyed.

---
id: aego/physics.RayHit.Hit
---
Reports whether the query found a collider. Check this before using the other result fields; a miss returns an empty result.

---
id: aego/physics.RayHit.Point
---
Stores the hit position in world coordinates and world units when Hit is true.

---
id: aego/physics.RayHit.Normal
---
Stores the surface normal at the hit point in world space when Hit is true.

---
id: aego/physics.RayHit.Distance
---
Stores distance from the query origin to the hit in world units, not a normalised fraction of the query length.

---
id: aego/physics.RayHit.Node
---
References the rigid-body node that owns the hit shape; Collider identifies the actual shape node.

---
id: aego/physics.RayHit.Collider
---
References the node containing the collider shape that was hit; it may be a child of the body returned in Node.

---
id: aego/physics
summary: Ray, line and overlap queries against the 2D physics world, layer lookups and world gravity.
---

Queries work only while physics runs, during Play and in `aego run`; outside it every function stops the script with `AES6010`. The `Overlap*` and `RaycastAll` functions fill a slice you pass, never more than its length, and return how many they wrote. `Layer` and `LayerMask` take a layer declared in Project Settings → Physics and stop the script for an unknown name; a mask of `-1` means every layer. The documentation sandbox has no physics world, so the example keeps every query in `OnCollisionEnter`, which runs once the node has a Rigid Body 2D and a collider and touches another body. See [Physics 2D](ref:physics).

## Example

```aes run
package demo

import (
    "aego/log"
    "aego/physics"
)

script Grenade {
    @export var Radius float = 2

    var hits []NodeRef
    var exploded bool

    func Ready() {
        for i := 0; i < 16; i++ {
            hits = append(hits, nil)
        }
        log.Info("waiting for a collision; this node needs a Rigid Body 2D and a collider")
    }

    func OnCollisionEnter(other NodeRef, contact physics.Contact) {
        if exploded {
            return
        }
        exploded = true
        log.Info("hit " + other.Name() + " with vertical speed " + string(contact.RelativeVelocity.Y))
        n := physics.OverlapCircle(contact.Point, Radius, hits)
        for i := 0; i < n; i++ {
            wall := physics.Linecast(contact.Point, hits[i].WorldPosition(), -1)
            if !wall.Hit {
                log.Info("blast reaches " + hits[i].Name())
            }
        }
        down := physics.Raycast(node.WorldPosition(), Vec2{0, 1}, 5)
        if down.Hit {
            log.Info("ground " + string(down.Distance) + " units below, gravity " + string(physics.Gravity().Y))
        }
    }
}
```
