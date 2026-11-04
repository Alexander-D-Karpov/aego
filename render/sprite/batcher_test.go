package sprite

import (
	"testing"

	"aego/core/mathx"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/texture"
)

func newTestBatcher(t *testing.T, n int) (*Batcher, *null.Device) {
	t.Helper()
	dev := null.New()
	b, err := NewBatcher(dev, n)
	if err != nil {
		t.Fatal(err)
	}
	b.SetView(0, 1, 1)
	return b, dev
}

func fakeRegion(t *testing.T, dev gpu.Device, label string) texture.Region {
	t.Helper()
	tex, err := dev.CreateTexture(gpu.TextureDesc{Width: 4, Height: 4, Format: gpu.TexSRGBA8, Label: label})
	if err != nil {
		t.Fatal(err)
	}
	return texture.Whole(tex, 4, 4)
}

func TestBatchSplitsOnStateChange(t *testing.T) {
	b, dev := newTestBatcher(t, 64)
	defer b.Close()
	a := fakeRegion(t, dev, "a")
	c := fakeRegion(t, dev, "b")
	s1 := gpu.SamplerHandle{Index: 1, Gen: 1}
	s2 := gpu.SamplerHandle{Index: 2, Gen: 1}

	push := func(reg texture.Region, sampler gpu.SamplerHandle, blend BlendMode, clip mathx.Rect, on bool) {
		d := Draw{Region: reg, Size: mathx.Vec2{X: 1, Y: 1}, Sampler: sampler, Blend: blend}
		if !b.Push(&d, s1, clip, on) {
			t.Fatal("push failed")
		}
	}
	push(a, s1, BlendPremultiplied, mathx.Rect{}, false)
	push(a, s1, BlendPremultiplied, mathx.Rect{}, false)
	push(c, s1, BlendPremultiplied, mathx.Rect{}, false)
	push(c, s2, BlendPremultiplied, mathx.Rect{}, false)
	push(c, s2, BlendAdditive, mathx.Rect{}, false)
	push(c, s2, BlendAdditive, mathx.Rect{W: 10, H: 10}, true)
	b.Sort()
	batches := b.Build()
	if len(batches) != 5 {
		t.Fatalf("expected 5 batches, got %d", len(batches))
	}
	if batches[0].Count != 2 {
		t.Fatalf("first batch = %d", batches[0].Count)
	}
	total := 0
	for _, bt := range batches {
		total += bt.Count
	}
	if total != 6 {
		t.Fatalf("instances lost: %d", total)
	}
}

func TestBatchMergesWithinLayer(t *testing.T) {
	b, dev := newTestBatcher(t, 64)
	defer b.Close()
	reg := fakeRegion(t, dev, "a")
	s := gpu.SamplerHandle{Index: 1, Gen: 1}
	for i := 0; i < 10; i++ {
		d := Draw{Region: reg, Size: mathx.Vec2{X: 1, Y: 1}, Layer: int16(i % 3)}
		b.Push(&d, s, mathx.Rect{}, false)
	}
	b.Sort()
	if got := len(b.Build()); got != 1 {
		t.Fatalf("same state must merge across layers: %d batches", got)
	}
}

func TestBudgetDrops(t *testing.T) {
	b, dev := newTestBatcher(t, 2)
	defer b.Close()
	reg := fakeRegion(t, dev, "a")
	s := gpu.SamplerHandle{Index: 1, Gen: 1}
	for i := 0; i < 5; i++ {
		d := Draw{Region: reg, Size: mathx.Vec2{X: 1, Y: 1}}
		b.Push(&d, s, mathx.Rect{}, false)
	}
	if b.Len() != 2 || b.Dropped() != 3 {
		t.Fatalf("len=%d dropped=%d", b.Len(), b.Dropped())
	}
}

func TestZClampCounted(t *testing.T) {
	b, dev := newTestBatcher(t, 8)
	defer b.Close()
	reg := fakeRegion(t, dev, "a")
	s := gpu.SamplerHandle{Index: 1, Gen: 1}
	d := Draw{Region: reg, Size: mathx.Vec2{X: 1, Y: 1}, Z: 5}
	b.Push(&d, s, mathx.Rect{}, false)
	if b.Clamped() != 1 {
		t.Fatalf("clamped = %d", b.Clamped())
	}
}

// With a limit the batcher grows instead of dropping, so a busy editor panel never hides what is drawn after it.
func TestBatcherGrowsUpToTheLimit(t *testing.T) {
	b, dev := newTestBatcher(t, 2)
	defer b.Close()
	b.SetLimit(6)
	reg := fakeRegion(t, dev, "a")
	s := gpu.SamplerHandle{Index: 1, Gen: 1}
	for i := 0; i < 8; i++ {
		d := Draw{Region: reg, Size: mathx.Vec2{X: 1, Y: 1}}
		b.Push(&d, s, mathx.Rect{}, false)
	}
	b.Sort()
	if b.Len() != 6 || b.Dropped() != 2 || len(b.Instances()) != 6 {
		t.Fatalf("len=%d dropped=%d", b.Len(), b.Dropped())
	}
}

func TestCoverageFlagIsLocalToTheParticleAndQualifiedProgram(t *testing.T) {
	b, dev := newTestBatcher(t, 8)
	defer b.Close()
	mk := func(flags shader.Features) *material.Material {
		m, ok := material.New("test", &material.Shader{Variants: []material.Variant{{}}, Schema: &material.Schema{}, Features: flags}, 0, shader.BlendPremultiplied)
		if !ok {
			t.Fatal("material rejected")
		}
		return m
	}
	region := fakeRegion(t, dev, "atlas")
	region.SourceW, region.SourceH, region.TrimX, region.TrimY = 8, 16, 1, 2
	stock := mk(shader.FeaturePixelCoverage)
	shared := Draw{Material: stock, Region: region, Size: mathx.V2(8, 16), FlipX: true, Rotation: .5, Origin: mathx.V2(.5, .5)}
	normal, _, _ := shared.Geometry(1)
	want := normal.LocalRect()
	for i, test := range []struct {
		mat              *material.Material
		particle, tagged bool
	}{
		{stock, true, true}, {stock, false, false}, {mk(shader.FeatureAffine), true, false}, {nil, true, false},
	} {
		d := shared
		d.Material, d.PixelCoverage = test.mat, test.particle
		b.Reset()
		b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
		if d.Material == nil {
			if len(b.trims) != 0 {
				t.Fatal("default path gained a stream")
			}
			continue
		}
		if len(b.trims) != 1 {
			t.Fatalf("case %d missing trim", i)
		}
		rect := b.trims[0]
		tag := rect[0] >= 2
		if tag {
			rect[0] -= 2
		}
		if tag != test.tagged || rect != want {
			t.Fatalf("case %d: tag %t rect %v, want %t %v", i, tag, rect, test.tagged, want)
		}
		b.Sort()
		if b.Instances()[0].SinCos.X == 0 {
			t.Fatal("rotation lost")
		}
	}
}
