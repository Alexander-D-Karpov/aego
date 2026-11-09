---
id: render2d.PostProcess
search_names: [bloom, glow, camera effects, post-processing]
icon: comp.camera2d
group: rendering
order: 6
title: Camera post-processing
summary: Add Bloom and camera colour effects; inspect ordered instances, typed parameters and shader-domain errors.
try: try/postfx/CameraOutput.aescene
try_select: render2d.PostProcess
---
# Camera post-processing

Run the example, focus Game View and press Space to cycle through Bloom, Color Grading, Vignette, Chromatic Aberration, Pixelate, CRT, a custom colour filter and the original image. The example includes a prepared linear identity LUT and its own material. Saturation makes grading visible even with the identity LUT.

## Minimal camera setup

Add Post Process (`render2d.PostProcess`) to the same node as [Camera2D](ref:engine.Camera2D). Enable the component, add one Bloom effect and enable that instance. Start with an opaque camera output and a bright sprite. Lower Threshold until its bright area receives a halo, then adjust Intensity and Radius. Disable the instance to compare. Bloom spreads bright pixels in the rendered image; use [Light2D](ref:engine.Light2D) to illuminate neighbouring surfaces. It does not create a light by itself.

Only one enabled stack may claim a logical camera output. If nothing changes, check the camera's output, component and instance Enabled flags, threshold, and [PFX001](ref:PFX001). A Custom material must use a postfx-domain shader; read [MAT004](ref:MAT004) before changing the shader's purpose.

## Add a glow to the platformer

Open the main scene of examples/platformer. Select Camera and expand Post Process. Its Bloom instance uses Threshold 0.75, Intensity 0.22 and Radius 6. Lower Threshold to pick up more bright surfaces. Increase Radius to spread the light over more pixels. Compare the lamp and its sparks with the stack disabled. Open Settings in the game to check that its text remains sharp. The lamp uses its Light2D for neighbouring surfaces and Bloom for the halo on screen.

The stack runs once after the cameras sharing a logical output finish drawing the world. Enable the component on one contributing camera. Scene-linear effects run before exposure and tone mapping. Display-linear effects run after that conversion and before final encoding. Reorder effects within the compatible phase. A linear offscreen output keeps its scene-linear values. Bloom and colour separation require opaque coverage in this renderer.

## Reuse a colour filter

Add Custom Material and choose a material backed by a shader with `#pragma postfx`. The editable example is `examples/platformer/assets/postfx/cold.aemat`. Enable the effect after assigning the material. It reads the previous effect through `aego_sample(aego_uv)`. Save the material, shader and scene and open a standalone export to verify the result. The camera also carries a disabled Color Grading entry with a LUT. LUTs are standalone linear data strips, 256×16 or 1024×32.

Animation and Tween address a numeric field by instance ID, for example `render2d.PostProcess.ef120d00000000000000000000000002:Intensity` on the platformer's camera. Reordering an effect preserves that target. Duplicating creates an independent target. Undo restores a removed instance and its ID.

## Materials in the demo games

Open `examples/platformer/assets/materials/character_flash_lit.aeshader` to see how a surface combines forward lighting, the current sprite normal and a per-instance Flash parameter. The Hurt clip animates the Flash parameter of the player’s material override. Other sprites sharing the material retain their own overrides. For an outline on selection, assign the built-in Outline material to the selected sprite, retain its previous material and restore that material on deselection. In examples/topdown the player's Torch is a Spot light whose orientation follows the same aim as the sword. The floor and walls receive its light.

## Inspect a pass or edit a camera from a tool

In the Lighting profiler, open **Inspect output passes** and select the output. With Bloom enabled, locate bright extraction, both blur directions and composite. Disable the stack and inspect the next frame. A timing entry appears when the driver returns a valid sample. A window target reports its drawing size, while a texture target also exposes its storage format.

Use `aego postfx get scenes/main.aescene Camera` to inspect the instance IDs. In the platformer, `aego postfx set scenes/main.aescene Camera ef120d00000000000000000000000002 Intensity 0.35` changes the existing Bloom. The same commands are available as MCP tools. In a bound editor session a write uses the active document and its Undo history.

---
id: render2d.PostProcess.SetEffectEnabled
params: [instanceID, enabled]
---

`instanceID` is the stable ID of an existing effect, not its type or position in the stack. A missing instance reports a script error before changing it. This changes the instance switch; the component must also be enabled.

---
id: render2d.PostProcess.SetFloat
params: [key, value]
---

`key` is `instanceID:Field`, for example the Bloom instance ID followed by `:Intensity`. `value` must match the descriptor's floating-point field and constraints. An unresolved instance, missing field or incompatible value reports a script error without applying that assignment. This edits the instance, not a shared material.

---
id: render2d.PostProcess.SetInt
params: [key, value]
---

`key` is `instanceID:Field`. The integer must fit signed int32 and match the field descriptor; an out-of-range integer or unresolved target reports a script error before assignment.

---
id: render2d.PostProcess.SetBool
params: [key, value]
---

`key` is `instanceID:Field`; the target must be a boolean parameter. Unknown targets or incompatible field kinds report a script error before assignment.

---
id: render2d.PostProcess.SetVec2
params: [key, value]
---

`key` is `instanceID:Field`; `value` supplies two lanes. The field descriptor must accept that value. An unresolved target or type mismatch reports a script error before assignment.

---
id: render2d.PostProcess.SetVec3
params: [key, value]
---

`key` is `instanceID:Field`; `value` supplies three lanes. The field descriptor must accept that value. An unresolved target or type mismatch reports a script error before assignment.

---
id: render2d.PostProcess.SetVec4
params: [key, value]
---

`key` is `instanceID:Field`; `value` is [material.Vec4](ref:aego/material.Vec4), with X, Y, Z and W lanes. The field descriptor must accept that value. An unresolved target or type mismatch reports a script error before assignment.
