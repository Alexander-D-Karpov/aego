---
id: formats/overview
icon: file.data
group: ref-formats
order: 0
title: Formats overview
---
# File formats

Aego projects contain editable source files; the importer prepares separate runtime artifacts. Most project-owned text formats use the `aetext` syntax. The importer for a format may require its corresponding module to be enabled.

## Project and source formats

| File | Purpose |
|---|---|
| [project.aego](ref:formats/aego) | Project identity, startup scene and module settings. |
| [.aescene](ref:formats/aescene) | A scene with nodes, transforms and components. |
| [.aeprefab](ref:formats/aeprefab) | A reusable node hierarchy and instance overrides. |
| [.aes](ref:formats/aes) | Aego Script source. |
| [.aeart](ref:formats/aeart) | Aego's editable drawing container; not an Aseprite file. |
| `.aeanim` | [Animation clip](ref:formats/aeanim) with tracks, sprite frames and events. |
| `.aectrl` | [Animation controller](ref:formats/aectrl) with parameters, states and transitions. |
| [.aepreset](ref:formats/aepreset) | Saved component or script values. |
| [.aedata](ref:formats/aedata), [.aetable](ref:formats/aetable) | Typed data records and tables. |
| [.aemat](ref:formats/aemat), [.aeshader](ref:formats/aeshader) | Material parameters and shader source. |
| `.glsl` | Shader include source. |
| [.meta](ref:formats/meta) | Stable asset identity and import settings; keep with its source. |
| [.aetext](ref:formats/aetext) | Shared structured-text syntax; each consuming format defines its own schema. |

## External inputs

| File | Import behavior |
|---|---|
| `.png` | Imported as a texture with separate sprite metadata. See [Art Studio](ref:render2d/art) for the development in-memory drawing workflow and first Save; opening alone must not overwrite the PNG. |
| `.ase`, `.aseprite` | Explicit Aseprite import creates a separate `.aeart` copy. Unsupported data can require confirmation or prevent import; the original is not overwritten. |
| `.wav`, `.ogg`, `.mp3` | Audio clips. WAV and Ogg Vorbis are supported directly; MP3 conversion requires FFmpeg. See [Audio clips](ref:audio/clips). |
| `.obj` | Mesh import when the 3D rendering module is enabled. |

An accepted import extension does not establish a complete audio or 3D-modelling editor. Follow the relevant guide for the actual authoring capabilities of this build.

## Generated files and game saves

[Prepared artifacts](ref:formats/artifacts), [.aetry documentation bundles](ref:formats/aetry) and [.aesave game save slots](ref:formats/aesave) have separate roles. They appear under [Generated and runtime files](ref:formats/runtime), not among artist-editable sources. Import caches, prepared binary signatures and export manifests must not be edited as replacements for source files.
