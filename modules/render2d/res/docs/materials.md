---
id: aego/material.Vec4.X
---
Stores the first floating-point lane of a numerical vec4 material value; it is not automatically a red colour channel.

---
id: aego/material.Vec4.Y
---
Stores the second floating-point lane of a numerical vec4 material value; its meaning is defined by the shader parameter.

---
id: aego/material.Vec4.Z
---
Stores the third floating-point lane of a numerical vec4 material value; its meaning is defined by the shader parameter.

---
id: aego/material.Vec4.W
---
Stores the fourth floating-point lane; it is not treated as alpha unless the consumer explicitly gives it that meaning.

---
id: engine.MaterialParameter
---
Stores one explicit, typed per-node material override. This is a data record inside MaterialOverride, not an addable component or a material asset.

---
id: engine.MaterialParameter.Name
---
Identifies the declared shader parameter by its exact name, not its position in the material layout.

---
id: engine.MaterialParameter.Kind
---
Preserves the parameter's declared type so removal or a later type change cannot silently reinterpret the saved value.

---
id: engine.MaterialParameter.Value
---
Stores floating-point, vector or colour lanes according to Kind. Integer and Boolean keep their values in separate fields.

---
id: engine.MaterialParameter.Integer
---
Stores an integer override as an exact signed 32-bit value instead of converting it through floating-point lanes.

---
id: engine.MaterialParameter.Boolean
---
Stores the explicit true/false value for a boolean parameter; the parameter's Kind determines whether this field is used.

---
id: render2d/materials
icon: comp.sprite
group: rendering
order: 3
title: Materials and shaders
try: MaterialOverrideGuideOriginal.aescene
---
# Materials and shaders

A material is a shared asset. Assign it to [SpriteRenderer](ref:engine.SpriteRenderer) or [Mesh2D](ref:engine.Mesh2D), then add [MaterialOverride](ref:engine.MaterialOverride) for values that belong to one node. The effective value is the shader default, then an explicit material value, then an explicit node override. Play and preview own separate node state.

## Create and edit

1. Choose **Shader**, **Material**, or **Material from Shader** in the searchable **Create** palette. The editable sources are [.aeshader](ref:formats/aeshader) and [.aemat](ref:formats/aemat).
2. Select the material in **Assets** and choose its shader, variant, blend, parameters and textures. Saving a shared material affects every consumer of that asset.
3. Assign it to the intended SpriteRenderer or Mesh2D. Save the material source and the scene containing that assignment separately.
4. Add **Material Override** when only this node should differ. Change a declared instance parameter, compare a second node using the same material, then use **Reset** to restore inheritance.

The material source and its Inspector share one undo history. Dragging one parameter produces one undo step. Reset restores inheritance even when an explicit value happened to equal the inherited value; it is not an assignment of zero.

Select a sprite, assign its Material field and add **Material Override**. Only numerical parameters marked `@instance` can be overridden. Textures, variants and blend belong to the material asset. Removed fields remain serialized as unknown values; unresolved references keep their AssetID.

## Portable shader

Author GLSL 330 with Aego metadata. The importer prepares and validates GLSL 330 and GLSL ES 3.00 sources. It resolves `engine/` includes and project includes below `assets/shaders/include/`. Public fields come from declarations, so an optimized-out parameter stays in the Inspector.

```glsl
#pragma aego abi 1
#pragma domain sprite
#include "engine/sprite.glsl"
#pragma material
uniform float Opacity; // @instance @default(1) @range(0, 1)
#pragma end
void fragment() {
    aego_out = aego_sample() * Opacity;
}
```

`aego_sample()` already returns linear, premultiplied color. Additional opacity scales the entire returned `vec4` once. Ordinary `vec4` values are numerical; only parameters marked `@color` use Color semantics. Sample a separate noise texture with `aego_local`, independent of the sprite's atlas location. `aego_sample_at` clamps to this sprite's region and returns transparent outside it.

Sprite and Mesh2D are separate shader domains. A material must match its consumer's vertex layout. Frame time belongs to the current world, View data to the current camera, and material values to their committed revision. A custom vertex stage disables CPU culling for its Sprite or Mesh2D consumer. The CPU does not analyze GLSL to guess vertex displacement bounds.

## Animate and reload

Tween and Animation address a declared instance parameter through a property path. The final segment is the parameter's name from the shader schema, for example:

```text
MaterialOverride.Flash
MaterialOverride.Threshold
```

Bindings refresh when a committed layout generation changes. Numeric integer animation keys retain signed integer precision; float tweens use float interpolation.

Save a shader or an include to import a candidate. Compile, link, layout validation and dependent material migration must all succeed before the next generation is committed at a frame boundary. An error retains the previous program, schema and values. The Problems panel opens the original source or include line; a column is omitted when the compiler did not provide one.

A failed current shader blocks strict export even when the scene still renders its last-good version. Save dirty shader/material/include buffers before exporting. Exports contain prepared shader and material artifacts; players do not need the importer or glslang.

## Demonstration

`examples/materials` contains the full project workflow: translucent A/B/A sprites sharing an atlas, adjacent instances with different overrides, a Mesh2D, two cameras, a Flash tween and a Dissolve animation. Its README gives the edit, undo, reload and native/web export steps. The Inspector preview uses a separate material and transparent offscreen target; a session without a GPU displays its reason instead of pretending a render succeeded.

---
id: aego/material
summary: Finds built-in material and shader references, assigns a material to a node, and represents numerical four-component values.
---

Use `material.Builtin("Flash")`, `Dissolve`, `Outline`, `Additive`, `PixelSnap`, `Unlit`, `Lit` or `LitMesh2D` (`Default` is an alias for Unlit). Unknown names fault. Lit and LitMesh2D evaluate each selected light against the receiver's own surface normal. `material.Assign` writes the current world's SpriteRenderer or Mesh2D reference. For custom assets, assign an exported `AssetRef<Material>` or set the component's Material property.

```aes run
package demo

import (
    "aego/material"
    "aego/log"
)

script SharedMaterial {
    func Ready() {
        ref := material.Builtin("Flash")
        sprite := node.Get<SpriteRenderer>()
        if sprite != nil {
            material.Assign(node, ref)
        }
        weights := material.Vec4{0.1, 0.2, 0.3, 0.4}
        log.Info("Flash reference available; numeric lane X=" + string(weights.X))
    }
}
```

For a node with MaterialOverride, use `SetFloat`, `SetInt`, `SetBool`, `SetVec2`, `SetVec3`, `SetVec4`, `SetColor`, their matching getters, and `Reset`. An ordinary vec4 uses `material.Vec4`; a color parameter uses linear `Color`. Type mismatches fail before changing the node.

---
id: engine.MaterialOverride
summary: Explicit, typed per-node material values that inherit from one shared material asset.
try: try/MaterialOverride.aescene
---

Use this component when two nodes share a material but need different numerical values. Its Inspector shows declared instance parameters and their inherited values. Save edit-mode overrides with the scene; changes in Play affect the play copy, not the shared `.aemat` source.

Setters take the shader parameter's exact `name`, such as `Flash`, not an AssetID or a post-processing instance key. The declared type must match the setter and the parameter must allow `@instance` overrides. Floating-point lanes must be finite, integers must fit int32, and a node supports at most eight active explicit parameters. A shader's Inspector range is not a promise that script setters clamp values into that range.

Getters read the effective value after shader defaults, material values and this node's overrides. Reset removes an explicit value instead of storing the current inherited value. Removed shader parameters remain in `values` for restoration, but are not submitted as active shader values. [PostProcess](ref:render2d.PostProcess) uses a different, effect-instance scope.

## One-node Flash example

Attach this script to a SpriteRenderer node with MaterialOverride, then start Play. It assigns the built-in Flash material, writes one override and resets it. The log shows the inherited value, the override, whether Reset removed a value, and the inherited value again. Without that node setup, the script reports the missing components instead of silently skipping the example.

```aes check
package demo

import (
    "aego/log"
    "aego/material"
)

script FlashProbe {
    func Ready() {
        sprite := node.Get<SpriteRenderer>()
        values := node.Get<MaterialOverride>()
        if sprite == nil || values == nil {
            log.Error("FlashProbe needs SpriteRenderer and MaterialOverride")
            return
        }
        material.Assign(node, material.Builtin("Flash"))
        inherited := values.GetFloat("Flash")
        values.SetFloat("Flash", 0.65)
        log.Info("inherited=" + string(inherited) + ", explicit=" + string(values.GetFloat("Flash")))
        removed := values.Reset("Flash")
        log.Info("reset=" + string(removed) + ", inherited=" + string(values.GetFloat("Flash")))
    }
}
```

```aescene check
scene version=1 {
  node "d3400000000000000000000000000001" name="Flash probe" enabled=true layers=1 {
    transform { position=[0, 0] rotation=0 scale=[1, 1] }
    component "engine.SpriteRenderer" version=1 {
      sprite=asset("ae900000000000000000000000000010") size=[2, 2] tint=#ffffffff
    }
    component "engine.MaterialOverride" version=1 { values=[] }
    component "engine.Script" version=1 { script "game.demo.FlashProbe" enabled=true {} }
  }
  node "d3400000000000000000000000000002" name="Camera" enabled=true layers=1 {
    transform { position=[0, 0] rotation=0 scale=[1, 1] }
    component "engine.Camera2D" version=1 { zoom=40 active=true }
  }
}
```

---
id: engine.MaterialOverride.SetFloat
params: [name, value]
---
`name` selects a float parameter marked `@instance`. `value` must be finite; the call writes only this node's override and does not save the scene or shared material.

---
id: engine.MaterialOverride.SetInt
params: [name, value]
---
`value` must fit signed int32, even though the script's `int` is wider. A range or type error leaves the old override unchanged.

---
id: engine.MaterialOverride.SetBool
params: [name, value]
---
`name` selects a bool parameter marked `@instance`. This is a typed boolean assignment, not an integer or float conversion.

---
id: engine.MaterialOverride.SetVec2
params: [name, value]
---
`value` supplies two finite components for a declared instance vec2. A different vector size is a type error.

---
id: engine.MaterialOverride.SetVec3
params: [name, value]
---
`value` supplies three finite components for a declared instance vec3. A different vector size is a type error.

---
id: engine.MaterialOverride.SetVec4
params: [name, value]
---
`value` is a numerical [material.Vec4](ref:aego/material.Vec4). Use SetColor instead when the shader declares a colour parameter; these types are not interchangeable.

---
id: engine.MaterialOverride.SetColor
params: [name, value]
---
`value` is a linear Color for a vec4 parameter with the colour hint and `@instance`. The call does not interpret numerical Vec4 values as colours.

---
id: engine.MaterialOverride.GetFloat
params: [name]
---
`name` selects a declared float parameter. The result includes inheritance when this node has no explicit value; an unresolved material or incompatible type faults the script.

---
id: engine.MaterialOverride.GetInt
params: [name]
---
Returns the effective signed int32 value as a script `int`, without a floating-point round trip. The declared parameter must have integer type.

---
id: engine.MaterialOverride.GetBool
params: [name]
---
Returns the effective typed boolean. This does not treat a numerical zero or nonzero shader parameter as a bool.

---
id: engine.MaterialOverride.GetVec2
params: [name]
---
Returns the two effective components of a declared vec2. The getter does not truncate a larger vector.

---
id: engine.MaterialOverride.GetVec3
params: [name]
---
Returns the three effective components of a declared vec3. The getter does not truncate a larger vector.

---
id: engine.MaterialOverride.GetVec4
params: [name]
---
Returns a numerical [material.Vec4](ref:aego/material.Vec4). A parameter with the colour hint requires GetColor instead.

---
id: engine.MaterialOverride.GetVec4Lane
params: [name, lane]
---
`lane` is 0 for X, 1 for Y, 2 for Z or 3 for W of an ordinary vec4. An out-of-range lane or colour parameter faults the script.

---
id: engine.MaterialOverride.GetColor
params: [name]
---
Returns the effective linear Color of a parameter declared with the colour hint. This is not a colour conversion for an ordinary vec4.

---
id: engine.MaterialOverride.Reset
params: [name]
---
Removes this node's explicit value for `name`, including a retained value for a removed shader parameter. Returns false when no explicit value was stored; it does not zero the shared material.

---
id: engine.Light2D
summary: Illuminates Lit sprites and 2D meshes with a point, spot, directional or textured light.
try: try/Light2D.aescene
try_select: engine.Light2D
---

[Light2D](ref:engine.Light2D) affects [SpriteRenderer](ref:engine.SpriteRenderer) with `Lit` and [Mesh2D](ref:engine.Mesh2D) with `LitMesh2D`. Unlit materials keep their original colour. Add a light from the Create palette, set its intensity and range in the Inspector, then drag its range and direction handles in Scene View. A handle gesture creates one Undo command.

Point lights illuminate a circle. Spot lights face local +X; `innerAngle` and `outerAngle` are full cone widths in radians, so an outer angle of π/2 spans 90°. Sprite lights apply the `cookie` texture as a mask using that texture's own region. Directional lights use node rotation for azimuth and `elevation` for the component toward the surface. Directional shadows are unavailable; enabling them produces a diagnostic.

`height` gives a local light a direction toward the receiver's normal. With a flat normal and a light exactly in the sprite plane, the diffuse dot product can be zero. Height changes shading, while occluders remain planar. Range uses world units and does not scale with the light node.

`layers` selects receiver visibility layers. `shadowCasterMask` separately selects occluder layers; neither replaces collision filtering. Eight lights can contribute to one receiver set. Priority, estimated influence and stable ID decide which lights remain when a set exceeds that budget. Scene View highlights excluded source markers and the Profiler counts exclusions and lighting batch splits. Transparent Sprite/Mesh2D order is preserved.

The Try scene places a warm light beside a cool spotlight. Change the warm light's intensity to compare its contribution, then use Reset to restore it. The lights remain still so the effect of each edit is easy to see.

---
id: engine.LightOccluder2D
summary: Defines a polygon or open chain that casts local-light shadows without requiring Physics.
try: try/LightOccluder2D.aescene
try_select: engine.LightOccluder2D
---

[LightOccluder2D](ref:engine.LightOccluder2D) describes shadow geometry independently of the visible sprite. `Polygon` uses local points; enable `closed` for a simple polygon without holes, or leave it off for a chain. The validator rejects non-finite points, zero-length edges and self-intersections. A sprite does not become an occluder automatically.

In Scene View, drag a vertex to move it, drag an edge midpoint to insert a vertex, or Alt-click a vertex to remove it. Each gesture uses one History transaction. `ShapeProvider` instead copies local geometry from a provider such as Physics, identified by `shapeNode` and a stable shape key. Physics is optional: the Try scene uses only a manual polygon. Changing collision masks does not change `layers` here.

Point, Spot and Sprite lights share the planar shadow algorithm. Overlapping volumes form a union. A light inside a closed contour is blocked by that contour; a receiver does not automatically ignore its own contour. `cullMode` chooses the accepted edge winding independently of a physics chain's one-sided setting. Softness filters each mask within a bounded radius and does not simulate an area light.

Offscreen casters remain eligible when their shadows reach a visible receiver. Light and occluder geometry use presentation transforms, including interpolated motion. Static light-space masks are reused while only the camera moves; moving a light or caster, changing its contour, resolution or filtering invalidates the relevant mask. The Inspector shows active mask layers with the owning light, and Scene View's shadow display shows visibility on Lit surfaces.

---
id: engine.LightingEnvironment2D
summary: Controls ambient illumination, output colour mode and shadow budgets for a camera output.
try: try/LightingEnvironment2D.aescene
try_select: engine.LightingEnvironment2D
---

[LightingEnvironment2D](ref:engine.LightingEnvironment2D) supplies ambient colour and intensity, exposure, `Auto`/`LDR`/`HDR`, mask resolution and the local-shadow budget. Set `camera` to target one Camera2D, or leave it empty to apply to the world's 2D outputs. Several matching environments produce a diagnostic; their order never chooses a winner. Without an environment, white ambient with intensity one preserves compatible Lit defaults.

LDR uses a linear RGBA8 scene target. HDR uses a verified renderable RGBA16F target and a display resolve. Auto chooses HDR when the output needs lighting and the device supports it. If the actual framebuffer cannot be used, the Inspector reports the LDR fallback reason. The UI and editor overlays stay outside scene tone mapping. A camera with an active Bloom pass filters the composed scene before this display transform.

Scene View has its own output, not the identity of a game camera. Select **Preview in Scene View** in an environment's Inspector to preview that environment explicitly. Changes to its fields become visible immediately. **Use scene environment defaults** clears the preview selection. The selection is per document and does not alter camera references, prefabs, Play state or Undo history. Deleting or disabling the selected environment preserves its preview ID and reports a diagnostic until it is restored or the selection is reset.

The Scene View toolbar can bypass lighting or display transformed normals and shadow visibility. These controls affect that view only. For a transparent HDR output, resolve preserves the premultiplied colour contract. Additive emission must first be composed into an opaque backdrop, or retained in a linear RGBA16F intermediate, before a display image is produced. Additive RGB preserves destination alpha.

---
id: aego/lighting
summary: Changes lighting and diagnostic display for the current world's presentation view.
---

Use `SetEnabled` to compare the lit image with the original albedo. `SetDebug` accepts `lighting`, `normals` or `shadows`; invalid names fault before changing the view. `Enabled` and `Debug` read the current view state. These calls do not edit material assets, scene components or gameplay state.

```aes check
package demo

import (
    "aego/lighting"
    "aego/log"
)

script InspectLighting {
    func Ready() {
        lighting.SetEnabled(true)
        lighting.SetDebug("normals")
        log.Info("Lighting display: " + lighting.Debug())
    }
}
```

Light components use the regular typed API and animation property paths. For example, `Light2D.intensity`, `Light2D.range`, `Light2D.color` and the cone angles can be animated with Tween or Animation. In the MaterialOverride track, select the declared Lit parameter by name: `NormalStrength`, `EmissionIntensity` or `ReceiveShadows`. A normal-map or cookie reference is a texture resource, not a numerical instance override.

### A lamp in the platformer

Open `examples/platformer`, whose main level contains the canonical lighting demonstration. Its lamp sprites use emission for their own appearance and separate Light2D components to illuminate nearby surfaces. Platforms and walls supply manual shadow contours. HUD and menus remain Unlit. The project includes actual albedo, normal and height source assets with `.meta` identities.

Change a lamp's colour, height or range, then use Undo/Redo, Play and the registered lighting debug actions. Bloom on the main camera spreads bright lamp and spark pixels into a halo. The lamp’s Light2D illuminates neighbouring surfaces. Export this project for native or web using the normal strict export workflow. The website demo is built from the same platformer export, with no independently edited copy of its level.

---
id: render2d/normal-mapping
icon: comp.sprite
group: rendering
order: 4
title: Normal mapping with a moving light
try: try/lighting/NormalMapping.aescene
try_select: engine.Light2D
---
# Normal mapping with a moving light

The left sprite uses a normal map generated from a height image; the right sprite uses the same albedo and a flat normal. The moving Point light evaluates each surface separately. Change Height, Intensity or Range in the live Inspector to see the difference. Height is measured above the 2D plane, while shadows remain planar.

The complete source project is `modules/render2d/res/docs/try/lighting/NormalMapping.assets`. Open its `project.aego` in the editor to edit the images, material and moving-light script together. A copied `.aescene` alone does not include those external assets. The browser receives prepared resources from that project through the ordinary asset loader. It does not run image importers or search for normal-map filenames.

`surface_height.png.meta` selects `semantic=height`, `generate_normal=true`, `height_channel=r`, `height_strength=12` and `normal_y=up`. Import produces a linear normal texture. The Lit material's explicit `NormalMap` reference points to that stable asset ID. The albedo uses color semantics and has no implicit normal association, so the flat comparison remains flat. Renaming either file with its `.meta` preserves the link.

For an authored normal image, use `semantic=normal`; choose `normal_y=up` or `down` to match the source's green-channel convention. Do not import normal, cookie or height data as sRGB color. An independently packed normal atlas keeps its own region, source dimensions and trim mapping. Rotated atlas entries remain unsupported. A missing normal frame keeps its reference and renders with a flat fallback until repaired.

If the surface looks flat, check that the material is Lit, the light's receiver mask includes the sprite, Height is positive, the normal reference resolves and NormalStrength is nonzero. Excessive ambient can hide the direct-light response. Scene View's **Surface normals** mode helps inspect the transformed normal without exposure or tone mapping. The project `examples/platformer` demonstrates the same pipeline in the actual game.

---
code: LGT001
severity: error
title: Invalid light parameters
example: try/diagnostics/LGT001.aescene
---

A light is excluded because its type, intensity, range, height or angle is invalid.

**Fix.** Choose a supported type, finite nonnegative intensity and height, positive local range, full Spot angles satisfying `0 <= InnerAngle <= OuterAngle <= pi`, and Directional elevation in `(0, pi/2]`.

---
code: LGT002
severity: error
title: Directional shadows are unsupported
example: try/diagnostics/LGT002.aescene
---

Directional lights illuminate surfaces but cannot cast local shadow masks.

**Fix.** Disable Shadows on the Directional light or use a Point, Spot or Sprite light for local shadows.

---
code: LGT003
severity: error
title: Invalid or conflicting lighting environment
example: try/diagnostics/LGT003.aescene
---

The output has conflicting environments, invalid settings or an unresolved camera or preview selection.

**Fix.** Select one environment per output with a valid camera, nonnegative ambient, positive exposure, power-of-two shadow resolution from 64 to 2048 and a shadow budget of 0 to 16, or reset a stale Scene View selection.

---
code: LGT004
severity: error
title: Invalid occluder contour
example: try/diagnostics/LGT004.aescene
---

The shadow contour has unsupported geometry, an invalid source or an unknown culling mode.

**Fix.** Choose a supported source and winding, then use handles to make a simple polygon of 3–256 finite vertices or a nonintersecting open chain with no zero-length edges.

---
code: LGT005
severity: warning
title: Missing shadow shape provider or shape
example: try/diagnostics/LGT005.aescene
---

The occluder cannot resolve the owner, geometry provider or shape key of its saved reference.

**Fix.** Restore the owner or provider, select an available shape or draw a manual polygon that works without Physics.

---
code: LGT006
severity: warning
title: Missing or incompatible surface texture
example: try/diagnostics/LGT006.aescene
---

A lighting texture reference is missing or resolves to a texture with incompatible semantics.

**Fix.** Restore the saved AssetID or assign a compatible linear normal or mask, checking the source dimensions and trim of paired frames.

---
code: LGT007
severity: warning
title: Invalid lighting presentation transform
example: try/diagnostics/LGT007.aescene
---

A non-finite or degenerate presentation transform prevents the light or occluder from using its intended orientation.

**Fix.** Check the node and its parents for zero scales or non-finite coordinates in both authored and interpolated poses.

---
code: LGT008
severity: error
title: Lighting geometry or resource budget exceeded
example: try/diagnostics/LGT008.aescene
---

A lighting input exceeds a resource budget, or the requested target cannot represent the lighting output.

**Fix.** Read the reported limit or target reason, then simplify the contours, adjust the light budget, or compose additive energy over an opaque backdrop before display resolve.

---
id: render2d/image-viewer
icon: file.texture
group: rendering
order: 5
title: Image and Sprite authoring
---
# Image and Sprite authoring

Double-click a PNG in Assets, open a sprite frame from an Inspector, or choose **Open Image / Sprite**. Each source image opens in an ordinary workspace tab. A frame uses its source image's document and metadata history: opening a second frame changes the selection in that same tab. PNG is the current texture importer's supported source format. Derived atlas pages and built-in textures have a read-only view; unsupported binary asset kinds receive asset information instead of being passed to the code editor.

## Inspect pixels and atlas packing

**Source pixels** shows the original file or selected source frame. **Imported pixels** shows the committed runtime artifact, including its format and linear/premultiplied processing. Imported frames are displayed in their original logical canvas, with transparent trim padding restored. **Open Atlas** opens the packed page and highlights the selected region separately. Source rectangles and frame IDs never become atlas coordinates.

Use the wheel to zoom at the cursor and drag with the middle or right mouse button to pan. **Fit** fills the available canvas; **1:1** maps one image texel to one physical display pixel, accounting for the editor's display scale. The pixel grid appears at sufficient magnification. RGB and individual R/G/B/Alpha modes expose raw channel bytes, including RGB stored under zero alpha; they do not modify the source or importer settings. The readout gives the logical pixel coordinate and four RGBA8 values from the selected representation. RGBA uses the image's coverage over a checker, light or dark background.

The source/frame pivot shown in the panel is normalized in the original logical canvas. It is authoring alignment information. The preview has its own center origin, and existing nodes keep their `SpriteRenderer.Origin`. For a sheet, select frames by their persistent IDs; a rename or ordering change does not create new channel identities. This viewer does not paint pixels or create animation frames.

## Roles and surface channels

The metadata document is the common model for the viewer, Inspector and structured CLI/MCP mutations. Changes stay dirty until **Save settings**. A numeric drag creates one undo step. A clean document follows external metadata changes; a dirty document presents a conflict. Saving cannot overwrite an unacknowledged external change.

| Source role | Import meaning |
|---|---|
| `color` | Color RGB, configurable sRGB/linear encoding, linear alpha, premultiplication in linear space |
| `normal` | Linear data, no premultiplication; explicit `normal_y=up/down` convention |
| `height` | Linear data; chosen height channel, strength and Y convention can generate a normal artifact during import |
| `emission-rgb` | Straight RGB with explicit color-space semantics; texture alpha does not multiply the emission |
| `emission-mask` | Scalar data from the chosen channel, used with the material's emission color/intensity |
| `cookie-mask` / `mask` | Linear scalar mask from the chosen channel |
| `data` | Unmodified linear data channels |

A color source exposes **Normal**, **Height source** and **Emission** associations. Select a compatible source/frame or drag it from Assets. Compatibility checks include role, logical dimensions and pivot; independently packed maps keep their own UV/trim mapping. **Disabled explicitly** and **Inherit / discover** are different states. A missing explicit AssetID remains visible and can be repaired or removed; it never silently points at a different file.

A whole-sheet association is used by the importer to establish persistent frame-to-frame bindings. It is not a fallback that stretches an entire normal sheet across a single sprite. Missing frame mappings use the documented flat-normal/uniform-emission fallback. Automatic `_n`, `_e` or `_em` discovery resolves to IDs during import, and explicit mappings take priority.

Height settings belong to the height source and are shared by all of its consumers. Open that linked source to change `height_channel`, `height_strength` or `normal_y`. An explicit Normal association wins over Height. A raw Height reference with generation disabled retains its ID and reports why it cannot yet supply a normal.

## Preview, Save and Undo

The **Lit surface** uses the prepared built-in Lit shader, the normal material cache and the actual renderer in a separate preview session. Drag within the surface to move the light. Height, color, intensity, ambient, Lit/Unlit, Normal/Emission and Flip X/Y controls affect that preview only. They do not call gameplay scripts, change the scene, create prefab overrides or save import settings. Normal maps transform correctly when flipped. Emission changes the surface's own light output and does not illuminate neighbours or produce Bloom. Without a GPU the ordinary image/metadata view remains available and the Lit preview explains the missing device.

For a height source, enable **Generate normal at import** and change its parameters. A private candidate is prepared from snapshots of the source image and dirty metadata. **Generated normal candidate** shows its raw pixels; the Lit surface uses the same prepared result. One request runs at a time and later edits replace the queued request. Failed imports or failed GPU uploads keep the previous valid preview. A changed source file, stale metadata, context generation or closed tab cannot publish an obsolete candidate.

**Save settings** validates the current height candidate before saving the metadata and applying the normal import/reload transaction. The PNG's pixels stay unchanged. After an explicit Save, **Undo/Redo** also prepares and atomically applies a working artifact generation to the running scene. The metadata buffer remains dirty and the disk version is unchanged until another explicit Save. A failed working import preserves the last-good runtime generation. Closing and discarding a working document restores saved resources. Strict editor export blocks dirty metadata, so an Undo preview cannot silently export the older disk version.

In `examples/platformer`, open `assets/surfaces/platform.png`, follow its Height source to `platform_height.png`, change strength, inspect the generated normal and Lit surface, then Save and Undo to compare the platform in the actual level. Open `runner.png`, select an existing frame and toggle Flip X/Y in the preview to inspect its linked normal frame. The lamp demonstrates a real emission-mask asset. These are the same assets included by native/web exports and the site's platformer demo.

## Diagnostics

| Code | Meaning and repair |
|---|---|
| `TEX001` | A setting, role, frame rectangle, dimension or channel pivot is invalid. Correct the typed value or assign a matching source/frame. |
| `TEX002` | An explicit channel AssetID cannot be resolved. Restore the asset, choose another compatible ID or disable/reset the association. The original ID is preserved. |
| `TEX003` | Candidate preparation or import failed. Read the accompanying importer diagnostic; the previous working generation remains active. |

The Channels Inspector, `texture.get/schema/set/channel/channel-reset/reset/validate/apply` commands and structured mutations use the same schema and validation. Asset dependencies and consumers are navigable from the image panel; imported artifact bytes are never an editable document.
