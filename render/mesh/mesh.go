package mesh

import (
	"unsafe"

	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/gpu"
)

const VertexStride = 36

const White uint32 = 0xffffffff

type Vertex struct {
	Pos    mathx.Vec3
	Normal mathx.Vec3
	UV     mathx.Vec2
	Color  uint32
}

const _ = uint(VertexStride - unsafe.Sizeof(Vertex{}))

const _ = uint(unsafe.Sizeof(Vertex{}) - VertexStride)

type Data struct {
	Vertices []Vertex
	Indices  []uint32
	Bounds   mathx.AABB3
}

func (d *Data) ComputeBounds() {
	if len(d.Vertices) == 0 {
		d.Bounds = mathx.AABB3{}
		return
	}
	b := mathx.AABB3{Min: d.Vertices[0].Pos, Max: d.Vertices[0].Pos}
	for i := 1; i < len(d.Vertices); i++ {
		b = b.Expand(d.Vertices[i].Pos)
	}
	d.Bounds = b
}

func (d *Data) Validate() error {
	if len(d.Vertices) == 0 || len(d.Indices) == 0 {
		return errs.New(errs.GPUResource, "mesh has no geometry")
	}
	if len(d.Indices)%3 != 0 {
		return errs.New(errs.GPUResource, "mesh index count is not a multiple of three", log.F("indices", len(d.Indices)))
	}
	n := uint32(len(d.Vertices))
	for i, idx := range d.Indices {
		if idx >= n {
			return errs.New(errs.GPUResource, "mesh index out of range", log.F("at", i), log.F("index", idx), log.F("vertices", n))
		}
	}
	return nil
}

type Mesh struct {
	VB       gpu.BufferHandle
	IB       gpu.BufferHandle
	Count    int
	Vertices int
	Bounds   mathx.AABB3
}

func Upload(dev gpu.Device, d *Data, label string) (*Mesh, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	vraw := unsafe.Slice((*byte)(unsafe.Pointer(&d.Vertices[0])), len(d.Vertices)*VertexStride)
	iraw := unsafe.Slice((*byte)(unsafe.Pointer(&d.Indices[0])), len(d.Indices)*4)
	vb, err := dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferVertex, Data: vraw, Label: label + ".vb"})
	if err != nil {
		return nil, err
	}
	ib, err := dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferIndex, IndexFormat: gpu.Index32, Data: iraw, Label: label + ".ib"})
	if err != nil {
		dev.DestroyBuffer(vb)
		return nil, err
	}
	return &Mesh{VB: vb, IB: ib, Count: len(d.Indices), Vertices: len(d.Vertices), Bounds: d.Bounds}, nil
}

func (m *Mesh) Destroy(dev gpu.Device) {
	if m == nil {
		return
	}
	if !m.VB.IsZero() {
		dev.DestroyBuffer(m.VB)
	}
	if !m.IB.IsZero() {
		dev.DestroyBuffer(m.IB)
	}
	*m = Mesh{}
}

func Layout() gpu.VertexLayout {
	return gpu.VertexLayout{
		Attrs: []gpu.VertexAttr{
			{Location: 0, Slot: 0, Format: gpu.VFFloat3, Offset: 0},
			{Location: 1, Slot: 0, Format: gpu.VFFloat3, Offset: 12},
			{Location: 2, Slot: 0, Format: gpu.VFFloat2, Offset: 24},
			{Location: 3, Slot: 0, Format: gpu.VFUByte4Norm, Offset: 32},
		},
		Strides: [gpu.MaxVertexSlots]uint16{VertexStride},
	}
}
