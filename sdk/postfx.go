package sdk

import (
	"fmt"

	"aego/engine"
	"aego/modules/render2d"
	"aego/render/postfx"
	"aego/render/shader"
	"aego/schema"
)

type PostProcess = render2d.PostProcess
type PostEffect = postfx.Instance
type PostEffectDescriptor = postfx.Descriptor
type PostEffectField = postfx.Field
type PostEffectParameter = postfx.Parameter
type PostEffectValue = schema.Value
type PostEffectRegistry = postfx.Registry
type PostEffectPhase = postfx.Phase
type PostEffectAlphaPolicy = postfx.AlphaPolicy
type PostEffectKind = shader.Kind

const (
	PostSceneLinear      = postfx.SceneLinear
	PostDisplayLinear    = postfx.DisplayLinear
	PostPreserveCoverage = postfx.PreserveCoverage
	PostOpaqueOnly       = postfx.OpaqueOnly
	PostResampleCoverage = postfx.ResampleCoverage
	PostFloat            = shader.KindFloat
	PostInt              = shader.KindInt
	PostBool             = shader.KindBool
	PostVec2             = shader.KindVec2
	PostVec3             = shader.KindVec3
	PostVec4             = shader.KindVec4
)

// ProvidePostEffect registers a descriptor when the project modules load.
// Runtime, strict validation and editor discovery all use that registration.
func (r *Registry) ProvidePostEffect(d PostEffectDescriptor) { r.r.Provide(postfx.ProvisionKind, d) }

func PostFloatValue(v float32) PostEffectValue { return schema.Float32(v) }
func PostIntValue(v int32) PostEffectValue     { return schema.Int32(v) }
func PostBoolValue(v bool) PostEffectValue     { return schema.Bool(v) }
func PostVec2Value(v Vec2) PostEffectValue     { return schema.Vec2(v) }
func PostVec3Value(v Vec3) PostEffectValue     { return schema.Vec3(v) }
func PostVec4Value(v Vec4) PostEffectValue     { return schema.Vec4(v) }

// PostEffects returns the same registry used by the renderer and the editor.
func PostEffects(ctx *Context) (*PostEffectRegistry, bool) {
	if ctx == nil || ctx.Engine == nil {
		return nil, false
	}
	return engine.Service[*postfx.Registry](ctx.Engine, render2d.PostEffectsService)
}

// SetPostEffectParameter resolves its target by instance ID, including after stack reordering.
func SetPostEffectParameter(s *Scene, h NodeHandle, id, field string, value PostEffectValue) error {
	if s == nil {
		return fmt.Errorf("missing scene")
	}
	td, ok := s.Registry().ByName(render2d.NamePostProcess)
	if !ok {
		return fmt.Errorf("PostProcess module unavailable")
	}
	p, err := s.Types().BindProperty(s, h, td.ID, id+":"+field)
	if err != nil {
		return err
	}
	if p.Access == nil || !p.Set(s, h, value) {
		return fmt.Errorf("invalid value or unresolved post effect %q", id+":"+field)
	}
	return nil
}
