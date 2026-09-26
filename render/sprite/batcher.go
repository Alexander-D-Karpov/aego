package sprite

import (
	"unsafe"

	"aego/core/bits"
	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/gpu"
)

const InstanceStride = 52

type Instance struct {
	Pos    mathx.Vec2
	Size   mathx.Vec2
	SinCos mathx.Vec2
	Origin mathx.Vec2
	UVMin  mathx.Vec2
	UVMax  mathx.Vec2
	Color  uint32
}

const _ = uint(InstanceStride - unsafe.Sizeof(Instance{}))

const _ = uint(unsafe.Sizeof(Instance{}) - InstanceStride)

type state struct {
	texture gpu.TextureHandle
	sampler gpu.SamplerHandle
	blend   BlendMode
	clip    mathx.Rect
	scissor bool
}

type Batch struct {
	First   int
	Count   int
	Texture gpu.TextureHandle
	Sampler gpu.SamplerHandle
	Blend   BlendMode
	Clip    mathx.Rect
	Scissor bool
}

type Batcher struct {
	dev        gpu.Device
	max        int
	vb         gpu.BufferHandle
	inst       []Instance
	meta       []state
	keys       []uint64
	scratch    []uint64
	sorted     []Instance
	sortedMeta []state
	sortedN    int
	batches    []Batch
	zMin, zMax float32
	ppu        float32
	dropped    uint32
	clamped    uint32
}

func NewBatcher(dev gpu.Device, maxSprites int) (*Batcher, error) {
	if maxSprites < 1 {
		maxSprites = 1
	}
	if maxSprites > MaxSubmission {
		maxSprites = MaxSubmission
	}
	vb, err := dev.CreateBuffer(gpu.BufferDesc{
		Usage:   gpu.BufferVertex,
		Size:    maxSprites * InstanceStride,
		Dynamic: true,
		Label:   "sprite.instances",
	})
	if err != nil {
		return nil, err
	}
	return &Batcher{
		dev:        dev,
		max:        maxSprites,
		vb:         vb,
		inst:       make([]Instance, 0, maxSprites),
		meta:       make([]state, 0, maxSprites),
		keys:       make([]uint64, 0, maxSprites),
		scratch:    make([]uint64, maxSprites),
		sorted:     make([]Instance, maxSprites),
		sortedMeta: make([]state, maxSprites),
		batches:    make([]Batch, 0, 64),
		zMax:       1,
		ppu:        1,
	}, nil
}

func (b *Batcher) Buffer() gpu.BufferHandle {
	return b.vb
}

func (b *Batcher) Reset() {
	b.inst = b.inst[:0]
	b.meta = b.meta[:0]
	b.keys = b.keys[:0]
	b.batches = b.batches[:0]
	b.sortedN = 0
	b.dropped = 0
	b.clamped = 0
}

func (b *Batcher) SetView(zMin, zMax, pixelsPerUnit float32) {
	b.zMin, b.zMax = zMin, zMax
	b.ppu = pixelsPerUnit
}

func (b *Batcher) Len() int {
	return len(b.inst)
}

func (b *Batcher) Dropped() uint32 {
	return b.dropped
}

func (b *Batcher) Clamped() uint32 {
	return b.clamped
}

func (b *Batcher) Push(d *Draw, defaultSampler gpu.SamplerHandle, clip mathx.Rect, scissor bool) bool {
	if len(b.inst) >= b.max {
		b.dropped++
		return false
	}
	if OutOfRange(d.Layer, d.Order, d.Z, b.zMin, b.zMax) {
		b.clamped++
	}
	size := d.WorldSize(b.ppu)
	sin, cos := mathx.Sin(d.Rotation), mathx.Cos(d.Rotation)
	reg := d.Region
	if d.FlipX {
		reg = reg.FlippedX()
	}
	if d.FlipY {
		reg = reg.FlippedY()
	}
	sampler := d.Sampler
	if sampler.IsZero() {
		sampler = defaultSampler
	}
	b.inst = append(b.inst, Instance{
		Pos:    d.Position,
		Size:   size,
		SinCos: mathx.Vec2{X: sin, Y: cos},
		Origin: d.Origin,
		UVMin:  reg.UVMin,
		UVMax:  reg.UVMax,
		Color:  d.tint().Premultiplied().PackRGBA8(),
	})
	b.meta = append(b.meta, state{
		texture: reg.Texture,
		sampler: sampler,
		blend:   d.Blend,
		clip:    clip,
		scissor: scissor,
	})
	b.keys = append(b.keys, uint64(MakeKey(d.Layer, d.Order, d.Z, b.zMin, b.zMax, uint32(len(b.inst)-1))))
	return true
}

func (b *Batcher) Sort() {
	n := len(b.keys)
	if n == 0 {
		b.sortedN = 0
		return
	}
	sorted := bits.RadixSort64(b.keys, b.scratch[:n])
	for i, k := range sorted {
		src := SortKey(k).Submission()
		b.sorted[i] = b.inst[src]
		b.sortedMeta[i] = b.meta[src]
	}
	b.sortedN = n
}

func (b *Batcher) Build() []Batch {
	n := b.sortedN
	b.batches = b.batches[:0]
	if n == 0 {
		return b.batches
	}
	cur := b.sortedMeta[0]
	first := 0
	for i := 1; i <= n; i++ {
		if i < n && b.sortedMeta[i] == cur {
			continue
		}
		b.batches = append(b.batches, Batch{
			First:   first,
			Count:   i - first,
			Texture: cur.texture,
			Sampler: cur.sampler,
			Blend:   cur.blend,
			Clip:    cur.clip,
			Scissor: cur.scissor,
		})
		if i < n {
			cur = b.sortedMeta[i]
			first = i
		}
	}
	return b.batches
}

func (b *Batcher) Upload() int {
	n := len(b.inst)
	if n == 0 {
		return 0
	}
	raw := unsafe.Slice((*byte)(unsafe.Pointer(&b.sorted[0])), n*InstanceStride)
	b.dev.UpdateBuffer(b.vb, 0, raw)
	return n * InstanceStride
}

func (b *Batcher) Instances() []Instance {
	return b.sorted[:len(b.inst)]
}

func (b *Batcher) Close() {
	b.dev.DestroyBuffer(b.vb)
}

func Layout() gpu.VertexLayout {
	return gpu.VertexLayout{
		Attrs: []gpu.VertexAttr{
			{Location: 0, Slot: 0, Format: gpu.VFFloat4, Offset: 0},
			{Location: 1, Slot: 0, Format: gpu.VFFloat4, Offset: 16},
			{Location: 2, Slot: 0, Format: gpu.VFFloat4, Offset: 32},
			{Location: 3, Slot: 0, Format: gpu.VFUByte4Norm, Offset: 48},
		},
		Strides:     [gpu.MaxVertexSlots]uint16{InstanceStride},
		PerInstance: [gpu.MaxVertexSlots]bool{true},
	}
}

func BlendState(m BlendMode) gpu.BlendState {
	switch m {
	case BlendOpaque:
		return gpu.BlendState{}
	case BlendAdditive:
		return gpu.BlendState{Enabled: true, SrcRGB: gpu.BlendOne, DstRGB: gpu.BlendOne, SrcA: gpu.BlendOne, DstA: gpu.BlendOne}
	}
	return gpu.BlendPremultiplied()
}

func ValidateMode(m BlendMode) error {
	if m >= blendCount {
		return errs.New(errs.GPUResource, "unknown blend mode", log.F("mode", uint8(m)))
	}
	return nil
}
