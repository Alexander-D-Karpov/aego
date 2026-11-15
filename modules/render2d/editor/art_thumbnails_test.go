package editor

import (
	stdctx "context"
	"testing"

	"aego/art"
	"aego/core/mathx"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render"
)

func exerciseArtThumbnailResources(t *testing.T, dev gpu.Device, renderer *render.Renderer) {
	t.Helper()
	p := artActionFixture(t)
	p.ctx.Renderer = renderer
	for range 32 {
		if _, err := p.model.Execute(stdctx.Background(), art.Operation{Kind: "add-layer", LayerKind: art.Raster, Name: "Thumbnail"}); err != nil {
			t.Fatal(err)
		}
	}
	info := p.model.Session.Document.Info()
	report := dev.(gpu.ResourceReporter)
	baseline := report.ResourceStats()
	draw := func(all bool) {
		p.ctx.UI.Begin(nil, mathx.Vec2{X: 500, Y: 500}, 1)
		for i, layer := range info.Layers {
			if !all && i >= 2 {
				break
			}
			for _, frame := range info.Frames {
				p.thumbnail(p.ctx.UI, mathx.Rect{W: 24, H: 24}, layer.ID, frame.ID)
			}
		}
		p.ctx.UI.End()
	}
	draw(true)
	if len(p.view.thumbnails) != 256 || report.ResourceStats().Textures-baseline.Textures != 256 {
		t.Fatal("thumbnail GPU cache is not bounded", len(p.view.thumbnails), report.ResourceStats(), baseline)
	}
	p.view.close(p.ctx)
	if report.ResourceStats() != baseline {
		t.Fatal("evicted or closed thumbnail resources leaked", report.ResourceStats(), baseline)
	}
	for cycle := 0; cycle < 30; cycle++ {
		draw(false)
		if report.ResourceStats().Textures <= baseline.Textures {
			t.Fatal("thumbnail cycle did not allocate real texture objects")
		}
		p.view.close(p.ctx)
		if report.ResourceStats() != baseline {
			t.Fatalf("thumbnail open/close cycle %d leaked: %+v, baseline %+v", cycle, report.ResourceStats(), baseline)
		}
	}
}

func TestArtThumbnailsBoundedResourcesAndThirtyCloseCycles(t *testing.T) {
	dev := null.New()
	defer dev.Close()
	exerciseArtThumbnailResources(t, dev, imageTestRenderer(t, dev))
}
