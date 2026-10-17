package shell

import (
	"strconv"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/play"
	"aego/editor/core/templates"
	"aego/editor/core/workspace"
	"aego/ui/widgets"
)

func (a *App) menuCategories() []string {
	reg := a.ctx.Commands
	base := []string{"File", "Edit", "Create", "Scene", "Play", "Assets", "Window"}
	known := map[string]bool{}
	var out []string
	for _, c := range base {
		known[c] = true
		if c == "Create" || len(reg.Category(c)) > 0 {
			out = append(out, c)
		}
	}
	for _, c := range reg.Categories() {
		if !known[c] {
			out = append(out, c)
		}
	}
	return out
}

func (a *App) commandItems(cat string) {
	ui := a.ui
	reg := a.ctx.Commands
	last := -1
	for _, d := range reg.Category(cat) {
		if last >= 0 && d.Order/100 != last/100 {
			widgets.Separator(ui)
		}
		last = d.Order
		label := d.Name
		if d.Label != nil {
			label = d.Label()
		}
		checked := d.Checked != nil && d.Checked()
		if widgets.MenuItem(ui, label, d.Shortcut.String(), reg.Enabled(d.ID), checked) {
			reg.Run(d.ID)
		}
	}
}

func (a *App) sceneDocs() {
	ui := a.ui
	docs := a.ctx.Docs.Docs()
	if len(docs) == 0 {
		return
	}
	widgets.Separator(ui)
	active := a.ctx.Docs.Edit()
	for _, d := range docs {
		name := d.Path
		if d.Dirty() {
			name += " *"
		}
		if widgets.MenuItem(ui, name, "", true, active != nil && d.ID == active.ID) {
			a.ctx.Docs.SetActive(d.ID)
		}
	}
}

func (a *App) drawMenuBar(r mathx.Rect) {
	ui := a.ui
	ctx := a.ctx
	mb := widgets.BeginMenuBar(ui, r)
	for _, cat := range a.menuCategories() {
		if !mb.Menu(cat, 280) {
			continue
		}
		switch cat {
		case "Create":
			parent := ids.NodeID{}
			if it, ok := ctx.Selection.Active(); ok && ctx.Doc() != nil && it.Doc == ctx.Doc().ID {
				parent = it.Node
			}
			templates.Menu(ctx, a.templates, parent, templates.Place{}, nil)
		case "Scene":
			a.commandItems(cat)
			a.sceneDocs()
		default:
			a.commandItems(cat)
		}
		mb.EndMenu()
	}
	if !a.opt.Supervised || a.buildState == widgets.DotOK {
		return
	}
	label, tip := "native building", "Native code is being rebuilt"
	if a.buildState == widgets.DotError {
		label = "Native API is stale"
		tip = "The last native build failed; the editor and scripts use the previous native API. Click to see the build output"
	}
	w := ui.TextSize(label).X + 24
	dr := mathx.Rect{X: r.X + r.W - 8 - w, Y: r.Y + 2, W: w, H: r.H - 4}
	if dr.X <= mb.X()+16 {
		return
	}
	did := ui.ID("native")
	if widgets.StatusDot(ui, did, dr, a.buildState, label) && a.buildState == widgets.DotError {
		a.ws.Show(workspace.Console)
		a.consolePanel.Focus("build")
	}
	widgets.Tooltip(ui, did, tip)
}

func (a *App) drawToolbar(r mathx.Rect) {
	ui := a.ui
	th := &ui.Theme
	reg := a.ctx.Commands
	st := a.play.State()
	tb := widgets.BeginToolbar(ui, r)
	if st != play.Stopped {
		tint := th.PlayTint
		tint.A = 0.18
		ui.Rect(r, tint)
	}
	saveX := r.X + r.W - tb.ButtonW() - 6
	scale, tick := scaleLabel(a.play.TimeScale()), "+1 tick"
	n := 3
	if st != play.Stopped {
		n = 4
	}
	width := float32(n)*tb.ButtonW() + ui.TextSize(scale).X + ui.TextSize(tick).X + 32
	x := max(r.X+(r.W-width)/2, r.X+6)
	sid := ui.ID("tb.scale")
	if x+width <= saveX-8 {
		tb.SetX(x)
		a.playRect = mathx.Rect{X: x, Y: r.Y + 3, W: tb.ButtonW() - 2, H: r.H - 6}
		blocked := 0
		if st == play.Stopped {
			blocked = a.scriptErrors()
		}
		playTip := "Play (" + reg.Shortcut(command.PlayStart) + ")"
		noSource := st == play.Stopped && a.ctx.Docs.Edit() != nil && !a.canPlay()
		switch {
		case blocked > 0:
			playTip = strconv.Itoa(blocked) + " script package error(s), see Problems"
		case noSource:
			playTip = playPrefabHint
		}
		if tb.Button(ui.ID("tb.play"), "play.start", playTip, st == play.Playing, blocked == 0 && !noSource) {
			switch st {
			case play.Stopped:
				reg.Run(command.PlayStart)
			case play.Paused:
				a.play.Pause()
			}
		}
		if tb.Button(ui.ID("tb.pause"), "play.pause", "Pause ("+reg.Shortcut(command.PlayPause)+")", st == play.Paused, st != play.Stopped) {
			a.play.Pause()
		}
		if tb.Button(ui.ID("tb.step"), "play.step", "Step one frame ("+reg.Shortcut(command.PlayStep)+")", false, st == play.Paused) {
			a.play.Step()
		}
		if clicked, _ := tb.TextButton(ui.ID("tb.tick"), tick, "Step one fixed tick ("+reg.Shortcut(command.PlayStepFixed)+"), while paused"); clicked && reg.Enabled(command.PlayStepFixed) {
			reg.Run(command.PlayStepFixed)
		}
		if clicked, sr := tb.TextButton(sid, scale, "Time scale"); clicked {
			if ui.PopupOpen(sid) {
				ui.ClosePopup(sid)
			} else {
				ui.CloseAllPopups()
				ui.OpenPopup(sid, mathx.Vec2{X: sr.X, Y: sr.Y + sr.H})
			}
		}
		if st != play.Stopped && tb.Button(ui.ID("tb.stop"), "play.stop", "Stop", false, true) {
			a.stopPlay()
		}
	}
	tb.SetX(saveX)
	if tb.Button(ui.ID("tb.save"), "file.save", "Save ("+reg.Shortcut(command.FileSave)+")", false, reg.Enabled(command.FileSave)) {
		reg.Run(command.FileSave)
	}
	a.scalePopup(sid)
}
