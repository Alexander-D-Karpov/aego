package sceneload

import (
	"aego/asset"
	"aego/core/ids"
	"aego/render/texture"
)

type stubResolver struct {
	region texture.Region
}

func (s *stubResolver) Texture(ids.AssetID) (texture.Region, bool) {
	return s.region, !s.region.IsZero()
}

func (s *stubResolver) Scene(ids.AssetID) ([]byte, bool) {
	return nil, false
}

func (s *stubResolver) Prefab(ids.AssetID) ([]byte, bool) {
	return nil, false
}

func (s *stubResolver) Generation() uint32 {
	return 1
}

func (s *stubResolver) Subscribe(func(asset.Reloaded)) func() {
	return func() {}
}
