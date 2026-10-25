package shell

import (
	"aego/editor/panels/taskcenter"
	"aego/ui/widgets"
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/context"
)

func clockText(sec float64) string {
	s := int(sec)
	m := s / 60
	s %= 60
	pad := ""
	if s < 10 {
		pad = "0"
	}
	return strconv.Itoa(m) + ":" + pad + strconv.Itoa(s)
}

func clockTextFine(sec float64) string {
	cs := int(sec*100) % 100
	pad := ""
	if cs < 10 {
		pad = "0"
	}
	return clockText(sec) + "." + pad + strconv.Itoa(cs)
}

func (a *App) scriptStatus() (string, bool) {
	if a.ctx.Scripts == nil {
		return "", false
	}
	gen, failed := a.ctx.Scripts.Status()
	_, n, _ := a.ctx.FirstError("")
	s := "Scripts: build " + strconv.FormatUint(gen, 10)
	switch {
	case n > 0:
		return s + " · " + strconv.Itoa(n) + " errors", true
	case failed:
		return s + " · latest failed", true
	}
	return s + " · ok", false
}

// buildDot shows the script build as a green dot, or the error icon with the number of errors; the full status is in the tooltip.
func (a *App) buildDot(r mathx.Rect, left *float32, tip string, bad bool) bool {
	ui := a.ui
	th := &ui.Theme
	n := ""
	if bad {
		if _, errs, _ := a.ctx.FirstError(""); errs > 0 {
			n = strconv.Itoa(errs)
		}
	}
	w := float32(24)
	if bad {
		w = 26 + ui.TextSize(n).X
	}
	sr := mathx.Rect{X: *left, Y: r.Y, W: w, H: r.H}
	*left += w + 4
	id := ui.ID("status/scripts")
	clicked := ui.Clicked(id, sr)
	if ui.Hot() == id {
		ui.Rect(sr, th.ButtonHover)
	}
	if bad {
		widgets.IconAt(ui, mathx.Rect{X: sr.X + 4, Y: float32(int(sr.Y + (sr.H-16)/2)), W: 16, H: 16}, "log.error", th.Error)
		ui.Text(n, mathx.Vec2{X: sr.X + 22, Y: ui.TextY(r)}, th.Error)
	} else {
		d := float32(8)
		ui.RoundRect(mathx.Rect{X: sr.X + (sr.W-d)/2, Y: sr.Y + (sr.H-d)/2, W: d, H: d}, d/2, th.Ok)
	}
	widgets.Tooltip(ui, id, tip)
	return clicked
}

func (a *App) drawStatus(r mathx.Rect) {
	ui := a.ui
	th := &ui.Theme
	ui.Rect(r, th.Header)
	ty := ui.TextY(r)
	left, right := r.X+th.Padding, r.X+r.W-th.Padding
	item := func(text string, col mathx.Color, key string, atLeft bool, icon ...string) bool {
		w := ui.TextSize(text).X + 12
		var iw float32
		if len(icon) > 0 {
			iw = 20
			w += iw
		}
		sr := mathx.Rect{X: right - w, Y: r.Y, W: w, H: r.H}
		if atLeft {
			sr.X = left
			left += w + 4
		} else {
			right = sr.X - 4
		}
		clicked := false
		if key != "" {
			id := ui.ID("status/" + key)
			clicked = ui.Clicked(id, sr)
			if ui.Hot() == id {
				ui.Rect(sr, th.ButtonHover)
			}
		}
		if iw > 0 {
			widgets.IconAt(ui, mathx.Rect{X: sr.X + 6, Y: float32(int(sr.Y + (sr.H-16)/2)), W: 16, H: 16}, icon[0], col)
		}
		ui.Text(text, mathx.Vec2{X: sr.X + 6 + iw, Y: ty}, col)
		return clicked
	}
	if s, bad := a.scriptStatus(); s != "" {
		if a.buildDot(r, &left, s, bad) && a.ctx.ShowProblems != nil {
			a.ctx.ShowProblems("")
		}
	}
	if text, col := a.playStatus(); text != "" {
		item(text, col, "", true)
	}
	if c := a.opt.Code; c != nil {
		if strings.HasPrefix(a.ctx.FocusedPanel, "script:") {
			if ln, col, ok := c.Caret(a.ctx.FocusedPanel); ok {
				item("UTF-8 · LF", th.TextDim, "", false)
				item("Ln "+strconv.Itoa(ln)+", Col "+strconv.Itoa(col), th.Text, "", false)
			}
		}
		label := "autosave off"
		if c.Autosave {
			label = "autosave on"
		}
		if item(label, th.TextDim, "autosave", false) {
			c.Autosave = !c.Autosave
		}
	}
	if n, failed := a.taskCounts(); n > 0 {
		if item(strconv.Itoa(n)+" task(s)", th.TextDim, "tasks", false, "status.busy") {
			a.ws.Show(taskcenter.ID)
		}
	} else if failed > 0 {
		if item(strconv.Itoa(failed)+" task(s) failed", th.Error, "tasks", false) {
			a.ws.Show(taskcenter.ID)
		}
	}
	if n := a.ctx.Docs.DirtyCount(); n > 0 {
		item(strconv.Itoa(n)+" unsaved", th.Warn, "", false)
	}
	mid := mathx.Rect{X: left + 8, Y: r.Y, W: max(0, right-left-16), H: r.H}
	text, col := a.contextHint(), th.TextDisabled
	if n, ok := a.ctx.LastNotice(); ok && ui.Time-n.Time < 8 {
		text, col = n.Text, th.TextDim
		switch n.Level {
		case context.LevelWarn:
			col = th.Warn
		case context.LevelError:
			col = th.Error
		}
	}
	ui.PushClip(mid)
	ui.Text(text, mathx.Vec2{X: mid.X, Y: ty}, col)
	ui.PopClip()
}
