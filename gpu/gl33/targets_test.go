//go:build aego_gltest

package gl33

import (
	"math"
	"runtime"
	"testing"

	"aego/core/mathx"
	"aego/gpu"
	"aego/gpu/gl33/gl"
	"aego/platform"
	"aego/platform/sdl"
)

func testDevice(t *testing.T) *Device {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	p, err := sdl.New()
	if err != nil {
		t.Fatalf("real GL context unavailable: %v", err)
	}
	t.Cleanup(p.Shutdown)
	w, err := p.CreateWindow(platform.WindowDesc{Width: 32, Height: 32, Title: "gpu targets", Hidden: true, SRGB: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	d, err := New(w.(*sdl.Window).GLProcAddress, Options{Debug: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	t.Logf("renderer=%s version=%s fragment texture units=%d array layers=%d RGBA16F=%v", d.Caps().Renderer, d.Caps().Version, d.Caps().MaxFragmentTextureUnits, d.Caps().MaxArrayTextureLayers, d.Caps().RGBA16FRenderable)
	return d
}

func TestArrayLayerMipTargetsAndReadback(t *testing.T) {
	d := testDevice(t)
	h, err := d.CreateTexture(gpu.TextureDesc{Width: 8, Height: 4, Dimension: gpu.Texture2DArray, Layers: 3, Mips: true, Format: gpu.TexRGBA8})
	if err != nil {
		t.Fatal(err)
	}
	colors := []mathx.Color{{R: 1, A: 1}, {G: 1, A: 1}, {B: 1, A: 1}}
	for layer, c := range colors {
		for mip := 0; mip < 4; mip++ {
			f := d.BeginFrame(32, 32)
			p := f.BeginPass(gpu.PassDesc{Target: h, TargetSubresource: gpu.TextureSubresource{Layer: layer, Mip: mip}, Color: gpu.LoadClear, ClearColor: c})
			p.End()
			d.EndFrame()
		}
	}
	if len(d.fbos) != 12 {
		t.Fatalf("framebuffer key aliases subresources: %d", len(d.fbos))
	}
	for layer := range colors {
		for mip := 0; mip < 4; mip++ {
			var got [4]byte
			if err := d.ReadTexturePixels(h, gpu.TextureSubresource{Layer: layer, Mip: mip}, 0, 0, 1, 1, got[:]); err != nil {
				t.Fatal(err)
			}
			for channel := 0; channel < 4; channel++ {
				want := byte(0)
				if channel == layer || channel == 3 {
					want = 255
				}
				if got[channel] != want {
					t.Fatalf("layer %d mip %d pixel %v", layer, mip, got)
				}
			}
		}
	}
	if err := d.UpdateTextureRegion(h, gpu.TextureUpdate{Subresource: gpu.TextureSubresource{Layer: 2, Mip: 1}, X: 1, Y: 0, Width: 1, Height: 1, Data: []byte{255, 255, 0, 255}}); err != nil {
		t.Fatal(err)
	}
	var got [8]byte
	if err := d.ReadTexturePixels(h, gpu.TextureSubresource{Layer: 2, Mip: 1}, 0, 0, 2, 1, got[:]); err != nil {
		t.Fatal(err)
	}
	if got != [8]byte{0, 0, 255, 255, 255, 255, 0, 255} {
		t.Fatalf("mip update changed neighbours: %v", got)
	}
	if d.ValidateRenderTarget(gpu.PassDesc{Target: h, TargetSubresource: gpu.TextureSubresource{Layer: 3}}) == nil {
		t.Fatal("out-of-range framebuffer accepted")
	}
	d.DestroyTexture(h)
	if len(d.fbos) != 0 {
		t.Fatal("destroying array leaked subresource framebuffers")
	}
	if d.ValidateRenderTarget(gpu.PassDesc{Target: h}) == nil {
		t.Fatal("stale framebuffer accepted")
	}
}

func TestArraySamplingAndInvalidPass(t *testing.T) {
	d := testDevice(t)
	array, err := d.CreateTexture(gpu.TextureDesc{Width: 1, Height: 1, Dimension: gpu.Texture2DArray, Layers: 2, Format: gpu.TexRGBA8, Data: []byte{255, 0, 0, 255, 0, 255, 0, 255}})
	if err != nil {
		t.Fatal(err)
	}
	target, err := d.CreateTexture(gpu.TextureDesc{Width: 1, Height: 1, Format: gpu.TexRGBA8})
	if err != nil {
		t.Fatal(err)
	}
	sh, err := d.CreateShader(gpu.ShaderDesc{Sources: []gpu.ShaderSource{
		{Stage: gpu.StageVertex, Source: `#version 330 core
void main(){vec2 p=vec2((gl_VertexID<<1)&2,gl_VertexID&2);gl_Position=vec4(p*2.0-1.0,0,1);}`},
		{Stage: gpu.StageFragment, Source: `#version 330 core
uniform sampler2DArray tex3;
out vec4 color;
void main(){color=texelFetch(tex3,ivec3(0,0,1),0);}`},
	}})
	if err != nil {
		t.Fatal(err)
	}
	pipe, err := d.CreatePipeline(gpu.PipelineDesc{Shader: sh, Primitive: gpu.PrimTriangles})
	if err != nil {
		t.Fatal(err)
	}
	f := d.BeginFrame(32, 32)
	p := f.BeginPass(gpu.PassDesc{Target: target, Color: gpu.LoadClear})
	p.SetPipeline(pipe)
	p.SetTexture(3, array, gpu.SamplerHandle{})
	p.Draw(0, 3)
	p.End()
	bad := f.BeginPass(gpu.PassDesc{Target: target, TargetSubresource: gpu.TextureSubresource{Layer: 1}, Color: gpu.LoadClear, ClearColor: mathx.Color{R: 1, A: 1}})
	bad.SetPipeline(pipe)
	bad.Draw(0, 3)
	bad.End()
	d.EndFrame()
	if d.frame.stats.DrawCalls != 1 {
		t.Fatalf("invalid pass submitted drawing: %+v", d.frame.stats)
	}
	var got [4]byte
	if err := d.ReadPixels(target, 0, 0, 1, 1, got[:]); err != nil {
		t.Fatal(err)
	}
	if got != [4]byte{0, 255, 0, 255} {
		t.Fatalf("array sampling or invalid pass damaged target: %v", got)
	}
	p = d.BeginFrame(32, 32).BeginPass(gpu.PassDesc{Target: target})
	p.SetTexture(3, array, gpu.SamplerHandle{})
	p.SetTexture(3, target, gpu.SamplerHandle{})
	p.SetTexture(3, gpu.TextureHandle{}, gpu.SamplerHandle{})
	p.End()
	d.EndFrame()
	gl.ActiveTexture(gl.TEXTURE0 + 3)
	if getInt(gl.TEXTURE_BINDING_2D_ARRAY) != 0 || getInt(gl.TEXTURE_BINDING_2D) != 0 {
		t.Fatal("zero texture handle leaves stale target binding")
	}

}

func TestRGBA16FProbeAndReadback(t *testing.T) {
	d := testDevice(t)
	if !d.Caps().RGBA16FRenderable {
		t.Fatal("RGBA16F draw/blend/readback probe failed on actual GL3.3 context")
	}
	// Reprobe with non-default state, checking that capability verification does
	// not contaminate subsequent render passes or binding cache assumptions.
	gl.Viewport(1, 2, 7, 9)
	gl.ClearColor(.2, .3, .4, .5)
	gl.ColorMask(true, false, true, false)
	gl.Enable(gl.SCISSOR_TEST)
	gl.Disable(gl.BLEND)
	if !d.probeRGBA16F() {
		t.Fatal("stateful probe failed")
	}
	var viewport [4]int32
	var mask [4]bool
	var color [4]float32
	gl.GetIntegerv(gl.VIEWPORT, &viewport[0])
	gl.GetBooleanv(gl.COLOR_WRITEMASK, &mask[0])
	gl.GetFloatv(gl.COLOR_CLEAR_VALUE, &color[0])
	if viewport != [4]int32{1, 2, 7, 9} || mask != [4]bool{true, false, true, false} || color != [4]float32{.2, .3, .4, .5} || !gl.IsEnabled(gl.SCISSOR_TEST) || gl.IsEnabled(gl.BLEND) {
		t.Fatal("probe leaked GL state")
	}
	gl.ColorMask(true, true, true, true)
	h, err := d.CreateTexture(gpu.TextureDesc{Width: 2, Height: 2, Format: gpu.TexRGBA16F})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.ValidateRenderTarget(gpu.PassDesc{Target: h}); err != nil {
		t.Fatal(err)
	}
	p := d.BeginFrame(32, 32).BeginPass(gpu.PassDesc{Target: h, Color: gpu.LoadClear, ClearColor: mathx.Color{R: 3, G: .5, B: .25, A: .75}})
	p.End()
	d.EndFrame()
	var got [4]float32
	if err := d.ReadTexturePixelsFloat(h, gpu.TextureSubresource{}, 0, 0, 1, 1, got[:]); err != nil {
		t.Fatal(err)
	}
	for i, want := range [4]float32{3, .5, .25, .75} {
		if math.Abs(float64(got[i]-want)) > .001 {
			t.Fatalf("float readback clamps or changes values: %v", got)
		}
	}
	if d.ReadPixels(h, 0, 0, 1, 1, make([]byte, 4)) == nil {
		t.Fatal("float attachment accepted byte readback")
	}
}

func TestR8MaskArrayAttachments(t *testing.T) {
	d := testDevice(t)
	before := d.ResourceStats()
	h, err := d.CreateTexture(gpu.TextureDesc{Width: 3, Height: 3, Dimension: gpu.Texture2DArray, Layers: 2, Format: gpu.TexR8, Data: make([]byte, 18)})
	if err != nil {
		t.Fatal(err)
	}
	if d.ResourceStats().TextureStorageBytes-before.TextureStorageBytes != 18 {
		t.Fatal("R8 array storage accounting")
	}
	p := d.BeginFrame(32, 32).BeginPass(gpu.PassDesc{Target: h, TargetSubresource: gpu.TextureSubresource{Layer: 1}, Color: gpu.LoadClear, ClearColor: mathx.Color{R: .75}})
	p.End()
	d.EndFrame()
	var got [4]byte
	if err := d.ReadTexturePixels(h, gpu.TextureSubresource{Layer: 1}, 0, 0, 1, 1, got[:]); err != nil {
		t.Fatal(err)
	}
	if got[0] < 190 || got[0] > 192 || got[1] != 0 || got[2] != 0 || got[3] != 255 {
		t.Fatalf("R8 rendered pixel %v", got)
	}
	if err := d.ReadTexturePixels(h, gpu.TextureSubresource{}, 0, 0, 1, 1, got[:]); err != nil {
		t.Fatal(err)
	}
	if got != [4]byte{0, 0, 0, 255} {
		t.Fatalf("R8 layer isolation %v", got)
	}
	d.DestroyTexture(h)
	if d.ResourceStats() != before {
		t.Fatalf("array/FBO resource leak: before=%+v after=%+v", before, d.ResourceStats())
	}
}

func TestAdditivePreservesCoverageAndLegacyIsExplicit(t *testing.T) {
	d := testDevice(t)
	if !d.Caps().RGBA16FRenderable {
		t.Fatal("RGBA16F required for HDR additive contract")
	}
	target, err := d.CreateTexture(gpu.TextureDesc{Width: 1, Height: 1, Format: gpu.TexRGBA16F})
	if err != nil {
		t.Fatal(err)
	}
	sh, err := d.CreateShader(gpu.ShaderDesc{Sources: []gpu.ShaderSource{
		{Stage: gpu.StageVertex, Source: `#version 330 core
void main(){vec2 p=vec2((gl_VertexID<<1)&2,gl_VertexID&2);gl_Position=vec4(p*2.0-1.0,0,1);}`},
		{Stage: gpu.StageFragment, Source: `#version 330 core
out vec4 color;
void main(){color=vec4(2.0,0.5,0.25,0.5);}`},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		blend gpu.BlendState
		alpha float32
	}{{"additive", gpu.BlendAdditive(), .25}, {"legacy", gpu.BlendAdditiveLegacy(), .75}} {
		pipe, err := d.CreatePipeline(gpu.PipelineDesc{Shader: sh, Primitive: gpu.PrimTriangles, Blend: test.blend})
		if err != nil {
			t.Fatal(err)
		}
		p := d.BeginFrame(32, 32).BeginPass(gpu.PassDesc{Target: target, Color: gpu.LoadClear, ClearColor: mathx.Color{R: 1, A: .25}})
		p.SetPipeline(pipe)
		p.Draw(0, 3)
		p.End()
		d.EndFrame()
		var got [4]float32
		if err := d.ReadTexturePixelsFloat(target, gpu.TextureSubresource{}, 0, 0, 1, 1, got[:]); err != nil {
			t.Fatal(err)
		}
		for i, want := range [4]float32{3, .5, .25, test.alpha} {
			if math.Abs(float64(got[i]-want)) > .001 {
				t.Fatalf("%s blend %v, want alpha %v", test.name, got, test.alpha)
			}
		}
		d.DestroyPipeline(pipe)
	}
}
