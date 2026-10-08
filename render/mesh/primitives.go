package mesh

import "aego/core/mathx"

func vtx(p, n mathx.Vec3, u, v float32) Vertex {
	return Vertex{Pos: p, Normal: n, UV: mathx.Vec2{X: u, Y: v}, Color: White}
}

func Cube(size float32) *Data {
	h := size / 2
	faces := [6][3]mathx.Vec3{
		{{X: 1}, {Z: -1}, {Y: 1}},
		{{X: -1}, {Z: 1}, {Y: 1}},
		{{Y: 1}, {X: 1}, {Z: -1}},
		{{Y: -1}, {X: 1}, {Z: 1}},
		{{Z: 1}, {X: 1}, {Y: 1}},
		{{Z: -1}, {X: -1}, {Y: 1}},
	}
	d := &Data{Vertices: make([]Vertex, 0, 24), Indices: make([]uint32, 0, 36)}
	for _, f := range faces {
		n, u, v := f[0], f[1].Scale(h), f[2].Scale(h)
		c := n.Scale(h)
		base := uint32(len(d.Vertices))
		d.Vertices = append(d.Vertices,
			vtx(c.Sub(u).Sub(v), n, 0, 1),
			vtx(c.Add(u).Sub(v), n, 1, 1),
			vtx(c.Add(u).Add(v), n, 1, 0),
			vtx(c.Sub(u).Add(v), n, 0, 0),
		)
		d.Indices = append(d.Indices, base, base+1, base+2, base, base+2, base+3)
	}
	d.ComputeBounds()
	return d
}

func grid(d *Data, cols, rows int) {
	stride := uint32(cols + 1)
	for j := 0; j < rows; j++ {
		for i := 0; i < cols; i++ {
			a := uint32(j)*stride + uint32(i)
			b, c, e := a+1, a+stride+1, a+stride
			d.Indices = append(d.Indices, a, b, c, a, c, e)
		}
	}
}

func Plane(w, h float32, segs int) *Data {
	if segs < 1 {
		segs = 1
	}
	d := &Data{}
	n := mathx.Vec3{Y: 1}
	for j := 0; j <= segs; j++ {
		t := float32(j) / float32(segs)
		for i := 0; i <= segs; i++ {
			s := float32(i) / float32(segs)
			d.Vertices = append(d.Vertices, vtx(mathx.Vec3{X: -w/2 + w*s, Z: h/2 - h*t}, n, s, 1-t))
		}
	}
	grid(d, segs, segs)
	d.ComputeBounds()
	return d
}

func Sphere(r float32, rings, segs int) *Data {
	rings, segs = max(rings, 2), max(segs, 3)
	d := &Data{}
	for k := 0; k <= rings; k++ {
		phi := mathx.Pi * float32(k) / float32(rings)
		sp, cp := mathx.Sin(phi), mathx.Cos(phi)
		for l := 0; l <= segs; l++ {
			theta := mathx.TwoPi * float32(l) / float32(segs)
			n := mathx.Vec3{X: sp * mathx.Cos(theta), Y: cp, Z: -sp * mathx.Sin(theta)}
			d.Vertices = append(d.Vertices, vtx(n.Scale(r), n, float32(l)/float32(segs), float32(k)/float32(rings)))
		}
	}
	stride := uint32(segs + 1)
	for k := 0; k < rings; k++ {
		for l := 0; l < segs; l++ {
			a := uint32(k)*stride + uint32(l)
			b, c, e := a+stride, a+stride+1, a+1
			d.Indices = append(d.Indices, a, b, c, a, c, e)
		}
	}
	d.ComputeBounds()
	return d
}

func Cylinder(r, h float32, segs int) *Data {
	segs = max(segs, 3)
	d := &Data{}
	half := h / 2
	for l := 0; l <= segs; l++ {
		theta := mathx.TwoPi * float32(l) / float32(segs)
		c, s := mathx.Cos(theta), -mathx.Sin(theta)
		n := mathx.Vec3{X: c, Z: s}
		u := float32(l) / float32(segs)
		d.Vertices = append(d.Vertices,
			vtx(mathx.Vec3{X: r * c, Y: -half, Z: r * s}, n, u, 1),
			vtx(mathx.Vec3{X: r * c, Y: half, Z: r * s}, n, u, 0),
		)
	}
	for l := 0; l < segs; l++ {
		b0, t0 := uint32(2*l), uint32(2*l+1)
		b1, t1 := b0+2, t0+2
		d.Indices = append(d.Indices, b0, b1, t1, b0, t1, t0)
	}
	cap := func(y, ny float32, top bool) {
		n := mathx.Vec3{Y: ny}
		center := uint32(len(d.Vertices))
		d.Vertices = append(d.Vertices, vtx(mathx.Vec3{Y: y}, n, 0.5, 0.5))
		for l := 0; l <= segs; l++ {
			theta := mathx.TwoPi * float32(l) / float32(segs)
			c, s := mathx.Cos(theta), -mathx.Sin(theta)
			d.Vertices = append(d.Vertices, vtx(mathx.Vec3{X: r * c, Y: y, Z: r * s}, n, 0.5+0.5*c, 0.5+0.5*s))
		}
		for l := 0; l < segs; l++ {
			a, b := center+1+uint32(l), center+2+uint32(l)
			if top {
				d.Indices = append(d.Indices, center, a, b)
			} else {
				d.Indices = append(d.Indices, center, b, a)
			}
		}
	}
	cap(half, 1, true)
	cap(-half, -1, false)
	d.ComputeBounds()
	return d
}
