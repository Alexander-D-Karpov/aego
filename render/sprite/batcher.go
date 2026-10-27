package sprite

import (
	"unsafe"

	"aego/core/bits"
	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/material"
	"aego/render/shader"
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
	mat     *material.Material
}

type Batch struct {
	First     int
	Count     int
	Texture   gpu.TextureHandle
	Sampler   gpu.SamplerHandle
	Blend     BlendMode
	Clip      mathx.Rect
	Scissor   bool
	Material  *material.Material
	LaneFirst int
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
	// limit is how far the batcher grows when a frame needs more than max sprites.
	limit int
	// lanes hold the @instance values of sprites whose material has them, compacted per batch into laneOut.
	lanes      [][16]float32
	laneIdx    []int32
	sortedLane []int32
	laneOut    [][16]float32
	lvb        gpu.BufferHandle
	laneCap    int
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
		laneIdx:    make([]int32, 0, maxSprites),
		sortedLane: make([]int32, maxSprites),
		zMax:       1,
		ppu:        1,
		limit:      maxSprites,
	}, nil
}

// SetLimit lets the batcher grow up to n sprites a frame instead of dropping the ones past its size.
func (b *Batcher) SetLimit(n int) {
	b.limit = min(max(n, b.max), MaxSubmission)
}

// grow doubles the buffers; it allocates only when a frame needs more than any frame before it.
func (b *Batcher) grow() bool {
	n := min(b.max*2, b.limit)
	if n <= b.max {
		return false
	}
	vb, err := b.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferVertex, Size: n * InstanceStride, Dynamic: true, Label: "sprite.instances"})
	if err != nil {
		return false
	}
	b.dev.DestroyBuffer(b.vb)
	b.vb, b.max = vb, n
	b.scratch = make([]uint64, n)
	b.sorted = make([]Instance, n)
	b.sortedMeta = make([]state, n)
	b.sortedLane = make([]int32, n)
	return true
}

func (b *Batcher) Buffer() gpu.BufferHandle {
	return b.vb
}

func (b *Batcher) Reset() {
	b.inst = b.inst[:0]
	b.meta = b.meta[:0]
	b.keys = b.keys[:0]
	b.batches = b.batches[:0]
	b.lanes = b.lanes[:0]
	b.laneIdx = b.laneIdx[:0]
	b.laneOut = b.laneOut[:0]
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
	if len(b.inst) >= b.max && !b.grow() {
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
		mat:     d.Material,
	})
	idx := int32(-1)
	if m := d.Material; m != nil && m.Lanes() > 0 {
		idx = int32(len(b.lanes))
		if d.Lanes != nil {
			b.lanes = append(b.lanes, *d.Lanes)
		} else {
			b.lanes = append(b.lanes, *m.DefaultLanes())
		}
	}
	b.laneIdx = append(b.laneIdx, idx)
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
		b.sortedLane[i] = b.laneIdx[src]
	}
	b.sortedN = n
}

func (b *Batcher) Build() []Batch {
	n := b.sortedN
	b.batches = b.batches[:0]
	b.laneOut = b.laneOut[:0]
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
			First:     first,
			Count:     i - first,
			Texture:   cur.texture,
			Sampler:   cur.sampler,
			Blend:     cur.blend,
			Clip:      cur.clip,
			Scissor:   cur.scissor,
			Material:  cur.mat,
			LaneFirst: b.collectLanes(cur.mat, first, i),
		})
		if i < n {
			cur = b.sortedMeta[i]
			first = i
		}
	}
	return b.batches
}

func (b *Batcher) collectLanes(m *material.Material, from, to int) int {
	if m == nil || m.Lanes() == 0 {
		return -1
	}
	at := len(b.laneOut)
	for j := from; j < to; j++ {
		b.laneOut = append(b.laneOut, b.lanes[b.sortedLane[j]])
	}
	return at
}

// UploadLanes writes the batches' lanes; false means the lane buffer could not grow.
func (b *Batcher) UploadLanes() bool {
	n := len(b.laneOut)
	if n == 0 {
		return true
	}
	if n > b.laneCap {
		c := max(n, 256, b.laneCap*2)
		vb, err := b.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferVertex, Size: c * shader.LaneStride, Dynamic: true, Label: "sprite.lanes"})
		if err != nil {
			return false
		}
		if !b.lvb.IsZero() {
			b.dev.DestroyBuffer(b.lvb)
		}
		b.lvb, b.laneCap = vb, c
	}
	raw := unsafe.Slice((*byte)(unsafe.Pointer(&b.laneOut[0])), n*shader.LaneStride)
	b.dev.UpdateBuffer(b.lvb, 0, raw)
	return true
}

func (b *Batcher) LaneBuffer() gpu.BufferHandle {
	return b.lvb
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
	if !b.lvb.IsZero() {
		b.dev.DestroyBuffer(b.lvb)
	}
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

// LaneLayout is Layout plus a material's @instance lanes in slot LaneSlot; Layout returns a fresh Attrs slice, so the append is safe.
func LaneLayout(lanes int) gpu.VertexLayout {
	l := Layout()
	if lanes == 0 {
		return l
	}
	for i := 0; i < lanes; i++ {
		l.Attrs = append(l.Attrs, gpu.VertexAttr{Location: uint8(shader.LaneLocation + i), Slot: shader.LaneSlot, Format: gpu.VFFloat4, Offset: uint16(16 * i)})
	}
	l.Strides[shader.LaneSlot] = shader.LaneStride
	l.PerInstance[shader.LaneSlot] = true
	return l
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
