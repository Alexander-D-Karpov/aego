package edit

import (
	"bufio"
	"bytes"
	"errors"
	"image"
	stddraw "image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"aego/app/projects"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/prefs"
	"aego/editor/core/sysopen"
	"aego/editor/shell"
	"aego/editor/shell/launcher"
	"aego/engine"
	"aego/engine/i18n"
	"aego/gpu"
	"aego/platform"
	"aego/render"
	renderhost "aego/render/host"
	"aego/render/icons"
	"aego/render/sprite"
	"aego/render/text"
	"aego/render/texture"
	"aego/render/view"
	"aego/ui/core"
	"aego/ui/draw"
	"aego/ui/theme"
	"aego/ui/widgets"
)

type LaunchSession struct {
	Engine *engine.Engine
	UI     *launcher.Launcher
	driver *launchDriver
}

type launchDriver struct {
	s        *LaunchSession
	eng      *engine.Engine
	renderer *render.Renderer
	ui       *core.Context
	list     *draw.List
	textOn   bool
	log      *log.ChannelLogger
	images   map[string]launchImage
	prefsDir string
	prefs    *prefs.Prefs
	retheme  bool
	updates  interface{ Tick(time.Time) }
}

type launchImage struct {
	reg texture.Region
	ok  bool
}

func CreateProject(dir, template, module, example string) (string, error) {
	return CreateProjectWith(launcher.CreateRequest{Dir: dir, Template: template, Module: module, Example: example})
}

func CreateProjectWith(req launcher.CreateRequest) (string, error) {
	opt := projects.Options{Dir: req.Dir, Template: req.Template, Module: req.Module, Example: req.Example, Native: req.Native}
	if req.Example != "" || req.Native {
		root, err := projects.EngineRoot("")
		if err != nil {
			return "", err
		}
		opt.Engine = root
	}
	if err := os.MkdirAll(filepath.Dir(req.Dir), 0o755); err != nil {
		return "", err
	}
	p, err := projects.Create(opt)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := projects.Verify(p, &out); err != nil {
		return p.Root, errors.New(err.Error() + "\n" + out.String())
	}
	return p.Root, nil
}

func documentsDir(home string) string {
	if runtime.GOOS == "linux" {
		cfg := os.Getenv("XDG_CONFIG_HOME")
		if cfg == "" {
			cfg = filepath.Join(home, ".config")
		}
		if f, err := os.Open(filepath.Join(cfg, "user-dirs.dirs")); err == nil {
			defer f.Close()
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "XDG_DOCUMENTS_DIR=")
				if ok {
					return strings.ReplaceAll(strings.Trim(v, `"`), "$HOME", home)
				}
			}
		}
	}
	return filepath.Join(home, "Documents")
}

func projectsHome() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	if docs := documentsDir(home); docs != "" {
		if st, err := os.Stat(docs); err == nil && st.IsDir() {
			return filepath.Join(docs, "AegoProjects")
		}
	}
	return filepath.Join(home, "AegoProjects")
}

func nativeHint() string {
	if _, err := exec.LookPath("go"); err != nil {
		return i18n.Tr("launcher.native.no_go", "Install Go to add native code; scripts need nothing else.")
	}
	if _, err := projects.EngineRoot(""); err != nil {
		return i18n.Tr("launcher.native.no_root", "Native code needs the engine sources: set AEGO_ROOT to an Aego checkout.")
	}
	return ""
}

// templateTexts are the launcher's names and descriptions of the templates; 2D, 2.5D and 3D are not translated.
var templateTexts = map[string][2]i18n.Text{
	projects.TemplateEmpty: {i18n.T("launcher.template.empty", "Empty"), i18n.T("launcher.template.empty_doc", "One scene with a 2D camera")},
	projects.Template2D:    {{}, i18n.T("launcher.template.2d_doc", "2D camera and a sprite")},
	projects.Template25D:   {{}, i18n.T("launcher.template.25d_doc", "Orthographic 3D camera with 2D sprites")},
	projects.Template3D:    {{}, i18n.T("launcher.template.3d_doc", "Perspective camera, a plane and a cube")},
}

func launcherTemplates() []launcher.Template {
	out := make([]launcher.Template, len(projects.Templates))
	for i, t := range projects.Templates {
		id := t.ID
		name, desc := t.Name, t.Description
		if tt, ok := templateTexts[id]; ok {
			if tt[0].Key != "" {
				name = tt[0].String()
			}
			if tt[1].Key != "" {
				desc = tt[1].String()
			}
		}
		out[i] = launcher.Template{
			ID: t.ID, Name: name, Description: desc, Icon: t.Icon,
			Files:   projects.TemplateFiles(id),
			Preview: func() ([]byte, error) { return projects.TemplatePreview(id) },
		}
	}
	return out
}

func launcherWorkspaces() []launcher.Choice {
	var out []launcher.Choice
	for _, p := range shell.BuiltinWorkspaces() {
		out = append(out, launcher.Choice{ID: p.ID, Title: p.Title})
	}
	return out
}

func OpenLauncher(cfg Config) (*LaunchSession, error) {
	lg := cfg.Logger
	if lg == nil {
		lg = log.New(log.NewTextSink(os.Stderr))
	}
	mod := renderhost.New(render.Options{MaxSprites: 4096, SpriteLimit: sprite.MaxSubmission})
	e, err := engine.NewBuilder(engine.Config{
		Title:    "Aego",
		Width:    1100,
		Height:   760,
		Headless: cfg.Headless,
		Hidden:   cfg.Hidden,
		Debug:    cfg.Debug,
		AppID:    "aego.launcher",
	}).WithLogger(lg).Use(mod).Build()
	if err != nil {
		return nil, err
	}
	var examples []string
	engineRoot, rootErr := projects.EngineRoot("")
	if rootErr == nil {
		examples = projects.Examples(engineRoot)
	}
	ls := &LaunchSession{Engine: e}
	d := &launchDriver{s: ls, renderer: mod.Renderer(), log: lg.With(shell.ChannelEditor), images: map[string]launchImage{}, prefsDir: prefsDir(cfg)}
	upd := updatesFor(d.prefsDir)
	if !cfg.Headless {
		d.updates = upd
	}
	d.prefs, _ = prefs.Load(d.prefsDir)
	if m := os.Getenv("AEGO_EDITOR_THEME"); m != "" {
		d.prefs.Theme = theme.ParseMode(m)
	}
	platform.Boot("launcher", "open")
	ls.UI = launcher.New(launcher.Options{
		Templates:  launcherTemplates(),
		Examples:   examples,
		Workspaces: launcherWorkspaces(),
		ExamplePreview: func(name string) ([]byte, error) {
			return projects.ExamplePreview(engineRoot, name)
		},
		CreateWith:  CreateProjectWith,
		NativeHint:  nativeHint(),
		Reveal:      sysopen.Reveal,
		RevealLabel: sysopen.RevealLabel(),
		OpenURL:     sysopen.Open,
		Updates:     upd,
		RestartToUpdate: func(install bool) error {
			change := upd.Restore
			if install {
				change = upd.Install
			}
			relaunch, err := change()
			if err != nil {
				return err
			}
			if !relaunch {
				e.Quit()
				return nil
			}
			if err := Relaunch("", TabRecent); err != nil {
				return err
			}
			e.Quit()
			return nil
		},
		Docs:        func() { sysopen.Open("https://aego.akarpov.ru/") },
		RecentPath:  cfg.Recent,
		Home:        projectsHome(),
		Image:       d.image,
		Remote:      launcherRemote{},
		Theme:       func() theme.Mode { return d.prefs.Theme },
		SetTheme:    d.setTheme,
		Language:    func() string { return d.prefs.Language },
		SetLanguage: d.setLanguage,
	})
	switch cfg.LauncherTab {
	case TabNew:
		ls.UI.SetTab(launcher.TabNew)
	case TabRecent:
		ls.UI.SetTab(launcher.TabRecent)
	}
	if cfg.ExampleLink != "" {
		if err := ls.UI.RequestExampleLink(cfg.ExampleLink); err != nil {
			ls.UI.Close()
			e.Close()
			return nil, err
		}
	}
	ls.driver = d
	return ls, nil
}

func (d *launchDriver) image(key string, load func() ([]byte, error)) (texture.Region, bool) {
	if c, ok := d.images[key]; ok {
		return c.reg, c.ok
	}
	c := launchImage{}
	if data, err := load(); err == nil {
		if img, err := png.Decode(bytes.NewReader(data)); err == nil {
			n, ok := img.(*image.NRGBA)
			if !ok {
				n = image.NewNRGBA(img.Bounds())
				stddraw.Draw(n, n.Bounds(), img, img.Bounds().Min, stddraw.Src)
			}
			w, h := n.Bounds().Dx(), n.Bounds().Dy()
			tex, err := d.renderer.Textures().Upload(&texture.Data{Width: w, Height: h, Format: gpu.TexSRGBA8, Pixels: n.Pix, Premultiplied: true}, texture.UploadOptions{Label: "launcher." + filepath.Base(key)})
			if err == nil {
				c = launchImage{reg: texture.Whole(tex, w, h), ok: true}
			}
		}
	}
	d.images[key] = c
	return c.reg, c.ok
}

func (ls *LaunchSession) Step() bool {
	return ls.Engine.Step(ls.driver)
}

func (ls *LaunchSession) Close() {
	if ls.UI != nil {
		ls.UI.Close()
	}
	ls.Engine.Close()
}

func Launch(cfg Config) (string, error) {
	root, _, remote, err := launchWithEntry(cfg)
	if err == nil && remote {
		return "", errors.New("remote examples require the entry-aware editor launch path")
	}
	return root, err
}

func launchWithEntry(cfg Config) (root, entry string, remote bool, err error) {
	ls, err := OpenLauncher(cfg)
	if err != nil {
		return "", "", false, err
	}
	if cfg.Frames > 0 {
		for i := 0; i < cfg.Frames && ls.Step(); i++ {
		}
	} else if err := ls.Engine.Run(ls.driver); err != nil {
		ls.Close()
		return "", "", false, err
	}
	ls.Close()
	root, _ = ls.UI.Result()
	entry, remote = ls.UI.ResultEntry()
	return root, entry, remote, nil
}

func touchRecent(cfg Config, root, name string, lg *log.ChannelLogger) {
	if cfg.Recent == "" {
		return
	}
	if err := launcher.Touch(cfg.Recent, root, name, time.Now()); err != nil {
		lg.Warn("cannot update the recent projects list", log.F("err", err.Error()))
	}
}

func (d *launchDriver) Init(e *engine.Engine) error {
	d.eng = e
	r := d.renderer
	ui := core.New(r.UIFont())
	ui.Theme = theme.For(d.prefs.Theme, d.systemDark())
	d.applyLanguage()
	if set, err := text.LoadSet(e.VFS(), r.Textures(), "engine://fonts", "ui", []int{12, 14, 16, 20}, []int{1, 2}); err == nil {
		ui.SetFonts(set)
	} else {
		d.log.Warn("ui fonts are missing, using the debug font", log.F("err", err.Error()))
	}
	if set, err := icons.Load(e.VFS(), r.Textures(), "engine://icons"); err == nil {
		ui.Icons = set
	}
	d.ui = ui
	win := e.Window()
	d.s.UI.SetPickers(sysopen.Picker(win, platform.DialogFolder, "", ""), sysopen.Picker(win, platform.DialogFolder, "", ""))
	return nil
}

func (d *launchDriver) Event(ev platform.Event) {
	switch ev.Kind {
	case platform.EvTextEditing:
		d.ui.SetComposition(ev.Str, int(ev.EditStart))
	case platform.EvTextInput:
		d.ui.SetComposition("", 0)
	case platform.EvDropFile:
		d.s.UI.Drop(ev.Str)
	case platform.EvDropText:
		if strings.HasPrefix(strings.ToLower(ev.Str), "aego:") {
			d.s.UI.Drop(ev.Str)
		}
	case platform.EvSystemTheme:
		if d.prefs.Theme == theme.ModeSystem {
			d.retheme = true
		}
	}
}

func (d *launchDriver) systemDark() bool {
	if st, ok := d.eng.Window().(platform.SystemThemer); ok {
		return st.SystemTheme() != platform.ThemeLight
	}
	return true
}

func (d *launchDriver) setTheme(m theme.Mode) {
	if d.prefs.Theme == m {
		return
	}
	d.prefs.Theme = m
	d.retheme = true
	d.prefs.Updates = updatesFor(d.prefsDir).Policy()
	if err := prefs.Save(d.prefsDir, d.prefs); err != nil {
		d.log.Warn("cannot save the editor preferences", log.F("err", err.Error()))
	}
}

func (d *launchDriver) applyLanguage() {
	widgets.Tr = i18n.Lookup
	var locales func() []string
	if l, ok := d.eng.Window().(platform.Localer); ok && !d.eng.Config().Headless {
		locales = l.PreferredLocales
	}
	i18n.Apply(d.prefs.Language, locales)
}

func (d *launchDriver) setLanguage(pref string) {
	if d.prefs.Language == pref {
		return
	}
	d.prefs.Language = pref
	d.applyLanguage()
	d.prefs.Updates = updatesFor(d.prefsDir).Policy()
	if err := prefs.Save(d.prefsDir, d.prefs); err != nil {
		d.log.Warn("cannot save the editor preferences", log.F("err", err.Error()))
	}
}

// prefsDir is the config directory that holds the recent projects list and prefs.aetext (D345).
func prefsDir(cfg Config) string {
	if cfg.Recent == "" {
		return ""
	}
	return filepath.Dir(cfg.Recent)
}

func (d *launchDriver) FixedUpdate(float64) {}

func (d *launchDriver) automaticUpdatesAllowed() bool {
	return d.s != nil && d.s.UI != nil && !d.s.UI.AwaitingExampleConfirmation()
}

func (d *launchDriver) Update(float64) {
	e, ui := d.eng, d.ui
	ui.Time = e.Platform().Now()
	if d.updates != nil && d.automaticUpdatesAllowed() {
		d.updates.Tick(time.Now())
	}
	if d.retheme {
		d.retheme = false
		ui.Theme = theme.For(d.prefs.Theme, d.systemDark())
	}
	m := e.Metrics()
	size := mathx.Vec2{X: float32(m.LogicalW), Y: float32(m.LogicalH)}
	ui.Begin(e.Input(), size, m.Density())
	d.s.UI.Draw(ui, mathx.Rect{W: size.X, H: size.Y})
	d.list = ui.End()
	d.applyPlatform()
	if _, ok := d.s.UI.Result(); ok {
		e.Quit()
	}
}

func (d *launchDriver) applyPlatform() {
	win := d.eng.Window()
	if cs, ok := win.(platform.Cursorer); ok {
		cs.SetCursor(platform.CursorShape(d.ui.CursorShape()))
	}
	ti, ok := win.(platform.TextInputer)
	if !ok {
		return
	}
	wants, r, cursor := d.ui.WantsTextInput()
	switch {
	case wants && !d.textOn:
		ti.StartTextInput()
		d.textOn = true
	case !wants && d.textOn:
		ti.StopTextInput()
		d.textOn = false
		d.ui.SetComposition("", 0)
	}
	if wants {
		ti.SetTextInputArea(r, cursor)
	}
}

func (d *launchDriver) Render(float64) {
	if d.list == nil || d.list.Empty() {
		return
	}
	m := d.eng.Metrics()
	v := view.ScreenView(m.PixelW, m.PixelH, m.Density())
	v.Clear = d.ui.Theme.Background
	v.DoClear = true
	v.Label = "launcher.ui"
	white := d.renderer.White()
	if f := d.ui.Font(); f != nil && !f.White.IsZero() {
		white = f.White
	}
	f := d.renderer.View(v)
	d.list.Flush(f, d.ui.Font(), white, 0)
	f.End()
}

func (d *launchDriver) Shutdown() {}
