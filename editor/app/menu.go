package app

import (
	"strconv"

	"aego/core/mathx"
	"aego/editor/context"
	"aego/editor/play"
	"aego/ui/core"
)

func (a *App) drawStatus(r mathx.Rect) {
	ui := a.ui
	th := &ui.Theme
	ui.Rect(r, th.Header)
	if n, ok := a.ctx.LastNotice(); ok && ui.Time-n.Time < 8 {
		col := th.TextDim
		switch n.Level {
		case context.LevelWarn:
			col = th.Warn
		case context.LevelError:
			col = th.Error
		}
		ui.PushClip(mathx.Rect{X: r.X, Y: r.Y, W: r.W * 0.6, H: r.H})
		ui.TextIn(r, n.Text, col, core.AlignLeft)
		ui.PopClip()
	}
	right := ""
	switch a.play.State() {
	case play.Playing:
		right = "Playing  "
	case play.Paused:
		right = "Paused  "
	}
	if n := a.ctx.Docs.DirtyCount(); n > 0 {
		right += strconv.Itoa(n) + " unsaved"
	}
	if right != "" {
		ui.TextIn(r, right, th.Warn, core.AlignRight)
	}
}
