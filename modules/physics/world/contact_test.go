package world

import "testing"

func TestContactBetweenOrientation(t *testing.T) {
	w, _ := New(DefaultConfig())
	gd := DefaultBodyDef()
	gd.Type = Static
	g, _ := w.CreateBody(gd)
	gs, _ := w.AddShape(g, DefaultShapeDef(Box{Half: Vec2{X: 5, Y: 0.5}}))
	bd := DefaultBodyDef()
	bd.Position = Vec2{Y: -1}
	b, _ := w.CreateBody(bd)
	bs, _ := w.AddShape(b, DefaultShapeDef(Box{Half: Vec2{X: 0.5, Y: 0.5}}))
	for i := 0; i < 20; i++ {
		w.Step(1.0 / 60)
	}
	ab, ok := w.ContactBetween(bs, gs)
	if !ok || ab.ShapeA != bs || ab.Normal.Y >= 0 {
		t.Fatalf("crate side %+v %v", ab, ok)
	}
	ba, ok := w.ContactBetween(gs, bs)
	if !ok || ba.ShapeA != gs || ba.Normal.Y <= 0 || ba.Normal != ab.Normal.Neg() {
		t.Fatalf("ground side %+v %v", ba, ok)
	}
}
