package gl33

import (
	"unsafe"

	"aego/core/errs"
	"aego/core/handle"
	"aego/core/log"
	"aego/gpu"
	"aego/gpu/gl33/gl"
	"aego/gpu/gl33/internal/glstate"
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
	d.cache.BindTexture(scratchUnit, id)
}

func (d *Device) CreateTexture(desc gpu.TextureDesc) (gpu.TextureHandle, error) {
	info, ok := textureInfo(desc.Format)
	if !ok {
		return gpu.TextureHandle{}, errs.New(errs.GPUResource, "unsupported texture format", log.F("label", desc.Label))
	}
	if desc.Width <= 0 || desc.Height <= 0 || desc.Width > d.caps.MaxTextureSize || desc.Height > d.caps.MaxTextureSize {
		return gpu.TextureHandle{}, errs.New(errs.GPUResource, "invalid texture size",
			log.F("label", desc.Label), log.F("w", desc.Width), log.F("h", desc.Height))
	}
	if need := desc.Width * desc.Height * info.bpp; desc.Data != nil && len(desc.Data) < need {
		return gpu.TextureHandle{}, errs.New(errs.GPUResource, "texture data too small",
			log.F("label", desc.Label), log.F("need", need), log.F("got", len(desc.Data)))
	}
	var id uint32
	gl.GenTextures(1, &id)
	d.bindScratch(id)
	gl.TexImage2D(gl.TEXTURE_2D, 0, info.internal, int32(desc.Width), int32(desc.Height), 0,
		info.format, info.xtype, bytesPtr(desc.Data))
	mips := desc.Mips && !info.depth
	minFilter := int32(gl.LINEAR)
	if mips {
		minFilter = gl.LINEAR_MIPMAP_LINEAR
		gl.GenerateMipmap(gl.TEXTURE_2D)
	}
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, minFilter)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	d.label(gl.TEXTURE, id, desc.Label)
	if err := d.checkError("create texture", desc.Label); err != nil {
		d.cache.ForgetTexture(id)
		gl.DeleteTextures(1, &id)
		return gpu.TextureHandle{}, err
	}
	h, t := d.textures.Alloc()
	*t = glTexture{id: id, w: desc.Width, h: desc.Height, info: info, mips: mips}
	return gpu.TextureHandle(h), nil
}

func (d *Device) UpdateTexture(th gpu.TextureHandle, x, y, w, h int, data []byte) {
	t, ok := d.textures.Get(handle.Cast[glTexture](th))
	if !ok || len(data) == 0 || t.info.depth {
		return
	}
	if x < 0 || y < 0 || w <= 0 || h <= 0 || x+w > t.w || y+h > t.h || len(data) < w*h*t.info.bpp {
		d.log.Error("texture update out of range", log.F("x", x), log.F("y", y), log.F("w", w), log.F("h", h))
		return
	}
	d.bindScratch(t.id)
	gl.TexSubImage2D(gl.TEXTURE_2D, 0, int32(x), int32(y), int32(w), int32(h),
		t.info.format, t.info.xtype, unsafe.Pointer(&data[0]))
	if t.mips {
		gl.GenerateMipmap(gl.TEXTURE_2D)
	}
}

func (d *Device) DestroyTexture(h gpu.TextureHandle) {
	d.destroy(kindTexture, h.Index, h.Gen)
}

func (d *Device) ReadPixels(th gpu.TextureHandle, x, y, w, h int, out []byte) error {
	t, ok := d.textures.Get(handle.Cast[glTexture](th))
	if !ok {
		return errs.New(errs.GPUInvalidHandle, "read pixels: texture handle is invalid")
	}
	if t.info.depth {
		return errs.New(errs.GPUResource, "read pixels: depth textures are not readable")
	}
	if x < 0 || y < 0 || w <= 0 || h <= 0 || x+w > t.w || y+h > t.h {
		return errs.New(errs.GPUResource, "read pixels: region out of range",
			log.F("x", x), log.F("y", y), log.F("w", w), log.F("h", h))
	}
	if need := w * h * 4; len(out) < need {
		return errs.New(errs.GPUResource, "read pixels: destination too small",
			log.F("need", need), log.F("got", len(out)))
	}
	if d.inFrame {
		return errs.New(errs.GPUResource, "read pixels: must be called after EndFrame")
	}
	fbo := d.ensureFBO(t)
	gl.BindFramebuffer(gl.READ_FRAMEBUFFER, fbo)
	d.cache.Invalidate()
	if status := gl.CheckFramebufferStatus(gl.READ_FRAMEBUFFER); status != gl.FRAMEBUFFER_COMPLETE {
		return errs.New(errs.GPUResource, "read framebuffer is incomplete", log.F("status", status))
	}
	gl.ReadBuffer(gl.COLOR_ATTACHMENT0)
	gl.PixelStorei(gl.PACK_ALIGNMENT, 1)
	for gl.GetError() != gl.NO_ERROR {
	}
	// The caller's y is top-down; GL reads bottom-up, so flip the origin.
	gl.ReadPixels(int32(x), int32(t.h-y-h), int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&out[0]))
	if e := gl.GetError(); e != gl.NO_ERROR {
		return errs.New(errs.GPUResource, "glReadPixels failed", log.F("gl", glErrorName(e)))
	}
	flipRows(out[:w*h*4], w*4, h)
	return nil
}

func flipRows(buf []byte, stride, rows int) {
	for top, bot := 0, rows-1; top < bot; top, bot = top+1, bot-1 {
		a := buf[top*stride : top*stride+stride]
		b := buf[bot*stride : bot*stride+stride]
		for i := range a {
			a[i], b[i] = b[i], a[i]
		}
	}
}

func (d *Device) ensureFBO(t *glTexture) uint32 {
	if t.fbo != 0 {
		return t.fbo
	}
	var fbo uint32
	gl.GenFramebuffers(1, &fbo)
	d.cache.BindFramebuffer(fbo)
	attach := uint32(gl.COLOR_ATTACHMENT0)
	if t.info.depth {
		attach = gl.DEPTH_STENCIL_ATTACHMENT
	}
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, attach, gl.TEXTURE_2D, t.id, 0)
	if t.info.depth {
		gl.DrawBuffer(gl.NONE)
		gl.ReadBuffer(gl.NONE)
	}
	if status := gl.CheckFramebufferStatus(gl.FRAMEBUFFER); status != gl.FRAMEBUFFER_COMPLETE {
		d.log.Error("framebuffer incomplete", log.F("status", status))
	}
	t.fbo = fbo
	return fbo
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
		if t.fbo != 0 {
			fbo := t.fbo
			d.cache.ForgetFramebuffer(fbo)
			gl.DeleteFramebuffers(1, &fbo)
		}
		id := t.id
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
