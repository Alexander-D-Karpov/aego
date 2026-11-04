//go:build js

package webgl2

import (
	"math"
	"syscall/js"

	"aego/gpu"
)

// The EXT_color_buffer_float gate is checked by the caller. RGBA16F blending
// does not require EXT_float_blend (which concerns 32-bit float attachments).
func (d *Device) probeRGBA16F() bool {
	oldTexture := d.param(glTextureBinding2D)
	oldDraw, oldRead := d.param(glDrawFramebufferBinding), d.param(glReadFramebufferBinding)
	oldProgram, oldVAO := d.param(glCurrentProgram), d.param(glVertexArrayBinding)
	viewport, clearColor, colorMask := d.param(glViewport), d.param(glColorClearValue), d.param(glColorWritemask)
	blend := [6]int{num(d.param(glBlendSrcRGB)), num(d.param(glBlendDstRGB)), num(d.param(glBlendSrcAlpha)), num(d.param(glBlendDstAlpha)), num(d.param(glBlendEquationRGB)), num(d.param(glBlendEquationAlpha))}
	states := [4]int{glBlend, glDepthTest, glCullFace, glScissorTest}
	var enabled [4]bool
	for i, state := range states {
		enabled[i] = d.call(d.f.IsEnabled, state).Truthy()
	}
	tex, fbo, vao, prog := js.Null(), js.Null(), js.Null(), js.Null()
	defer func() {
		d.call(d.f.BindFramebuffer, glDrawFramebuffer, oldDraw)
		d.call(d.f.BindFramebuffer, glReadFramebuffer, oldRead)
		d.call(d.f.UseProgram, oldProgram)
		d.call(d.f.BindVertexArray, oldVAO)
		d.call(d.f.BindTexture, glTexture2D, oldTexture)
		d.call(d.f.Viewport, num(viewport.Index(0)), num(viewport.Index(1)), num(viewport.Index(2)), num(viewport.Index(3)))
		d.call(d.f.ClearColor, clearColor.Index(0).Float(), clearColor.Index(1).Float(), clearColor.Index(2).Float(), clearColor.Index(3).Float())
		d.call(d.f.ColorMask, colorMask.Index(0).Bool(), colorMask.Index(1).Bool(), colorMask.Index(2).Bool(), colorMask.Index(3).Bool())
		d.call(d.f.BlendFuncSeparate, blend[0], blend[1], blend[2], blend[3])
		d.call(d.f.BlendEquationSeparate, blend[4], blend[5])
		for i, state := range states {
			if enabled[i] {
				d.call(d.f.Enable, state)
			} else {
				d.call(d.f.Disable, state)
			}
		}
		d.call(d.f.DeleteProgram, prog)
		d.call(d.f.DeleteVertexArray, vao)
		d.call(d.f.DeleteFramebuffer, fbo)
		d.call(d.f.DeleteTexture, tex)
		d.cache.Invalidate()
	}()
	for i := 0; i < 16 && num(d.call(d.f.GetError)) != glNoError; i++ {
	}
	tex = d.call(d.f.CreateTexture)
	d.call(d.f.BindTexture, glTexture2D, tex)
	d.call(d.f.TexImage2D, glTexture2D, 0, glRGBA16F, 1, 1, 0, glRGBA, glFloat, nil)
	d.call(d.f.TexParameteri, glTexture2D, glTextureMinFilter, glNearest)
	d.call(d.f.TexParameteri, glTexture2D, glTextureMagFilter, glNearest)
	fbo = d.call(d.f.CreateFramebuffer)
	d.call(d.f.BindFramebuffer, glFramebuffer, fbo)
	d.call(d.f.FramebufferTexture2D, glFramebuffer, glColorAttachment0, glTexture2D, tex, 0)
	d.call(d.f.DrawBuffers, d.drawColor)
	d.call(d.f.ReadBuffer, glColorAttachment0)
	if num(d.call(d.f.CheckFramebufferStatus, glFramebuffer)) != glFramebufferComplete || num(d.call(d.f.GetError)) != glNoError {
		return false
	}
	candidate, err := d.link(gpu.ShaderDesc{Label: "RGBA16F capability probe", Sources: []gpu.ShaderSource{
		{Stage: gpu.StageVertex, Source: `#version 300 es
void main() { vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2); gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0); }`},
		{Stage: gpu.StageFragment, Source: `#version 300 es
precision highp float;
out vec4 color;
void main() { color = vec4(2.0, 0.5, 0.25, 0.5); }`},
	}})
	if err != nil {
		return false
	}
	prog = candidate
	vao = d.call(d.f.CreateVertexArray)
	d.call(d.f.BindVertexArray, vao)
	d.call(d.f.UseProgram, prog)
	d.call(d.f.Disable, glDepthTest)
	d.call(d.f.Disable, glCullFace)
	d.call(d.f.Disable, glScissorTest)
	d.call(d.f.ColorMask, true, true, true, true)
	d.call(d.f.Viewport, 0, 0, 1, 1)
	d.call(d.f.ClearColor, 1, 0, 0, 0.25)
	d.call(d.f.Clear, glColorBufferBit)
	d.call(d.f.Enable, glBlend)
	d.call(d.f.BlendEquationSeparate, glFuncAdd, glFuncAdd)
	d.call(d.f.BlendFuncSeparate, glOne, glOne, glOne, glOne)
	d.call(d.f.DrawArrays, glTriangles, 0, 3)
	pixel := d.f32.New(4)
	d.call(d.f.ReadPixels, 0, 0, 1, 1, glRGBA, glFloat, pixel)
	if num(d.call(d.f.GetError)) != glNoError || d.call(d.f.IsContextLost).Truthy() {
		return false
	}
	for i, want := range [4]float64{3, 0.5, 0.25, 0.75} {
		got := pixel.Index(i).Float()
		if math.IsNaN(got) || math.Abs(got-want) > 0.01 {
			return false
		}
	}
	return true
}
