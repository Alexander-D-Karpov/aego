package render2d

import (
	"testing"

	"aego/core/mathx"
	"aego/scene/frame"
)

func Test2DSpritesIn3DEditorPreviewOnly(t *testing.T) {
	f := lightingTestWorld(t)
	h := f.sprite("player", 1)
	f.s.SetPosition(h, mathx.Vec2{X: 3, Y: 4})
	f.s.SetScale(h, mathx.Vec2{X: 2, Y: 3})
	f.r.Prepare(640, 480, 1)
	var b frame.Bucket
	collect3(f.r, &b, 1)
	if len(b.Sprites3D) != 0 {
		t.Fatal("ordinary 2D sprites must not change runtime 3D camera output")
	}
	f.r.SetPreview2DIn3D(true)
	collect3(f.r, &b, 1)
	if len(b.Sprites3D) != 1 {
		t.Fatalf("editor preview sprites = %d, want 1", len(b.Sprites3D))
	}
	got := b.Sprites3D[0].World
	want := mathx.Mat4Lift(f.s.RenderWorld(h))
	if got != want || got.Translation() != (mathx.Vec3{X: 3, Y: -4}) {
		t.Fatalf("preview transform = %v, want %v", got, want)
	}
	b.Sprites3D = nil
	collect3(f.r, &b, 2)
	if len(b.Sprites3D) != 0 {
		t.Fatal("3D preview ignored the view's layer mask")
	}
}
