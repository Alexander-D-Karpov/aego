package asset

import (
	"aego/core/ids"
	"aego/render/material"
	"aego/render/mesh"
	"aego/render/texture"
)

type ArtifactKind string

const (
	ArtTexture       ArtifactKind = "texture"
	ArtAtlasPage     ArtifactKind = "atlaspage"
	ArtSpriteRegion  ArtifactKind = "spriteregion"
	ArtScene         ArtifactKind = "scene"
	ArtPrefab        ArtifactKind = "prefab"
	ArtMesh          ArtifactKind = "mesh"
	ArtScript        ArtifactKind = "script"
	ArtData          ArtifactKind = "data"
	ArtTable         ArtifactKind = "table"
	ArtPreset        ArtifactKind = "preset"
	ArtAudio         ArtifactKind = "audio"
	ArtAnim          ArtifactKind = "anim"
	ArtCtrl          ArtifactKind = "animctrl"
	ArtShader        ArtifactKind = "shader"
	ArtShaderInclude ArtifactKind = "shaderinclude"
	ArtMaterial      ArtifactKind = "material"
)

type Reloaded struct {
	ID   ids.AssetID
	Kind ArtifactKind
}

type Resolver interface {
	Texture(id ids.AssetID) (texture.Region, bool)
	Mesh(id ids.AssetID) (*mesh.Mesh, bool)
	Scene(id ids.AssetID) ([]byte, bool)
	Prefab(id ids.AssetID) ([]byte, bool)
	Generation() uint32
	Subscribe(fn func(Reloaded)) func()
}

type Decoder interface {
	Decoded(id ids.AssetID, key string, build func() (any, error)) (any, error)
}

type AnimSource interface {
	Anim(id ids.AssetID) ([]byte, bool)
}

type CtrlSource interface {
	Controller(id ids.AssetID) ([]byte, bool)
}

type PresetSource interface {
	Preset(ids.AssetID) ([]byte, bool)
}

type DataSource interface {
	Data(id ids.AssetID) ([]byte, bool)
	Table(id ids.AssetID) ([]byte, bool)
}

// MaterialSource is optional so resolvers for scenes without materials remain
// valid. Runtime loaders expose only prepared definitions through this API.
type MaterialSource interface {
	Material(id ids.AssetID) (*material.Material, bool)
}

type ShaderSource interface {
	Shader(id ids.AssetID) (*material.Shader, bool)
}

// NormalSource is an optional prepared-asset association. The returned ID stays
// nonzero when a saved normal reference cannot currently be resolved. Regions
// have their own atlas UV and original-canvas trim mapping; callers must not
// sample the normal texture using the albedo region's atlas coordinates.
type NormalSource interface {
	NormalFor(albedo ids.AssetID) (normalID ids.AssetID, region texture.Region, ok bool)
}

// TextureMetadata describes prepared source semantics without requiring a GPU
// texture. Validators and export can distinguish a valid data mask from an
// unresolved reference even when no rendering device is present.
type TextureMetadata struct {
	Width, Height int
	Semantic      TextureSemantic
	Region        bool
}

type TextureMetadataSource interface {
	TextureMetadata(ids.AssetID) (TextureMetadata, bool)
}

// EmissionSource exposes prepared independent region mappings. A nonzero ID
// survives a missing source; callers use uniform emission as the fallback.
type EmissionSource interface {
	EmissionFor(albedo ids.AssetID) (ids.AssetID, texture.Emission, bool)
}
