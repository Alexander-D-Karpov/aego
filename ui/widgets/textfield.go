package widgets

import (
	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/textedit"
)

func TextField(c *core.Context, id core.ID, value *string) (changed, committed bool) {
	return TextFieldRect(c, id, c.Cursor().Next(0), value, false)
}

func SearchText(c *core.Context, id core.ID, value *string) bool {
	changed, _ := TextFieldRect(c, id, c.Cursor().Next(0), value, true)
	return changed
}

func TextFieldRect(c *core.Context, id core.ID, r mathx.Rect, value *string, live bool) (changed, committed bool) {
	if s, ok := c.LostText(id); ok && s != *value {
		*value = s
		committed = true
	}
	th := &c.Theme
	focused := c.Focus() == id
	inner := mathx.Rect{X: r.X + 4, Y: r.Y, W: r.W - 8, H: r.H}

	if c.MouseOver(id, r) {
		c.SetCursorShape(core.CursorText)
	}
	if c.Pressed(id, r) {
		if !focused {
			c.SetFocus(id)
			focused = true
			c.EditBuffer(id, *value).SelectAll()
		} else {
			b := c.EditBuffer(id, *value)
			b.SetCursor(indexAt(c, b, inner), c.Mods().Has(input.ModShift))
			if c.DoubleClicked(id) {
				b.SelectAll()
			}
		}
	}

	if focused {
		b := c.EditBuffer(id, *value)
		if c.Active() == id && c.MouseDown(input.MouseLeft) && c.MouseDelta() != (mathx.Vec2{}) {
			b.SetCursor(indexAt(c, b, inner), true)
		}
		changed = editKeys(c, b, true)
		switch {
		case c.KeyPressed(input.KeyEnter) || c.KeyPressed(input.KeyKPEnter):
			if s := b.String(); s != *value {
				*value = s
				committed = true
			}
			c.CancelFocus()
			focused = false
		case c.KeyPressed(input.KeyEscape):
			c.CancelFocus()
			focused = false
		}
		if live && changed {
			*value = b.String()
		}
	}

	bg := th.Widget
	switch {
	case focused:
		bg = th.WidgetDown
	case c.Hot() == id:
		bg = th.WidgetHot
	}
	border := th.Border
	if focused {
		border = th.Accent
	}
	c.Box(r, th.Radius.Field, bg, border)

	c.PushClip(inner)
	font := c.Font()
	ty := c.TextY(r)
	if focused && font != nil {
		drawEditing(c, c.EditBuffer(id, *value), inner, ty)
	} else {
		c.Text(*value, mathx.Vec2{X: inner.X, Y: ty}, th.Text)
	}
	c.PopClip()
	return changed, committed
}

func indexAt(c *core.Context, b *textedit.Buffer, inner mathx.Rect) int {
	font := c.Font()
	if font == nil {
		return b.Len()
	}
	return font.IndexAtRunes(b.Runes(), c.Mouse().X-(inner.X-*c.EditScroll()))
}

func editKeys(c *core.Context, b *textedit.Buffer, pasteText bool) bool {
	before := b.Revision()
	mods := c.Mods()
	extend := mods.Has(input.ModShift)
	word := mods&c.WordMod() != 0
	if t := c.TextInput(); len(t) > 0 && !c.Primary() {
		b.Insert(string(t))
	}
	if c.KeyRepeated(input.KeyLeft) {
		b.Move(textedit.Left, word, extend)
	}
	if c.KeyRepeated(input.KeyRight) {
		b.Move(textedit.Right, word, extend)
	}
	if c.KeyPressed(input.KeyHome) {
		b.Move(textedit.Home, false, extend)
	}
	if c.KeyPressed(input.KeyEnd) {
		b.Move(textedit.End, false, extend)
	}
	if c.KeyRepeated(input.KeyBackspace) {
		b.Delete(textedit.Left, word)
	}
	if c.KeyRepeated(input.KeyDelete) {
		b.Delete(textedit.Right, word)
	}
	if c.Primary() {
		switch {
		case c.KeyPressed(input.KeyA):
			b.SelectAll()
		case c.KeyPressed(input.KeyC):
			if c.Clipboard != nil && b.HasSelection() {
				c.Clipboard.SetText(b.Copy())
			}
		case c.KeyPressed(input.KeyX):
			if c.Clipboard != nil && b.HasSelection() {
				c.Clipboard.SetText(b.Cut())
			}
		case pasteText && c.KeyPressed(input.KeyV):
			if c.Clipboard != nil {
				b.Insert(c.Clipboard.Text())
			}
		case c.KeyRepeated(input.KeyZ):
			b.Undo()
		}
	}
	comp, cc := c.Composition()
	b.Composition(comp, cc)
	return b.Revision() != before
}

func drawEditing(c *core.Context, b *textedit.Buffer, inner mathx.Rect, ty float32) {
	th := &c.Theme
	font := c.Font()
	runes := b.Runes()
	comp := b.CompositionRunes()
	cursor := b.Cursor()
	caret := font.XRunes(runes, cursor) + font.XRunes(comp, b.CompositionCursor())
	scroll := c.EditScroll()
	if caret-*scroll > inner.W-2 {
		*scroll = caret - inner.W + 2
	}
	if caret-*scroll < 0 {
		*scroll = caret
	}
	x0 := inner.X - *scroll
	if b.HasSelection() {
		lo, hi := b.Selection()
		sx, ex := font.XRunes(runes, lo), font.XRunes(runes, hi)
		c.Rect(mathx.Rect{X: x0 + sx, Y: inner.Y + 3, W: ex - sx, H: inner.H - 6}, th.Selection)
	}
	head := string(runes[:cursor])
	c.Text(head, mathx.Vec2{X: x0, Y: ty}, th.Text)
	hx := x0 + font.XRunes(runes, cursor)
	if len(comp) > 0 {
		cw := font.WidthRunes(comp)
		c.Text(string(comp), mathx.Vec2{X: hx, Y: ty}, th.Accent)
		c.Rect(mathx.Rect{X: hx, Y: ty + font.LineHeight - 1, W: cw, H: 1}, th.Accent)
		hx += cw
	}
	c.Text(string(runes[cursor:]), mathx.Vec2{X: hx, Y: ty}, th.Text)
	if int(c.Time*2)%2 == 0 {
		c.Rect(mathx.Rect{X: x0 + caret, Y: inner.Y + 4, W: 1, H: inner.H - 8}, th.Text)
	}
	c.WantTextInput(mathx.Rect{X: x0 + caret, Y: inner.Y, W: 1, H: inner.H}, cursor)
}
