package render

import (
	"math/rand"
	"testing"

	"aego/core/log"
	"aego/core/mathx"
	"aego/engine/res"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/render/view"
	"aego/vfs"
)

func newTestRenderer(t testing.TB, maxSprites int) (*Renderer, *null.Device) {
	t.Helper()
	files, err := vfs.New(vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	dev := null.New()
	r, err := New(dev, dev.Caps(), files, log.Nop(), Options{MaxSprites: maxSprites})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r, dev
}

func testDraws(t testing.TB, r *Renderer, n int, extent float32) []sprite.Draw {
	t.Helper()
	tex, err := r.Textures().Upload(texture.Solid(4, 4, mathx.Color{R: 1, G: 1, B: 1, A: 1}), texture.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	reg := texture.Whole(tex, 4, 4)
	rng := rand.New(rand.NewSource(11))
	out := make([]sprite.Draw, n)
	for i := range out {
		out[i] = sprite.Draw{
			Region:   reg,
			Position: mathx.Vec2{X: (rng.Float32()*2 - 1) * extent, Y: (rng.Float32()*2 - 1) * extent},
			Size:     mathx.Vec2{X: 1, Y: 1},
			Tint:     mathx.Color{R: 1, G: 1, B: 1, A: 1},
		}
	}
	return out
}

func worldView() view.View {
	return view.View{
		Viewport:   mathx.Rect{W: 1280, H: 720},
		TargetW:    1280,
		TargetH:    720,
		PixelScale: 1,
		Camera:     view.Camera2D{Zoom: 16},
		Label:      "test",
	}
}

func TestSubmitCountsAndCulls(t *testing.T) {
	r, _ := newTestRenderer(t, 4096)
	draws := testDraws(t, r, 200, 200)
	if err := r.BeginFrame(nil, 1280, 720, 1); err != nil {
		t.Fatal(err)
	}
	f := r.View(worldView())
	f.Sprites(draws)
	f.End()
	s := r.EndFrame()
	if s.Submitted != 200 {
		t.Fatalf("submitted = %d", s.Submitted)
	}
	if s.Culled == 0 || s.Visible == 0 || s.Visible+s.Culled != 200 {
		t.Fatalf("visible=%d culled=%d", s.Visible, s.Culled)
	}
}

func TestBudgetOverflowIsCounted(t *testing.T) {
	r, _ := newTestRenderer(t, 16)
	draws := testDraws(t, r, 64, 1)
	if err := r.BeginFrame(nil, 1280, 720, 1); err != nil {
		t.Fatal(err)
	}
	f := r.View(worldView())
	f.Sprites(draws)
	f.End()
	s := r.EndFrame()
	if s.Dropped != 48 {
		t.Fatalf("dropped = %d", s.Dropped)
	}
}

func TestPassOrdering(t *testing.T) {
	r, _ := newTestRenderer(t, 64)
	var order []string
	add := func(id string, after []string) {
		if err := r.AddPass(PassDesc{ID: id, After: after, Pass: recordPass{id: id, out: &order}}); err != nil {
			t.Fatal(err)
		}
	}
	add("late", []string{"debug"})
	add("early", nil)
	if err := r.BeginFrame(nil, 64, 64, 1); err != nil {
		t.Fatal(err)
	}
	if err := r.AddPass(PassDesc{ID: "toolate", Pass: recordPass{id: "x", out: &order}}); err == nil {
		t.Fatal("passes must be frozen after the first frame")
	}
	r.EndFrame()
}

type recordPass struct {
	id  string
	out *[]string
}

func (p recordPass) Execute(*PassContext) {
	*p.out = append(*p.out, p.id)
}

func TestPassCycleDetected(t *testing.T) {
	r, _ := newTestRenderer(t, 64)
	var order []string
	r.AddPass(PassDesc{ID: "a", After: []string{"b"}, Pass: recordPass{id: "a", out: &order}})
	r.AddPass(PassDesc{ID: "b", After: []string{"a"}, Pass: recordPass{id: "b", out: &order}})
	if err := r.BeginFrame(nil, 64, 64, 1); err == nil {
		t.Fatal("cycle must be rejected")
	}
}

func TestNoAllocationsPerFrame(t *testing.T) {
	r, dev := newTestRenderer(t, 60000)
	draws := testDraws(t, r, 50000, 30)
	v := worldView()
	frame := func() {
		f := dev.BeginFrame(1280, 720)
		r.BeginFrame(f, 1280, 720, 1)
		vf := r.View(v)
		vf.Sprites(draws)
		vf.Debug().Line(mathx.Vec2{}, mathx.Vec2{X: 1}, mathx.Color{R: 1, A: 1})
		vf.End()
		r.EndFrame()
		dev.EndFrame()
	}
	for i := 0; i < 10; i++ {
		frame()
	}
	if n := testing.AllocsPerRun(20, frame); n != 0 {
		t.Fatalf("frame allocates %.1f objects", n)
	}
}

func TestClipIsInLogicalPixels(t *testing.T) {
	r, _ := newTestRenderer(t, 64)
	draws := testDraws(t, r, 1, 0)
	v := view.ScreenView(1280, 720, 2)
	r.BeginFrame(nil, 1280, 720, 2)
	f := r.View(v)
	f.PushClip(mathx.Rect{X: 10, Y: 20, W: 100, H: 50})
	f.Sprites(draws)
	f.PopClip()
	f.End()
	r.EndFrame()
	batches := r.batcher.Build()
	if len(batches) == 0 {
		t.Fatal("sprite was culled, nothing to check")
	}
	if got := batches[0].Clip; got.X != 20 || got.Y != 40 || got.W != 200 || got.H != 100 {
		t.Fatalf("clip not scaled to framebuffer pixels: %+v", got)
	}
	if !batches[0].Scissor {
		t.Fatal("scissor flag not set")
	}
}

var _ gpu.Device = (*null.Device)(nil)
