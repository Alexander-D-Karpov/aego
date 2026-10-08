package mesh

import (
	"slices"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render/sprite"
	"aego/render/texture"
)

type Draw struct {
	Mesh        *Mesh
	World       mathx.Mat4
	Texture     texture.Region
	Sampler     gpu.SamplerHandle
	Color       mathx.Color
	Blend       sprite.BlendMode
	DoubleSided bool
	Layer       int16
}

type Item struct {
	Draw
	Dist float32
}

type Batcher struct {
	max     int
	opaque  []Item
	blend   []Item
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
	}
}

func (b *Batcher) Reset() {
	b.opaque = b.opaque[:0]
	b.blend = b.blend[:0]
	b.dropped = 0
}

func (b *Batcher) Len() int {
	return len(b.opaque) + len(b.blend)
}

func (b *Batcher) Dropped() uint32 {
	return b.dropped
}

func (b *Batcher) Push(d *Draw, cam mathx.Vec3, white texture.Region, sampler gpu.SamplerHandle) bool {
	if d.Mesh == nil {
		return false
	}
	if b.Len() >= b.max {
		b.dropped++
		return false
	}
	it := Item{Draw: *d}
	if it.Texture.IsZero() {
		it.Texture = white
	}
	if it.Sampler.IsZero() {
		it.Sampler = sampler
	}
	it.Dist = d.World.TransformPoint(d.Mesh.Bounds.Center()).Sub(cam).LenSq()
	if it.Blend == sprite.BlendOpaque {
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
	if a.Layer != b.Layer {
		return int(a.Layer) - int(b.Layer)
	}
	return cmpDist(b.Dist, a.Dist)
}

func (b *Batcher) Sort() {
	slices.SortFunc(b.opaque, cmpOpaque)
	slices.SortStableFunc(b.blend, cmpBlend)
}

func (b *Batcher) Opaque() []Item {
	return b.opaque
}

func (b *Batcher) Transparent() []Item {
	return b.blend
}
