package shell

import (
	"strconv"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/play"
	"aego/editor/core/templates"
	"aego/editor/core/workspace"
	"aego/engine/i18n"
	"aego/ui/widgets"
)

func (a *App) menuCategories() []string {
	reg := a.ctx.Commands
	base := []string{"File", "Edit", "Create", "Scene", "Play", "Assets", "Window"}
	known := map[string]bool{themeCategory: true, languageCategory: true}
	var out []string
	for _, c := range base {
		known[c] = true
		if c == "Create" || reg.HasCategory(c) {
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
		it := widgets.Item{
			Label:    d.Title(),
			Shortcut: d.Shortcut.String(),
			Icon:     d.Icon,
			Tip:      d.TipText(),
			Enabled:  reg.Enabled(d.ID),
			Checked:  d.Checked != nil && d.Checked(),
		}
		if widgets.Entry(ui, it) {
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
		if cat == "Create" {
			if hit, at := mb.Button(menuTitle(cat)); hit && ctx.Doc() != nil {
				parent := ids.NodeID{}
				if it, ok := ctx.Selection.Active(); ok && it.Doc == ctx.Doc().ID {
					parent = it.Node
				}
				a.addPal.CreateNode(parent, templates.Place{}, &at, nil)
			}
			continue
		}
		if !mb.Menu(menuTitle(cat), 280) {
			continue
		}
		switch cat {
		case "Scene":
			a.commandItems(cat)
			a.sceneDocs()
		case "Window":
			a.commandItems(cat)
			a.subMenu(themeCategory, true)
			a.subMenu(languageCategory, false)
		default:
			a.commandItems(cat)
		}
		mb.EndMenu()
	}
	if !a.opt.Supervised || a.buildState == widgets.DotOK {
		return
	}
	label, tip := "native building", i18n.Tr("editor.shell.native_code_is_being_rebuilt", "Native code is being rebuilt")
	if a.buildState == widgets.DotError {
		label = i18n.Tr("editor.shell.native_api_is_stale", "Native API is stale")
		tip = i18n.Tr("editor.shell.the_last_native_build_failed", "The last native build failed; the editor and scripts use the previous native API. Click to see the build output")
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
	saveX := r.X + r.W - tb.ButtonW() - 6
	wsLabel := a.ws.Current().Title + "   "
	wsX := saveX - ui.TextSize(wsLabel).X - 22
	bw := tb.ButtonW()
	scale, tick := scaleLabel(a.play.TimeScale())+"   ", i18n.Tr("editor.shell.plus_one_tick", "+1 tick")
	width := 4*bw + ui.TextSize(scale).X + ui.TextSize(tick).X + 28 + 2*9 + 8
	x := max(r.X+(r.W-width)/2, r.X+6)
	sid := ui.ID("tb.scale")
	if x+width <= wsX-8 {
		if st != play.Stopped {
			glow := th.PlayTint
			glow.A = 0.22
			ui.RoundRect(mathx.Rect{X: x, Y: r.Y + 2, W: width, H: r.H - 4}, th.Radius.Popup, glow)
			tb.Label = th.Text
		}
		tb.Active = th.Accent
		tb.SetX(x + 4)
		a.playRect = mathx.Rect{X: x + 4, Y: r.Y + 3, W: bw - 2, H: r.H - 6}
		blocked := 0
		if st == play.Stopped {
			blocked = a.scriptErrors()
		}
		playTip := i18n.Tr("editor.shell.play_playstart_full_screen_commandfullscreenplay", "Play ({playstart}), full screen ({commandfullscreenplay})", i18n.Str("playstart", reg.Shortcut(command.PlayStart)), i18n.Str("commandfullscreenplay", reg.Shortcut(CommandFullscreenPlay)))
		noSource := st == play.Stopped && a.ctx.Docs.Edit() != nil && !a.canPlay()
		switch {
		case blocked > 0:
			playTip = i18n.Tr("editor.shell.blocked_script_package_error_s", "{blocked} script package error(s), see Problems", i18n.Str("blocked", strconv.Itoa(blocked)))
		case noSource:
			playTip = playPrefabHint
		}
		playTint := th.Accent
		if st == play.Playing {
			playTint = th.Text
		}
		if tb.TintButton(ui.ID("tb.play"), "play.start", playTip, st == play.Playing, blocked == 0 && !noSource, playTint) {
			switch st {
			case play.Stopped:
				reg.Run(command.PlayStart)
			case play.Paused:
				a.play.Pause()
			}
		}
		if tb.Button(ui.ID("tb.pause"), "play.pause", i18n.Tr("editor.shell.pause_playpause", "Pause ({playpause})", i18n.Str("playpause", reg.Shortcut(command.PlayPause))), st == play.Paused, st != play.Stopped) {
			a.play.Pause()
		}
		if tb.Button(ui.ID("tb.step"), "play.step", i18n.Tr("editor.shell.step_one_frame_playstep", "Step one frame ({playstep})", i18n.Str("playstep", reg.Shortcut(command.PlayStep))), false, st == play.Paused) {
			a.play.Step()
		}
		if clicked, _ := tb.TextButton(ui.ID("tb.tick"), tick, i18n.Tr("editor.shell.step_one_fixed_tick_playstepfixed", "Step one fixed tick ({playstepfixed}), while paused", i18n.Str("playstepfixed", reg.Shortcut(command.PlayStepFixed)))); clicked && reg.Enabled(command.PlayStepFixed) {
			reg.Run(command.PlayStepFixed)
		}
		tb.Separator()
		clicked, sr := tb.TextButton(sid, scale, i18n.Tr("editor.shell.time_scale", "Time scale"))
		caret := th.TextDim
		if tb.Label.A > 0 {
			caret = tb.Label
		}
		widgets.Chevron(ui, mathx.Rect{X: sr.X + sr.W - 18, Y: float32(int(sr.Y + (sr.H-19)/2 + 1)), W: 19, H: 19}, true, caret)
		if clicked {
			if ui.PopupOpen(sid) {
				ui.ClosePopup(sid)
			} else {
				ui.CloseAllPopups()
				ui.OpenPopup(sid, mathx.Vec2{X: sr.X, Y: sr.Y + sr.H})
			}
		}
		tb.Separator()
		tb.Active = th.Error
		if tb.TintButton(ui.ID("tb.stop"), "play.stop", i18n.Tr("editor.shell.stop", "Stop"), st != play.Stopped, st != play.Stopped, th.Text) {
			a.stopPlay()
		}
		tb.Active, tb.Label = mathx.Color{}, mathx.Color{}
	}
	wid := ui.ID("tb.workspace")
	if wsX > r.X+6 {
		tb.SetX(wsX)
		clicked, wr := tb.TextButton(wid, wsLabel, i18n.Tr("editor.shell.workspace_v_4", "Workspace ({v}…4)", i18n.Str("v", reg.Shortcut(WorkspaceCommand(0)))))
		widgets.Chevron(ui, mathx.Rect{X: wr.X + wr.W - 18, Y: float32(int(wr.Y + (wr.H-19)/2 + 1)), W: 19, H: 19}, true, th.TextDim)
		if clicked {
			if ui.PopupOpen(wid) {
				ui.ClosePopup(wid)
			} else {
				ui.CloseAllPopups()
				ui.OpenPopup(wid, mathx.Vec2{X: wr.X, Y: wr.Y + wr.H})
			}
		}
	}
	tb.SetX(saveX)
	if tb.Button(ui.ID("tb.save"), "file.save", i18n.Tr("editor.shell.save_filesave", "Save ({filesave})", i18n.Str("filesave", reg.Shortcut(command.FileSave))), false, reg.Enabled(command.FileSave)) {
		reg.Run(command.FileSave)
	}
	a.scalePopup(sid)
	a.workspacePopup(wid)
}

func (a *App) subMenu(cat string, sep bool) {
	ui := a.ui
	reg := a.ctx.Commands
	if !reg.HasCategory(cat) {
		return
	}
	if sep {
		widgets.Separator(ui)
	}
	if !widgets.SubMenu(ui, menuTitle(cat), 160) {
		return
	}
	for _, d := range reg.Category(cat) {
		if widgets.Entry(ui, widgets.Item{Label: subLabel(cat, d.Title()), Tip: d.TipText(), Enabled: true, Checked: d.Checked()}) {
			reg.Run(d.ID)
		}
	}
	widgets.EndSubMenu(ui)
}
