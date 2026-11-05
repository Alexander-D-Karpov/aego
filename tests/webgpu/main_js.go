//go:build js && wasm

// This executable uses an actual browser context. A missing browser capability
// is reported as unavailable or failed, never as a successful rendering check.
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"syscall/js"
	"time"

	"aego/core/mathx"
	"aego/gpu"
	"aego/gpu/webgl2"
	"aego/platform"
	"aego/platform/web"
)

type surface struct{ ctx js.Value }

func (s surface) WebGL2() js.Value { return s.ctx }

type report struct {
	Passed                  []string `json:"passed"`
	Failures                []string `json:"failures"`
	Unverified              []string `json:"unverified"`
	Renderer                string   `json:"renderer"`
	Restores                int      `json:"contextRestores"`
	FloatExtension          bool     `json:"floatExtension"`
	FloatRenderable         bool     `json:"rgba16fRenderable"`
	Parallel                bool     `json:"parallelShaderCompile"`
	MaxFragmentTextureUnits int      `json:"maxFragmentTextureUnits"`
	MaxArrayTextureLayers   int      `json:"maxArrayTextureLayers"`
}

func (r *report) check(name string, ok bool, detail any) {
	if ok {
		r.Passed = append(r.Passed, name)
	} else {
		r.Failures = append(r.Failures, fmt.Sprintf("%s: %v", name, detail))
	}
}
func waitFor(fn func() bool) bool {
	until := time.Now().Add(15 * time.Second)
	for time.Now().Before(until) {
		if fn() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func source() gpu.ShaderDesc {
	return gpu.ShaderDesc{Label: "array sampler", Sources: []gpu.ShaderSource{
		{Stage: gpu.StageVertex, Source: `#version 300 es
void main(){vec2 p=vec2((gl_VertexID<<1)&2,gl_VertexID&2);gl_Position=vec4(p*2.0-1.0,0,1);}`},
		{Stage: gpu.StageFragment, Source: `#version 300 es
precision highp float;
precision highp sampler2DArray;
uniform sampler2DArray tex3;
out vec4 color;
void main(){color=texelFetch(tex3,ivec3(0,0,1),0);}`},
	}}
}

func arrayLayers(r *report, d *webgl2.Device) {
	h, err := d.CreateTexture(gpu.TextureDesc{Width: 8, Height: 4, Dimension: gpu.Texture2DArray, Layers: 3, Mips: true, Format: gpu.TexRGBA8})
	r.check("array-create", err == nil, err)
	if err != nil {
		return
	}
	defer d.DestroyTexture(h)
	colors := []mathx.Color{{R: 1, A: 1}, {G: 1, A: 1}, {B: 1, A: 1}}
	for layer, c := range colors {
		for mip := 0; mip < 4; mip++ {
			p := d.BeginFrame(32, 32).BeginPass(gpu.PassDesc{Target: h, TargetSubresource: gpu.TextureSubresource{Layer: layer, Mip: mip}, Color: gpu.LoadClear, ClearColor: c})
			p.End()
			d.EndFrame()
		}
	}
	for layer := range colors {
		for mip := 0; mip < 4; mip++ {
			var got [4]byte
			err := d.ReadTexturePixels(h, gpu.TextureSubresource{Layer: layer, Mip: mip}, 0, 0, 1, 1, got[:])
			want := [4]byte{0, 0, 0, 255}
			want[layer] = 255
			r.check(fmt.Sprintf("array-layer-%d-mip-%d", layer, mip), err == nil && got == want, fmt.Sprintf("%v, %v", got, err))
		}
	}
	err = d.UpdateTextureRegion(h, gpu.TextureUpdate{Subresource: gpu.TextureSubresource{Layer: 2, Mip: 1}, X: 1, Y: 0, Width: 1, Height: 1, Data: []byte{255, 255, 0, 255}})
	var got [8]byte
	readErr := d.ReadTexturePixels(h, gpu.TextureSubresource{Layer: 2, Mip: 1}, 0, 0, 2, 1, got[:])
	r.check("array-partial-mip-update", err == nil && readErr == nil && got == [8]byte{0, 0, 255, 255, 255, 255, 0, 255}, fmt.Sprintf("%v %v %v", got, err, readErr))
	r.check("reject-array-layer-overflow", d.ValidateRenderTarget(gpu.PassDesc{Target: h, TargetSubresource: gpu.TextureSubresource{Layer: 3}}) != nil, "out-of-range layer accepted")
	r.check("reject-array-mip-overflow", d.ValidateRenderTarget(gpu.PassDesc{Target: h, TargetSubresource: gpu.TextureSubresource{Mip: 4}}) != nil, "out-of-range mip accepted")
	r.check("reject-mip-read-overflow", d.ReadTexturePixels(h, gpu.TextureSubresource{Mip: 3}, 0, 0, 2, 1, make([]byte, 8)) != nil, "read exceeds mip dimensions")
	checkClearRect(r, d, h)
}

// With a viewport smaller than the physical 4x2 array mip, prime a nonempty scissor before each clear; the full-mip and unrestricted clears must use identical commands.
func checkClearRect(r *report, d *webgl2.Device, h gpu.TextureHandle) {
	sub := gpu.TextureSubresource{Layer: 1, Mip: 1}
	full, partial := mathx.Rect{W: 4, H: 2}, mathx.Rect{W: 2, H: 1}
	red, green := mathx.Color{R: 1, A: 1}, mathx.Color{G: 1, A: 1}
	clear := func(rect mathx.Rect, color mathx.Color) uint64 {
		f := d.BeginFrame(32, 32)
		desc := gpu.PassDesc{Target: h, TargetSubresource: sub, Color: gpu.LoadKeep}
		p := f.BeginPass(desc)
		p.SetViewport(partial)
		p.SetScissor(mathx.Rect{X: 2, Y: 1, W: 1, H: 1}, true)
		p.End()
		desc.Color, desc.ClearColor, desc.ClearRect = gpu.LoadClear, color, rect
		before := d.BridgeStats().Calls
		p = f.BeginPass(desc)
		p.End()
		count := d.BridgeStats().Calls - before
		d.EndFrame()
		return count
	}
	unrestricted := clear(mathx.Rect{}, green)
	explicit := clear(full, red)
	r.check("full-clear-no-redundant-scissor", explicit == unrestricted, fmt.Sprintf("full=%d unrestricted=%d", explicit, unrestricted))
	clear(partial, green)
	var pixels [32]byte
	err := d.ReadTexturePixels(h, sub, 0, 0, 4, 2, pixels[:])
	correct := err == nil
	for i := 0; i < 8; i++ {
		want := [4]byte{255, 0, 0, 255}
		if i < 2 {
			want = [4]byte{0, 255, 0, 255}
		}
		correct = correct && [4]byte(pixels[i*4:i*4+4]) == want
	}
	r.check("partial-clear-physical-mip-extent", correct, fmt.Sprintf("%v %v", pixels, err))

	draw := source()
	draw.Sources[1].Source = "#version 300 es\nprecision highp float; out vec4 color; void main(){color=vec4(0,0,1,1);}"
	sh, err := d.CreateShader(draw)
	if err != nil {
		r.check("draw-after-partial-clear", false, err)
		return
	}
	defer d.DestroyShader(sh)
	pipe, err := d.CreatePipeline(gpu.PipelineDesc{Shader: sh, Primitive: gpu.PrimTriangles})
	if err != nil {
		r.check("draw-after-partial-clear", false, err)
		return
	}
	defer d.DestroyPipeline(pipe)
	p := d.BeginFrame(32, 32).BeginPass(gpu.PassDesc{Target: h, TargetSubresource: sub, Color: gpu.LoadKeep})
	p.SetPipeline(pipe)
	p.Draw(0, 3)
	p.End()
	d.EndFrame()
	err = d.ReadTexturePixels(h, sub, 0, 0, 4, 2, pixels[:])
	correct = err == nil
	for i := 0; i < 8; i++ {
		correct = correct && [4]byte(pixels[i*4:i*4+4]) == [4]byte{0, 0, 255, 255}
	}
	var sibling [4]byte
	e2 := d.ReadTexturePixels(h, gpu.TextureSubresource{Layer: 0, Mip: 1}, 0, 0, 1, 1, sibling[:])
	r.check("draw-after-partial-clear", correct && e2 == nil && sibling == [4]byte{255, 0, 0, 255}, fmt.Sprintf("%v sibling=%v %v %v", pixels, sibling, err, e2))

}

// A legacy flat 2D pass may omit only the mismatched depth attachment;
// strict validation must still reject the pair and arrays never get this fallback.
func legacyDepthSize(r *report, d *webgl2.Device) {
	color, err := d.CreateTexture(gpu.TextureDesc{Width: 2, Height: 2, Format: gpu.TexRGBA8})
	if err != nil {
		r.check("legacy-depth-color-create", false, err)
		return
	}
	defer d.DestroyTexture(color)
	depth, err := d.CreateTexture(gpu.TextureDesc{Width: 4, Height: 4, Format: gpu.TexDepth24Stencil8})
	if err != nil {
		r.check("legacy-depth-create", false, err)
		return
	}
	defer d.DestroyTexture(depth)
	desc := gpu.PassDesc{Target: color, DepthTarget: depth, Color: gpu.LoadClear, ClearColor: mathx.Color{G: 1, A: 1}}
	r.check("strict-depth-size-validation", d.ValidateRenderTarget(desc) != nil, "mismatch accepted")
	p := d.BeginFrame(32, 32).BeginPass(desc)
	p.End()
	d.EndFrame()
	var pixel [4]byte
	err = d.ReadPixels(color, 0, 0, 1, 1, pixel[:])
	r.check("legacy-depth-mismatch-preserves-color-target", err == nil && pixel == [4]byte{0, 255, 0, 255}, fmt.Sprintf("%v %v", pixel, err))
}

func maskArray(r *report, d *webgl2.Device) {
	h, err := d.CreateTexture(gpu.TextureDesc{Width: 3, Height: 3, Dimension: gpu.Texture2DArray, Layers: 2, Format: gpu.TexR8, Data: make([]byte, 18)})
	r.check("r8-mask-array-create", err == nil, err)
	if err != nil {
		return
	}
	defer d.DestroyTexture(h)
	p := d.BeginFrame(32, 32).BeginPass(gpu.PassDesc{Target: h, TargetSubresource: gpu.TextureSubresource{Layer: 1}, Color: gpu.LoadClear, ClearColor: mathx.Color{R: .75}})
	p.End()
	d.EndFrame()
	var got [4]byte
	err = d.ReadTexturePixels(h, gpu.TextureSubresource{Layer: 1}, 0, 0, 1, 1, got[:])
	r.check("r8-mask-array-render", err == nil && got[0] >= 190 && got[0] <= 192 && got[1] == 0 && got[2] == 0 && got[3] == 255, fmt.Sprintf("%v %v", got, err))
	err = d.ReadTexturePixels(h, gpu.TextureSubresource{}, 0, 0, 1, 1, got[:])
	r.check("r8-mask-array-layer-isolation", err == nil && got == [4]byte{0, 0, 0, 255}, fmt.Sprintf("%v %v", got, err))
}

// These two deliberate capability faults exercise fallback decisions. They do
// not represent hardware that physically lacks the float extension.
func forcedFloatFailure(r *report, missingExtension bool) {
	label := "forced-probe-failure"
	if missingExtension {
		label = "forced-no-float-extension"
	}
	canvas := js.Global().Get("document").Call("createElement", "canvas")
	canvas.Set("width", 4)
	canvas.Set("height", 4)
	ctx := canvas.Call("getContext", "webgl2")
	if !ctx.Truthy() {
		r.check(label+"-context", false, "unavailable")
		return
	}
	inProbe := true
	var hook js.Func
	method := "checkFramebufferStatus"
	if missingExtension {
		method = "getExtension"
	}
	original := ctx.Get(method).Call("bind", ctx)
	hook = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if missingExtension && args[0].String() == "EXT_color_buffer_float" {
			return js.Null()
		}
		if !missingExtension && inProbe {
			return ctx.Get("FRAMEBUFFER_UNSUPPORTED")
		}
		if len(args) > 0 {
			return original.Invoke(args[0])
		}
		return original.Invoke()
	})
	ctx.Set(method, hook)
	defer hook.Release()
	d, err := webgl2.New(surface{ctx}, webgl2.Options{Debug: true})
	inProbe = false
	r.check(label+"-device", err == nil, err)
	if err != nil {
		return
	}
	defer d.Close()
	r.check(label+"-capability-disabled", !d.Caps().RGBA16FRenderable, d.Caps().RGBA16FRenderable)
	h, err := d.CreateTexture(gpu.TextureDesc{Width: 1, Height: 1, Format: gpu.TexRGBA16F})
	if err == nil {
		r.check(label+"-float-target-rejected", d.ValidateRenderTarget(gpu.PassDesc{Target: h}) != nil, "HDR target accepted")
		d.DestroyTexture(h)
	} else {
		r.check(label+"-float-allocation", false, err)
	}
	ldr, err := d.CreateTexture(gpu.TextureDesc{Width: 1, Height: 1, Format: gpu.TexRGBA8})
	r.check(label+"-ldr-create", err == nil, err)
	if err != nil {
		return
	}
	defer d.DestroyTexture(ldr)
	p := d.BeginFrame(4, 4).BeginPass(gpu.PassDesc{Target: ldr, Color: gpu.LoadClear, ClearColor: mathx.Color{G: 1, A: 1}})
	p.End()
	d.EndFrame()
	var px [4]byte
	err = d.ReadPixels(ldr, 0, 0, 1, 1, px[:])
	r.check(label+"-ldr-render", err == nil && px == [4]byte{0, 255, 0, 255}, fmt.Sprintf("%v %v", px, err))
}

func floatTarget(r *report, d *webgl2.Device, label string) {
	if !d.Caps().RGBA16FRenderable {
		if r.FloatExtension {
			r.check(label+"-rgba16f-probe", false, "EXT_color_buffer_float is present but actual draw/blend/readback probe failed")
		} else {
			r.Unverified = append(r.Unverified, label+"-rgba16f (EXT_color_buffer_float unavailable)")
		}
		return
	}
	h, err := d.CreateTexture(gpu.TextureDesc{Width: 2, Height: 2, Format: gpu.TexRGBA16F})
	r.check(label+"-float-create", err == nil, err)
	if err != nil {
		return
	}
	defer d.DestroyTexture(h)
	err = d.ValidateRenderTarget(gpu.PassDesc{Target: h})
	r.check(label+"-float-framebuffer", err == nil, err)
	if err != nil {
		return
	}
	p := d.BeginFrame(32, 32).BeginPass(gpu.PassDesc{Target: h, Color: gpu.LoadClear, ClearColor: mathx.Color{R: 3, G: .5, B: .25, A: .75}})
	p.End()
	d.EndFrame()
	var got [4]float32
	err = d.ReadTexturePixelsFloat(h, gpu.TextureSubresource{}, 0, 0, 1, 1, got[:])
	valid := err == nil
	for i, want := range [4]float32{3, .5, .25, .75} {
		valid = valid && !math.IsNaN(float64(got[i])) && math.Abs(float64(got[i]-want)) < .001
	}
	r.check(label+"-float-values-over-one", valid, fmt.Sprintf("%v %v", got, err))
	r.check(label+"-float-reject-byte-readback", d.ReadPixels(h, 0, 0, 1, 1, make([]byte, 4)) != nil, "UNSIGNED_BYTE used for float attachment")
}

func additive(r *report, d *webgl2.Device) {
	if !d.Caps().RGBA16FRenderable {
		r.Unverified = append(r.Unverified, "additive HDR blend: RGBA16F unavailable")
		return
	}
	target, err := d.CreateTexture(gpu.TextureDesc{Width: 1, Height: 1, Format: gpu.TexRGBA16F})
	r.check("additive-hdr-create", err == nil, err)
	if err != nil {
		return
	}
	defer d.DestroyTexture(target)
	desc := source()
	desc.Label = "additive coverage"
	desc.Sources[1].Source = `#version 300 es
precision highp float;
out vec4 color;
void main(){color=vec4(2.0,0.5,0.25,0.5);}`
	sh, err := d.CreateShader(desc)
	r.check("additive-shader-link", err == nil, err)
	if err != nil {
		return
	}
	defer d.DestroyShader(sh)
	for _, test := range []struct {
		name  string
		blend gpu.BlendState
		alpha float32
	}{{"additive-preserves-coverage", gpu.BlendAdditive(), .25}, {"additive-legacy-alpha-sum", gpu.BlendAdditiveLegacy(), .75}} {
		pipe, err := d.CreatePipeline(gpu.PipelineDesc{Shader: sh, Primitive: gpu.PrimTriangles, Blend: test.blend})
		if err != nil {
			r.check(test.name, false, err)
			continue
		}
		p := d.BeginFrame(32, 32).BeginPass(gpu.PassDesc{Target: target, Color: gpu.LoadClear, ClearColor: mathx.Color{R: 1, A: .25}})
		p.SetPipeline(pipe)
		p.Draw(0, 3)
		p.End()
		d.EndFrame()
		var got [4]float32
		err = d.ReadTexturePixelsFloat(target, gpu.TextureSubresource{}, 0, 0, 1, 1, got[:])
		valid := err == nil
		for i, want := range [4]float32{3, .5, .25, test.alpha} {
			valid = valid && !math.IsNaN(float64(got[i])) && math.Abs(float64(got[i]-want)) < .001
		}
		r.check(test.name, valid, fmt.Sprintf("%v %v", got, err))
		d.DestroyPipeline(pipe)
	}
}

func restore(r *report, d *webgl2.Device, canvas js.Value) {
	// Use the production platform listeners on the very canvas whose context
	// is lost and restored below. Each native edge must reset exactly once.
	clock, err := web.New(web.Options{Canvas: canvas})
	if err != nil {
		r.check("restore-platform", false, err)
		return
	}
	defer clock.Shutdown()
	if _, err = clock.CreateWindow(platform.WindowDesc{Width: 32, Height: 32}); err != nil {
		r.check("restore-platform", false, err)
		return
	}

	data := make([]byte, 8*4*2*4)
	for i := 0; i < len(data); i += 4 {
		data[i+1] = 255
		data[i+3] = 255
	}
	array, err := d.CreateTexture(gpu.TextureDesc{Width: 8, Height: 4, Dimension: gpu.Texture2DArray, Layers: 2, Mips: true, Format: gpu.TexRGBA8, Data: data})
	r.check("restore-array-create", err == nil, err)
	if err != nil {
		return
	}
	defer d.DestroyTexture(array)
	err = d.UpdateTextureRegion(array, gpu.TextureUpdate{Subresource: gpu.TextureSubresource{Layer: 1, Mip: 1}, X: 1, Y: 0, Width: 1, Height: 1, Data: []byte{255, 255, 0, 255}})
	r.check("restore-mip-update", err == nil, err)
	target, err := d.CreateTexture(gpu.TextureDesc{Width: 1, Height: 1, Format: gpu.TexRGBA8})
	r.check("restore-target-create", err == nil, err)
	if err != nil {
		return
	}
	defer d.DestroyTexture(target)
	sh, err := d.CreateShader(source())
	r.check("array-shader-link", err == nil, err)
	if err != nil {
		return
	}
	defer d.DestroyShader(sh)
	pipe, err := d.CreatePipeline(gpu.PipelineDesc{Shader: sh, Primitive: gpu.PrimTriangles})
	r.check("array-pipeline", err == nil, err)
	if err != nil {
		return
	}
	defer d.DestroyPipeline(pipe)
	draw := func(label string) {
		f := d.BeginFrame(32, 32)
		p := f.BeginPass(gpu.PassDesc{Target: target, Color: gpu.LoadClear})
		p.SetPipeline(pipe)
		p.SetTexture(3, array, gpu.SamplerHandle{})
		p.Draw(0, 3)
		p.End()
		invalid := f.BeginPass(gpu.PassDesc{Target: target, TargetSubresource: gpu.TextureSubresource{Layer: 1}, Color: gpu.LoadClear, ClearColor: mathx.Color{R: 1, A: 1}})
		invalid.SetPipeline(pipe)
		invalid.Draw(0, 3)
		invalid.End()
		d.EndFrame()
		var got [4]byte
		err := d.ReadPixels(target, 0, 0, 1, 1, got[:])
		r.check(label+"-array-sampling", err == nil && got == [4]byte{0, 255, 0, 255}, fmt.Sprintf("%v %v", got, err))
		r.check(label+"-invalid-pass-discards-draw", f.Stats().DrawCalls == 1, f.Stats())
	}
	draw("before-loss")
	p := d.BeginFrame(32, 32).BeginPass(gpu.PassDesc{Target: target})
	p.SetTexture(3, array, gpu.SamplerHandle{})
	p.SetTexture(3, target, gpu.SamplerHandle{})
	p.SetTexture(3, gpu.TextureHandle{}, gpu.SamplerHandle{})
	p.End()
	d.EndFrame()
	ctx := canvas.Call("getContext", "webgl2")
	ctx.Call("activeTexture", ctx.Get("TEXTURE0").Int()+3)
	r.check("zero-texture-clears-all-targets", !ctx.Call("getParameter", ctx.Get("TEXTURE_BINDING_2D_ARRAY")).Truthy() && !ctx.Call("getParameter", ctx.Get("TEXTURE_BINDING_2D")).Truthy(), "stale 2D/array binding")
	// Real compile/link submission, deliberately left unconsumed until after
	// loss. The driver may finish before loss; publication is still pending.
	job, err := d.BeginShaderCompile(source())
	r.check("pending-compile-submitted", err == nil, err)
	if err != nil {
		return
	}
	defer job.Cancel()
	lostEvents, restoredEvents := 0, 0
	onLost := js.FuncOf(func(js.Value, []js.Value) any { lostEvents++; return nil })
	onRestore := js.FuncOf(func(js.Value, []js.Value) any { restoredEvents++; return nil })
	canvas.Call("addEventListener", "webglcontextlost", onLost)
	canvas.Call("addEventListener", "webglcontextrestored", onRestore)
	defer func() {
		canvas.Call("removeEventListener", "webglcontextlost", onLost)
		canvas.Call("removeEventListener", "webglcontextrestored", onRestore)
		onLost.Release()
		onRestore.Release()
	}()
	before := d.ContextGeneration()
	if !d.SimulateLoss(true) {
		r.check("WEBGL_lose_context", false, "extension unavailable")
		return
	}
	if !waitFor(func() bool { return d.Lost() && lostEvents == 1 }) {
		r.check("loss-event", false, "timed out")
		return
	}
	r.check("loss-event", true, nil)
	r.check("loss-clock-reset-once", clock.ConsumeFrameClockReset() && !clock.ConsumeFrameClockReset(), "lost event must rebase only the first ordinary frame")
	var px [4]byte
	r.check("lost-readback-rejected", d.ReadPixels(target, 0, 0, 1, 1, px[:]) != nil, "readback accepted on lost context")
	_, done, pollErr := job.Poll()
	r.check("old-generation-candidate-cancelled", done && pollErr != nil, fmt.Sprintf("done=%v err=%v", done, pollErr))
	err = d.UpdateTextureRegion(array, gpu.TextureUpdate{Subresource: gpu.TextureSubresource{Layer: 1, Mip: 2}, X: 1, Y: 0, Width: 1, Height: 1, Data: []byte{255, 0, 0, 255}})
	r.check("lost-context-keeps-cpu-mip-update", err == nil, err)
	d.SimulateLoss(false)
	if !waitFor(func() bool { return restoredEvents == 1 && d.Restores() == 1 && !d.Lost() }) {
		r.check("restore-event", false, "timed out")
		return
	}
	r.check("restore-event", d.ContextGeneration() > before, fmt.Sprintf("generation %d -> %d", before, d.ContextGeneration()))
	r.check("restore-clock-reset-once", clock.ConsumeFrameClockReset() && !clock.ConsumeFrameClockReset(), "restored event must rebase only the first ordinary frame")
	r.Restores = d.Restores()
	draw("after-loss")
	var got [8]byte
	err = d.ReadTexturePixels(array, gpu.TextureSubresource{Layer: 1, Mip: 1}, 0, 0, 2, 1, got[:])
	r.check("restore-manual-mip-and-generated-neighbour", err == nil && got == [8]byte{0, 255, 0, 255, 255, 255, 0, 255}, fmt.Sprintf("%v %v", got, err))
	err = d.ReadTexturePixels(array, gpu.TextureSubresource{Layer: 1, Mip: 2}, 0, 0, 2, 1, got[:])
	r.check("restore-update-made-during-loss", err == nil && got == [8]byte{0, 255, 0, 255, 255, 0, 0, 255}, fmt.Sprintf("%v %v", got, err))
	floatTarget(r, d, "restored")
	fresh, err := d.BeginShaderCompile(source())
	r.check("restored-compile-submitted", err == nil, err)
	if err == nil {
		defer fresh.Cancel()
		var shader gpu.ShaderHandle
		var complete bool
		var compileErr error
		ok := waitFor(func() bool { shader, complete, compileErr = fresh.Poll(); return complete })
		r.check("restored-compile-published", ok && compileErr == nil && !shader.IsZero(), compileErr)
		if !shader.IsZero() {
			d.DestroyShader(shader)
		}
	}
}

func main() {
	r := &report{}
	defer func() {
		if p := recover(); p != nil {
			r.Failures = append(r.Failures, fmt.Sprint(p))
		}
		data, _ := json.Marshal(r)
		js.Global().Set("aegoGPUResults", string(data))
	}()
	canvas := js.Global().Get("document").Call("getElementById", "surface")
	ctx := canvas.Call("getContext", "webgl2", map[string]any{"alpha": true, "premultipliedAlpha": true, "antialias": false, "preserveDrawingBuffer": true})
	if !ctx.Truthy() {
		r.check("WebGL2-context", false, "unavailable")
		return
	}
	r.FloatExtension = ctx.Call("getExtension", "EXT_color_buffer_float").Truthy()
	r.Parallel = ctx.Call("getExtension", "KHR_parallel_shader_compile").Truthy()
	if !r.Parallel {
		r.Unverified = append(r.Unverified, "real KHR_parallel_shader_compile branch (extension unavailable)")
	}
	d, err := webgl2.New(surface{ctx}, webgl2.Options{Debug: true})
	r.check("device-create", err == nil, err)
	if err != nil {
		return
	}
	defer d.Close()
	c := d.Caps()
	r.Renderer = c.Renderer
	r.FloatRenderable = c.RGBA16FRenderable
	r.MaxFragmentTextureUnits = c.MaxFragmentTextureUnits
	r.MaxArrayTextureLayers = c.MaxArrayTextureLayers
	r.check("fragment-unit-capability", c.MaxFragmentTextureUnits == ctx.Call("getParameter", ctx.Get("MAX_TEXTURE_IMAGE_UNITS")).Int(), c.MaxFragmentTextureUnits)
	r.check("array-layer-capability", c.MaxArrayTextureLayers == ctx.Call("getParameter", ctx.Get("MAX_ARRAY_TEXTURE_LAYERS")).Int(), c.MaxArrayTextureLayers)
	arrayLayers(r, d)
	legacyDepthSize(r, d)
	maskArray(r, d)
	floatTarget(r, d, "initial")
	additive(r, d)
	restore(r, d, canvas)
	r.check("released-resource-counts", d.ResourceStats() == (gpu.ResourceStats{}), d.ResourceStats())
	forcedFloatFailure(r, true)
	forcedFloatFailure(r, false)
}
