//go:build js

package webgl2

import (
	"bytes"
	"syscall/js"

	"aego/core/errs"
	"aego/core/handle"
	"aego/core/log"
	"aego/gpu"
	"aego/gpu/internal/glstate"
)

const outputVert = `#version 300 es
void main() {
    vec2 p = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2));
    gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);
}
`

const outputHead = `#version 300 es
precision highp float;
precision highp sampler2D;
uniform sampler2D tex0;
out vec4 o_color;
`

const outputEncode = `vec3 encode(vec3 c) {
    c = clamp(c, 0.0, 1.0);
    return mix(c * 12.92, 1.055 * pow(c, vec3(1.0 / 2.4)) - 0.055, step(vec3(0.0031308), c));
}
`

const outputEncodeSRGB8 = `vec3 encode(vec3 c) {
    vec3 z = sqrt(sqrt(c));
    vec3 y = ((-0.12917127 * z + 0.93648654) * z + 0.26622550) * z - 0.07379133;
    return mix(c * 12.92, y, step(vec3(0.0031308), c));
}
`

const outputMain = `void main() {
    vec4 c = texelFetch(tex0, ivec2(gl_FragCoord.xy), 0);
    o_color = vec4(encode(c.rgb), 1.0);
}
`

const outputLabel = "webgl2.output"
const backbufferFormat = glSRGB8Alpha8
const outputFrag = outputHead + outputEncode + outputMain

type backbuffer struct {
	color uint32
	depth uint32
	fbo   uint32
	w, h  int
}

type output struct {
	prog uint32
	vao  uint32
	smp  uint32
}

func (d *Device) initOutput() error {
	link := func(fragment string) (js.Value, *errs.Error) {
		return d.link(gpu.ShaderDesc{Label: outputLabel, Sources: []gpu.ShaderSource{
			{Stage: gpu.StageVertex, Source: outputVert},
			{Stage: gpu.StageFragment, Source: fragment},
		}})
	}
	prog, err := link(outputFrag)
	if err != nil {
		return err
	}
	if d.canUseSRGB8Encoder() {
		fast, err := link(outputHead + outputEncodeSRGB8 + outputMain)
		if err == nil {
			if d.sameOutputCodes(prog, fast) {
				d.call(d.f.DeleteProgram, prog)
				prog = fast
			} else {
				d.call(d.f.DeleteProgram, fast)
			}
		}
	}
	d.out.prog = d.obj.reuse(d.out.prog, prog)
	d.bindProgramSlots(prog, d.out.prog, outputLabel)
	d.out.vao = d.obj.reuse(d.out.vao, d.call(d.f.CreateVertexArray))
	d.out.smp = d.obj.reuse(d.out.smp, d.call(d.f.CreateSampler))
	d.applySampler(d.out.smp, gpu.SamplerDesc{})
	return nil
}

func (d *Device) canUseSRGB8Encoder() bool {
	const rgb8 = 0x8051
	const colorEncoding = 0x8210
	format := num(d.ctx.Get("drawingBufferFormat"))
	if backbufferFormat != glSRGB8Alpha8 || (format != rgb8 && format != glRGBA8) {
		return false
	}
	d.cache.BindFramebuffer(0)
	return num(d.ctx.Call("getFramebufferAttachmentParameter", glFramebuffer, glBack, colorEncoding)) == glLinear
}

// Qualify the 256 unfiltered sRGB8 codes on every context generation. The
// approximation must produce the reference encoder's actual framebuffer bytes.
func (d *Device) sameOutputCodes(reference, fast js.Value) bool {
	const size = 128
	var data, first [size * size * 4]byte
	for y := range size {
		for x := range size {
			code := byte((y/8)*16 + x/8)
			at := (y*size + x) * 4
			data[at], data[at+1], data[at+2], data[at+3] = code, code*13+17, 255-code, byte(x%4)*85
		}
	}
	src, err := d.CreateTexture(gpu.TextureDesc{Width: size, Height: size, Format: gpu.TexSRGBA8, Data: data[:]})
	if err != nil {
		return false
	}
	defer d.DestroyTexture(src)
	dst, err := d.CreateTexture(gpu.TextureDesc{Width: size, Height: size, Format: gpu.TexRGBA8})
	if err != nil {
		return false
	}
	defer d.DestroyTexture(dst)
	defer d.cache.Invalidate()
	fbo, _, _, _, err := d.framebufferSub(handle.Cast[glTexture](dst), gpu.TextureSubresource{}, handle.Handle[glTexture]{}, gpu.TextureSubresource{})
	if err != nil {
		return false
	}
	t, ok := d.textures.Get(handle.Cast[glTexture](src))
	if !ok {
		return false
	}
	d.cache.BindVertexArray(0)
	d.cache.SetBlend(glstate.Blend{})
	d.cache.SetDepth(glstate.Depth{})
	d.cache.SetCull(glstate.Cull{})
	d.cache.SetScissor(false, glstate.Rect{})
	d.cache.SetViewport(glstate.Rect{W: size, H: size})
	d.cache.BindTexture(0, t.id)
	d.cache.BindSampler(0, 0)
	for i, program := range [2]js.Value{reference, fast} {
		d.cache.BindFramebuffer(fbo)
		d.call(d.f.UseProgram, program)
		d.call(d.f.DrawArrays, glTriangles, 0, 3)
		if num(d.call(d.f.GetError)) != glNoError {
			return false
		}
		pixels := data[:]
		if i == 0 {
			pixels = first[:]
		}
		if err := d.ReadPixels(dst, 0, 0, size, size, pixels); err != nil {
			return false
		}
	}
	return first[0] == 0 && first[2] == 255 && first[3] == 255 && bytes.Equal(first[:], data[:])
}

func (d *Device) freeOutput() {
	d.cache.ForgetProgram(d.out.prog)
	d.cache.ForgetVertexArray(d.out.vao)
	d.cache.ForgetSampler(d.out.smp)
	d.call(d.f.DeleteProgram, d.obj.drop(d.out.prog))
	d.call(d.f.DeleteVertexArray, d.obj.drop(d.out.vao))
	d.call(d.f.DeleteSampler, d.obj.drop(d.out.smp))
	d.out = output{}
}

func (d *Device) ensureBackbuffer(w, h int) uint32 {
	b := &d.bb
	if b.fbo != 0 && b.w == w && b.h == h {
		return b.fbo
	}
	d.freeBackbuffer()
	if w <= 0 || h <= 0 {
		return 0
	}
	tex := d.call(d.f.CreateTexture)
	b.color = d.obj.put(tex)
	d.bindScratch(b.color)
	d.call(d.f.TexImage2D, glTexture2D, 0, backbufferFormat, w, h, 0, glRGBA, glUnsignedByte, nil)
	d.call(d.f.TexParameteri, glTexture2D, glTextureMinFilter, glNearest)
	d.call(d.f.TexParameteri, glTexture2D, glTextureMagFilter, glNearest)
	d.call(d.f.TexParameteri, glTexture2D, glTextureWrapS, glClampToEdge)
	d.call(d.f.TexParameteri, glTexture2D, glTextureWrapT, glClampToEdge)
	rb := d.call(d.f.CreateRenderbuffer)
	b.depth = d.obj.put(rb)
	d.call(d.f.BindRenderbuffer, glRenderbuffer, rb)
	d.call(d.f.RenderbufferStorage, glRenderbuffer, glDepth24Stencil8, w, h)
	b.fbo = d.obj.put(d.call(d.f.CreateFramebuffer))
	d.cache.BindFramebuffer(b.fbo)
	d.call(d.f.FramebufferTexture2D, glFramebuffer, glColorAttachment0, glTexture2D, tex, 0)
	d.call(d.f.FramebufferRenderbuffer, glFramebuffer, glDepthStencilAttachment, glRenderbuffer, rb)
	if s := uint32(num(d.call(d.f.CheckFramebufferStatus, glFramebuffer))); s != glFramebufferComplete && !d.lost {
		d.log.Error("the window backbuffer is incomplete", log.F("status", fbStatusName(s)), log.F("w", w), log.F("h", h))
	}
	b.w, b.h = w, h
	return b.fbo
}

func (d *Device) freeBackbuffer() {
	b := &d.bb
	if b.fbo != 0 {
		d.cache.ForgetFramebuffer(b.fbo)
		d.call(d.f.DeleteFramebuffer, d.obj.drop(b.fbo))
	}
	if b.color != 0 {
		d.cache.ForgetTexture(b.color)
		d.call(d.f.DeleteTexture, d.obj.drop(b.color))
	}
	if b.depth != 0 {
		d.call(d.f.DeleteRenderbuffer, d.obj.drop(b.depth))
	}
	*b = backbuffer{}
}

func (d *Device) present() {
	b := &d.bb
	if b.fbo == 0 || d.out.prog == 0 {
		return
	}
	c := d.cache
	c.BindFramebuffer(0)
	c.SetViewport(glstate.Rect{W: int32(b.w), H: int32(b.h)})
	c.SetScissor(false, glstate.Rect{})
	c.SetBlend(glstate.Blend{})
	c.SetDepth(glstate.Depth{})
	c.SetCull(glstate.Cull{})
	c.UseProgram(d.out.prog)
	c.BindVertexArray(d.out.vao)
	c.BindTexture(0, b.color)
	c.BindSampler(0, d.out.smp)
	d.call(d.f.DrawArrays, glTriangles, 0, 3)
	c.BindTexture(0, 0)
}
