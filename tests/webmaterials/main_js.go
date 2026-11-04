//go:build js && wasm

package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"syscall/js"
	"time"

	"aego/asset"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/engine/res"
	"aego/gpu"
	"aego/gpu/webgl2"
	"aego/render"
	"aego/render/material"
	"aego/render/mesh"
	"aego/render/shader"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/render/view"
	"aego/tests/materialgolden"
	"aego/vfs"
)

//go:embed fixtures/*.aesh
var fixtures embed.FS

type report struct {
	Passed   []string          `json:"passed"`
	Failures []string          `json:"failures"`
	Renderer string            `json:"renderer"`
	Parallel bool              `json:"parallelShaderCompile"`
	Restores int               `json:"contextRestores"`
	Images   map[string]string `json:"imageSHA256"`
	PNG      map[string]string `json:"imagePNG"`
}

func (r *report) check(name string, ok bool, detail any) {
	if ok {
		r.Passed = append(r.Passed, name)
	} else {
		r.Failures = append(r.Failures, fmt.Sprintf("%s: %v", name, detail))
	}
}
func (r *report) image(name string, pixels []byte) {
	h := sha256.Sum256(pixels)
	r.Images[name] = hex.EncodeToString(h[:])
	if _, ok := materialgolden.Reference(name); ok {
		err := materialgolden.Compare(name, pixels, 2)
		r.check("golden-"+name, err == nil, err)
	}
	canvas := js.Global().Get("document").Call("createElement", "canvas")
	canvas.Set("width", 32)
	canvas.Set("height", 32)
	ctx := canvas.Call("getContext", "2d")
	im := ctx.Call("createImageData", 32, 32)
	js.CopyBytesToJS(im.Get("data"), pixels)
	ctx.Call("putImageData", im, 0, 0)
	r.PNG[name] = canvas.Call("toDataURL", "image/png").String()
}

type surface struct{ ctx js.Value }

func (s surface) WebGL2() js.Value { return s.ctx }

// The completion gate deterministically places context loss between submission
// of a REAL WebGL compile/link and the cache consuming its result, even on a fast
// driver. The driver, context events and recreation are not mocked.
type device struct {
	*webgl2.Device
	hold      bool
	submitted int
}
type job struct {
	gpu.ShaderCompilation
	d *device
}

func (j job) Poll() (gpu.ShaderHandle, bool, error) {
	if j.d.hold {
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

type rig struct {
	r      *render.Renderer
	d      *device
	target gpu.TextureHandle
	region texture.Region
	white  texture.Region
	pixels []byte
	errors []string
	tex    map[ids.AssetID]material.Tex
}

func newRig(ctx js.Value) (*rig, error) {
	d, err := webgl2.New(surface{ctx}, webgl2.Options{Debug: true, Log: log.Nop()})
	if err != nil {
		return nil, err
	}
	wrapped := &device{Device: d}
	files, err := vfs.New(vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true})
	if err != nil {
		return nil, err
	}
	r, err := render.New(wrapped, d.Caps(), files, log.Nop(), render.Options{MaxSprites: 64})
	if err != nil {
		return nil, err
	}
	target, err := d.CreateTexture(gpu.TextureDesc{Width: 32, Height: 32, Format: gpu.TexRGBA8, Label: "webmaterials.target"})
	if err != nil {
		return nil, err
	}
	g := &rig{r: r, d: wrapped, target: target, white: r.White(), pixels: make([]byte, 32*32*4), tex: map[ids.AssetID]material.Tex{}}
	r.Materials().OnError = func(sh *material.Shader, v int, err error) {
		g.errors = append(g.errors, fmt.Sprintf("%s/%d: %v", sh.Name, v, err))
	}
	data := texture.Solid(4, 4, mathx.Color{R: 1, A: 0.5})
	data.SetPixel(0, 0, mathx.Color{})
	h, err := r.Textures().Upload(data, texture.UploadOptions{})
	if err != nil {
		return nil, err
	}
	g.region = texture.Whole(h, 4, 4)
	return g, nil
}

func (g *rig) builtin(name string) *material.Material {
	mid, ok := asset.BuiltinMaterialNamed(name)
	if !ok {
		panic("unknown builtin " + name)
	}
	data, ok := asset.BuiltinMaterial(mid)
	if !ok {
		panic("missing builtin artifact")
	}
	doc, err := asset.DecodeMaterial(data, name)
	if err != nil {
		panic(err)
	}
	src, ok := asset.BuiltinShader(doc.Shader)
	if !ok {
		panic("missing builtin shader")
	}
	a, err := asset.DecodeShader(src)
	if err != nil {
		panic(err)
	}
	sh := a.Runtime(doc.Shader, name)
	m, probs := doc.Build(name, sh, func(id ids.AssetID) (material.Tex, bool) {
		if t, ok := g.tex[id]; ok {
			return t, true
		}
		d, ok := asset.BuiltinTexture(id)
		if !ok {
			return material.Tex{}, false
		}
		h, err := g.r.Textures().Upload(d, texture.UploadOptions{})
		if err != nil {
			return material.Tex{}, false
		}
		t := material.Tex{Texture: h}
		g.tex[id] = t
		return t, true
	})
	if m == nil {
		panic(probs)
	}
	m.ID = mid
	return m
}

func (g *rig) shader(name string) *material.Shader {
	data, err := fixtures.ReadFile("fixtures/" + name + ".aesh")
	if err != nil {
		panic(err)
	}
	a, err := asset.DecodeShader(data)
	if err != nil {
		panic(err)
	}
	return a.Runtime(ids.AssetID{}, name)
}
func (g *rig) material(name string) *material.Material {
	m, ok := material.New(name, g.shader(name), 0, shader.BlendPremultiplied)
	if !ok {
		panic("missing variant")
	}
	return m
}
func (g *rig) draw(v *render.ViewFrame, m *material.Material, lanes *[16]float32) {
	v.Sprite(sprite.Draw{Region: g.region, Position: mathx.Vec2{X: 16, Y: 16}, Size: mathx.Vec2{X: 24, Y: 24}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: m, Lanes: lanes})
}
func (g *rig) checker(v *render.ViewFrame) {
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			c := float32(.25)
			if (x+y)%2 != 0 {
				c = .75
			}
			v.Sprite(sprite.Draw{Region: g.white, Position: mathx.Vec2{X: float32(x * 8), Y: float32(y * 8)}, Size: mathx.Vec2{X: 8, Y: 8}, Tint: mathx.Color{R: c, G: c, B: c, A: 1}})
		}
	}
}
func (g *rig) frame(draw func(*render.ViewFrame)) []byte {
	f := g.d.BeginFrame(32, 32)
	if err := g.r.BeginFrame(f, 32, 32, 1); err != nil {
		panic(err)
	}
	v := view.ScreenView(32, 32, 1)
	v.Target = g.target
	v.DoClear = true
	vf := g.r.View(v)
	draw(vf)
	vf.End()
	g.r.EndFrame()
	g.d.EndFrame()
	if !g.d.ContextLost() {
		if err := g.d.ReadPixels(g.target, 0, 0, 32, 32, g.pixels); err != nil {
			panic(err)
		}
	}
	return g.pixels
}
func pixel(b []byte, x, y int) [4]byte { return [4]byte(b[(y*32+x)*4 : (y*32+x)*4+4]) }
func near(v [4]byte, w [4]int) bool {
	for i, n := range w {
		if int(v[i]) < n-2 || int(v[i]) > n+2 {
			return false
		}
	}
	return true
}
func wait(test func() bool, what string) {
	until := time.Now().Add(15 * time.Second)
	for !test() {
		if time.Now().After(until) {
			panic("timeout: " + what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The Go scheduler may resume the waiting test while the js.FuncOf lost-event
// callback is still on the JavaScript stack. restoreContext is permitted only
// after the browser finishes dispatching that event and observes preventDefault.
// A new JS task proves this boundary; a longer polling timeout would not.
func afterBrowserEvent() {
	done := make(chan struct{})
	callback := js.FuncOf(func(js.Value, []js.Value) any {
		close(done)
		return nil
	})
	defer callback.Release()
	js.Global().Call("setTimeout", callback, 0)
	<-done
}
func (g *rig) commit() {
	wait(func() bool { g.frame(func(*render.ViewFrame) {}); return g.r.Materials().Stats().Pending == 0 }, "material transaction")
}

func run(out *report) {
	canvas := js.Global().Get("document").Call("getElementById", "surface")
	ctx := canvas.Call("getContext", "webgl2", map[string]any{"alpha": true, "premultipliedAlpha": true, "antialias": false, "preserveDrawingBuffer": true})
	if !ctx.Truthy() {
		panic("WebGL2 unavailable")
	}
	out.Parallel = ctx.Call("getExtension", "KHR_parallel_shader_compile").Truthy()
	g, err := newRig(ctx)
	if err != nil {
		panic(err)
	}
	defer g.d.Close()
	defer g.r.Close()
	out.Renderer = g.d.Caps().Renderer
	builtins := map[string]*material.Material{}
	for _, name := range materialgolden.Stage12aBuiltins {
		m := g.builtin(name)
		builtins[name] = m
		img := g.frame(func(v *render.ViewFrame) { g.draw(v, m, nil) })
		out.check("builtin-"+name+"-links", len(g.errors) == 0, g.errors)
		out.check("builtin-"+name+"-transparent-hole", pixel(img, 6, 6) == [4]byte{}, pixel(img, 6, 6))
		out.image("builtin-"+name, img)
	}
	unlit := builtins["Unlit"]
	unlit.Set("Opacity", [4]float32{.5})
	img := g.frame(func(v *render.ViewFrame) { g.draw(v, unlit, nil) })
	out.check("premultiplied-opacity-once", near(pixel(img, 16, 16), [4]int{64, 0, 0, 64}), pixel(img, 16, 16))
	out.image("opacity", img)
	unlit.Reset("Opacity")
	img = g.frame(func(v *render.ViewFrame) { g.checker(v); g.draw(v, unlit, nil) })
	out.check("unlit-over-checkerboard", near(pixel(img, 16, 16), [4]int{160, 32, 32, 255}), pixel(img, 16, 16))
	out.image("checker-unlit", img)
	flash := builtins["Flash"]
	flash.Set("Flash", [4]float32{1})
	img = g.frame(func(v *render.ViewFrame) { g.draw(v, flash, nil) })
	out.check("flash-preserves-coverage", near(pixel(img, 16, 16), [4]int{128, 128, 128, 128}) && pixel(img, 6, 6) == [4]byte{}, pixel(img, 16, 16))
	out.image("flash", img)
	dissolve := builtins["Dissolve"]
	dissolve.Set("Threshold", [4]float32{1})
	img = g.frame(func(v *render.ViewFrame) { g.draw(v, dissolve, nil) })
	allZero := true
	for _, b := range img {
		if b != 0 {
			allZero = false
			break
		}
	}
	out.check("dissolve-one-is-fully-transparent", allZero, pixel(img, 16, 16))
	out.image("dissolved", img)
	dissolve.Reset("Threshold")
	img = g.frame(func(v *render.ViewFrame) { g.draw(v, dissolve, nil) })
	out.check("dissolve-zero-is-original", near(pixel(img, 16, 16), [4]int{128, 0, 0, 128}), pixel(img, 16, 16))
	add := builtins["Additive"]
	img = g.frame(func(v *render.ViewFrame) { g.draw(v, add, nil); g.draw(v, add, nil) })
	out.check("additive-preserves-background-coverage", near(pixel(img, 16, 16), [4]int{255, 0, 0, 0}), pixel(img, 16, 16))
	out.image("additive", img)
	outline := builtins["Outline"]
	img = g.frame(func(v *render.ViewFrame) { g.draw(v, outline, nil) })
	out.check("outline-is-interior", near(pixel(img, 6, 16), [4]int{0, 0, 0, 128}) && pixel(img, 3, 16) == [4]byte{}, pixel(img, 6, 16))
	typed := g.material("typed")
	lanes := *typed.DefaultLanes()
	p, _ := typed.Shader().Schema.Param("Code")
	material.PutLaneInt(&lanes, p, 16777217)
	p, _ = typed.Shader().Schema.Param("Enabled")
	material.PutLane(&lanes, p, [4]float32{1})
	p, _ = typed.Shader().Schema.Param("Amount")
	material.PutLane(&lanes, p, [4]float32{.5})
	img = g.frame(func(v *render.ViewFrame) {
		g.draw(v, typed, &lanes)
		v.Sprite(sprite.Draw{Region: g.region, Position: mathx.Vec2{X: 1, Y: 1}, Size: mathx.Vec2{X: 1, Y: 1}, Tint: mathx.White, Material: typed})
	})
	out.check("typed-int32-bool-and-std140", near(pixel(img, 16, 16), [4]int{0, 128, 0, 128}), pixel(img, 16, 16))
	out.check("overrides-do-not-split-batch", g.r.Stats().Batches == 1, g.r.Stats())
	geometryData := &mesh.Data{Vertices: []mesh.Vertex{{Pos: mathx.Vec3{X: 4, Y: 4}, Color: mesh.White}, {Pos: mathx.Vec3{X: 28, Y: 4}, Color: mesh.White}, {Pos: mathx.Vec3{X: 28, Y: 28}, Color: mesh.White}, {Pos: mathx.Vec3{X: 4, Y: 28}, Color: mesh.White}}, Indices: []uint32{0, 1, 2, 0, 2, 3}}
	geometryData.ComputeBounds()
	geometry, e := mesh.Upload(g.d, geometryData, "web-mesh")
	if e != nil {
		panic(e)
	}
	defer geometry.Destroy(g.d)
	meshMaterial := g.material("mesh-typed")
	meshLanes := *meshMaterial.DefaultLanes()
	mp, _ := meshMaterial.Shader().Schema.Param("Code")
	material.PutLaneInt(&meshLanes, mp, 16777217)
	img = g.frame(func(v *render.ViewFrame) {
		v.Mesh(mesh.Draw{Mesh: geometry, World: mathx.Mat4Identity(), Is2D: true, Material: meshMaterial, Color: mathx.White, Lanes: &meshLanes})
	})
	out.check("mesh2d-typed-override", near(pixel(img, 16, 16), [4]int{0, 128, 0, 128}), pixel(img, 16, 16))
	red := g.material("reload-red")
	img = g.frame(func(v *render.ViewFrame) {
		g.draw(v, red, nil)
		v.Mesh(mesh.Draw{Mesh: geometry, World: mathx.Mat4Identity(), Is2D: true, Material: meshMaterial, Color: mathx.White, Lanes: &meshLanes})
		g.draw(v, red, nil)
	})
	out.check("sprite-mesh-sprite-painter-order", near(pixel(img, 16, 16), [4]int{159, 64, 0, 223}), pixel(img, 16, 16))
	frame := g.d.BeginFrame(32, 32)
	if e := g.r.BeginFrame(frame, 32, 32, 1); e != nil {
		panic(e)
	}
	camera := view.ScreenView(32, 32, 1)
	camera.Target = g.target
	camera.DoClear = true
	camera.Clear = mathx.Color{R: 1, A: 1}
	g.r.View(camera).End()
	camera.Viewport = mathx.Rect{X: 20, Y: 20, W: 10, H: 10}
	camera.Clear = mathx.Color{G: 1, A: 1}
	g.r.View(camera).End()
	g.r.EndFrame()
	g.d.EndFrame()
	if e := g.d.ReadPixels(g.target, 0, 0, 32, 32, g.pixels); e != nil {
		panic(e)
	}
	out.check("camera-clear-preserves-other-viewport", near(pixel(g.pixels, 3, 3), [4]int{255, 0, 0, 255}) && near(pixel(g.pixels, 25, 25), [4]int{0, 255, 0, 255}), pixel(g.pixels, 3, 3))
	trimMaterial := g.material("trim")
	trimLanes := *trimMaterial.DefaultLanes()
	tp, _ := trimMaterial.Shader().Schema.Param("Code")
	material.PutLaneInt(&trimLanes, tp, 16777217)
	trimData := texture.Solid(5, 4, mathx.Color{G: 1, A: 1})
	for y := 1; y < 3; y++ {
		for x := 1; x < 4; x++ {
			trimData.SetPixel(x, y, mathx.White)
		}
	}
	trimTexture, err := g.r.Textures().Upload(trimData, texture.UploadOptions{})
	if err != nil {
		panic(err)
	}
	trimRegion := texture.Whole(trimTexture, 5, 4).Sub(1, 1, 3, 2)
	trimRegion.SourceW, trimRegion.SourceH, trimRegion.TrimX, trimRegion.TrimY = 8, 6, 2, 1
	for i, name := range []string{"trim-none", "trim-x", "trim-y", "trim-xy"} {
		img = g.frame(func(v *render.ViewFrame) {
			v.Sprite(sprite.Draw{Region: trimRegion, Position: mathx.Vec2{X: 16, Y: 16}, Size: mathx.Vec2{X: 16, Y: 12}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: trimMaterial, Lanes: &trimLanes, FlipX: i&1 != 0, FlipY: i&2 != 0})
		})
		out.image(name, img)
	}
	m := g.material("reload-red")
	m.Set("Amount", [4]float32{.25})
	draw := func(v *render.ViewFrame) { g.draw(v, m, nil) }
	img = g.frame(draw)
	before := pixel(img, 16, 16)
	old := m.Shader()
	g.r.Materials().QueueReload(old, g.shader("broken"), []*material.Material{m}, nil)
	g.commit()
	img = g.frame(draw)
	out.check("broken-driver-reload-keeps-last-good", m.Shader() == old && m.Failed() && pixel(img, 16, 16) == before, pixel(img, 16, 16))
	g.errors = nil
	next := g.shader("reload-green")
	g.d.hold = true
	g.r.Materials().QueueReload(old, next, []*material.Material{m}, nil)
	g.frame(draw)
	out.check("real-compile-awaits-commit", g.r.Materials().Stats().Pending == 1 && g.d.ResourceStats().PendingCompiles > 0, g.d.ResourceStats())
	if !g.d.SimulateLoss(true) {
		panic("WEBGL_lose_context unavailable")
	}
	wait(g.d.ContextLost, "webglcontextlost event")
	afterBrowserEvent()
	out.check("loss-keeps-cpu-last-good", m.Shader() == old, m.Shader().Name)
	// Poll the cache while lost so it observes cancellation of the old device.
	g.r.Materials().Begin()
	g.r.Materials().End()
	if !g.d.SimulateLoss(false) {
		panic("context restoration unavailable")
	}
	wait(func() bool { return !g.d.ContextLost() && g.d.Restores() > 0 }, "webglcontextrestored event")
	g.d.hold = false
	g.commit()
	img = g.frame(draw)
	out.check("restored-reload-commits-new-device", m.Shader() == next && !m.Failed() && near(pixel(img, 16, 16), [4]int{0, 64, 0, 64}), pixel(img, 16, 16))
	out.Restores = g.d.Restores()
	base := g.d.ResourceStats()
	for i := 0; i < 12; i++ {
		candidate := g.shader("reload-green")
		g.r.Materials().QueueReload(m.Shader(), candidate, []*material.Material{m}, nil)
		g.commit()
		g.frame(draw)
	}
	counts := g.d.ResourceStats()
	out.check("repeated-reload-releases-programs", counts.Shaders == base.Shaders && counts.Pipelines == base.Pipelines && counts.Buffers == base.Buffers && counts.PendingCompiles == 0, map[string]any{"before": base, "after": counts})
	img = g.frame(func(v *render.ViewFrame) { g.checker(v); g.draw(v, flash, nil) })
	out.check("restored-textures-and-lanes", near(pixel(img, 16, 16), [4]int{160, 160, 160, 255}), pixel(img, 16, 16))
	out.image("checker-flash-restored", img)
	img = g.frame(func(v *render.ViewFrame) {
		v.Sprite(sprite.Draw{Region: trimRegion, Position: mathx.Vec2{X: 16, Y: 16}, Size: mathx.Vec2{X: 16, Y: 12}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: trimMaterial, Lanes: &trimLanes, FlipX: true, FlipY: true})
	})
	out.image("trim-xy-restored", img)
	pixelSnap := builtins["PixelSnap"]
	pixelSnap.Set("GridPixels", [4]float32{4})
	pf := g.d.BeginFrame(32, 32)
	if e := g.r.BeginFrame(pf, 32, 32, 2); e != nil {
		panic(e)
	}
	pv := view.ScreenView(32, 32, 2)
	pv.Target = g.target
	pv.DoClear = true
	g.r.View(pv).End()
	pv.Viewport = mathx.Rect{X: 3, Y: 5, W: 24, H: 22}
	pv.DoClear = false
	pvf := g.r.View(pv)
	pvf.Sprite(sprite.Draw{Region: g.white, Position: mathx.Vec2{X: 5.2, Y: 6.2}, Size: mathx.Vec2{X: 4.6, Y: 4.6}, Tint: mathx.White, Material: pixelSnap})
	pvf.End()
	g.r.EndFrame()
	g.d.EndFrame()
	if e := g.d.ReadPixels(g.target, 0, 0, 32, 32, g.pixels); e != nil {
		panic(e)
	}
	out.image("pixelsnap-dpr2-offset", g.pixels)
	g.contractChecks(out)
	out.check("no-unexpected-driver-errors", len(g.errors) == 0, g.errors)
}

func main() {
	go func() {
		out := report{Images: map[string]string{}, PNG: map[string]string{}}
		defer func() {
			if p := recover(); p != nil {
				out.Failures = append(out.Failures, fmt.Sprint(p))
			}
			b, _ := json.Marshal(out)
			js.Global().Set("aegoMaterialResults", string(b))
		}()
		run(&out)
	}()
	select {}
}
