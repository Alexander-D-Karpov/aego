//go:build aego_gltest

package gl

import (
	"testing"

	"aego/core/mathx"
	"aego/gpu"
)

const depthSize = 32

const colorVert = `#version 330 core
layout(location = 0) in vec3 pos;
layout(location = 1) in vec4 col;
out vec4 v_col;
void main() {
	v_col = col;
	gl_Position = vec4(pos, 1.0);
}
`

const colorFrag = `#version 330 core
in vec4 v_col;
out vec4 frag;
void main() {
	frag = v_col;
}
`

type rig struct {
	t      *testing.T
	dev    gpu.Device
	shader gpu.ShaderHandle
}

func quad(z, r, g, b float32) []float32 {
	out := make([]float32, 0, 42)
	for _, p := range [6][2]float32{{-1, -1}, {1, -1}, {1, 1}, {-1, -1}, {1, 1}, {-1, 1}} {
		out = append(out, p[0], p[1], z, r, g, b, 1)
	}
	return out
}

func newRig(t *testing.T, dev gpu.Device) *rig {
	t.Helper()
	sh, err := dev.CreateShader(gpu.ShaderDesc{
		Label: "colored",
		Sources: []gpu.ShaderSource{
			{Stage: gpu.StageVertex, Source: colorVert},
			{Stage: gpu.StageFragment, Source: colorFrag},
		},
	})
	if err != nil {
		t.Fatalf("shader: %v", err)
	}
	return &rig{t: t, dev: dev, shader: sh}
}

func (r *rig) pipeline(depth gpu.DepthState, cull gpu.CullMode) gpu.PipelineHandle {
	r.t.Helper()
	p, err := r.dev.CreatePipeline(gpu.PipelineDesc{
		Shader: r.shader,
		Layout: gpu.VertexLayout{
			Attrs: []gpu.VertexAttr{
				{Location: 0, Slot: 0, Format: gpu.VFFloat3, Offset: 0},
				{Location: 1, Slot: 0, Format: gpu.VFFloat4, Offset: 12},
			},
			Strides: [gpu.MaxVertexSlots]uint16{28},
		},
		Depth:     depth,
		Cull:      cull,
		Primitive: gpu.PrimTriangles,
		Label:     "colored",
	})
	if err != nil {
		r.t.Fatalf("pipeline: %v", err)
	}
	return p
}

func (r *rig) buffer(data []float32) gpu.BufferHandle {
	r.t.Helper()
	b, err := r.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferVertex, Data: f32bytes(data...), Label: "quad"})
	if err != nil {
		r.t.Fatalf("buffer: %v", err)
	}
	return b
}

func (r *rig) texture(format gpu.TextureFormat, size int) gpu.TextureHandle {
	r.t.Helper()
	tex, err := r.dev.CreateTexture(gpu.TextureDesc{Width: size, Height: size, Format: format})
	if err != nil {
		r.t.Fatalf("texture: %v", err)
	}
	return tex
}

func (r *rig) draw(desc gpu.PassDesc, pipe gpu.PipelineHandle, bufs ...gpu.BufferHandle) {
	fr := r.dev.BeginFrame(depthSize, depthSize)
	p := fr.BeginPass(desc)
	p.SetViewport(mathx.Rect{W: depthSize, H: depthSize})
	p.SetPipeline(pipe)
	for _, b := range bufs {
		p.SetVertexBuffer(0, b)
		p.Draw(0, 6)
	}
	p.End()
	r.dev.EndFrame()
}

func (r *rig) center(tex gpu.TextureHandle) [4]byte {
	r.t.Helper()
	var out [4]byte
	if err := r.dev.ReadPixels(tex, depthSize/2, depthSize/2, 1, 1, out[:]); err != nil {
		r.t.Fatalf("read pixels: %v", err)
	}
	return out
}

func isGreen(px [4]byte) bool {
	return px[1] > 200 && px[0] < 50 && px[2] < 50
}

func isRed(px [4]byte) bool {
	return px[0] > 200 && px[1] < 50 && px[2] < 50
}

func isBlue(px [4]byte) bool {
	return px[2] > 200 && px[0] < 50 && px[1] < 50
}

func TestDepthTargetRejectsFartherFragments(t *testing.T) {
	e := newGLEngine(t)
	defer e.Close()
	r := newRig(t, e.GPU())
	pipe := r.pipeline(gpu.DepthState{Test: true, Write: true, Compare: gpu.CompareLess}, gpu.CullNone)
	near := r.buffer(quad(-0.5, 0, 1, 0))
	far := r.buffer(quad(0.5, 1, 0, 0))

	color := r.texture(gpu.TexRGBA8, depthSize)
	depth := r.texture(gpu.TexDepth24Stencil8, depthSize)
	r.draw(gpu.PassDesc{
		Target:      color,
		DepthTarget: depth,
		Color:       gpu.LoadClear,
		ClearColor:  mathx.Color{B: 1, A: 1},
		Depth:       gpu.LoadClear,
		ClearDepth:  1,
	}, pipe, near, far)
	if px := r.center(color); !isGreen(px) {
		t.Fatalf("depth test did not keep the nearer quad: %v", px)
	}

	plain := r.texture(gpu.TexRGBA8, depthSize)
	r.draw(gpu.PassDesc{
		Target:     plain,
		Color:      gpu.LoadClear,
		ClearColor: mathx.Color{B: 1, A: 1},
		Depth:      gpu.LoadClear,
		ClearDepth: 1,
	}, pipe, near, far)
	if px := r.center(plain); !isRed(px) {
		t.Fatalf("an offscreen pass without a depth target must not depth test: %v", px)
	}
}

func TestDepthTargetSizeMismatchIsIgnored(t *testing.T) {
	e := newGLEngine(t)
	defer e.Close()
	r := newRig(t, e.GPU())
	pipe := r.pipeline(gpu.DepthState{Test: true, Write: true, Compare: gpu.CompareLess}, gpu.CullNone)
	near := r.buffer(quad(-0.5, 0, 1, 0))
	far := r.buffer(quad(0.5, 1, 0, 0))
	color := r.texture(gpu.TexRGBA8, depthSize)
	depth := r.texture(gpu.TexDepth24Stencil8, depthSize*2)
	r.draw(gpu.PassDesc{
		Target:      color,
		DepthTarget: depth,
		Color:       gpu.LoadClear,
		Depth:       gpu.LoadClear,
		ClearDepth:  1,
	}, pipe, near, far)
	if px := r.center(color); !isRed(px) {
		t.Fatalf("a mismatched depth target must be dropped, not used: %v", px)
	}
}

func TestFlipWindingCullsMirroredFaces(t *testing.T) {
	e := newGLEngine(t)
	defer e.Close()
	r := newRig(t, e.GPU())
	pipe := r.pipeline(gpu.DepthState{}, gpu.CullBack)
	front := r.buffer(quad(0, 0, 1, 0))
	color := r.texture(gpu.TexRGBA8, depthSize)
	clear := mathx.Color{B: 1, A: 1}

	r.draw(gpu.PassDesc{Target: color, Color: gpu.LoadClear, ClearColor: clear}, pipe, front)
	if px := r.center(color); !isGreen(px) {
		t.Fatalf("a CCW quad must survive back-face culling: %v", px)
	}
	r.draw(gpu.PassDesc{Target: color, Color: gpu.LoadClear, ClearColor: clear, FlipWinding: true}, pipe, front)
	if px := r.center(color); !isBlue(px) {
		t.Fatalf("FlipWinding must turn the CCW quad into a back face: %v", px)
	}
	r.draw(gpu.PassDesc{Target: color, Color: gpu.LoadClear, ClearColor: clear}, pipe, front)
	if px := r.center(color); !isGreen(px) {
		t.Fatalf("front face state leaked into the next pass: %v", px)
	}
}
