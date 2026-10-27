//go:build js

package webgl2

import (
	"math"

	"aego/core/handle"
	"aego/core/log"
	"aego/core/mathx"
	"aego/gpu"
	"aego/gpu/internal/glstate"
)

type frame struct {
	d     *Device
	stats gpu.FrameStats
}

func (f *frame) BeginPass(desc gpu.PassDesc) gpu.Pass {
	return f.d.beginPass(desc)
}

func (f *frame) Stats() gpu.FrameStats {
	return f.stats
}

type pass struct {
	d       *Device
	active  bool
	flipY   bool
	pipe    *glPipeline
	vb      [gpu.MaxVertexSlots]gpu.BufferHandle
	vbOff   [gpu.MaxVertexSlots]int
	ib      gpu.BufferHandle
	ibType  uint32
	ibSize  int
	targetH int
	query   gpu.QueryHandle
}

func (d *Device) beginPass(desc gpu.PassDesc) gpu.Pass {
	p := &d.pass
	if p.active {
		d.log.Warn("previous render pass was not ended")
	}
	*p = pass{d: d, active: true, flipY: true}
	fbo, w, h, hasDepth := uint32(0), d.fbW, d.fbH, true
	if !desc.Target.IsZero() {
		f, fw, fh, depth, err := d.framebuffer(handle.Cast[glTexture](desc.Target), handle.Cast[glTexture](desc.DepthTarget))
		if err == nil {
			fbo, w, h, hasDepth = f, fw, fh, depth
			p.flipY = false
		} else {
			d.log.Warn("render target is invalid, drawing to window", log.F("err", err.Error()))
		}
	}
	if fbo == 0 {
		fbo = d.ensureBackbuffer(d.fbW, d.fbH)
		d.window = true
	}
	p.targetH = h
	c := d.cache
	c.BindFramebuffer(fbo)
	c.SetViewport(glstate.Rect{W: int32(w), H: int32(h)})
	c.SetScissor(false, glstate.Rect{})
	c.SetFrontFace(desc.FlipWinding)
	mask := 0
	if desc.Color == gpu.LoadClear {
		cc := desc.ClearColor
		d.call(d.f.ClearColor, cc.R, cc.G, cc.B, cc.A)
		mask |= glColorBufferBit
	}
	if desc.Depth == gpu.LoadClear && hasDepth {
		d.call(d.f.ClearDepth, desc.ClearDepth)
		c.SetDepth(glstate.Depth{Write: true, Func: glLess})
		mask |= glDepthBufferBit | glStencilBufferBit
	}
	if mask != 0 {
		d.call(d.f.Clear, mask)
	}
	return p
}

func (p *pass) rect(r mathx.Rect) glstate.Rect {
	x, y, w, h := int32(r.X), int32(r.Y), int32(r.W), int32(r.H)
	if p.flipY {
		y = int32(p.targetH) - y - h
	}
	return glstate.Rect{X: x, Y: y, W: w, H: h}
}

func (p *pass) SetViewport(r mathx.Rect) {
	p.d.cache.SetViewport(p.rect(r))
}

func (p *pass) SetScissor(r mathx.Rect, enabled bool) {
	x0, y0 := float32(math.Floor(float64(r.X))), float32(math.Floor(float64(r.Y)))
	x1, y1 := float32(math.Ceil(float64(r.X+r.W))), float32(math.Ceil(float64(r.Y+r.H)))
	p.d.cache.SetScissor(enabled, p.rect(mathx.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}))
}

func (p *pass) SetPipeline(h gpu.PipelineHandle) {
	pl, ok := p.d.pipelines.Get(handle.Cast[glPipeline](h))
	if !ok {
		p.pipe = nil
		return
	}
	c := p.d.cache
	changed := c.UseProgram(pl.program)
	if c.BindVertexArray(pl.vao) {
		changed = true
	}
	c.SetBlend(pl.blend)
	c.SetDepth(pl.depth)
	c.SetCull(pl.cull)
	if changed || p.pipe != pl {
		p.d.frame.stats.PipelineSwitches++
	}
	p.pipe = pl
}

func (p *pass) SetVertexBuffer(slot int, h gpu.BufferHandle) {
	p.SetVertexBufferAt(slot, h, 0)
}

func (p *pass) SetVertexBufferAt(slot int, h gpu.BufferHandle, offsetBytes int) {
	if slot < 0 || slot >= gpu.MaxVertexSlots || offsetBytes < 0 {
		return
	}
	p.vb[slot] = gpu.BufferHandle{}
	p.vbOff[slot] = 0
	if b, ok := p.d.buffers.Get(handle.Cast[glBuffer](h)); ok && b.usage == gpu.BufferVertex {
		p.vb[slot] = h
		p.vbOff[slot] = offsetBytes
	}
}

func (p *pass) SetIndexBuffer(h gpu.BufferHandle) {
	p.ib = gpu.BufferHandle{}
	b, ok := p.d.buffers.Get(handle.Cast[glBuffer](h))
	if !ok || b.usage != gpu.BufferIndex {
		return
	}
	p.ib = h
	p.ibSize = b.index.Size()
	p.ibType = glUnsignedShort
	if b.index == gpu.Index32 {
		p.ibType = glUnsignedInt
	}
}

func (p *pass) SetUniformBuffer(slot int, h gpu.BufferHandle) {
	if b, ok := p.d.buffers.Get(handle.Cast[glBuffer](h)); ok && b.usage == gpu.BufferUniform {
		p.d.cache.BindUniformBuffer(slot, b.id)
	}
}

func (p *pass) SetTexture(slot int, t gpu.TextureHandle, s gpu.SamplerHandle) {
	if slot < 0 || slot >= scratchUnit {
		return
	}
	var texID, smpID uint32
	if tex, ok := p.d.textures.Get(handle.Cast[glTexture](t)); ok {
		texID = tex.id
	}
	if smp, ok := p.d.samplers.Get(handle.Cast[glSampler](s)); ok {
		smpID = smp.id
	}
	if p.d.cache.BindTexture(slot, texID) {
		p.d.frame.stats.TexSwitches++
	}
	p.d.cache.BindSampler(slot, smpID)
}

func (p *pass) prepare(indexed bool) *glPipeline {
	pl := p.pipe
	if pl == nil || !p.active {
		return nil
	}
	d := p.d
	d.cache.UseProgram(pl.program)
	d.cache.BindVertexArray(pl.vao)
	a := api{d: d}
	for s := 0; s < gpu.MaxVertexSlots; s++ {
		if !pl.used[s] {
			continue
		}
		h := p.vb[s]
		b, ok := d.buffers.Get(handle.Cast[glBuffer](h))
		if !ok {
			return nil
		}
		key := glstate.Key{Index: h.Index, Gen: h.Gen, Offset: uint32(p.vbOff[s])}
		if !pl.bind.BindSlot(a, s, key, b.id, p.vbOff[s], pl.attrs) {
			return nil
		}
	}
	if indexed {
		b, ok := d.buffers.Get(handle.Cast[glBuffer](p.ib))
		if !ok {
			return nil
		}
		if !pl.bind.BindIndex(a, glstate.Key{Index: p.ib.Index, Gen: p.ib.Gen}, b.id) {
			return nil
		}
	}
	return pl
}

func (p *pass) count(pl *glPipeline, n, instances int) {
	s := &p.d.frame.stats
	s.DrawCalls++
	s.Vertices += uint32(n * instances)
	s.Triangles += uint32(gpu.TriangleCount(pl.prim, n) * instances)
	p.d.st.Draws++
}

func (p *pass) Draw(first, count int) {
	if pl := p.prepare(false); pl != nil {
		p.d.call(p.d.f.DrawArrays, pl.mode, first, count)
		p.count(pl, count, 1)
	}
}

func (p *pass) DrawIndexed(count, first int) {
	if pl := p.prepare(true); pl != nil {
		p.d.call(p.d.f.DrawElements, pl.mode, count, p.ibType, first*p.ibSize)
		p.count(pl, count, 1)
	}
}

func (p *pass) DrawInstanced(count, instances int) {
	if pl := p.prepare(false); pl != nil {
		p.d.call(p.d.f.DrawArraysInstanced, pl.mode, 0, count, instances)
		p.count(pl, count, instances)
	}
}

func (p *pass) DrawIndexedInstanced(count, first, instances int) {
	if pl := p.prepare(true); pl != nil {
		p.d.call(p.d.f.DrawElementsInstanced, pl.mode, count, p.ibType, first*p.ibSize, instances)
		p.count(pl, count, instances)
	}
}

func (p *pass) End() {
	p.active = false
}
