---
id: curves
icon: vec.angular
group: gameplay
title: Curves
order: 4
---
# Curves

A curve maps time to a value. Component fields of type `curve.Curve` describe falloffs, speed ramps and fades; a tween can follow a curve instead of an easing.

---
id: curves/shape
icon: code.hints
parent: curves
order: 1
title: Presets and keys
---
# Presets and keys

A curve is either a preset or a list of keys. A preset is quick to pick; keys give any shape.

## Presets

A preset is an easing on time `0..1`: `linear` and the `in`/`out`/`inOut` family of quad, cubic, sine, expo, back, elastic and bounce.

## Keys

Each key has a time, a value, two slopes, a segment mode and a tangent mode.

| Segment | From this key to the next |
|---|---|
| Bezier | smooth, shaped by the slopes |
| Linear | a straight line |
| Step | holds this key's value |

| Tangent | Slopes |
|---|---|
| Auto | follow the neighbours and never overshoot them |
| Linear | point at the neighbouring keys |
| Free | set by hand; In and Out may differ |

**Before** and **After** decide what happens outside the keys: `Clamp` holds the end value, `Loop` repeats, `Ping-pong` repeats mirrored. Curves are float32 and evaluate identically on every platform.

---
id: curves/editor
icon: edit.rename
parent: curves
order: 2
title: Editing curves
---
# Editing curves

The Inspector draws the curve inside the field; a click opens the curve editor.

- **Preset** switches between presets and **Keys**. Choosing Keys on a preset turns it into editable keys of the same shape.
- **Double-click** the graph to add a key on the curve, **right-click** a key to remove it, drag a key or a slope handle to change it.
- Hold **Ctrl** (Cmd on macOS) to snap to a 1/20 grid of the view, hold **Shift** while dragging a slope handle to move one side only.
- **Time**, **Value**, **Segment** and **Tangent** edit the selected key; dragging a handle makes its tangent Free.

Every drag is one undo step. Prefab instances show an overridden curve like any other field.

---
id: curves/scripts
icon: file.script
parent: curves
order: 3
title: Curves in scripts
---
# Curves in scripts

A script reads a curve field of a component and evaluates it with `aego/curve`:

    import "aego/curve"

    func Update(dt float) {
      age += dt
      fade := node.Get<Fader>().Alpha
      sr.Tint.A = curve.Eval(fade, age / curve.Duration(fade))
    }

| Function | |
|---|---|
| `Eval(c, t) float` | the value at `t`, wrapped by Before and After |
| `EvalClamped(c, t) float` | the value at `t` clamped to the first and last key |
| `Duration(c) float` | time from the first to the last key, 1 for a preset |
| `Ease(e, t) float` | a preset easing at `t` clamped to `0..1` |

The constants `curve.Linear`, `curve.OutQuad` and the rest have the type `curve.Easing` and are the `ease` argument of tweens.

A `Curve` value is a read-only view of the field, not a copy: it sees edits made after it was read. Scripts cannot build curves or export curve fields. A view whose node or component is gone faults with `AES6004`.

---
id: curves/go
icon: code.brackets
parent: curves
order: 4
title: Curves in Go
---
# Curves in Go

    type Fader struct {
      Alpha curve.Curve `aego:"alpha"`
    }

    c := curve.Keyed(curve.Key{T: 0, V: 1}, curve.Key{T: 0.5, V: 1, Interp: curve.InterpLinear}, curve.Key{T: 1, V: 0})
    v := c.Eval(0.75)

| Function | |
|---|---|
| `FromEasing(e)`, `Line(t0, v0, t1, v1)`, `Constant(v)`, `Keyed(keys...)` | construction |
| `Eval`, `EvalClamped`, `Span`, `Duration`, `Range(samples)` | reading |
| `Fix`, `Smooth`, `Normalize`, `Bake(samples)` | editing helpers |

`Keyed` and `Fix` sort the keys, drop keys with NaN and keep the last of two keys at the same time. Change `Keys` directly only together with `Fix` or `Smooth`.

In scenes and prefabs a curve is written as a struct; defaults are omitted and a preset has no keys:

    falloff={ ease=outQuad post=pingPong }
    alpha={ keys=[[0, 1, 0, 0, bezier, auto], [1, 0, -2, -2, linear, free]] }
