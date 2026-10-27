//go:build js

package webgl2

import (
	"slices"
	"syscall/js"

	"aego/core/errs"
	"aego/core/handle"
	"aego/core/log"
	"aego/gpu"
	"aego/gpu/internal/glstate"
)

func bufferTarget(u gpu.BufferUsage) uint32 {
	switch u {
	case gpu.BufferIndex:
		return glElementArrayBuffer
	case gpu.BufferUniform:
		return glUniformBuffer
	}
	return glArrayBuffer
}

func (d *Device) bindBuffer(b *glBuffer) uint32 {
	t := bufferTarget(b.usage)
	if t == glElementArrayBuffer {
		d.cache.BindVertexArray(0)
	}
	d.call(d.f.BindBuffer, t, d.obj.get(b.id))
	return t
}

func (d *Device) uploadBuffer(b *glBuffer, data []byte) {
	t := d.bindBuffer(b)
	usage := glStaticDraw
	if b.dynamic {
		usage = glDynamicDraw
	}
	d.call(d.f.BufferData, t, b.size, usage)
	if len(data) > 0 {
		d.st.BufferUploads++
		d.st.BufferBytes += uint64(len(data))
		d.call(d.f.BufferSubData, t, 0, d.bytes(data), 0, len(data))
	}
}

func (d *Device) CreateBuffer(desc gpu.BufferDesc) (gpu.BufferHandle, error) {
	size := max(desc.Size, len(desc.Data))
	if size <= 0 {
		return gpu.BufferHandle{}, errs.New(errs.GPUResource, "buffer size must be positive", log.F("label", desc.Label))
	}
	b := glBuffer{usage: desc.Usage, size: size, index: desc.IndexFormat, dynamic: desc.Dynamic || desc.Usage == gpu.BufferUniform}
	if !b.dynamic && len(desc.Data) > 0 {
		b.shadow = make([]byte, size)
		copy(b.shadow, desc.Data)
	}
	b.id = d.obj.put(d.call(d.f.CreateBuffer))
	d.uploadBuffer(&b, desc.Data)
	if err := d.checkError("create buffer", desc.Label); err != nil {
		d.call(d.f.DeleteBuffer, d.obj.drop(b.id))
		return gpu.BufferHandle{}, err
	}
	h, dst := d.buffers.Alloc()
	*dst = b
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
	if !b.dynamic {
		if b.shadow == nil {
			b.shadow = make([]byte, b.size)
		}
		copy(b.shadow[offset:], data)
	}
	d.st.BufferUploads++
	d.st.BufferBytes += uint64(len(data))
	t := d.bindBuffer(b)
	d.call(d.f.BufferSubData, t, offset, d.bytes(data), 0, len(data))
}

func (d *Device) DestroyBuffer(h gpu.BufferHandle) {
	d.destroy(kindBuffer, h.Index, h.Gen)
}

func (d *Device) uploadTexture(t *glTexture) {
	d.bindScratch(t.id)
	i := t.info
	if t.shadow != nil {
		d.st.TexUploads++
		d.st.TexBytes += uint64(len(t.shadow))
		d.call(d.f.TexImage2D, glTexture2D, 0, i.internal, t.w, t.h, 0, i.format, i.xtype, d.view(i, t.shadow), 0)
	} else {
		d.call(d.f.TexImage2D, glTexture2D, 0, i.internal, t.w, t.h, 0, i.format, i.xtype, nil)
	}
	minFilter := glLinear
	if t.mips {
		minFilter = glLinearMipmapLinear
		d.call(d.f.GenerateMipmap, glTexture2D)
	}
	d.call(d.f.TexParameteri, glTexture2D, glTextureMinFilter, minFilter)
	d.call(d.f.TexParameteri, glTexture2D, glTextureMagFilter, glLinear)
	d.call(d.f.TexParameteri, glTexture2D, glTextureWrapS, glClampToEdge)
	d.call(d.f.TexParameteri, glTexture2D, glTextureWrapT, glClampToEdge)
}

func (d *Device) CreateTexture(desc gpu.TextureDesc) (gpu.TextureHandle, error) {
	info, ok := textureInfo(desc.Format)
	if !ok {
		return gpu.TextureHandle{}, errs.New(errs.GPUResource, "unsupported texture format", log.F("label", desc.Label))
	}
	if desc.Width <= 0 || desc.Height <= 0 || desc.Width > d.caps.MaxTextureSize || desc.Height > d.caps.MaxTextureSize {
		return gpu.TextureHandle{}, errs.New(errs.GPUResource, "invalid texture size", log.F("label", desc.Label), log.F("w", desc.Width), log.F("h", desc.Height))
	}
	need := desc.Width * desc.Height * info.bpp
	if desc.Data != nil && len(desc.Data) < need {
		return gpu.TextureHandle{}, errs.New(errs.GPUResource, "texture data too small", log.F("label", desc.Label), log.F("need", need), log.F("got", len(desc.Data)))
	}
	t := glTexture{w: desc.Width, h: desc.Height, info: info, mips: desc.Mips && !info.depth}
	if desc.Data != nil {
		t.shadow = slices.Clone(desc.Data[:need])
	}
	t.id = d.obj.put(d.call(d.f.CreateTexture))
	d.uploadTexture(&t)
	if err := d.checkError("create texture", desc.Label); err != nil {
		d.cache.ForgetTexture(t.id)
		d.call(d.f.DeleteTexture, d.obj.drop(t.id))
		return gpu.TextureHandle{}, err
	}
	h, dst := d.textures.Alloc()
	*dst = t
	return gpu.TextureHandle(h), nil
}

func (d *Device) UpdateTexture(th gpu.TextureHandle, x, y, w, h int, data []byte) {
	t, ok := d.textures.Get(handle.Cast[glTexture](th))
	if !ok || len(data) == 0 || t.info.depth {
		return
	}
	bpp := t.info.bpp
	if x < 0 || y < 0 || w <= 0 || h <= 0 || x+w > t.w || y+h > t.h || len(data) < w*h*bpp {
		d.log.Error("texture update out of range", log.F("x", x), log.F("y", y), log.F("w", w), log.F("h", h))
		return
	}
	if t.shadow == nil {
		t.shadow = make([]byte, t.w*t.h*bpp)
	}
	row := w * bpp
	for r := 0; r < h; r++ {
		copy(t.shadow[((y+r)*t.w+x)*bpp:], data[r*row:(r+1)*row])
	}
	d.st.TexUploads++
	d.st.TexBytes += uint64(w * h * bpp)
	d.bindScratch(t.id)
	d.call(d.f.TexSubImage2D, glTexture2D, 0, x, y, w, h, t.info.format, t.info.xtype, d.view(t.info, data[:w*h*bpp]), 0)
	if t.mips {
		d.call(d.f.GenerateMipmap, glTexture2D)
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
		return errs.New(errs.GPUResource, "read pixels: region out of range", log.F("x", x), log.F("y", y), log.F("w", w), log.F("h", h))
	}
	n := w * h * 4
	if len(out) < n {
		return errs.New(errs.GPUResource, "read pixels: destination too small", log.F("need", n), log.F("got", len(out)))
	}
	if d.inFrame {
		return errs.New(errs.GPUResource, "read pixels: must be called after EndFrame")
	}
	fbo, _, _, _, err := d.framebuffer(handle.Cast[glTexture](th), handle.Handle[glTexture]{})
	if err != nil {
		return errs.Wrap(err, errs.GPUResource, "read pixels")
	}
	d.call(d.f.BindFramebuffer, glReadFramebuffer, d.obj.get(fbo))
	d.cache.Invalidate()
	d.call(d.f.ReadBuffer, glColorAttachment0)
	for num(d.call(d.f.GetError)) != glNoError {
	}
	dst := d.scratch(n)
	d.call(d.f.ReadPixels, x, y, w, h, glRGBA, glUnsignedByte, dst, 0)
	if e := uint32(num(d.call(d.f.GetError))); e != glNoError {
		return errs.New(errs.GPUResource, "readPixels failed", log.F("gl", glErrorName(e)))
	}
	js.CopyBytesToGo(out[:n], dst)
	return nil
}

func (d *Device) applySampler(id uint32, desc gpu.SamplerDesc) {
	s := d.obj.get(id)
	d.call(d.f.SamplerParameteri, s, glTextureMinFilter, minFilterValue(desc.MinFilter, desc.Mipmaps))
	d.call(d.f.SamplerParameteri, s, glTextureMagFilter, filterValue(desc.MagFilter))
	d.call(d.f.SamplerParameteri, s, glTextureWrapS, wrapValue(desc.WrapU))
	d.call(d.f.SamplerParameteri, s, glTextureWrapT, wrapValue(desc.WrapV))
}

func (d *Device) CreateSampler(desc gpu.SamplerDesc) (gpu.SamplerHandle, error) {
	id := d.obj.put(d.call(d.f.CreateSampler))
	d.applySampler(id, desc)
	if err := d.checkError("create sampler", ""); err != nil {
		d.call(d.f.DeleteSampler, d.obj.drop(id))
		return gpu.SamplerHandle{}, err
	}
	h, s := d.samplers.Alloc()
	*s = glSampler{id: id, desc: desc}
	return gpu.SamplerHandle(h), nil
}

func (d *Device) DestroySampler(h gpu.SamplerHandle) {
	d.destroy(kindSampler, h.Index, h.Gen)
}

func (d *Device) setupVAO(p *glPipeline) {
	d.cache.BindVertexArray(p.vao)
	for i, a := range p.attrs {
		d.call(d.f.EnableVertexAttribArray, a.Location)
		d.call(d.f.VertexAttribDivisor, a.Location, p.divisor[i])
	}
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
			return gpu.PipelineHandle{}, errs.New(errs.GPUResource, "vertex slot out of range", log.F("label", desc.Label), log.F("slot", slot))
		}
		stride := desc.Layout.Strides[slot]
		if stride == 0 {
			return gpu.PipelineHandle{}, errs.New(errs.GPUResource, "vertex slot has zero stride", log.F("label", desc.Label), log.F("slot", slot))
		}
		size, xtype, norm, integer := vertexFormat(a.Format)
		p.attrs = append(p.attrs, glstate.Attrib{
			Location: uint32(a.Location), Slot: slot, Size: size, Type: xtype,
			Normalized: norm, Integer: integer, Stride: int32(stride), Offset: int(a.Offset),
		})
		var div uint32
		if desc.Layout.PerInstance[slot] {
			div = 1
		}
		p.divisor = append(p.divisor, div)
		p.used[slot] = true
	}
	p.vao = d.obj.put(d.call(d.f.CreateVertexArray))
	d.setupVAO(&p)
	if err := d.checkError("create pipeline", desc.Label); err != nil {
		d.cache.BindVertexArray(0)
		d.call(d.f.DeleteVertexArray, d.obj.drop(p.vao))
		return gpu.PipelineHandle{}, err
	}
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
		if b, ok := d.buffers.Get(h); ok {
			d.cache.ForgetUniformBuffer(b.id)
			d.call(d.f.DeleteBuffer, d.obj.drop(b.id))
			d.buffers.Free(h)
		}
	case kindTexture:
		h := handle.Handle[glTexture]{Index: index, Gen: gen}
		if t, ok := d.textures.Get(h); ok {
			d.forgetFramebuffers(h)
			d.cache.ForgetTexture(t.id)
			d.call(d.f.DeleteTexture, d.obj.drop(t.id))
			d.textures.Free(h)
		}
	case kindSampler:
		h := handle.Handle[glSampler]{Index: index, Gen: gen}
		if s, ok := d.samplers.Get(h); ok {
			d.cache.ForgetSampler(s.id)
			d.call(d.f.DeleteSampler, d.obj.drop(s.id))
			d.samplers.Free(h)
		}
	case kindShader:
		h := handle.Handle[glShader]{Index: index, Gen: gen}
		if s, ok := d.shaders.Get(h); ok {
			d.cache.ForgetProgram(s.program)
			d.call(d.f.DeleteProgram, d.obj.drop(s.program))
			d.shaders.Free(h)
		}
	case kindPipeline:
		h := handle.Handle[glPipeline]{Index: index, Gen: gen}
		if p, ok := d.pipelines.Get(h); ok {
			if d.pass.pipe == p {
				d.pass.pipe = nil
			}
			d.cache.ForgetVertexArray(p.vao)
			d.call(d.f.DeleteVertexArray, d.obj.drop(p.vao))
			d.pipelines.Free(h)
		}
	case kindQuery:
		h := handle.Handle[glQuery]{Index: index, Gen: gen}
		if q, ok := d.queries.Get(h); ok {
			if d.activeQuery == q.id {
				d.call(d.f.EndQuery, glTimeElapsed)
				d.activeQuery = 0
			}
			d.call(d.f.DeleteQuery, d.obj.drop(q.id))
			d.queries.Free(h)
		}
	}
}
