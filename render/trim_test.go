package render

import (
	"testing"

	"aego/render/material"
	"aego/render/shader"
)

func TestTrimmedMaterialSteadyStateDoesNotAllocate(t *testing.T) {
	r, dev := newTestRenderer(t, 256)
	draws := testDraws(t, r, 128, 3)
	sh := &material.Shader{Name: "trimmed", Domain: shader.DomainSprite, Variants: []material.Variant{{}}, Schema: material.NewSchema([]shader.Param{{Name: "Code", Kind: shader.KindInt, Instance: true, IntDefault: 16777217}}, 0, 1)}
	m, _ := material.New("trimmed", sh, 0, shader.BlendPremultiplied)
	for i := range draws {
		draws[i].Material = m
		draws[i].Region.W, draws[i].Region.H = 3, 2
		draws[i].Region.SourceW, draws[i].Region.SourceH = 8, 6
		draws[i].Region.TrimX, draws[i].Region.TrimY = 2, 1
		draws[i].FlipX = i%2 == 0
		draws[i].FlipY = i%3 == 0
	}
	v := worldView()
	frame := func() {
		f := dev.BeginFrame(1280, 720)
		r.BeginFrame(f, 1280, 720, 1)
		vf := r.View(v)
		vf.Sprites(draws)
		vf.End()
		r.EndFrame()
		dev.EndFrame()
	}
	for range 5 {
		frame()
	}
	if n := testing.AllocsPerRun(20, frame); n != 0 {
		t.Fatalf("trim + typed material frame allocates %.1f objects", n)
	}
	if r.Stats().Batches != 1 || r.Stats().InstanceBytes != 128*(52+shader.TrimStride) || r.Stats().MaterialLaneBytes != 128*shader.LaneStride {
		t.Fatalf("trim/override streams split or uncounted %+v", r.Stats())
	}
}
