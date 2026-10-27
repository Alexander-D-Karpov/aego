package render

import (
	"testing"

	"aego/render/material"
	"aego/render/shader"
)

func benchMaterials(tb testing.TB) []*material.Material {
	tb.Helper()
	plain := &material.Shader{Name: "plain", Variants: []material.Variant{{Sources: [4]string{"v", "f", "v", "f"}}},
		Schema: material.NewSchema([]shader.Param{{Name: "Speed", Kind: shader.KindFloat, Default: [4]float32{1}}}, 16, 0)}
	lanes := &material.Shader{Name: "lanes", Variants: []material.Variant{{Sources: [4]string{"v", "f", "v", "f"}}},
		Schema: material.NewSchema([]shader.Param{{Name: "Flash", Kind: shader.KindFloat, Instance: true}}, 0, 1)}
	var out []*material.Material
	for _, sh := range []*material.Shader{plain, lanes, plain} {
		m, ok := material.New(sh.Name, sh, 0, shader.BlendPremultiplied)
		if !ok {
			tb.Fatal("no variant")
		}
		out = append(out, m)
	}
	return out
}

// BenchmarkSpritesMaterials draws 3000 sprites over three materials, one with @instance lanes; make bench-allocs holds it at 0 allocations.
func BenchmarkSpritesMaterials(b *testing.B) {
	r, dev := newTestRenderer(b, 4096)
	draws := testDraws(b, r, 3000, 30)
	mats := benchMaterials(b)
	lanes := make([][16]float32, len(draws))
	for i := range draws {
		draws[i].Material = mats[i%3]
		draws[i].Layer = int16(i % 3)
		if i%3 == 1 {
			lanes[i][0] = float32(i%7) / 7
			draws[i].Lanes = &lanes[i]
		}
	}
	v := worldView()
	frame := func() {
		r.SetClock(Clock{Time: 1, Delta: 1.0 / 60, Frame: 60})
		f := dev.BeginFrame(1280, 720)
		r.BeginFrame(f, 1280, 720, 1)
		vf := r.View(v)
		vf.Sprites(draws)
		vf.End()
		r.EndFrame()
		dev.EndFrame()
	}
	for i := 0; i < 10; i++ {
		frame()
	}
	if s := r.stats; s.Batches != 3 {
		b.Fatalf("%d batches, want 3", s.Batches)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		frame()
	}
}
