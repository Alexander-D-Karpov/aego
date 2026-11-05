package widgets

import (
	"aego/core/mathx"
	"aego/platform/input"
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/textedit"
	"strings"
	"unicode"
	"unicode/utf8"
)

type editLine struct {
	start, end int
	text       string
}

// Multiline holds the soft-wrap layout and selection for a single draft.
type Multiline struct {
	b      *textedit.Buffer
	source string
	width  float32
	font   *text.Font
	lines  []editLine
	first  int
}

func (m *Multiline) buffer(value string) *textedit.Buffer {
	if m.b == nil {
		m.b = textedit.New(value)
		m.b.Multiline = true
	} else if m.b.String() != value {
		m.b.Set(value)
	}
	return m.b
}

// Insert replaces the current selection without moving the caret to the end.
func (m *Multiline) Insert(value *string, text string) {
	b := m.buffer(*value)
	b.Insert(text)
	*value = b.String()
}

// RemoveBeforeCursor removes a typed trigger without losing the caret position.
func (m *Multiline) RemoveBeforeCursor(value *string, trigger rune) bool {
	b := m.buffer(*value)
	if b.HasSelection() || b.Cursor() == 0 || b.Runes()[b.Cursor()-1] != trigger {
		return false
	}
	b.Delete(textedit.Left, false)
	*value = b.String()
	return true
}

// ReplaceFirst changes a marker while retaining the user's caret and selection.
func (m *Multiline) ReplaceFirst(value *string, old, replacement string) bool {
	pos := strings.Index(*value, old)
	if pos < 0 || old == "" {
		return false
	}
	b := m.buffer(*value)
	start := utf8.RuneCountInString((*value)[:pos])
	end := start + utf8.RuneCountInString(old)
	inserted := utf8.RuneCountInString(replacement)
	adjust := func(n int) int {
		switch {
		case n <= start:
			return n
		case n >= end:
			return n + inserted - (end - start)
		default:
			return start + inserted
		}
	}
	cursor, anchor := adjust(b.Cursor()), adjust(b.Anchor())
	b.SetCursor(start, false)
	b.SetCursor(end, true)
	if replacement == "" {
		b.Delete(textedit.Right, false)
	} else {
		b.Insert(replacement)
	}
	b.SetCursor(anchor, false)
	b.SetCursor(cursor, true)
	*value = b.String()
	return true
}

func (m *Multiline) layout(c *core.Context, s string, width float32) {
	m.buffer(s)
	if m.source == s && m.width == width && m.font == c.Font() && len(m.lines) > 0 {
		return
	}
	if m.b.String() != s {
		m.b.Set(s)
	}
	m.source, m.width, m.font = s, width, c.Font()
	clear(m.lines)
	m.lines = m.lines[:0]
	rs := m.b.Runes()
	start, breakAt := 0, 0
	var x, breakX float32
	add := func(end int) { m.lines = append(m.lines, editLine{start, end, string(rs[start:end])}) }
	for i, r := range rs {
		if r == '\n' {
			add(i)
			start = i + 1
			breakAt = start
			x, breakX = 0, 0
			continue
		}
		w := float32(8)
		if m.font != nil {
			w = m.font.Advance(r)
		}
		if x+w > width && i > start {
			cut := i
			if breakAt > start {
				cut = breakAt
				x -= breakX
			} else {
				x = 0
			}
			add(cut)
			start, breakAt = cut, cut
			breakX = 0
		}
		x += w
		if unicode.IsSpace(r) {
			breakAt = i + 1
			breakX = x
		}
	}
	add(len(rs))
}
func (m *Multiline) Height(c *core.Context, s string, w float32, maxRows int) float32 {
	m.layout(c, s, max(1, w-10))
	rows := min(max(1, maxRows), len(m.lines))
	if rows == 1 {
		return c.Theme.RowHeight
	}
	return max(c.Theme.RowHeight, float32(rows)*c.LineHeight()+6)
}
func (m *Multiline) Draw(c *core.Context, id core.ID, r mathx.Rect, value *string, pasteImage func() bool) {
	m.layout(c, *value, max(1, r.W-10))
	h := max(float32(1), c.LineHeight())
	visible := max(1, int((r.H-5)/h))
	f := c.Font()
	inset, top := float32(5), r.Y+3
	if len(m.lines) == 1 {
		inset, top = 4, c.TextY(r)
	}
	point := func(line int, x float32) int {
		l := m.lines[line]
		if f == nil {
			return l.start
		}
		return l.start + f.IndexAtRunes(m.b.Runes()[l.start:l.end], x)
	}
	caretLine := func() int {
		for i := len(m.lines) - 1; i > 0; i-- {
			if m.b.Cursor() >= m.lines[i].start {
				return i
			}
		}
		return 0
	}
	m.first = min(m.first, max(0, len(m.lines)-visible))
	over := c.MouseOver(id, r)
	if over {
		c.SetCursorShape(core.CursorText)
	}
	pressed := c.Pressed(id, r)
	if pressed {
		c.SetFocus(id)
	}
	focused := c.Focus() == id
	if focused {
		old := m.b.Cursor()
		rev := m.b.Revision()
		if pressed || (c.Active() == id && c.MouseDown(input.MouseLeft) && c.MouseDelta() != (mathx.Vec2{})) {
			row := max(0, min(len(m.lines)-1, m.first+int((c.Mouse().Y-r.Y-3)/h)))
			m.b.SetCursor(point(row, c.Mouse().X-r.X-inset), !pressed || c.Mods().Has(input.ModShift))
		}
		imagePaste := c.Primary() && c.KeyPressed(input.KeyV) && pasteImage != nil && pasteImage()
		editKeys(c, m.b, !imagePaste)
		if (c.KeyPressed(input.KeyEnter) || c.KeyPressed(input.KeyKPEnter)) && c.Mods().Has(input.ModShift) {
			m.b.Insert("\n")
		}
		if m.b.Revision() != rev {
			*value = m.b.String()
			m.layout(c, *value, max(1, r.W-10))
		}
		line := caretLine()
		if f != nil && (c.KeyRepeated(input.KeyUp) || c.KeyRepeated(input.KeyDown)) {
			d := -1
			if c.KeyRepeated(input.KeyDown) {
				d = 1
			}
			x := f.XRunes(m.b.Runes()[m.lines[line].start:], m.b.Cursor()-m.lines[line].start)
			line = max(0, min(len(m.lines)-1, line+d))
			m.b.SetCursor(point(line, x), c.Mods().Has(input.ModShift))
		}
		if m.b.Cursor() != old || m.b.Revision() != rev {
			m.first = max(0, min(line, max(m.first, line-visible+1)))
		}
	}
	if over {
		m.first = max(0, min(max(0, len(m.lines)-visible), m.first-int(c.ScrollY(h*2)/h)))
	}
	bg, border := c.Theme.Widget, c.Theme.Border
	if focused {
		bg, border = c.Theme.WidgetDown, c.Theme.Accent
	} else if c.Hot() == id {
		bg = c.Theme.WidgetHot
	}
	c.Box(r, c.Theme.Radius.Field, bg, border)
	c.PushClip(r)
	defer c.PopClip()
	lo, hi := m.b.Selection()
	rs := m.b.Runes()
	for i := m.first; i < min(len(m.lines), m.first+visible); i++ {
		l := m.lines[i]
		pos := mathx.Vec2{X: r.X + inset, Y: top + float32(i-m.first)*h}
		advance := func(n int) float32 {
			if f == nil {
				return 0
			}
			return f.XRunes(rs[l.start:l.end], n-l.start)
		}
		if focused && max(l.start, lo) < min(l.end, hi) {
			left, right := advance(max(l.start, lo)), advance(min(l.end, hi))
			c.Rect(mathx.Rect{X: pos.X + left, Y: pos.Y, W: right - left, H: h}, c.Theme.Selection)
		}
		c.Text(l.text, pos, c.Theme.Text)
		if focused && i == caretLine() {
			p := mathx.Rect{X: pos.X + advance(m.b.Cursor()), Y: pos.Y, W: 1, H: h}
			c.WantTextInput(p, m.b.Cursor())
			if len(m.b.CompositionRunes()) > 0 {
				c.Text(string(m.b.CompositionRunes()), mathx.Vec2{X: p.X, Y: p.Y}, c.Theme.Accent)
			} else if int(c.Time*2)%2 == 0 {
				c.Rect(p, c.Theme.Text)
			}
		}
	}
}
