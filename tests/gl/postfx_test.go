//go:build aego_gltest

package gl

import (
	"testing"

	"aego/core/mathx"
	"aego/render"
	"aego/render/material"
	"aego/render/postfx"
	"aego/render/shader"
	"aego/render/sprite"
	"aego/tests/lightinggolden"
)

func TestPostProcessOutputPixels(t *testing.T) {
	e := newGLEngine(t)
	defer e.Close()
	g, err := lightinggolden.New(e.GPU(), e.Caps(), e.VFS())
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	report := lightinggolden.NewReport()
	lightinggolden.RunPostProcess(g, report)
	lightinggolden.RunExtendedPostProcess(g, report)
	t.Logf("renderer=%s passed=%v", e.Caps().Renderer, report.Passed)
	for _, err := range report.Failures {
		t.Error(err)
	}
}

func TestCustomPostFXReloadKeepsLastGoodPhaseAndPixels(t *testing.T) {
	r, dev, target := materialRig(t)
	const src = "#pragma aego abi 1\n#pragma postfx display_linear\nvoid fragment(){aego_out=vec4(1,0,0,1);}"
	sh := runtimeShader(t, "post", src)
	m, _ := material.New("custom", sh, 0, shader.BlendOpaque)
	d, _ := postfx.NewRegistry().Lookup(postfx.CustomID)
	p := postfx.Plan{Passes: []postfx.Pass{{Descriptor: d, Material: m}}}
	draw := func(f *render.ViewFrame) {
		f.View().Clear = mathx.Color{A: 1}
		f.PostProcess(&p)
		f.Sprite(sprite.Draw{Region: r.White(), Position: mathx.V2(16, 16), Size: mathx.V2(24, 24), Origin: mathx.V2(.5, .5), Tint: mathx.White})
	}
	shot := func() [4]byte { return drawMaterialFrame(t, r, dev, target, draw) }
	if got := shot(); got != ([4]byte{255, 0, 0, 255}) {
		t.Fatal("original", got, r.PostProcessError())
	}
	n := 0
	r.Materials().OnError = func(*material.Shader, int, error) { n++ }
	bad := runtimeShader(t, "broken", "#pragma aego abi 1\n#pragma postfx scene_linear\nvoid fragment(){aego_out=undefined_name;}")
	r.Materials().QueueReload(sh, bad, []*material.Material{m}, nil)
	shot()
	if got := shot(); got != ([4]byte{255, 0, 0, 255}) || p.Passes[0].Phase() != postfx.DisplayLinear || n != 1 || !m.Failed() {
		t.Fatal("last good was lost", got, n, m.Failed())
	}
	good := runtimeShader(t, "repaired", "#pragma aego abi 1\n#pragma postfx scene_linear\nvoid fragment(){aego_out=vec4(0,1,0,1);}")
	r.Materials().QueueReload(sh, good, []*material.Material{m}, nil)
	shot()
	got := shot()
	if got[1] < 180 || got[0] != 0 || got[2] != 0 || p.Passes[0].Phase() != postfx.SceneLinear || m.Failed() || m.Shader() != good {
		t.Fatal("replacement not published", got, m.Failed())
	}
}
