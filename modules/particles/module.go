package particles

import (
	"aego/asset"
	"aego/engine"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/engine/settings"

	"aego/scene/frame"
	"aego/schema"
	"aego/script/bind"
)

const (
	ID             = "particles"
	ServiceID      = ID + ".service"
	NameSystem     = ID + ".ParticleSystem2D"
	SystemSimulate = ID + ".simulate"
	SystemExtract  = ID + ".extract"
	ScriptModule   = "aego/particles"
	KeyEnabled     = ID + ".enabled"
)

type Module struct {
	problems  *problems.Service
	completed module.Signal0
	store     *settings.Store
}

func New() *Module { return &Module{} }

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Requires: []string{"render2d"}, Builtin: true,
		DisplayName: "Particles", DocsOrder: 58, DocsIcon: "cat.particles", Codes: []string{"PRT001", "PRT002"}}
}

func (*Module) IconDir() string { return "modules/particles/res/icons/src" }

func (m *Module) Register(r *module.Registry) error {
	if err := module.Enum[Shape](r, ID+".Shape", schema.EnumValue{Name: "point", Value: int64(Point)}, schema.EnumValue{Name: "circle", Value: int64(Circle)}, schema.EnumValue{Name: "rect", Value: int64(Rect)}, schema.EnumValue{Name: "edge", Value: int64(Edge)}, schema.EnumValue{Name: "spriteMask", Value: int64(SpriteMask)}); err != nil {
		return err
	}
	if err := module.Enum[Space](r, ID+".Space", schema.EnumValue{Name: "local", Value: int64(Local)}, schema.EnumValue{Name: "world", Value: int64(World)}); err != nil {
		return err
	}
	if err := module.Enum[TimeDomain](r, ID+".TimeDomain", schema.EnumValue{Name: "game", Value: int64(Game)}, schema.EnumValue{Name: "unscaled", Value: int64(Unscaled)}); err != nil {
		return err
	}
	if err := module.Enum[SortMode](r, ID+".SortMode", schema.EnumValue{Name: "age", Value: int64(Age)}, schema.EnumValue{Name: "youngestFirst", Value: int64(YoungestFirst)}, schema.EnumValue{Name: "distance", Value: int64(Distance)}); err != nil {
		return err
	}
	if err := module.Enum[ReducedMotionMode](r, ID+".ReducedMotionMode", schema.EnumValue{Name: "keep", Value: int64(Keep)}, schema.EnumValue{Name: "reduce", Value: int64(Reduce)}, schema.EnumValue{Name: "hide", Value: int64(Hide)}); err != nil {
		return err
	}
	if err := module.Struct[ColorKey](r, schema.Name(ID+".ColorKey"), schema.Doc("A normalized age and its straight linear color."), schema.FieldDoc("time", "Normalized age from zero to one."), schema.FieldDoc("color", "Straight linear color, including opacity.")); err != nil {
		return err
	}
	b := module.Component[ParticleSystem2D](r, schema.Name(NameSystem), schema.Space(schema.Space2D), schema.FullClone(cloneParticleSystem),
		schema.Doc("Emits visual sprites from a bounded CPU pool. World-space particles stay at their birth location when the emitter moves."),
		schema.FieldDoc("playOnStart", "Starts the emission schedule when this emitter becomes active."),
		schema.FieldDoc("rate", "Continuous births per second. Zero allows manual bursts without a continuous stream."),
		schema.FieldDoc("lifetime", "Minimum and maximum particle lifetime in seconds."),
		schema.FieldDoc("maxParticles", "Maximum live particles. When full, keep living particles and drop excess births."),
		schema.FieldDoc("bursts", "Cycle keys as (time in seconds, birth count). Times must be in [0, duration)."),
		schema.FieldDoc("space", "Local particles follow the emitter; world particles keep their birth transform."),
		schema.FieldDoc("spread", "Full angular width around direction, in radians."),
		schema.FieldDoc("sprite", "Texture region to draw. Empty uses the built-in white texture."),
		schema.FieldDoc("material", "Optional unlit sprite-domain material. Its shader controls the blend mode."),
		schema.FieldDoc("size", "Minimum and maximum starting side length in world units."),
		schema.FieldDoc("seed", "Nonzero locks the seed. Zero derives it from the session, stable node ID and activation."),
		schema.FieldDoc("velocityOverLife", "Speed multiplier by normalized age, applied without compounding."),
		schema.FieldDoc("colorOverLife", "Ordered keys for the straight color multiplier at normalized ages."),
		schema.FieldDoc("planeDistance", "Plane offset along its normalized normal."),
		schema.FieldDoc("planeNormal", "Outward normal of the clipping plane in simulation space."),
		schema.FieldDoc("planeEnabled", "Remove particles past the configured clipping plane."),
		schema.FieldDoc("killY", "Threshold in simulation space, in world units."),
		schema.FieldDoc("killYEnabled", "Remove particles on the side of increasing Y past Kill Y."),
		schema.FieldDoc("noiseFrequency", "How fast the noise field changes."),
		schema.FieldDoc("noiseStrength", "Magnitude of deterministic presentation noise."),
		schema.FieldDoc("drag", "Nonnegative damping coefficient per second."),
		schema.FieldDoc("reducedRate", "Fraction of births kept under the Reduce policy."),
		schema.FieldDoc("reducedMotion", "Keep, reduce or hide this effect when Reduce Motion is enabled."),
		schema.FieldDoc("sortMode", "Oldest first, youngest first or farthest first for each camera."),
		schema.FieldDoc("frameLoop", "Repeat the flipbook; otherwise hold its last frame."),
		schema.FieldDoc("frameFPS", "Flipbook playback speed in frames per second."),
		schema.FieldDoc("frames", "Ordered sprite frame references for a flipbook. Missing frames are diagnosed."),
		schema.FieldDoc("maskThreshold", "Minimum source alpha used by the sprite mask, from zero to one."),
		schema.FieldDoc("mask", "Source sprite frame. Its prepared alpha mask is independent of atlas packing."),
		schema.FieldDoc("edgeB", "Second endpoint in emitter coordinates."),
		schema.FieldDoc("edgeA", "First endpoint in emitter coordinates."),
		schema.FieldDoc("prewarm", "Seconds of bounded preparation for a looping effect, up to two."),
		schema.FieldDoc("order", "Painter order within the layer."),
		schema.FieldDoc("layer", "Painter layer shared with SpriteRenderer and Mesh2D."),
		schema.FieldDoc("rotationSpeed", "Angular speed in radians per second."),
		schema.FieldDoc("alphaOverLife", "Opacity multiplier sampled at normalized particle age."),
		schema.FieldDoc("sizeOverLife", "Size multiplier sampled at normalized particle age."),
		schema.FieldDoc("gravity", "Acceleration in units per second squared."),
		schema.FieldDoc("color", "Straight linear tint and opacity at birth."),
		schema.FieldDoc("direction", "Central angle of launch in radians."),
		schema.FieldDoc("rotation", "Minimum and maximum initial rotation in radians."),
		schema.FieldDoc("speed", "Minimum and maximum initial speed in units per second."),
		schema.FieldDoc("radius", "Circle radius in world units."),
		schema.FieldDoc("rectSize", "Full rectangle dimensions in world units."),
		schema.FieldDoc("offset", "Local offset of the emitter shape."),
		schema.FieldDoc("shape", "Birth positions come from a point, disk, rectangle, line segment or sprite alpha mask."),
		schema.FieldDoc("timeDomain", "Game pauses with the world; unscaled uses real presentation time."),
		schema.FieldDoc("duration", "Length of one emission cycle in seconds."),
		schema.FieldDoc("loop", "Repeats the schedule at the end of each cycle."),
	).Category("Effects").Display("Particle System 2D").Icon("particles.system").Default(DefaultParticleSystem2D).Validate(validate).Template("Particle System", nil)
	m.completed = b.Signal0("Completed", "A finite emission schedule has ended and its last particle expired. A presentation signal, not a gameplay timer.")
	registerMethods(b)
	registerPresets(r)
	registerDocs(r)
	if err := b.Err(); err != nil {
		return err
	}
	r.Setting(settings.Desc{ID: KeyEnabled, Kind: schema.KindBool, Default: schema.Bool(true), Title: "Particle Effects", Category: settings.CategoryAccessibility,
		Order: 15, Flags: settings.User | settings.Project | settings.Accessibility, Doc: "Show particle effects. Turning this off clears live particles without changing gameplay."})
	r.System(engine.SystemDesc{ID: SystemSimulate, Phase: engine.PhaseAnimation, Scope: engine.ScopePlay, Pause: engine.PauseAlways, System: simulate{}})
	r.System(engine.SystemDesc{ID: SystemExtract, Phase: engine.PhaseRender, Scope: engine.ScopeWorld, Pause: engine.PauseAlways, After: []string{"render2d.extract"}, Before: []string{frame.SystemSubmit}, System: extract{}})
	if r.Bindings() != nil {
		ns := r.ScriptModule(ScriptModule)
		bind.Proc1(ns, "SetEnabled", func(c *bind.Ctx, on bool) {
			if s := Of(c.World); s != nil {
				s.SetEnabled(on)
			}
		}, bind.Params("enabled"), bind.Doc("Turns particles on or off in this Play world. Turning them off clears current effects."), bind.Example("particles.SetEnabled(false)"))
		bind.Func0(ns, "Enabled", func(c *bind.Ctx) bool { s := Of(c.World); return s != nil && s.Enabled() }, bind.Doc("Reports whether this world presents particle effects."), bind.Example("visible := particles.Enabled()"))
	}
	return nil
}

func validate(ctx module.ValidateContext, c *ParticleSystem2D) []problems.Diagnostic {
	if err := c.Validate(); err != nil {
		return []problems.Diagnostic{{Code: "PRT001", Severity: problems.Error, Message: err.Error(), Help: "Correct the emitter settings, then restart the effect."}}
	}
	var out []problems.Diagnostic
	var res asset.Resolver
	if ctx.Scene != nil {
		res = ctx.Scene.Resolver()
	}
	if res == nil {
		return nil
	}
	points, err := resources(c, res)
	if err != nil {
		out = append(out, problems.Diagnostic{Code: "PRT002", Severity: problems.Error, BlocksPlay: true, Message: err.Error() + "; asset references are preserved.", Help: "Restore the missing source or use a prepared sprite-domain material and valid frame or mask."})
	} else if c.Shape == SpriteMask && len(points) == 0 {
		out = append(out, problems.Diagnostic{Code: "PRT002", Severity: problems.Warning, Message: "No source pixels pass the particle mask threshold.", Help: "Lower the threshold or choose a nonempty source frame."})
	}

	return out
}

func (m *Module) Init(ctx *module.InitContext) error {
	m.store, m.problems = ctx.Settings, ctx.Problems
	return nil
}

func (m *Module) AttachWorld(ctx *module.AttachContext) error {
	if !ctx.Play || ctx.World == nil || ctx.Scene == nil {
		return nil
	}
	if old := Of(ctx.World); old != nil {
		if old.scene == ctx.Scene {
			return nil
		}
		old.close()
	}
	s := newService(ctx.World, ctx.Scene, m)
	if m.store != nil {
		if key := m.store.Key(KeyEnabled); key != 0 {
			s.SetEnabled(m.store.Bool(key))
			reduced := m.store.Key("accessibility.reducedMotion")
			if reduced != 0 {
				s.reduced = m.store.Bool(reduced)
			}
			s.unsubscribeSetting = m.store.Subscribe(0, func(k settings.Key) {
				switch k {
				case key:
					s.SetEnabled(m.store.Bool(k))
				case reduced:
					s.reduced = m.store.Bool(k)
				}
			})
		}
	}
	s.prepareMaterials()
	ctx.World.SetService(ServiceID, s)
	return nil
}

func (*Module) DetachWorld(ctx *module.AttachContext) {
	if s := Of(ctx.World); s != nil {
		s.close()
		ctx.World.RemoveService(ServiceID)
	}
}
