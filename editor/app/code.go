package app

import (
	"strconv"

	"aego/core/mathx"
	"aego/editor/context"
	"aego/editor/play"
	"aego/ui/widgets"
)

func (a *App) attachCode() {
	a.consolePanel.OnOpen = func(rel string, line int) {
		a.ctx.Open(context.At(rel, line, 1))
	}
	if a.opt.Code == nil {
		return
	}
	a.opt.Code.Attach(a.ctx, a.ws)
	a.ctx.OpenScript = a.opt.Code.Open
}

func (a *App) drawScriptsBanner(body mathx.Rect) {
	if a.play.State() == play.Stopped || a.ctx.Scripts == nil {
		return
	}
	gen, failed := a.ctx.Scripts.Status()
	if !failed {
		return
	}
	ui := a.ui
	th := &ui.Theme
	msg := "Running script build " + strconv.FormatUint(gen, 10) + ", the latest source failed to compile"
	h := th.RowHeight + 8
	w := min(body.W-16, ui.TextSize(msg).X+2*th.Padding+100)
	r := mathx.Rect{X: body.X + (body.W-w)/2, Y: body.Y + 6, W: w, H: h}
	ui.Rect(r, th.PanelAlt)
	ui.Outline(r, th.Warn)
	ui.PushClip(r)
	ui.Text(msg, mathx.Vec2{X: r.X + th.Padding, Y: ui.TextY(r)}, th.Warn)
	ui.PopClip()
	br := mathx.Rect{X: r.X + r.W - 90, Y: r.Y + 4, W: 84, H: h - 8}
	if widgets.ButtonRect(ui, ui.ID("scripts.problems"), br, "Problems") && a.ctx.ShowProblems != nil {
		a.ctx.ShowProblems("")
	}
}
