package launcher

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"aego/core/mathx"
	"aego/project"
	"aego/ui/core"
	"aego/ui/widgets"
)

type Template struct {
	ID          string
	Name        string
	Description string
	Icon        string
}

type Options struct {
	Templates  []Template
	Examples   []string
	Create     func(dir, template, module, example string) (string, error)
	PickFolder func(start string, done func(string))
	Docs       func()
	RecentPath string
	Home       string
}

type Launcher struct {
	opt     Options
	recent  []Entry
	name    string
	dir     string
	module  string
	open    string
	tmpl    int
	example int
	err     string
	result  string
	scroll  float32
	content float32
}

func New(opt Options) *Launcher {
	l := &Launcher{opt: opt, name: "new-game", dir: opt.Home, tmpl: 1}
	if l.tmpl >= len(opt.Templates) {
		l.tmpl = 0
	}
	l.reload()
	return l
}

func (l *Launcher) reload() {
	if l.opt.RecentPath == "" {
		return
	}
	list, err := LoadRecent(l.opt.RecentPath)
	if err != nil {
		l.err = "Recent projects are unreadable: " + err.Error()
	}
	l.recent = list
}

func (l *Launcher) Result() (string, bool) {
	return l.result, l.result != ""
}

func (l *Launcher) Recent() []Entry {
	return l.recent
}

func (l *Launcher) templateID() string {
	if l.tmpl < 0 || l.tmpl >= len(l.opt.Templates) {
		return ""
	}
	return l.opt.Templates[l.tmpl].ID
}

func (l *Launcher) Create(dir, template, module, example string) (string, error) {
	if l.opt.Create == nil {
		return "", errors.New("project creation is not available")
	}
	root, err := l.opt.Create(dir, template, module, example)
	if err != nil {
		l.err = err.Error()
		return "", err
	}
	l.err, l.result = "", root
	return root, nil
}

func (l *Launcher) Open(dir string) error {
	abs, err := filepath.Abs(strings.TrimSpace(dir))
	if err == nil {
		_, err = os.Stat(filepath.Join(abs, project.FileName))
	}
	if err != nil {
		l.err = "No " + project.FileName + " in " + dir
		return errors.New(l.err)
	}
	l.err, l.result = "", abs
	return nil
}

func (l *Launcher) Draw(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	ui.Rect(r, th.Background)
	if ui.MouseOver(ui.ID("launcher.scroll"), r) {
		if w := ui.WheelY(); w != 0 {
			l.scroll -= w * th.RowHeight * 3
		}
	}
	l.scroll = mathx.Clamp(l.scroll, 0, max(0, l.content-r.H+48))
	w := min(r.W-48, 900)
	cur := ui.Cursor()
	cur.X, cur.W = r.X+(r.W-w)/2, w
	cur.Y = r.Y + 24 - l.scroll
	top := cur.Y
	widgets.Label(ui, "Aego")
	widgets.LabelDim(ui, "Create a project, open a folder or pick a recent project.")
	if l.err != "" {
		row := cur.Next(0)
		ui.PushClip(row)
		ui.Text(l.err, mathx.Vec2{X: row.X, Y: ui.TextY(row)}, th.Error)
		ui.PopClip()
	}
	l.heading(ui, "New project")
	l.newForm(ui)
	l.heading(ui, "Open")
	l.openForm(ui)
	l.heading(ui, "Recent")
	l.recentList(ui)
	l.links(ui)
	l.content = cur.Y - top
}

func (l *Launcher) heading(ui *core.Context, s string) {
	ui.Cursor().Y += 10
	widgets.Label(ui, s)
	widgets.Separator(ui)
}

func (l *Launcher) newForm(ui *core.Context) {
	th := &ui.Theme
	widgets.TextFieldRect(ui, ui.ID("new.name"), widgets.Field(ui, "Name"), &l.name, true)
	row := widgets.Field(ui, "Location")
	if l.opt.PickFolder != nil {
		bw := ui.TextSize("Browse…").X + 20
		widgets.TextFieldRect(ui, ui.ID("new.dir"), mathx.Rect{X: row.X, Y: row.Y, W: row.W - bw - 4, H: row.H}, &l.dir, true)
		if widgets.ButtonRect(ui, ui.ID("new.pick"), mathx.Rect{X: row.X + row.W - bw, Y: row.Y, W: bw, H: row.H}, "Browse…") {
			l.opt.PickFolder(l.dir, func(p string) { l.dir = p })
		}
	} else {
		widgets.TextFieldRect(ui, ui.ID("new.dir"), row, &l.dir, true)
	}
	mid := ui.ID("new.module")
	mr := widgets.Field(ui, "Module")
	widgets.TextFieldRect(ui, mid, mr, &l.module, true)
	if l.module == "" && ui.Focus() != mid {
		ui.Text(project.ModuleName(strings.TrimSpace(l.name)), mathx.Vec2{X: mr.X + 4, Y: ui.TextY(mr)}, th.TextDisabled)
	}
	target := filepath.Join(l.dir, strings.TrimSpace(l.name))
	widgets.LabelDim(ui, target)
	l.cards(ui)
	if widgets.Button(ui, "Create") && strings.TrimSpace(l.name) != "" {
		l.Create(target, l.templateID(), strings.TrimSpace(l.module), "")
	}
}

func (l *Launcher) cards(ui *core.Context) {
	th := &ui.Theme
	n := len(l.opt.Templates)
	if n == 0 {
		return
	}
	row := ui.Cursor().Next(100)
	const gap = float32(8)
	w := (row.W - gap*float32(n-1)) / float32(n)
	for i, t := range l.opt.Templates {
		cr := mathx.Rect{X: row.X + float32(i)*(w+gap), Y: row.Y, W: w, H: row.H}
		id := ui.ID("tmpl/" + t.ID)
		if ui.Clicked(id, cr) {
			l.tmpl = i
		}
		bg, border := th.CardBg, th.Border
		switch {
		case i == l.tmpl:
			bg, border = th.SelectionDim, th.Accent
		case ui.Hot() == id:
			bg = th.WidgetHot
		}
		ui.Rect(cr, bg)
		ui.Outline(cr, border)
		ui.PushClip(cr)
		widgets.IconAt(ui, mathx.Rect{X: cr.X + 10, Y: cr.Y + 10, W: 32, H: 32}, t.Icon, th.Text)
		ui.Text(t.Name, mathx.Vec2{X: cr.X + 52, Y: cr.Y + 18}, th.Text)
		lh := ui.TextSize("Ag").Y + 2
		for j, line := range widgets.WrapLines(ui, t.Description, cr.W-20) {
			if j == 2 {
				break
			}
			ui.Text(line, mathx.Vec2{X: cr.X + 10, Y: cr.Y + 52 + float32(j)*lh}, th.TextDim)
		}
		ui.PopClip()
		widgets.Tooltip(ui, id, t.Description)
	}
}

func (l *Launcher) openForm(ui *core.Context) {
	row := ui.Cursor().Next(0)
	bw := float32(90)
	widgets.TextFieldRect(ui, ui.ID("open.dir"), mathx.Rect{X: row.X, Y: row.Y, W: row.W - bw - 4, H: row.H}, &l.open, true)
	if widgets.ButtonRect(ui, ui.ID("open.go"), mathx.Rect{X: row.X + row.W - bw, Y: row.Y, W: bw, H: row.H}, "Open") {
		l.Open(l.open)
	}
}

func (l *Launcher) recentList(ui *core.Context) {
	th := &ui.Theme
	if len(l.recent) == 0 {
		widgets.LabelDim(ui, "No recent projects")
		return
	}
	forget := -1
	for i, e := range l.recent {
		row := ui.Cursor().Next(th.RowHeight + 8)
		id := ui.ID("recent/" + e.Path)
		name := e.Name
		if name == "" {
			name = filepath.Base(e.Path)
		}
		nameCol, dimCol := th.Text, th.TextDim
		right := row.X + row.W
		if e.Missing {
			nameCol, dimCol = th.TextDisabled, th.TextDisabled
			bw := ui.TextSize("Remove").X + 20
			if widgets.ButtonRect(ui, ui.ID("forget/"+e.Path), mathx.Rect{X: right - bw, Y: row.Y + 4, W: bw, H: row.H - 8}, "Remove") {
				forget = i
			}
			right -= bw + 8
		} else {
			if ui.Clicked(id, row) {
				l.Open(e.Path)
			}
			if ui.Hot() == id {
				ui.Rect(row, th.WidgetHot)
			}
		}
		date := ""
		if !e.Time.IsZero() {
			date = e.Time.Format("2006-01-02 15:04")
		}
		dw := ui.TextSize(date).X
		ui.Text(date, mathx.Vec2{X: right - dw - 6, Y: ui.TextY(row)}, dimCol)
		ui.PushClip(mathx.Rect{X: row.X, Y: row.Y, W: max(0, right-dw-row.X-16), H: row.H})
		ui.Text(name, mathx.Vec2{X: row.X + 6, Y: ui.TextY(row)}, nameCol)
		ui.Text(e.Path, mathx.Vec2{X: row.X + 20 + ui.TextSize(name).X, Y: ui.TextY(row)}, dimCol)
		ui.PopClip()
	}
	if forget >= 0 && l.opt.RecentPath != "" {
		if err := Forget(l.opt.RecentPath, l.recent[forget].Path); err != nil {
			l.err = err.Error()
		}
		l.reload()
	}
}

func (l *Launcher) links(ui *core.Context) {
	if l.opt.Docs == nil && len(l.opt.Examples) == 0 {
		return
	}
	l.heading(ui, "Learn")
	if l.opt.Docs != nil && widgets.Button(ui, "Documentation") {
		l.opt.Docs()
	}
	if len(l.opt.Examples) == 0 {
		return
	}
	row := ui.Cursor().Next(0)
	bw := ui.TextSize("Create from example").X + 20
	widgets.Combo(ui, ui.ID("example"), mathx.Rect{X: row.X, Y: row.Y, W: row.W - bw - 4, H: row.H}, &l.example, l.opt.Examples, false)
	if widgets.ButtonRect(ui, ui.ID("example.go"), mathx.Rect{X: row.X + row.W - bw, Y: row.Y, W: bw, H: row.H}, "Create from example") {
		ex := l.opt.Examples[l.example]
		l.Create(filepath.Join(l.dir, ex), "", strings.TrimSpace(l.module), ex)
	}
}
