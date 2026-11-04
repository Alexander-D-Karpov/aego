package texshadow

import (
	"bytes"
	"testing"

	"aego/gpu"
)

func TestPartialMipReplayPreservesGeneratedNeighboursAndLayers(t *testing.T) {
	d := gpu.TextureDesc{Width: 8, Height: 4, Dimension: gpu.Texture2DArray, Layers: 3, Format: gpu.TexR8, Mips: true, Data: bytes.Repeat([]byte{17}, 8*4*3)}
	s := New(d)
	d.Data[0] = 99
	if s.Base()[0] != 17 {
		t.Fatal("shadow aliases caller memory")
	}
	s.Update(gpu.TextureUpdate{Subresource: gpu.TextureSubresource{Layer: 2, Mip: 1}, X: 1, Y: 0, Width: 2, Height: 2, Data: []byte{1, 2, 3, 4}})
	s.Update(gpu.TextureUpdate{Subresource: gpu.TextureSubresource{Layer: 2, Mip: 1}, X: 2, Y: 0, Width: 1, Height: 1, Data: []byte{9}})
	got := bytes.Repeat([]byte{17}, 4*2*3)
	count := 0
	s.ManualMipRuns(func(u gpu.TextureUpdate) {
		count++
		if u.Subresource.Mip != 1 || u.Subresource.Layer != 2 || u.X != 1 || u.Width != 2 || u.Height != 1 {
			t.Fatalf("bad run %+v", u)
		}
		copy(got[(u.Subresource.Layer*2+u.Y)*4+u.X:], u.Data)
	})
	want := bytes.Repeat([]byte{17}, 4*2*3)
	copy(want[17:19], []byte{1, 9})
	copy(want[21:23], []byte{3, 4})
	if count != 2 || !bytes.Equal(got, want) {
		t.Fatalf("replay overwrote generated neighbours: %v", got)
	}
	if !bytes.Equal(s.Base(), bytes.Repeat([]byte{17}, 96)) {
		t.Fatal("mip update changed base")
	}
}

func TestBaseUpdateInvalidatesManualMipsAndReusesStorage(t *testing.T) {
	s := New(gpu.TextureDesc{Width: 8, Height: 8, Format: gpu.TexR8, Mips: true})
	upper := gpu.TextureUpdate{Subresource: gpu.TextureSubresource{Mip: 1}, X: 1, Y: 1, Width: 2, Height: 1, Data: []byte{4, 5}}
	base := gpu.TextureUpdate{X: 0, Y: 0, Width: 1, Height: 1, Data: []byte{3}}
	s.Update(upper)
	s.Update(base)
	s.ManualMipRuns(func(gpu.TextureUpdate) { t.Fatal("base update did not invalidate manual mips") })
	upper.X = 0
	upper.Width = 1
	s.Update(upper)
	s.ManualMipRuns(func(u gpu.TextureUpdate) {
		if u.X != 0 || u.Width != 1 {
			t.Fatal("stale dirty texels survived mip regeneration")
		}
	})
	if allocs := testing.AllocsPerRun(100, func() { s.Update(base); s.Update(upper) }); allocs != 0 {
		t.Fatalf("warmed updates allocate: %g", allocs)
	}
}
