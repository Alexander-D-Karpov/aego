package quad

import (
	"encoding/binary"
	"math"

	"aego/core/mathx"
	"aego/gpu"
	"aego/vfs"
)

const (
	uniformSize = 80
	checkerSize = 64
	checkerCell = 8
)

type Renderer struct {
	dev    gpu.Device
	shader gpu.ShaderHandle
	pipe   gpu.PipelineHandle
	vb     gpu.BufferHandle
	ib     gpu.BufferHandle
	ub     gpu.BufferHandle
	tex    gpu.TextureHandle
	smp    gpu.SamplerHandle
	ubo    []byte
}

func New(dev gpu.Device, files vfs.FS, uvScale float32) (*Renderer, error) {
	vsrc, err := files.ReadFile("res://shaders/basic.vert")
	if err != nil {
		return nil, err
	}
	fsrc, err := files.ReadFile("res://shaders/basic.frag")
	if err != nil {
		return nil, err
	}
	r := &Renderer{dev: dev, ubo: make([]byte, uniformSize)}
	r.shader, err = dev.CreateShader(gpu.ShaderDesc{
		Label: "quad",
		Sources: []gpu.ShaderSource{
			{Stage: gpu.StageVertex, Source: string(vsrc)},
			{Stage: gpu.StageFragment, Source: string(fsrc)},
		},
	})
	if err != nil {
		r.Destroy()
		return nil, err
	}
	r.pipe, err = dev.CreatePipeline(gpu.PipelineDesc{
		Shader: r.shader,
		Layout: gpu.VertexLayout{
			Attrs: []gpu.VertexAttr{
				{Location: 0, Slot: 0, Format: gpu.VFFloat2, Offset: 0},
				{Location: 1, Slot: 0, Format: gpu.VFFloat2, Offset: 8},
			},
			Strides: [gpu.MaxVertexSlots]uint16{16},
		},
		Blend:     gpu.BlendAlpha(),
		Primitive: gpu.PrimTriangles,
		Label:     "quad",
	})
	if err != nil {
		r.Destroy()
		return nil, err
	}
	vertices := floats(
		-1, -1, 0, 0,
		1, -1, uvScale, 0,
		1, 1, uvScale, uvScale,
		-1, 1, 0, uvScale,
	)
	r.vb, err = dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferVertex, Data: vertices, Label: "quad.vb"})
	if err != nil {
		r.Destroy()
		return nil, err
	}
	indices := shorts(0, 1, 2, 0, 2, 3)
	r.ib, err = dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferIndex, IndexFormat: gpu.Index16, Data: indices, Label: "quad.ib"})
	if err != nil {
		r.Destroy()
		return nil, err
	}
	r.ub, err = dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferUniform, Size: uniformSize, Dynamic: true, Label: "quad.ub"})
	if err != nil {
		r.Destroy()
		return nil, err
	}
	r.tex, err = dev.CreateTexture(gpu.TextureDesc{
		Width:  checkerSize,
		Height: checkerSize,
		Format: gpu.TexSRGBA8,
		Data:   checkerboard(checkerSize, checkerCell),
		Label:  "quad.checker",
	})
	if err != nil {
		r.Destroy()
		return nil, err
	}
	r.smp, err = dev.CreateSampler(gpu.SamplerDesc{
		MinFilter: gpu.FilterNearest,
		MagFilter: gpu.FilterNearest,
		WrapU:     gpu.WrapRepeat,
		WrapV:     gpu.WrapRepeat,
	})
	if err != nil {
		r.Destroy()
		return nil, err
	}
	return r, nil
}

func (r *Renderer) Draw(p gpu.Pass, mvp mathx.Mat4, tint mathx.Color) {
	putMat4(r.ubo[0:64], mvp)
	putFloat(r.ubo[64:68], tint.R)
	putFloat(r.ubo[68:72], tint.G)
	putFloat(r.ubo[72:76], tint.B)
	putFloat(r.ubo[76:80], tint.A)
	r.dev.UpdateBuffer(r.ub, 0, r.ubo)
	p.SetPipeline(r.pipe)
	p.SetVertexBuffer(0, r.vb)
	p.SetIndexBuffer(r.ib)
	p.SetUniformBuffer(0, r.ub)
	p.SetTexture(0, r.tex, r.smp)
	p.DrawIndexed(6, 0)
}

func (r *Renderer) Destroy() {
	if r.dev == nil {
		return
	}
	r.dev.DestroySampler(r.smp)
	r.dev.DestroyTexture(r.tex)
	r.dev.DestroyBuffer(r.ub)
	r.dev.DestroyBuffer(r.ib)
	r.dev.DestroyBuffer(r.vb)
	r.dev.DestroyPipeline(r.pipe)
	r.dev.DestroyShader(r.shader)
}

func putFloat(dst []byte, v float32) {
	binary.LittleEndian.PutUint32(dst, math.Float32bits(v))
}

func putMat4(dst []byte, m mathx.Mat4) {
	for i, v := range m {
		putFloat(dst[i*4:i*4+4], v)
	}
}

func floats(values ...float32) []byte {
	out := make([]byte, len(values)*4)
	for i, v := range values {
		putFloat(out[i*4:i*4+4], v)
	}
	return out
}

func shorts(values ...uint16) []byte {
	out := make([]byte, len(values)*2)
	for i, v := range values {
		binary.LittleEndian.PutUint16(out[i*2:i*2+2], v)
	}
	return out
}

func checkerboard(size, cell int) []byte {
	out := make([]byte, size*size*4)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			i := (y*size + x) * 4
			if ((x/cell)+(y/cell))%2 == 0 {
				out[i], out[i+1], out[i+2] = 0xE8, 0xE4, 0xDC
			} else {
				out[i], out[i+1], out[i+2] = 0x35, 0x3B, 0x48
			}
			out[i+3] = 0xFF
		}
	}
	return out
}
