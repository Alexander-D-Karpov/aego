---
id: aego/builtin
summary: Methods of the builtin `NodeRef` type: hierarchy, tags, enabled state and 2D and 3D transforms.
---

Every script reaches its own node through the implicit `node` field, and other nodes through `NodeRef` fields, `Find` and `Child`. Transforms are local to the parent and read and written by value; a 2D method on a 3D node faults. See [Nodes, components and references](ref:language/nodes-components-and-references) for components and script references.

## Example

```aes run
package demo

import (
    "aego/log"
    "aego/math"
)

script Door {
    @export var OpenAngle float = 90

    var frames int

    func Ready() {
        log.Info("node " + node.Name() + ", 2D: " + string(node.Space() == 0) + ", top level: " + string(node.Parent() == nil))
        node.AddTag("Interactable.Door")
        log.Info("is interactable: " + string(node.HasTag("Interactable")))
        node.SetScale(Vec2{-1, 1})
        t := node.Transform2D()
        t.Position.X += 2
        node.SetTransform2D(t)
        log.Info("moved to " + string(node.Position().X) + ", mirrored: " + string(node.Scale().X < 0))
    }

    func Update(dt float) {
        frames++
        target := math.Deg2Rad(OpenAngle)
        node.SetRotation(math.Min(node.Rotation() + 2 * dt, target))
        if frames % 30 == 0 {
            log.Info("angle " + string(math.Round(math.Rad2Deg(node.Rotation()))) + " deg")
        }
        if frames == 100 {
            removed := node.RemoveTag("Interactable.Door")
            node.SetEnabled(false)
            log.Info("tag removed: " + string(removed) + ", enabled: " + string(node.Enabled()))
        }
    }
}
```

---
id: aego/math
summary: Float, int, vector and rotation math: trigonometry, clamping, interpolation, Vec2 and Vec3 helpers and quaternions.
---

There is no overloading: Vec3 variants carry a `3` suffix (`Length3`, `Dot3`) and int variants an `Int` suffix (`MinInt`, `ClampInt`). Angles are in radians; `Deg2Rad` and `Rad2Deg` convert. Vector arithmetic (`v + w`, `v * s`) is part of the language and needs no import.

## Example

```aes run
package demo

import (
    "aego/log"
    "aego/math"
)

script Orbit {
    @export var Radius float = 3
    @export var Center Vec2 = Vec2{1, 1}

    var angle float
    var frames int

    func Ready() {
        to := Vec2{3, 4}
        log.Info("length " + string(math.Length(to)) + ", heading " + string(math.Round(math.Rad2Deg(math.Angle(to)))) + " deg")
        log.Info("unit " + string(math.Normalize(to).X) + ", " + string(math.Normalize(to).Y))
        log.Info("clamp 12 to 0..10: " + string(math.Clamp(12, 0, 10)))
        log.Info("ammo after pickup: " + string(math.MinInt(25 + 10, 30)))
    }

    func Update(dt float) {
        angle += math.Tau / 2 * dt
        frames++
        if frames % 30 == 0 {
            p := Center + Vec2{math.Cos(angle), math.Sin(angle)} * Radius
            node.SetPosition(p)
            log.Info("frame " + string(frames) + ": at " + string(math.Round(p.X * 100) / 100) + ", " + string(math.Round(p.Y * 100) / 100) + ", " + string(math.Dist(p, Center)) + " from center")
        }
    }
}
```

---
id: aego/log
summary: Writes messages to the Console at four levels: Debug, Info, Warn and Error.
---

Build messages with `+` and `string(x)`. None of the functions stops the script; `Error` is for problems the script can recover from. Debug messages are hidden in the editor's Console unless it runs with `-debug`; the Run output below shows every level.

## Example

```aes run
package demo

import "aego/log"

script Health {
    @export var Max int = 3

    var hp int
    var frames int

    func Ready() {
        hp = Max
        log.Info(node.Name() + " starts with " + string(hp) + " hp")
        log.Debug("Debug lines are hidden in the editor unless it runs with -debug")
    }

    func Update(dt float) {
        frames++
        if frames % 30 != 0 || hp == 0 {
            return
        }
        hp--
        if hp == 0 {
            log.Error("died on frame " + string(frames))
        } else if hp == 1 {
            log.Warn("one hit left")
        } else {
            log.Info("hit, " + string(hp) + " hp left")
        }
    }
}
```

---
id: aego/rand
summary: Random numbers from the world's seeded generator, so a replay with the same seed gives the same numbers.
---

`Int(min, max)` and `Range(min, max)` include `min` and exclude `max`. Every script of a world draws from the same generator, so the numbers a script gets depend on the order of calls in the frame. The documentation sandbox always runs with seed 1.

## Example

```aes run
package demo

import (
    "aego/log"
    "aego/rand"
)

script Loot {
    @export var DropChance float = 0.5

    func Ready() {
        log.Info("seed " + string(rand.Seed()))
        for i := 0; i < 5; i++ {
            roll := rand.Int(1, 7)
            if rand.Float() < DropChance {
                gold := rand.Int(5, 20)
                log.Info("chest " + string(i) + ": rolled " + string(roll) + ", drops " + string(gold) + " gold")
            } else {
                log.Info("chest " + string(i) + ": rolled " + string(roll) + ", empty")
            }
        }
        delay := rand.Range(0.5, 1.5)
        log.Info("next spawn in " + string(delay) + " s, facing left: " + string(rand.Bool()))
    }
}
```

---
id: aego/time
summary: Reads and controls the world clock: frame delta, elapsed game time, frame and tick counters, pause and time scale.
---

`Delta` and `Elapsed` count game time, which follows the time scale and stops while paused; `Unscaled` and `UnscaledElapsed` count real time. `SetPaused` and `SetScale` change the clock of a running game; the documentation sandbox has no world clock, so calling them there stops the script, and the example only reads the clock. See [Time](ref:time).

## Example

```aes run
package demo

import (
    "aego/log"
    "aego/math"
    "aego/time"
)

script Bob {
    @export var Amplitude float = 0.5

    func Update(dt float) {
        y := math.Sin(time.Elapsed() * 3) * Amplitude
        node.SetPosition(Vec2{0, y})
        if time.Frame() % 30 == 0 {
            log.Info("frame " + string(time.Frame()) + ", elapsed " + string(time.Elapsed()) + " s, delta " + string(time.Delta()))
        }
        if time.Frame() == 60 {
            log.Info("scale " + string(time.Scale()) + ", paused " + string(time.IsPaused()) + ", fixed step " + string(time.Fixed()))
            log.Info("real time since start " + string(time.UnscaledElapsed()) + " s")
        }
    }
}
```

---
id: aego/scene
summary: Finds, creates and destroys nodes of the running scene: lookups by path, id, tag, component and area, prefab instances and pools.
---

`Find` takes a path from the scene root. The `With*` and `InArea` functions fill a slice you pass, never more than its length, and return how many nodes they wrote. New nodes and destroyed nodes take effect at the end of the phase. `Spawn`, `SpawnAt`, `Pool` and `Despawn` come from the Spawning module, which the documentation sandbox does not load; `Instantiate` needs a prefab assigned in the Inspector, so the example skips it when the field is empty.

## Example

```aes run
package demo

import (
    "aego/log"
    "aego/scene"
)

script Crate {
    func Ready() {
        node.SetPosition(Vec2{1, 0})
        node.AddTag("Breakable")
    }
}

script Level {
    @export var Coin AssetRef<Prefab>

    var found []NodeRef
    var frames int

    func Ready() {
        for i := 0; i < 8; i++ {
            found = append(found, nil)
        }
        if Coin != nil {
            coin := scene.Instantiate(Coin, node)
            coin.SetPosition(Vec2{0, -2})
        } else {
            log.Info("Coin prefab is not set, nothing to spawn")
        }
    }

    func Update(dt float) {
        frames++
        if frames == 1 {
            n := scene.WithComponent("SpriteRenderer", found)
            log.Info(string(n) + " nodes with a SpriteRenderer")
            n = scene.WithTag("Breakable", found)
            log.Info(string(n) + " breakable, first is " + found[0].Name())
            n = scene.InArea(Rect{0.5, -0.5, 1, 1}, -1, found)
            log.Info(string(n) + " node in the area right of the origin")
        }
        if frames == 60 {
            crate := scene.Find("Crate")
            if crate != nil {
                log.Info("destroying " + crate.Name())
                scene.Destroy(crate)
            }
        }
        if frames == 61 {
            log.Info("Crate still found: " + string(scene.Find("Crate") != nil))
        }
    }
}
```

---
id: aego/curve
summary: Evaluates easing presets and the curve fields of components.
---

`Ease` takes a preset such as `curve.OutCubic` and a time from 0 to 1. `Eval`, `EvalClamped` and `Duration` take a `Curve`, which comes from a curve field of a component (`fader.Alpha`); the documentation sandbox has no component with a curve field, so the example uses `Ease`. See [Curves](ref:curves).

## Example

```aes run
package demo

import (
    "aego/curve"
    "aego/log"
    "aego/math"
)

script Slide {
    @export var From Vec2 = Vec2{0, 0}
    @export var To Vec2 = Vec2{4, 0}
    @export var Duration float = 1.5

    var age float

    func Ready() {
        log.Info("at t=0.5: linear " + string(curve.Ease(curve.Linear, 0.5)) + ", in quad " + string(curve.Ease(curve.InQuad, 0.5)) + ", out cubic " + string(curve.Ease(curve.OutCubic, 0.5)))
        log.Info("out back overshoots: " + string(curve.Ease(curve.OutBack, 0.8)))
    }

    func Update(dt float) {
        if age >= Duration {
            return
        }
        age += dt
        k := curve.Ease(curve.InOutSine, age / Duration)
        node.SetPosition(math.LerpVec2(From, To, k))
        if age >= Duration {
            log.Info("arrived at x=" + string(node.Position().X))
        } else if int(age * 60) % 15 == 0 {
            log.Info("x=" + string(node.Position().X))
        }
    }
}
```

---
id: aego/testing
summary: Assertions for script tests: a failed check stops the test and reports the file and line.
---

Test functions are package functions named `TestXxx` in `*_test.aes` files, and `aego test` runs them headless. The documentation sandbox does not run `aego test`, so the example calls its test function from `Ready`; in a project, move `TestDamage` into a `_test.aes` file and drop the script.

## Example

```aes run
package demo

import (
    "aego/log"
    "aego/math"
    "aego/testing"
)

func damage(hp int, hit int) int {
    return math.MaxInt(hp - hit, 0)
}

func label(hp int) string {
    if hp == 0 {
        return "dead"
    }
    return "alive"
}

func TestDamage() {
    testing.Equal(damage(10, 3), 7)
    testing.Equal(damage(2, 5), 0)
    testing.EqualString(label(damage(2, 5)), "dead")
    testing.EqualFloat(math.Sqrt(2), 1.4142, 0.001)
    testing.True(damage(10, 0) == 10, "a zero hit changes nothing")
    testing.False(damage(1, 1) > 0, "the last hit kills")
}

script Runner {
    func Ready() {
        TestDamage()
        log.Info("TestDamage passed")
    }
}
```

---
id: events
---

Declare the method in a script; the module that owns the event calls it when the event happens. The trigger events below come from Physics 2D and need a body with a trigger collider, which the documentation sandbox does not have, so the example calls the same `Collect` method that `OnTriggerEnter` calls.

## Example

```aes run
package demo

import "aego/log"

script Coin {
    @export var Value int = 10

    var taken bool

    func Ready() {
        log.Info("coin ready")
    }

    func OnTriggerEnter(other NodeRef) {
        if other.HasTag("Player") {
            Collect(other)
        }
    }

    func OnTriggerExit(other NodeRef) {
        log.Info(other.Name() + " left the coin")
    }

    func Collect(by NodeRef) {
        if taken {
            return
        }
        taken = true
        log.Info(by.Name() + " picked up " + string(Value) + " coins")
        node.Destroy()
    }
}

script Player {
    var frames int

    func Ready() {
        node.AddTag("Player")
    }

    func Update(dt float) {
        frames++
        if frames == 30 {
            coin := node.Find("/Coin")
            if coin != nil {
                coin.GetScript<Coin>().Collect(node)
            }
        }
        if frames == 31 {
            log.Info("coin left in the scene: " + string(node.Find("/Coin") != nil))
        }
    }
}
```
