//go:build js

package webgl2

import (
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

const outputFrag = `#version 300 es
precision highp float;
precision highp sampler2D;
uniform sampler2D tex0;
out vec4 o_color;
vec3 encode(vec3 c) {
    c = clamp(c, 0.0, 1.0);
    return mix(c * 12.92, 1.055 * pow(c, vec3(1.0 / 2.4)) - 0.055, step(vec3(0.0031308), c));
}
void main() {
    vec4 c = texelFetch(tex0, ivec2(gl_FragCoord.xy), 0);
    o_color = vec4(encode(c.rgb), 1.0);
}
`

const outputLabel = "webgl2.output"

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
	prog, err := d.link(gpu.ShaderDesc{Label: outputLabel, Sources: []gpu.ShaderSource{
		{Stage: gpu.StageVertex, Source: outputVert},
		{Stage: gpu.StageFragment, Source: outputFrag},
	}})
	if err != nil {
		return err
	}
	d.out.prog = d.obj.reuse(d.out.prog, prog)
	d.bindProgramSlots(prog, d.out.prog, outputLabel)
	d.out.vao = d.obj.reuse(d.out.vao, d.call(d.f.CreateVertexArray))
	d.out.smp = d.obj.reuse(d.out.smp, d.call(d.f.CreateSampler))
	d.applySampler(d.out.smp, gpu.SamplerDesc{})
	return nil
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
	d.call(d.f.TexImage2D, glTexture2D, 0, glSRGB8Alpha8, w, h, 0, glRGBA, glUnsignedByte, nil)
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
