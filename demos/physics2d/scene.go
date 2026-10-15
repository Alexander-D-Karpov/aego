package physics2d

import (
	"aego/core/mathx"
	"aego/modules/physics/world"
)

type builder struct {
	w   *world.World
	err error
}

func (b *builder) body(d world.BodyDef) world.BodyHandle {
	if b.err != nil {
		return world.BodyHandle{}
	}
	h, err := b.w.CreateBody(d)
	b.err = err
	return h
}

func (b *builder) shape(h world.BodyHandle, sd world.ShapeDef) {
	if b.err == nil {
		_, b.err = b.w.AddShape(h, sd)
	}
}

func (b *builder) joint(j world.JointDef) {
	if b.err == nil {
		_, b.err = b.w.CreateJoint(j)
	}
}

func (b *builder) static(p mathx.Vec2) world.BodyHandle {
	d := world.DefaultBodyDef()
	d.Type, d.Position = world.Static, p
	return b.body(d)
}

func (b *builder) box(p mathx.Vec2, half float32) world.BodyHandle {
	return b.brick(p, half, half)
}

func (b *builder) brick(p mathx.Vec2, hx, hy float32) world.BodyHandle {
	d := world.DefaultBodyDef()
	d.Position = p
	h := b.body(d)
	b.shape(h, world.DefaultShapeDef(world.Box{Half: mathx.Vec2{X: hx, Y: hy}}))
	return h
}

func (b *builder) ground() {
	h := b.static(mathx.Vec2{})
	sd := world.DefaultShapeDef(world.Chain{Pts: []mathx.Vec2{
		{X: -60}, {X: -8}, {X: 8, Y: -6}, {X: 30, Y: -6}, {X: 45}, {X: 70},
	}})
	sd.OneSided = true
	b.shape(h, sd)
}

func (b *builder) wall() {
	const half = 0.25
	const left, right = float32(-50), float32(-40)
	for k := 0; k < 25; k++ {
		y := -half - float32(k)*2*half
		n, x0 := 20, left+half
		if k%2 == 0 {
			b.brick(mathx.Vec2{X: left + half/2, Y: y}, half/2, half)
			b.brick(mathx.Vec2{X: right - half/2, Y: y}, half/2, half)
			n, x0 = 19, x0+half
		}
		for i := 0; i < n; i++ {
			b.box(mathx.Vec2{X: x0 + float32(i)*2*half, Y: y}, half)
		}
	}
}

func (b *builder) pyramid() {
	const rows = 15
	for r := 0; r < rows; r++ {
		n := rows - r
		for k := 0; k < n; k++ {
			b.box(mathx.Vec2{X: -20 + (float32(k)-float32(n-1)/2)*0.5, Y: -0.25 - float32(r)*0.5}, 0.24)
		}
	}
}

func (b *builder) rope() {
	anchor := b.static(mathx.Vec2{X: 20, Y: -25})
	prev, la := anchor, mathx.Vec2{}
	for i := 0; i < 30; i++ {
		d := world.DefaultBodyDef()
		d.Position = mathx.Vec2{X: 20 + 0.5*float32(i) + 0.25, Y: -25}
		link := b.body(d)
		b.shape(link, world.DefaultShapeDef(world.Capsule{HalfLen: 0.25, R: 0.08}))
		b.joint(world.RevoluteJoint{A: prev, B: link, LocalAnchorA: la, LocalAnchorB: mathx.Vec2{X: -0.25}})
		prev, la = link, mathx.Vec2{X: 0.25}
	}
}

func (b *builder) ball() {
	d := world.DefaultBodyDef()
	d.Position = mathx.Vec2{X: 0, Y: -20}
	h := b.body(d)
	sd := world.DefaultShapeDef(world.Circle{R: 0.6})
	sd.Restitution = 0.9
	b.shape(h, sd)
}

func buildScene() (*world.World, error) {
	cfg := world.DefaultConfig()
	cfg.InitialBodies, cfg.InitialShapes, cfg.InitialContacts = 2048, 2048, 4096
	w, err := world.New(cfg)
	if err != nil {
		return nil, err
	}
	b := &builder{w: w}
	b.ground()
	b.wall()
	b.pyramid()
	b.rope()
	b.ball()
	return w, b.err
}
