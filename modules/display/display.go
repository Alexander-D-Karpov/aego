package display

import (
	"strconv"
	"strings"

	"aego/engine"
	"aego/engine/module"
	"aego/engine/settings"
	"aego/platform"
	"aego/scene/frame"
	"aego/schema"
)

const (
	ID             = "display"
	KeyMode        = ID + ".window.mode"
	KeyMonitor     = ID + ".window.monitor"
	KeyResolution  = ID + ".window.resolution"
	KeyVSync       = ID + ".vsync"
	KeyFrameLimit  = ID + ".frameLimit"
	KeyRenderScale = ID + ".renderScale"
	KeyUIScale     = ID + ".uiScale"
	KeyPreset      = ID + ".preset"
)

var Presets = []string{"Low", "Medium", "High", "Ultra"}

type Module struct {
	st       *settings.Store
	eng      *engine.Engine
	disp     platform.Displayer
	project  platform.DisplayMode
	mode     settings.Key
	monitor  settings.Key
	res      settings.Key
	vsync    settings.Key
	live     bool
	scaleKey settings.Key
}

func New() *Module {
	return &Module{}
}

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Builtin: true}
}

func enum(name, label string, v int) settings.Choice {
	return settings.Choice{Name: name, Label: label, Value: schema.Enum(int64(v))}
}

func (m *Module) Register(r *module.Registry) error {
	both := settings.User | settings.Project
	r.Setting(settings.Desc{
		ID: KeyMode, Kind: schema.KindEnum, Default: schema.Enum(int64(platform.Windowed)),
		Title: "Window Mode", Category: settings.CategoryDisplay, Order: 10, Flags: both | settings.Confirm,
		Choices: []settings.Choice{
			enum("windowed", "Windowed", int(platform.Windowed)),
			enum("borderless", "Borderless", int(platform.Borderless)),
			enum("fullscreen", "Fullscreen", int(platform.Fullscreen)),
		},
		Apply: m.applyWindow,
	})
	r.Setting(settings.Desc{
		ID: KeyMonitor, Kind: schema.KindInt64, Title: "Monitor", Category: settings.CategoryDisplay, Order: 20,
		Flags: both | settings.Confirm, ChoicesWhen: m.monitors, Apply: m.applyWindow,
	})
	r.Setting(settings.Desc{
		ID: KeyResolution, Kind: schema.KindString, Title: "Resolution", Category: settings.CategoryDisplay, Order: 30,
		Doc:   "Window size, or the screen mode in fullscreen. Empty uses the project window size.",
		Flags: both | settings.Confirm, ChoicesWhen: m.resolutions, EnabledWhen: m.sized, Supports: m.supports, Apply: m.applyWindow,
	})
	r.Setting(settings.Desc{
		ID: KeyVSync, Kind: schema.KindEnum, Default: schema.Enum(int64(platform.VSyncOn)), Title: "VSync",
		Category: settings.CategoryDisplay, Order: 40, Flags: both,
		Choices: []settings.Choice{
			enum("off", "Off", int(platform.VSyncOff)),
			enum("on", "On", int(platform.VSyncOn)),
			enum("adaptive", "Adaptive", int(platform.VSyncAdaptive)),
		},
		Apply: m.applyVSync,
	})
	r.Setting(settings.Desc{
		ID: KeyFrameLimit, Kind: schema.KindInt64, Title: "Frame Limit", Unit: "fps", Category: settings.CategoryDisplay, Order: 50,
		Doc:   "Caps the frame rate while VSync is off. Zero means no limit.",
		Flags: both, Min: 0, Max: 1000, HasMin: true, HasMax: true, Step: 1,
		EnabledWhen: func(s *settings.Store) bool { return s.Int(m.vsync) == int64(platform.VSyncOff) },
		Apply: func(v schema.Value) error {
			if m.eng != nil {
				m.eng.SetFrameLimit(float64(v.I))
			}
			return nil
		},
	})
	r.Setting(settings.Desc{
		ID: KeyRenderScale, Kind: schema.KindFloat64, Default: schema.Float64(1), Title: "Render Scale",
		Category: settings.CategoryGraphics, Order: 20, Flags: both, Min: 0.5, Max: 2, HasMin: true, HasMax: true, Step: 0.05,
	})
	r.Setting(settings.Desc{
		ID: KeyUIScale, Kind: schema.KindFloat64, Default: schema.Float64(1), Title: "UI Scale",
		Category: settings.CategoryDisplay, Order: 60, Flags: both | settings.Accessibility,
		Min: 0.5, Max: 2, HasMin: true, HasMax: true, Step: 0.05,
	})
	choices := make([]settings.Choice, 0, len(Presets)+1)
	for i, p := range Presets {
		choices = append(choices, enum(p, p, i))
	}
	choices = append(choices, enum(settings.PresetCustom, settings.PresetCustom, len(Presets)))
	r.Setting(settings.Desc{
		ID: KeyPreset, Kind: schema.KindEnum, Default: schema.Enum(2), Title: "Quality Preset",
		Category: settings.CategoryGraphics, Order: 10, Flags: both, Preset: true, Choices: choices,
	})
	r.System(engine.SystemDesc{
		ID: SystemScale, Phase: engine.PhaseRenderPrepare, Scope: engine.ScopePlay,
		Before: []string{frame.SystemTransform}, System: scaleSystem{m: m},
	})
	return nil
}

func (m *Module) renderScale() float32 {
	if !m.live || m.st == nil {
		return 1
	}
	k := float32(m.st.Float(m.scaleKey))
	if k <= 0 {
		return 1
	}
	return min(max(k, 0.25), 2)
}

func (m *Module) Init(ctx *module.InitContext) error {
	m.st = ctx.Settings
	if m.st == nil {
		return nil
	}
	m.mode, m.monitor = m.st.Key(KeyMode), m.st.Key(KeyMonitor)
	m.res, m.vsync = m.st.Key(KeyResolution), m.st.Key(KeyVSync)
	m.scaleKey = m.st.Key(KeyRenderScale)
	if ctx.Engine == nil || ctx.Editor {
		return nil
	}
	m.eng = ctx.Engine
	m.live = true
	m.disp, _ = ctx.Engine.Window().(platform.Displayer)
	cfg := ctx.Engine.Config()
	m.project = platform.DisplayMode{W: cfg.Width, H: cfg.Height}
	return nil
}

func ParseResolution(s string) (int, int, bool) {
	a, b, ok := strings.Cut(s, "x")
	if !ok {
		return 0, 0, false
	}
	w, err1 := strconv.Atoi(a)
	h, err2 := strconv.Atoi(b)
	return w, h, err1 == nil && err2 == nil && w > 0 && h > 0
}

func FormatResolution(w, h int) string {
	return strconv.Itoa(w) + "x" + strconv.Itoa(h)
}

func (m *Module) display(s *settings.Store) (platform.Display, bool) {
	if m.disp == nil {
		return platform.Display{}, false
	}
	ds := m.disp.Displays()
	i := int(s.Int(m.monitor))
	if i < 0 || i >= len(ds) {
		return platform.Display{}, false
	}
	return ds[i], true
}

func (m *Module) monitors(*settings.Store) []settings.Choice {
	if m.disp == nil {
		return nil
	}
	ds := m.disp.Displays()
	out := make([]settings.Choice, len(ds))
	for i, d := range ds {
		label := d.Name
		if label == "" {
			label = "Monitor " + strconv.Itoa(i+1)
		}
		out[i] = settings.Choice{Name: "monitor" + strconv.Itoa(i), Label: label, Value: schema.Int64(int64(i))}
	}
	return out
}

func (m *Module) resolutions(s *settings.Store) []settings.Choice {
	d, ok := m.display(s)
	if !ok {
		return nil
	}
	out := []settings.Choice{{Name: "project", Label: "Project Default", Value: schema.String("")}}
	seen := map[string]bool{}
	for _, md := range d.Modes {
		r := FormatResolution(md.W, md.H)
		if !seen[r] {
			seen[r] = true
			out = append(out, settings.Choice{Name: r, Label: r, Value: schema.String(r)})
		}
	}
	return out
}

func (m *Module) sized(s *settings.Store) bool {
	return s.Int(m.mode) != int64(platform.Borderless)
}

func (m *Module) supports(v schema.Value) bool {
	if v.S == "" || m.st == nil {
		return true
	}
	w, h, ok := ParseResolution(v.S)
	if !ok {
		return false
	}
	d, ok := m.display(m.st)
	if !ok || platform.WindowMode(m.st.Int(m.mode)) != platform.Fullscreen {
		return true
	}
	return d.Has(platform.DisplayMode{W: w, H: h})
}

func (m *Module) applyWindow(schema.Value) error {
	if m.disp == nil || m.st == nil {
		return nil
	}
	size := m.project
	if w, h, ok := ParseResolution(m.st.Text(m.res)); ok {
		size = platform.DisplayMode{W: w, H: h}
	}
	mode := platform.WindowMode(m.st.Int(m.mode))
	if mode == platform.Fullscreen && m.st.Text(m.res) == "" {
		size = platform.DisplayMode{}
	}
	return m.disp.SetWindowMode(mode, int(m.st.Int(m.monitor)), size)
}

func (m *Module) applyVSync(v schema.Value) error {
	if m.eng != nil {
		m.eng.SetVSync(platform.VSyncMode(v.I))
	}
	return nil
}
