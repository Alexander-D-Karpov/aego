package gl33

import (
	"aego/gpu/gl33/gl"
	"aego/gpu/internal/glstate"
)

type realAPI struct{}

func (realAPI) UseProgram(id uint32) {
	gl.UseProgram(id)
}

func (realAPI) BindVertexArray(id uint32) {
	gl.BindVertexArray(id)
}

func (realAPI) BindUniformBuffer(slot int, id uint32) {
	gl.BindBufferBase(gl.UNIFORM_BUFFER, uint32(slot), id)
}

func (realAPI) BindTexture(unit int, id uint32) {
	gl.ActiveTexture(gl.TEXTURE0 + uint32(unit))
	gl.BindTexture(gl.TEXTURE_2D, id)
}

func (realAPI) BindSampler(unit int, id uint32) {
	gl.BindSampler(uint32(unit), id)
}

func (realAPI) BindFramebuffer(id uint32) {
	gl.BindFramebuffer(gl.FRAMEBUFFER, id)
}

func (realAPI) SetBlend(b glstate.Blend) {
	if !b.Enabled {
		gl.Disable(gl.BLEND)
		return
	}
	gl.Enable(gl.BLEND)
	gl.BlendFuncSeparate(b.SrcRGB, b.DstRGB, b.SrcA, b.DstA)
}

func (realAPI) SetDepth(d glstate.Depth) {
	if d.Test {
		gl.Enable(gl.DEPTH_TEST)
		gl.DepthFunc(d.Func)
	} else {
		gl.Disable(gl.DEPTH_TEST)
	}
	gl.DepthMask(d.Write)
}

func (realAPI) SetCull(c glstate.Cull) {
	if !c.Enabled {
		gl.Disable(gl.CULL_FACE)
		return
	}
	gl.Enable(gl.CULL_FACE)
	gl.CullFace(c.Face)
}

func (realAPI) FrontFace(cw bool) {
	if cw {
		gl.FrontFace(gl.CW)
		return
	}
	gl.FrontFace(gl.CCW)
}

func (realAPI) Viewport(r glstate.Rect) {
	gl.Viewport(r.X, r.Y, r.W, r.H)
}

func (realAPI) Scissor(enabled bool, r glstate.Rect) {
	if !enabled {
		gl.Disable(gl.SCISSOR_TEST)
		return
	}
	gl.Enable(gl.SCISSOR_TEST)
	gl.Scissor(r.X, r.Y, r.W, r.H)
}

func (realAPI) BindArrayBuffer(id uint32) {
	gl.BindBuffer(gl.ARRAY_BUFFER, id)
}

func (realAPI) BindElementBuffer(id uint32) {
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, id)
}

func (realAPI) AttribPointer(a glstate.Attrib) {
	if a.Integer {
		gl.VertexAttribIPointer(a.Location, a.Size, a.Type, a.Stride, offsetPtr(a.Offset))
		return
	}
	gl.VertexAttribPointer(a.Location, a.Size, a.Type, a.Normalized, a.Stride, offsetPtr(a.Offset))
}
