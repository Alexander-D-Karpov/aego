---
id: render2d/cameras
icon: comp.camera2d
group: rendering
order: 2
title: Cameras
---
# Cameras

[Camera2D](ref:engine.Camera2D) renders an orthographic view from the node's 2D transform. Only cameras with [Active](ref:engine.Camera2D.Active) on render.

## Zoom and scaling

[Zoom](ref:engine.Camera2D.Zoom) is pixels per world unit; the default is 64. Larger values show a smaller part of the world.

[Scale](ref:engine.Camera2D.Scale) decides how the view follows the window size:

| Mode | Behaviour |
|---|---|
| free | the visible area grows with the window |
| pixelPerfect | keeps whole-pixel scaling of the [Virtual](ref:engine.Camera2D.Virtual) resolution |
| fitVirtual | fits the [Virtual](ref:engine.Camera2D.Virtual) resolution into the window |

## Several cameras

[Viewport](ref:engine.Camera2D.Viewport) is the part of the screen the camera draws into, in normalized coordinates; (0, 0, 1, 1) is the whole screen. Cameras render by [Order](ref:engine.Camera2D.Order), lower first. [Mask](ref:engine.Camera2D.Mask) selects visibility layers, -1 renders all of them. A camera drawn over another one usually turns [Clear](ref:engine.Camera2D.Clear) off; otherwise it fills its viewport with [ClearColor](ref:engine.Camera2D.ClearColor) first.

## Virtual cameras

A [Virtual Camera 2D](ref:camera.VirtualCamera2D) can drive the view of a Camera 2D: follow a target, blend between shots, shake and react to zones. See [Cameras 2D](ref:camera).
