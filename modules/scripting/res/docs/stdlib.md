---
id: language/standard-library
icon: code.block
parent: language
order: 10
title: Standard library
---
# Standard library

The standard library is the set of `aego/…` packages every script can import. Each package has its own page in the Script API reference with every function; this page says what each one is for.

| Package | For |
|---|---|
| [`aego/math`](ref:aego/math) | numbers, vectors and rotations |
| [`aego/log`](ref:aego/log) | messages in the Console |
| [`aego/rand`](ref:aego/rand) | random numbers from the game's seed |
| [`aego/time`](ref:aego/time) | frame time, fixed step and elapsed time |
| [`aego/input`](ref:aego/input) | actions, contexts and raw keys; see [Input](ref:input) |
| [`aego/settings`](ref:aego/settings) | player and project settings; see [Settings](ref:settings) |
| [`aego/scene`](ref:aego/scene) | finding, creating and destroying nodes |
| [`aego/testing`](ref:aego/testing) | assertions in script tests |

Engine modules add their own packages, such as `aego/physics` and `aego/audio`. They are listed in the same reference.

## Rules worth knowing

- There is no overloading. Vec3 variants carry a `3` suffix (`Dot3`), int variants an `Int` suffix (`ClampInt`).
- Build log messages with `+` and `string(x)`.
- `aego/rand` draws from the world's seeded generator, so a replay with the same seed gives the same numbers.
- Nodes made with `scene.Instantiate` appear at the end of the phase. Their scripts receive `Ready` before the next `Update`.
- `aego/testing` is available only under `aego test`. A failed assertion stops the test and reports the file and line.

Run it:

```aes run
package demo

import (
    "aego/log"
    "aego/math"
    "aego/rand"
    "aego/scene"
    "aego/time"
)

script Wander {
    @export var Speed float = 2

    var dir Vec2

    func Ready() {
        dir = math.Normalize(Vec2{rand.Range(-1, 1), rand.Range(-1, 1)})
        log.Info("found myself: " + string(scene.Find("Wander") == node))
    }

    func Update(dt float) {
        node.SetPosition(node.Position() + dir * Speed * dt)
        if time.Frame() % 40 == 0 {
            p := node.Position()
            log.Info("t=" + string(time.Elapsed()) + " s, " + string(math.Length(p)) + " units from the start")
        }
    }
}
```
