//go:build aego_gltest

// Package gl holds tests that need a real OpenGL context. They are excluded
// from the default build; run them with `make test-gl`, which points SDL at the
// offscreen video driver and Mesa at llvmpipe so results are deterministic and
// no display server is required.
package gl

import (
	"math"
	"testing"

	"aego/core/log"
	"aego/core/mathx"
	"aego/engine"
	"aego/gpu"
)

const vertSrc = `#version 330 core
layout(location = 0) in vec2 pos;
void main() { gl_Position = vec4(pos, 0.0, 1.0); }
`

const fragSrc = `#version 330 core
out vec4 frag;
void main() { frag = vec4(1.0, 0.0, 0.0, 1.0); }
`

func newGLEngine(t *testing.T) *engine.Engine {
	t.Helper()
	e, err := engine.NewBuilder(engine.Config{
		Title: "aego-gltest", Width: 64, Height: 64, Hidden: true,
		ResRoot: t.TempDir(), UserRoot: t.TempDir(), AppID: "aego-gltest",
	}).WithLogger(log.Nop()).Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if e.Caps().Renderer == "null" {
		e.Close()
		t.Skip("no OpenGL context available")
	}
	return e
}

// TestReadPixelsOrientation pins two contracts at once:
//
//   - D17: gl33 flips viewport and scissor only for the window, never for an
//     offscreen target. A target-space viewport at y=H/2..H is therefore GL's
//     own upper half.
//   - ReadPixels always returns rows top-down, whatever the backend's origin.
//
// If either regresses the image comes back mirrored, which is precisely the
// failure that would silently invalidate every Stage 2 golden.
func TestReadPixelsOrientation(t *testing.T) {
	e := newGLEngine(t)
	defer e.Close()
	dev := e.GPU()

	if !dev.Caps().OriginBottomLeft {
		t.Skip("backend is already top-down; this test targets bottom-left origins")
	}

	const size = 64
	target, err := dev.CreateTexture(gpu.TextureDesc{
		Width: size, Height: size, Format: gpu.TexRGBA8, Label: "target",
	})
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	sh, err := dev.CreateShader(gpu.ShaderDesc{
		Label: "flat",
		Sources: []gpu.ShaderSource{
			{Stage: gpu.StageVertex, Source: vertSrc},
			{Stage: gpu.StageFragment, Source: fragSrc},
		},
	})
	if err != nil {
		t.Fatalf("shader: %v", err)
	}
	pipe, err := dev.CreatePipeline(gpu.PipelineDesc{
		Shader: sh,
		Layout: gpu.VertexLayout{
			Attrs:   []gpu.VertexAttr{{Location: 0, Slot: 0, Format: gpu.VFFloat2, Offset: 0}},
			Strides: [gpu.MaxVertexSlots]uint16{8},
		},
		Primitive: gpu.PrimTriangles,
		Label:     "flat",
	})
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	vb, err := dev.CreateBuffer(gpu.BufferDesc{
		Usage: gpu.BufferVertex,
		Data:  f32bytes(-1, -1, 1, -1, 1, 1, -1, -1, 1, 1, -1, 1),
		Label: "vb",
	})
	if err != nil {
		t.Fatalf("vbuf: %v", err)
	}

	fr := dev.BeginFrame(size, size)
	p := fr.BeginPass(gpu.PassDesc{
		Color:      gpu.LoadClear,
		ClearColor: mathx.Color{R: 0, G: 0, B: 1, A: 1},
		Target:     target,
		Label:      "orientation",
	})
	p.SetViewport(mathx.Rect{X: 0, Y: size / 2, W: size, H: size / 2})
	p.SetPipeline(pipe)
	p.SetVertexBuffer(0, vb)
	p.Draw(0, 6)
	p.End()
	dev.EndFrame()

	out := make([]byte, size*size*4)
	if err := dev.ReadPixels(target, 0, 0, size, size, out); err != nil {
		t.Fatalf("read pixels: %v", err)
	}
	row := func(y int) (r, g, b, a byte) {
		i := y * size * 4
		return out[i], out[i+1], out[i+2], out[i+3]
	}
	rt, _, bt, at := row(0)
	rb, _, bb, ab := row(size - 1)
	if at != 255 || ab != 255 {
		t.Fatalf("alpha not opaque: top=%d bottom=%d", at, ab)
	}
	if !(rt > 200 && bt < 50) || !(rb < 50 && bb > 200) {
		t.Fatalf("image is mirrored or wrong: top=(%d,_,%d) bottom=(%d,_,%d)", rt, bt, rb, bb)
	}
}

func TestReadPixelsRejectsBadArgs(t *testing.T) {
	e := newGLEngine(t)
	defer e.Close()
	dev := e.GPU()

	tex, err := dev.CreateTexture(gpu.TextureDesc{Width: 8, Height: 8, Format: gpu.TexRGBA8})
	if err != nil {
		t.Fatal(err)
	}
	big := make([]byte, 8*8*4)
	if err := dev.ReadPixels(tex, 0, 0, 16, 16, big); err == nil {
		t.Fatal("out-of-range region accepted")
	}
	if err := dev.ReadPixels(tex, 0, 0, 8, 8, big[:4]); err == nil {
		t.Fatal("undersized destination accepted")
	}
	if err := dev.ReadPixels(gpu.TextureHandle{}, 0, 0, 8, 8, big); err == nil {
		t.Fatal("invalid handle accepted")
	}
}

func f32bytes(vals ...float32) []byte {
	out := make([]byte, len(vals)*4)
	for i, v := range vals {
		b := math.Float32bits(v)
		out[i*4+0], out[i*4+1] = byte(b), byte(b>>8)
		out[i*4+2], out[i*4+3] = byte(b>>16), byte(b>>24)
	}
	return out
}
