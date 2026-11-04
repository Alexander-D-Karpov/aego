package asset

import (
	"aego/core/handle"
	"aego/core/ids"
	"aego/render/material"
	"aego/render/mesh"
	"aego/render/texture"
)

type assetTag struct{}

type Handle = handle.Handle[assetTag]

type Texture struct{}

type Scene struct{}

type Prefab struct{}

type Mesh struct{}

// Material and Shader are source-independent asset reference markers.
type Material struct{}
type Shader struct{}

type Kind interface {
	Texture | Scene | Prefab | Mesh | Material | Shader
}

func kindName[T Kind]() string {
	var zero T
	switch any(zero).(type) {
	case Texture:
		return "texture"
	case Scene:
		return "scene"
	case Prefab:
		return "prefab"
	case Mesh:
		return "mesh"
	case Material:
		return "material"
	case Shader:
		return "shader"
	}
	return "unknown"
}

type Ref[T Kind] struct {
	ID     ids.AssetID
	region texture.Region
	gen    uint32
	ok     bool
}

func NewRef[T Kind](id ids.AssetID) Ref[T] {
	return Ref[T]{ID: id}
}

func (r *Ref[T]) AssetRefID() ids.AssetID {
	return r.ID
}

func (r *Ref[T]) SetAssetRefID(id ids.AssetID) {
	if r.ID == id {
		return
	}
	r.ID = id
	r.region = texture.Region{}
	r.gen = 0
	r.ok = false
}

func (r *Ref[T]) AssetKind() string {
	return kindName[T]()
}

func (r *Ref[T]) IsZero() bool {
	return r.ID.IsZero()
}

func (r *Ref[T]) Region(res Resolver) (texture.Region, bool) {
	if res == nil || r.ID.IsZero() {
		return texture.Region{}, false
	}
	if gen := res.Generation(); r.ok && r.gen == gen {
		return r.region, true
	}
	reg, ok := res.Texture(r.ID)
	if !ok {
		r.ok = false
		return texture.Region{}, false
	}
	r.region = reg
	r.gen = res.Generation()
	r.ok = true
	return reg, true
}

func (r *Ref[T]) Mesh(res Resolver) (*mesh.Mesh, bool) {
	if res == nil || r.ID.IsZero() {
		return nil, false
	}
	return res.Mesh(r.ID)
}

// Material resolves a prepared runtime material. The reference retains its ID
// when the asset is missing or its current import is invalid.
func (r *Ref[T]) Material(res Resolver) (*material.Material, bool) {
	if res == nil || r.ID.IsZero() {
		return nil, false
	}
	source, ok := res.(MaterialSource)
	if !ok {
		return nil, false
	}
	return source.Material(r.ID)
}
