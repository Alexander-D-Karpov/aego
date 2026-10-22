---
id: camera
try: try/CameraFollow2D.aescene
try_select: camera.CameraFollow2D
icon: comp.vcam
group: camera
group_title: Cameras 2D
group_icon: cat.camera
group_order: 55
title: Cameras 2D
order: 1
---
# Cameras 2D

A [Camera 2D](ref:engine.Camera2D) draws the view; virtual cameras decide where it looks. Every [Virtual Camera 2D](ref:camera.VirtualCamera2D) is a candidate pose. The enabled one with the highest priority drives the view, and switching between them blends. Follow, shake and zones are components on virtual cameras; scripts trigger them.

Quick start: Create → Follow Camera gives a node with Camera 2D, Virtual Camera 2D and Camera Follow 2D. Set Target to the player and press Play.

| Component | Purpose |
|---|---|
| Virtual Camera 2D | a pose with a priority, a zoom, bounds and a blend |
| Camera Follow 2D | moves its virtual camera after a target |
| Camera Shake 2D | tunes the shake of its virtual camera |
| Camera Zone 2D | lifts a camera while a target is inside an area |
| Camera Brain 2D | makes a Camera 2D show another output |

Cameras run during Play and in `aego run`. In edit mode the Scene View shows the camera frame, the deadzone, the bounds and the zones.

---
id: camera/virtual
try: try/VirtualCamera2D.aescene
try_select: camera.VirtualCamera2D
icon: comp.vcam
parent: camera
order: 1
title: Virtual cameras
---
# Virtual cameras

Each frame the brain picks one virtual camera per output:

1. The enabled virtual camera with the highest [Priority](ref:camera.VirtualCamera2D.Priority). Equal priorities pick the camera created first.
2. A [Camera Zone 2D](ref:camera.CameraZone2D) with the target inside lifts its camera to the zone's priority. On a tie the zone beats an ordinary camera, and the zone entered later beats an earlier one.
3. `camera.BlendTo` and `camera.Cut` override everything until `camera.Release`.

The pose comes from the virtual camera's node: its interpolated position and rotation, and [Zoom](ref:camera.VirtualCamera2D.Zoom), where 0 keeps the zoom of the Camera 2D. A Camera Follow 2D on the same node replaces the position. The node's transform is never changed; the camera only moves the view.

In Try it, the Director script raises **Close**, then **Tilt**, for two seconds each, and the view blends to them and back. Change Zoom, Blend Time or Blend Curve of Close in the Inspector and watch the next blend; the script is under the scene.

## Blends

When another camera becomes active, the view blends from the pose it shows now to the new camera over its [Blend Time](ref:camera.VirtualCamera2D.BlendTime), shaped by [Blend Curve](ref:camera.VirtualCamera2D.BlendCurve). A switch during a blend starts from the current view, never from the old camera. Zoom blends evenly on screen. Blend Time 0 cuts.

## Bounds

[Use Bounds](ref:camera.VirtualCamera2D.UseBounds) keeps the visible area inside [Bounds](ref:camera.VirtualCamera2D.Bounds), a world rectangle. When the area is smaller than the view on an axis, the view is centred on it there.

---
id: camera/outputs
try: try/CameraBrain2D.aescene
try_select: camera.CameraBrain2D
icon: comp.camera2d
parent: camera
order: 2
title: Outputs
---
# Outputs

Virtual cameras drive the main output unless [Output](ref:camera.VirtualCamera2D.Output) names another one. A Camera 2D shows the main output; add [Camera Brain 2D](ref:camera.CameraBrain2D) with the same Output to make it show another, such as a minimap. Each output has its own active camera, blends and shake. A Camera 2D whose output has no virtual cameras is left as it is.

In Try it, the minimap in the corner is a second Camera 2D with a small Viewport and Camera Brain 2D set to `map`; its own virtual camera follows the ball from far away.

---
id: camera/follow
try: try/CameraFollow2D.aescene
try_select: camera.CameraFollow2D
icon: comp.follow
parent: camera
order: 3
title: Follow
---
# Follow

[Camera Follow 2D](ref:camera.CameraFollow2D) moves its virtual camera after [Target](ref:camera.CameraFollow2D.Target).

| Field | Unit | Meaning |
|---|---|---|
| `Deadzone` | 0..1 of the view | area where the target moves freely; (0.4, 0.4, 0.2, 0.2) is a centred fifth |
| `Damping` | per second | how fast the camera catches up on each axis; 0 follows rigidly |
| `Lookahead` | seconds of velocity | how far ahead of a moving target to look |
| `MaxLookahead` | world units | largest lookahead; 0 is unlimited |
| `LookaheadSmoothing` | per second | how fast the lookahead changes |
| `SnapDistance` | world units | a longer jump of the target moves the camera at once |

![Camera Follow 2D selected: the yellow deadzone, the line to the target and the bounds](shots/camera-follow.png)

The camera follows the interpolated pose of a physics body, so it is smooth at any frame rate. Damping is exponential: a camera at 30, 60 and 144 frames per second ends up in the same place. When the target teleports, call `Snap()` on the component, or let Snap Distance catch it.

In Try it, the ball rolls down a long ramp. Set Damping to 0 for a rigid camera, or widen the Deadzone to let the ball run ahead.

---
id: camera/zones
try: try/CameraZone2D.aescene
try_select: camera.CameraZone2D
icon: comp.zone
parent: camera
order: 4
title: Zones
---
# Zones

[Camera Zone 2D](ref:camera.CameraZone2D) changes the camera while a target is inside its [Area](ref:camera.CameraZone2D.Area), a rectangle in the zone node's space. Inside, [Camera](ref:camera.CameraZone2D.Camera) gets the zone's [Priority](ref:camera.CameraZone2D.Priority), [Zoom](ref:camera.CameraZone2D.Zoom) and [Bounds](ref:camera.CameraZone2D.Bounds); outside, everything is back as before. The fields of the camera are never changed.

- Empty Camera uses the virtual camera on the zone node; give it a low priority so it shows only through the zone.
- Empty Target uses the follow target of the zone's camera.
- Zones use the exact position of the target, not the interpolated one, so the same input always enters a zone on the same frame.
- Overlapping zones: the higher priority wins, then the zone entered later. Leaving one zone does not affect the others.

In Try it, the purple area in the middle of the ramp is the zone: the view blends to a closer, fixed shot while the ball rolls through it, and back to the follow camera after.


![A Camera Zone 2D selected: its area with handles over the arena](shots/camera-zone.png)

---
id: camera/shake
try: try/CameraShake2D.aescene
try_select: camera.CameraShake2D
icon: comp.shake
parent: camera
order: 5
title: Shake
---
# Shake

In Try it, the crate drops every two seconds and its script calls `camera.Shake(0.9)` when it lands. Raise Max Angle or lower Exponent and Decay on the camera to feel the difference.

Shake is driven by trauma, a value from 0 to 1 per output. `camera.Shake(0.4)` adds trauma, which fades by [Decay](ref:camera.CameraShake2D.Decay) per second. The view moves by up to [Max Offset](ref:camera.CameraShake2D.MaxOffset) and turns by up to [Max Angle](ref:camera.CameraShake2D.MaxAngle), scaled by trauma to the power [Exponent](ref:camera.CameraShake2D.Exponent), so small hits stay subtle. Without a Camera Shake 2D on the active camera the defaults are used.

The motion comes from deterministic noise, so a replay shakes the same way, and it never touches the node's transform or the game's random numbers.

Trauma adds up and fades in real time. With the default Decay of 1.5 a hit of 0.6 is gone in 0.4 seconds:

```aes run
package demo

import (
    "aego/camera"
    "aego/log"
)

script Hits {
    var frames int

    func Ready() {
        camera.Shake(0.6)
        log.Info("hit, trauma " + string(camera.Trauma()))
    }

    func Update(dt float) {
        frames++
        if frames % 6 == 0 && frames <= 30 {
            log.Info("trauma " + string(camera.Trauma()))
        }
        if frames == 30 {
            camera.Shake(0.5)
            camera.Shake(0.8)
            log.Info("two hits, capped at " + string(camera.Trauma()))
        }
    }
}
```

## Accessibility

Settings → Accessibility → Screen Shake scales every shake; 0 turns it off. Reduce Motion turns shake off and removes lookahead. Trauma still fades, so gameplay that reads `camera.Trauma()` is not affected.

---
id: camera/time
icon: play.speed
parent: camera
order: 6
title: Time and replays
---
# Time and replays

[Time Domain](ref:camera.VirtualCamera2D.TimeDomain) decides how blends and shake of the active camera count time. `unscaled`, the default, uses real time: a hit-stop with time scale 0 does not freeze the camera. `game` follows the time scale and the pause.

Follow always uses game time: when the game is paused or stepped, a still target keeps the camera still.

Rewind and replay keep the active camera, overrides, zones and everything in the game domain. After a seek an unscaled blend is finished at once; an unscaled shake keeps its trauma.

---
id: camera/scripting
icon: file.script
parent: camera
order: 7
title: Scripting
---
# Scripting

    import "aego/camera"

    func OnCollisionEnter(other NodeRef, c physics.Contact) {
      if c.Normal.Y < -0.5 && c.NormalImpulse > 8 {
        camera.Shake(0.35)
      }
    }

    func Boss() coroutine {
      yield camera.BlendTo(BossCam, 1.5)
      yield waitSeconds(3)
      camera.Release(0)
    }

| Function | |
|---|---|
| `Active() NodeRef`, `ActiveOn(output) NodeRef` | the virtual camera that drives an output |
| `BlendTo(camera, seconds) Blend` | make a camera active over priorities and blend to it; 0 uses its Blend Time |
| `Cut(camera)` | the same without a blend |
| `Release(seconds) Blend` | hand the main output back to priorities |
| `Shake(trauma)`, `ShakeOn(output, trauma)` | add trauma |
| `Trauma() float` | trauma of the main output |
| `LogicalPose() Pose` | pose without shake and pixel snapping |
| `RenderPose() Pose` | pose the main output was drawn with |
| `ScreenToWorld(p)`, `WorldToScreen(p)` | conversions with the last drawn view, in output pixels |

`Blend` has `Done()`, `Running()`, `Stop()` and the signal `Completed(finished bool)`; `yield` waits for it. Component methods: `VirtualCamera2D.IsActive()`, `CameraFollow2D.Snap()` and `CameraShake2D.Add(trauma)`.

Each output keeps its own trauma:

```aes run
package demo

import (
    "aego/camera"
    "aego/log"
)

script Outputs {
    func Ready() {
        camera.Shake(0.3)
        camera.ShakeOn("map", 0.9)
        log.Info("main " + string(camera.Trauma()))
        log.Info("active camera is nil: " + string(camera.Active() == nil))
    }
}
```

---
id: camera/editor
icon: tool.move
parent: camera
order: 8
title: Editor tools
---
# Editor tools

- **Handles.** Select a virtual camera to drag its bounds, a zone to drag its area. A follow camera shows its frame, its deadzone and a line to its target. Every drag is one undo step.
- **Camera → Camera Frames** turns the frames on and off in the Scene View and in the Game View during Play, where the deadzone of the active camera, its bounds and the zones are drawn over the game.
- **Profiler → Camera** lists each output with its active virtual camera, zoom and blend progress.
- **Inspector during Play.** The virtual camera shows whether it is active, its priority, blend progress and trauma. **Look Through** moves the Scene View to its pose. Camera Follow 2D has **Snap to Target**.
- **Problems** lists follow and shake components without a virtual camera ([CAM001](ref:CAM001)), zones without a camera ([CAM002](ref:CAM002)), empty rectangles ([CAM003](ref:CAM003)) and follows without a target ([CAM004](ref:CAM004)).

---
id: camera/go
icon: code.brackets
parent: camera
order: 9
title: Go
---
# Go

    cams := sdk.CamerasOf(ctx)
    cams.Shake("", 0.5)
    if k, err := cams.BlendTo(bossCam, 1.5, false); err == nil {
      cams.OnBlendCompleted(k, func(finished bool) { ... })
    }

`Service` has `Active`, `BlendTo`, `Release`, `Shake`, `Trauma`, `Logical`, `Render`, `View` and `Snap`, all keyed by output name; `""` is the main output.

The module publishes the `camera2d.adjust` contract; `render2d.cameras` asks it to adjust the view of every Camera 2D. The system `camera.update` (PostUpdate, also while paused) picks cameras, advances blends and zones and fades trauma; follow and the final pose are computed when the view is built, once per output and frame. A module can scale the shake by providing `camera2d.accessibility` with the IDs of its settings.
