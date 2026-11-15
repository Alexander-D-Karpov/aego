---
id: formats/source
group: ref-formats
order: 1
icon: file.folder
title: Project and authoring files
summary: Editable Aego project, scene, script, drawing, animation, material and data sources, with their metadata.
---
# Project and authoring files

Choose a file by the work you want to save. These are project sources, not files copied out of an import cache. Keep an asset's source and `.meta` together in version control. Use [generated and runtime files](ref:formats/runtime) to identify outputs that should not be edited as sources.

---
id: formats/aego
parent: formats/source
order: 1
icon: doc.project
title: project.aego — project configuration
summary: Project identity, main scene, window, modules and project-wide defaults.
---
# project.aego — project configuration

`project.aego` is the project entry point. It records the game and module identity, main scene reference, window and module settings. Project Settings edits the corresponding registered sections; the file can also be reviewed as structured text.

Example: the `render { pixelsPerUnit=64 }` fragment sets a project rendering default; it is not a complete project. Use the new-project template to create the required project structure. Save settings explicitly, then reopen the project or run its checks to verify the saved configuration. Preserve the main scene's AssetID rather than substituting an arbitrary path. See [project configuration fields](ref:section/engine) and [player settings](ref:settings).

---
id: formats/aescene
parent: formats/source
order: 2
icon: file.scene
title: .aescene — scenes
summary: A saved hierarchy of nodes, transforms, components and asset references.
---
# .aescene — scenes

A scene is structured text containing nodes with stable node IDs, transforms and registered components. Open it in Scene View, edit the hierarchy and save the scene document. Saving a script or an imported image does not also save scene assignments.

Example: `scenes/main.aescene` can contain a camera and a player whose SpriteRenderer references a prepared sprite by AssetID. Verify by closing and reopening the saved scene, then starting Play. A copied scene does not automatically include its scripts, textures or prefabs. See [Your first project](ref:quickstart) and [Prefabs](ref:prefabs).

---
id: formats/aeprefab
parent: formats/source
order: 3
icon: file.prefab
title: .aeprefab — reusable hierarchies
summary: Reusable node hierarchies with instance references and overrides.
---
# .aeprefab — reusable hierarchies

A prefab stores a reusable hierarchy, not a screenshot or an exported game. Edit its source in the prefab workflow; an instance in a scene keeps its link and its own overrides. Applying an override to the prefab and saving the scene are different operations.

Example: `prefabs/enemy.aeprefab` supplies several enemy instances in a level. After saving a source change, verify an instance without an overriding value and one with an override. Preserve source metadata and inspect dependencies before moving the prefab to another project. See [Prefabs](ref:prefabs).

---
id: formats/aes
parent: formats/source
order: 4
icon: file.script
title: .aes — Aego Script
summary: Typed gameplay scripts compiled by Aego, not Go source or prepared runtime bytecode.
---
# .aes — Aego Script

An `.aes` file declares an Aego Script package and its scripts, functions and types. Edit it in the script editor or a text editor, then save and resolve the compiler's diagnostics. A script component must also be attached to the intended scene node.

Example: `scripts/game/spinner.aes` from [Your first project](ref:quickstart) rotates a node. Verify the saved script during Play and in an exported game. Import prepares runtime code; do not edit the prepared script artifact instead of the `.aes` source. Native extensions use Go and a different build path; see [Aego Script](ref:language).

---
id: formats/aeart
parent: formats/source
order: 5
icon: file.art
title: .aeart — drawing documents
summary: Aego's editable drawing container for layers, frames, cels, palettes, tags and surface channels.
---
# .aeart — drawing documents

`.aeart` is Aego's own drawing container, not an Aseprite file. Art Studio edits its layers, frames, linked cels, palette, durations, tags and supported surface channels through one document history. Save the drawing source before publishing its prepared sprite and animation resources.

Example: create `assets/hero.aeart`, draw two frames, save, close and reopen it to verify pixels and durations. A PNG linked to a drawing is not permission to overwrite that PNG. Exporting an interchange `.aseprite` copy is not the same as saving this source or migrating its stable IDs. See [Art Studio](ref:render2d/art) for the documented development status and import limitations.

---
id: formats/aepreset
parent: formats/source
order: 8
icon: file.preset
title: .aepreset — component and script presets
summary: Reusable component or script values, including particle-effect presets.
---
# .aepreset — component and script presets

A preset stores authored component or script values in a prefab-shaped text document. The importer requires a `prefab` root containing a component or script attachment. A preset is not a complete scene and does not bundle every referenced texture or animation.

Example: `assets/effects/dust.aepreset` can hold ParticleSystem2D settings. Apply the preset to an appropriate node, then save the affected scene or source separately. Verify the resulting component and its referenced sprites during Play. See [Particles](ref:particles) for emission, Local/World space and Stop/Clear behaviour.

---
id: formats/aedata
parent: formats/source
order: 9
icon: file.data
title: .aedata — typed data records
summary: A single registered data record with defaults, optional inheritance and asset references.
---
# .aedata — typed data records

An `.aedata` source identifies a registered data type and stores its authored fields. A compatible base record supplies inherited values; only overrides need to be written. The type must be supplied by the project or a module before the importer can validate the record.

Example: `assets/data/enemy.aedata` can use the `mygame.EnemyStats` type defined in [Data assets](ref:data). Open the data document, change a value and save. Verify import diagnostics and the consumer's reference. A reference reads the asset; it is not a mutable copy of the record during Play.

---
id: formats/aetable
parent: formats/source
order: 10
icon: file.table
title: .aetable — typed data tables
summary: Rows of one registered data type, addressed by unique row IDs.
---
# .aetable — typed data tables

An `.aetable` contains rows of one registered type. Each row has a unique ID used by consumers. Open the table editor to edit cells, filter and sort; save the table document to persist source changes.

Example: `assets/data/loot.aetable` can contain rows named `goblin` and `boss` of a registered loot type. Preserve row IDs when other data refers to them. CSV import/export is an exchange operation, not a replacement for the table's schema, source metadata or Save. Check the imported rows and references after a round trip. See [Data assets](ref:data).

---
id: formats/aemat
parent: formats/source
order: 11
icon: file.preset
title: .aemat — materials
summary: A shader reference, variant keywords, typed parameters and texture bindings.
---
# .aemat — materials

A material source selects a shader and supplies typed parameter values, keywords and texture references. The material document and its shader are separate sources. Saving a material that several sprites share can affect all of its consumers; per-instance overrides are a different scope.

Example: `assets/materials/hero.aemat` can reference a sprite-compatible shader; a camera Custom effect instead needs a postfx shader. Save the corrected source, inspect its current import result and test the actual consumer. Follow [MAT004](ref:MAT004) for domain or texture-binding errors. See [Materials and shaders](ref:render2d/materials).

---
id: formats/aeshader
parent: formats/source
order: 12
icon: code.block
title: .aeshader — shader sources
summary: Shader declarations, rendering domain, parameters and GLSL stages prepared for the graphics backend.
---
# .aeshader — shader sources

An `.aeshader` is the editable shader source. Its declarations determine the rendering domain and material interface; imported and prepared shader data is a different format. Include files such as `.glsl` remain dependencies of this source, not independent materials.

Example: `assets/shaders/cold.aeshader` with the `#pragma postfx` declaration belongs in a camera post-processing material, not on a sprite. That declaration alone is a fragment, not a complete runnable shader. Save the source and includes, read import diagnostics and verify the intended graphics backend. [SHD009](ref:SHD009) explains driver/link failures and nested causes; see [Materials and shaders](ref:render2d/materials).

---
id: formats/meta
parent: formats/source
order: 13
icon: panel.settings
title: .meta — asset identity and import settings
summary: Sidecars that preserve AssetIDs, importer settings, sprite slices and source associations.
---
# .meta — asset identity and import settings

An asset's `.meta` sidecar records its stable identity and import settings. For an image, it can include persistent sprite regions and surface associations. The metadata document can be dirty independently of the image's pixels.

Example: keep `assets/hero.png` and `assets/hero.png.meta` together when renaming or transferring the asset. Prefer asset operations that update references. Do not delete metadata to clear an import error: generating another ID can break scene and animation references. Save import settings explicitly and resolve external conflicts before writing. See [Image and Sprite authoring](ref:render2d/image-viewer).

---
id: formats/aetext
parent: formats/source
order: 14
icon: file.data
title: .aetext — structured text
summary: The shared text syntax used by configuration, registries and several source formats.
---
# .aetext — structured text

`aetext` is a syntax, not a promise that every text document is an importable game asset. A format defines its own root, version, fields and validation. Configuration and generated manifests can both use `.aetext`; their ownership and save rules differ.

Example: a project can contain the `render { pixelsPerUnit=64 }` fragment, while `export.aetext` describes an exported build. Edit the owning source or settings, not a generated manifest. A syntax-valid unknown field can still be rejected by that format's schema. See [Project configuration](ref:formats/aego) and [generated files](ref:formats/runtime).

---
id: formats/runtime
group: ref-formats
order: 50
icon: file.package
title: Generated and runtime files
summary: Import artifacts, documentation bundles, build manifests and game saves are not authoring source files.
---
# Generated and runtime files

These formats serve import, deployment or the running game. Do not replace an artist's source with a prepared artifact. Source metadata links the authoring inputs to generated asset identities; strict export validates the saved source state before packaging.

---
id: formats/artifacts
parent: formats/runtime
order: 1
icon: file.package
title: Prepared asset artifacts
summary: Generated textures, sprite regions, shaders, materials, meshes, scripts, audio and data for the runtime.
---
# Prepared asset artifacts

The importer produces runtime artifacts for texture/atlas pages, sprite regions, scenes, prefabs, meshes, scripts, data records, tables, presets, audio, animation clips/controllers, shaders/includes and materials. An export lists its content-addressed files in the build's asset manifest; their cache filename is not a new source format to edit manually.

Examples include AETX texture bytes, AERG sprite-region metadata, AESH prepared shaders, AEMT materials and AEAU audio. These binary signatures are not interchangeable with `.png`, `.aeart`, `.aeshader`, `.aemat` or an audio source. Rebuild the affected artifact from its source with the matching importer instead of patching bytes. Keep the complete exported data folder and verify its manifest rather than copying one artifact in isolation. See [Export](ref:cli).

---
id: formats/aetry
parent: formats/runtime
order: 2
icon: play.start
title: .aetry — prepared documentation examples
summary: Bounded prepared resources for a documentation Try scene, not an editable source project.
---
# .aetry — prepared documentation examples

A documentation `.aetry` holds prepared runtime resources in the AETB bundle format. It accompanies a Try scene whose source project lives in a corresponding `.assets` directory. The generator verifies source/importer dependencies so a stale bundle is not accepted as current evidence.

Example: `CameraOutput.aetry` supplies prepared resources for the camera-effects demonstration. Changing its source requires regeneration and validation, not editing the bundle by hand. Downloading only the `.aescene` does not include these original sources. A complete example-project download is a separate package with scripts, prefabs, source assets and metadata.

---
id: formats/aesave
parent: formats/runtime
order: 3
icon: panel.saves
title: .aesave — game save slots
summary: Persistent game state written by the save system, separate from editor document Save.
---
# .aesave — game save slots

The save system writes `.aesave` game-state documents and optional PNG previews to player storage. This is not how a scene, script or drawing source is saved. The save data identifies the game, engine version, scene and the participating saved state.

Example: a game's Save command writes a slot through the save API; loading it restores supported saved state in the appropriate game. Test by changing state, saving a slot and loading it again. Keep the original slot when diagnosing a version or scene mismatch. Desktop profiles and browser storage have different locations and limits; see [Saving games](ref:save).
