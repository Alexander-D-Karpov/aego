package launcher

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/preview"
	"aego/editor/core/state"
	"aego/editor/core/updateui"
	"aego/engine/i18n"
	"aego/project"
	"aego/render/texture"
	"aego/ui/core"
	"aego/ui/theme"
	"aego/ui/widgets"
	"aego/update"
)

type Tab uint8

const (
	TabRecent Tab = iota
	TabNew
	TabExamples
)

var tabNames = [...]i18n.Text{i18n.T("launcher.tab.recent", "Recent"), i18n.T("launcher.tab.new", "New Project"), i18n.T("launcher.tab.examples", "Examples")}

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
	Templates      []Template
	Examples       []string
	ExamplePreview func(name string) ([]byte, error)
	Workspaces     []Choice
	Create         func(dir, template, module, example string) (string, error)
	CreateWith     func(CreateRequest) (string, error)
	NativeHint     string
	PickFolder     func(start string, done func(string))
	PickProject    func(start string, done func(string))
	Reveal         func(path string) error
	RevealLabel    string
	OpenURL        func(url string) error
	// Updates is Aego's update service; RestartToUpdate installs the downloaded version, or goes back to the previous one, and restarts (D362).
	Updates         updateui.Host
	RestartToUpdate func(install bool) error
	Docs            func()
	RecentPath      string
	Home            string
	Image           func(key string, load func() ([]byte, error)) (texture.Region, bool)
	// Remote is data-only transport; the launcher asks before network access.
	Remote RemoteExamples
	// Theme and SetTheme read and change the editor theme preference; nil hides the Theme combo (D345).
	Theme    func() theme.Mode
	SetTheme func(theme.Mode)
	// Language and SetLanguage read and change the editor language preference, auto, en or ru; nil hides the combo (D348).
	Language    func() string
	SetLanguage func(string)
}

var languagePrefs = []string{i18n.Auto, i18n.English, i18n.Russian}

func themeModes() []string {
	return []string{i18n.Tr("launcher.theme.system", "System"), i18n.Tr("launcher.theme.dark", "Dark"), i18n.Tr("launcher.theme.light", "Light")}
}

func languageNames() []string {
	return []string{i18n.Tr("launcher.language.auto", "Auto"), "English", "Русский"}
}

type Launcher struct {
	opt          Options
	recent       []Entry
	tab          Tab
	name         string
	dir          string
	module       string
	open         string
	tmpl         int
	example      int
	ws           int
	native       bool
	autoName     bool
	menu         string
	notes        bool
	err          string
	result       string
	scroll       float32
	content      float32
	remote       remoteState
	resultEntry  string
	resultRemote bool
}

func New(opt Options) *Launcher {
	l := &Launcher{opt: opt, name: nextProjectName(opt.Home), autoName: true, dir: opt.Home, tmpl: 1}
	if l.tmpl >= len(opt.Templates) {
		l.tmpl = 0
	}
	l.reload()
	if len(l.recent) == 0 {
		l.tab = TabNew
	}
	return l
}

// nextProjectName never overwrites an existing directory, file or symlink.
func nextProjectName(dir string) string {
	const base = "new-game"
	for n := 1; ; n++ {
		name := base
		if n > 1 {
			name += "-" + strconv.Itoa(n)
		}
		if _, err := os.Lstat(filepath.Join(dir, name)); err != nil {
			// Other filesystem errors should be reported, not treated as a free name.
			if errors.Is(err, os.ErrNotExist) {
				return name
			}
			return base
		}
	}
}

func projectTarget(dir, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) {
		return "", errors.New(i18n.Tr("launcher.invalid_project_name", "Enter a project name without path separators."))
	}
	target := filepath.Join(dir, name)
	if _, err := os.Lstat(target); err == nil {
		return target, errors.New(i18n.Tr("launcher.project_folder_exists", "A file or folder with this name already exists."))
	} else if !errors.Is(err, os.ErrNotExist) {
		return target, err
	}
	return target, nil
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
		l.err = i18n.Tr("launcher.recent_unreadable", "Recent projects are unreadable: {error}", i18n.Str("error", err.Error()))
	}
	l.recent = list
}

func (l *Launcher) SetPickers(folder, proj func(start string, done func(string))) {
	l.opt.PickFolder, l.opt.PickProject = folder, proj
}

func (l *Launcher) Result() (string, bool) {
	return l.result, l.result != ""
}

// ResultEntry identifies a remote manifest's entry scene. Local projects keep
// their existing startup behavior; opening a remote project never starts Play.
func (l *Launcher) ResultEntry() (string, bool) { return l.resultEntry, l.resultRemote }

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
			l.err = i18n.Tr("launcher.workspace_not_saved", "The project was created, but its workspace was not saved: {error}", i18n.Str("error", err.Error()))
		}
	}
	l.cancelRemote()
	l.result, l.resultEntry, l.resultRemote = root, "", false
	return root, nil
}

func (l *Launcher) Open(dir string) error {
	abs, err := filepath.Abs(strings.TrimSpace(dir))
	if err == nil {
		_, err = os.Stat(filepath.Join(abs, project.FileName))
	}
	if err != nil {
		l.err = i18n.Tr("launcher.no_project_file", "No {file} in {dir}", i18n.Str("file", project.FileName), i18n.Str("dir", dir))
		return errors.New(l.err)
	}
	l.cancelRemote()
	l.err, l.result, l.resultEntry, l.resultRemote = "", abs, "", false
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
	if strings.HasPrefix(strings.ToLower(p), "aego:") {
		if err := l.RequestExampleLink(p); err != nil {
			l.err = err.Error()
			return false
		}
		return true
	}
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
	l.pollRemote()
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
	ui.Text(i18n.Tr("launcher.aego", "Aego"), mathx.Vec2{X: head.X + s + 6, Y: ui.TextY(head)}, th.Text)
	widgets.LabelDim(ui, i18n.Tr("launcher.open_a_recent_project_start", "Open a recent project, start a new one or learn from an example."))
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
	if l.opt.Docs != nil && widgets.Button(ui, i18n.Tr("launcher.documentation", "Documentation")) {
		l.opt.Docs()
	}
	l.updateSettings(ui)
	if l.opt.SetTheme != nil && l.opt.Theme != nil {
		m := int(l.opt.Theme())
		if widgets.Combo(ui, ui.ID("launcher.theme"), widgets.Field(ui, i18n.Tr("launcher.theme", "Theme")), &m, themeModes(), false) {
			l.opt.SetTheme(theme.Mode(m))
		}
	}
	if l.opt.SetLanguage != nil && l.opt.Language != nil {
		m := max(0, slices.Index(languagePrefs, l.opt.Language()))
		if widgets.Combo(ui, ui.ID("launcher.language"), widgets.Field(ui, i18n.Tr("launcher.language", "Language")), &m, languageNames(), false) {
			l.opt.SetLanguage(languagePrefs[m])
		}
	}
	l.content = cur.Y - top
}

func (l *Launcher) banner(ui *core.Context) {
	u := l.opt.Updates
	if u == nil {
		return
	}
	st := u.State()
	th := &ui.Theme
	if st.RolledBack != "" {
		row := ui.Cursor().Next(0)
		ui.RoundRect(row, th.Radius.Card, th.SelectionDim)
		ui.Text(i18n.Tr("launcher.update_rolled_back", "Aego {version} stopped right after it started, so the previous version was restored", i18n.Str("version", st.RolledBack)), mathx.Vec2{X: row.X + 8, Y: ui.TextY(row)}, th.Warn)
	}
	if st.Hidden {
		return
	}
	var text string
	var buttons []string
	switch st.Phase {
	case updateui.Available:
		text = i18n.Tr("launcher.update_available", "Aego {version} is available", i18n.Str("version", st.Version))
		if st.Installable {
			buttons = []string{i18n.Tr("launcher.update_download", "Download"), i18n.Tr("launcher.update_later", "Later"), i18n.Tr("launcher.update_skip", "Skip This Version")}
		} else {
			buttons = []string{i18n.Tr("launcher.update_page", "Get It"), i18n.Tr("launcher.update_later", "Later")}
		}
	case updateui.Downloading:
		pct := 0
		if st.Total > 0 {
			pct = int(st.Done * 100 / st.Total)
		}
		text = i18n.Tr("launcher.update_downloading", "Downloading Aego {version}: {n} %", i18n.Str("version", st.Version), i18n.Int("n", int64(pct)))
	case updateui.Ready:
		text = i18n.Tr("launcher.update_ready", "Aego {version} downloaded", i18n.Str("version", st.Version))
		buttons = []string{i18n.Tr("launcher.update_restart", "Restart to Update"), i18n.Tr("launcher.update_later", "Later"), i18n.Tr("launcher.update_skip", "Skip This Version")}
	default:
		return
	}
	if st.Notes != "" {
		buttons = append(buttons, i18n.Tr("launcher.update_notes", "Release Notes"))
	}
	row := ui.Cursor().Next(0)
	ui.RoundRect(row, th.Radius.Card, th.SelectionDim)
	x := row.X + row.W
	pick := -1
	for i := len(buttons) - 1; i >= 0; i-- {
		bw := ui.TextSize(buttons[i]).X + 20
		x -= bw
		if widgets.ButtonRect(ui, core.IDSeq(ui.ID("update.btn"), i), mathx.Rect{X: x, Y: row.Y, W: bw, H: row.H}, buttons[i]) {
			pick = i
		}
		x -= 4
	}
	ui.PushClip(mathx.Rect{X: row.X, Y: row.Y, W: max(0, x-row.X), H: row.H})
	ui.Text(text, mathx.Vec2{X: row.X + 8, Y: ui.TextY(row)}, th.Text)
	ui.PopClip()
	if pick < 0 {
		return
	}
	if st.Notes != "" && pick == len(buttons)-1 {
		l.notes = !l.notes
		return
	}
	switch {
	case st.Phase == updateui.Available && !st.Installable && pick == 0:
		if l.opt.OpenURL != nil {
			l.opt.OpenURL(u.Page())
		}
	case st.Phase == updateui.Available && pick == 0:
		u.Download()
	case st.Phase == updateui.Ready && pick == 0 && l.opt.RestartToUpdate != nil:
		if err := l.opt.RestartToUpdate(true); err != nil {
			l.err = err.Error()
		}
	case pick == 1:
		u.Later()
	case pick == 2:
		u.Skip()
	}
}

// updateSettings are the update choices under the tabs: the mode, the channel and going back to the previous version.
func (l *Launcher) updateSettings(ui *core.Context) {
	u := l.opt.Updates
	if u == nil {
		return
	}
	if l.notes {
		widgets.LabelWrapped(ui, u.State().Notes, false)
	}
	pol := u.Policy()
	modes := []update.Mode{update.Auto, update.Ask, update.Off}
	m := max(0, slices.Index(modes, pol.Mode))
	names := []string{i18n.Tr("launcher.updates_auto", "Download and install on restart"), i18n.Tr("launcher.updates_ask", "Ask before downloading"), i18n.Tr("launcher.updates_off", "Only when I check")}
	if widgets.Combo(ui, ui.ID("launcher.updates"), widgets.Field(ui, i18n.Tr("launcher.updates", "Updates")), &m, names, false) {
		u.SetMode(modes[m])
	}
	c := 0
	if pol.Channel == update.Nightly {
		c = 1
	}
	if widgets.Combo(ui, ui.ID("launcher.channel"), widgets.Field(ui, i18n.Tr("launcher.update_channel", "Channel")), &c, []string{i18n.Tr("launcher.channel_stable", "Stable"), i18n.Tr("launcher.channel_nightly", "Nightly")}, false) {
		u.SetChannel([]update.Channel{update.Stable, update.Nightly}[c])
	}
	if widgets.Button(ui, i18n.Tr("launcher.check_updates", "Check for Updates")) {
		u.Check()
	}
	if st := u.State(); st.Phase == updateui.UpToDate {
		widgets.LabelDim(ui, i18n.Tr("launcher.up_to_date", "Aego is up to date"))
	} else if st.Phase == updateui.Failed {
		widgets.LabelDim(ui, i18n.Tr("launcher.update_failed", "Could not check for updates: {error}", i18n.Str("error", st.Err)))
	}
	if u.CanRestore() && l.opt.RestartToUpdate != nil && widgets.Button(ui, i18n.Tr("launcher.restore_previous", "Restore Previous Version")) {
		if err := l.opt.RestartToUpdate(false); err != nil {
			l.err = err.Error()
		}
	}
}

func (l *Launcher) tabs(ui *core.Context) {
	row := ui.Cursor().Next(0)
	w := min(float32(150), row.W/float32(len(tabNames)))
	for i, name := range tabNames {
		on := l.tab == Tab(i)
		if widgets.ToggleButton(ui, core.IDSeq(ui.ID("tab"), i), mathx.Rect{X: row.X + float32(i)*(w+4), Y: row.Y, W: w, H: row.H}, name.String(), &on) {
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
	tint := mathx.White
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
		widgets.LabelDim(ui, i18n.Tr("launcher.no_recent_projects_yet", "No recent projects yet"))
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
			sub = i18n.Tr("launcher.missing", "missing, click to remove")
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
		if widgets.MenuItem(ui, i18n.Tr("launcher.open", "Open"), "", true, false) {
			l.Open(l.menu)
		}
		if l.opt.Reveal != nil && widgets.MenuItem(ui, l.opt.RevealLabel, "", true, false) {
			if err := l.opt.Reveal(filepath.Join(l.menu, project.FileName)); err != nil {
				l.err = err.Error()
			}
		}
		if widgets.MenuItem(ui, i18n.Tr("launcher.remove_from_list", "Remove from List"), "", true, false) {
			forget = l.menu
		}
		widgets.EndContextMenu(ui)
	}
	if forget != "" {
		l.forget(forget)
	}
	l.heading(ui, i18n.Tr("launcher.open_a_project", "Open a project"))
	row := ui.Cursor().Next(0)
	bw := float32(90)
	pw := float32(0)
	if l.opt.PickProject != nil {
		pw = ui.TextSize(i18n.Tr("launcher.browse", "Browse…")).X + 20
	}
	widgets.TextFieldRect(ui, ui.ID("open.dir"), mathx.Rect{X: row.X, Y: row.Y, W: max(0, row.W-bw-pw-8), H: row.H}, &l.open, true)
	if pw > 0 && widgets.ButtonRect(ui, ui.ID("open.pick"), mathx.Rect{X: row.X + row.W - bw - pw - 4, Y: row.Y, W: pw, H: row.H}, i18n.Tr("launcher.browse", "Browse…")) {
		start := l.open
		if start == "" {
			start = l.opt.Home
		}
		l.opt.PickProject(start, func(p string) {
			l.open = p
			l.Drop(p)
		})
	}
	if widgets.ButtonRect(ui, ui.ID("open.go"), mathx.Rect{X: row.X + row.W - bw, Y: row.Y, W: bw, H: row.H}, i18n.Tr("launcher.open", "Open")) {
		l.Drop(l.open)
	}
	widgets.LabelDim(ui, i18n.Tr("launcher.drop_hint", "Or drop a project folder, or its {file}, on this window.", i18n.Str("file", project.FileName)))
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
			widgets.LabelWrapped(ui, i18n.Tr("launcher.contains", "Contains: {files}", i18n.Str("files", strings.Join(t.Files, ", "))), true)
		}
	}
	l.heading(ui, i18n.Tr("launcher.project", "Project"))
	nameID := ui.ID("new.name")
	if l.autoName && ui.Focus() != nameID {
		l.name = nextProjectName(l.dir)
	}
	if changed, _ := widgets.TextFieldRect(ui, nameID, widgets.Field(ui, i18n.Tr("launcher.name", "Name")), &l.name, true); changed {
		l.autoName = false
	}
	previousDir := l.dir
	l.location(ui)
	if l.autoName && previousDir != l.dir {
		l.name = nextProjectName(l.dir)
	}
	mid := ui.ID("new.module")
	mr := widgets.Field(ui, i18n.Tr("launcher.module", "Module"))
	widgets.TextFieldRect(ui, mid, mr, &l.module, true)
	if l.module == "" && ui.Focus() != mid {
		ui.Text(project.ModuleName(strings.TrimSpace(l.name)), mathx.Vec2{X: mr.X + 4, Y: ui.TextY(mr)}, th.TextDisabled)
	}
	if len(l.opt.Workspaces) > 0 {
		names := make([]string, len(l.opt.Workspaces))
		for i, c := range l.opt.Workspaces {
			names[i] = c.Title
		}
		widgets.Combo(ui, ui.ID("new.workspace"), widgets.Field(ui, i18n.Tr("launcher.workspace", "Workspace")), &l.ws, names, false)
	}
	l.nativeRow(ui)
	target, targetErr := projectTarget(l.dir, l.name)
	widgets.LabelDim(ui, target)
	if targetErr != nil {
		row := ui.Cursor().Next(0)
		ui.Text(targetErr.Error(), mathx.Vec2{X: row.X, Y: ui.TextY(row)}, th.Error)
	}
	label := i18n.Tr("launcher.create", "Create")
	if targetErr != nil {
		row := ui.Cursor().Next(0)
		ui.Box(row, th.Radius.Button, th.Widget, th.Border)
		ui.TextIn(row, label, th.TextDisabled, core.AlignCenter)
	} else if widgets.Button(ui, label) {
		// Recheck on the click, since a folder may have appeared since the last frame.
		if target, err := projectTarget(l.dir, l.name); err == nil {
			l.Create(target, l.templateID(), strings.TrimSpace(l.module), "")
		} else {
			l.err = err.Error()
		}
	}
}

func (l *Launcher) nativeRow(ui *core.Context) {
	if l.opt.NativeHint == "" {
		widgets.Checkbox(ui, i18n.Tr("launcher.native_go_code", "Native Go code"), &l.native)
		return
	}
	row := ui.Cursor().Next(0)
	bw := float32(0)
	if l.opt.OpenURL != nil {
		bw = ui.TextSize(i18n.Tr("launcher.get_go", "Get Go")).X + 20
		if widgets.ButtonRect(ui, ui.ID("new.go"), mathx.Rect{X: row.X + row.W - bw, Y: row.Y, W: bw, H: row.H}, i18n.Tr("launcher.get_go", "Get Go")) {
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
	row := widgets.Field(ui, i18n.Tr("launcher.location", "Location"))
	if l.opt.PickFolder == nil {
		widgets.TextFieldRect(ui, ui.ID("new.dir"), row, &l.dir, true)
		return
	}
	bw := ui.TextSize(i18n.Tr("launcher.browse", "Browse…")).X + 20
	widgets.TextFieldRect(ui, ui.ID("new.dir"), mathx.Rect{X: row.X, Y: row.Y, W: row.W - bw - 4, H: row.H}, &l.dir, true)
	if widgets.ButtonRect(ui, ui.ID("new.pick"), mathx.Rect{X: row.X + row.W - bw, Y: row.Y, W: bw, H: row.H}, i18n.Tr("launcher.browse", "Browse…")) {
		l.opt.PickFolder(l.dir, func(p string) { l.dir = p })
	}
}

func (l *Launcher) examplesTab(ui *core.Context) {
	// A protocol request must show its origin immediately, even when many
	// installed example cards would otherwise push the confirmation offscreen.
	if l.remote.fromLink && (l.remote.pending != nil || l.remote.job != nil) {
		l.remoteTab(ui)
		return
	}
	if len(l.opt.Examples) == 0 {
		widgets.LabelWrapped(ui, i18n.Tr("launcher.examples_come_from_the_engine", "Examples come from the engine source tree; set AEGO_ROOT to its folder."), true)
	} else {
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
		l.heading(ui, i18n.Tr("launcher.copy_to", "Copy to"))
		l.location(ui)
		ex := l.opt.Examples[min(l.example, len(l.opt.Examples)-1)]
		target := filepath.Join(l.dir, ex)
		widgets.LabelDim(ui, target)
		if widgets.Button(ui, i18n.Tr("launcher.create_from", "Create from {example}", i18n.Str("example", ex))) {
			l.Create(target, "", strings.TrimSpace(l.module), ex)
		}
	}
	l.remoteTab(ui)
}
