package export

import (
	"path/filepath"
	"strconv"
	"sync/atomic"

	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/editor/core/tasks"
	"aego/engine/i18n"
	"aego/engine/problems"
	uicore "aego/ui/core"
	"aego/ui/widgets"
)

const (
	ID          = "export"
	Release     = "release"
	Development = "development"
)

var profiles = []string{Development, Release}

type Deps struct {
	Exporter   services.Exporter
	Tasks      *tasks.Queue
	PickFolder func(start string, done func(string))
}

type runner interface {
	Run(out, target string) error
}

type line struct {
	icon  string
	col   mathx.Color
	text  string
	right string
}

type Panel struct {
	d       Deps
	target  string
	profile string
	out     string
	running *tasks.Task
	stage   atomic.Value
	rep     *services.ExportReport
	issues  []problems.Diagnostic
	msg     string
	scroll  float32
	shown   string
	shownOf string
	shownW  float32
}

func New(d Deps) *Panel {
	return &Panel{d: d, target: "host", profile: Development}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	return i18n.Tr("editor.export.title", "Export")
}

func (p *Panel) Icon() string {
	return "file.package"
}

func errorCount(ds []problems.Diagnostic) (int, int) {
	e, w := 0, 0
	for _, d := range ds {
		switch d.Severity {
		case problems.Error:
			e++
		case problems.Warning:
			w++
		}
	}
	return e, w
}

func (p *Panel) check() {
	x := p.d.Exporter
	prep, prof := x.Prepare(), p.profile
	var found []problems.Diagnostic
	p.stage.Store("checking")
	p.running = p.d.Tasks.Work(tasks.Spec{Title: i18n.Tr("editor.export.check_export_prof", "Check export ({prof})", i18n.Str("prof", prof)), Kind: "export"}, func(*tasks.Task) error {
		found = x.Check(prep, prof)
		return nil
	}).Then(func(t *tasks.Task) {
		p.running = nil
		if t.State() != tasks.Done {
			p.msg = "Check canceled"
			return
		}
		p.rep, p.issues = nil, found
		e, w := errorCount(found)
		switch {
		case e > 0:
			p.msg = strconv.Itoa(e) + " problem(s) block the export"
		case w > 0:
			p.msg = i18n.Tr("editor.export.ready_warnings", "Ready to export, {warnings}", i18n.Str("warnings", i18n.TrN("editor.export.warnings", int64(w), "{n} warning", "{n} warnings")))
		default:
			p.msg = "Ready to export"
		}
	})
}

func (p *Panel) export(out string) {
	x := p.d.Exporter
	prep := x.Prepare()
	req := services.ExportRequest{Target: p.target, Profile: p.profile, Out: out}
	var rep services.ExportReport
	var err error
	p.stage.Store("starting")
	p.running = p.d.Tasks.Work(tasks.Spec{Title: i18n.Tr("editor.export.export_target_profile", "Export {target} {profile}", i18n.Str("target", req.Target), i18n.Str("profile", req.Profile)), Kind: "export", Detail: out}, func(t *tasks.Task) error {
		rep, err = x.Export(prep, req, func(f float32, s string) {
			t.SetProgress(f)
			p.stage.Store(s)
		})
		return err
	}).Then(func(t *tasks.Task) {
		p.running = nil
		if t.State() == tasks.Canceled {
			p.msg = "Export canceled"
			return
		}
		p.rep, p.issues = &rep, rep.Issues
		switch {
		case rep.Blocked:
			e, _ := errorCount(rep.Issues)
			p.msg = "Export is blocked by " + strconv.Itoa(e) + " problem(s)"
		case err != nil:
			p.msg = "Export failed: " + err.Error()
		default:
			p.msg = "Exported to " + filepath.Base(rep.Out)
		}
	})
}

func (p *Panel) lines(ui *uicore.Context) []line {
	th := &ui.Theme
	var out []line
	for _, d := range p.issues {
		l := line{icon: "log.info", col: th.TextDim, text: d.Message, right: d.Code}
		switch d.Severity {
		case problems.Error:
			l.icon, l.col = "log.error", th.Error
		case problems.Warning:
			l.icon, l.col = "status.warning", th.Warn
		}
		if d.Subject.File != "" {
			l.text = d.Subject.File + ": " + d.Message
		}
		out = append(out, l)
	}
	if p.rep != nil && !p.rep.Blocked {
		for _, kv := range p.rep.Lines {
			out = append(out, line{text: kv[0], right: kv[1], col: th.Text})
		}
	}
	return out
}

func (p *Panel) output() string {
	def := p.d.Exporter.Out(p.target, p.profile)
	if p.out == "" {
		return def
	}
	return filepath.Join(p.out, filepath.Base(def))
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	pad := th.Padding
	wide := r.W >= 760
	left := mathx.Rect{X: r.X + pad, Y: r.Y + pad, W: r.W - 2*pad, H: r.H - 2*pad}
	right := left
	if wide {
		left.W = min(560, r.W*0.55)
		right = mathx.Rect{X: left.X + left.W + 2*pad, Y: left.Y, W: r.X + r.W - pad - (left.X + left.W + 2*pad), H: left.H}
		ui.Rect(mathx.Rect{X: left.X + left.W + pad, Y: r.Y + pad, W: 1, H: r.H - 2*pad}, th.Separator)
	}
	ui.PushCursor(uicore.Cursor{X: left.X, Y: left.Y, W: left.W, RowH: th.RowHeight, Pad: pad, Space: 6})
	p.settings(ui)
	end := ui.Cursor().Y
	ui.PopCursor()
	if !wide {
		right = mathx.Rect{X: left.X, Y: end + pad, W: left.W, H: max(0, r.Y+r.H-end-2*pad)}
	}
	p.results(ui, right)
}

func targetNames() map[string]string {
	return map[string]string{"host": i18n.Tr("editor.export.target.host", "This computer"), "linux": "Linux", "windows": "Windows", "darwin": "macOS", "web": i18n.Tr("editor.export.target.web", "Web")}
}

func profileNote(p string) string {
	switch p {
	case Development:
		return i18n.Tr("editor.export.profile.development.note", "Logs and runtime checks on, faster to build.")
	case Release:
		return i18n.Tr("editor.export.profile.release.note", "Optimized and stripped, for players.")
	}
	return ""
}

const labelW = 96

func profileNames() map[string]string {
	return map[string]string{Development: i18n.Tr("editor.export.profile.development", "Development"), Release: i18n.Tr("editor.export.profile.release", "Release")}
}

// field reserves a row with a label on the left and returns the rest of it.
func field(ui *uicore.Context, text string) mathx.Rect {
	th := &ui.Theme
	row := ui.Cursor().Next(th.RowHeight + 4)
	ui.Text(text, mathx.Vec2{X: row.X, Y: ui.TextY(row)}, th.TextDim)
	return mathx.Rect{X: row.X + labelW, Y: row.Y, W: max(0, row.W-labelW), H: row.H}
}

// segments draws one choice as a joined row of buttons; the chosen one is filled.
func (p *Panel) segments(ui *uicore.Context, row mathx.Rect, key string, opts []string, names map[string]string, cur *string) float32 {
	th := &ui.Theme
	var total float32
	for _, o := range opts {
		total += ui.TextSize(label(names, o)).X + 24
	}
	scale := min(1, row.W/max(total, 1))
	x := row.X
	ui.Box(mathx.Rect{X: row.X, Y: row.Y, W: min(total, row.W), H: row.H}, th.Radius.Button, th.Widget, th.Border)
	for i, o := range opts {
		name := label(names, o)
		w := (ui.TextSize(name).X + 24) * scale
		rr := mathx.Rect{X: x, Y: row.Y, W: w, H: row.H}
		id := uicore.IDSeq(ui.ID(key), i)
		hit := ui.Clicked(id, rr)
		switch {
		case *cur == o:
			ui.RoundRect(mathx.Rect{X: rr.X + 2, Y: rr.Y + 2, W: rr.W - 4, H: rr.H - 4}, th.Radius.Button, th.Accent)
		case ui.Hot() == id && p.running == nil:
			ui.RoundRect(mathx.Rect{X: rr.X + 2, Y: rr.Y + 2, W: rr.W - 4, H: rr.H - 4}, th.Radius.Button, th.WidgetHot)
		}
		col := th.Text
		if *cur == o {
			col = th.Background
		}
		ui.PushClip(rr)
		ui.TextIn(rr, name, col, uicore.AlignCenter)
		ui.PopClip()
		if hit && p.running == nil {
			*cur = o
		}
		x += w
	}
	return x - row.X
}

func label(names map[string]string, o string) string {
	if n, ok := names[o]; ok {
		return n
	}
	return o
}

// action is a button with an icon; primary ones are filled with the accent.
func (p *Panel) action(ui *uicore.Context, key string, x *float32, y, h float32, icon, text string, on, primary bool) bool {
	th := &ui.Theme
	w := ui.TextSize(text).X + 40
	r := mathx.Rect{X: *x, Y: y, W: w, H: h}
	*x += w + 6
	id := ui.ID(key)
	hit := on && ui.Clicked(id, r)
	bg, fg, border := th.Widget, th.Text, th.Border
	switch {
	case !on:
		bg, fg = th.Panel, th.TextDisabled
	case primary:
		bg, fg, border = th.Accent, th.Background, th.Accent
		if ui.Hot() == id {
			bg = th.AccentDim
		}
	case ui.Hot() == id:
		bg = th.WidgetHot
	}
	ui.Box(r, th.Radius.Button, bg, border)
	widgets.IconAt(ui, mathx.Rect{X: r.X + 10, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, icon, fg)
	ui.Text(text, mathx.Vec2{X: r.X + 30, Y: ui.TextY(r)}, fg)
	return hit
}

func (p *Panel) settings(ui *uicore.Context) {
	th := &ui.Theme
	x := p.d.Exporter
	p.segments(ui, field(ui, i18n.Tr("editor.export.target", "Target")), "target", x.Targets(), targetNames(), &p.target)
	prof := field(ui, i18n.Tr("editor.export.profile", "Profile"))
	w := p.segments(ui, prof, "profile", profiles, profileNames(), &p.profile)
	note := mathx.Rect{X: prof.X + w + 10, Y: prof.Y, W: max(0, prof.W-w-10), H: prof.H}
	if n := profileNote(p.profile); ui.TextSize(n).X <= note.W {
		ui.Text(n, mathx.Vec2{X: note.X, Y: ui.TextY(note)}, th.TextDisabled)
	}
	out := p.output()
	row := field(ui, i18n.Tr("editor.export.output", "Output"))
	bw := float32(0)
	if p.d.PickFolder != nil {
		bw = row.H + 4
		if p.out != "" {
			bw += row.H + 4
		}
	}
	field := mathx.Rect{X: row.X, Y: row.Y, W: max(0, row.W-bw), H: row.H}
	ui.Box(field, th.Radius.Field, th.Background, th.Border)
	inner := mathx.Rect{X: field.X + 8, Y: field.Y, W: max(0, field.W-16), H: field.H}
	if out != p.shownOf || inner.W != p.shownW {
		p.shownOf, p.shownW, p.shown = out, inner.W, out
		if ui.TextSize(out).X > inner.W {
			rs := []rune(out)
			for len(rs) > 1 && ui.TextSize("…"+string(rs)).X > inner.W {
				rs = rs[1:]
			}
			p.shown = "…" + string(rs)
		}
	}
	shown := p.shown
	ui.PushClip(inner)
	ui.Text(shown, mathx.Vec2{X: inner.X, Y: ui.TextY(field)}, th.Text)
	ui.PopClip()
	widgets.Tooltip(ui, ui.ID("outtip"), out)
	ui.MouseOver(ui.ID("outtip"), field)
	if p.d.PickFolder != nil {
		bx := field.X + field.W + 4
		if widgets.IconButton(ui, ui.ID("browse"), mathx.Rect{X: bx, Y: row.Y, W: row.H, H: row.H}, "file.folder", i18n.Tr("editor.export.choose_the_output_folder", "Choose the output folder")) && p.running == nil {
			p.d.PickFolder(filepath.Dir(out), func(dir string) { p.out = dir })
		}
		if p.out != "" && widgets.IconButton(ui, ui.ID("default"), mathx.Rect{X: bx + row.H + 4, Y: row.Y, W: row.H, H: row.H}, "ui.close", i18n.Tr("editor.export.back_to_the_default_folder", "Back to the default folder")) && p.running == nil {
			p.out = ""
		}
	}
	ui.Cursor().Next(2)
	busy := p.running != nil
	done := p.rep != nil && !p.rep.Blocked && p.rep.Out != ""
	bar := ui.Cursor().Next(th.RowHeight + 6)
	bx := bar.X + labelW
	if p.action(ui, "export", &bx, bar.Y, bar.H, "file.package", i18n.Tr("editor.export.action.export", "Export"), !busy, true) {
		p.export(out)
	}
	if p.action(ui, "check", &bx, bar.Y, bar.H, "ui.check", i18n.Tr("editor.export.action.check", "Check"), !busy, false) {
		p.check()
	}
	if done {
		if p.action(ui, "reveal", &bx, bar.Y, bar.H, "file.folder", i18n.Tr("editor.export.action.reveal", "Open Folder"), !busy, false) {
			if err := x.Reveal(p.rep.Out); err != nil {
				p.msg = "Cannot open the file manager: " + err.Error()
			}
		}
		if rn, ok := x.(runner); ok && p.action(ui, "run", &bx, bar.Y, bar.H, "play.start", i18n.Tr("editor.export.action.run", "Run"), !busy, false) {
			if err := rn.Run(p.rep.Out, p.target); err != nil {
				p.msg = "Cannot run the export: " + err.Error()
			}
		}
	}
	p.progress(ui)
}

// progress shows the running task as a bar with its stage, or the last result as a line with an icon.
func (p *Panel) progress(ui *uicore.Context) {
	th := &ui.Theme
	if p.running != nil {
		s, _ := p.stage.Load().(string)
		pct := p.running.Progress()
		row := indent(ui.Cursor().Next(th.RowHeight))
		cancel := mathx.Rect{X: row.X + row.W - row.H, Y: row.Y, W: row.H, H: row.H}
		track := mathx.Rect{X: row.X, Y: row.Y + row.H/2 - 3, W: max(0, row.W-row.H-8), H: 6}
		ui.RoundRect(track, 3, th.Widget)
		ui.RoundRect(mathx.Rect{X: track.X, Y: track.Y, W: track.W * mathx.Clamp(pct, 0, 1), H: track.H}, 3, th.Accent)
		if widgets.IconButton(ui, ui.ID("cancel"), cancel, "ui.close", i18n.Tr("editor.export.cancel", "Cancel")) {
			p.running.Cancel()
		}
		line := indent(ui.Cursor().Next(ui.LineHeight() + 2))
		ui.PushClip(line)
		ui.Text(strconv.Itoa(int(pct*100))+"%  "+s, mathx.Vec2{X: line.X, Y: line.Y}, th.TextDim)
		ui.PopClip()
		return
	}
	if p.msg == "" {
		return
	}
	icon, col := "log.info", th.TextDim
	switch e, w := errorCount(p.issues); {
	case p.rep != nil && !p.rep.Blocked && p.rep.Out != "" && e == 0:
		icon, col = "status.ok", th.Ok
	case e > 0 || (p.rep != nil && p.rep.Blocked):
		icon, col = "log.error", th.Error
	case w > 0:
		icon, col = "status.warning", th.Warn
	case p.rep == nil && len(p.issues) == 0:
		icon, col = "status.ok", th.Ok
	}
	row := indent(ui.Cursor().Next(0))
	widgets.IconAt(ui, mathx.Rect{X: row.X, Y: row.Y + (row.H-16)/2, W: 16, H: 16}, icon, col)
	ui.PushClip(row)
	ui.Text(p.msg, mathx.Vec2{X: row.X + 22, Y: ui.TextY(row)}, col)
	ui.PopClip()
	tip := p.msg
	if p.rep != nil && p.rep.Out != "" {
		tip = p.rep.Out
	}
	widgets.Tooltip(ui, ui.ID("msgtip"), tip)
	ui.MouseOver(ui.ID("msgtip"), row)
}

func (p *Panel) results(ui *uicore.Context, r mathx.Rect) {
	th := &ui.Theme
	lines := p.lines(ui)
	head := mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: th.RowHeight}
	title := i18n.Tr("editor.export.report", "Report")
	if e, w := errorCount(p.issues); e > 0 {
		title += " · " + i18n.TrN("editor.export.errors", int64(e), "{n} error", "{n} errors")
	} else if w > 0 {
		title += " · " + i18n.TrN("editor.export.warnings", int64(w), "{n} warning", "{n} warnings")
	}
	ui.Text(title, mathx.Vec2{X: head.X, Y: ui.TextY(head)}, th.TextDim)
	list := mathx.Rect{X: r.X, Y: head.Y + head.H + 4, W: r.W, H: max(0, r.H-head.H-4)}
	ui.Box(list, th.Radius.Field, th.PanelAlt, th.Border)
	if len(lines) == 0 {
		ui.TextIn(list, "Check or export to see the report here", th.TextDisabled, uicore.AlignCenter)
		return
	}
	inner := mathx.Rect{X: list.X + 4, Y: list.Y + 4, W: list.W - 8, H: list.H - 8}
	widgets.VirtualList(ui, ui.ID("rows"), inner, len(lines), th.RowHeight+2, &p.scroll, func(i int, rr mathx.Rect) {
		l := lines[i]
		if i%2 == 1 {
			ui.Rect(rr, th.Panel)
		}
		tx := rr.X + 6
		if l.icon != "" {
			widgets.IconAt(ui, mathx.Rect{X: rr.X + 4, Y: rr.Y + (rr.H-16)/2, W: 16, H: 16}, l.icon, l.col)
			tx += 22
		}
		rw := float32(0)
		if l.right != "" {
			rw = ui.TextSize(l.right).X + 16
			ui.TextIn(mathx.Rect{X: rr.X, Y: rr.Y, W: rr.W - 6, H: rr.H}, l.right, th.TextDim, uicore.AlignRight)
		}
		ui.PushClip(mathx.Rect{X: rr.X, Y: rr.Y, W: max(0, rr.W-rw), H: rr.H})
		ui.Text(l.text, mathx.Vec2{X: tx, Y: ui.TextY(rr)}, l.col)
		ui.PopClip()
	})
}

// ShowReport shows a finished export, for tests and screenshots.
func (p *Panel) ShowReport(target, profile string, rep services.ExportReport, msg string) {
	p.target, p.profile, p.rep, p.issues, p.msg = target, profile, &rep, rep.Issues, msg
}

func indent(r mathx.Rect) mathx.Rect {
	return mathx.Rect{X: r.X + labelW, Y: r.Y, W: max(0, r.W-labelW), H: r.H}
}
