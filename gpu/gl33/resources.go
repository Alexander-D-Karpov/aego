package gl33

import (
	"unsafe"

	"aego/core/errs"
	"aego/core/handle"
	"aego/core/log"
	"aego/gpu"
	"aego/gpu/gl33/gl"
	"aego/gpu/internal/glstate"
)

func bytesPtr(b []byte) unsafe.Pointer {
	if len(b) == 0 {
		return nil
	}
	return unsafe.Pointer(&b[0])
}

func offsetPtr(off int) unsafe.Pointer {
	return unsafe.Add(unsafe.Pointer(nil), off)
}

func (d *Device) CreateBuffer(desc gpu.BufferDesc) (gpu.BufferHandle, error) {
	size := max(desc.Size, len(desc.Data))
	if size <= 0 {
		return gpu.BufferHandle{}, errs.New(errs.GPUResource, "buffer size must be positive", log.F("label", desc.Label))
	}
	usage := uint32(gl.STATIC_DRAW)
	if desc.Dynamic || desc.Usage == gpu.BufferUniform {
		usage = gl.DYNAMIC_DRAW
	}
	var id uint32
	gl.GenBuffers(1, &id)
	gl.BindBuffer(gl.COPY_WRITE_BUFFER, id)
	if len(desc.Data) == size {
		gl.BufferData(gl.COPY_WRITE_BUFFER, size, bytesPtr(desc.Data), usage)
	} else {
		gl.BufferData(gl.COPY_WRITE_BUFFER, size, nil, usage)
		if len(desc.Data) > 0 {
			gl.BufferSubData(gl.COPY_WRITE_BUFFER, 0, len(desc.Data), bytesPtr(desc.Data))
		}
	}
	gl.BindBuffer(gl.COPY_WRITE_BUFFER, 0)
	d.label(gl.BUFFER, id, desc.Label)
	if err := d.checkError("create buffer", desc.Label); err != nil {
		gl.DeleteBuffers(1, &id)
		return gpu.BufferHandle{}, err
	}
	h, b := d.buffers.Alloc()
	*b = glBuffer{id: id, usage: desc.Usage, size: size, index: desc.IndexFormat}
	return gpu.BufferHandle(h), nil
}

func (d *Device) UpdateBuffer(h gpu.BufferHandle, offset int, data []byte) {
	b, ok := d.buffers.Get(handle.Cast[glBuffer](h))
	if !ok || len(data) == 0 {
		return
	}
	if offset < 0 || offset+len(data) > b.size {
		d.log.Error("buffer update out of range", log.F("offset", offset), log.F("len", len(data)), log.F("size", b.size))
		return
	}
	gl.BindBuffer(gl.COPY_WRITE_BUFFER, b.id)
	gl.BufferSubData(gl.COPY_WRITE_BUFFER, offset, len(data), unsafe.Pointer(&data[0]))
	gl.BindBuffer(gl.COPY_WRITE_BUFFER, 0)
}

func (d *Device) DestroyBuffer(h gpu.BufferHandle) {
	d.destroy(kindBuffer, h.Index, h.Gen)
}

func (d *Device) bindScratch(id uint32) {
	gl.ActiveTexture(gl.TEXTURE0 + scratchUnit)
	d.cache.BindTexture(scratchUnit, id)
}

func (d *Device) bindTextureScratch(target, id uint32) {
	gl.ActiveTexture(gl.TEXTURE0 + scratchUnit)
	d.cache.BindTextureTarget(scratchUnit, target, id)
}

func (d *Device) CreateTexture(desc gpu.TextureDesc) (gpu.TextureHandle, error) {
	if err := gpu.ValidateTextureDesc(desc, d.caps); err != nil {
		return gpu.TextureHandle{}, errs.Wrap(err, errs.GPUResource, "create texture")
	}
	info, ok := d.formatInfo(desc.Format)
	if !ok {
		return gpu.TextureHandle{}, errs.New(errs.GPUResource, "unsupported texture format")
	}
	target := uint32(gl.TEXTURE_2D)
	if desc.Dimension == gpu.Texture2DArray {
		target = gl.TEXTURE_2D_ARRAY
	}
	var id uint32
	gl.GenTextures(1, &id)
	d.bindTextureScratch(target, id)
	if desc.Dimension == gpu.Texture2DArray {
		gl.TexImage3D(target, 0, info.internal, int32(desc.Width), int32(desc.Height), int32(desc.Layers), 0, info.format, info.xtype, bytesPtr(desc.Data))
	} else {
		gl.TexImage2D(target, 0, info.internal, int32(desc.Width), int32(desc.Height), 0, info.format, info.xtype, bytesPtr(desc.Data))
	}
	mips := desc.Mips && !info.depth
	minFilter := int32(gl.LINEAR)
	if mips {
		minFilter = gl.LINEAR_MIPMAP_LINEAR
		gl.GenerateMipmap(target)
	}
	gl.TexParameteri(target, gl.TEXTURE_MIN_FILTER, minFilter)
	gl.TexParameteri(target, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(target, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(target, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	d.label(gl.TEXTURE, id, desc.Label)
	if err := d.checkError("create texture", desc.Label); err != nil {
		d.cache.ForgetTexture(id)
		gl.DeleteTextures(1, &id)
		return gpu.TextureHandle{}, err
	}
	h, t := d.textures.Alloc()
	desc.Data = nil
	desc.Mips = mips
	*t = glTexture{id: id, w: desc.Width, h: desc.Height, info: info, mips: mips, target: target, desc: desc}
	return gpu.TextureHandle(h), nil
}

func (d *Device) UpdateTexture(th gpu.TextureHandle, x, y, w, h int, data []byte) {
	if len(data) == 0 {
		return
	}
	if err := d.UpdateTextureRegion(th, gpu.TextureUpdate{X: x, Y: y, Width: w, Height: h, Data: data}); err != nil {
		d.log.Error("texture update failed", log.F("err", err.Error()))
	}
}

func (d *Device) UpdateTextureRegion(th gpu.TextureHandle, u gpu.TextureUpdate) error {
	t, ok := d.textures.Get(handle.Cast[glTexture](th))
	if !ok {
		return errs.New(errs.GPUInvalidHandle, "texture update: invalid handle")
	}
	if err := gpu.ValidateTextureUpdate(t.desc, u); err != nil {
		return errs.Wrap(err, errs.GPUResource, "texture update")
	}
	d.bindTextureScratch(t.target, t.id)
	if t.desc.Dimension == gpu.Texture2DArray {
		gl.TexSubImage3D(t.target, int32(u.Subresource.Mip), int32(u.X), int32(u.Y), int32(u.Subresource.Layer), int32(u.Width), int32(u.Height), 1, t.info.format, t.info.xtype, bytesPtr(u.Data))
	} else {
		gl.TexSubImage2D(t.target, int32(u.Subresource.Mip), int32(u.X), int32(u.Y), int32(u.Width), int32(u.Height), t.info.format, t.info.xtype, bytesPtr(u.Data))
	}
	// Base-level changes rebuild the generated chain. Explicit writes to later
	// levels remain intact until the next base-level update.
	if t.mips && u.Subresource.Mip == 0 {
		gl.GenerateMipmap(t.target)
	}
	return d.checkError("update texture", t.desc.Label)
}

func (d *Device) DestroyTexture(h gpu.TextureHandle) {
	d.destroy(kindTexture, h.Index, h.Gen)
}

func (d *Device) ReadPixels(th gpu.TextureHandle, x, y, w, h int, out []byte) error {
	return d.ReadTexturePixels(th, gpu.TextureSubresource{}, x, y, w, h, out)
}

func (d *Device) readTarget(th gpu.TextureHandle, sub gpu.TextureSubresource, x, y, w, h, count int, floating bool) error {
	t, ok := d.textures.Get(handle.Cast[glTexture](th))
	if !ok {
		return errs.New(errs.GPUInvalidHandle, "read pixels: texture handle is invalid")
	}
	if t.info.depth {
		return errs.New(errs.GPUResource, "read pixels: depth textures are not readable")
	}
	if (t.desc.Format == gpu.TexRGBA16F) != floating {
		return errs.New(errs.GPUResource, "read pixels: use float readback for RGBA16F, RGBA8 readback for normalized targets")
	}
	tw, thh, valid := t.desc.SubresourceSize(sub)
	if !valid || x < 0 || y < 0 || w <= 0 || h <= 0 || w > tw || h > thh || x > tw-w || y > thh-h {
		return errs.New(errs.GPUResource, "read pixels: region or subresource out of range")
	}
	if uint64(w)*uint64(h)*4 > uint64(count) {
		return errs.New(errs.GPUResource, "read pixels: destination too small")
	}
	if d.inFrame {
		return errs.New(errs.GPUResource, "read pixels: must be called after EndFrame")
	}
	fbo, _, _, _, err := d.framebufferSub(handle.Cast[glTexture](th), sub, handle.Handle[glTexture]{}, gpu.TextureSubresource{})
	if err != nil {
		return errs.Wrap(err, errs.GPUResource, "read pixels")
	}
	gl.BindFramebuffer(gl.READ_FRAMEBUFFER, fbo)
	d.cache.Invalidate()
	gl.ReadBuffer(gl.COLOR_ATTACHMENT0)
	gl.PixelStorei(gl.PACK_ALIGNMENT, 1)
	for gl.GetError() != gl.NO_ERROR {
	}
	return nil
}

func (d *Device) ReadTexturePixels(th gpu.TextureHandle, sub gpu.TextureSubresource, x, y, w, h int, out []byte) error {
	if err := d.readTarget(th, sub, x, y, w, h, len(out), false); err != nil {
		return err
	}
	gl.ReadPixels(int32(x), int32(y), int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&out[0]))
	if e := gl.GetError(); e != gl.NO_ERROR {
		return errs.New(errs.GPUResource, "glReadPixels failed", log.F("gl", glErrorName(e)))
	}
	return nil
}

func (d *Device) ReadTexturePixelsFloat(th gpu.TextureHandle, sub gpu.TextureSubresource, x, y, w, h int, out []float32) error {
	if err := d.readTarget(th, sub, x, y, w, h, len(out), true); err != nil {
		return err
	}
	gl.ReadPixels(int32(x), int32(y), int32(w), int32(h), gl.RGBA, gl.FLOAT, unsafe.Pointer(&out[0]))
	if e := gl.GetError(); e != gl.NO_ERROR {
		return errs.New(errs.GPUResource, "float glReadPixels failed", log.F("gl", glErrorName(e)))
	}
	return nil
}

// ReadBackbuffer reads the default framebuffer after EndFrame and before the swap.
func (d *Device) ReadBackbuffer(w, h int, out []byte) error {
	if need := w * h * 4; w <= 0 || h <= 0 || len(out) < need {
		return errs.New(errs.GPUResource, "read backbuffer: bad size", log.F("w", w), log.F("h", h))
	}
	if d.inFrame {
		return errs.New(errs.GPUResource, "read backbuffer: must be called after EndFrame")
	}
	gl.BindFramebuffer(gl.READ_FRAMEBUFFER, 0)
	d.cache.Invalidate()
	gl.PixelStorei(gl.PACK_ALIGNMENT, 1)
	var e uint32
	for _, buf := range []uint32{gl.BACK, gl.FRONT} {
		for gl.GetError() != gl.NO_ERROR {
		}
		gl.ReadBuffer(buf)
		gl.ReadPixels(0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&out[0]))
		if e = gl.GetError(); e == gl.NO_ERROR {
			return nil
		}
	}
	return errs.New(errs.GPUResource, "glReadPixels failed", log.F("gl", glErrorName(e)))
}

func (d *Device) CreateSampler(desc gpu.SamplerDesc) (gpu.SamplerHandle, error) {
	var id uint32
	gl.GenSamplers(1, &id)
	gl.SamplerParameteri(id, gl.TEXTURE_MIN_FILTER, minFilterValue(desc.MinFilter, desc.Mipmaps))
	gl.SamplerParameteri(id, gl.TEXTURE_MAG_FILTER, filterValue(desc.MagFilter))
	gl.SamplerParameteri(id, gl.TEXTURE_WRAP_S, wrapValue(desc.WrapU))
	gl.SamplerParameteri(id, gl.TEXTURE_WRAP_T, wrapValue(desc.WrapV))
	if err := d.checkError("create sampler", ""); err != nil {
		gl.DeleteSamplers(1, &id)
		return gpu.SamplerHandle{}, err
	}
	h, s := d.samplers.Alloc()
	s.id = id
	return gpu.SamplerHandle(h), nil
}

func (d *Device) DestroySampler(h gpu.SamplerHandle) {
	d.destroy(kindSampler, h.Index, h.Gen)
}

func (d *Device) CreatePipeline(desc gpu.PipelineDesc) (gpu.PipelineHandle, error) {
	if err := gpu.ValidateVertexLayout(desc.Layout, d.Caps()); err != nil {
		return gpu.PipelineHandle{}, err
	}
	sh, ok := d.shaders.Get(handle.Cast[glShader](desc.Shader))
	if !ok {
		return gpu.PipelineHandle{}, errs.New(errs.GPUInvalidHandle, "pipeline shader handle is invalid", log.F("label", desc.Label))
	}
	p := glPipeline{
		program: sh.program,
		prim:    desc.Primitive,
		mode:    primitiveMode(desc.Primitive),
		blend:   toBlend(desc.Blend),
		depth:   toDepth(desc.Depth),
		cull:    toCull(desc.Cull),
	}
	for _, a := range desc.Layout.Attrs {
		slot := int(a.Slot)
		if slot >= gpu.MaxVertexSlots {
			return gpu.PipelineHandle{}, errs.New(errs.GPUResource, "vertex slot out of range",
				log.F("label", desc.Label), log.F("slot", slot))
		}
		stride := desc.Layout.Strides[slot]
		if stride == 0 {
			return gpu.PipelineHandle{}, errs.New(errs.GPUResource, "vertex slot has zero stride",
				log.F("label", desc.Label), log.F("slot", slot))
		}
		size, xtype, norm, integer := vertexFormat(a.Format)
		p.attrs = append(p.attrs, glstate.Attrib{
			Location:   uint32(a.Location),
			Slot:       slot,
			Size:       size,
			Type:       xtype,
			Normalized: norm,
			Integer:    integer,
			Stride:     int32(stride),
			Offset:     int(a.Offset),
		})
		p.used[slot] = true
	}
	var vao uint32
	gl.GenVertexArrays(1, &vao)
	d.cache.BindVertexArray(vao)
	for _, a := range p.attrs {
		gl.EnableVertexAttribArray(a.Location)
		var divisor uint32
		if desc.Layout.PerInstance[a.Slot] {
			divisor = 1
		}
		gl.VertexAttribDivisor(a.Location, divisor)
	}
	d.label(gl.VERTEX_ARRAY, vao, desc.Label)
	if err := d.checkError("create pipeline", desc.Label); err != nil {
		d.cache.BindVertexArray(0)
		gl.DeleteVertexArrays(1, &vao)
		return gpu.PipelineHandle{}, err
	}
	p.vao = vao
	h, dst := d.pipelines.Alloc()
	*dst = p
	return gpu.PipelineHandle(h), nil
}

func (d *Device) DestroyPipeline(h gpu.PipelineHandle) {
	d.destroy(kindPipeline, h.Index, h.Gen)
}

func (d *Device) destroy(kind resourceKind, index, gen uint32) {
	if d.inFrame {
		d.pending = append(d.pending, pendingDestroy{kind: kind, index: index, gen: gen})
		return
	}
	d.destroyNow(kind, index, gen)
}

func (d *Device) destroyNow(kind resourceKind, index, gen uint32) {
	switch kind {
	case kindBuffer:
		h := handle.Handle[glBuffer]{Index: index, Gen: gen}
		b, ok := d.buffers.Get(h)
		if !ok {
			return
		}
		id := b.id
		d.forgetBuffer(id)
		gl.DeleteBuffers(1, &id)
		d.buffers.Free(h)
	case kindTexture:
		h := handle.Handle[glTexture]{Index: index, Gen: gen}
		t, ok := d.textures.Get(h)
		if !ok {
			return
		}
		id := t.id
		d.forgetFramebuffers(h)
		d.cache.ForgetTexture(id)
		gl.DeleteTextures(1, &id)
		d.textures.Free(h)
	case kindSampler:
		h := handle.Handle[glSampler]{Index: index, Gen: gen}
		s, ok := d.samplers.Get(h)
		if !ok {
			return
		}
		id := s.id
		d.cache.ForgetSampler(id)
		gl.DeleteSamplers(1, &id)
		d.samplers.Free(h)
	case kindShader:
		h := handle.Handle[glShader]{Index: index, Gen: gen}
		s, ok := d.shaders.Get(h)
		if !ok {
			return
		}
		d.cache.ForgetProgram(s.program)
		gl.DeleteProgram(s.program)
		d.shaders.Free(h)
	case kindPipeline:
		h := handle.Handle[glPipeline]{Index: index, Gen: gen}
		p, ok := d.pipelines.Get(h)
		if !ok {
			return
		}
		vao := p.vao
		if d.pass.pipe == p {
			d.pass.pipe = nil
		}
		d.cache.ForgetVertexArray(vao)
		gl.DeleteVertexArrays(1, &vao)
		d.pipelines.Free(h)
	case kindQuery:
		h := handle.Handle[glQuery]{Index: index, Gen: gen}
		q, ok := d.queries.Get(h)
		if !ok {
			return
		}
		id := q.id
		if d.activeQuery == id {
			gl.EndQuery(gl.TIME_ELAPSED)
			d.activeQuery = 0
		}
		gl.DeleteQueries(1, &id)
		d.queries.Free(h)
	}
}

func (d *Device) forgetBuffer(id uint32) {
	d.cache.ForgetUniformBuffer(id)
}

func (d *Device) TextureInfo(h gpu.TextureHandle) (gpu.TextureDesc, bool) {
	t, ok := d.textures.Get(handle.Cast[glTexture](h))
	if !ok {
		return gpu.TextureDesc{}, false
	}
	return t.desc, true
}
