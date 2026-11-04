# Material and shader authoring contract

Materials combine a declared shader schema, shared values and texture references. Use this reference when writing `.aeshader` files, integrating material tools, or diagnosing differences between native and browser rendering. The editor workflow is described in the [Materials guide](../modules/render2d/res/docs/materials.md); the [demonstration project](../examples/materials/README.md) exercises creation, overrides, animation, reload and export.

This document describes the implementation in this repository. A successful parser or reference-compiler check does not establish that a graphics driver linked the shader. Test reports must identify which validation levels actually ran.

## A portable sprite material

Create a Shader asset with this source, then choose **Material from Shader**. Assign the resulting material to a SpriteRenderer. Add Material Override to the node when its opacity should be independent of other nodes using the asset.

```glsl
#pragma aego abi 1
#pragma domain sprite
#include "engine/sprite.glsl"

#pragma material
uniform float Opacity; // @instance @default(1) @range(0, 1)
#pragma end

void fragment() {
    aego_out = aego_sample() * clamp(Opacity, 0.0, 1.0);
}
```

`aego_sample()` returns linear, premultiplied color including the sprite tint. Multiplying the complete result by opacity changes RGB and alpha together. Multiplying its RGB by its existing alpha again darkens translucent edges.

The importer supplies `main()`, version declarations, engine inputs and uniform blocks. A custom `void vertex()` may modify `aego_world` before the generated vertex stage computes its final clip position. Do not supply another `main()`, engine block, or version declaration.

## Domains and the GLSL profile

| Domain | Geometry contract |
|---|---|
| `sprite` | Engine instanced quad, fragment entry, optional vertex entry |
| `mesh2d` | Engine mesh vertex layout: position, normal, UV and color; fragment entry, optional vertex entry |
| `postfx` | Reserved; rejected because this domain has no renderer here |

A Sprite shader cannot be assigned to Mesh2D as a universal material. Domain validation reports the incompatibility before binding the material. The built-in Lit shader uses the Sprite domain, and LitMesh2D supplies the corresponding Mesh2D domain. Other Sprite effects require a separate compatible Mesh2D shader when used on a mesh.

Authoring syntax is the shared GLSL 330 / GLSL ES 3.00 surface with Aego directives. The importer prepares separate vertex and fragment sources for both dialects. ES sources use `#version 300 es` with explicit high precision for floats, integers and sampler2D. Native sources use `#version 330 core`.

Portable arithmetic, control flow, helper functions, local numerical structures and supported texture operations remain GLSL. Public material fields are deliberately narrower than the language: scalar float/int/bool, vec2/vec3/vec4, and sampler2D. Public arrays, matrices and structures are rejected. Engine matrices are available through the ABI.

The importer rejects user `#version` and `#extension` directives, legacy `texture2D` and `gl_FragColor`, doubles, unsupported sampler kinds, image load/store, atomics, SSBOs, compute shared storage, geometry emission and other explicitly excluded tokens. Comments do not trigger those checks. The token check is an early diagnostic; reference compilation of every generated stage and permitted variant establishes actual GLSL/ESSL semantic validity.

GPU linking remains necessary after reference validation. WebGL also imposes restrictions and validation beyond desktop GLSL. The authoritative API boundary is the [WebGL 2.0 specification](https://registry.khronos.org/webgl/specs/2.0.0/); [glslang](https://github.com/KhronosGroup/glslang) supplies the development reference frontend.

Implementation: [parser](../render/shaderx/unit.go), [lexical handling](../render/shaderx/lex.go), [profile checks](../render/shaderx/shaderx.go), [source generator](../render/shaderx/gen.go).

## Includes

```glsl
#include "engine/sprite.glsl"
#include "effects/noise.glsl"
#include "./local.glsl"
#include "../shared/math.glsl"
```

The `engine/` prefix resolves below `engine://shaders/include/`. A bare logical name such as `effects/noise.glsl` keeps the shared-root contract and resolves to `assets/shaders/include/effects/noise.glsl`, including when used inside another include.

Explicit `./` and `../` paths resolve relative to the file containing the directive. For `assets/effects/water.aeshader`, `./local.glsl` means `assets/effects/local.glsl`; if that helper includes `../shared/math.glsl`, the next owner is `assets/effects/local.glsl` and the result is `assets/shared/math.glsl`. Project paths must stay inside `assets/`. Owner-relative paths inside engine shader resources must stay inside `engine://shaders/`. A missing relative include never falls back to the shared root, so moving or renaming a helper cannot silently select another file with the same basename.

Includes use canonical paths and include-once semantics. A cycle is diagnosed before it can recurse indefinitely. Missing files, backslashes, OS paths, URLs and traversal outside the owning namespace are errors. A file whose content changes between repeated reads in one import invalidates that attempt. Resolution uses VFS callbacks; the parser does not open OS files or network resources.

Material declarations and Aego metadata belong in the main shader file. Includes contain helper GLSL. Metadata must be at global scope and outside conditional preprocessing. The parser tracks comments, directives and scopes; text resembling a declaration inside a comment does not create a parameter.

| Limit | Value |
|---|---:|
| Include depth | 8 |
| Source files, including the main shader | 128 |
| Expanded source bytes | 1 MiB |
| Material declaration text before annotations | 256 bytes |
| Parameters | 32 |
| Parameter or keyword name | 32 ASCII identifier characters |
| User texture bindings in addition to the sprite texture | 6 |

Every source file has a content hash, including embedded engine includes without project AssetIDs. Those hashes participate in the import cache key together with importer/generator/ABI versions, target profile and validator identity. Changing or removing a transitive include invalidates its shader. Creating a previously missing include allows a failed shader import to recover.

The prepared artifact keeps filenames and generated-line mappings. Diagnostics retain the source/include line, backend, stage, variant, shader AssetID and raw compiler log when the compiler provides them. A missing column remains missing; the editor must not invent an exact underline.

## Parameter declarations and annotations

Declare parameters between one `#pragma material` and `#pragma end` section. Declaration order is Inspector order and contributes to the GPU layout. A declaration may span lines; put its annotations on the line with its terminating semicolon.

| Type | Source material value | Default without annotation |
|---|---|---|
| `float` | `0.5` or an integer literal | `0` |
| `int` | Exact signed 32-bit integer or declared enum name | `0` |
| `bool` | `true` or `false` | `false` |
| `vec2` | `[x, y]` | Zero vector |
| `vec3` | `[x, y, z]` | Zero vector |
| `vec4` | `[x, y, z, w]` | Zero vector |
| `vec3` / `vec4` with `@color` | Linear numerical vector or sRGB hex color | White |
| `sampler2D` | `asset("asset-id")` | White fallback when unset |

An integer such as `2147483647` is not converted through float32 in an int field. A bool field does not accept a numerical source value. Numerical values must be finite. Vector component counts must match their declaration.

| Annotation | Meaning |
|---|---|
| `@default(0.5)` | Scalar default; one number broadcasts to a vector |
| `@default(1, 0.5, 0, 1)` | Explicit vector components |
| `@default(-2147483648)` | Exact int32 default for an int field |
| `@default(true)` | Boolean default; the declaration parser also accepts `0` / `1` |
| `@range(0, 1)` | Inspector range and validation warning when outside it; not an implicit GPU clamp |
| `@color` | Color semantics for vec3 or vec4 |
| `@instance` | Allows a numeric/bool field to be overridden per node |
| `@former(OldName, OlderName)` | Retains values authored under previous names |
| `@enum(Off, On, Auto)` | Symbolic names for zero-based int values; up to 64 unique names |
| `@display("Flash amount")` | Display label |
| `@doc("Amount of white flash")` | Help text |
| `@unit("region texels")` | Display unit |
| `@hidden` | Hides a declared field from normal Inspector presentation |
| `@texture` | Explicit texture hint on sampler2D |
| `@normal` | Optional normal-map sampler on a `forward2d` shader; its own source/atlas mapping uses the reserved normal binding |
| `@emission` | Optional Emission RGB or scalar Emission Mask sampler on a `forward2d` shader; its own mapping uses the reserved emission binding |

Texture defaults are material asset references, not GLSL annotation values. Duplicate/unknown annotations, conflicting aliases, malformed arguments and unsupported parameter types produce diagnostics. A parameter supports at most 32 former names. Former names cannot also name another current parameter or another alias.

A numeric `@default` for a color is already linear. In `.aemat`, `#808080ff` is decoded from sRGB only when the target field has color semantics. The same hex value assigned to an ordinary vec4 remains numerical channel values. Alpha is never sRGB-decoded.

The schema comes from declarations, including fields the GPU optimizer removes. Active uniform reflection checks the linked layout; it does not remove declared fields from the Inspector or material document.

## Material source and value inheritance

The following is the source shape. Replace the example ID with the shader AssetID created by the asset pipeline.

```aetext
material version=1 shader=asset("00000000000000000000000000000001") {
    blend=premultiplied
    variants=[]
    params {
        Opacity=1
    }
}
```

The effective value is shader default → explicit material value → explicit node override. `Opacity=1` stays explicit even if the shader default is also one. Reset removes the explicit property and restores inheritance. Omitting `blend` inherits the shader's declared blend mode.

Material source editing preserves unknown fields, nested semantic data, additional blocks and unresolved AssetIDs. A removed parameter is retained without being uploaded. If a later shader adds that name again, its explicit value is restored when compatible; an incompatible retained value rejects reload until it is repaired. Explicit unresolved textures retain their IDs and block export. Ordinary material samplers reject activation; an explicit missing normal reference instead produces a diagnostic and flat runtime normal so authoring can continue, with strict export still blocked. An explicit zero normal reference deliberately selects flat normal rather than the sprite-frame pair.

Aliases participate in source validation and runtime migration. Setting/resetting a renamed property removes assignments through its current and former names. Two explicit assignments that resolve to the same parameter are an error. Historical Undo can restore semantic source that no longer matches the current shader; diagnostics remain visible and rendering keeps its compatible generation.

Material source documents are limited to 2 MiB, nesting depth 32 and 8192 semantic items. These bounds also protect unknown fields. Source semantics use `MaterialDoc.Value`, `SetValue`, `Unset` and `MarshalMaterial`; `Values` is a derived runtime projection rather than the editable document.

Implementation: [source document](../asset/material.go), [shared material mutations](../scene/mutate/material.go), [unknown value retention](../render/material/retained.go).

## Variants

Declare keywords, then optionally restrict their permitted combinations:

```glsl
#pragma variant GLOW DISTORT OUTLINE
#pragma variants GLOW, DISTORT, GLOW+OUTLINE
```

The example permits no keywords, GLOW, DISTORT, and GLOW+OUTLINE. The empty combination is always included when an explicit list is used. An omitted list means all combinations, provided they fit the budget.

There are at most eight keywords and 32 permitted combinations per domain. Eight independent keywords need 256 combinations and are rejected without a restricted list. A material may not name an unknown keyword or a combination absent from the shader's list. Keyword order in a material does not affect its canonical mask.

Variant conditionals can change shader behavior, not the material schema. Do not conditionally add/remove a public field or change its type. The importer validates every permitted combination in both dialects. The runtime creates programs for combinations it needs, with device/context generation included in cache identity.

## Engine ABI and packing

The source ABI version is 1. Its declarations are owned by the generator and defined in [render/shader/abi.go](../render/shader/abi.go).

| Data | Binding | Update ownership |
|---|---|---|
| Frame/world time | `ub0` | Current world frame; game time, delta, unscaled time, frame index |
| View | `ub1` | Camera/view; view-projection matrix, physical viewport, scale and camera data |
| Material | `ub2` | Shared material revision; non-instance numerical fields |
| Mesh draw | `ub3` | Model transform, tint and main texture UV rectangle |
| Main texture | `tex0`, exposed as `aego_main` | Sprite/mesh texture |
| Optional normal map | `tex1` | Forward2D normal binding, independently mapped from albedo |
| Material textures | `tex2` through `tex7` | Six whole texture references and sampler bindings, unchanged by Lighting |
| Optional lighting | `ub4` | Ambient/count plus five vec4 arrays for eight selected lights; 656 bytes |
| Optional surface data | `ub5` | Independent mesh normal/emission UV and original local rectangles, channel mode; 80 bytes |
| Optional shadow masks | `tex8` | R8 array, one layer per selected shadowed local light |
| Optional cookies | `tex9` | R8 array, common binding for selected Sprite lights |
| Optional emission | `tex10` | Straight RGB or scalar R mask, with its own source/frame mapping |

`aego_time`, `aego_delta`, `aego_unscaled` and `aego_frame_index` expose Frame data. `aego_view_proj`, `aego_viewport`, `aego_pixel_scale`, `aego_pixels_per_unit`, `aego_zoom` and `aego_camera_pos` expose View data. Viewport coordinates and dimensions are physical target pixels, including the chosen DPR/offscreen size. `aego_clip_to_pixel_y` is −1 for a normal top-left window view and +1 for the renderer's flipped offscreen view; use it in both directions when converting clip Y to viewport pixel Y.

Uniform blocks use std140. Scalar float/int/bool storage is four bytes. vec2 aligns to eight bytes; vec3/vec4 align to sixteen. A following scalar can occupy the fourth word after vec3; block size rounds up to sixteen bytes. The CPU packer writes explicit offsets and zero padding instead of copying a Go structure.

| Declaration order | Byte offsets | Block size |
|---|---|---:|
| float → vec3 → float | 0, 16, 28 | 32 |
| vec2 → bool → vec4 | 0, 8, 16 | 32 |

Backend integration queries active block members and compares offsets/types against the schema. A relink establishes uniform-block and sampler bindings again. Frame, View and Material are independent update groups; several cameras or materials need their own valid uploads.

### Typed per-instance data

Up to eight numeric/bool parameters may allow overrides. They must also fit into four uvec4 lanes. Parameters are packed in declaration order and do not straddle a lane. Eight vec4 parameters therefore do not fit, while several scalars may share one lane.

The full vertex layout includes four base engine attributes and up to four lane attributes at locations 4–7. Trimmed sprites additionally use an optional vec4 attribute at location 8 in buffer slot 2. Reflected/sheared or Lit sprites use the complete affine/normal/emission-region stream at locations 9–13, slot 3, 80 bytes per instance; ordinary positive orthogonal Unlit sprites keep the original 52-byte stream. All transformations preserve the full affine basis, including inherited shear. The device validates the complete layout against its vertex-attribute capability. A second buffer does not create another attribute budget.

Lane data preserves 32-bit words. Floating values use their float bits, ints use exact signed integer bits, and bools use zero or one. Vertex attributes are unsigned integer inputs and flat integer varyings. Generated entry setup reads float/vector values with `uintBitsToFloat`, ints with integer conversion, and bools with a zero comparison.

Before a draw stream is prepared, the renderer chooses each effective material or node value. The shader receives that result. Instance parameters are real typed GLSL variables, so local variables and structure members with the same spelling keep their lexical scope. There is no blanket regular-expression replacement of identifiers.

Generated globals have constant initializers, then receive the effective values before `vertex()` or `fragment()` is called. In the vertex stage, engine UV/color/region values use initialized private working storage and are published to the unchanged varyings after the custom entry returns. This preserves custom vertex edits and avoids misleading uninitialized-variable diagnostics from drivers that do not analyze the entry-point call graph. Shader defaults still come from material/instance data, not those initializers.

Sprites batch adjacent compatible instances with different lane values. Mesh2D uses its supported mesh layout and a single-instance lane binding. A change to texture, sampler, blend, variant, shader generation, clip, target or selected LightSetID can split batches. Visual order remains layer → order → z → submission, including material sequences A/B/A.

## UVs, transparency and built-in effects

`aego_uv` is the main texture's atlas UV. Sprite `aego_local` addresses the original, untrimmed image, including FlipX/FlipY. `aego_region` contains the packed region's atlas endpoints. `aego_local_rect` contains the corresponding signed interval in original local UV. Mesh2D uses its supplied local UV and a full local rectangle. Sample a noise/data texture using its own UV expression rather than reusing atlas UV accidentally.

The sprite helper supplies `aego_sample()`, `aego_sample_at(atlasUV)`, `aego_sample_local(originalLocalUV)` and `aego_texel()`. Region samples clamp to half-texel centers and return transparent outside the region. This prevents a linear sample near an edge from reading the neighboring tile. Local sampling inverse-maps `aego_local_rect` before addressing the atlas; cropped transparent margins remain transparent and flips remain consistent.

The trim contract retains original image size and packed content offset. Geometry is cropped while its placement and pivot remain relative to the original sprite. Untrimmed batches keep their base instance stream; the shader recognizes a missing optional trim attribute and reconstructs the full local rectangle. Atlas rotation is unsupported and must be rejected instead of being displayed as an unrotated region. A custom vertex stage bypasses automatic CPU sprite bounds culling, because arbitrary GLSL displacement cannot be inferred from simulation transforms.

Color textures and data textures have separate import semantics. Premultiplication of color texture RGB occurs in linear space before storage; an sRGB texture is encoded again for upload. Data channels such as noise remain unchanged. Existing premultiplied data is not multiplied again. The renderer supplies a premultiplied tint.

Premultiplied source-over uses ONE and ONE_MINUS_SRC_ALPHA for both RGB and alpha. Starting with Stage 12b, Additive uses ONE/ONE for RGB and ZERO/ONE for alpha: energy adds while destination coverage is preserved. This is an explicit change from 12a alpha summation. `gpu.BlendAdditiveLegacy()` keeps the former state for native callers that deliberately require it; builtin Additive uses the new policy. An additive surface may therefore leave nonzero RGB at zero alpha. Such intermediate results stay linear until composition with the final opaque background, before tone mapping.

All built-ins are ordinary source materials and shaders compiled by the same pipeline:

| Name | Parameters and behavior |
|---|---|
| Unlit / Default | Opacity; preserves ordinary sampled appearance |
| Additive | Opacity, Intensity; adds premultiplied RGB while preserving destination alpha |
| Flash | Flash, FlashColor, Opacity; changes RGB within sampled alpha, preserving transparent holes |
| Dissolve | Threshold, Noise, NoiseScale, EdgeWidth, EdgeColor, Opacity; threshold ≤0 is unchanged and ≥1 is transparent; noise uses local UV |
| Outline | Thickness in region texels, OutlineColor, Opacity; eight-neighbor internal alpha outline, with bounded atlas sampling |
| PixelSnap | GridPixels in physical pixels, Strength, Opacity; quantizes rendered vertex positions in viewport space without changing simulation transforms |
| Lit | Sprite Forward2D, NormalMap, NormalStrength, EmissionColor/Intensity and ReceiveShadows |
| LitMesh2D | The same lighting model for the validated Mesh2D layout, with UV-derived tangent basis and flat fallback |

Outline does not generate an external silhouette or expand geometry. PixelSnap uses the actual viewport and Y orientation, and does not multiply its dimensions by DPR a second time. A transformed or rotated quad can change shape slightly as its individual corners snap. Lit and LitMesh2D have distinct validated domains and perform per-surface forward lighting; neither is an alias for Unlit.

The definitions live in [engine/res/shaders/materials](../engine/res/shaders/materials). `asset.BuiltinMaterialNamed` and `asset.BuiltinShaderNamed` resolve their stable IDs. Dissolve's default Noise is a deterministic 64×64 linear data texture.


## Forward 2D lighting

Use `Lit` for a Sprite and `LitMesh2D` for a compatible mesh. Add Light2D and a LightingEnvironment2D through the normal palette/Inspector. A light does not change the global order of transparent objects. Its layer mask intersects the receiver's visibility layers; ShadowCasterMask separately selects occluders. Both masks retain all 64 bits. New lighting masks treat an explicit zero as none. The old Camera2D zero-as-all convention remains unchanged.

A user shader opts into the same renderer with:

```glsl
#pragma aego abi 1
#pragma domain sprite
#pragma lighting forward2d
#pragma material
uniform sampler2D NormalMap; // @normal
uniform sampler2D EmissionMap; // @emission
uniform float NormalStrength; // @instance @default(1)
uniform vec4 EmissionColor; // @instance @color @default(0,0,0,1)
uniform float EmissionIntensity; // @instance @default(0)
uniform bool ReceiveShadows; // @instance @default(true)
#pragma end
#include "engine/sprite.glsl"
#include "engine/lighting.glsl"
void fragment() {
    aego_out = aego_lit(aego_sample(), NormalStrength,
        aego_emission(EmissionColor.rgb, EmissionIntensity), ReceiveShadows);
}
```

`@normal` is a single sampler2D role at tex1; `@emission` is a distinct optional role at tex10. Neither consumes any of the six material texture slots. EmissionRGB is straight color decoded according to its declared color space; EmissionMask uses its selected channel imported into linear R. Both multiply EmissionColor and EmissionIntensity, and ignore image alpha: albedo supplies coverage. An absent map preserves uniform emission. An explicit material map with independent mapping wins over the current frame pair; resetting the field restores inheritance. Other sampler declarations retain their full-texture semantics. The helper adds diffuse light to the already premultiplied albedo and emission multiplied by coverage once; alpha is unchanged. Emission increases the surface’s own radiance. A Light2D illuminates neighbouring geometry. For a halo around an emissive lamp, enable Bloom on the output camera and set its threshold below the lamp’s brightness. Bloom samples the composed scene before tone mapping, with the menu drawn afterwards.

Point and Spot use world-unit Range and a virtual Height above the plane. A flat normal cannot receive ordinary XY-only diffuse light, so use positive Height for the expected 2D appearance. Spot direction is the node's transformed local +X; InnerAngle and OuterAngle are full cone angles in radians. Directional uses the node's in-plane direction and an elevation in (0, pi/2]. Sprite lights multiply local lighting by a linear cookie mask. Color and Intensity are runtime data, not shader keywords.

At most eight lights are selected per compatible receiver set, using Priority, estimated influence and stable ID. Overflow is visible in the profiler and Scene View overlay. Different selected sets can split adjacent batches; they never reorder the painter stream.

### Normal sources and imports

Normal source priority is explicit material NormalMap with its own mapping, then the current sprite/animation frame's normal pair, then flat (0,0,1). A missing explicit map or broken pair preserves its ID and reports a diagnostic. A null explicit material map is an intentional flat override. Albedo UV is not reused for an independently packed normal region.

Texture `.meta` settings include `semantic=normal|mask|height|data|color` (or auto), `normal_y=up|down`, `generate_normal=true`, `height_channel=r|g|b|a`, and `height_strength`. The canonical normal convention is +Y image-up. Height generation runs during import, including frame-aware edge handling; no per-frame normal generation occurs. `_n.png` discovery writes a durable AssetID pair once, while explicit references take precedence. Renaming with `.meta` preserves the association.

Color and data assets may name an atlas group, but different semantics produce separate pages. Normal/data pixels never undergo premultiply or sRGB conversion. Normal page padding extrudes valid vectors, and the shader normalizes the sampled vector. Atlas rotation and atlas mipmaps without region isolation are rejected. Standalone textures may use mipmaps. A sliced flipbook sheet currently stays outside an atlas page; each paired frame still retains independent UV/original-size/trim mapping. Reimport validates matching source dimensions and frame mappings before replacing either live resource.

Sprite normals use the inverse transpose of the full affine basis, with FlipX/Y orientation applied once. Mesh2D reconstructs a local tangent basis from geometry and UV, including its handedness, then applies the model basis. Missing maps, degenerate UV and singular bases use flat normals, with transform diagnostics supplied by extraction. Simulation transforms are not changed by lighting or pixel snapping.

### Occluders and shadows

LightOccluder2D accepts a simple closed polygon without holes or an open nonintersecting chain. It can read component geometry from a neutral ShapeProvider. The Physics adapter works in Edit, Preview and Play without starting the solver. Geometry revision and presentation transform revision are independent. A sprite is not an occluder unless the component is explicitly added.

Point, Spot and cookie lights cast local shadows; enabling Directional shadows is an error. A closed contour with the light inside or on its boundary fully occludes that source. A mask represents one source and its caster set, so there is no per-receiver self-occluder exclusion. `ReceiveShadows=false` bypasses masks for a material while leaving its lighting and casting behaviour intact.

Masks use layers of one R8 array, configurable up to sixteen shadowed sources per camera. Extruded volumes write an opaque union. Softness filters only the source's layer with a bounded 3x3 kernel, up to four texels. It is mask filtering, not an area-light model. Offscreen casters are considered by influence range. Colour/intensity changes and camera panning do not rebuild unchanged light-space geometry; shape, pose, masks, resolution, softness and device generation do.

Cookies share an R8 array of 256-square layers. Import mapping preserves the source aspect ratio with zero-mask margins. These resources plus albedo, normal, emission and all six user textures are validated together against device limits (eleven fragment units in total).

### HDR, composition and inspection

Environment ColorMode Auto selects linear RGBA16F when the output has Lit receivers and the actual format probe succeeds; LDR uses linear RGBA8. Explicit HDR can activate the output for Unlit content. The runtime reports the real mode and fallback reason. It does not require a 32-bit float blending extension for RGBA16F. A fallback after framebuffer failure is retained until size/device changes, avoiding failed allocations each frame.

World Unlit objects receive no lights but share their output's exposure and resolve. HDR display mapping is `x / (1 + x)` after exposure. Transparent surface color is unpremultiplied, mapped and premultiplied. An sRGB attachment stores encoded linear-premultiplied RGB and decodes it when later sampled; an explicit final RGBA8 display target stores encoded straight display color multiplied by coverage. Do not treat final encoded RGBA8 display bytes as a linear world texture. UI/editor overlays are rendered after world resolve.

Cameras with one logical OutputID compose before one resolve and must agree on Environment and destination subresource. Their clear rectangles still apply. A retained editor backdrop is composited at display time, and a fresh linear intermediate prevents accumulating the previous frame's world. ViewID and OutputID survive pooled target replacement/resize and are independent of labels and device generations.

`view.OutputLinear` requires an explicit RGBA16F target. It preserves RGB energy with coverage alpha, including RGB greater than zero at alpha zero. Sample and compose that intermediate into the final linear scene before display mapping. The two-background integration test checks this ordering. A standalone transparent display output containing Additive is rejected unless an opaque output background was established; a PNG cannot promise background-independent HDR emission composition.

Lighting Off is inspection state: the same assets are displayed in LDR at exposure 1 with builtin Lit lighting/emission bypassed. Normal/shadow debug displays also use an unmapped LDR view. None of these controls changes materials, simulation state or prefab overrides.

### Runtime lifetime and measurements

Loss of a WebGL context invalidates pending generations and restores light UBOs, array layers, texture contents, output targets and bindings. Old async results cannot publish into a restored device. Logical GPU object counts are checked through repeated good/bad reload, resize and session Close. View/output caches retain at most sixteen entries and evict after inactivity; resident bytes are reported separately from per-frame upload bytes.

Profiler counters distinguish selected sets, unique lights dropped by budget, receiver assignments dropped, shadow cache hits, dirty masks, lighting batch splits, uploads and target storage. CPU timings are labeled CPU: select, shadow geometry, auxiliary submission, scene submission and resolve submission. The existing asynchronous GPU interval covers the first scene pass only, not shadow/resolve passes; disjoint/context-canceled samples are invalid and unavailable extensions remain unavailable. Do not substitute CPU wall time for GPU time. `BenchmarkLightingRender` checks cached, moving and sixteen-source stress paths with gpu/null; this is CPU preparation evidence, not a hardware GPU result.

The canonical demonstration is [examples/platformer](../engine/res/docs/platformer.md). It keeps the existing game and compares updated assets with Lighting Off/On, the old short replay and a full route to Finish. Native/web exports and the site's demo use this same project.

## Prepared artifacts and exported runtime

`.aeshader` and `.aemat` are source assets. The importer produces these runtime artifacts:

| Artifact | Version | Contents |
|---|---:|---|
| AESH | 4 | Prepared GLSL330/ESSL300 stages per variant, schema, optional Affine/Lighting/SurfaceChannels feature flags, domain/blend, custom-vertex policy, exact defaults, aliases, source/dependency maps, layout/content hashes and validation flags |
| AEMT | 1 | Binary semantic material document with shader/variant/blend, explicit typed values, texture IDs and retained unknown data |
| AETX | 3 | Texture pixels and explicit Color/Data/Normal/Mask/Height/EmissionRGB/EmissionMask/CookieMask semantic |
| AERG | 4 | Sprite region, original image mapping and paired normal/emission AssetIDs |

AESH2/3, AETX1/2 and AERG1/2/3 are still decoded. Legacy AESH2 has no new affine/lighting feature bits, so reimport its source before using the new optional stream. Shader generator version 6, texture importer version 5 and atlas packer version 4 invalidate their respective caches; AEMT did not change.

AESH is not a GPU binary. The player's driver still compiles and links its prepared sources. It does not expand includes or run the shader importer, reference validator or editor. AEMT decoding does not parse `.aemat` text.

AESH/AEMT decoders bound sizes/counts, validate headers, versions and their content checksums, reject truncated or trailing data, validate shader schemas/layouts/source maps, and preserve unknown semantic material properties. AETX/AERG decoders validate their headers, versions and texture/region bounds; they do not carry a separate checksum or reject all trailing bytes. Export manifest hashes verify the complete prepared artifact payloads. AESH is limited to 64 MiB. Reimport source assets when an artifact version or ABI no longer matches; do not patch version bytes by hand.

Validation provenance is explicit:

| Flag | Evidence |
|---|---|
| Schema checked | Parser, declarations, resource limits and prepared artifact checks |
| Reference checked | glslang validated every permitted native and ES stage pair |
| GL33 linked | A real native driver linked the program |
| WebGL2 linked | A real browser context linked the program |

The importer marks the first two only when performed. A headless/null GPU test must not claim the final two. Runtime GPU diagnostics are separate from an asset's imported validation provenance.

Strict export compares current shader/material sources and all includes with an immutable artifact snapshot. A failed import, unsaved authoring buffer, changed current source, missing dependency, invalid variant or unchecked portable shader blocks export. Continuing to render last-good in the editor does not relax that check. Export uses the existing manifest/directory format.

Implementation: [AESH](../asset/shader.go), [AEMT](../asset/material_binary.go), [strict checks](../app/export/material.go), [immutable snapshot](../app/export/frozen.go).

## Reload, history and automation

Source or include edits import a candidate. GPU compilation/linking, linked-layout validation, the actually used trim/affine pipeline layouts and all dependent material migrations must succeed before a frame-boundary commit. Texture/normal atlas pages removed by a reimport are dropped only after the replacement loader transaction succeeds. The program, schema, parameter storage and instance layout change together. A generation token prevents an older completion from replacing a newer request or a restored device.

| Schema edit | Result |
|---|---|
| Same name and type | Preserve explicit material/node values |
| New field | Use its shader default unless retained compatible source data exists |
| Changed default | Update inherited values; retain explicit values |
| Rename with `@former` | Resolve existing explicit values through the alias |
| Removed field | Preserve semantic data without uploading it |
| Incompatible explicit type | Report the conflict and retain last-good |

Context loss invalidates device resources, bindings and locations. CPU definitions survive. Restoration recreates material resources for the new device generation; a candidate from the old device cannot commit afterward. `KHR_parallel_shader_compile`, when available, permits completion polling. Its absence does not make synchronous driver work asynchronous.

Material assets are shared definitions. Material Override belongs to a node and is cloned with its scene. Play and animation preview own their state. Tween/Animation address `MaterialOverride.Parameter`; resolved bindings are refreshed after layout generation changes.

The searchable Create palette offers Shader, Material and Material from Shader. The material source editor and Inspector share history. The node override Inspector uses scene history. A slider drag becomes one undo operation. A preview uses a separate material/session and the actual renderer; a missing GPU produces an explanatory placeholder.

CLI/MCP commands are registered by the common agent command service: `shader.create`, `shader.schema`, `shader.validate`, `material.create`, `material.get`, `material.validate`, `material.set`, `material.reset`, `material.variants`, `material.apply` and `material.assign`. They use the same semantic mutation layer as Inspector. Editor commands use dirty in-memory documents; file sessions use transactions. `sourceHash` and `expected` support optimistic concurrency. Atomic batches fail without retaining earlier partial edits.

## Diagnostics and corrective actions

| Code | Meaning | Fix |
|---|---|---|
| SHD001 | Invalid source structure, ABI, entry, or required validator unavailable | Follow the reported syntax/entry error or install the pinned validator |
| SHD002 | Include error, forbidden location/path/depth, or changing source snapshot | Correct the include path/content and retry import |
| SHD003 | Invalid parameter, annotation, scope, conditional schema or conflicting alias | Keep unconditional typed declarations and unique names |
| SHD004 | Parameter, source, texture, lane or device resource budget exceeded | Reduce the named resource or use a simpler layout |
| SHD005 | Include cycle | Remove the cycle; include-once does not authorize recursive dependencies |
| SHD006 | Invalid keyword or excessive/unknown variant combination | Declare keywords and restrict the permitted combinations |
| SHD007 | Unsupported shader domain | Use Sprite or Mesh2D with its matching consumer |
| SHD008 | Construct outside the portable profile | Rewrite it with GLSL330/ESSL300 baseline operations |
| SHD009 | Reference/GPU compile, link or linked layout failure | Inspect stage/backend/variant, mapped source line and raw log |
| MAT001 | Unknown retained material parameter; warning | Keep it for migration or explicitly reset/remove it |
| MAT002 | Type/components/explicit alias conflict, or range warning | Use the declared type/count and one explicit spelling |
| MAT003 | Unknown or forbidden material variant | Select one permitted combination |
| MAT004 | Missing/incompatible shader, domain or explicit texture | Repair the reference or assign a compatible asset |
| MAT005 | Candidate material GPU buffer could not be allocated | Free resources or reduce material usage; last-good stays active |
| MAT006 | Sprite/Mesh2D consumer has an incompatible material domain | Assign a material declared for that consumer |
| EXP013 | Current shader/material source or portable shader artifact cannot be exported | Save, fix and reimport current source, then take a fresh export snapshot |
| EXP014 | Invalid/missing material artifact, parameter or dependency | Correct material values/references and reimport |
| EXP015 | Unsaved shader/material/include buffer or texture metadata `.meta` document | Save and validate the authoring document before export |

The Problems panel can navigate into an include. If a driver reports only a generated line or an unstructured log, show the available location and raw message without a guessed column.

Runtime candidate failures are grouped under `SHD009` in Problems and structured runtime logs. A more specific `SHD004` (device budget) or `MAT005` (candidate buffer allocation) cause may appear in the diagnostic message; the table above describes those causes as well as standalone importer/validation codes.

## Validation commands

The reference frontend is pinned to glslang 15.1.0 in `tools/glslang.version`. Changing it requires an explicit pin review and validation of generated artifacts.

```sh
make shader-validator
make materials-check
AEGO_REQUIRE_GLSLANG=1 go test -tags aego_nonative ./render/shader/... ./render/shaderx/... ./render/material/... ./asset/... ./scene/mutate ./app/export
make depcheck
make test-gl
make lighting-smoke
make web-lighting WEB_LIGHTING_BROWSERS=chromium,firefox
make web-smoke
```

`make materials` regenerates committed built-in AESH/AEMT artifacts through the real importers. `make materials-check` verifies them without rewriting files. These generated-source checks do not authorize updating image goldens.

Use `make ci` for the repository's full gate. Run native export and web export for the demonstration project, then inspect actual rendered pixels on a transparent offscreen target and checkerboard. Record unavailable GPU/browser tests as not executed. The [GL tests](../tests/gl/material_test.go), [browser material harness](../tests/webmaterials), pure parser/layout/material tests and export/mutation tests cover different evidence levels.

Measure steady-state allocations and uploads after buffers have grown. Compare the same scene, viewport, DPR, seed and build mode. Neighboring compatible sprites differing only in permitted overrides should keep the same batch count; unrelated material resources or mesh layouts do not have that guarantee.
