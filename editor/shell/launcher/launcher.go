package launcher

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/preview"
	"aego/editor/core/state"
	"aego/project"
	"aego/render/texture"
	"aego/ui/core"
	"aego/ui/widgets"
)

type Tab uint8

const (
	TabRecent Tab = iota
	TabNew
	TabExamples
)

var tabNames = [...]string{"Recent", "New Project", "Examples"}

const (
	cardW float32 = 200
	cardH float32 = 176
	gap   float32 = 10
	goURL         = "https://go.dev/dl/"
)

type Template struct {
	ID          string
	Name        string
	Description string
	Icon        string
	Files       []string
	Preview     func() ([]byte, error)
}

type Choice struct {
	ID    string
	Title string
}

type CreateRequest struct {
	Dir      string
	Template string
	Module   string
	Example  string
	Native   bool
}

type Options struct {
	Templates       []Template
	Examples        []string
	ExamplePreview  func(name string) ([]byte, error)
	Workspaces      []Choice
	Create          func(dir, template, module, example string) (string, error)
	CreateWith      func(CreateRequest) (string, error)
	NativeHint      string
	PickFolder      func(start string, done func(string))
	PickProject     func(start string, done func(string))
	Reveal          func(path string) error
	RevealLabel     string
	OpenURL         func(url string) error
	Update          func() (version, url string, ok bool)
	UpdateChecks    bool
	SetUpdateChecks func(on bool)
	Docs            func()
	RecentPath      string
	Home            string
	Image           func(key string, load func() ([]byte, error)) (texture.Region, bool)
}

type Launcher struct {
	opt     Options
	recent  []Entry
	tab     Tab
	name    string
	dir     string
	module  string
	open    string
	tmpl    int
	example int
	ws      int
	native  bool
	updates bool
	menu    string
	err     string
	result  string
	scroll  float32
	content float32
}

func New(opt Options) *Launcher {
	l := &Launcher{opt: opt, name: "new-game", dir: opt.Home, tmpl: 1, updates: opt.UpdateChecks}
	if l.tmpl >= len(opt.Templates) {
		l.tmpl = 0
	}
	l.reload()
	if len(l.recent) == 0 {
		l.tab = TabNew
	}
	return l
}

func PreviewPath(root string) string {
	return filepath.Join((&project.Project{Root: root}).CacheDir(), preview.CoverFile)
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

func (l *Launcher) SetPickers(folder, proj func(start string, done func(string))) {
	l.opt.PickFolder, l.opt.PickProject = folder, proj
}

func (l *Launcher) Result() (string, bool) {
	return l.result, l.result != ""
}

func (l *Launcher) Recent() []Entry {
	return l.recent
}

func (l *Launcher) Tab() Tab {
	return l.tab
}

func (l *Launcher) SetTab(t Tab) {
	l.tab, l.scroll = t, 0
}

func (l *Launcher) SetWorkspace(id string) {
	for i, c := range l.opt.Workspaces {
		if c.ID == id {
			l.ws = i
		}
	}
}

func (l *Launcher) templateID() string {
	if l.tmpl < 0 || l.tmpl >= len(l.opt.Templates) {
		return ""
	}
	return l.opt.Templates[l.tmpl].ID
}

func (l *Launcher) workspace() string {
	if l.ws <= 0 || l.ws >= len(l.opt.Workspaces) {
		return ""
	}
	return l.opt.Workspaces[l.ws].ID
}

func (l *Launcher) Create(dir, template, module, example string) (string, error) {
	return l.create(CreateRequest{Dir: dir, Template: template, Module: module, Example: example, Native: l.native && l.opt.NativeHint == "" && example == ""})
}

func (l *Launcher) create(req CreateRequest) (string, error) {
	var root string
	var err error
	switch {
	case l.opt.CreateWith != nil:
		root, err = l.opt.CreateWith(req)
	case l.opt.Create != nil:
		root, err = l.opt.Create(req.Dir, req.Template, req.Module, req.Example)
	default:
		err = errors.New("project creation is not available")
	}
	if err != nil {
		l.err = err.Error()
		return "", err
	}
	if id := l.workspace(); id != "" {
		st := state.Default()
		st.Workspace = id
		if err := state.Save(root, st); err != nil {
			l.err = "The project was created, but its workspace was not saved: " + err.Error()
		}
	}
	l.result = root
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

func ProjectRoot(path string) (string, bool) {
	if filepath.Base(path) == project.FileName {
		return filepath.Dir(path), true
	}
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		if _, err := os.Stat(filepath.Join(path, project.FileName)); err == nil {
			return path, true
		}
	}
	return "", false
}

func (l *Launcher) Drop(path string) bool {
	p := strings.TrimSpace(path)
	if filepath.Base(p) == project.FileName {
		p = filepath.Dir(p)
	}
	return l.Open(p) == nil
}

func (l *Launcher) image(key string, load func() ([]byte, error)) (texture.Region, bool) {
	if l.opt.Image == nil || load == nil {
		return texture.Region{}, false
	}
	return l.opt.Image(key, load)
}

func (l *Launcher) Draw(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	ui.Rect(r, th.Background)
	if ui.MouseOver(ui.ID("launcher.scroll"), r) {
		if w := ui.ScrollY(th.RowHeight * 3); w != 0 {
			l.scroll -= w
		}
	}
	l.scroll = mathx.Clamp(l.scroll, 0, max(0, l.content-r.H+48))
	w := min(r.W-48, 960)
	cur := ui.Cursor()
	cur.X, cur.W = r.X+(r.W-w)/2, w
	cur.Y = r.Y + 24 - l.scroll
	top := cur.Y
	head := cur.Next(0)
	s := min(head.H, 16)
	widgets.IconAt(ui, mathx.Rect{X: head.X, Y: head.Y + (head.H-s)/2, W: s, H: s}, "app", th.Text)
	ui.Text("Aego", mathx.Vec2{X: head.X + s + 6, Y: ui.TextY(head)}, th.Text)
	widgets.LabelDim(ui, "Open a recent project, start a new one or learn from an example.")
	l.banner(ui)
	l.tabs(ui)
	if l.err != "" {
		row := cur.Next(0)
		ui.PushClip(row)
		ui.Text(l.err, mathx.Vec2{X: row.X, Y: ui.TextY(row)}, th.Error)
		ui.PopClip()
	}
	switch l.tab {
	case TabRecent:
		l.recentTab(ui)
	case TabNew:
		l.newTab(ui)
	case TabExamples:
		l.examplesTab(ui)
	}
	cur.Y += 10
	if l.opt.Docs != nil && widgets.Button(ui, "Documentation") {
		l.opt.Docs()
	}
	if l.opt.SetUpdateChecks != nil && widgets.Checkbox(ui, "Check for Aego updates once a day", &l.updates) {
		l.opt.SetUpdateChecks(l.updates)
	}
	l.content = cur.Y - top
}

func (l *Launcher) banner(ui *core.Context) {
	if l.opt.Update == nil {
		return
	}
	ver, url, ok := l.opt.Update()
	if !ok {
		return
	}
	th := &ui.Theme
	row := ui.Cursor().Next(0)
	ui.RoundRect(row, th.Radius.Card, th.SelectionDim)
	ui.Text("Aego "+ver+" is available", mathx.Vec2{X: row.X + 8, Y: ui.TextY(row)}, th.Text)
	bw := ui.TextSize("Download").X + 20
	if l.opt.OpenURL != nil && widgets.ButtonRect(ui, ui.ID("update.get"), mathx.Rect{X: row.X + row.W - bw, Y: row.Y, W: bw, H: row.H}, "Download") {
		if err := l.opt.OpenURL(url); err != nil {
			l.err = err.Error()
		}
	}
}

func (l *Launcher) tabs(ui *core.Context) {
	row := ui.Cursor().Next(0)
	w := min(float32(150), row.W/float32(len(tabNames)))
	for i, name := range tabNames {
		on := l.tab == Tab(i)
		if widgets.ToggleButton(ui, core.IDSeq(ui.ID("tab"), i), mathx.Rect{X: row.X + float32(i)*(w+4), Y: row.Y, W: w, H: row.H}, name, &on) {
			l.SetTab(Tab(i))
		}
	}
	ui.Cursor().Y += 6
}

func (l *Launcher) heading(ui *core.Context, s string) {
	ui.Cursor().Y += 10
	widgets.Label(ui, s)
	widgets.Separator(ui)
}

func (l *Launcher) grid(ui *core.Context, n int, fn func(i int, r mathx.Rect)) {
	cur := ui.Cursor()
	cols := max(1, int((cur.W+gap)/(cardW+gap)))
	cw := (cur.W - gap*float32(cols-1)) / float32(cols)
	for i := 0; i < n; i += cols {
		row := cur.Next(cardH)
		for c := 0; c < cols && i+c < n; c++ {
			fn(i+c, mathx.Rect{X: row.X + float32(c)*(cw+gap), Y: row.Y, W: cw, H: row.H})
		}
		cur.Y += gap - cur.Space
	}
}

func fit(r mathx.Rect, w, h int) mathx.Rect {
	if w <= 0 || h <= 0 {
		return r
	}
	s := min(r.W/float32(w), r.H/float32(h))
	fw, fh := float32(w)*s, float32(h)*s
	return mathx.Rect{X: r.X + (r.W-fw)/2, Y: r.Y + (r.H-fh)/2, W: fw, H: fh}
}

type cardSpec struct {
	title    string
	sub      string
	icon     string
	key      string
	load     func() ([]byte, error)
	selected bool
	dim      bool
}

func (l *Launcher) card(ui *core.Context, id core.ID, r mathx.Rect, c cardSpec) bool {
	th := &ui.Theme
	clicked := ui.Clicked(id, r)
	bg, border := th.CardBg, th.Border
	switch {
	case c.selected:
		bg, border = th.SelectionDim, th.Accent
	case ui.Hot() == id:
		bg = th.WidgetHot
	}
	ui.Box(r, th.Radius.Card, bg, border)
	lh := ui.LineHeight()
	pr := mathx.Rect{X: r.X + 6, Y: r.Y + 6, W: r.W - 12, H: r.H - 16 - 2*lh}
	ui.Rect(pr, th.PanelAlt)
	tint := mathx.Color{R: 1, G: 1, B: 1, A: 1}
	if c.dim {
		tint.A = 0.4
	}
	if reg, ok := l.image(c.key, c.load); ok {
		ui.List().Texture(reg, fit(pr, int(reg.W), int(reg.H)), tint)
	} else if c.icon != "" {
		s := min(pr.H*0.5, 40)
		widgets.IconAt(ui, mathx.Rect{X: pr.X + (pr.W-s)/2, Y: pr.Y + (pr.H-s)/2, W: s, H: s}, c.icon, th.TextDim)
	}
	text, dim := th.Text, th.TextDim
	if c.dim {
		text, dim = th.TextDisabled, th.TextDisabled
	}
	ui.PushClip(r)
	ui.Text(c.title, mathx.Vec2{X: r.X + 8, Y: pr.Y + pr.H + 4}, text)
	ui.Text(widgets.Elide(ui, c.sub, r.W-16), mathx.Vec2{X: r.X + 8, Y: pr.Y + pr.H + 4 + lh}, dim)
	ui.PopClip()
	return clicked
}

func (l *Launcher) forget(path string) {
	if l.opt.RecentPath == "" {
		return
	}
	if err := Forget(l.opt.RecentPath, path); err != nil {
		l.err = err.Error()
	}
	l.reload()
}

func (l *Launcher) recentTab(ui *core.Context) {
	if len(l.recent) == 0 {
		widgets.LabelDim(ui, "No recent projects yet")
	}
	mid := ui.ID("recent.menu")
	forget := ""
	l.grid(ui, len(l.recent), func(i int, r mathx.Rect) {
		e := l.recent[i]
		name := e.Name
		if name == "" {
			name = filepath.Base(e.Path)
		}
		sub := e.Path
		if !e.Time.IsZero() {
			sub = e.Time.Format("2006-01-02 15:04") + " · " + e.Path
		}
		if e.Missing {
			sub = "missing, click to remove"
		}
		root := e.Path
		id := ui.ID("recent/" + e.Path)
		if ui.RightClicked(id, r) {
			l.menu = e.Path
			widgets.OpenContextMenu(ui, mid)
		}
		spec := cardSpec{title: name, sub: sub, icon: "file.folder", key: "recent:" + root, dim: e.Missing,
			load: func() ([]byte, error) { return os.ReadFile(PreviewPath(root)) }}
		if !l.card(ui, id, r, spec) {
			return
		}
		if e.Missing {
			forget = e.Path
			return
		}
		l.Open(e.Path)
	})
	if widgets.ContextMenu(ui, mid) {
		if widgets.MenuItem(ui, "Open", "", true, false) {
			l.Open(l.menu)
		}
		if l.opt.Reveal != nil && widgets.MenuItem(ui, l.opt.RevealLabel, "", true, false) {
			if err := l.opt.Reveal(filepath.Join(l.menu, project.FileName)); err != nil {
				l.err = err.Error()
			}
		}
		if widgets.MenuItem(ui, "Remove from List", "", true, false) {
			forget = l.menu
		}
		widgets.EndContextMenu(ui)
	}
	if forget != "" {
		l.forget(forget)
	}
	l.heading(ui, "Open a project")
	row := ui.Cursor().Next(0)
	bw := float32(90)
	pw := float32(0)
	if l.opt.PickProject != nil {
		pw = ui.TextSize("Browse…").X + 20
	}
	widgets.TextFieldRect(ui, ui.ID("open.dir"), mathx.Rect{X: row.X, Y: row.Y, W: max(0, row.W-bw-pw-8), H: row.H}, &l.open, true)
	if pw > 0 && widgets.ButtonRect(ui, ui.ID("open.pick"), mathx.Rect{X: row.X + row.W - bw - pw - 4, Y: row.Y, W: pw, H: row.H}, "Browse…") {
		start := l.open
		if start == "" {
			start = l.opt.Home
		}
		l.opt.PickProject(start, func(p string) {
			l.open = p
			l.Drop(p)
		})
	}
	if widgets.ButtonRect(ui, ui.ID("open.go"), mathx.Rect{X: row.X + row.W - bw, Y: row.Y, W: bw, H: row.H}, "Open") {
		l.Drop(l.open)
	}
	widgets.LabelDim(ui, "Or drop a project folder, or its "+project.FileName+", on this window.")
}

func (l *Launcher) newTab(ui *core.Context) {
	th := &ui.Theme
	l.grid(ui, len(l.opt.Templates), func(i int, r mathx.Rect) {
		t := l.opt.Templates[i]
		spec := cardSpec{title: t.Name, sub: t.Description, icon: t.Icon, key: "template:" + t.ID, load: t.Preview, selected: i == l.tmpl}
		if l.card(ui, ui.ID("tmpl/"+t.ID), r, spec) {
			l.tmpl = i
		}
	})
	if l.tmpl >= 0 && l.tmpl < len(l.opt.Templates) {
		t := l.opt.Templates[l.tmpl]
		l.heading(ui, t.Name)
		widgets.LabelWrapped(ui, t.Description, true)
		if len(t.Files) > 0 {
			widgets.LabelWrapped(ui, "Contains: "+strings.Join(t.Files, ", "), true)
		}
	}
	l.heading(ui, "Project")
	widgets.TextFieldRect(ui, ui.ID("new.name"), widgets.Field(ui, "Name"), &l.name, true)
	l.location(ui)
	mid := ui.ID("new.module")
	mr := widgets.Field(ui, "Module")
	widgets.TextFieldRect(ui, mid, mr, &l.module, true)
	if l.module == "" && ui.Focus() != mid {
		ui.Text(project.ModuleName(strings.TrimSpace(l.name)), mathx.Vec2{X: mr.X + 4, Y: ui.TextY(mr)}, th.TextDisabled)
	}
	if len(l.opt.Workspaces) > 0 {
		names := make([]string, len(l.opt.Workspaces))
		for i, c := range l.opt.Workspaces {
			names[i] = c.Title
		}
		widgets.Combo(ui, ui.ID("new.workspace"), widgets.Field(ui, "Workspace"), &l.ws, names, false)
	}
	l.nativeRow(ui)
	target := filepath.Join(l.dir, strings.TrimSpace(l.name))
	widgets.LabelDim(ui, target)
	if widgets.Button(ui, "Create") && strings.TrimSpace(l.name) != "" {
		l.Create(target, l.templateID(), strings.TrimSpace(l.module), "")
	}
}

func (l *Launcher) nativeRow(ui *core.Context) {
	if l.opt.NativeHint == "" {
		widgets.Checkbox(ui, "Native Go code", &l.native)
		return
	}
	row := ui.Cursor().Next(0)
	bw := float32(0)
	if l.opt.OpenURL != nil {
		bw = ui.TextSize("Get Go").X + 20
		if widgets.ButtonRect(ui, ui.ID("new.go"), mathx.Rect{X: row.X + row.W - bw, Y: row.Y, W: bw, H: row.H}, "Get Go") {
			if err := l.opt.OpenURL(goURL); err != nil {
				l.err = err.Error()
			}
		}
	}
	ui.PushClip(mathx.Rect{X: row.X, Y: row.Y, W: max(0, row.W-bw-8), H: row.H})
	ui.Text(l.opt.NativeHint, mathx.Vec2{X: row.X, Y: ui.TextY(row)}, ui.Theme.TextDim)
	ui.PopClip()
}

func (l *Launcher) location(ui *core.Context) {
	row := widgets.Field(ui, "Location")
	if l.opt.PickFolder == nil {
		widgets.TextFieldRect(ui, ui.ID("new.dir"), row, &l.dir, true)
		return
	}
	bw := ui.TextSize("Browse…").X + 20
	widgets.TextFieldRect(ui, ui.ID("new.dir"), mathx.Rect{X: row.X, Y: row.Y, W: row.W - bw - 4, H: row.H}, &l.dir, true)
	if widgets.ButtonRect(ui, ui.ID("new.pick"), mathx.Rect{X: row.X + row.W - bw, Y: row.Y, W: bw, H: row.H}, "Browse…") {
		l.opt.PickFolder(l.dir, func(p string) { l.dir = p })
	}
}

func (l *Launcher) examplesTab(ui *core.Context) {
	if len(l.opt.Examples) == 0 {
		widgets.LabelWrapped(ui, "Examples come from the engine source tree; set AEGO_ROOT to its folder.", true)
		return
	}
	l.grid(ui, len(l.opt.Examples), func(i int, r mathx.Rect) {
		name := l.opt.Examples[i]
		var load func() ([]byte, error)
		if l.opt.ExamplePreview != nil {
			load = func() ([]byte, error) { return l.opt.ExamplePreview(name) }
		}
		spec := cardSpec{title: name, sub: "examples/" + name, icon: "file.scene", key: "example:" + name, load: load, selected: i == l.example}
		if l.card(ui, ui.ID("example/"+name), r, spec) {
			l.example = i
		}
	})
	l.heading(ui, "Copy to")
	l.location(ui)
	ex := l.opt.Examples[min(l.example, len(l.opt.Examples)-1)]
	target := filepath.Join(l.dir, ex)
	widgets.LabelDim(ui, target)
	if widgets.Button(ui, "Create from "+ex) {
		l.Create(target, "", strings.TrimSpace(l.module), ex)
	}
}
