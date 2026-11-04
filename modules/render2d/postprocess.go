package render2d

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"aego/core/mathx"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/render/postfx"
	"aego/render/shader"
	"aego/render/view"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
)

const (
	NamePostProcess    = "render2d.PostProcess"
	PostEffectsService = "render2d.postfx"
)

type PostProcess struct {
	Enabled bool              `aego:"enabled"`
	Effects []postfx.Instance `aego:"effects,noscript"`
}

// Authored 2D cameras contribute to the runner's default output. Runtime
// checks their actual OutputIDs again after native camera adjustments.
func authoredOutputOpaque(sc *scene.Scene, node scene.NodeHandle, owner *Camera2D) bool {
	if !owner.Active || !sc.EffectiveEnabled(node) || scene.Store[Camera2D](sc).Len() == 1 {
		return owner.Clear && owner.ClearColor.A >= 1
	}
	var coverage view.OutputCoverage
	scene.Query1[Camera2D](sc).Each(func(h scene.NodeHandle, c *Camera2D) {
		if !c.Active || sc.Space(h) != scene.Space2D || sc.Under3D(h) {
			return
		}
		vp := c.Viewport
		if vp.W <= 0 || vp.H <= 0 {
			vp = mathx.Rect{W: 1, H: 1}
		}
		// Export has normalized coordinates only; pixels outside the output
		// do not participate. Runtime checks the final rasterized rectangles.
		x, y := max(float32(0), vp.X), max(float32(0), vp.Y)
		vp = mathx.Rect{X: x, Y: y, W: max(float32(0), min(float32(1), vp.X+vp.W)-x), H: max(float32(0), min(float32(1), vp.Y+vp.H)-y)}
		coverage.Add(view.View{Viewport: vp, DoClear: c.Clear, Clear: c.ClearColor}, c.Order)
	})
	return coverage.Opaque()
}

func registerPostProcess(r *module.Registry, reg *postfx.Registry) error {
	r.Service(PostEffectsService, reg)
	r.Finalize(func(r *module.Registry) error {
		for _, p := range r.Provided(postfx.ProvisionKind) {
			d, ok := p.Value.(postfx.Descriptor)
			if !ok {
				return fmt.Errorf("%s: expected postfx.Descriptor from %s", postfx.ProvisionKind, p.Owner)
			}
			if err := reg.Register(d); err != nil {
				return fmt.Errorf("post effect from %s: %w", p.Owner, err)
			}
		}
		return nil
	})
	if err := module.Struct[postfx.Parameter](r, schema.Name("render2d.PostParameter"),
		schema.Doc("A typed effect parameter. Missing values inherit the descriptor default."),
		schema.FieldDoc("name", "Stable descriptor field name."), schema.FieldDoc("kind", "Declared shader type."),
		schema.FieldDoc("value", "Floating-point scalar or vector lanes."), schema.FieldDoc("integer", "Exact integer value."),
		schema.FieldDoc("boolean", "Boolean value."), schema.FieldDoc("asset", "Referenced texture for an asset parameter.")); err != nil {
		return err
	}
	if err := module.Struct[postfx.Instance](r, schema.Name("render2d.PostEffect"),
		schema.Doc("An independently addressable effect in a camera output stack."),
		schema.FieldDoc("typeID", "Registered effect type."), schema.FieldDoc("instanceID", "Stable animation and script target. Reordering preserves this ID."),
		schema.FieldDoc("enabled", "Enable this effect."), schema.FieldDoc("version", "Version of the effect parameter schema."),
		schema.FieldDoc("lut", "Optional linear data LUT strip."), schema.FieldDoc("material", "Material using the postfx shader domain."), schema.FieldDoc("parameters", "Explicit typed values for this instance.")); err != nil {
		return err
	}
	b := module.Component[PostProcess](r, schema.Name(NamePostProcess), schema.Space(schema.Space2D), schema.CheckedLiterals(),
		schema.Doc("Applies an ordered effect stack after all cameras sharing this camera's logical output, before UI."),
		schema.FieldDoc("enabled", "Enable this camera's output stack. Disabled stacks do not claim the output."),
		schema.FieldDoc("effects", "Ordered effect instances. Each instance keeps its own identity and typed parameters."),
	).Category("Rendering").Display("Post Process").Icon("comp.camera2d").Requires("engine.Camera2D").Default(func() PostProcess { return PostProcess{Enabled: true} }).Validate(func(ctx module.ValidateContext, c *PostProcess) []problems.Diagnostic {
		cam, ok := scene.Get[Camera2D](ctx.Scene, ctx.Node)
		if !ok {
			return postProcessIssue("PostProcess requires a Camera2D on the same node.")
		}
		var plan postfx.Plan
		src, _ := ctx.Scene.Resolver().(postfx.MaterialSource)
		if err := reg.BuildWithMaterials(&plan, c.Enabled, c.Effects, postfx.Profile{Transparent: !authoredOutputOpaque(ctx.Scene, ctx.Node, cam)}, src); err != nil {
			return postProcessIssue(err.Error())
		}
		if err := plan.CheckOutput(); err != nil {
			return postProcessIssue(err.Error())
		}
		if err := resolvePostAssets(&plan, ctx.Scene.Resolver(), false); err != nil {
			return postProcessIssue(err.Error())
		}
		return nil
	})
	b.Proc2("SetEffectEnabled", func(ctx *bind.Ctx, c *PostProcess, id string, on bool) {
		i, ok := c.Effect(id)
		if !ok {
			ctx.Fail("PostProcess instance not found: " + id)
			return
		}
		i.Enabled = on
	}, bind.Doc("Enables or disables an effect by its stable instance ID."))
	set := func(c *PostProcess, key string, v schema.Value) error {
		id, name, _ := strings.Cut(key, ":")
		e, ok := c.Effect(id)
		if !ok {
			return fmt.Errorf("PostProcess instance %q is unresolved", id)
		}
		f, ok := effectField(reg, e, name)
		if !ok {
			return fmt.Errorf("PostProcess parameter %q is unresolved", key)
		}
		return SetEffectValue(e, f, v)
	}
	b.Proc2("SetFloat", func(ctx *bind.Ctx, c *PostProcess, key string, v float32) {
		ctx.FailErr(set(c, key, schema.Float32(v)))
	}, bind.Doc("Sets a float parameter using instanceID:Field, without changing any shared material."))
	b.Proc2("SetVec2", func(ctx *bind.Ctx, c *PostProcess, key string, v mathx.Vec2) {
		ctx.FailErr(set(c, key, schema.Vec2(v)))
	}, bind.Doc("Sets a vec2 parameter using instanceID:Field."))
	b.Proc2("SetInt", func(ctx *bind.Ctx, c *PostProcess, key string, v int64) {
		if v < math.MinInt32 || v > math.MaxInt32 {
			ctx.Fail("post effect integer is outside int32 range")
			return
		}
		ctx.FailErr(set(c, key, schema.Int32(int32(v))))
	}, bind.Doc("Sets a signed int32 parameter by instanceID:Field."))
	b.Proc2("SetBool", func(ctx *bind.Ctx, c *PostProcess, key string, v bool) {
		ctx.FailErr(set(c, key, schema.Bool(v)))
	}, bind.Doc("Sets a boolean parameter by instanceID:Field."))
	b.Proc2("SetVec3", func(ctx *bind.Ctx, c *PostProcess, key string, v mathx.Vec3) {
		ctx.FailErr(set(c, key, schema.Vec3(v)))
	}, bind.Doc("Sets a vec3 parameter by instanceID:Field."))
	b.Proc2("SetVec4", func(ctx *bind.Ctx, c *PostProcess, key string, v mathx.Vec4) {
		ctx.FailErr(set(c, key, schema.Vec4(v)))
	}, bind.Doc("Sets a vec4 parameter by instanceID:Field."))
	if err := b.Err(); err != nil {
		return err
	}
	return r.Types().RegisterProperties(NamePostProcess, func(s *scene.Scene, h scene.NodeHandle, key string) (*scene.PropertyBinding, error) {
		return resolvePostProperty(reg, s, h, key)
	})
}

func postProcessIssue(msg string) []problems.Diagnostic {
	return []problems.Diagnostic{{Code: "PFX001", Severity: problems.Error, Message: msg, Help: "Check the camera and the named effect in its Post Process stack.", BlocksPlay: true}}
}

func (c *PostProcess) Effect(id string) (*postfx.Instance, bool) {
	for i := range c.Effects {
		if c.Effects[i].InstanceID == id {
			return &c.Effects[i], true
		}
	}
	return nil, false
}

func effectField(reg *postfx.Registry, e *postfx.Instance, name string) (*postfx.Field, bool) {
	d, ok := reg.Lookup(e.TypeID)
	if !ok || e.Version != d.Version {
		return nil, false
	}
	for i := range d.Fields {
		if d.Fields[i].Name == name {
			return &d.Fields[i], true
		}
	}
	return nil, false
}

func EffectValue(e *postfx.Instance, f *postfx.Field) schema.Value {
	v := f.Default
	for _, p := range e.Parameters {
		if p.Name == f.Name {
			v = p
			break
		}
	}
	return MaterialParameter{Value: v.Value, Integer: v.Integer, Boolean: v.Boolean, Kind: v.Kind}.schemaValue(&shader.Param{Kind: f.Kind})
}

func SetEffectValue(e *postfx.Instance, f *postfx.Field, v schema.Value) error {
	val, err := parameterFromValue(&shader.Param{Name: f.Name, Kind: f.Kind}, v)
	if err != nil {
		return err
	}
	p := postfx.Parameter{Name: f.Name, Kind: f.Kind, Value: val.Value, Integer: val.Integer, Boolean: val.Boolean}
	if err = f.Validate(p); err != nil {
		return err
	}
	for i := range e.Parameters {
		if e.Parameters[i].Name == f.Name {
			e.Parameters[i] = p
			return nil
		}
	}
	e.Parameters = append(e.Parameters, p)
	return nil
}

func resolvePostProperty(reg *postfx.Registry, s *scene.Scene, h scene.NodeHandle, key string) (*scene.PropertyBinding, error) {
	id, name, _ := strings.Cut(key, ":")
	c, ok := scene.Get[PostProcess](s, h)
	if !ok {
		return nil, fmt.Errorf("no PostProcess component")
	}
	e, ok := c.Effect(id)
	if !ok {
		return nil, fmt.Errorf("PostProcess instance %q is unresolved", id)
	}
	f, ok := effectField(reg, e, name)
	if !ok {
		return nil, fmt.Errorf("PostProcess parameter %q is unresolved", key)
	}
	typeID, version := e.TypeID, e.Version
	get := func(sc *scene.Scene, n scene.NodeHandle) (*postfx.Instance, bool) {
		c, ok := scene.Get[PostProcess](sc, n)
		if !ok {
			return nil, false
		}
		e, ok := c.Effect(id)
		return e, ok && e.TypeID == typeID && e.Version == version
	}
	return &scene.PropertyBinding{Key: key, Kind: ParameterKind(&shader.Param{Kind: f.Kind}), Valid: func(sc *scene.Scene, n scene.NodeHandle) bool { _, ok := get(sc, n); return ok },
		Get: func(sc *scene.Scene, n scene.NodeHandle, v *schema.Value) bool {
			e, ok := get(sc, n)
			if ok {
				*v = EffectValue(e, f)
			}
			return ok
		},
		Set: func(sc *scene.Scene, n scene.NodeHandle, v schema.Value) bool {
			e, ok := get(sc, n)
			return ok && SetEffectValue(e, f, v) == nil
		}}, nil
}

func (c *PostProcess) Clone() PostProcess {
	out := PostProcess{Enabled: c.Enabled, Effects: slices.Clone(c.Effects)}
	for i := range out.Effects {
		out.Effects[i].Parameters = slices.Clone(c.Effects[i].Parameters)
	}
	return out
}
