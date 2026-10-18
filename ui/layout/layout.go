package layout

import (
	"aego/core/mathx"
	"aego/ui/core"
)

func Padding(c *core.Context, p float32, fn func()) {
	cur := c.Cursor()
	saveX, saveW := cur.X, cur.W
	cur.X += p
	cur.W -= p * 2
	cur.Y += p
	fn()
	cur.Y += p
	cur.X, cur.W = saveX, saveW
}

func Indent(c *core.Context, amount float32, fn func()) {
	cur := c.Cursor()
	cur.X += amount
	cur.W -= amount
	fn()
	cur.X -= amount
	cur.W += amount
}

func Spacer(c *core.Context, size float32) {
	c.Cursor().Y += size
}

func Row(c *core.Context, weights []float32, fn func(int)) {
	cur := c.Cursor()
	startY := cur.Y
	baseX, baseW := cur.X, cur.W
	total := float32(0)
	for _, w := range weights {
		total += w
	}
	if total <= 0 {
		return
	}
	gap := c.Theme.Spacing
	available := baseW - gap*float32(len(weights)-1)
	x := baseX
	maxY := startY
	for i, w := range weights {
		width := available * w / total
		cur.X = x
		cur.W = width
		cur.Y = startY
		fn(i)
		if cur.Y > maxY {
			maxY = cur.Y
		}
		x += width + gap
	}
	cur.X, cur.W, cur.Y = baseX, baseW, maxY
}

func ScrollArea(c *core.Context, id core.ID, height float32, offset *float32, fn func()) {
	cur := c.Cursor()
	view := mathx.Rect{X: cur.X, Y: cur.Y, W: cur.W, H: height}
	c.PushClip(view)
	startY := cur.Y
	cur.Y -= *offset
	fn()
	content := cur.Y + *offset - startY
	c.PopClip()
	if c.MouseOver(id, view) {
		if w := c.ScrollY(c.Theme.RowHeight * 3); w != 0 {
			*offset = mathx.Clamp(*offset-w, 0, max(0, content-height))
		}
	}
	cur.Y = startY + height + c.Theme.Spacing
}
