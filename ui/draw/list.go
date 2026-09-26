package draw

import (
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
)

type command struct {
	kind   cmdKind
	rect   mathx.Rect
	color  mathx.Color
	region texture.Region
	textAt int
	textN  int
	order  int16
}

type List struct {
	streams [Layers][]command
	order   [Layers]int16
	chars   []byte
	layer   int
	stack   []int
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
	if c.kind != cmdPushClip && c.kind != cmdPopClip {
		l.order[l.layer]++
		c.order = l.order[l.layer]
	}
	l.streams[l.layer] = append(l.streams[l.layer], c)
}

func (l *List) Rect(r mathx.Rect, c mathx.Color) {
	l.add(command{kind: cmdRect, rect: r, color: c})
}

func (l *List) Reserve() int {
	l.add(command{kind: cmdRect})
	return l.layer<<24 | (len(l.streams[l.layer]) - 1)
}

func (l *List) Patch(i int, r mathx.Rect, c mathx.Color) {
	layer, idx := i>>24, i&0xffffff
	if layer < 0 || layer >= Layers || idx >= len(l.streams[layer]) {
		return
	}
	l.streams[layer][idx].rect = r
	l.streams[layer][idx].color = c
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
	l.add(command{kind: cmdText, rect: mathx.Rect{X: pos.X, Y: pos.Y}, color: c, textAt: at, textN: len(s)})
}

func (l *List) PushClip(r mathx.Rect) {
	l.add(command{kind: cmdPushClip, rect: r})
}

func (l *List) PopClip() {
	l.add(command{kind: cmdPopClip})
}

func (l *List) Flush(f *render.ViewFrame, font *text.Font, white texture.Region, layer int16) {
	for li := range l.streams {
		lay := layer + int16(li)
		depth := 0
		for i := range l.streams[li] {
			c := &l.streams[li][i]
			switch c.kind {
			case cmdPushClip:
				f.PushClip(c.rect)
				depth++
			case cmdPopClip:
				if depth > 0 {
					f.PopClip()
					depth--
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
				if font == nil {
					continue
				}
				s := unsafeString(l.chars[c.textAt : c.textAt+c.textN])
				f.TextFont(font, s, mathx.Vec2{X: c.rect.X, Y: c.rect.Y}, 1, c.color, lay, c.order)
			}
		}
		for ; depth > 0; depth-- {
			f.PopClip()
		}
	}
}
