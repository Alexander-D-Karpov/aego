//go:build js && wasm

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"syscall/js"
	"time"

	"aego/core/log"
	"aego/engine/res"
	"aego/gpu"
	"aego/gpu/webgl2"
	"aego/render"
	"aego/render/material"
	"aego/render/postfx"
	"aego/tests/lightinggolden"
	"aego/vfs"
)

type surface struct{ ctx js.Value }

func (s surface) WebGL2() js.Value { return s.ctx }

// This gate holds consumption of a genuine driver compile/link result. It
// deterministically exercises loss while a reload is pending even when this
// particular browser has no parallel compilation extension or links very fast.
type device struct {
	*webgl2.Device
	hold      bool
	submitted int
}
type job struct {
	gpu.ShaderCompilation
	owner *device
}

func (j job) Poll() (gpu.ShaderHandle, bool, error) {
	if j.owner.hold {
		return gpu.ShaderHandle{}, false, nil
	}
	return j.ShaderCompilation.Poll()
}
func (d *device) BeginShaderCompile(desc gpu.ShaderDesc) (gpu.ShaderCompilation, error) {
	inner, err := d.Device.BeginShaderCompile(desc)
	if err != nil {
		return nil, err
	}
	d.submitted++
	return job{inner, d}, nil
}

type report struct {
	*lightinggolden.Report
	Renderer string            `json:"renderer"`
	Parallel bool              `json:"parallelShaderCompile"`
	Float    bool              `json:"rgba16fRenderable"`
	Restores int               `json:"contextRestores"`
	PNG      map[string]string `json:"imagePNG"`
	Hashes   map[string]string `json:"imageSHA256"`
}

func wait(test func() bool, what string) error {
	deadline := time.Now().Add(15 * time.Second)
	for !test() {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout: %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}
func afterBrowserEvent() {
	done := make(chan struct{})
	callback := js.FuncOf(func(js.Value, []js.Value) any { close(done); return nil })
	defer callback.Release()
	js.Global().Call("setTimeout", callback, 0)
	<-done
}

func contextTests(d *device, files vfs.FS, out *report) {
	g, err := lightinggolden.New(d, d.Caps(), files)
	if err != nil {
		out.Failures = append(out.Failures, err.Error())
		return
	}
	defer g.Close()
	in, draw, expected := lightinggolden.RichScene(g)
	baseline, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(draw) })
	if err != nil {
		out.Failures = append(out.Failures, err.Error())
		return
	}
	baseline = slices.Clone(baseline)
	baselineResources := d.ResourceStats()
	if err = out.Image("before-context-loss", baseline, lightinggolden.Reference(in, [4]float64{}, expected), 2); err != nil {
		out.Failures = append(out.Failures, err.Error())
		return
	}
	loseAndRestore := func() error {
		generation, restores := d.ContextGeneration(), d.Restores()
		if !d.SimulateLoss(true) {
			return fmt.Errorf("unverified: WEBGL_lose_context is unavailable")
		}
		// Exercise the interval before the browser dispatches the loss event.
		// A live animation frame may request a material here.
		probe := gpu.ShaderDesc{Label: "lost-before-event", Sources: []gpu.ShaderSource{
			{Stage: gpu.StageVertex, Source: "#version 300 es\nvoid main(){gl_Position=vec4(0.0,0.0,0.0,1.0);}"},
			{Stage: gpu.StageFragment, Source: "#version 300 es\nprecision mediump float;out vec4 color;void main(){color=vec4(1.0);}"},
		}}
		if h, err := d.CreateShader(probe); err == nil || !h.IsZero() {
			return fmt.Errorf("shader creation unexpectedly succeeded during context loss")
		}
		if err := wait(d.ContextLost, "webglcontextlost event"); err != nil {
			return err
		}
		afterBrowserEvent()
		// Observe cancellation at the lost generation. Definitions remain CPU
		// objects; no GPU resource from this generation may be committed.
		g.Renderer.Materials().Begin()
		g.Renderer.Materials().End()
		if !d.SimulateLoss(false) {
			return fmt.Errorf("WEBGL_lose_context restoration unavailable")
		}
		if err := wait(func() bool { return !d.ContextLost() && d.Restores() > restores }, "webglcontextrestored event"); err != nil {
			return err
		}
		if d.ContextGeneration() == generation {
			return fmt.Errorf("restored device kept old generation")
		}
		out.Restores = d.Restores()
		return nil
	}
	out.Case("real-context-loss-restores-normal-cookie-shadow-HDR", func() error {
		if err := loseAndRestore(); err != nil {
			return err
		}
		got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(draw) })
		if err != nil {
			return err
		}
		if stats := g.Renderer.Stats(); stats.DirtyShadowMasks != 1 || stats.CookieCopies != 1 {
			return fmt.Errorf("lighting GPU resources were not rebuilt: %+v", stats)
		}
		return out.Image("after-context-restore", got, baseline, 2)
	})
	out.Case("real-context-loss-during-pending-Lit-reload", func() error {
		old := g.Lit.Shader()
		next := lightinggolden.CloneShader(old, "restored-lit-reload", false)
		submitted := d.submitted
		d.hold = true
		defer func() { d.hold = false }()
		g.Renderer.Materials().QueueReload(old, next, []*material.Material{g.Lit}, nil)
		if _, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(draw) }); err != nil {
			return err
		}
		if g.Renderer.Materials().Stats().Pending != 1 || d.ResourceStats().PendingCompiles == 0 || d.submitted <= submitted {
			return fmt.Errorf("no real compile in flight: %+v", d.ResourceStats())
		}
		if err := loseAndRestore(); err != nil {
			return err
		}
		if g.Lit.Shader() != old {
			return fmt.Errorf("candidate committed while old device was lost")
		}
		d.hold = false
		if err := g.WaitReload(&in, draw); err != nil {
			return err
		}
		if g.Lit.Shader() != next || g.Lit.Failed() {
			return fmt.Errorf("candidate did not relink/commit on restored device")
		}
		got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(draw) })
		if err != nil {
			return err
		}
		if d.ResourceStats().PendingCompiles != 0 {
			return fmt.Errorf("stale compile leaked after restoration")
		}
		return out.Image("after-pending-reload-context-restore", got, baseline, 2)
	})
	out.Case("stale-Lit-compile-cannot-replace-newer-candidate", func() error {
		old := g.Lit.Shader()
		stale := lightinggolden.CloneShader(old, "stale-lit", false)
		latest := lightinggolden.CloneShader(old, "newest-lit", false)
		d.hold = true
		defer func() { d.hold = false }()
		staleCommitted, newCommitted := false, false
		g.Renderer.Materials().QueueReload(old, stale, []*material.Material{g.Lit}, func() { staleCommitted = true })
		if _, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(draw) }); err != nil {
			return err
		}
		g.Renderer.Materials().QueueReload(old, latest, []*material.Material{g.Lit}, func() { newCommitted = true })
		d.hold = false
		if err := g.WaitReload(&in, draw); err != nil {
			return err
		}
		if staleCommitted || !newCommitted || g.Lit.Shader() != latest {
			return fmt.Errorf("stale result published: stale=%v latest=%v", staleCommitted, newCommitted)
		}
		return nil
	})
	out.Case("restored-lighting-resources-have-bounded-lifetime", func() error {
		// One draw retires the shader still used in the commit frame.
		if _, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(draw) }); err != nil {
			return err
		}
		if after := d.ResourceStats(); after != baselineResources {
			return fmt.Errorf("context restores grew resources: before=%+v after=%+v", baselineResources, after)
		}
		return nil
	})
	out.Case("postfx-chain-survives-context-restored", func() error {
		reg := postfx.NewRegistry()
		one, err := reg.NewInstance(postfx.VignetteID, "first")
		if err != nil {
			return err
		}
		two := one
		two.InstanceID = "second"
		var plan postfx.Plan
		if err = reg.Build(&plan, true, []postfx.Instance{one, two}, postfx.Profile{}); err != nil {
			return err
		}
		shot := func() ([]byte, error) {
			img, e := g.Draw(&in, func(f *render.ViewFrame) { f.PostProcess(&plan); f.Sprite(draw) })
			if e == nil {
				e = g.Renderer.PostProcessError()
			}
			if s := g.Renderer.Stats(); e == nil && (s.PostFXPasses != 2 || s.PostFXTargets != 1 || s.ResolvePasses != 1) {
				e = fmt.Errorf("unexpected post-process stack after draw: %+v", s)
			}
			return img, e
		}
		ref, err := shot()
		if err != nil {
			return err
		}
		ref = slices.Clone(ref)
		before := d.ResourceStats()
		if err = loseAndRestore(); err != nil {
			return err
		}
		actual, err := shot()
		if err != nil {
			return err
		}
		if after := d.ResourceStats(); after.Textures != before.Textures || after.Shaders != before.Shaders || after.Pipelines != before.Pipelines || after.Buffers != before.Buffers || after.Framebuffers > before.Framebuffers {
			return fmt.Errorf("post-process restore changed resource ownership: before=%+v after=%+v", before, after)
		}
		return out.Image("postfx-context-restore", actual, ref, 1)
	})
}

func run(out *report) {
	canvas := js.Global().Get("document").Call("getElementById", "surface")
	ctx := canvas.Call("getContext", "webgl2", map[string]any{"alpha": true, "premultipliedAlpha": true, "antialias": false, "preserveDrawingBuffer": true})
	if !ctx.Truthy() {
		out.Unverified = append(out.Unverified, "real WebGL2: context unavailable")
		panic("WebGL2 unavailable")
	}
	out.Parallel = ctx.Call("getExtension", "KHR_parallel_shader_compile").Truthy()
	if !out.Parallel {
		out.Unverified = append(out.Unverified, "actual KHR_parallel_shader_compile extension branch: unavailable on this browser/renderer; pending reload still uses a real compile with a completion gate")
	}
	driver, err := webgl2.New(surface{ctx}, webgl2.Options{Debug: true, Log: log.Nop()})
	if err != nil {
		panic(err)
	}
	defer driver.Close()
	d := &device{Device: driver}
	out.Renderer = d.Caps().Renderer
	out.Float = d.Caps().RGBA16FRenderable
	files, err := vfs.New(vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true})
	if err != nil {
		panic(err)
	}
	g, err := lightinggolden.New(d, d.Caps(), files)
	if err != nil {
		panic(err)
	}
	func() {
		defer g.Close()
		lightinggolden.Run(g, out.Report)
		lightinggolden.RunSurfaceResources(g, out.Report)
		lightinggolden.RunSelection(g, out.Report)
		lightinggolden.RunOutputs(g, out.Report)
		lightinggolden.RunPostProcess(g, out.Report)
		lightinggolden.RunExtendedPostProcess(g, out.Report)
	}()
	lightinggolden.RunFallbacks(d, files, out.Report)
	lightinggolden.RunLifetime(d, files, out.Report)
	contextTests(d, files, out)
}

func encodeImages(out *report) {
	out.PNG = make(map[string]string)
	out.Hashes = make(map[string]string)
	for name, pixels := range out.Images {
		h := sha256.Sum256(pixels)
		out.Hashes[name] = hex.EncodeToString(h[:])
		// ImageData is straight RGBA; test readback is premultiplied. Convert
		// only the downloadable preview, never the bytes used by assertions.
		straight := slices.Clone(pixels)
		for i := 0; i < len(straight); i += 4 {
			a := int(pixels[i+3])
			if a > 0 {
				for c := 0; c < 3; c++ {
					straight[i+c] = byte(min(255, (int(pixels[i+c])*255+a/2)/a))
				}
			}
		}
		canvas := js.Global().Get("document").Call("createElement", "canvas")
		canvas.Set("width", lightinggolden.Size)
		canvas.Set("height", lightinggolden.Size)
		ctx := canvas.Call("getContext", "2d")
		im := ctx.Call("createImageData", lightinggolden.Size, lightinggolden.Size)
		js.CopyBytesToJS(im.Get("data"), straight)
		ctx.Call("putImageData", im, 0, 0)
		out.PNG[name] = canvas.Call("toDataURL", "image/png").String()
	}
}
func main() {
	go func() {
		out := &report{Report: lightinggolden.NewReport()}
		defer func() {
			if p := recover(); p != nil {
				out.Failures = append(out.Failures, fmt.Sprint(p))
			}
			encodeImages(out)
			data, _ := json.Marshal(out)
			js.Global().Set("aegoLightingResults", string(data))
		}()
		run(out)
	}()
	select {}
}
