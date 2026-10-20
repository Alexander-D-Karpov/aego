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
