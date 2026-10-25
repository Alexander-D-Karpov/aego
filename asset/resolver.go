package asset

import (
	"aego/core/ids"
	"aego/render/mesh"
	"aego/render/texture"
)

type ArtifactKind string

const (
	ArtTexture      ArtifactKind = "texture"
	ArtAtlasPage    ArtifactKind = "atlaspage"
	ArtSpriteRegion ArtifactKind = "spriteregion"
	ArtScene        ArtifactKind = "scene"
	ArtPrefab       ArtifactKind = "prefab"
	ArtMesh         ArtifactKind = "mesh"
	ArtScript       ArtifactKind = "script"
	ArtData         ArtifactKind = "data"
	ArtTable        ArtifactKind = "table"
	ArtPreset       ArtifactKind = "preset"
	ArtAudio        ArtifactKind = "audio"
	ArtAnim         ArtifactKind = "anim"
	ArtCtrl         ArtifactKind = "animctrl"
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

type DataSource interface {
	Data(id ids.AssetID) ([]byte, bool)
	Table(id ids.AssetID) ([]byte, bool)
}
