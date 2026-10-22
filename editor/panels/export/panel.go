package export

import (
	"path/filepath"
	"strconv"
	"sync/atomic"

	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/editor/core/tasks"
	"aego/engine/problems"
	uicore "aego/ui/core"
	"aego/ui/widgets"
)

const (
	ID          = "export"
	Release     = "release"
	Development = "development"
	labelW      = 80
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
}

func New(d Deps) *Panel {
	return &Panel{d: d, target: "host", profile: Development}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	return "Export"
}

func (p *Panel) Icon() string {
	return "file.package"
}

func (p *Panel) choices(ui *uicore.Context, label string, opts []string, cur *string, key string) {
	th := &ui.Theme
	row := ui.Cursor().Next(0)
	ui.Text(label, mathx.Vec2{X: row.X, Y: ui.TextY(row)}, th.TextDim)
	x := row.X + labelW
	for i, o := range opts {
		w := ui.TextSize(o).X + 16
		rr := mathx.Rect{X: x, Y: row.Y, W: w, H: row.H}
		if widgets.ButtonRect(ui, uicore.IDSeq(ui.ID(key), i), rr, o) && p.running == nil {
			*cur = o
		}
		if *cur == o {
			ui.List().RectOutline(rr, th.Accent, 2)
		}
		x += w + 4
	}
}

func (p *Panel) button(ui *uicore.Context, key string, row mathx.Rect, x *float32, label string, on bool) bool {
	th := &ui.Theme
	w := ui.TextSize(label).X + 16
	rr := mathx.Rect{X: *x, Y: row.Y, W: w, H: row.H}
	*x += w + 6
	if !on {
		ui.List().RectOutline(rr, th.TextDisabled, 1)
		ui.Text(label, mathx.Vec2{X: rr.X + 8, Y: ui.TextY(rr)}, th.TextDisabled)
		return false
	}
	return widgets.ButtonRect(ui, ui.ID(key), rr, label)
}

func (p *Panel) status() string {
	if p.running != nil {
		s, _ := p.stage.Load().(string)
		return "Working… " + strconv.Itoa(int(p.running.Progress()*100)) + "% · " + s
	}
	return p.msg
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
	p.running = p.d.Tasks.Work(tasks.Spec{Title: "Check export (" + prof + ")", Kind: "export"}, func(*tasks.Task) error {
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
			p.msg = "Ready to export, " + strconv.Itoa(w) + " warning(s)"
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
	p.running = p.d.Tasks.Work(tasks.Spec{Title: "Export " + req.Target + " " + req.Profile, Kind: "export", Detail: out}, func(t *tasks.Task) error {
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
			p.msg = "Exported to " + rep.Out
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
	x := p.d.Exporter
	p.choices(ui, "Target", x.Targets(), &p.target, "target")
	p.choices(ui, "Profile", profiles, &p.profile, "profile")
	out := p.output()
	row := ui.Cursor().Next(0)
	ui.Text("Output", mathx.Vec2{X: row.X, Y: ui.TextY(row)}, th.TextDim)
	bw := float32(0)
	if p.d.PickFolder != nil {
		bw = ui.TextSize("Browse…").X + 16
		if p.out != "" {
			bw += ui.TextSize("Default").X + 22
		}
	}
	path := mathx.Rect{X: row.X + labelW, Y: row.Y, W: max(0, row.W-labelW-bw), H: row.H}
	ui.PushClip(path)
	ui.Text(out, mathx.Vec2{X: path.X, Y: ui.TextY(row)}, th.Text)
	ui.PopClip()
	if bw > 0 {
		bx := path.X + path.W + 6
		if p.button(ui, "browse", row, &bx, "Browse…", p.running == nil) {
			p.d.PickFolder(filepath.Dir(out), func(dir string) { p.out = dir })
		}
		if p.out != "" && p.button(ui, "default", row, &bx, "Default", p.running == nil) {
			p.out = ""
		}
	}
	row = ui.Cursor().Next(0)
	busy := p.running != nil
	bx := row.X
	if p.button(ui, "check", row, &bx, "Check", !busy) {
		p.check()
	}
	if p.button(ui, "export", row, &bx, "Export", !busy) {
		p.export(out)
	}
	done := p.rep != nil && !p.rep.Blocked && p.rep.Out != ""
	if p.button(ui, "reveal", row, &bx, "Open Folder", !busy && done) {
		if err := x.Reveal(p.rep.Out); err != nil {
			p.msg = "Cannot open the file manager: " + err.Error()
		}
	}
	if rn, ok := x.(runner); ok && p.button(ui, "run", row, &bx, "Run", !busy && done) {
		if err := rn.Run(p.rep.Out, p.target); err != nil {
			p.msg = "Cannot run the export: " + err.Error()
		}
	}
	row = ui.Cursor().Next(0)
	ui.PushClip(row)
	ui.Text(p.status(), mathx.Vec2{X: row.X, Y: ui.TextY(row)}, th.TextDim)
	ui.PopClip()
	lines := p.lines(ui)
	top := ui.Cursor().Y
	list := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	widgets.VirtualList(ui, ui.ID("rows"), list, len(lines), th.RowHeight+2, &p.scroll, func(i int, rr mathx.Rect) {
		l := lines[i]
		tx := rr.X + 4
		if l.icon != "" {
			widgets.IconAt(ui, mathx.Rect{X: rr.X + 4, Y: rr.Y + (rr.H-16)/2, W: 16, H: 16}, l.icon, l.col)
			tx += 20
		}
		ui.PushClip(rr)
		ui.Text(l.text, mathx.Vec2{X: tx, Y: ui.TextY(rr)}, l.col)
		if l.right != "" {
			ui.TextIn(rr, l.right, th.TextDim, uicore.AlignRight)
		}
		ui.PopClip()
	})
}
