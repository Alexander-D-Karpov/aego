package postfx

import (
	"aego/core/mathx"
	"aego/render/shader"
)

const VignetteID = "postfx.vignette"

func vignette() Descriptor {
	return Descriptor{
		ID: VignetteID, Label: "Vignette", Version: 1, Phase: DisplayLinear, Alpha: PreserveCoverage,
		Fields: []Field{
			{Name: "Intensity", Kind: shader.KindFloat, Default: Parameter{Value: mathx.Vec4{X: .35}}, Bounded: true, Max: 1},
			{Name: "Center", Kind: shader.KindVec2, Default: Parameter{Value: mathx.Vec4{X: .5, Y: .5}}, Bounded: true, Max: 1},
			{Name: "Softness", Kind: shader.KindFloat, Default: Parameter{Value: mathx.Vec4{X: .65}}, Bounded: true, Max: 1},
		},
		Neutral: func(p [4][4]float32) bool { return p[0][0] == 0 },
		Fragment: `
vec4 aego_effect(vec4 inputColor, vec2 localUV, vec2 outputSize, vec4 params[4]) {
    vec2 delta = (localUV - params[1].xy) * outputSize;
    float radius = 2.0 * length(delta) / max(1.0, min(outputSize.x, outputSize.y));
    float softness = params[2].x;
    float edge = softness > 0.0 ? smoothstep(1.0 - softness, 1.0, radius) : step(1.0, radius);
    return vec4(inputColor.rgb * (1.0 - params[0].x * edge), inputColor.a);
}
`,
	}
}
