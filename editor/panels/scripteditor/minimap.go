package scripteditor

import (
	"aego/core/mathx"
	"aego/editor/core/code/highlight"
	"aego/editor/core/code/minimap"
	"aego/gpu"
	"aego/platform/input"
	"aego/render/texture"
	"aego/script/diag"
	"aego/ui/core"
)

const (
	mapBase   = 700
	mapWidth  = minimap.Cols + 10
	mapMinW   = 420
	emptyLine = uint64(14695981039346656037)
	mapAlpha  = 0.9
)

type mapView struct {
	tiles  []gpu.TextureHandle
	hashes []uint64
	rev    uint64
	ready  bool
	pal    minimap.Palette
	palOK  bool
	dark   bool
	buf    []byte
	line   []byte
	spans  []highlight.Span
}

func mapPalette(th *core.Theme) minimap.Palette {
	pal := palette(th)
	var out minimap.Palette
	enc := func(v float32) byte {
		return byte(mathx.Clamp(mathx.LinearToSRGB(mathx.Clamp(v, 0, 1))*mapAlpha, 0, 1)*255 + 0.5)
	}
	for k := range pal {
		c := pal[k]
		out[k] = [4]byte{enc(c.R), enc(c.G), enc(c.B), byte(mapAlpha*255 + 0.5)}
	}
	return out
}

func (m *mapView) update(p *Panel, th *core.Theme) {
	c := p.doc.Code
	if m.palOK && m.dark != th.Dark {
		m.palOK = false
		for i := range m.hashes {
			m.hashes[i] = ^uint64(0)
		}
	}
	if m.ready && m.palOK && m.rev == c.Rev() {
		return
	}
	if !m.palOK {
		m.pal, m.palOK, m.dark = mapPalette(th), true, th.Dark
	}
	n := c.Lines()
	need := (n + minimap.TileLines - 1) / minimap.TileLines
	up := p.host.d.Viewport.Renderer.Textures()
	for len(m.tiles) < need {
		data := texture.New(minimap.Cols, minimap.TileH, gpu.TexSRGBA8)
		data.Premultiplied = true
		tex, err := up.Upload(data, texture.UploadOptions{Label: "code.minimap"})
		if err != nil {
			return
		}
		m.tiles = append(m.tiles, tex)
	}
	for len(m.hashes) < len(m.tiles)*minimap.TileLines {
		m.hashes = append(m.hashes, ^uint64(0))
	}
	dev := p.host.d.Viewport.GPU
	for t := 0; t < need; t++ {
		lo, hi := -1, -1
		for r := 0; r < minimap.TileLines; r++ {
			l := t*minimap.TileLines + r
			h := emptyLine
			if l < n {
				m.line = c.LineBytes(l, m.line[:0])
				h = hashBytes(m.line) ^ uint64(c.LineState(l))<<1
			}
			if m.hashes[l] == h {
				continue
			}
			m.hashes[l] = h
			if lo < 0 {
				lo = r
			}
			hi = r
		}
		if lo < 0 {
			continue
		}
		size := (hi - lo + 1) * minimap.RowBytes
		if cap(m.buf) < size {
			m.buf = make([]byte, size)
		}
		m.buf = m.buf[:size]
		for r := lo; r <= hi; r++ {
			l := t*minimap.TileLines + r
			dst := m.buf[(r-lo)*minimap.RowBytes : (r-lo+1)*minimap.RowBytes]
			if l >= n {
				clear(dst)
				continue
			}
			m.line = c.LineBytes(l, m.line[:0])
			m.spans, _ = highlight.Line(m.line, c.LineState(l), m.spans[:0])
			minimap.Paint(dst, m.line, m.spans, &m.pal)
		}
		dev.UpdateTexture(m.tiles[t], 0, lo*minimap.LineH, minimap.Cols, (hi-lo+1)*minimap.LineH, m.buf)
	}
	m.rev, m.ready = c.Rev(), true
}

func (m *mapView) draw(p *Panel, ui *core.Context, r mathx.Rect, first int) {
	th := &ui.Theme
	c := p.doc.Code
	n := c.Lines()
	lh := float32(minimap.LineH)
	total := float32(n) * lh
	maxScroll := max(0, float32(n)*p.lineH-r.H+p.lineH)
	off := float32(0)
	if total > r.H && maxScroll > 0 {
		off = mathx.Clamp(p.scrollY/maxScroll, 0, 1) * (total - r.H)
	}
	id := core.IDSeq(p.fid, mapBase)
	if ui.Pressed(id, r) {
		ui.CaptureMouse(id)
	}
	if ui.Active() == id {
		if ui.MouseDown(input.MouseLeft) {
			l := (ui.Mouse().Y - r.Y + off) / lh
			p.scrollY = l*p.lineH - r.H/2
		} else {
			ui.ReleaseMouse()
		}
	}
	hot := ui.Hot() == id || ui.Active() == id
	if hot {
		ui.SetCursorShape(core.CursorPointer)
	}
	ui.Rect(r, th.Code.GutterBg)
	ui.PushClip(r)
	x := r.X + 5
	for t, tex := range m.tiles {
		h := min(minimap.TileH, n*minimap.LineH-t*minimap.TileH)
		if h <= 0 {
			break
		}
		y := r.Y + float32(t*minimap.TileH) - off
		if y > r.Y+r.H || y+float32(h) < r.Y {
			continue
		}
		reg := texture.Whole(tex, minimap.Cols, minimap.TileH).Sub(0, 0, minimap.Cols, h)
		ui.List().Texture(reg, mathx.Rect{X: x, Y: y, W: minimap.Cols, H: float32(h)}, mathx.White)
	}
	for l, sev := range p.marks {
		col := th.Error
		if sev == diag.Warning {
			col = th.Warn
		}
		y := r.Y + float32(l)*lh - off
		band := col
		band.A = 0.35
		ui.Rect(mathx.Rect{X: r.X, Y: y, W: r.W, H: lh}, band)
		ui.Rect(mathx.Rect{X: r.X + r.W - 3, Y: y - 1, W: 3, H: lh + 2}, col)
	}
	ui.Rect(mathx.Rect{X: r.X, Y: r.Y + float32(c.Line(c.Cursor()))*lh - off, W: r.W, H: 1}, th.Accent)
	vr := mathx.Rect{X: r.X, Y: r.Y + float32(first)*lh - off, W: r.W, H: float32(p.viewLines+1) * lh}
	vc := ui.Theme.Code.MinimapView
	if hot {
		vc.A = 0.11
	}
	ui.Rect(vr, vc)
	ui.Outline(vr, th.Border)
	ui.PopClip()
}

func (p *Panel) dispose() {
	dev := p.host.d.Viewport.GPU
	for _, t := range p.mini.tiles {
		if !t.IsZero() {
			dev.DestroyTexture(t)
		}
	}
	p.mini = mapView{}
}
