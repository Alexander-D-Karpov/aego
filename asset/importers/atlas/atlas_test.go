package atlas

import (
	"testing"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
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
