package render

import (
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/culling"
	"aego/render/debug"
	"aego/render/sprite"
	"aego/render/text"
	"aego/render/view"
)

const maxClipDepth = 16

type ViewFrame struct {
	r          *Renderer
	view       view.View
	pass       gpu.Pass
	bounds     mathx.AABB2
	frustum    mathx.Frustum
	cam        mathx.Vec3
	clips      [maxClipDepth]mathx.Rect
	clipOn     [maxClipDepth]bool
	depth      int
	submission uint32
	ended      bool
}

func (f *ViewFrame) reset(v view.View) {
	r := f.r
	w, h := r.framebuffer()
	v.Normalize(w, h, r.fbScale)
	f.view = v
	f.bounds = v.VisibleBounds()
	f.frustum = v.Frustum()
	f.cam = v.CameraPosition()
	f.depth = 0
	f.submission = 0
	f.ended = false
	f.pass = nil
	r.batcher.Reset()
	r.batcher.SetView(v.ZMin, v.ZMax, v.PixelsPerUnit)
	r.sprites3d.Reset()
	r.meshes.Reset()
	r.dbg.Reset()
}

func (f *ViewFrame) End() {
	if f.ended {
		return
	}
	f.ended = true
	r := f.r
	r.batcher.Sort()
	r.sprites3d.Sort()
	r.meshes.Sort()
	r.stats.ZClamped += r.batcher.Clamped()
	if f.pass == nil {
		return
	}
	r.passCtx = PassContext{Renderer: r, View: &f.view, Pass: f.pass, Frame: f}
	for i := range r.order {
		z := r.tracer.Begin(r.passZone[i])
		r.order[i].Pass.Execute(&r.passCtx)
		z.End()
	}
	if r.stats.Views == 1 {
		r.prof.End(f.pass)
	}
	f.pass.End()
	f.pass = nil
}
func (f *ViewFrame) View() *view.View {
	return &f.view
}

func (f *ViewFrame) Debug() *debug.Draw {
	return f.r.dbg
}

func (f *ViewFrame) PushClip(r mathx.Rect) {
	if f.depth >= maxClipDepth {
		return
	}
	scale := f.view.PixelScale
	px := mathx.Rect{X: r.X * scale, Y: r.Y * scale, W: r.W * scale, H: r.H * scale}
	if f.depth > 0 && f.clipOn[f.depth-1] {
		px = intersect(px, f.clips[f.depth-1])
	}
	f.clips[f.depth] = px
	f.clipOn[f.depth] = true
	f.depth++
}

func (f *ViewFrame) PopClip() {
	if f.depth > 0 {
		f.depth--
	}
}

func intersect(a, b mathx.Rect) mathx.Rect {
	x0 := max(a.X, b.X)
	y0 := max(a.Y, b.Y)
	x1 := min(a.X+a.W, b.X+b.W)
	y1 := min(a.Y+a.H, b.Y+b.H)
	return mathx.Rect{X: x0, Y: y0, W: max(0, x1-x0), H: max(0, y1-y0)}
}

func (f *ViewFrame) clip() (mathx.Rect, bool) {
	if f.depth == 0 {
		return mathx.Rect{}, false
	}
	return f.clips[f.depth-1], f.clipOn[f.depth-1]
}

func (f *ViewFrame) Sprite(d sprite.Draw) {
	f.submit(&d)
}

func (f *ViewFrame) Sprites(ds []sprite.Draw) {
	for i := range ds {
		f.submit(&ds[i])
	}
}

func (f *ViewFrame) submit(d *sprite.Draw) {
	r := f.r
	r.stats.Submitted++
	if d.Region.IsZero() {
		return
	}
	if !culling.Visible(f.bounds, d, f.view.PixelsPerUnit) {
		r.stats.Culled++
		return
	}
	clip, on := f.clip()
	if !r.batcher.Push(d, r.defSampler, clip, on) {
		r.stats.Dropped++
		if !r.warned {
			r.warned = true
			r.log.Warn("sprite budget exceeded, extra sprites are dropped")
		}
		return
	}
	r.stats.Visible++
	f.submission++
}

func (f *ViewFrame) Text(font *text.BitmapFont, s string, pos mathx.Vec2, scale float32, c mathx.Color, layer, order int16) {
	if font == nil || s == "" {
		return
	}
	r := f.r
	r.textBuf = font.Layout(s, pos, scale, c, layer, order, r.textBuf[:0])
	f.Sprites(r.textBuf)
}

func (f *ViewFrame) TextFont(font *text.Font, s string, pos mathx.Vec2, scale float32, c mathx.Color, layer, order int16) {
	if font == nil || s == "" {
		return
	}
	r := f.r
	r.textBuf = font.Layout(s, pos, scale, c, layer, order, r.textBuf[:0])
	f.Sprites(r.textBuf)
}
