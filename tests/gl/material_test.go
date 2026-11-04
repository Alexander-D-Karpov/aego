//go:build aego_gltest

package gl

import (
	"io/fs"
	"strings"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/engine/res"
	"aego/gpu"
	"aego/render"
	"aego/render/material"
	"aego/render/mesh"
	"aego/render/shader"
	"aego/render/shaderx"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/render/view"
)

func runtimeShader(t *testing.T, name, src string) *material.Shader {
	t.Helper()
	u, diags := shaderx.Parse([]byte(src), name, func(path string) (string, []byte, error) {
		p := "shaders/include/" + strings.TrimPrefix(path, "engine/")
		data, err := fs.ReadFile(res.FS(), p)
		return "engine/" + p, data, err
	})
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	return asset.BuildShader(u).Runtime(ids.AssetID{}, name)
}

func materialRig(t *testing.T) (*render.Renderer, gpu.Device, gpu.TextureHandle) {
	t.Helper()
	e := newGLEngine(t)
	t.Cleanup(func() { _ = e.Close() })
	r, err := render.New(e.GPU(), e.Caps(), e.VFS(), log.Nop(), render.Options{MaxSprites: 64})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	r.Materials().OnError = func(sh *material.Shader, v int, err error) { t.Errorf("shader %s variant %d: %v", sh.Name, v, err) }
	target, err := e.GPU().CreateTexture(gpu.TextureDesc{Width: 32, Height: 32, Format: gpu.TexRGBA8, Label: "material-test"})
	if err != nil {
		t.Fatal(err)
	}
	return r, e.GPU(), target
}

func drawMaterialFrame(t *testing.T, r *render.Renderer, dev gpu.Device, target gpu.TextureHandle, draw func(*render.ViewFrame)) [4]byte {
	t.Helper()
	f := dev.BeginFrame(32, 32)
	if err := r.BeginFrame(f, 32, 32, 1); err != nil {
		t.Fatal(err)
	}
	v := view.ScreenView(32, 32, 1)
	v.Target = target
	v.DoClear = true
	vf := r.View(v)
	draw(vf)
	vf.End()
	r.EndFrame()
	dev.EndFrame()
	var out [4]byte
	if err := dev.ReadPixels(target, 16, 16, 1, 1, out[:]); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMaterialStd140MatchesDriverAndReadsEveryKind(t *testing.T) {
	r, dev, target := materialRig(t)
	sh := runtimeShader(t, "layout", `#pragma aego abi 1
#pragma material
uniform float Head; // @default(0.25)
uniform vec3 Three; // @default(0.1,0.2,0.3)
uniform float Tail; // @default(0.125)
uniform vec2 Two; // @default(0.25,0.5)
uniform bool Enabled; // @default(1)
uniform vec4 Four; // @default(0.2,0.3,0.4,0.5)
uniform int Counter; // @default(16777217)
#pragma end
void fragment(){
    bool good = abs(Head-0.25)<0.001 && distance(Three,vec3(0.1,0.2,0.3))<0.001
      && abs(Tail-0.125)<0.001 && distance(Two,vec2(0.25,0.5))<0.001
      && Enabled && distance(Four,vec4(0.2,0.3,0.4,0.5))<0.001 && Counter==16777217;
    aego_out=good?vec4(0.0,0.5,0.0,0.5):vec4(1.0,0.0,0.0,1.0);
}`)
	h, err := dev.CreateShader(gpu.ShaderDesc{Label: "layout-query", Sources: []gpu.ShaderSource{{Stage: gpu.StageVertex, Source: sh.Variants[0].Sources[0]}, {Stage: gpu.StageFragment, Source: sh.Variants[0].Sources[1]}}})
	if err != nil {
		t.Fatal(err)
	}
	defer dev.DestroyShader(h)
	inspector, ok := dev.(gpu.UniformBlockInspector)
	if !ok {
		t.Fatal("real backend has no layout inspector")
	}
	layout, ok, err := inspector.UniformBlockLayout(h, shader.SlotMaterial)
	if err != nil || !ok {
		t.Fatalf("layout %v %v", ok, err)
	}
	want := []int{0, 16, 28, 32, 40, 48, 64}
	if layout.Size != 80 || sh.Schema.Size != 80 {
		t.Fatalf("block size driver=%d CPU=%d", layout.Size, sh.Schema.Size)
	}
	if len(layout.Members) != len(want) {
		t.Fatalf("active fields %d", len(layout.Members))
	}
	for i, p := range sh.Schema.Params {
		if p.Offset != want[i] {
			t.Fatalf("CPU %s offset=%d", p.Name, p.Offset)
		}
		found := false
		for _, m := range layout.Members {
			if strings.TrimPrefix(m.Name, "ub2.") == p.Name {
				found = true
				if m.Offset != want[i] || m.Type != p.Kind.GLSL() {
					t.Fatalf("driver member %+v", m)
				}
			}
		}
		if !found {
			t.Fatalf("missing active %s", p.Name)
		}
	}
	m, _ := material.New("m", sh, 0, shader.BlendPremultiplied)
	px := drawMaterialFrame(t, r, dev, target, func(v *render.ViewFrame) {
		v.Sprite(sprite.Draw{Region: r.White(), Position: mathx.Vec2{X: 16, Y: 16}, Size: mathx.Vec2{X: 24, Y: 24}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: m})
	})
	if px[0] != 0 || px[1] < 127 || px[1] > 129 || px[3] < 127 || px[3] > 129 {
		t.Fatalf("typed UBO/transparent alpha mismatch: %v", px)
	}
}

func TestIntegerOverridesAreTypedAndDoNotSplitBatch(t *testing.T) {
	r, dev, target := materialRig(t)
	sh := runtimeShader(t, "typed-instance", `#pragma aego abi 1
#pragma material
uniform int Code; // @instance @default(1)
uniform bool On; // @instance @default(0)
uniform float Amount; // @instance @default(0.25)
#pragma end
void fragment(){
    bool good=Code==16777217 && On && abs(Amount-0.5)<0.001;
    aego_out=good?vec4(0.0,0.5,0.0,0.5):vec4(1.0,0.0,0.0,1.0);
}`)
	m, _ := material.New("m", sh, 0, shader.BlendPremultiplied)
	lanes := *m.DefaultLanes()
	p, _ := sh.Schema.Param("Code")
	material.PutLaneInt(&lanes, p, 16777217)
	p, _ = sh.Schema.Param("On")
	material.PutLane(&lanes, p, [4]float32{1})
	p, _ = sh.Schema.Param("Amount")
	material.PutLane(&lanes, p, [4]float32{.5})
	px := drawMaterialFrame(t, r, dev, target, func(v *render.ViewFrame) {
		v.Sprite(sprite.Draw{Region: r.White(), Position: mathx.Vec2{X: 16, Y: 16}, Size: mathx.Vec2{X: 24, Y: 24}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: m, Lanes: &lanes})
		v.Sprite(sprite.Draw{Region: r.White(), Position: mathx.Vec2{X: 2, Y: 2}, Size: mathx.Vec2{X: 1, Y: 1}, Tint: mathx.White, Material: m})
	})
	if px[0] != 0 || px[1] < 127 || px[1] > 129 || px[3] < 127 || px[3] > 129 {
		t.Fatalf("integer/bool instance or alpha mismatch: %v", px)
	}
	if r.Stats().Batches != 1 {
		t.Fatalf("overrides split batch: %+v", r.Stats())
	}
	if n, _ := m.GetInt("Code"); n != 1 {
		t.Fatal("node override changed material")
	}
}

func TestSpriteMeshSpritePainterOrderOnTransparentTarget(t *testing.T) {
	r, dev, target := materialRig(t)
	a := runtimeShader(t, "red-sprite", `#pragma aego abi 1
void fragment(){aego_out=vec4(0.5,0.0,0.0,0.5);}`)
	b := runtimeShader(t, "green-mesh", `#pragma aego abi 1
#pragma domain mesh2d
void fragment(){aego_out=vec4(0.0,0.5,0.0,0.5);}`)
	ma, _ := material.New("a", a, 0, shader.BlendPremultiplied)
	mb, _ := material.New("b", b, 0, shader.BlendPremultiplied)
	geo := &mesh.Data{Vertices: []mesh.Vertex{{Pos: mathx.Vec3{X: 4, Y: 4}, Color: mesh.White}, {Pos: mathx.Vec3{X: 28, Y: 4}, Color: mesh.White}, {Pos: mathx.Vec3{X: 28, Y: 28}, Color: mesh.White}, {Pos: mathx.Vec3{X: 4, Y: 28}, Color: mesh.White}}, Indices: []uint32{0, 1, 2, 0, 2, 3}}
	geo.ComputeBounds()
	m, err := mesh.Upload(dev, geo, "quad2d")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Destroy(dev)
	px := drawMaterialFrame(t, r, dev, target, func(v *render.ViewFrame) {
		d := sprite.Draw{Region: r.White(), Position: mathx.Vec2{X: 16, Y: 16}, Size: mathx.Vec2{X: 24, Y: 24}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: ma}
		v.Sprite(d)
		v.Mesh(mesh.Draw{Mesh: m, World: mathx.Mat4Identity(), Is2D: true, Material: mb, Color: mathx.White})
		v.Sprite(d)
	})
	want := [4]int{159, 64, 0, 223}
	for i, w := range want {
		if int(px[i]) < w-2 || int(px[i]) > w+2 {
			t.Fatalf("A/Mesh/A reordered or alpha wrong: got %v want approximately %v", px, want)
		}
	}
}

func TestAtlasSampleCannotReadNeighbour(t *testing.T) {
	r, dev, target := materialRig(t)
	sh := runtimeShader(t, "atlas-safe", `#pragma aego abi 1
#include "engine/sprite.glsl"
void fragment(){aego_out=aego_sample_local(aego_local+vec2(0.75,0.0));}`)
	m, _ := material.New("m", sh, 0, shader.BlendPremultiplied)
	d := texture.New(4, 2, gpu.TexRGBA8)
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			c := mathx.Color{R: 1, A: 1}
			if x >= 2 {
				c = mathx.Color{G: 1, A: 1}
			}
			d.SetPixel(x, y, c)
		}
	}
	h, err := r.Textures().Upload(d, texture.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	region := texture.Whole(h, 4, 2).Sub(0, 0, 2, 2)
	px := drawMaterialFrame(t, r, dev, target, func(v *render.ViewFrame) {
		v.Sprite(sprite.Draw{Region: region, Position: mathx.Vec2{X: 16, Y: 16}, Size: mathx.Vec2{X: 24, Y: 24}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: m})
	})
	if px != [4]byte{} {
		t.Fatalf("atlas neighbour leaked through region boundary: %v", px)
	}
}

func TestRealReloadKeepsLastGoodRebindsAndReleasesGenerations(t *testing.T) {
	r, dev, target := materialRig(t)
	const original = "#pragma aego abi 1\n#pragma material\nuniform float Amount; // @default(0.5)\n#pragma end\nvoid fragment(){aego_out=vec4(Amount,0.0,0.0,Amount);}\n"
	sh := runtimeShader(t, "reload", original)
	m, _ := material.New("m", sh, 0, shader.BlendPremultiplied)
	m.Set("Amount", [4]float32{0.25})
	draw := func(v *render.ViewFrame) {
		v.Sprite(sprite.Draw{Region: r.White(), Position: mathx.Vec2{X: 16, Y: 16}, Size: mathx.Vec2{X: 24, Y: 24}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: m})
	}
	first := drawMaterialFrame(t, r, dev, target, draw)
	failures := 0
	r.Materials().OnError = func(*material.Shader, int, error) { failures++ }
	bad := runtimeShader(t, "reload-broken", "#pragma aego abi 1\nvoid fragment(){aego_out=undefined_name;}\n")
	r.Materials().QueueReload(sh, bad, []*material.Material{m}, nil)
	beforeCommit := drawMaterialFrame(t, r, dev, target, draw)
	afterFailure := drawMaterialFrame(t, r, dev, target, draw)
	if failures != 1 || !m.Failed() || m.Shader() != sh || first != beforeCommit || first != afterFailure {
		t.Fatalf("last-good was lost failures=%d shader=%s before=%v after=%v", failures, m.Shader().Name, first, afterFailure)
	}
	reporter, ok := dev.(gpu.ResourceReporter)
	if !ok {
		t.Fatal("backend has no resource counts")
	}
	baseline := reporter.ResourceStats()
	for i := 0; i < 16; i++ {
		next := runtimeShader(t, "reload-fixed", `#pragma aego abi 1
#pragma material
uniform vec3 Padding; // @default(0.0,0.0,0.0)
uniform float Amount; // @default(0.9)
#pragma end
void fragment(){aego_out=vec4(Padding.x,Amount,Padding.z,Amount);}`)
		r.Materials().QueueReload(m.Shader(), next, []*material.Material{m}, nil)
		drawMaterialFrame(t, r, dev, target, draw)
		px := drawMaterialFrame(t, r, dev, target, draw)
		if m.Failed() || m.Shader() != next || px[0] != 0 || px[1] < 63 || px[1] > 65 || px[3] < 63 || px[3] > 65 {
			t.Fatalf("relink %d did not preserve/rebind typed block: %v", i, px)
		}
		got := reporter.ResourceStats()
		if got.Shaders != baseline.Shaders || got.Pipelines != baseline.Pipelines || got.Buffers != baseline.Buffers {
			t.Fatalf("reload %d leaks GPU objects: base=%+v current=%+v", i, baseline, got)
		}
	}
}

func TestMesh2DTypedOverridesUseDetachedLaneStorage(t *testing.T) {
	r, dev, target := materialRig(t)
	sh := runtimeShader(t, "mesh-typed-instance", `#pragma aego abi 1
#pragma domain mesh2d
#pragma material
uniform int Code; // @instance @default(1)
uniform bool Enabled; // @instance @default(0)
uniform float Amount; // @instance @default(0.25)
#pragma end
void fragment(){
    bool good=Code==16777217 && Enabled && abs(Amount-0.5)<0.001;
    aego_out=good?vec4(0.0,0.5,0.0,0.5):vec4(1.0,0.0,0.0,1.0);
}`)
	m, _ := material.New("typed-mesh", sh, 0, shader.BlendPremultiplied)
	lanes := *m.DefaultLanes()
	p, _ := sh.Schema.Param("Code")
	material.PutLaneInt(&lanes, p, 16777217)
	p, _ = sh.Schema.Param("Enabled")
	material.PutLane(&lanes, p, [4]float32{1})
	p, _ = sh.Schema.Param("Amount")
	material.PutLane(&lanes, p, [4]float32{.5})
	geo := &mesh.Data{Vertices: []mesh.Vertex{
		{Pos: mathx.Vec3{X: 4, Y: 4}, Color: mesh.White},
		{Pos: mathx.Vec3{X: 28, Y: 4}, Color: mesh.White},
		{Pos: mathx.Vec3{X: 28, Y: 28}, Color: mesh.White},
		{Pos: mathx.Vec3{X: 4, Y: 28}, Color: mesh.White},
	}, Indices: []uint32{0, 1, 2, 0, 2, 3}}
	geo.ComputeBounds()
	geometry, err := mesh.Upload(dev, geo, "typed-quad")
	if err != nil {
		t.Fatal(err)
	}
	defer geometry.Destroy(dev)
	px := drawMaterialFrame(t, r, dev, target, func(v *render.ViewFrame) {
		v.Mesh(mesh.Draw{Mesh: geometry, World: mathx.Mat4Identity(), Is2D: true, Material: m, Color: mathx.White, Lanes: &lanes})
	})
	if px[0] != 0 || px[1] < 127 || px[1] > 129 || px[3] < 127 || px[3] > 129 {
		t.Fatalf("Mesh2D typed instance payload mismatch: %v", px)
	}
	if got, _ := m.GetInt("Code"); got != 1 {
		t.Fatal("mesh override changed shared material")
	}
}

func TestSecondCameraClearPreservesFirstCamera(t *testing.T) {
	r, dev, target := materialRig(t)
	f := dev.BeginFrame(32, 32)
	if err := r.BeginFrame(f, 32, 32, 1); err != nil {
		t.Fatal(err)
	}
	first := view.ScreenView(32, 32, 1)
	first.Target, first.DoClear, first.Clear = target, true, mathx.Color{R: 1, A: 1}
	r.View(first).End()
	inset := first
	inset.Viewport = mathx.Rect{X: 20, Y: 20, W: 10, H: 10}
	inset.Clear = mathx.Color{G: 1, A: 1}
	r.View(inset).End()
	r.EndFrame()
	dev.EndFrame()
	var outside, inside [4]byte
	if err := dev.ReadPixels(target, 3, 3, 1, 1, outside[:]); err != nil {
		t.Fatal(err)
	}
	if err := dev.ReadPixels(target, 25, 25, 1, 1, inside[:]); err != nil {
		t.Fatal(err)
	}
	if !isRed(outside) || !isGreen(inside) {
		t.Fatalf("second view cleared another viewport: outside=%v inside=%v", outside, inside)
	}
}
