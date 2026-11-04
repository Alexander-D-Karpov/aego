//go:build js

package webgl2

import "aego/gpu/internal/glstate"

type api struct {
	d *Device
}

func (a api) UseProgram(id uint32) {
	d := a.d
	d.st.Programs++
	d.call(d.f.UseProgram, d.obj.get(id))
}

func (a api) BindVertexArray(id uint32) {
	d := a.d
	d.call(d.f.BindVertexArray, d.obj.get(id))
}

func (a api) BindUniformBuffer(slot int, id uint32) {
	d := a.d
	d.call(d.f.BindBufferBase, glUniformBuffer, slot, d.obj.get(id))
}

func (a api) BindTexture(unit int, id uint32) {
	d := a.d
	d.call(d.f.ActiveTexture, glTexture0+unit)
	d.call(d.f.BindTexture, glTexture2D, d.obj.get(id))
}

func (a api) BindTextureTarget(unit int, target, id uint32) {
	d := a.d
	d.call(d.f.ActiveTexture, glTexture0+unit)
	d.call(d.f.BindTexture, target, d.obj.get(id))
}

func (a api) BindSampler(unit int, id uint32) {
	d := a.d
	d.call(d.f.BindSampler, unit, d.obj.get(id))
}

func (a api) BindFramebuffer(id uint32) {
	d := a.d
	d.st.Targets++
	d.call(d.f.BindFramebuffer, glFramebuffer, d.obj.get(id))
}

func (a api) SetBlend(b glstate.Blend) {
	d := a.d
	if !b.Enabled {
		d.call(d.f.Disable, glBlend)
		return
	}
	d.call(d.f.Enable, glBlend)
	d.call(d.f.BlendFuncSeparate, b.SrcRGB, b.DstRGB, b.SrcA, b.DstA)
}

func (a api) SetDepth(x glstate.Depth) {
	d := a.d
	if x.Test {
		d.call(d.f.Enable, glDepthTest)
		d.call(d.f.DepthFunc, x.Func)
	} else {
		d.call(d.f.Disable, glDepthTest)
	}
	d.call(d.f.DepthMask, x.Write)
}

func (a api) SetCull(c glstate.Cull) {
	d := a.d
	if !c.Enabled {
		d.call(d.f.Disable, glCullFace)
		return
	}
	d.call(d.f.Enable, glCullFace)
	d.call(d.f.CullFace, c.Face)
}

func (a api) FrontFace(cw bool) {
	d := a.d
	if cw {
		d.call(d.f.FrontFace, glCW)
		return
	}
	d.call(d.f.FrontFace, glCCW)
}

func (a api) Viewport(r glstate.Rect) {
	d := a.d
	d.call(d.f.Viewport, r.X, r.Y, r.W, r.H)
}

func (a api) Scissor(enabled bool, r glstate.Rect) {
	d := a.d
	if !enabled {
		d.call(d.f.Disable, glScissorTest)
		return
	}
	d.call(d.f.Enable, glScissorTest)
	d.call(d.f.Scissor, r.X, r.Y, r.W, r.H)
}

func (a api) BindArrayBuffer(id uint32) {
	d := a.d
	d.call(d.f.BindBuffer, glArrayBuffer, d.obj.get(id))
}

func (a api) BindElementBuffer(id uint32) {
	d := a.d
	d.call(d.f.BindBuffer, glElementArrayBuffer, d.obj.get(id))
}

func (a api) AttribPointer(at glstate.Attrib) {
	d := a.d
	if at.Integer {
		d.call(d.f.VertexAttribIPointer, at.Location, at.Size, at.Type, at.Stride, at.Offset)
		return
	}
	d.call(d.f.VertexAttribPointer, at.Location, at.Size, at.Type, at.Normalized, at.Stride, at.Offset)
}
