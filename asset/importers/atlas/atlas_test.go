package atlas

import (
	"testing"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/format/aetext"
	"aego/gpu"
	rtex "aego/render/texture"
)

func member(t *testing.T, w, h int, c mathx.Color) pipeline.GroupMember {
	t.Helper()
	d := rtex.Solid(w, h, c)
	d.Premultiply()
	return pipeline.GroupMember{
		ID:     ids.NewAssetID(),
		Name:   "sprite",
		Result: &pipeline.Result{Raw: d, RawFilter: int(gpu.FilterNearest)},
	}
}

func TestPackNoOverlap(t *testing.T) {
	var members []pipeline.GroupMember
	for i := 0; i < 30; i++ {
		members = append(members, member(t, 16+i%8, 16+(i%3)*4, mathx.Color{R: 1, A: 1}))
	}
	placements, w, h, err := pack(members, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(placements) != 30 {
		t.Fatalf("placed %d of 30", len(placements))
	}
	if w > MaxPage || h > MaxPage {
		t.Fatalf("page %dx%d exceeds the limit", w, h)
	}
	for i := range placements {
		a := &placements[i]
		if a.x < 2 || a.y < 2 || a.x+a.w+2 > w || a.y+a.h+2 > h {
			t.Fatalf("placement %+v is outside the padded page %dx%d", a, w, h)
		}
		for j := i + 1; j < len(placements); j++ {
			b := &placements[j]
			if overlaps(a, b, 2) {
				t.Fatalf("placements overlap: %+v and %+v", a, b)
			}
		}
	}
}

func overlaps(a, b *placement, padding int) bool {
	ax0, ax1 := a.x-padding, a.x+a.w+padding
	ay0, ay1 := a.y-padding, a.y+a.h+padding
	bx0, bx1 := b.x, b.x+b.w
	by0, by1 := b.y, b.y+b.h
	return ax0 < bx1 && bx0 < ax1 && ay0 < by1 && by0 < ay1
}

func TestFinalizeProducesRegionsAndPage(t *testing.T) {
	members := []pipeline.GroupMember{
		member(t, 32, 32, mathx.Color{R: 1, A: 1}),
		member(t, 16, 48, mathx.Color{G: 1, A: 1}),
		member(t, 64, 16, mathx.Color{B: 1, A: 1}),
	}
	pageID := ids.NewAssetID()
	imp := Wrap(nil)
	ctx := &pipeline.GroupContext{Group: "characters", PageID: pageID, Log: log.Nop().With(log.Asset)}
	perMember, pages, err := imp.Finalize(ctx, members)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 {
		t.Fatalf("pages = %d", len(pages))
	}
	if len(perMember) != 3 {
		t.Fatalf("regions = %d", len(perMember))
	}
	for _, m := range members {
		res, ok := perMember[m.ID]
		if !ok || len(res.Artifacts) != 1 {
			t.Fatalf("member %s has no region", m.ID)
		}
		ra, err := assetDecodeRegion(res.Artifacts[0].Data)
		if err != nil {
			t.Fatal(err)
		}
		if ra.Page != pageID {
			t.Fatal("region points at the wrong page")
		}
		if ra.W != m.Result.Raw.Width || ra.H != m.Result.Raw.Height {
			t.Fatalf("region size = %dx%d, want %dx%d", ra.W, ra.H, m.Result.Raw.Width, m.Result.Raw.Height)
		}
	}
}

func TestExtrudeCopiesEdgePixels(t *testing.T) {
	page := rtex.New(16, 16, gpu.TexSRGBA8)
	src := rtex.Solid(4, 4, mathx.Color{R: 1, G: 1, B: 1, A: 1})
	src.Premultiply()
	blit(page, src, 4, 4)
	extrudeEdges(page, 4, 4, 4, 4, 2)

	inside := pixelAt(page, 4, 4)
	for _, p := range [][2]int{{2, 4}, {3, 4}, {4, 2}, {4, 3}, {9, 7}, {2, 2}} {
		if pixelAt(page, p[0], p[1]) != inside {
			t.Fatalf("pixel %v was not extruded", p)
		}
	}
	if pixelAt(page, 0, 0) == inside {
		t.Fatal("extrusion went further than the padding")
	}
}

func pixelAt(d *rtex.Data, x, y int) [4]byte {
	i := (y*d.Width + x) * 4
	return [4]byte{d.Pixels[i], d.Pixels[i+1], d.Pixels[i+2], d.Pixels[i+3]}
}

func TestEmptyGroup(t *testing.T) {
	imp := Wrap(nil)
	ctx := &pipeline.GroupContext{Group: "empty", PageID: ids.NewAssetID(), Log: log.Nop().With(log.Asset)}
	perMember, pages, err := imp.Finalize(ctx, nil)
	if err != nil || perMember != nil || pages != nil {
		t.Fatalf("empty group must be a no-op: %v %v %v", perMember, pages, err)
	}
}

func assetDecodeRegion(data []byte) (*asset.RegionArtifact, error) {
	return asset.DecodeRegion(data)
}

func TestTrimArtifactPreservesOriginalCanvasAndConvertsLinearPage(t *testing.T) {
	d := rtex.New(8, 6, gpu.TexRGBA8)
	for y := 1; y < 3; y++ {
		for x := 2; x < 5; x++ {
			d.SetPixel(x, y, mathx.Color{R: .5, A: .5})
		}
	}
	d.Premultiply()
	m := pipeline.GroupMember{ID: ids.NewAssetID(), Settings: pipeline.Settings{"trim": aetext.Bool(true)}, Result: &pipeline.Result{Raw: d}}
	ctx := &pipeline.GroupContext{Group: "trim", PageID: ids.NewAssetID(), Log: log.Nop().With(log.Asset)}
	regions, pages, err := Wrap(nil).Finalize(ctx, []pipeline.GroupMember{m})
	if err != nil {
		t.Fatal(err)
	}
	r, err := asset.DecodeRegion(regions[m.ID].Artifacts[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	if r.SourceW != 8 || r.SourceH != 6 || r.TrimX != 2 || r.TrimY != 1 || r.W != 3 || r.H != 2 {
		t.Fatalf("trim metadata %+v", r)
	}
	page, err := asset.DecodeTexture(pages[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	p := (r.Y*page.Width + r.X) * 4
	// The source is already premultiplied linear .25. Atlas storage encodes it
	// to sRGB (~137), preserving alpha128 and avoiding another multiplication.
	if page.Format != gpu.TexSRGBA8 || page.Pixels[p] < 136 || page.Pixels[p] > 138 || page.Pixels[p+3] != 128 {
		t.Fatalf("linear atlas conversion %v", page.Pixels[p:p+4])
	}
}

func TestAtlasRotationIsExplicitlyRejected(t *testing.T) {
	m := member(t, 2, 2, mathx.White)
	m.Settings = pipeline.Settings{"rotate": aetext.Bool(true)}
	if _, _, _, err := pack([]pipeline.GroupMember{m}, 2); err == nil {
		t.Fatal("unsupported rotation was ignored")
	}
}

func TestNormalAtlasPreservesUnpremultipliedDataAndFullPadding(t *testing.T) {
	d := rtex.New(2, 2, gpu.TexRGBA8)
	d.NonColor = true
	for i := 0; i < len(d.Pixels); i += 4 {
		copy(d.Pixels[i:i+4], []byte{210, 40, 250, 32})
	}
	normalID, albedoID := ids.NewAssetID(), ids.NewAssetID()
	m := pipeline.GroupMember{ID: normalID, Result: &pipeline.Result{Raw: d, RawSemantic: asset.TextureNormal, Deps: []ids.AssetID{albedoID}}}
	ctx := &pipeline.GroupContext{PageID: ids.NewAssetID()}
	regions, pages, err := Wrap(nil).Finalize(ctx, []pipeline.GroupMember{m})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := asset.DecodeRegion(regions[normalID].Artifacts[0].Data)
	page, err := asset.DecodeTexture(pages[0].Data)
	if err != nil || page.Semantic != asset.TextureNormal || page.Format != gpu.TexRGBA8 || page.Mips {
		t.Fatalf("normal page is not linear data: %+v %v", page, err)
	}
	for y := r.Y - 2; y < r.Y+r.H+2; y++ {
		for x := r.X - 2; x < r.X+r.W+2; x++ {
			i := (y*page.Width + x) * 4
			if string(page.Pixels[i:i+4]) != string([]byte{210, 40, 250, 32}) {
				t.Fatalf("padding at %d,%d was omitted or data changed: %v", x, y, page.Pixels[i:i+4])
			}
		}
	}
	if len(regions[normalID].Deps) != 1 || regions[normalID].Deps[0] != albedoID {
		t.Fatal("atlas discarded source dependencies")
	}
	m.Settings = pipeline.Settings{"mips": aetext.Bool(true)}
	if _, _, err := Wrap(nil).Finalize(ctx, []pipeline.GroupMember{m}); err == nil {
		t.Fatal("unsafe cross-region mip chain accepted")
	}
	m.Settings = nil
	color := member(t, 2, 2, mathx.White)
	if _, _, err := Wrap(nil).Finalize(ctx, []pipeline.GroupMember{m, color}); err == nil {
		t.Fatal("normal data shared a color atlas page")
	}
}
