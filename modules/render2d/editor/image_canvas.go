package editor

import (
	"fmt"
	"math"

	"aego/asset"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/gpu"
	"aego/platform/input"
	"aego/render/texture"
	"aego/ui/core"
)

type imageDisplay struct {
	pixels  *imagePixels
	channel imageChannel
	handle  gpu.TextureHandle
	err     error
}

func (d *imageDisplay) region(ed *context.Editor, pixels *imagePixels, channel imageChannel) texture.Region {
	if ed.Renderer == nil || pixels == nil {
		return texture.Region{}
	}
	if d.pixels != pixels || d.channel != channel {
		d.close(ed)
		d.pixels, d.channel = pixels, channel
		data, err := pixels.projection(channel)
		if err == nil {
			d.handle, err = ed.Renderer.Device().CreateTexture(gpu.TextureDesc{Width: data.Width, Height: data.Height, Format: data.Format, Data: data.Pixels, Label: "image.channels"})
		}
		d.err = err
	}
	return texture.Whole(d.handle, pixels.data.Width, pixels.data.Height)
}

func (d *imageDisplay) close(ed *context.Editor) {
	if !d.handle.IsZero() && ed.Renderer != nil {
		ed.Renderer.Device().DestroyTexture(d.handle)
	}
	*d = imageDisplay{}
}

func drawImageBackground(ui *core.Context, area mathx.Rect, mode int) {
	if mode == 1 {
		ui.Rect(area, mathx.Color{R: .72, G: .72, B: .72, A: 1})
		return
	}
	if mode == 2 {
		ui.Rect(area, mathx.Color{R: .025, G: .025, B: .025, A: 1})
		return
	}
	const cell = float32(12)
	for y := float32(0); y < area.H; y += cell {
		for x := float32(0); x < area.W; x += cell {
			shade := float32(.13)
			if (int(x/cell)+int(y/cell))%2 != 0 {
				shade = .25
			}
			ui.Rect(mathx.Rect{X: area.X + x, Y: area.Y + y, W: min(cell, area.W-x), H: min(cell, area.H-y)}, mathx.Color{R: shade, G: shade, B: shade, A: 1})
		}
	}
}

// croppedPixels preserves raw bytes. Imported frames are reconstructed in
// their logical source canvas; Open Atlas is the separate packed-page view.
func croppedPixels(p *imagePixels, crop [4]int, logical [4]int) (*imagePixels, error) {
	if p == nil {
		return nil, nil
	}
	x, y, w, h := crop[0], crop[1], crop[2], crop[3]
	if x < 0 || y < 0 || w <= 0 || h <= 0 || x+w > p.data.Width || y+h > p.data.Height {
		return nil, fmt.Errorf("frame rectangle is outside its image")
	}
	lw, lh, tx, ty := logical[0], logical[1], logical[2], logical[3]
	if lw == 0 || lh == 0 {
		lw, lh = w, h
	}
	if lw <= 0 || lh <= 0 || lw > maxViewerPixels/lh || tx < 0 || ty < 0 || tx+w > lw || ty+h > lh {
		return nil, fmt.Errorf("invalid logical frame or trim rectangle")
	}
	if x == 0 && y == 0 && w == p.data.Width && h == p.data.Height && lw == w && lh == h {
		return p, nil
	}
	d := texture.New(lw, lh, p.data.Format)
	d.Premultiplied, d.NonColor = p.data.Premultiplied, p.data.NonColor
	for row := range h {
		copy(d.Pixels[((ty+row)*lw+tx)*4:((ty+row)*lw+tx+w)*4], p.data.Pixels[((y+row)*p.data.Width+x)*4:((y+row)*p.data.Width+x+w)*4])
	}
	return &imagePixels{data: *d, semantic: p.semantic}, nil
}

func (c *imageContent) sourcePixels() (*imagePixels, error) {
	if c.Source == nil {
		return nil, nil
	}
	return croppedPixels(c.Source, c.SourceRect, [4]int{})
}

func (c *imageContent) importedPixels() (*imagePixels, error) {
	if c.Imported == nil {
		return nil, nil
	}
	r := c.Region
	return croppedPixels(c.Imported, [4]int{r.X, r.Y, r.W, r.H}, [4]int{r.SourceW, r.SourceH, r.TrimX, r.TrimY})
}

func imageRegionRect(r asset.RegionArtifact) [4]int { return [4]int{r.X, r.Y, r.W, r.H} }

func drawImageCanvas(ed *context.Editor, area mathx.Rect, pixels *imagePixels, display *imageDisplay, viewport *imageViewport, channel imageChannel, background int, grid bool, highlight [4]int, pivot *[2]float32) string {
	ui := ed.UI
	drawImageBackground(ui, area, background)
	if pixels == nil || area.W < 1 || area.H < 1 {
		return ""
	}
	w, h := pixels.data.Width, pixels.data.Height
	if !viewport.Ready {
		viewport.fit(area, w, h)
	}
	id := ui.ID("image.canvas")
	hover := ui.MouseOver(id, area)
	if hover {
		viewport.zoomAt(area, ui.Mouse(), ui.ZoomY())
		if ui.MousePressed(input.MouseMiddle) || ui.MousePressed(input.MouseRight) {
			ui.CaptureMouse(id)
		}
	}
	if ui.Captured() == id {
		viewport.pan(ui.MouseDelta())
		if !ui.MouseDown(input.MouseMiddle) && !ui.MouseDown(input.MouseRight) {
			ui.ReleaseMouse()
		}
	}
	ui.PushClip(area)
	reg := display.region(ed, pixels, channel)
	if !reg.IsZero() {
		ui.List().Texture(reg, viewport.rect(area, w, h), mathx.White)
	}
	if grid && viewport.Zoom*ui.Scale() >= 8 {
		minp := viewport.imagePoint(area, mathx.Vec2{X: area.X, Y: area.Y})
		maxp := viewport.imagePoint(area, mathx.Vec2{X: area.X + area.W, Y: area.Y + area.H})
		col := ui.Theme.Border
		col.A = .6
		for x := max(0, int(math.Floor(float64(minp.X)))); x <= min(w, int(math.Ceil(float64(maxp.X)))); x++ {
			a := viewport.screenPoint(area, mathx.Vec2{X: float32(x)})
			b := viewport.screenPoint(area, mathx.Vec2{X: float32(x), Y: float32(h)})
			ui.Line(a, b, 1/ui.Scale(), col)
		}
		for y := max(0, int(math.Floor(float64(minp.Y)))); y <= min(h, int(math.Ceil(float64(maxp.Y)))); y++ {
			a := viewport.screenPoint(area, mathx.Vec2{Y: float32(y)})
			b := viewport.screenPoint(area, mathx.Vec2{X: float32(w), Y: float32(y)})
			ui.Line(a, b, 1/ui.Scale(), col)
		}
	}
	if highlight[2] > 0 && highlight[3] > 0 {
		a := viewport.screenPoint(area, mathx.Vec2{X: float32(highlight[0]), Y: float32(highlight[1])})
		ui.List().RectOutline(mathx.Rect{X: a.X, Y: a.Y, W: float32(highlight[2]) * viewport.Zoom, H: float32(highlight[3]) * viewport.Zoom}, ui.Theme.Accent, 2)
	}
	if pivot != nil {
		p := viewport.screenPoint(area, mathx.Vec2{X: (*pivot)[0] * float32(w), Y: (*pivot)[1] * float32(h)})
		ui.Line(p.Sub(mathx.Vec2{X: 6}), p.Add(mathx.Vec2{X: 6}), 1, ui.Theme.Accent)
		ui.Line(p.Sub(mathx.Vec2{Y: 6}), p.Add(mathx.Vec2{Y: 6}), 1, ui.Theme.Accent)
	}
	ui.PopClip()
	if display.err != nil {
		return display.err.Error()
	}
	if hover {
		p := viewport.imagePoint(area, ui.Mouse())
		x, y := int(math.Floor(float64(p.X))), int(math.Floor(float64(p.Y)))
		if rgba, ok := pixels.pixel(x, y); ok {
			return fmt.Sprintf("x %d  y %d  ·  RGBA8 %d %d %d %d  ·  %.1f%%", x, y, rgba[0], rgba[1], rgba[2], rgba[3], viewport.Zoom*ui.Scale()*100)
		}
	}
	return fmt.Sprintf("%d × %d  ·  %.1f%%", w, h, viewport.Zoom*ui.Scale()*100)
}
