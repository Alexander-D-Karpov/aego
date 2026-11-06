---
id: platformer-effects
icon: doc.tutorial
group: start
title: Platformer lighting and effects
order: 4
---
# Platformer lighting and effects

This guide explores the finished `examples/platformer` project. To build the basic game from scratch, start with [Platformer from scratch](ref:platformer).

## Lighting and surface assets

Open `examples/platformer` to inspect the complete lighting example. The level, collisions and controls remain the gameplay example; surface assets and lights make normal mapping, shadows and emission visible in that same level. F6 toggles lighting for the view. F7 cycles the lighting, normal and shadow views. Both keys are registered Input Actions and do not change materials or simulation state.

| Existing asset | Surface association |
|---|---|
| `assets/surfaces/platform.png` | Its explicit Height source is `platform_height.png`, with import-time normal generation enabled. This generated normal is shared by the level's platforms. |
| `ground.png` and `wall.png` | Explicit normal maps take priority; their height sources remain linked for authoring. |
| `runner.png` | Each of the eight original animation frames has its own normal-frame association in `runner_n.png`. The source silhouette, frame size and gameplay origin are preserved. |
| `lamp.png` | `lamp_em.png` is an emission mask. It brightens the lamp's core using `lamp_lit.aemat`; the nearby Point light illuminates other objects. The mask does not create Bloom. |
| `window_cookie.png` | A linear cookie mask shapes the Sprite light without using the albedo atlas coordinates. |

To edit the platform's surface, open `platform.png` from Assets and follow **Height source** to `platform_height.png`. Enable **Lit surface** and change the height strength in that shared height document. The generated-normal candidate and movable preview light show the result before Save. Press Play, keep the image tab open, and use **Save settings**: the running level receives the new prepared normal generation. Undo restores the earlier appearance while leaving the metadata document dirty and the saved file unchanged; Redo restores the saved result. Stop returns to the unchanged EditScene. An explicit Normal assignment overrides the Height-generated normal.

Open `runner.png`, select an existing frame and use the preview's Flip X/Y controls to check the paired normal. These controls affect only the preview; the player script sets FlipX when changing direction. Source pixels are edited in an external image tool, while channel links and import settings use the shared asset document and its Undo history.

The player and enemy use separate `LightOccluder2D` polygons for their shadows. These approximate the head and legs of the shared runner sprite without changing the player's capsule or the enemy's box collider. The outlines are static: they do not follow individual animation frames.

See [Image and Sprite authoring](ref:render2d/image-viewer) and [Materials and shaders](ref:render2d/materials) for role and color-space rules, missing-reference diagnostics and the lighting model. Native and web exports use this same project and include its normal, height-derived, emission and cookie artifacts. Preparing the site's demo does not publish it.

## Dust and sparks

Jump and keep moving after landing. Dust starts at the player's feet and stays where it was emitted. Select `Player/Dust` to change `rectSize` and `lifetime`. In `scripts/player/player.aes`, `OnLanded` emits eight particles, `OnJumped` five and `OnClimbed` six. `playOnStart` is off and `rate` is zero, so these events trigger the bursts. `Player/Slash/Sparks` emits on the attack animation's Hit event.

`space=world` fixes each particle's birth transform. Use `space=local` for an effect that should travel with its parent. To compare the level with and without particles, change **Particle Effects** in the game's accessibility settings. Leave F6 and F7 unchanged: they control lighting.

At `StartLantern`, select the `Smoke` or `Embers` child to inspect the two continuous emitters. Use the Inspector preview to pause the effect, lock its seed and compare changes before Play. Under **Over Life**, drag a gradient key, release it and use Undo to restore the previous color transition. See [CPU particle effects](ref:particles) for presets and the runnable example.

## Camera post-processing

Open the platformer in Game View and select **Camera → Post Process**. Bloom is enabled with Threshold 0.75, Intensity 0.22 and Radius 6. Watch the halo around the lantern while changing Intensity. Vignette, Color Grading and the Cold custom material are in the stack but start disabled. Enable one to compare its effect. Open Settings during Play and compare the opaque menu with the component on and off. To restore the original world image, disable Post Process. F6 and F7 control lighting and diagnostics.

Center is relative to the camera output; (0.5, 0.5) is its centre. Intensity controls darkening, while Softness controls the transition width. The boundary is circular in physical viewport pixels even on a wide display. See [post-processing](ref:render2d.PostProcess) for a runnable example.
