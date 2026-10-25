---
id: formats/aeanim
icon: file.data
group: ref-formats
order: 1
title: .aeanim clips
---
# .aeanim clips

A clip is an aetext file. Every name below is checked: an unknown block, attribute or field, or an attribute written inside a block, fails the import with [ANM001](ref:ANM001) and the line to fix.

    anim version=1 length=0.38 fps=12 loop=loop {
      track "" prop="SpriteRenderer.size" kind=vec2 {
        key t=0 v=[1, 1] interp=linear {}
        key t=0.13 v=[1.3, 0.84] {}
      }
      frames "Slash" start=0.08 {
        sprites=[asset("5a1a5c00000000000000000000000010"), asset("5a1a5c00000000000000000000000011")]
        durations=[0.04, 0.04]
      }
      event t=0.14 name="Hit" payload=1 {}
    }

## anim

| Attribute | Meaning |
|---|---|
| `version` | text version, 1 |
| `length` | length in seconds, required |
| `fps` | frame rate the editor uses to snap recorded keys and to show time as frames; the runtime ignores it |
| `loop` | `once`, `loop` or `pingPong` |

It holds `track`, `frames` and `event` blocks.

## track

The label is the target: `""` is the node itself, `"Sword"` or `"Arm/Hand"` a descendant.

| Attribute | Meaning |
|---|---|
| `prop` | `Component.field`, required |
| `kind` | `float32`, `float64`, `vec2`, `vec3`, `vec4`, `color`, `rect`, `quat`, `int32`, `int64`, `bool`, `enum` or `assetRef`, required |
| `numeric` | an `int32` or `int64` track that interpolates instead of stepping |
| `root` | the root motion track (`transform.position`, `vec2`) |
| `authority` | `presentation` (default) or `simulation` |
| `onEnd` | `hold` (default) or `restore` |

It holds `key` blocks only.

## key

| Attribute | Meaning |
|---|---|
| `t` | time in seconds, required |
| `v` | value of the track's kind, required |
| `interp` | `bezier`, `linear` or `step` |
| `tangent` | `auto`, `linear` or `free` |
| `in`, `out` | tangents of a `free` key |

## frames

A shorthand for a sprite track with step keys. The label is the target, as for `track`.

| Attribute | Meaning |
|---|---|
| `start` | time of the first frame |
| `fps` | frame rate when there are no `durations`, 12 by default |
| `prop` | sprite property, `SpriteRenderer.sprite` by default |
| `flipProp` | flip property, `SpriteRenderer.flipX` by default |
| `authority` | as for `track` |

| Field | Meaning |
|---|---|
| `sprites` | list of sprite assets, required |
| `durations` | seconds per frame, one per sprite |
| `flipX` | `true` or `false` per sprite |

## event

| Attribute | Meaning |
|---|---|
| `t` | time in seconds, required |
| `name` | event name, required |
| `payload` | a number or `true`/`false` |
| `mode` | `gameplay` (default), `presentation` or `preview` |

---
id: formats/aectrl
icon: anim.graph
group: ref-formats
order: 2
title: .aectrl controllers
---
# .aectrl controllers

A controller is an aetext file. Unknown blocks, attributes and fields fail the import with [ANC006](ref:ANC006); names that do not resolve give [ANC001](ref:ANC001) and [ANC002](ref:ANC002).

    controller version=1 entryPos=[-176, 0] anyPos=[-176, 110] {
      entry="Idle"
      param "Speed" kind=float {}
      param "Attack" kind=trigger {}
      state "Idle" clip=asset("a0a10000000000000000000000000001") pos=[0, 0] {}
      state "Run" clip=asset("a0a10000000000000000000000000002") speed=1.2 pos=[220, 0] {}
      transition "Idle" to="Run" duration=0.1 {
        cond "Speed" op=gt value=0.1 {}
      }
      transition "any" to="Hit" interrupt=true {
        cond "Attack" {}
      }
    }

## controller

| Attribute | Meaning |
|---|---|
| `version` | text version, 1 |
| `ease` | easing of blends, `inOutQuad` by default |
| `entryPos` | `[x, y]` of the Entry pill in the Animator panel |
| `anyPos` | `[x, y]` of the Any State pill in the Animator panel |

| Field | Meaning |
|---|---|
| `entry` | the state the Animator starts in, the first state when left out |

It holds `param`, `state` and `transition` blocks. `pos`, `entryPos` and `anyPos` are for the editor only: the runtime ignores them, and the panel lays out what has no position.

## param

The label is the name.

| Attribute | Meaning |
|---|---|
| `kind` | `bool`, `int`, `float` or `trigger`, required |
| `default` | starting value; triggers have none |

## state

The label is the name; `any` is reserved.

| Attribute | Meaning |
|---|---|
| `clip` | the clip asset |
| `speed` | playback speed, 1 by default |
| `loop` | overrides the clip's loop mode |
| `pos` | `[x, y]` of the box in the Animator panel |
| `events` | during a blend: `dominant` (default), `both` or `incoming` |

## transition

The label is the source state, or `any`. Transitions are tried in file order.

| Attribute | Meaning |
|---|---|
| `to` | target state, required |
| `duration` | blend time in seconds |
| `exitTime` | passes of the state's clip that must have played; 1 is one pass |
| `interrupt` | may cut a running blend |

It holds `cond` blocks only; all must hold.

## cond

The label is the parameter.

| Attribute | Meaning |
|---|---|
| `op` | `set`, `eq`, `ne`, `gt`, `ge`, `lt` or `le`; triggers use `set` |
| `value` | a number or `true`/`false`; required except for `set` |
