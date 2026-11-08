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
| `project.aego` | Project identity, startup scene and module settings. See [Project settings](ref:section/engine). |
| `.aescene` | A scene with nodes, transforms and components. |
| `.aeprefab` | A reusable node hierarchy; see [Prefabs](ref:prefabs). |
| `.aes` | Aego Script source. |
| `.aeart` | Aego's editable drawing container, including its frames and layers. It is not an Aseprite file; see [Pixel art](ref:render2d/art). |
| `.aeanim` | [Animation clip](ref:formats/aeanim) with tracks, sprite frames and events. |
| `.aectrl` | [Animation controller](ref:formats/aectrl) with parameters, states and transitions. |
| `.aepreset` | Saved component or script values. |
| `.aedata`, `.aetable` | Typed data assets and tables; see [Data](ref:data). |
| `.aemat`, `.aeshader` | Material parameters and shader source. |
| `.glsl` | Shader include source. |
| `.meta` | Sidecar containing an asset's stable ID and import settings. Keep it with its source when moving or renaming files. |

## External inputs

| File | Import behavior |
|---|---|
| `.png` | Imported as a texture; sprite slices and settings are stored separately. The image viewer can create a new editable `.aeart` copy without replacing the PNG or changing scene references. |
| `.ase`, `.aseprite` | Explicit Aseprite import creates a separate `.aeart` copy. Unsupported data can require confirmation or prevent import; the original is not overwritten. |
| `.wav`, `.ogg`, `.mp3` | Audio clips. WAV and Ogg Vorbis are supported directly; MP3 conversion requires FFmpeg. See [Audio clips](ref:audio/clips). |
| `.obj` | Mesh import when the 3D rendering module is enabled. |

Importing audio or OBJ does not mean Aego includes full audio or 3D-model editors. Cache and export artifacts are generated outputs, not substitutes for the original source and its `.meta` file.
