package editor

import (
	"aego/art"
	"aego/asset"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/gpu"
	"aego/render/texture"
	"aego/ui/core"
)

type artThumbnailKey struct {
	layer, frame art.ID
	channel      art.Channel
}

type artThumbnail struct {
	display          imageDisplay
	pixels           *imagePixels
	owner            *art.Document
	revision, device uint64
	used             uint64
}

func (v *artView) close(ed *context.Editor) {
	v.display.close(ed)
	v.toolSample.display.close(ed)
	for key, thumb := range v.thumbnails {
		thumb.display.close(ed)
		delete(v.thumbnails, key)
	}
}

// Both layer and cel thumbnails sample the canonical Go compositor. The small
// LRU bounds CPU/GPU allocations when scrolling large documents; a frame switch
// never decodes the document or allocates a full-resolution layer image.
func (p *artPanel) thumbnail(ui *core.Context, r mathx.Rect, layer, frame art.ID) {
	if r.W <= 0 || r.H <= 0 || p.ctx.Renderer == nil {
		return
	}
	v, s := p.view, p.model.Session
	device, lost := imageDevice(p.ctx)
	if lost {
		return
	}
	if v.thumbnails == nil {
		v.thumbnails = make(map[artThumbnailKey]*artThumbnail)
	}
	key := artThumbnailKey{layer, frame, s.Channel}
	thumb := v.thumbnails[key]
	if thumb == nil {
		if len(v.thumbnails) >= 256 {
			var oldest artThumbnailKey
			var use uint64 = ^uint64(0)
			for k, value := range v.thumbnails {
				if value.used < use {
					oldest, use = k, value.used
				}
			}
			v.thumbnails[oldest].display.close(p.ctx)
			delete(v.thumbnails, oldest)
		}
		thumb = &artThumbnail{}
		v.thumbnails[key] = thumb
	}
	v.thumbnailClock++
	thumb.used = v.thumbnailClock
	revision := s.Document.Revision()
	if thumb.owner != s.Document || thumb.revision != revision || thumb.device != device || thumb.pixels == nil {
		thumb.display.close(p.ctx)
		pixels, err := s.Document.Thumbnail(layer, frame, art.RenderOptions{Channel: s.Channel, References: true}, 32)
		if err != nil {
			return
		}
		thumb.owner, thumb.revision, thumb.device = s.Document, revision, device
		thumb.pixels = &imagePixels{data: texture.Data{Width: 32, Height: 32, Format: gpu.TexSRGBA8, Pixels: pixels.Pix}, semantic: asset.TextureColor}
	}
	drawImageBackground(ui, r, 0)
	region := thumb.display.region(p.ctx, thumb.pixels, imageRGBA)
	if !region.IsZero() {
		ui.List().Texture(region, r, mathx.White)
	}
}
