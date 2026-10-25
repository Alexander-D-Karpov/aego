---
id: engine.SpriteRenderer
---
## Из скрипта

При попадании спрайт вспыхивает белым и отражается по направлению движения.

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
## Из скрипта

Пока клавиша зажата, камера плавно приближается, а когда её отпускают — отдаляется обратно.

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
## Из скрипта

Во время бега поле зрения расширяется.

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
## Из скрипта

Цвет меша пульсирует — например, у подбираемого предмета.

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
## Из скрипта

Скрипты читают теги через `aego/scene`: найдите все узлы с тегом или проверьте узел, с которым столкнулись.

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
## Из скрипта

Бег по действию `Move`, прыжок импульсом и чтение скорости; собственные поля тела по-прежнему можно менять из скрипта.

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
## Из скрипта

Коллайдер растёт вместе со спрайтом — для усиления.

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
## Из скрипта

Мяч становится упруже при каждом приземлении, до предела.

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
## Из скрипта

Приседание укорачивает капсулу.

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
## Из скрипта

В скриптах `Points` доступно только для чтения; форму правят в Инспекторе. Вместо неё скрипты меняют поверхность.

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
## Из скрипта

Платформа становится односторонней во время игры.

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
## Из скрипта

Верёвка сматывается и разматывается изменением длины соединения.

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
## Из скрипта

Дверь открывается мотором шарнира и свободно качается, когда мотор выключен.

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
## Из скрипта

`Persist` отмечает узел для сохранений; когда сохранять и загружать, решает скрипт.

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
