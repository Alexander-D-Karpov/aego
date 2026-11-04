package sdk_test

// This is a project module: all Aego types in its source come from the public SDK.
import "aego/sdk"

type outlineGrade struct{}

func (outlineGrade) Info() sdk.ModuleInfo { return sdk.ModuleInfo{ID: "outlinegrade", Version: "1"} }
func (outlineGrade) Register(r *sdk.Registry) error {
	r.ProvidePostEffect(sdk.PostEffectDescriptor{
		ID: "demo.gain", Label: "Gain", Version: 1, Phase: sdk.PostSceneLinear, Alpha: sdk.PostPreserveCoverage,
		Fields:   []sdk.PostEffectField{{Name: "Gain", Kind: sdk.PostFloat, Bounded: true, Max: 2, Default: sdk.PostEffectParameter{Value: sdk.Vec4{X: 1}}}},
		Neutral:  func(p [4][4]float32) bool { return p[0][0] == 1 },
		Fragment: "vec4 aego_effect(vec4 c,vec2 uv,vec2 size,vec4 p[4]){return vec4(c.rgb*p[0].x,c.a);}",
	})
	return nil
}
func animateGain(s *sdk.Scene, h sdk.NodeHandle, id string) error {
	return sdk.SetPostEffectParameter(s, h, id, "Gain", sdk.PostFloatValue(1.5))
}
