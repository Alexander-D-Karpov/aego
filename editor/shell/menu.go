package shell

import (
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

func (a *App) drawStatus(r mathx.Rect) {
	ui := a.ui
	th := &ui.Theme
	ui.Rect(r, th.Header)
	ty := ui.TextY(r)
	left, right := r.X+th.Padding, r.X+r.W-th.Padding
	item := func(text string, col mathx.Color, key string, atLeft bool) bool {
		w := ui.TextSize(text).X + 12
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
		ui.Text(text, mathx.Vec2{X: sr.X + 6, Y: ty}, col)
		return clicked
	}
	if s, bad := a.scriptStatus(); s != "" {
		col := th.TextDim
		if bad {
			col = th.Error
		}
		if item(s, col, "scripts", true) && a.ctx.ShowProblems != nil {
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
