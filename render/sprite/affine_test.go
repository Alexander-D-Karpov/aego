package sprite

import (
	"testing"

	"aego/core/mathx"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render/texture"
)

func nearVec(a, b mathx.Vec2) bool {
	return mathx.Abs(a.X-b.X) < 2e-5 && mathx.Abs(a.Y-b.Y) < 2e-5
}

func TestSetTransformPreservesReflectionShearAndDegenerateBasis(t *testing.T) {
	parent := mathx.Mat3TRS(mathx.Vec2{X: 4, Y: -3}, .37, mathx.Vec2{X: -2, Y: 3})
	child := mathx.Mat3TRS(mathx.Vec2{X: 1, Y: 2}, .63, mathx.Vec2{X: 2, Y: .5})
	for _, m := range []mathx.Mat3{
		parent.Mul(child),
		mathx.Mat3TRS(mathx.Vec2{}, 0, mathx.Vec2{X: -1, Y: 1}),
		mathx.Mat3TRS(mathx.Vec2{}, 0, mathx.Vec2{}),
	} {
		var d Draw
		d.SetTransform(m)
		if !d.HasLinear || d.Basis() != ([4]float32{m[0], m[1], m[3], m[4]}) || d.Rotation != 0 || d.Scale != (mathx.Vec2{X: 1, Y: 1}) {
			t.Fatalf("lost complete affine transform: %+v", d)
		}
	}
	m := mathx.Mat3TRS(mathx.Vec2{X: 4, Y: 5}, .37, mathx.Vec2{X: 2, Y: 3})
	var ordinary Draw
	ordinary.SetTransform(m)
	if ordinary.HasLinear || ordinary.Position != (mathx.Vec2{X: 4, Y: 5}) {
		t.Fatal("ordinary positive TRS lost the original sprite path")
	}
	a := ordinary.Basis()
	for i, want := range [4]float32{m[0], m[1], m[3], m[4]} {
		if mathx.Abs(a[i]-want) > 1e-6 {
			t.Fatal("TRS decomposition changed geometry")
		}
	}
}

func TestAffineTrimmedReflectedSpriteMatchesPresentationCorners(t *testing.T) {
	b, dev := newTestBatcher(t, 8)
	defer b.Close()
	reg := fakeRegion(t, dev, "trim")
	reg.W, reg.H, reg.SourceW, reg.SourceH, reg.TrimX, reg.TrimY = 3, 2, 8, 6, 1, 1
	m := mathx.Mat3TRS(mathx.Vec2{X: 7, Y: -2}, .4, mathx.Vec2{X: -3, Y: 2}).Mul(mathx.Mat3TRS(mathx.Vec2{}, .7, mathx.Vec2{X: 1, Y: 1}))
	for _, flip := range []bool{false, true} {
		b.Reset()
		d := Draw{Region: reg, Size: mathx.Vec2{X: 8, Y: 6}, Origin: mathx.Vec2{X: .5, Y: .5}, FlipX: flip, FlipY: flip}
		d.SetTransform(m)
		b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
		b.Sort()
		bs := b.Build()
		if len(bs) != 1 || bs[0].AuxFirst != 0 {
			t.Fatal("affine stream missing")
		}
		inst, aux := b.sorted[0], b.auxOut[0]
		if inst.Size != (mathx.Vec2{X: 3, Y: 2}) || inst.SinCos != (mathx.Vec2{Y: 1}) {
			t.Fatalf("scale/rotation applied twice: %+v", inst)
		}
		x0, y0 := float32(-3), float32(-2)
		if flip {
			x0, y0 = 0, 0
		}
		for _, p := range []mathx.Vec2{{}, {X: 1}, {X: 1, Y: 1}, {Y: 1}} {
			q := mathx.Vec2{X: (p.X - inst.Origin.X) * inst.Size.X, Y: (p.Y - inst.Origin.Y) * inst.Size.Y}
			got := mathx.Vec2{X: inst.Pos.X + (aux.Affine[0]+1)*q.X + aux.Affine[2]*q.Y, Y: inst.Pos.Y + aux.Affine[1]*q.X + aux.Affine[3]*q.Y}
			want := m.Apply(mathx.Vec2{X: x0 + p.X*3, Y: y0 + p.Y*2})
			if !nearVec(got, want) {
				t.Fatalf("flip=%v corner %v: got %v want %v", flip, p, got, want)
			}
		}
	}
}

func TestAuxMixedBatchKeepsPainterOrderAndNoAuxFastPath(t *testing.T) {
	dev := &countingDevice{Device: null.New()}
	b, err := NewBatcher(dev, 8)
	if err != nil {
		t.Fatal(err)
	}
	reg := fakeRegion(t, dev, "shared")
	d := Draw{Region: reg, Size: mathx.Vec2{X: 1, Y: 1}}
	b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
	d.SetTransform(mathx.Mat3{1, 0, 0, 2, 1, 0, 0, 0, 1})
	b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
	d.HasLinear = false
	b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
	b.Sort()
	bs := b.Build()
	if len(bs) != 1 || bs[0].Count != 3 || bs[0].AuxFirst != 0 || b.AuxBytes() != 3*AuxStride || b.Breaks() != (BatchBreaks{}) {
		t.Fatalf("aux presence split otherwise compatible sprites: %+v", bs)
	}
	if b.auxOut[0] != identityAux() || b.auxOut[2] != identityAux() || b.auxOut[1].Affine != ([4]float32{0, 0, 2, 1}) {
		t.Fatal("mixed batch lacks identity fill or painter-order aux")
	}
	if !b.UploadAux() || b.AuxBuffer().IsZero() {
		t.Fatal("aux upload failed")
	}
	b.Reset()
	b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
	b.Sort()
	bs = b.Build()
	creates, updates := dev.creates, dev.updates
	if bs[0].AuxFirst != -1 || b.AuxBytes() != 0 || !b.UploadAux() || dev.creates != creates || dev.updates != updates {
		t.Fatal("ordinary sprite reused stale aux or made an auxiliary GPU call")
	}
	b.Close()
	if dev.ResourceStats().Buffers != 0 {
		t.Fatal("aux buffer leaked on batcher destruction")
	}
}

func TestLightingPromotesLegacyTransformAndUsesIndependentNormalRegion(t *testing.T) {
	b, dev := newTestBatcher(t, 8)
	defer b.Close()
	albedo, normal := fakeRegion(t, dev, "albedo"), fakeRegion(t, dev, "normal")
	normal.SourceW, normal.SourceH, normal.W, normal.H, normal.TrimX, normal.TrimY = 16, 8, 12, 4, 2, 1
	normal.UVMin, normal.UVMax = mathx.Vec2{X: .2, Y: .3}, mathx.Vec2{X: .5, Y: .8}
	d := Draw{Region: albedo, Normal: normal, Lighting: true, Size: mathx.Vec2{X: 4, Y: 2}, Scale: mathx.Vec2{X: -2, Y: 3}, Rotation: .4, FlipX: true, FlipY: true}
	b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
	b.Sort()
	bs := b.Build()
	inst, aux := b.sorted[0], b.auxOut[0]
	if inst.Size != d.Size || inst.SinCos != (mathx.Vec2{Y: 1}) || bs[0].Normal != normal.Texture {
		t.Fatal("Lit legacy transform not promoted to full basis")
	}
	want := d.Basis()
	want[0]--
	if aux.Affine != want || aux.NormalUV != ([4]float32{.2, .3, .5, .8}) || aux.NormalLocal != normal.LocalRect() {
		t.Fatalf("normal mapping reused albedo packing or double flipped: %+v", aux)
	}
}

func TestNormalTextureSplitsPreserveABAAndWarmAuxHasNoAllocations(t *testing.T) {
	b, dev := newTestBatcher(t, 16)
	defer b.Close()
	albedo, a, c := fakeRegion(t, dev, "albedo"), fakeRegion(t, dev, "normal-a"), fakeRegion(t, dev, "normal-b")
	draws := []Draw{{Region: albedo, Normal: a, Lighting: true}, {Region: albedo, Normal: c, Lighting: true}, {Region: albedo, Normal: a, Lighting: true}}
	run := func() {
		b.Reset()
		for i := range draws {
			b.Push(&draws[i], gpu.SamplerHandle{}, mathx.Rect{}, false)
		}
		b.Sort()
		b.Build()
		b.UploadAux()
	}
	run()
	if len(b.batches) != 3 || b.batches[0].Normal != a.Texture || b.batches[1].Normal != c.Texture || b.batches[2].Normal != a.Texture || b.Breaks().Texture != 2 {
		t.Fatal("normal textures were reordered or merged across a different binding")
	}
	if n := testing.AllocsPerRun(500, run); n != 0 {
		t.Fatalf("warm auxiliary stream allocates %v times per frame", n)
	}
}

func TestLightingBindingsSurviveSortingGrowthAndNextFrame(t *testing.T) {
	b, dev := newTestBatcher(t, 4)
	defer b.Close()
	b.SetLimit(512)
	albedo := fakeRegion(t, dev, "albedo")
	channels := []texture.Region{fakeRegion(t, dev, "a"), fakeRegion(t, dev, "b"), fakeRegion(t, dev, "c")}
	for frame := 0; frame < 3; frame++ {
		b.Reset()
		for i := 0; i < 257; i++ {
			d := Draw{Region: albedo, Normal: channels[i%3], Emission: texture.Emission{Region: channels[(i+1)%3], Mode: texture.EmissionMode(1 + i%2)}, Lighting: true, LightSet: uint32(1 + i + frame*300), Order: int16(256 - i), Position: mathx.Vec2{X: float32(i)}}
			if !b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false) {
				t.Fatal("bounded growth dropped a sprite")
			}
		}
		b.Sort()
		batches := b.Build()
		if len(batches) != 257 || b.Breaks().Lighting != 256 {
			t.Fatalf("different light sets merged: %d batches, %+v", len(batches), b.Breaks())
		}
		for j, batch := range batches {
			i := 256 - j
			if b.Instances()[batch.First].Pos.X != float32(i) || batch.LightSet != uint32(1+i+frame*300) || batch.Normal != channels[i%3].Texture || batch.Emission != channels[(i+1)%3].Texture || batch.EmissionMode != texture.EmissionMode(1+i%2) {
				t.Fatalf("frame %d draw %d: sorted instance and lighting resources disagree: %+v", frame, i, batch)
			}
		}
	}
	// A following Unlit frame must neither inherit bindings nor allocate an
	// auxiliary upload merely because the preceding frame used lighting.
	b.Reset()
	d := Draw{Region: albedo}
	b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
	b.Sort()
	batch := b.Build()[0]
	if batch.LightSet != 0 || !batch.Normal.IsZero() || !batch.Emission.IsZero() || batch.EmissionMode != texture.EmissionNone || batch.AuxFirst != -1 {
		t.Fatalf("Unlit frame retained lighting state: %+v", batch)
	}
}
