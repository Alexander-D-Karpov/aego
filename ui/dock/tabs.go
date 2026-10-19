package dock

import (
	"aego/core/mathx"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	tabIcon float32 = 16
	tabDot  float32 = 6
	tabGap  float32 = 6
)

func (t *Tree) icon(p string) string {
	if t.Icon == nil {
		return ""
	}
	return t.Icon(p)
}

func (t *Tree) dirty(p string) bool {
	return t.Dirty != nil && t.Dirty(p)
}

func (t *Tree) tabWidth(ui *core.Context, p string) float32 {
	th := &ui.Theme
	w := ui.TextSize(t.title(p)).X + 2*th.Padding + 8
	if t.icon(p) != "" {
		w += tabIcon + tabGap
	}
	if t.dirty(p) {
		w += tabDot + tabGap
	}
	return w
}

func (t *Tree) tabLabel(ui *core.Context, p string, tr mathx.Rect, col mathx.Color) {
	th := &ui.Theme
	x := tr.X + th.Padding + 4
	if ic := t.icon(p); ic != "" {
		cy := ui.TextY(tr) + ui.LineHeight()/2
		widgets.IconAt(ui, mathx.Rect{X: x, Y: cy - tabIcon/2, W: tabIcon, H: tabIcon}, ic, col)
		x += tabIcon + tabGap
	}
	title := t.title(p)
	ui.Text(title, mathx.Vec2{X: x, Y: ui.TextY(tr)}, col)
	if t.dirty(p) {
		x += ui.TextSize(title).X + tabGap
		ui.RoundRect(mathx.Rect{X: x, Y: tr.Y + 2 + (tr.H-tabDot)/2, W: tabDot, H: tabDot}, tabDot/2, col)
	}
}
