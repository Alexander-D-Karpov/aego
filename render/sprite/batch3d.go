package sprite

import (
	"slices"
	"unsafe"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render/texture"
)

const Instance3DStride = 56

type Instance3D struct {
	Pos   mathx.Vec3
	Right mathx.Vec3
	Down  mathx.Vec3
	UVMin mathx.Vec2
	UVMax mathx.Vec2
	Color uint32
}

const _ = uint(Instance3DStride - unsafe.Sizeof(Instance3D{}))

const _ = uint(unsafe.Sizeof(Instance3D{}) - Instance3DStride)

type Draw3D struct {
	Region  texture.Region
	World   mathx.Mat4
	Size    mathx.Vec2
	Origin  mathx.Vec2
	Tint    mathx.Color
	Layer   int16
	Order   int16
	Blend   BlendMode
	Sampler gpu.SamplerHandle
	FlipX   bool
	FlipY   bool
}

func (d *Draw3D) Instance() Instance3D {
	size := d.Size
	if size.X == 0 && size.Y == 0 {
		size = d.Region.Size()
	}
	reg := d.Region
	if d.FlipX {
		reg = reg.FlippedX()
	}
	if d.FlipY {
		reg = reg.FlippedY()
	}
	tint := d.Tint
	if tint == (mathx.Color{}) {
		tint = mathx.Color{R: 1, G: 1, B: 1, A: 1}
	}
	w := d.World
	return Instance3D{
		Pos:   w.TransformPoint(mathx.Vec3{X: -d.Origin.X * size.X, Y: d.Origin.Y * size.Y}),
		Right: w.TransformDir(mathx.Vec3{X: size.X}),
		Down:  w.TransformDir(mathx.Vec3{Y: -size.Y}),
		UVMin: reg.UVMin,
		UVMax: reg.UVMax,
		Color: tint.Premultiplied().PackRGBA8(),
	}
}

func (i *Instance3D) Center() mathx.Vec3 {
	return i.Pos.Add(i.Right.Scale(0.5)).Add(i.Down.Scale(0.5))
}

func (i *Instance3D) Bounds() mathx.AABB3 {
	b := mathx.AABB3{Min: i.Pos, Max: i.Pos}
	b = b.Expand(i.Pos.Add(i.Right))
	b = b.Expand(i.Pos.Add(i.Down))
	return b.Expand(i.Pos.Add(i.Right).Add(i.Down))
}

type key3 struct {
	layer int16
	order int16
	dist  float32
	idx   int32
}

type Batcher3D struct {
	dev        gpu.Device
	max        int
	vb         gpu.BufferHandle
	inst       []Instance3D
	meta       []state
	keys       []key3
	sorted     []Instance3D
	sortedMeta []state
	sortedN    int
	batches    []Batch
	dropped    uint32
}

func NewBatcher3D(dev gpu.Device, maxSprites int) (*Batcher3D, error) {
	if maxSprites < 1 {
		maxSprites = 1
	}
	vb, err := dev.CreateBuffer(gpu.BufferDesc{
		Usage:   gpu.BufferVertex,
		Size:    maxSprites * Instance3DStride,
		Dynamic: true,
		Label:   "sprite3d.instances",
	})
	if err != nil {
		return nil, err
	}
	return &Batcher3D{
		dev:        dev,
		max:        maxSprites,
		vb:         vb,
		inst:       make([]Instance3D, 0, maxSprites),
		meta:       make([]state, 0, maxSprites),
		keys:       make([]key3, 0, maxSprites),
		sorted:     make([]Instance3D, maxSprites),
		sortedMeta: make([]state, maxSprites),
		batches:    make([]Batch, 0, 32),
	}, nil
}

func (b *Batcher3D) Buffer() gpu.BufferHandle {
	return b.vb
}

func (b *Batcher3D) Reset() {
	b.inst = b.inst[:0]
	b.meta = b.meta[:0]
	b.keys = b.keys[:0]
	b.batches = b.batches[:0]
	b.sortedN = 0
	b.dropped = 0
}

func (b *Batcher3D) Len() int {
	return len(b.inst)
}

func (b *Batcher3D) Dropped() uint32 {
	return b.dropped
}

func (b *Batcher3D) Push(inst Instance3D, d *Draw3D, defaultSampler gpu.SamplerHandle, cam mathx.Vec3) bool {
	if len(b.inst) >= b.max {
		b.dropped++
		return false
	}
	sampler := d.Sampler
	if sampler.IsZero() {
		sampler = defaultSampler
	}
	idx := len(b.inst)
	b.inst = append(b.inst, inst)
	b.meta = append(b.meta, state{texture: d.Region.Texture, sampler: sampler, blend: d.Blend})
	b.keys = append(b.keys, key3{layer: d.Layer, order: d.Order, dist: inst.Center().Sub(cam).LenSq(), idx: int32(idx)})
	return true
}

func cmpKey3(a, b key3) int {
	if a.layer != b.layer {
		return int(a.layer) - int(b.layer)
	}
	if a.order != b.order {
		return int(a.order) - int(b.order)
	}
	switch {
	case a.dist > b.dist:
		return -1
	case a.dist < b.dist:
		return 1
	}
	return int(a.idx - b.idx)
}

func (b *Batcher3D) Sort() {
	slices.SortFunc(b.keys, cmpKey3)
	for i, k := range b.keys {
		b.sorted[i] = b.inst[k.idx]
		b.sortedMeta[i] = b.meta[k.idx]
	}
	b.sortedN = len(b.keys)
}

func (b *Batcher3D) Build() []Batch {
	n := b.sortedN
	b.batches = b.batches[:0]
	if n == 0 {
		return b.batches
	}
	cur, first := b.sortedMeta[0], 0
	for i := 1; i <= n; i++ {
		if i < n && b.sortedMeta[i] == cur {
			continue
		}
		b.batches = append(b.batches, Batch{First: first, Count: i - first, Texture: cur.texture, Sampler: cur.sampler, Blend: cur.blend})
		if i < n {
			cur, first = b.sortedMeta[i], i
		}
	}
	return b.batches
}

func (b *Batcher3D) Upload() int {
	n := b.sortedN
	if n == 0 {
		return 0
	}
	raw := unsafe.Slice((*byte)(unsafe.Pointer(&b.sorted[0])), n*Instance3DStride)
	b.dev.UpdateBuffer(b.vb, 0, raw)
	return n * Instance3DStride
}

func (b *Batcher3D) Instances() []Instance3D {
	return b.sorted[:b.sortedN]
}

func (b *Batcher3D) Close() {
	b.dev.DestroyBuffer(b.vb)
}

func Layout3D() gpu.VertexLayout {
	return gpu.VertexLayout{
		Attrs: []gpu.VertexAttr{
			{Location: 0, Slot: 0, Format: gpu.VFFloat3, Offset: 0},
			{Location: 1, Slot: 0, Format: gpu.VFFloat3, Offset: 12},
			{Location: 2, Slot: 0, Format: gpu.VFFloat3, Offset: 24},
			{Location: 3, Slot: 0, Format: gpu.VFFloat4, Offset: 36},
			{Location: 4, Slot: 0, Format: gpu.VFUByte4Norm, Offset: 52},
		},
		Strides:     [gpu.MaxVertexSlots]uint16{Instance3DStride},
		PerInstance: [gpu.MaxVertexSlots]bool{true},
	}
}
