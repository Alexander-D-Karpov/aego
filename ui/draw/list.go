package draw

import (
	"math"

	"aego/core/mathx"
	"aego/render"
	"aego/render/sprite"
	"aego/render/text"
	"aego/render/texture"
)

const (
	LayerBase = iota
	LayerPopup
	LayerModal
	LayerTop
	Layers
)

type cmdKind uint8

const (
	cmdRect cmdKind = iota
	cmdTexture
	cmdText
	cmdPushClip
	cmdPopClip
	cmdPushClipAbs
	cmdRound
	cmdBox
	cmdShadow
)

type command struct {
	kind    cmdKind
	rect    mathx.Rect
	color   mathx.Color
	edge    mathx.Color
	region  texture.Region
	textAt  int
	textN   int
	order   int16
	corners Corners
	radius  float32
	blur    float32
	dy      float32
	font    *text.Font
}

type List struct {
	streams   [Layers][]command
	order     [Layers]int16
	chars     []byte
	layer     int
	stack     []int
	clipStack []mathx.Rect
	font      *text.Font
	shapes    *Shapes
}

func NewList(capacity int) *List {
	l := &List{chars: make([]byte, 0, capacity*8), stack: make([]int, 0, 8)}
	for i := range l.streams {
		l.streams[i] = make([]command, 0, capacity)
	}
	return l
}

func (l *List) Reset() {
	for i := range l.streams {
		l.streams[i] = l.streams[i][:0]
		l.order[i] = 0
	}
	l.chars = l.chars[:0]
	l.layer = LayerBase
	l.stack = l.stack[:0]
	l.font = nil
}

func (l *List) SetFont(f *text.Font) {
	l.font = f
}

func (l *List) Font() *text.Font {
	return l.font
}

func (l *List) SetShapes(s *Shapes) {
	l.shapes = s
}

func (l *List) Shapes() *Shapes {
	return l.shapes
}

func (l *List) Empty() bool {
	for i := range l.streams {
		if len(l.streams[i]) > 0 {
			return false
		}
	}
	return true
}

func (l *List) Layer() int {
	return l.layer
}

func (l *List) LayerDepth() int {
	return len(l.stack)
}

func (l *List) PushLayer(n int) {
	if n < 0 {
		n = 0
	}
	if n >= Layers {
		n = Layers - 1
	}
	l.stack = append(l.stack, l.layer)
	l.layer = n
}

func (l *List) PopLayer() {
	if n := len(l.stack); n > 0 {
		l.layer = l.stack[n-1]
		l.stack = l.stack[:n-1]
	}
}

func (l *List) add(c command) {
	if c.kind != cmdPushClip && c.kind != cmdPopClip && c.kind != cmdPushClipAbs {
		l.order[l.layer]++
		c.order = l.order[l.layer]
	}
	l.streams[l.layer] = append(l.streams[l.layer], c)
}

func (l *List) Rect(r mathx.Rect, c mathx.Color) {
	l.add(command{kind: cmdRect, rect: r, color: c})
}

func (l *List) at(i int) *command {
	layer, idx := i>>24, i&0xffffff
	if layer < 0 || layer >= Layers || idx >= len(l.streams[layer]) {
		return nil
	}
	return &l.streams[layer][idx]
}

func (l *List) reserve(k cmdKind) int {
	l.add(command{kind: k})
	return l.layer<<24 | (len(l.streams[l.layer]) - 1)
}

func (l *List) Reserve() int {
	return l.reserve(cmdRect)
}

func (l *List) ReserveBox() int {
	return l.reserve(cmdBox)
}

func (l *List) ReserveShadow() int {
	return l.reserve(cmdShadow)
}

func (l *List) Patch(i int, r mathx.Rect, c mathx.Color) {
	if cmd := l.at(i); cmd != nil {
		cmd.rect, cmd.color = r, c
	}
}

func (l *List) PatchBox(i int, r mathx.Rect, radius float32, fill, border mathx.Color) {
	if cmd := l.at(i); cmd != nil {
		cmd.rect, cmd.radius, cmd.color, cmd.edge = r, radius, fill, border
	}
}

func (l *List) PatchShadow(i int, r mathx.Rect, radius, blur, dy float32, c mathx.Color) {
	if cmd := l.at(i); cmd != nil {
		cmd.rect, cmd.radius, cmd.blur, cmd.dy, cmd.color = r, radius, blur, dy, c
	}
}

func (l *List) RoundRect(r mathx.Rect, radius float32, c mathx.Color, corners Corners) {
	l.add(command{kind: cmdRound, rect: r, color: c, radius: radius, corners: corners})
}

func (l *List) Box(r mathx.Rect, radius float32, fill, border mathx.Color) {
	l.add(command{kind: cmdBox, rect: r, color: fill, edge: border, radius: radius})
}

func (l *List) Shadow(r mathx.Rect, radius, blur, dy float32, c mathx.Color) {
	l.add(command{kind: cmdShadow, rect: r, color: c, radius: radius, blur: blur, dy: dy})
}

func (l *List) RectOutline(r mathx.Rect, c mathx.Color, thickness float32) {
	if thickness <= 0 {
		thickness = 1
	}
	l.Rect(mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: thickness}, c)
	l.Rect(mathx.Rect{X: r.X, Y: r.Y + r.H - thickness, W: r.W, H: thickness}, c)
	l.Rect(mathx.Rect{X: r.X, Y: r.Y + thickness, W: thickness, H: r.H - 2*thickness}, c)
	l.Rect(mathx.Rect{X: r.X + r.W - thickness, Y: r.Y + thickness, W: thickness, H: r.H - 2*thickness}, c)
}

func (l *List) Texture(reg texture.Region, r mathx.Rect, tint mathx.Color) {
	l.add(command{kind: cmdTexture, rect: r, color: tint, region: reg})
}

func (l *List) Text(s string, pos mathx.Vec2, c mathx.Color) {
	if s == "" {
		return
	}
	at := len(l.chars)
	l.chars = append(l.chars, s...)
	l.add(command{kind: cmdText, rect: mathx.Rect{X: pos.X, Y: pos.Y}, color: c, textAt: at, textN: len(s), font: l.font})
}

func (l *List) PushClip(r mathx.Rect) {
	l.add(command{kind: cmdPushClip, rect: r})
}

func (l *List) PopClip() {
	l.add(command{kind: cmdPopClip})
}

func (l *List) PushClipAbs(r mathx.Rect) {
	l.add(command{kind: cmdPushClipAbs, rect: r})
}

func intersectRect(a, b mathx.Rect) mathx.Rect {
	x0, y0 := max(a.X, b.X), max(a.Y, b.Y)
	x1, y1 := min(a.X+a.W, b.X+b.W), min(a.Y+a.H, b.Y+b.H)
	return mathx.Rect{X: x0, Y: y0, W: max(0, x1-x0), H: max(0, y1-y0)}
}

func snap(p mathx.Vec2, scale float32) mathx.Vec2 {
	return mathx.Vec2{X: float32(math.Round(float64(p.X*scale))) / scale, Y: float32(math.Round(float64(p.Y*scale))) / scale}
}

func quad(f *render.ViewFrame, reg texture.Region, x, y, w, h float32, col mathx.Color, lay, order int16) {
	if w <= 0 || h <= 0 {
		return
	}
	f.Sprite(sprite.Draw{Region: reg, Position: mathx.Vec2{X: x, Y: y}, Size: mathx.Vec2{X: w, Y: h}, Tint: col, Layer: lay, Order: order})
}

func (l *List) round(f *render.ViewFrame, r mathx.Rect, radius float32, col mathx.Color, corners Corners, white texture.Region, lay, order int16) {
	s := l.shapes
	rad := min(radius, r.W/2, r.H/2)
	if s == nil || rad < 1 || corners == 0 {
		quad(f, white, r.X, r.Y, r.W, r.H, col, lay, order)
		return
	}
	w := s.White
	disc := s.discs(rad)
	quad(f, w, r.X+rad, r.Y, r.W-2*rad, r.H, col, lay, order)
	quad(f, w, r.X, r.Y+rad, rad, r.H-2*rad, col, lay, order)
	quad(f, w, r.X+r.W-rad, r.Y+rad, rad, r.H-2*rad, col, lay, order)
	xs := [4]float32{r.X, r.X + r.W - rad, r.X, r.X + r.W - rad}
	ys := [4]float32{r.Y, r.Y, r.Y + r.H - rad, r.Y + r.H - rad}
	for i := range 4 {
		reg := w
		if corners&(1<<i) != 0 {
			reg = disc[i]
		}
		quad(f, reg, xs[i], ys[i], rad, rad, col, lay, order)
	}
}

func (l *List) box(f *render.ViewFrame, c *command, white texture.Region, lay int16) {
	if c.edge.A <= 0 {
		l.round(f, c.rect, c.radius, c.color, CornersAll, white, lay, c.order)
		return
	}
	l.round(f, c.rect, c.radius, c.edge, CornersAll, white, lay, c.order)
	in := mathx.Rect{X: c.rect.X + 1, Y: c.rect.Y + 1, W: c.rect.W - 2, H: c.rect.H - 2}
	l.round(f, in, max(c.radius-1, 0), c.color, CornersAll, white, lay, c.order)
}

func (l *List) shadow(f *render.ViewFrame, c *command, lay int16) {
	s := l.shapes
	if s == nil || c.color.A <= 0 || c.rect.W <= 0 || c.rect.H <= 0 {
		return
	}
	b := c.blur
	r := mathx.Rect{X: c.rect.X - b, Y: c.rect.Y - b + c.dy, W: c.rect.W + 2*b, H: c.rect.H + 2*b}
	k := min(c.radius+b, r.W/2, r.H/2)
	if k < 1 {
		return
	}
	col, o := c.color, c.order
	quad(f, s.White, r.X+k, r.Y+k, r.W-2*k, r.H-2*k, col, lay, o)
	quad(f, s.edge[0], r.X+k, r.Y, r.W-2*k, k, col, lay, o)
	quad(f, s.edge[1], r.X+k, r.Y+r.H-k, r.W-2*k, k, col, lay, o)
	quad(f, s.edge[2], r.X, r.Y+k, k, r.H-2*k, col, lay, o)
	quad(f, s.edge[3], r.X+r.W-k, r.Y+k, k, r.H-2*k, col, lay, o)
	xs := [4]float32{r.X, r.X + r.W - k, r.X, r.X + r.W - k}
	ys := [4]float32{r.Y, r.Y, r.Y + r.H - k, r.Y + r.H - k}
	for i := range 4 {
		quad(f, s.blur[i], xs[i], ys[i], k, k, col, lay, o)
	}
}

func (l *List) Flush(f *render.ViewFrame, font *text.Font, white texture.Region, layer int16) {
	for li := range l.streams {
		lay := layer + int16(li)
		l.clipStack = l.clipStack[:0]
		for i := range l.streams[li] {
			c := &l.streams[li][i]
			switch c.kind {
			case cmdPushClip, cmdPushClipAbs:
				r := c.rect
				n := len(l.clipStack)
				if c.kind == cmdPushClip && n > 0 {
					r = intersectRect(r, l.clipStack[n-1])
				}
				if n > 0 {
					f.PopClip()
				}
				l.clipStack = append(l.clipStack, r)
				f.PushClip(r)
			case cmdPopClip:
				n := len(l.clipStack)
				if n == 0 {
					continue
				}
				l.clipStack = l.clipStack[:n-1]
				f.PopClip()
				if n > 1 {
					f.PushClip(l.clipStack[n-2])
				}
			case cmdRect:
				if c.rect.W <= 0 || c.rect.H <= 0 {
					continue
				}
				f.Sprite(sprite.Draw{
					Region:   white,
					Position: mathx.Vec2{X: c.rect.X, Y: c.rect.Y},
					Size:     mathx.Vec2{X: c.rect.W, Y: c.rect.H},
					Tint:     c.color,
					Layer:    lay,
					Order:    c.order,
				})
			case cmdTexture:
				f.Sprite(sprite.Draw{
					Region:   c.region,
					Position: mathx.Vec2{X: c.rect.X, Y: c.rect.Y},
					Size:     mathx.Vec2{X: c.rect.W, Y: c.rect.H},
					Tint:     c.color,
					Layer:    lay,
					Order:    c.order,
				})
			case cmdText:
				fnt := c.font
				if fnt == nil {
					fnt = font
				}
				if fnt == nil {
					continue
				}
				s := unsafeString(l.chars[c.textAt : c.textAt+c.textN])
				f.TextFont(fnt, s, snap(mathx.Vec2{X: c.rect.X, Y: c.rect.Y}, fnt.Scale), 1, c.color, lay, c.order)
			case cmdRound:
				l.round(f, c.rect, c.radius, c.color, c.corners, white, lay, c.order)
			case cmdBox:
				l.box(f, c, white, lay)
			case cmdShadow:
				l.shadow(f, c, lay)
			}
		}
		if len(l.clipStack) > 0 {
			f.PopClip()
		}
	}
}

func (l *List) ClipDepth() [Layers]int {
	var out [Layers]int
	for li := range l.streams {
		for i := range l.streams[li] {
			switch l.streams[li][i].kind {
			case cmdPushClip, cmdPushClipAbs:
				out[li]++
			case cmdPopClip:
				out[li]--
			}
		}
	}
	return out
}
