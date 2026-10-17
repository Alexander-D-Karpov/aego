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
	Overlay
)

const VertexStride = 16

type Vertex struct {
	Pos   mathx.Vec3
	Color uint32
}

const _ = uint(VertexStride - unsafe.Sizeof(Vertex{}))

const _ = uint(unsafe.Sizeof(Vertex{}) - VertexStride)

type Range struct {
	First int
	Count int
}

type Draw struct {
	dev     gpu.Device
	vb      gpu.BufferHandle
	max     int
	world   []Vertex
	overlay []Vertex
	screen  []Vertex
	space   Space
	dropped uint32
}

func New(dev gpu.Device, maxVerts int) (*Draw, error) {
	if maxVerts < 2 {
		maxVerts = 2
	}
	vb, err := dev.CreateBuffer(gpu.BufferDesc{
		Usage:   gpu.BufferVertex,
		Size:    maxVerts * VertexStride,
		Dynamic: true,
		Label:   "render.debug_lines",
	})
	if err != nil {
		return nil, err
	}
	return &Draw{
		dev:     dev,
		vb:      vb,
		max:     maxVerts,
		world:   make([]Vertex, 0, maxVerts),
		overlay: make([]Vertex, 0, maxVerts),
		screen:  make([]Vertex, 0, maxVerts),
	}, nil
}

func (d *Draw) Buffer() gpu.BufferHandle {
	return d.vb
}

func (d *Draw) Reset() {
	d.world = d.world[:0]
	d.overlay = d.overlay[:0]
	d.screen = d.screen[:0]
	d.space = World
	d.dropped = 0
}

func (d *Draw) Space(s Space) {
	d.space = s
}

func (d *Draw) Dropped() uint32 {
	return d.dropped
}

func (d *Draw) Counts() (world, overlay, screen int) {
	return len(d.world), len(d.overlay), len(d.screen)
}

func (d *Draw) push3(a, b mathx.Vec3, c mathx.Color) {
	if len(d.world)+len(d.overlay)+len(d.screen)+2 > d.max {
		d.dropped += 2
		return
	}
	packed := c.Premultiplied().PackRGBA8()
	va, vb := Vertex{Pos: a, Color: packed}, Vertex{Pos: b, Color: packed}
	switch d.space {
	case Screen:
		d.screen = append(d.screen, va, vb)
	case Overlay:
		d.overlay = append(d.overlay, va, vb)
	default:
		d.world = append(d.world, va, vb)
	}
}

func (d *Draw) push(a, b mathx.Vec2, c mathx.Color) {
	d.push3(mathx.Vec3{X: a.X, Y: a.Y}, mathx.Vec3{X: b.X, Y: b.Y}, c)
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

func (d *Draw) AABB(b mathx.AABB2, c mathx.Color) {
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

func (d *Draw) Line3(a, b mathx.Vec3, c mathx.Color) {
	d.push3(a, b, c)
}

func (d *Draw) AABB3(b mathx.AABB3, c mathx.Color) {
	lo, hi := b.Min, b.Max
	p := [8]mathx.Vec3{
		{X: lo.X, Y: lo.Y, Z: lo.Z}, {X: hi.X, Y: lo.Y, Z: lo.Z},
		{X: hi.X, Y: hi.Y, Z: lo.Z}, {X: lo.X, Y: hi.Y, Z: lo.Z},
		{X: lo.X, Y: lo.Y, Z: hi.Z}, {X: hi.X, Y: lo.Y, Z: hi.Z},
		{X: hi.X, Y: hi.Y, Z: hi.Z}, {X: lo.X, Y: hi.Y, Z: hi.Z},
	}
	d.box(&p, c)
}

func (d *Draw) box(p *[8]mathx.Vec3, c mathx.Color) {
	for i := 0; i < 4; i++ {
		d.push3(p[i], p[(i+1)%4], c)
		d.push3(p[4+i], p[4+(i+1)%4], c)
		d.push3(p[i], p[4+i], c)
	}
}

func (d *Draw) Grid3(center mathx.Vec3, step float32, count int, c mathx.Color) {
	if step <= 0 || count <= 0 {
		return
	}
	ext := step * float32(count)
	for i := -count; i <= count; i++ {
		o := step * float32(i)
		d.push3(mathx.Vec3{X: center.X + o, Y: center.Y, Z: center.Z - ext}, mathx.Vec3{X: center.X + o, Y: center.Y, Z: center.Z + ext}, c)
		d.push3(mathx.Vec3{X: center.X - ext, Y: center.Y, Z: center.Z + o}, mathx.Vec3{X: center.X + ext, Y: center.Y, Z: center.Z + o}, c)
	}
}

func (d *Draw) Axes3(m mathx.Mat4, size float32) {
	o := m.Translation()
	d.push3(o, o.Add(m.TransformDir(mathx.Vec3{X: size})), mathx.Color{R: 1, G: 0.2, B: 0.2, A: 1})
	d.push3(o, o.Add(m.TransformDir(mathx.Vec3{Y: size})), mathx.Color{R: 0.2, G: 1, B: 0.3, A: 1})
	d.push3(o, o.Add(m.TransformDir(mathx.Vec3{Z: size})), mathx.Color{R: 0.3, G: 0.5, B: 1, A: 1})
}

func Basis(normal mathx.Vec3) (mathx.Vec3, mathx.Vec3) {
	n := normal.Normalized()
	a := mathx.Vec3{Y: 1}
	if mathx.Abs(n.Y) > 0.9 {
		a = mathx.Vec3{X: 1}
	}
	u := n.Cross(a).Normalized()
	return u, n.Cross(u)
}

func (d *Draw) Circle3(center, normal mathx.Vec3, r float32, segs int, c mathx.Color) {
	if segs < 3 {
		segs = 3
	}
	u, w := Basis(normal)
	step := mathx.TwoPi / float32(segs)
	prev := center.Add(u.Scale(r))
	for i := 1; i <= segs; i++ {
		a := step * float32(i)
		next := center.Add(u.Scale(r * mathx.Cos(a))).Add(w.Scale(r * mathx.Sin(a)))
		d.push3(prev, next, c)
		prev = next
	}
}

func (d *Draw) Frustum(viewProj mathx.Mat4, c mathx.Color) {
	inv, ok := viewProj.Inverse()
	if !ok {
		return
	}
	var p [8]mathx.Vec3
	for i := 0; i < 8; i++ {
		x := float32(-1)
		if i == 1 || i == 2 || i == 5 || i == 6 {
			x = 1
		}
		y := float32(-1)
		if i == 2 || i == 3 || i == 6 || i == 7 {
			y = 1
		}
		z := float32(-1)
		if i >= 4 {
			z = 1
		}
		p[i] = inv.TransformPoint(mathx.Vec3{X: x, Y: y, Z: z})
	}
	d.box(&p, c)
}

func (d *Draw) upload(list []Vertex, first int) Range {
	if len(list) == 0 {
		return Range{First: first}
	}
	raw := unsafe.Slice((*byte)(unsafe.Pointer(&list[0])), len(list)*VertexStride)
	d.dev.UpdateBuffer(d.vb, first*VertexStride, raw)
	return Range{First: first, Count: len(list)}
}

func (d *Draw) Upload() (world, overlay, screen Range) {
	world = d.upload(d.world, 0)
	overlay = d.upload(d.overlay, world.First+world.Count)
	screen = d.upload(d.screen, overlay.First+overlay.Count)
	return world, overlay, screen
}

func (d *Draw) Close() {
	d.dev.DestroyBuffer(d.vb)
}

func Layout() gpu.VertexLayout {
	return gpu.VertexLayout{
		Attrs: []gpu.VertexAttr{
			{Location: 0, Slot: 0, Format: gpu.VFFloat3, Offset: 0},
			{Location: 1, Slot: 0, Format: gpu.VFUByte4Norm, Offset: 12},
		},
		Strides: [gpu.MaxVertexSlots]uint16{VertexStride},
	}
}
