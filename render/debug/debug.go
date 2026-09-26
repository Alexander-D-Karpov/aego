package debug

import (
	"unsafe"

	"aego/core/mathx"
	"aego/gpu"
)

type Space uint8

const (
	World Space = iota
	Screen
)

const VertexStride = 12

type Vertex struct {
	Pos   mathx.Vec2
	Color uint32
}

const _ = uint(VertexStride - unsafe.Sizeof(Vertex{}))

const _ = uint(unsafe.Sizeof(Vertex{}) - VertexStride)

type Draw struct {
	dev      gpu.Device
	vb       gpu.BufferHandle
	max      int
	world    []Vertex
	screen   []Vertex
	space    Space
	dropped  uint32
	uploaded int
}

func New(dev gpu.Device, maxVerts int) (*Draw, error) {
	if maxVerts < 2 {
		maxVerts = 2
	}
	vb, err := dev.CreateBuffer(gpu.BufferDesc{
		Usage:   gpu.BufferVertex,
		Size:    maxVerts * VertexStride,
		Dynamic: true,
		Label:   "debug.lines",
	})
	if err != nil {
		return nil, err
	}
	return &Draw{
		dev:    dev,
		vb:     vb,
		max:    maxVerts,
		world:  make([]Vertex, 0, maxVerts/2),
		screen: make([]Vertex, 0, maxVerts/2),
	}, nil
}

func (d *Draw) Buffer() gpu.BufferHandle {
	return d.vb
}

func (d *Draw) Reset() {
	d.world = d.world[:0]
	d.screen = d.screen[:0]
	d.space = World
	d.dropped = 0
	d.uploaded = 0
}

func (d *Draw) Space(s Space) {
	d.space = s
}

func (d *Draw) Dropped() uint32 {
	return d.dropped
}

func (d *Draw) Counts() (world, screen int) {
	return len(d.world), len(d.screen)
}

func (d *Draw) push(a, b mathx.Vec2, c mathx.Color) {
	if len(d.world)+len(d.screen)+2 > d.max {
		d.dropped += 2
		return
	}
	packed := c.Premultiplied().PackRGBA8()
	if d.space == Screen {
		d.screen = append(d.screen, Vertex{Pos: a, Color: packed}, Vertex{Pos: b, Color: packed})
		return
	}
	d.world = append(d.world, Vertex{Pos: a, Color: packed}, Vertex{Pos: b, Color: packed})
}

func (d *Draw) Line(a, b mathx.Vec2, c mathx.Color) {
	d.push(a, b, c)
}

func (d *Draw) Rect(r mathx.Rect, c mathx.Color) {
	p0 := mathx.Vec2{X: r.X, Y: r.Y}
	p1 := mathx.Vec2{X: r.X + r.W, Y: r.Y}
	p2 := mathx.Vec2{X: r.X + r.W, Y: r.Y + r.H}
	p3 := mathx.Vec2{X: r.X, Y: r.Y + r.H}
	d.push(p0, p1, c)
	d.push(p1, p2, c)
	d.push(p2, p3, c)
	d.push(p3, p0, c)
}

func (d *Draw) AABB(b mathx.AABB, c mathx.Color) {
	size := b.Size()
	d.Rect(mathx.Rect{X: b.Min.X, Y: b.Min.Y, W: size.X, H: size.Y}, c)
}

func (d *Draw) RectRot(center, size mathx.Vec2, rad float32, c mathx.Color) {
	hx, hy := size.X/2, size.Y/2
	s, co := mathx.Sin(rad), mathx.Cos(rad)
	rot := func(x, y float32) mathx.Vec2 {
		return mathx.Vec2{X: center.X + x*co - y*s, Y: center.Y + x*s + y*co}
	}
	p0 := rot(-hx, -hy)
	p1 := rot(hx, -hy)
	p2 := rot(hx, hy)
	p3 := rot(-hx, hy)
	d.push(p0, p1, c)
	d.push(p1, p2, c)
	d.push(p2, p3, c)
	d.push(p3, p0, c)
}

func (d *Draw) Circle(center mathx.Vec2, radius float32, segments int, c mathx.Color) {
	if segments < 3 {
		segments = 3
	}
	step := mathx.TwoPi / float32(segments)
	prev := mathx.Vec2{X: center.X + radius, Y: center.Y}
	for i := 1; i <= segments; i++ {
		a := step * float32(i)
		next := mathx.Vec2{X: center.X + radius*mathx.Cos(a), Y: center.Y + radius*mathx.Sin(a)}
		d.push(prev, next, c)
		prev = next
	}
}

func (d *Draw) Cross(p mathx.Vec2, size float32, c mathx.Color) {
	h := size / 2
	d.push(mathx.Vec2{X: p.X - h, Y: p.Y}, mathx.Vec2{X: p.X + h, Y: p.Y}, c)
	d.push(mathx.Vec2{X: p.X, Y: p.Y - h}, mathx.Vec2{X: p.X, Y: p.Y + h}, c)
}

func (d *Draw) Arrow(a, b mathx.Vec2, c mathx.Color) {
	d.push(a, b, c)
	dir := b.Sub(a)
	l := dir.Len()
	if l == 0 {
		return
	}
	dir = dir.Scale(1 / l)
	head := min(l*0.25, 16)
	left := dir.Rotated(2.6).Scale(head)
	right := dir.Rotated(-2.6).Scale(head)
	d.push(b, b.Add(left), c)
	d.push(b, b.Add(right), c)
}

func (d *Draw) Upload() (worldFirst, worldCount, screenFirst, screenCount int) {
	total := len(d.world) + len(d.screen)
	if total == 0 {
		return 0, 0, 0, 0
	}
	if len(d.world) > 0 {
		raw := unsafe.Slice((*byte)(unsafe.Pointer(&d.world[0])), len(d.world)*VertexStride)
		d.dev.UpdateBuffer(d.vb, 0, raw)
	}
	if len(d.screen) > 0 {
		raw := unsafe.Slice((*byte)(unsafe.Pointer(&d.screen[0])), len(d.screen)*VertexStride)
		d.dev.UpdateBuffer(d.vb, len(d.world)*VertexStride, raw)
	}
	d.uploaded = total
	return 0, len(d.world), len(d.world), len(d.screen)
}

func (d *Draw) Close() {
	d.dev.DestroyBuffer(d.vb)
}

func Layout() gpu.VertexLayout {
	return gpu.VertexLayout{
		Attrs: []gpu.VertexAttr{
			{Location: 0, Slot: 0, Format: gpu.VFFloat2, Offset: 0},
			{Location: 1, Slot: 0, Format: gpu.VFUByte4Norm, Offset: 8},
		},
		Strides: [gpu.MaxVertexSlots]uint16{VertexStride},
	}
}
