//go:build aego_gltest

// Package gl holds tests that need a real OpenGL context. They are excluded
// from the default build; run them with `make test-gl`, which points SDL at the
// offscreen video driver and Mesa at llvmpipe so results are deterministic and
// no display server is required.
package gl

import (
	"math"
	"runtime"
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
	runtime.LockOSThread()
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
//     own upper half, which is memory rows H/2..H.
//   - ReadPixels returns the target's memory rows in order. Render targets are
//     stored top-down (render/view compensates Y), so row 0 is the top of
//     anything drawn through the renderer, and a raw draw into GL's upper half
//     comes back in the bottom rows.
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
	if !(rt < 50 && bt > 200) || !(rb > 200 && bb < 50) {
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

// TestUpdateTextureKeepsItsScratchUnitActive covers the case where the texture
// is already cached on the scratch unit, but another unit was activated since
// the previous update. Target-based GL calls such as TexSubImage2D operate on
// the active unit, so skipping both ActiveTexture and BindTexture would update
// the texture on unit 0 instead.
func TestUpdateTextureKeepsItsScratchUnitActive(t *testing.T) {
	e := newGLEngine(t)
	defer e.Close()
	dev := e.GPU()

	const size = 4
	fill := func(r, g, b byte) []byte {
		out := make([]byte, size*size*4)
		for i := 0; i < len(out); i += 4 {
			out[i], out[i+1], out[i+2], out[i+3] = r, g, b, 255
		}
		return out
	}
	a, err := dev.CreateTexture(gpu.TextureDesc{
		Width: size, Height: size, Format: gpu.TexRGBA8, Data: fill(255, 0, 0), Label: "update-unit-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := dev.CreateTexture(gpu.TextureDesc{
		Width: size, Height: size, Format: gpu.TexRGBA8, Data: fill(0, 0, 255), Label: "update-unit-b",
	})
	if err != nil {
		t.Fatal(err)
	}

	// B is now cached on the scratch unit. Make unit 0 active without changing
	// that cached scratch binding, matching the state left by a rendered frame.
	fr := dev.BeginFrame(size, size)
	p := fr.BeginPass(gpu.PassDesc{Color: gpu.LoadKeep, Label: "activate-unit-zero"})
	p.SetTexture(0, a, gpu.SamplerHandle{})
	p.End()
	dev.EndFrame()

	dev.UpdateTexture(b, 0, 0, size, size, fill(0, 255, 0))

	check := func(tex gpu.TextureHandle, want [4]byte) {
		t.Helper()
		out := make([]byte, size*size*4)
		if err := dev.ReadPixels(tex, 0, 0, size, size, out); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < len(out); i += 4 {
			if got := [4]byte(out[i : i+4]); got != want {
				t.Fatalf("pixel %d: rgba=%v, want %v", i/4, got, want)
			}
		}
	}
	check(a, [4]byte{255, 0, 0, 255})
	check(b, [4]byte{0, 255, 0, 255})
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
