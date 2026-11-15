---
id: camera.CameraBrain2D
summary: Select which virtual-camera output drives a Camera2D; use a separate map output for a minimap.
---

## Choose the camera output

An output is a named group of virtual cameras, each with its own active camera, blend and shake. CameraBrain2D selects which group drives the [Camera2D](ref:engine.Camera2D) on the same node; it is not a follow behaviour and does not render a minimap by itself. The empty Output selects the main group.

1. Add CameraBrain2D to the Camera2D that should show a second view.
2. Give a [VirtualCamera2D](ref:camera.VirtualCamera2D) Output `map` and a zoom suited to an overview.
3. Set the selector's Output to `map`. Set that Camera2D's Viewport for an inset and its Order after the main camera.

Check: the inset uses the map camera's pose and zoom while the main view keeps its own virtual camera. If the named output has no virtual cameras, the Camera2D keeps its authored view; a misspelled name does not create a new virtual camera.

## Experiment and reset

In Try it, open the example parameters: the selected CameraBrain2D starts with Output `map`. Clear Output to select the main group. Compare the inset's zoom, then press Reset: Output returns to `map` and the original scene state restarts. Auxiliary scenery is in Scene, not part of the selector settings. See [virtual-camera outputs](ref:camera/outputs) for the complete setup.
