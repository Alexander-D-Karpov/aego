---
id: engine.SpriteRenderer
---
## From a script

Flash the sprite white when hit and mirror it to face the direction of travel.

```aes check
package player

script HitFlash {
    var Flash float = 0.1
    var timer float
    var base Color

    func Ready() {
        base = node.Get<SpriteRenderer>().Tint
    }

    func Hit() {
        node.Get<SpriteRenderer>().Tint = Color{1, 1, 1, 1}
        timer = Flash
    }

    func Face(dir float) {
        sr := node.Get<SpriteRenderer>()
        sr.FlipX = dir < 0
    }

    func Update(dt float) {
        if timer > 0 {
            timer -= dt
            if timer <= 0 {
                node.Get<SpriteRenderer>().Tint = base
            }
        }
    }
}
```

---
id: engine.Camera2D
---
## From a script

Zoom the camera in smoothly while a key is held, and back out when it is released.

```aes check
package game

import (
    "aego/input"
    "aego/math"
)

script ZoomCamera {
    var Normal float = 40
    var Close float = 80

    func Update(dt float) {
        cam := node.Get<Camera2D>()
        target := Normal
        if input.Down(input.KeyZ) {
            target = Close
        }
        cam.Zoom = math.Lerp(cam.Zoom, target, math.Clamp(dt*6, 0, 1))
    }
}
```

---
id: engine.Camera3D
---
## From a script

Widen the field of view while sprinting.

```aes check
package game

import "aego/math"

script SprintFov {
    var Walk float = 60
    var Sprint float = 75
    var Sprinting bool

    func Update(dt float) {
        cam := node.Get<Camera3D>()
        target := Walk
        if Sprinting {
            target = Sprint
        }
        cam.Fov = math.Lerp(cam.Fov, target, math.Clamp(dt*4, 0, 1))
    }
}
```

---
id: engine.MeshRenderer
---
## From a script

Pulse the mesh colour, for example on a pickup.

```aes check
package game

import (
    "aego/math"
    "aego/time"
)

script Pulse {
    var Speed float = 3

    func Update(dt float) {
        mr := node.Get<MeshRenderer>()
        k := 0.75 + 0.25*math.Sin(time.Elapsed()*Speed)
        mr.Color = Color{k, k, 1, 1}
    }
}
```

---
id: engine.Tags
---
## From a script

Tags are read from scripts through `aego/scene`: find every node with a tag, or check a node you collided with.

```aes check
package game

import (
    "aego/log"
    "aego/scene"
)

script CountEnemies {
    var found []NodeRef

    func Ready() {
        for i := 0; i < 32; i++ {
            found = append(found, nil)
        }
        n := scene.WithTag("Enemy", found)
        log.Info("enemies in the level: " + string(n))
    }
}
```

---
id: physics.Body2D
---
## From a script

Run with the `Move` action, jump with an impulse and read the velocity back; the body's own fields stay editable from the script.

```aes check
package player

import "aego/input"

script Runner {
    var Speed float = 6
    var Jump Vec2 = Vec2{0, -6}

    func Update(dt float) {
        body := node.Get<Body2D>()
        dir := input.Axis("Move")
        body.SetVelocity(Vec2{dir * Speed, body.Velocity.Y})
        if input.Pressed("Jump") {
            body.ApplyImpulse(Jump * body.Mass())
        }
        body.GravityScale = 1
        if body.Velocity.Y > 0 {
            body.GravityScale = 2
        }
    }
}
```

---
id: physics.BoxCollider2D
---
## From a script

Grow the collider together with the sprite, for a power-up.

```aes check
package player

script Grow {
    func Apply(factor float) {
        box := node.Get<BoxCollider2D>()
        box.Size = box.Size * factor
        sr := node.Get<SpriteRenderer>()
        sr.Size = sr.Size * factor
    }
}
```

---
id: physics.CircleCollider2D
---
## From a script

Make a ball bouncier every time it lands, up to a limit.

```aes check
package game

import (
    "aego/math"
    "aego/physics"
)

script Bouncy {
    func OnCollisionEnter(other NodeRef, contact physics.Contact) {
        c := node.Get<CircleCollider2D>()
        c.Restitution = math.Min(c.Restitution+0.1, 0.9)
    }
}
```

---
id: physics.CapsuleCollider2D
---
## From a script

Crouch by shortening the capsule.

```aes check
package player

import "aego/input"

script Crouch {
    var Standing float = 1
    var Crouching float = 0.4

    func Update(dt float) {
        capsule := node.Get<CapsuleCollider2D>()
        capsule.Length = Standing
        if input.Held("Crouch") {
            capsule.Length = Crouching
        }
    }
}
```

---
id: physics.PolygonCollider2D
---
## From a script

`Points` is read-only in scripts; the shape is edited in the Inspector. Scripts change the surface instead.

```aes check
package game

script Ice {
    func Ready() {
        poly := node.Get<PolygonCollider2D>()
        poly.Friction = 0.02
        poly.Restitution = 0
    }
}
```

---
id: physics.EdgeCollider2D
---
## From a script

Turn a platform into a one-way platform at run time.

```aes check
package level

script OneWay {
    var Enabled bool = true

    func Ready() {
        edge := node.Get<EdgeCollider2D>()
        edge.OneSided = Enabled
    }
}
```

---
id: physics.DistanceJoint2D
---
## From a script

Reel a rope in and out by changing the joint length.

```aes check
package player

import (
    "aego/input"
    "aego/math"
)

script Rope {
    var Reel float = 2

    func Update(dt float) {
        j := node.Get<DistanceJoint2D>()
        j.Length = math.Max(j.Length+input.Axis("Reel")*Reel*dt, 0.5)
        j.MaxLength = j.Length
    }
}
```

---
id: physics.RevoluteJoint2D
---
## From a script

Open a door with the joint motor and let it swing freely when the motor is off.

```aes check
package level

script Door {
    var Open bool

    func Toggle() {
        Open = !Open
        hinge := node.Get<RevoluteJoint2D>()
        hinge.EnableMotor = Open
        hinge.Speed = 1.5
        hinge.MaxTorque = 200
    }
}
```

---
id: save.Persist
---
## From a script

`Persist` marks a node for save games; the script decides when to save and load.

```aes check
package game

import (
    "aego/input"
    "aego/save"
)

script QuickSave {
    func Update(dt float) {
        if input.KeyPressed(input.KeyF5) {
            save.Save("quick")
        }
        if input.KeyPressed(input.KeyF9) && save.Exists("quick") {
            save.Load("quick")
        }
    }
}
```
