package playground

import (
	_ "embed"
	"errors"

	"aego/asset"
	"aego/core/ids"
	"aego/render"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/shaderx"
)

//go:embed bench.aeshader
var benchShader []byte

// BenchClock is the time the material bench and its GL golden are drawn at.
var BenchClock = render.Clock{Time: 0.5, Delta: 1.0 / 60, Unscaled: 1.0 / 60, Frame: 30}

// BenchMaterial is the test material on bench.aeshader with Speed 2; it reads aego_time and a parameter.
func BenchMaterial() (*material.Material, error) {
	u, diags := shaderx.Parse(benchShader, "bench.aeshader", nil)
	if len(diags) > 0 {
		return nil, diags[0]
	}
	sh := asset.BuildShader(u).Runtime(ids.AssetID{}, "bench")
	m, ok := material.New("bench", sh, 0, shader.BlendPremultiplied)
	if !ok {
		return nil, errors.New("the bench shader has no variant")
	}
	m.Set("Speed", [4]float32{2})
	return m, nil
}
