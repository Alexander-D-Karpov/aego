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
	"aego/render/texture"
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
	opaque  bool
	texture gpu.TextureHandle
	sampler gpu.SamplerHandle
	mat     *material.Material
	clip    mathx.Rect
	light   uint32
	blend   BlendMode
	scissor bool
}

// Lighting bindings are interned only for Lit draws, keeping handles out of sortable state to preserve Unlit memory traffic.
// UV mappings stay per-instance without splitting compatible neighbours; IDs are frame-local and survive slice growth during Push.
type lightingState struct {
	normal, emission gpu.TextureHandle
	set              uint32
	mode             texture.EmissionMode
}

type Batch struct {
	Opaque       bool
	LightSet     uint32
	First        int
	Count        int
	Texture      gpu.TextureHandle
	Normal       gpu.TextureHandle
	Emission     gpu.TextureHandle
	EmissionMode texture.EmissionMode
	Sampler      gpu.SamplerHandle
	Blend        BlendMode
	Clip         mathx.Rect
	Scissor      bool
	Material     *material.Material
	LaneFirst    int
	TrimFirst    int
	AuxFirst     int
}

// BatchBreaks assigns each state boundary its first incompatible resource.
// Pipeline includes program, variant and blend; Texture includes sampler.
type BatchBreaks struct{ Material, Texture, Pipeline, Clip, Lighting uint32 }

func (b *Batcher) batchBreak(a, next state) BatchBreaks {
	if b.lighting(a.light).set != b.lighting(next.light).set {
		return BatchBreaks{Lighting: 1}
	}
	if a.blend != next.blend || (a.mat == nil) != (next.mat == nil) {
		return BatchBreaks{Pipeline: 1}
	}
	if a.mat != nil && next.mat != nil && (a.mat.Shader() != next.mat.Shader() || a.mat.Mask() != next.mat.Mask()) {
		return BatchBreaks{Pipeline: 1}
	}
	if a.mat != next.mat {
		return BatchBreaks{Material: 1}
	}
	if a.texture != next.texture || a.light != next.light || a.sampler != next.sampler {
		return BatchBreaks{Texture: 1}
	}
	return BatchBreaks{Clip: 1}
}

type Batcher struct {
	profile             []*SubmissionStats
	streams             []profileStreams
	drawn               []*SubmissionStats
	lightingStates      []lightingState
	lightingIndex       map[lightingState]uint32
	lastLighting        lightingState
	lastLightingID      uint32
	aux                 []Aux
	auxIdx, sortedAux   []int32
	auxOut              []Aux
	auxVB               gpu.BufferHandle
	auxCap              int
	trims               [][4]float32
	trimIdx, sortedTrim []int32
	trimOut             [][4]float32
	trimVB              gpu.BufferHandle
	trimCap             int
	breaks              BatchBreaks
	dev                 gpu.Device
	max                 int
	vb                  gpu.BufferHandle
	inst                []Instance
	meta                []state
	keys                []uint64
	scratch             []uint64
	sorted              []Instance
	sortedMeta          []state
	sortedN             int
	batches             []Batch
	zMin, zMax          float32
	ppu                 float32
	dropped             uint32
	clamped             uint32
	// limit is how far the batcher grows when a frame needs more than max sprites.
	limit int
	// lanes hold the @instance values of sprites whose material has them, compacted per batch into laneOut.
	lanes      [][16]float32
	laneIdx    []int32
	sortedLane []int32
	laneOut    [][16]float32
	lvb        gpu.BufferHandle
	laneCap    int
	sequence   []uint32
	sortKeys   []uint64
	sortedKeys []uint64
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
		sequence:   make([]uint32, 0, maxSprites),
		sortKeys:   make([]uint64, maxSprites),
		sortedLane: make([]int32, maxSprites),
		trimIdx:    make([]int32, 0, maxSprites), sortedTrim: make([]int32, maxSprites),
		auxIdx: make([]int32, 0, maxSprites), sortedAux: make([]int32, maxSprites),
		zMax:  1,
		ppu:   1,
		limit: maxSprites,
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
	b.sortedTrim = make([]int32, n)
	b.sortedAux = make([]int32, n)
	b.sortKeys = make([]uint64, n)
	return true
}

func (b *Batcher) Buffer() gpu.BufferHandle {
	return b.vb
}

func (b *Batcher) Reset() {
	clear(b.profile)
	b.profile = b.profile[:0]
	clear(b.streams)
	b.streams = b.streams[:0]
	clear(b.drawn)
	b.drawn = b.drawn[:0]
	b.lightingStates = b.lightingStates[:0]
	clear(b.lightingIndex)
	b.lastLightingID = 0
	b.inst = b.inst[:0]
	b.meta = b.meta[:0]
	b.keys = b.keys[:0]
	b.batches = b.batches[:0]
	b.lanes = b.lanes[:0]
	b.laneIdx = b.laneIdx[:0]
	b.trims = b.trims[:0]
	b.trimIdx = b.trimIdx[:0]
	b.trimOut = b.trimOut[:0]
	b.aux = b.aux[:0]
	b.auxIdx = b.auxIdx[:0]
	b.auxOut = b.auxOut[:0]
	b.sequence = b.sequence[:0]
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
	return b.PushOrdered(d, defaultSampler, clip, scissor, uint32(len(b.inst)))
}

// PushOrdered preserves a shared submission order across sprites and Mesh2D.
func (b *Batcher) PushOrdered(d *Draw, defaultSampler gpu.SamplerHandle, clip mathx.Rect, scissor bool, sequence uint32) bool {
	if len(b.inst) >= b.max && !b.grow() {
		b.dropped++
		return false
	}
	if OutOfRange(d.Layer, d.Order, d.Z, b.zMin, b.zMax) {
		b.clamped++
	}
	geometry := d
	withAux := d.HasLinear || d.Lighting
	if withAux {
		// The complete scale/rotation is now in Aux.Affine. Keep the quad's
		// dimensions local so neither geometry nor normals apply it twice.
		copyDraw := *d
		copyDraw.Scale = mathx.Vec2{X: 1, Y: 1}
		copyDraw.HasLinear = false
		copyDraw.Rotation = 0
		geometry = &copyDraw
	}
	reg, size, origin := geometry.Region, geometry.WorldSize(b.ppu), geometry.Origin
	trimmed := reg.Trimmed()
	if trimmed {
		reg, size, origin = geometry.Geometry(b.ppu)
	} else {
		if d.FlipX {
			reg = reg.FlippedX()
		}
		if d.FlipY {
			reg = reg.FlippedY()
		}
	}
	sin, cos := float32(0), float32(1)
	if geometry.Rotation != 0 {
		sin, cos = mathx.Sin(geometry.Rotation), mathx.Cos(geometry.Rotation)
	}
	trimIndex := int32(-1)
	coverage := d.PixelCoverage && d.Material != nil && d.Material.Shader() != nil && d.Material.Shader().Features&shader.FeaturePixelCoverage != 0
	if d.Material != nil && (trimmed || coverage) {
		trimIndex = int32(len(b.trims))
		rect := reg.LocalRect()
		if coverage {
			// The logical region is always in [0,1]. Tag one coordinate in the
			// existing optional stream, preserving all four values for the shader.
			rect[0] += 2
		}
		b.trims = append(b.trims, rect)
	}
	at := len(b.inst)
	auxIndex := int32(-1)
	if withAux {
		auxIndex = int32(len(b.aux))
		b.aux = append(b.aux, makeAux(d))
	}
	if auxIndex >= 0 || len(b.auxIdx) > 0 {
		for len(b.auxIdx) < at {
			b.auxIdx = append(b.auxIdx, -1)
		}
		b.auxIdx = append(b.auxIdx, auxIndex)
	}
	if trimIndex >= 0 || len(b.trimIdx) > 0 {
		for len(b.trimIdx) < at {
			b.trimIdx = append(b.trimIdx, -1)
		}
		b.trimIdx = append(b.trimIdx, trimIndex)
	}
	sampler := d.Sampler
	if sampler.IsZero() {
		sampler = defaultSampler
	}
	light := uint32(0)
	if d.Lighting || d.LightSet != 0 {
		light = b.internLighting(d)
	}
	b.inst = append(b.inst, Instance{
		Pos:    d.Position,
		Size:   size,
		SinCos: mathx.Vec2{X: sin, Y: cos},
		Origin: origin,
		UVMin:  reg.UVMin,
		UVMax:  reg.UVMax,
		Color:  d.tint().Premultiplied().PackRGBA8(),
	})
	if d.Profile != nil {
		for len(b.profile) < len(b.inst)-1 {
			b.profile = append(b.profile, nil)
		}
		b.profile = append(b.profile, d.Profile)
	}
	opaque := false
	if m := d.Material; m != nil && m.Shader() != nil && m.Shader().OpaqueWithOpaqueAlbedo && m.Blend() == shader.BlendPremultiplied && d.Tint.A >= 1 {
		if tracker, ok := b.dev.(interface {
			OpaqueRegion(gpu.TextureHandle, float32, float32, float32, float32) bool
		}); ok {
			opaque = tracker.OpaqueRegion(reg.Texture, reg.UVMin.X, reg.UVMin.Y, reg.UVMax.X, reg.UVMax.Y)
		}
	}
	b.meta = append(b.meta, state{
		opaque:  opaque,
		light:   light,
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
	if sequence != uint32(at) || len(b.sequence) > 0 {
		for len(b.sequence) < at {
			b.sequence = append(b.sequence, uint32(len(b.sequence)))
		}
		b.sequence = append(b.sequence, sequence)
	}
	b.keys = append(b.keys, uint64(MakeKey(d.Layer, d.Order, d.Z, b.zMin, b.zMax, uint32(len(b.inst)-1))))
	return true
}

func (b *Batcher) internLighting(d *Draw) uint32 {
	if !d.Lighting && d.LightSet == 0 {
		return 0
	}
	s := lightingState{set: d.LightSet}
	if d.Lighting {
		s.normal = d.Normal.Texture
		if d.Emission.Valid() {
			s.emission, s.mode = d.Emission.Region.Texture, d.Emission.Mode
		}
	}
	if b.lastLightingID != 0 && b.lastLighting == s {
		return b.lastLightingID
	}
	id := b.lightingIndex[s]
	if id == 0 {
		if b.lightingIndex == nil {
			b.lightingIndex = make(map[lightingState]uint32)
		}
		b.lightingStates = append(b.lightingStates, s)
		id = uint32(len(b.lightingStates))
		b.lightingIndex[s] = id
	}
	b.lastLighting, b.lastLightingID = s, id
	return id
}

func (b *Batcher) lighting(id uint32) lightingState {
	if id == 0 {
		return lightingState{}
	}
	return b.lightingStates[id-1]
}

func (b *Batcher) Sort() {
	n := len(b.keys)
	if n == 0 {
		b.sortedN = 0
		return
	}
	sorted := bits.RadixSort64(b.keys, b.scratch[:n])
	b.sortedKeys = sorted
	for i, k := range sorted {
		src := SortKey(k).Submission()
		b.sorted[i] = b.inst[src]
		b.sortedMeta[i] = b.meta[src]
		b.sortedLane[i] = b.laneIdx[src]
	}
	if len(b.trims) > 0 {
		for i, k := range sorted {
			src := SortKey(k).Submission()
			b.sortedTrim[i] = b.trimIdx[src]
		}
	}
	if len(b.aux) > 0 {
		for i, k := range sorted {
			src := SortKey(k).Submission()
			b.sortedAux[i] = b.auxIdx[src]
		}
	}
	if len(b.sequence) > 0 {
		for i, k := range sorted {
			src := SortKey(k).Submission()
			b.sortKeys[i] = (k &^ uint64(MaxSubmission)) | uint64(b.sequence[src]&MaxSubmission)
		}
	}
	b.sortedN = n
}

func (b *Batcher) Build() []Batch {
	n := b.sortedN
	b.batches = b.batches[:0]
	b.laneOut = b.laneOut[:0]
	b.trimOut = b.trimOut[:0]
	b.auxOut = b.auxOut[:0]
	b.breaks = BatchBreaks{}
	if n == 0 {
		return b.batches
	}
	cur := b.sortedMeta[0]
	first := 0
	for i := 1; i <= n; i++ {
		if i < n && b.sortedMeta[i] == cur {
			continue
		}
		trimFirst := -1
		if len(b.trims) > 0 {
			trimFirst = b.collectTrim(cur.mat, first, i)
		}
		light := b.lighting(cur.light)
		b.batches = append(b.batches, Batch{
			LightSet:     light.set,
			First:        first,
			Count:        i - first,
			Texture:      cur.texture,
			Normal:       light.normal,
			Emission:     light.emission,
			EmissionMode: light.mode,
			Sampler:      cur.sampler,
			Blend:        cur.blend,
			Opaque:       cur.opaque,
			Clip:         cur.clip,
			Scissor:      cur.scissor,
			Material:     cur.mat,
			LaneFirst:    b.collectLanes(cur.mat, first, i),
			TrimFirst:    trimFirst,
			AuxFirst:     b.collectAux(first, i),
		})
		if i < n {
			x := b.batchBreak(cur, b.sortedMeta[i])
			b.breaks.Material += x.Material
			b.breaks.Texture += x.Texture
			b.breaks.Pipeline += x.Pipeline
			b.breaks.Clip += x.Clip
			b.breaks.Lighting += x.Lighting
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
	if !b.auxVB.IsZero() {
		b.dev.DestroyBuffer(b.auxVB)
	}
	if !b.trimVB.IsZero() {
		b.dev.DestroyBuffer(b.trimVB)
	}
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
		l.Attrs = append(l.Attrs, gpu.VertexAttr{Location: uint8(shader.LaneLocation + i), Slot: shader.LaneSlot, Format: gpu.VFUInt4, Offset: uint16(16 * i)})
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
		return gpu.BlendAdditive()
	}
	return gpu.BlendPremultiplied()
}

func ValidateMode(m BlendMode) error {
	if m >= blendCount {
		return errs.New(errs.GPUResource, "unknown blend mode", log.F("mode", uint8(m)))
	}
	return nil
}

// Key is the painter key of a sorted sprite, with the view submission sequence.
func (b *Batcher) Key(i int) uint64 {
	if len(b.sequence) == 0 {
		return b.sortedKeys[i]
	}
	return b.sortKeys[i]
}

func (b *Batcher) Breaks() BatchBreaks { return b.breaks }
func (b *Batcher) LaneBytes() int      { return len(b.laneOut) * shader.LaneStride }

func (b *Batcher) collectTrim(m *material.Material, from, to int) int {
	if m == nil || len(b.trims) == 0 {
		return -1
	}
	found := false
	for i := from; i < to; i++ {
		if b.sortedTrim[i] >= 0 {
			found = true
			break
		}
	}
	if !found {
		return -1
	}
	start := len(b.trimOut)
	for i := from; i < to; i++ {
		v := [4]float32{}
		if j := b.sortedTrim[i]; j >= 0 {
			v = b.trims[j]
		}
		b.trimOut = append(b.trimOut, v)
	}
	return start
}

func (b *Batcher) UploadTrim() bool {
	n := len(b.trimOut)
	if n == 0 {
		return true
	}
	if n > b.trimCap {
		c := max(n, 256, b.trimCap*2)
		h, err := b.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferVertex, Size: c * shader.TrimStride, Dynamic: true, Label: "sprite.trim"})
		if err != nil {
			return false
		}
		if !b.trimVB.IsZero() {
			b.dev.DestroyBuffer(b.trimVB)
		}
		b.trimVB, b.trimCap = h, c
	}
	raw := unsafe.Slice((*byte)(unsafe.Pointer(&b.trimOut[0])), n*shader.TrimStride)
	b.dev.UpdateBuffer(b.trimVB, 0, raw)
	return true
}
func (b *Batcher) TrimBuffer() gpu.BufferHandle { return b.trimVB }
func (b *Batcher) TrimBytes() int               { return len(b.trimOut) * shader.TrimStride }
