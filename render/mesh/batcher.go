package mesh

import (
	"slices"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render/material"
	"aego/render/sprite"
	"aego/render/texture"
)

type Draw struct {
	VisibilityLayers uint64
	LightSet         uint32
	Normal           texture.Region
	Emission         texture.Emission
	Mesh             *Mesh
	World            mathx.Mat4
	Texture          texture.Region
	Sampler          gpu.SamplerHandle
	Color            mathx.Color
	Blend            sprite.BlendMode
	DoubleSided      bool
	Layer            int16
	Order            int16
	Z                float32
	Is2D             bool
	Material         *material.Material
	Lanes            *[16]float32
}

type Item struct {
	Draw
	Dist float32
	Key  uint64
}

type Batcher struct {
	max     int
	opaque  []Item
	blend   []Item
	twoD    []Item
	dropped uint32
}

func NewBatcher(maxDraws int) *Batcher {
	if maxDraws < 1 {
		maxDraws = 1
	}
	return &Batcher{
		max:    maxDraws,
		opaque: make([]Item, 0, maxDraws),
		blend:  make([]Item, 0, maxDraws),
		twoD:   make([]Item, 0, maxDraws),
	}
}

func (b *Batcher) Reset() {
	b.opaque = b.opaque[:0]
	b.blend = b.blend[:0]
	b.twoD = b.twoD[:0]
	b.dropped = 0
}

func (b *Batcher) Len() int {
	return len(b.opaque) + len(b.blend) + len(b.twoD)
}

func (b *Batcher) Dropped() uint32 {
	return b.dropped
}

func (b *Batcher) Push(d *Draw, cam mathx.Vec3, white texture.Region, sampler gpu.SamplerHandle) bool {
	return b.PushOrdered(d, cam, white, sampler, 0, 0, 1)
}

func (b *Batcher) PushOrdered(d *Draw, cam mathx.Vec3, white texture.Region, sampler gpu.SamplerHandle, sequence uint32, zMin, zMax float32) bool {
	if d.Mesh == nil {
		return false
	}
	if b.Len() >= b.max {
		b.dropped++
		return false
	}
	it := Item{Draw: *d}
	if it.Material != nil {
		it.Blend = sprite.BlendMode(it.Material.Blend())
	}
	if it.Texture.IsZero() {
		it.Texture = white
	}
	if it.Sampler.IsZero() {
		it.Sampler = sampler
	}
	it.Dist = d.World.TransformPoint(d.Mesh.Bounds.Center()).Sub(cam).LenSq()
	if it.Is2D {
		it.Key = uint64(sprite.MakeKey(it.Layer, it.Order, it.Z, zMin, zMax, sequence))
		b.twoD = append(b.twoD, it)
	} else if it.Blend == sprite.BlendOpaque {
		b.opaque = append(b.opaque, it)
	} else {
		b.blend = append(b.blend, it)
	}
	return true
}

func cmpDist(a, b float32) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpOpaque(a, b Item) int {
	if a.Is2D || b.Is2D {
		return cmp2D(a, b)
	}
	if a.DoubleSided != b.DoubleSided {
		if !a.DoubleSided {
			return -1
		}
		return 1
	}
	ta, tb := a.Texture.Texture, b.Texture.Texture
	if ta.Index != tb.Index {
		return int(ta.Index) - int(tb.Index)
	}
	if ta.Gen != tb.Gen {
		return int(ta.Gen) - int(tb.Gen)
	}
	return cmpDist(a.Dist, b.Dist)
}

func cmpBlend(a, b Item) int {
	if a.Is2D || b.Is2D {
		return cmp2D(a, b)
	}
	if a.Layer != b.Layer {
		return int(a.Layer) - int(b.Layer)
	}
	return cmpDist(b.Dist, a.Dist)
}

func (b *Batcher) Sort() {
	slices.SortStableFunc(b.opaque, cmpOpaque)
	slices.SortStableFunc(b.blend, cmpBlend)
	slices.SortStableFunc(b.twoD, func(a, b Item) int {
		if a.Key < b.Key {
			return -1
		}
		if a.Key > b.Key {
			return 1
		}
		return 0
	})
}

func (b *Batcher) Opaque() []Item {
	return b.opaque
}

func (b *Batcher) Transparent() []Item {
	return b.blend
}

func cmp2D(a, b Item) int {
	if a.Is2D != b.Is2D {
		if a.Is2D {
			return 1
		}
		return -1
	}
	if a.Layer != b.Layer {
		return int(a.Layer) - int(b.Layer)
	}
	if a.Order != b.Order {
		return int(a.Order) - int(b.Order)
	}
	return cmpDist(a.Z, b.Z)
}

func (b *Batcher) TwoD() []Item { return b.twoD }
