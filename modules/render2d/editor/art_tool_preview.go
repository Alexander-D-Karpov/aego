package editor

import (
	"context"
	"crypto/sha256"
	"image"

	"aego/art"
	"aego/asset"
	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/gpu"
	"aego/render/texture"
	"aego/ui/core"
	"aego/ui/widgets"
)

type artToolSampleKey struct {
	session       *art.Session
	brushRevision uint64
	brush         art.Brush
	tool          int
	filled        bool
	channel       art.Channel
	mode          art.Mode
	palette       [32]byte
}

type artToolSample struct {
	key     artToolSampleKey
	valid   bool
	device  uint64
	pixels  *imagePixels
	display imageDisplay
	err     error
}

func artSampleOperation(tool int) (string, []image.Point, bool) {
	switch tool {
	case 0, 1:
		return "stroke", []image.Point{{8, 43}, {28, 26}, {47, 34}, {70, 16}, {96, 29}, {119, 17}}, true
	case 2:
		return "fill", []image.Point{{64, 32}}, true
	case 3:
		return "line", []image.Point{{9, 48}, {118, 15}}, true
	case 4:
		return "rectangle", []image.Point{{18, 12}, {109, 51}}, true
	case 5:
		return "ellipse", []image.Point{{18, 12}, {109, 51}}, true
	case 6:
		return "gradient", []image.Point{{8, 32}, {119, 32}}, true
	case artCurve:
		return "curve", []image.Point{{8, 44}, {119, 20}, {30, 4}, {91, 59}}, true
	case artPolygon:
		return "polygon", []image.Point{{12, 46}, {36, 13}, {73, 25}, {110, 11}, {117, 49}}, true
	default:
		return "", nil, false
	}
}

// Generate an example with the actual Core operations in a small, private
// document. The active layer, selection, pixels and Undo history are untouched.
func artRenderToolSample(s *art.Session, brush art.Brush, tool int, filled bool) (*image.NRGBA, error) {
	kind, points, ok := artSampleOperation(tool)
	if !ok {
		return nil, nil
	}
	mode := s.Document.Info().Canvas.Mode
	if s.Channel != art.Albedo || tool == 1 {
		mode = art.RGBA
	}
	doc, err := art.New(128, 64, mode, art.DefaultLimits())
	if err != nil {
		return nil, err
	}
	defer doc.Close()
	info := doc.Info()
	edit, err := doc.Begin(doc.Revision(), "Tool sample")
	if err != nil {
		return nil, err
	}
	defer edit.Cancel()
	if mode == art.Indexed {
		if err := edit.ReplacePalette(s.Document.Info().Palette, false); err != nil {
			return nil, err
		}
	}
	cel := info.Cels[0].ID
	if tool == 1 {
		// Erasing transparent pixels is invisible, so show it on a neutral
		// opaque sample; the checkerboard behind the sample reveals the result.
		if err := edit.Fill(context.Background(), cel, 0, 0, 0, art.Paint{Color: art.Color{R: 112, G: 146, B: 184, A: 255}}, nil, true); err != nil {
			return nil, err
		}
		if brush.Smudge {
			contrast := art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 232, G: 120, B: 78, A: 255}}}
			if err := edit.Shape(cel, "rectangle", image.Pt(0, 0), image.Pt(63, 63), contrast, nil, true); err != nil {
				return nil, err
			}
		}
		brush.Erase = true
	}
	_, err = edit.Execute(context.Background(), art.Operation{Kind: kind, Cel: cel, Points: points, Brush: brush, Filled: filled}, nil)
	if err != nil {
		return nil, err
	}
	return edit.Render(info.Frames[0].ID, art.RenderOptions{Channel: art.Albedo})
}

func (p *artPanel) toolStrokePreview(ui *core.Context) {
	v, s := p.view, p.model.Session
	if _, _, ok := artSampleOperation(v.tool); !ok {
		widgets.LabelWrapped(ui, i18n.Tr("editor.art.tool_no_stroke", "This tool does not paint a stroke."), true)
		return
	}
	widgets.LabelDim(ui, i18n.Tr("editor.art.stroke_preview", "Stroke preview"))
	brush := p.currentBrush()
	info := s.Document.Info()
	key := artToolSampleKey{session: s, brushRevision: s.BrushRevision(), brush: brush, tool: v.tool, filled: v.filled, channel: s.Channel, mode: info.Canvas.Mode}
	if info.Canvas.Mode == art.Indexed && s.Channel == art.Albedo {
		colors := make([]byte, 0, len(info.Palette)*4)
		for _, swatch := range info.Palette {
			c := swatch.Color
			colors = append(colors, c.R, c.G, c.B, c.A)
		}
		key.palette = sha256.Sum256(colors)
	}
	sample := &v.toolSample
	device, lost := imageDevice(p.ctx)
	if lost {
		sample.display.close(p.ctx)
		return
	}
	if !sample.valid || sample.key != key || sample.device != device {
		sample.display.close(p.ctx)
		sample.key, sample.device, sample.valid = key, device, true
		sample.pixels, sample.err = nil, nil
		resolved, err := s.PreviewBrush(brush)
		if err == nil {
			var pixels *image.NRGBA
			pixels, err = artRenderToolSample(s, resolved, v.tool, v.filled)
			if err == nil && pixels != nil {
				sample.pixels = &imagePixels{data: texture.Data{Width: pixels.Bounds().Dx(), Height: pixels.Bounds().Dy(), Format: gpu.TexSRGBA8, Pixels: pixels.Pix}, semantic: asset.TextureColor}
			}
		}
		sample.err = err
	}
	if sample.err != nil {
		widgets.LabelWrapped(ui, sample.err.Error(), true)
		return
	}
	width := min(float32(256), max(0, ui.Cursor().W))
	if width <= 0 {
		return
	}
	row := ui.Cursor().Next(width / 2)
	area := mathx.Rect{X: row.X, Y: row.Y, W: width, H: row.H}
	drawImageBackground(ui, area, 0)
	if region := sample.display.region(p.ctx, sample.pixels, imageRGBA); !region.IsZero() {
		ui.List().Texture(region, area, mathx.White)
	}
	ui.Outline(area, ui.Theme.Border)
	ui.MouseOver(ui.ID("stroke-preview"), area)
	widgets.Tooltip(ui, ui.ID("stroke-preview"), i18n.Tr("editor.art.stroke_preview_hint", "Example on an empty layer; selection, layer blending and locks are not shown."))
}
