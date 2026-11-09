package gpupreview

import (
	"fmt"
	"math"
	"slices"

	"aego/art"
	"aego/asset"
	"aego/asset/loader"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/gpu"
	"aego/modules/particles"
	"aego/modules/render2d"
	"aego/platform"
	"aego/render"
	renderhost "aego/render/host"
	"aego/render/lighting"
	"aego/render/material"
	"aego/render/postfx"
	"aego/render/shader"
	"aego/scene"
	"aego/scene/frame"
	"aego/schema"
)

// Options affect only this isolated presentation world, never the art document
// or an editor's EditScene. Frame selection uses the same Art Core IDs.
type Options struct {
	EffectTemplate string   `json:"effectTemplate,omitempty"`
	RateOverride   *float32 `json:"rateOverride,omitempty"`
	Paused         bool     `json:"paused"`
	Mode           string   `json:"mode"`
	Frame          art.ID   `json:"frame"`
	FlipX          bool     `json:"flipX"`
	FlipY          bool     `json:"flipY"`
	Bloom          bool     `json:"bloom"`
	Shadows        bool     `json:"shadows"`
	LightX         float32  `json:"lightX"`
	LightY         float32  `json:"lightY"`
	Intensity      float32  `json:"intensity"`
	Emission       float32  `json:"emission"`
	Amount         float32  `json:"amount"`
	Rate           float32  `json:"rate"`
	Speed          float32  `json:"speed"`
	Lifetime       float32  `json:"lifetime"`
}

func DefaultOptions() Options {
	return Options{Mode: "sprite", LightX: -.7, LightY: -.65, Intensity: 2.2, Emission: 3, Amount: .5, Rate: 16, Speed: 30, Lifetime: 1.4}
}

type Status struct {
	EffectTemplate  string       `json:"effectTemplate,omitempty"`
	ParticlePreset  string       `json:"particlePreset,omitempty"`
	ParticleRate    float32      `json:"particleRate"`
	ParticleEmitted uint64       `json:"particleEmitted"`
	State           string       `json:"state"`
	Error           string       `json:"error,omitempty"`
	Mode            string       `json:"mode"`
	Document        string       `json:"document"`
	Revision        uint64       `json:"revision"`
	Sequence        uint64       `json:"sequence"`
	Frame           art.ID       `json:"frame"`
	Particles       int          `json:"particles"`
	Renderer        string       `json:"renderer"`
	Stats           render.Stats `json:"stats"`
	Frames          int          `json:"frames"`
	HasNormal       bool         `json:"hasNormal"`
	HasHeight       bool         `json:"hasHeight"`
	HasEmission     bool         `json:"hasEmission"`
}

type App struct {
	e                                                  *engine.Engine
	r                                                  *render.Renderer
	loader                                             *loader.Loader
	types                                              *scene.Types
	man                                                *module.Manifest
	world                                              *engine.World
	sc                                                 *scene.Scene
	runner                                             *frame.Runner
	detach                                             func()
	sprite, camera, light, environment, floor, emitter scene.NodeHandle
	data                                               *prepared
	options                                            Options
	presets                                            map[string]particles.ParticleSystem2D
	particleDocument, particleTemplate                 string
	particleExtent                                     float32
	status                                             Status
	closed                                             bool
}

// New builds only the base, render2d and particles modules. It excludes the
// desktop shell, file importers, networking, scripting and scene-editor UI.
func New(headless bool) (*App, error) {
	a := &App{options: DefaultOptions(), status: Status{State: "empty"}, presets: make(map[string]particles.ParticleSystem2D)}
	var err error
	a.types, err = base.NewTypes()
	if err != nil {
		return nil, err
	}
	a.man, err = module.Load(a.types, nil, base.Core{}, render2d.New(), particles.New())
	if err != nil {
		return nil, err
	}
	if err = a.types.Schema().Freeze(); err != nil {
		return nil, err
	}
	rmod := renderhost.New(render.Options{MaxSprites: 2048})
	b := engine.NewBuilder(engine.Config{Title: "Asset Studio Preview", Width: 480, Height: 320,
		Headless: headless, HeadlessFrameTime: 1.0 / 60, AppID: "aego-art-preview", Seed: 1,
		VSync: platform.VSyncOn, LoopScope: engine.ScopeEditor}).WithLogger(log.Nop()).Use(rmod)
	if err = frame.Register(b.Registry()); err != nil {
		return nil, err
	}
	b.Use(a.man.Engine())
	a.e, err = b.Build()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			a.Close()
		}
	}()
	a.r = rmod.Renderer()
	a.loader = loader.New(a.e.GPU(), a.r.Textures(), a.e.Log())
	a.loader.SetMaterials(a.r.Materials())
	if err = a.loader.Mount(asset.NewIndex(), a.e.VFS()); err != nil {
		return nil, err
	}
	a.e.SetService(loader.ServiceID, a.loader)
	if err = a.man.Init(&module.InitContext{Engine: a.e, Assets: a.loader, Log: a.e.Log(), Editor: true}); err != nil {
		return nil, err
	}
	a.world = engine.NewWorld("asset-studio-preview")
	a.sc = scene.New(a.types, a.loader, scene.Options{InitialNodes: 16, InitialComponents: 16})
	a.runner = frame.New()
	a.runner.Bind(a.sc, a.r, a.loader)
	a.world.SetService(scene.ServiceID, a.sc)
	a.world.SetService(frame.ServiceID, a.runner)
	a.camera = a.sc.Create(a.sc.Root(), "Preview Camera")
	*scene.Add[render2d.Camera2D](a.sc, a.camera) = render2d.DefaultCamera2D()
	post := scene.Add[render2d.PostProcess](a.sc, a.camera)
	bloom, _ := postfx.NewRegistry().NewInstance(postfx.BloomID, "preview-bloom")
	bloom.Parameters = []postfx.Parameter{{Name: "Threshold", Kind: shader.KindFloat, Value: mathx.Vec4{X: .65}}, {Name: "Intensity", Kind: shader.KindFloat, Value: mathx.Vec4{X: .7}}}
	post.Effects = []postfx.Instance{bloom}
	a.sprite = a.sc.Create(a.sc.Root(), "Artwork")
	*scene.Add[render2d.SpriteRenderer](a.sc, a.sprite) = render2d.DefaultSpriteRenderer()
	oc := scene.Add[render2d.LightOccluder2D](a.sc, a.sprite)
	*oc = render2d.DefaultLightOccluder2D()
	oc.Source = render2d.OccluderSpriteAlpha
	a.floor = a.sc.Create(a.sc.Root(), "Light receiver")
	floor := scene.Add[render2d.SpriteRenderer](a.sc, a.floor)
	*floor = render2d.DefaultSpriteRenderer()
	floor.Order = -1
	floor.Tint = mathx.Color{R: .22, G: .25, B: .3, A: 1}
	floor.Material.ID, _ = asset.BuiltinMaterialNamed("Lit")
	a.light = a.sc.Create(a.sc.Root(), "Preview Light")
	*scene.Add[render2d.Light2D](a.sc, a.light) = render2d.DefaultLight2D()
	a.environment = a.sc.Create(a.sc.Root(), "Preview Lighting")
	*scene.Add[render2d.LightingEnvironment2D](a.sc, a.environment) = render2d.DefaultLightingEnvironment2D()
	a.emitter = a.sc.Create(a.sc.Root(), "Preview Particles")
	*scene.Add[particles.ParticleSystem2D](a.sc, a.emitter) = particles.DefaultParticleSystem2D()
	a.sc.Flush()
	a.detach, err = a.man.Attach(&module.AttachContext{World: a.world, Scene: a.sc, Log: a.e.Log(), Editor: true, Play: true})
	if err != nil {
		return nil, err
	}
	a.r.Materials().OnError = func(_ *material.Shader, _ int, failure error) {
		a.status.Error = failure.Error()
		a.status.State = "error"
	}
	return a, nil
}

func (a *App) Init(*engine.Engine) error { return nil }
func (a *App) Event(platform.Event)      {}
func (a *App) FixedUpdate(float64)       {}
func (a *App) Update(dt float64) {
	if a.closed || a.data == nil || a.options.Paused {
		return
	}
	a.e.TickWorld(a.world, engine.ScopePlay, math.Min(dt, .1))
}
func (a *App) Render(float64) {
	if a.closed || a.data == nil || a.options.Paused {
		return
	}
	if d, ok := a.e.GPU().(interface{ ContextLost() bool }); ok && d.ContextLost() {
		a.status.State = "context-lost"
		return
	}
	m := a.e.Metrics()
	cam, _ := scene.Get[render2d.Camera2D](a.sc, a.camera)
	extent := float32(max(a.data.info.Canvas.Width, a.data.info.Canvas.Height)) * 1.55
	if a.options.Mode == "particles" {
		extent *= 2.2
		if a.particleTemplate != "" {
			extent = a.particleExtent
		}
	}
	cam.Zoom = float32(min(m.PixelW, m.PixelH)) / max(extent, 1)
	a.e.RunPhase(a.world, engine.PhaseRenderPrepare, engine.ScopePlay, 0, 0)
	a.e.RunPhase(a.world, engine.PhaseRender, engine.ScopePlay, 0, 0)
	a.status.State = "ready"
	if a.status.Error != "" {
		a.status.State = "error"
	}
}
func (a *App) Shutdown()  { a.release() }
func (a *App) Run() error { return a.e.Run(a) }
func (a *App) Step() bool { return a.e.Step(a) }

func (a *App) Publish(sequence, revision uint64, data []byte) error {
	if a.closed {
		return fmt.Errorf("preview is closed")
	}
	if sequence == 0 || sequence <= a.status.Sequence {
		return art.ErrConflict
	}
	next, err := prepare(data)
	if err != nil {
		return err
	}
	if next.info.Revision != revision {
		return art.ErrConflict
	}
	// Reject a bad template before publishing textures or replacing the previous
	// document. Templates are trusted catalog IDs, never paths or arbitrary code.
	if id := next.info.Properties[art.EffectTemplateProperty]; id != "" {
		if _, _, err = a.effectPreset(id); err != nil {
			return err
		}
	}
	// Preserve unchanged resident pages and shader/material generations. Only
	// changed prepared bytes enter the existing atomic loader publication path.
	previous := map[ids.AssetID]asset.Entry{}
	if a.data != nil {
		for _, entry := range a.data.entries {
			previous[entry.ID] = entry
		}
	}
	var changed []asset.Entry
	for _, entry := range next.entries {
		old, ok := previous[entry.ID]
		if !ok || old.Kind != entry.Kind || !slices.Equal(old.Artifacts, entry.Artifacts) || !slices.Equal(old.Deps, entry.Deps) {
			changed = append(changed, entry)
		}
	}
	if err = a.loader.Swap(changed, next.files); err != nil {
		return err
	}
	if old := a.data; old != nil {
		keep := make(map[ids.AssetID]bool, len(next.entries))
		for _, e := range next.entries {
			keep[e.ID] = true
		}
		for _, e := range old.entries {
			if !keep[e.ID] {
				a.loader.Drop(e.ID)
			}
		}
	}
	if a.data == nil || a.data.info.ID != next.info.ID {
		// A document's own template wins when switching tabs. Presentation rate
		// overrides belong to the previous document and must not leak across tabs.
		a.options.EffectTemplate = ""
		a.options.RateOverride = nil
	}
	a.data = next
	a.status.Document = next.info.ID
	a.status.Revision = revision
	a.status.Sequence = sequence
	a.status.Frames = len(next.info.Frames)
	a.status.Error = ""
	return a.Configure(a.options)
}

func (a *App) Configure(o Options) error {
	if a.closed {
		return fmt.Errorf("preview is closed")
	}
	switch o.Mode {
	case "sprite", "lit", "normal", "height", "emission", "flash", "dissolve", "particles":
	default:
		return fmt.Errorf("unsupported preview mode %q", o.Mode)
	}
	for _, v := range []float32{o.LightX, o.LightY, o.Intensity, o.Emission, o.Amount, o.Rate, o.Speed, o.Lifetime} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return fmt.Errorf("preview parameters must be finite")
		}
	}
	if o.Intensity < 0 || o.Intensity > 8 || o.Emission < 0 || o.Emission > 16 || o.Amount < 0 || o.Amount > 1 || o.Rate < 0 || o.Rate > 256 || o.Speed < 0 || o.Speed > 256 || o.Lifetime <= 0 || o.Lifetime > 10 {
		return fmt.Errorf("preview parameter outside supported range")
	}
	if rate := o.RateOverride; rate != nil && (math.IsNaN(float64(*rate)) || math.IsInf(float64(*rate), 0) || *rate < 0 || *rate > 256) {
		return fmt.Errorf("preview particle rate outside supported range")
	}
	templateID := o.EffectTemplate
	if templateID == "" && a.data != nil {
		templateID = a.data.info.Properties[art.EffectTemplateProperty]
	}
	var preset particles.ParticleSystem2D
	var presetName string
	if templateID != "" {
		var err error
		preset, presetName, err = a.effectPreset(templateID)
		if err != nil {
			return err
		}
	}
	restart := a.options.Mode != "particles" || (a.options.Paused && !o.Paused)
	clearRateOverride := a.options.RateOverride != nil && o.RateOverride == nil
	a.options = o
	a.status.Mode = o.Mode
	a.status.Error = ""
	if a.data == nil {
		return nil
	}
	p := a.data
	f := o.Frame
	if _, ok := p.frames[f]; !ok {
		f = p.info.Frames[0].ID
	}
	a.status.Frame = f
	refs := p.frames[f]
	a.status.HasNormal = !refs[art.Normal].IsZero()
	a.status.HasHeight = !refs[art.Height].IsZero()
	a.status.HasEmission = !refs[art.Emission].IsZero()
	sr, _ := scene.Get[render2d.SpriteRenderer](a.sc, a.sprite)
	// Presentation selections do not republish assets or bump loader generation.
	// Use the common setter so a previous frame/channel region cannot stay cached.
	sr.Sprite.SetAssetRefID(refs[art.Albedo])
	sr.FlipX, sr.FlipY = o.FlipX, o.FlipY
	sr.Size = mathx.V2(float32(p.info.Canvas.Width), float32(p.info.Canvas.Height))
	sr.Origin = mathx.V2(.5, .5)
	if pivot, ok := p.info.FramePivot(f); ok {
		sr.Origin = mathx.V2(float32(pivot.X)/sr.Size.X, float32(pivot.Y)/sr.Size.Y)
	}
	mat := "Unlit"
	lit := o.Mode == "lit"
	if lit {
		mat = "Lit"
	}
	if o.Mode == "flash" {
		mat = "Flash"
	}
	if o.Mode == "dissolve" {
		mat = "Dissolve"
	}
	if o.Mode == "normal" || o.Mode == "height" || o.Mode == "emission" {
		id := refs[art.Channel(o.Mode)]
		if id.IsZero() {
			a.status.Error = "The selected frame has no " + o.Mode + " channel"
			a.sc.SetEnabled(a.sprite, false)
			a.sc.SetEnabled(a.emitter, false)
			a.sc.SetEnabled(a.floor, false)
			return nil
		}
		sr.Sprite.SetAssetRefID(id)
	}
	sr.Material.ID, _ = asset.BuiltinMaterialNamed(mat)
	// The Lit material is also used by the shadow receiver. Authoring preview
	// values belong to Artwork's instance; changing the shared material would
	// make the receiver emit too. Clear the private instance on other shaders,
	// whose schemas do not contain these parameters.
	if !lit {
		if overrides, ok := scene.Get[render2d.MaterialOverride](a.sc, a.sprite); ok {
			overrides.Values = overrides.Values[:0]
		}
	}
	if material, ok := a.loader.Material(sr.Material.ID); ok {
		if mat == "Flash" {
			material.Set("Flash", [4]float32{o.Amount})
		}
		if mat == "Dissolve" {
			material.Set("Threshold", [4]float32{o.Amount})
		}
		if mat == "Lit" {
			intensity := float32(0)
			if a.status.HasEmission {
				intensity = o.Emission
			}
			if err := render2d.SetMaterialParameter(a.sc, a.sprite, "EmissionColor", schema.Color(mathx.Color{R: 1, G: 1, B: 1, A: 1})); err != nil {
				return err
			}
			if err := render2d.SetMaterialParameter(a.sc, a.sprite, "EmissionIntensity", schema.Float32(intensity)); err != nil {
				return err
			}
		}
	}
	a.sc.SetEnabled(a.sprite, o.Mode != "particles")
	a.sc.SetEnabled(a.emitter, o.Mode == "particles")
	a.sc.SetEnabled(a.floor, lit && o.Shadows)
	floor, _ := scene.Get[render2d.SpriteRenderer](a.sc, a.floor)
	floor.Size = sr.Size.Scale(5)
	l, _ := scene.Get[render2d.Light2D](a.sc, a.light)
	l.Intensity = o.Intensity
	l.Range = float32(max(p.info.Canvas.Width, p.info.Canvas.Height)) * 3
	l.Height = l.Range * .4
	l.Shadows = o.Shadows
	l.ShadowCasterMask = -1
	a.sc.SetPosition(a.light, mathx.V2(o.LightX*sr.Size.X, o.LightY*sr.Size.Y))
	env, _ := scene.Get[render2d.LightingEnvironment2D](a.sc, a.environment)
	env.AmbientIntensity = .16
	env.ColorMode = lighting.Auto
	post, _ := scene.Get[render2d.PostProcess](a.sc, a.camera)
	post.Enabled = o.Bloom
	return a.configureParticles(templateID, presetName, preset, restart, clearRateOverride, sr.Size.X)
}

func (a *App) Status() Status {
	s := a.status
	if a.e != nil {
		s.Renderer = a.e.Caps().Renderer
	}
	if a.r != nil {
		s.Stats = a.r.Stats()
		if s.Stats.MaterialCompileFailures > 0 || s.Stats.LightingFailures > 0 || s.Stats.PostFXFailures > 0 {
			s.State = "error"
			if s.Error == "" {
				s.Error = "GPU preview reported a shader, lighting or PostFX failure"
			}
		}
	}
	if a.e != nil {
		if device, ok := a.e.GPU().(interface{ ContextLost() bool }); ok && device.ContextLost() {
			s.State = "context-lost"
		}
	}
	if a.world != nil {
		if p := particles.Of(a.world); p != nil {
			s.Particles = p.Stats().Alive
			s.ParticleEmitted = p.Emitted(a.emitter)
		}
	}
	return s
}
func (a *App) Resources() gpu.ResourceStats {
	if a.e != nil {
		if r, ok := a.e.GPU().(gpu.ResourceReporter); ok {
			return r.ResourceStats()
		}
	}
	return gpu.ResourceStats{}
}
func (a *App) ContextLoss(lose bool) bool {
	if a.e != nil {
		if d, ok := a.e.GPU().(interface{ SimulateLoss(bool) bool }); ok {
			return d.SimulateLoss(lose)
		}
	}
	return false
}
func (a *App) release() {
	if a.closed {
		return
	}
	a.closed = true
	if a.detach != nil {
		a.detach()
		a.detach = nil
	}
	if a.runner != nil {
		a.runner.Close()
		a.runner = nil
	}
	if a.loader != nil {
		a.loader.Close()
		a.loader = nil
	}
	if a.data != nil {
		a.data = nil
	}
	if a.man != nil {
		a.man.Shutdown()
	}
	a.status.State = "closed"
}
func (a *App) Close() {
	a.release()
	if a.e != nil {
		_ = a.e.Close()
	}
}
