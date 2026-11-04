// Package lightinggolden runs identical real-GPU scenes on GL33 and WebGL2.
// Expected images are computed from lighting, alpha and display contracts;
// driver screenshots never become their own acceptance reference.
package lightinggolden

import (
	"fmt"
	"runtime/debug"
	"slices"
	"strings"

	"aego/asset"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render"
	"aego/render/lighting"
	"aego/render/material"
	"aego/render/mesh"
	"aego/render/shader"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/render/view"
	"aego/vfs"
)

const Size = 32

type Report struct {
	Passed     []string                     `json:"passed"`
	Failures   []string                     `json:"failures"`
	Unverified []string                     `json:"unverified"`
	Images     map[string][]byte            `json:"-"`
	Stats      map[string]render.Stats      `json:"stats"`
	Outputs    map[string]render.OutputInfo `json:"outputs"`
}

func NewReport() *Report {
	return &Report{Images: make(map[string][]byte), Stats: make(map[string]render.Stats), Outputs: make(map[string]render.OutputInfo)}
}

func (r *Report) Check(name string, ok bool, detail any) {
	if ok {
		r.Passed = append(r.Passed, name)
	} else {
		r.Failures = append(r.Failures, fmt.Sprintf("%s: %v", name, detail))
	}
}

func (r *Report) Case(name string, fn func() error) {
	defer func() {
		if p := recover(); p != nil {
			detail := fmt.Sprintf("%s: %v", name, p)
			if len(r.Failures) == 0 {
				detail += "\n" + string(debug.Stack())
			}
			r.Failures = append(r.Failures, detail)
		}
	}()
	err := fn()
	r.Check(name, err == nil, err)
}

func (r *Report) Image(name string, got, want []byte, tolerance int) error {
	r.Images[name] = slices.Clone(got)
	return Compare(got, want, tolerance)
}

func Compare(got, want []byte, tolerance int) error {
	if len(got) != len(want) {
		return fmt.Errorf("image size %d, expected %d", len(got), len(want))
	}
	for i, v := range want {
		d := int(got[i]) - int(v)
		if d < -tolerance || d > tolerance {
			return fmt.Errorf("pixel(%d,%d) channel %d: got %d expected %d (tolerance %d)", (i/4)%Size, (i/4)/Size, i%4, got[i], v, tolerance)
		}
	}
	return nil
}

func Pixel(img []byte, x, y int) [4]byte { return [4]byte(img[(y*Size+x)*4 : (y*Size+x+1)*4]) }
func NearPixel(a [4]byte, b [4]int, tolerance int) bool {
	for i := range a {
		if int(a[i]) < b[i]-tolerance || int(a[i]) > b[i]+tolerance {
			return false
		}
	}
	return true
}

type Rig struct {
	Device                        gpu.Device
	Renderer                      *render.Renderer
	Target                        gpu.TextureHandle
	Lit, LitMesh, Unlit, Additive *material.Material
	Normal, Cookie                texture.Region
	Quad, DegenerateQuad          *mesh.Mesh
	owned                         []gpu.TextureHandle
	meshes                        []*mesh.Mesh
	Pixels                        []byte
	Errors                        []string
	AllowErrors                   bool
}

func New(dev gpu.Device, caps gpu.Capabilities, files vfs.FS) (*Rig, error) {
	r, err := render.New(dev, caps, files, log.Nop(), render.Options{MaxSprites: 128, MaxMeshes: 32})
	if err != nil {
		return nil, err
	}
	g := &Rig{Device: dev, Renderer: r, Pixels: make([]byte, Size*Size*4)}
	r.Materials().OnError = func(sh *material.Shader, v int, err error) {
		g.Errors = append(g.Errors, fmt.Sprintf("%s/%d: %v", sh.Name, v, err))
	}
	failed := true
	defer func() {
		if failed {
			g.Close()
		}
	}()
	g.Target, err = g.Texture(gpu.TextureDesc{Width: Size, Height: Size, Format: gpu.TexRGBA8, Label: "lighting-test.display"})
	if err != nil {
		return nil, err
	}
	for _, entry := range []struct {
		name string
		dst  **material.Material
	}{{"Lit", &g.Lit}, {"LitMesh2D", &g.LitMesh}, {"Unlit", &g.Unlit}, {"Additive", &g.Additive}} {
		*entry.dst, err = g.Builtin(entry.name)
		if err != nil {
			return nil, err
		}
	}
	h, err := g.Texture(gpu.TextureDesc{Width: 1, Height: 1, Format: gpu.TexRGBA8, Data: []byte{191, 223, 223, 255}, Label: "lighting-test.normal"})
	if err != nil {
		return nil, err
	}
	g.Normal = texture.Whole(h, 1, 1)
	h, err = g.Texture(gpu.TextureDesc{Width: 8, Height: 4, Format: gpu.TexR8, Data: []byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}, Label: "lighting-test.cookie"})
	if err != nil {
		return nil, err
	}
	g.Cookie = texture.Whole(h, 8, 4)
	g.Quad, err = g.QuadMesh(false)
	if err != nil {
		return nil, err
	}
	g.DegenerateQuad, err = g.QuadMesh(true)
	if err != nil {
		return nil, err
	}
	failed = false
	return g, nil
}

func (g *Rig) Texture(desc gpu.TextureDesc) (gpu.TextureHandle, error) {
	h, err := g.Device.CreateTexture(desc)
	if err == nil {
		g.owned = append(g.owned, h)
	}
	return h, err
}

func (g *Rig) Builtin(name string) (*material.Material, error) {
	id, ok := asset.BuiltinMaterialNamed(name)
	if !ok {
		return nil, fmt.Errorf("missing builtin %s", name)
	}
	data, ok := asset.BuiltinMaterial(id)
	if !ok {
		return nil, fmt.Errorf("missing builtin material artifact %s", name)
	}
	doc, err := asset.DecodeMaterial(data, name)
	if err != nil {
		return nil, err
	}
	data, ok = asset.BuiltinShader(doc.Shader)
	if !ok {
		return nil, fmt.Errorf("missing builtin shader artifact %s", name)
	}
	sh, err := asset.DecodeShader(data)
	if err != nil {
		return nil, err
	}
	m, issues := doc.Build(name, sh.Runtime(doc.Shader, name), func(id ids.AssetID) (material.Tex, bool) {
		d, ok := asset.BuiltinTexture(id)
		if !ok {
			return material.Tex{}, false
		}
		h, err := g.Renderer.Textures().Upload(d, texture.UploadOptions{})
		if err != nil {
			return material.Tex{}, false
		}
		g.owned = append(g.owned, h)
		return material.Tex{Texture: h}, true
	})
	if m == nil {
		return nil, fmt.Errorf("%s: %v", name, issues)
	}
	m.ID = id
	return m, nil
}

func (g *Rig) QuadMesh(degenerateUV bool) (*mesh.Mesh, error) {
	d := &mesh.Data{Vertices: []mesh.Vertex{
		{Pos: mathx.Vec3{X: -12, Y: -12}, Normal: mathx.Vec3{Z: 1}, UV: mathx.Vec2{}, Color: mesh.White},
		{Pos: mathx.Vec3{X: 12, Y: -12}, Normal: mathx.Vec3{Z: 1}, UV: mathx.Vec2{X: 1}, Color: mesh.White},
		{Pos: mathx.Vec3{X: 12, Y: 12}, Normal: mathx.Vec3{Z: 1}, UV: mathx.Vec2{X: 1, Y: 1}, Color: mesh.White},
		{Pos: mathx.Vec3{X: -12, Y: 12}, Normal: mathx.Vec3{Z: 1}, UV: mathx.Vec2{Y: 1}, Color: mesh.White},
	}, Indices: []uint32{0, 1, 2, 0, 2, 3}}
	if degenerateUV {
		for i := range d.Vertices {
			d.Vertices[i].UV = mathx.Vec2{X: .5, Y: .5}
		}
	}
	d.ComputeBounds()
	m, err := mesh.Upload(g.Device, d, "lighting-test.quad")
	if err == nil {
		g.meshes = append(g.meshes, m)
	}
	return m, err
}

func (g *Rig) View() view.View {
	v := view.ScreenView(Size, Size, 1)
	v.ID = view.ID{Scope: 1, Node: ids.NodeID{Lo: 1}}
	v.OutputID = view.ID{Scope: 1}
	v.Target = g.Target
	v.DoClear = true
	return v
}

func (g *Rig) Sprite(m *material.Material) sprite.Draw {
	return sprite.Draw{VisibilityLayers: ^uint64(0), Region: g.Renderer.White(), Position: mathx.Vec2{X: 16, Y: 16}, Size: mathx.Vec2{X: 24, Y: 24}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: m, Lighting: m != nil && m.Shader().Features&shader.FeatureLighting != 0}
}

func (g *Rig) Draw(in *lighting.Input, draw func(*render.ViewFrame)) ([]byte, error) {
	return g.DrawView(g.View(), in, draw)
}

func (g *Rig) DrawView(v view.View, in *lighting.Input, draw func(*render.ViewFrame)) ([]byte, error) {
	if err := g.Frame(v, in, draw); err != nil {
		return nil, err
	}
	if context, ok := g.Device.(gpu.ContextState); ok && context.ContextLost() {
		return nil, nil
	}
	if err := g.Device.ReadPixels(v.Target, 0, 0, Size, Size, g.Pixels); err != nil {
		return nil, err
	}
	return g.Pixels, nil
}

// Frame leaves readback to the caller for linear float attachments and composed
// camera outputs, whose contracts differ from the RGBA8 screenshot contract.
func (g *Rig) Frame(v view.View, in *lighting.Input, draw func(*render.ViewFrame)) error {
	before := len(g.Errors)
	f := g.Device.BeginFrame(Size, Size)
	if err := g.Renderer.BeginFrame(f, Size, Size, 1); err != nil {
		return err
	}
	vf := g.Renderer.View(v)
	vf.Lighting(in)
	draw(vf)
	vf.End()
	g.Renderer.EndFrame()
	g.Device.EndFrame()
	if !g.AllowErrors {
		if len(g.Errors) > before {
			return fmt.Errorf("driver compile/link: %s", strings.Join(g.Errors[before:], "; "))
		}
		if g.Renderer.Stats().LightingFailures > 0 {
			return fmt.Errorf("lighting: %v", g.Renderer.LightingError())
		}
	}
	return nil
}

func (g *Rig) Close() {
	if g == nil {
		return
	}
	for _, m := range g.meshes {
		m.Destroy(g.Device)
	}
	g.meshes = nil
	if g.Renderer != nil {
		g.Renderer.Close()
		g.Renderer = nil
	}
	for _, h := range g.owned {
		g.Device.DestroyTexture(h)
	}
	g.owned = nil
}

func CloneShader(source *material.Shader, name string, broken bool) *material.Shader {
	x := *source
	x.Name = name
	x.Variants = slices.Clone(source.Variants)
	if broken {
		for i := range x.Variants {
			x.Variants[i].Sources[1] += "\n#error intentional lighting reload test\n"
			x.Variants[i].Sources[3] += "\n#error intentional lighting reload test\n"
		}
	}
	return &x
}

func DarkEnvironment(mode lighting.ColorMode) lighting.Environment {
	e := lighting.DefaultEnvironment()
	e.AmbientIntensity = 0
	e.ColorMode = mode
	e.MaxShadowedLights = 3
	e.ShadowResolution = 256
	return e
}
func Input(mode lighting.ColorMode, lights ...lighting.Light) lighting.Input {
	return lighting.Input{Environment: DarkEnvironment(mode), HasEnvironment: true, Lights: lights}
}
func Light(kind lighting.Type) lighting.Light {
	return lighting.Light{ID: ids.NodeID{Lo: 1}, Type: kind, Position: mathx.Vec2{X: 16, Y: 16}, Direction: mathx.Vec2{X: 1}, Color: mathx.White, Intensity: .75, Range: 30, Height: 8, Elevation: mathx.Pi / 3, InnerAngle: mathx.Pi / 3, OuterAngle: mathx.Pi / 2, Falloff: 1, Layers: ^uint64(0), ShadowCasterMask: ^uint64(0)}
}
