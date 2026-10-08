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
