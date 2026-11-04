//go:build js

package webgl2

import (
	"math"
	"syscall/js"
	"unsafe"

	"aego/core/errs"
	"aego/core/handle"
	"aego/core/log"
	"aego/gpu"
	"aego/gpu/internal/glstate"
	"aego/gpu/internal/texshadow"
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
	if desc.Usage == gpu.BufferUniform {
		b.last.ReserveSmall(size)
	}
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
	if b.dynamic {
		offset, data = b.last.Delta(offset, data)
		if len(data) == 0 {
			return
		}
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

func (d *Device) bindTextureScratch(target uint32, id uint32) {
	d.call(d.f.ActiveTexture, glTexture0+scratchUnit)
	d.cache.BindTextureTarget(scratchUnit, target, id)
	d.cache.BindSampler(scratchUnit, 0)
}

func (d *Device) uploadTexture(t *glTexture) {
	d.bindTextureScratch(t.target, t.id)
	i := t.info
	var data any
	if base := t.shadow.Base(); base != nil {
		d.st.TexUploads++
		d.st.TexBytes += uint64(len(base))
		data = d.view(i, base)
	}
	if t.target == glTexture2DArray {
		d.call(d.f.TexImage3D, t.target, 0, i.internal, t.w, t.h, t.desc.LayerCount(), 0, i.format, i.xtype, data)
	} else {
		d.call(d.f.TexImage2D, t.target, 0, i.internal, t.w, t.h, 0, i.format, i.xtype, data)
	}
	minFilter := glLinear
	if t.mips {
		minFilter = glLinearMipmapLinear
		d.call(d.f.GenerateMipmap, t.target)
	}
	d.call(d.f.TexParameteri, t.target, glTextureMinFilter, minFilter)
	d.call(d.f.TexParameteri, t.target, glTextureMagFilter, glLinear)
	d.call(d.f.TexParameteri, t.target, glTextureWrapS, glClampToEdge)
	d.call(d.f.TexParameteri, t.target, glTextureWrapT, glClampToEdge)
	t.shadow.ManualMipRuns(func(u gpu.TextureUpdate) { d.uploadTextureRegion(t, u) })
}

func (d *Device) CreateTexture(desc gpu.TextureDesc) (gpu.TextureHandle, error) {
	if d.lost {
		return gpu.TextureHandle{}, errs.New(errs.GPUResource, "cannot create a GPU texture while the WebGL context is lost")
	}
	if err := gpu.ValidateTextureDesc(desc, d.caps); err != nil {
		return gpu.TextureHandle{}, errs.Wrap(err, errs.GPUResource, "create texture").With(log.F("label", desc.Label))
	}
	info, ok := textureInfo(desc.Format)
	if !ok {
		return gpu.TextureHandle{}, errs.New(errs.GPUResource, "unsupported texture format")
	}
	t := glTexture{w: desc.Width, h: desc.Height, info: info, mips: desc.Mips && !info.depth, target: glTexture2D, desc: desc, shadow: texshadow.New(desc)}
	t.desc.Data = nil
	t.desc.Mips = t.mips
	if desc.Dimension == gpu.Texture2DArray {
		t.target = glTexture2DArray
	}
	t.id = d.obj.put(d.call(d.f.CreateTexture))
	if !d.lost {
		d.uploadTexture(&t)
	}
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
	if len(data) == 0 {
		return
	}
	if err := d.UpdateTextureRegion(th, gpu.TextureUpdate{X: x, Y: y, Width: w, Height: h, Data: data}); err != nil {
		d.log.Error("texture update failed", log.F("err", err.Error()))
	}
}

func (d *Device) uploadTextureRegion(t *glTexture, u gpu.TextureUpdate) {
	n := u.Width * u.Height * t.info.bpp
	d.st.TexUploads++
	d.st.TexBytes += uint64(n)
	d.bindTextureScratch(t.target, t.id)
	if t.target == glTexture2DArray {
		d.call(d.f.TexSubImage3D, t.target, u.Subresource.Mip, u.X, u.Y, u.Subresource.Layer, u.Width, u.Height, 1, t.info.format, t.info.xtype, d.view(t.info, u.Data[:n]), 0)
	} else {
		d.call(d.f.TexSubImage2D, t.target, u.Subresource.Mip, u.X, u.Y, u.Width, u.Height, t.info.format, t.info.xtype, d.view(t.info, u.Data[:n]), 0)
	}
}

func (d *Device) UpdateTextureRegion(th gpu.TextureHandle, u gpu.TextureUpdate) error {
	t, ok := d.textures.Get(handle.Cast[glTexture](th))
	if !ok {
		return errs.New(errs.GPUInvalidHandle, "texture update: invalid texture handle")
	}
	if err := gpu.ValidateTextureUpdate(t.desc, u); err != nil {
		return errs.Wrap(err, errs.GPUResource, "texture update")
	}
	clear(t.opaqueRegions)
	t.shadow.Update(u)
	if d.lost {
		return nil
	}
	d.uploadTextureRegion(t, u)
	if t.mips && u.Subresource.Mip == 0 {
		d.call(d.f.GenerateMipmap, t.target)
	}
	return d.checkError("update texture", t.desc.Label)
}

func (d *Device) DestroyTexture(h gpu.TextureHandle) { d.destroy(kindTexture, h.Index, h.Gen) }

func (d *Device) ReadPixels(th gpu.TextureHandle, x, y, w, h int, out []byte) error {
	return d.ReadTexturePixels(th, gpu.TextureSubresource{}, x, y, w, h, out)
}

func (d *Device) readTarget(th gpu.TextureHandle, sub gpu.TextureSubresource, x, y, w, h, n int, floating bool) error {
	if d.lost {
		return errs.New(errs.GPUResource, "read pixels: WebGL context is lost")
	}
	t, ok := d.textures.Get(handle.Cast[glTexture](th))
	if !ok {
		return errs.New(errs.GPUInvalidHandle, "read pixels: texture handle is invalid")
	}
	if t.info.depth {
		return errs.New(errs.GPUResource, "read pixels: depth textures are not readable")
	}
	if (t.desc.Format == gpu.TexRGBA16F) != floating {
		return errs.New(errs.GPUResource, "read pixels: RGBA16F requires float readback; normalized targets require byte readback")
	}
	tw, thh, valid := t.desc.SubresourceSize(sub)
	if !valid || x < 0 || y < 0 || w <= 0 || h <= 0 || w > tw || h > thh || x > tw-w || y > thh-h {
		return errs.New(errs.GPUResource, "read pixels: region or subresource out of range")
	}
	if uint64(n) < uint64(w)*uint64(h)*4 {
		return errs.New(errs.GPUResource, "read pixels: destination too small")
	}
	if d.inFrame {
		return errs.New(errs.GPUResource, "read pixels: must be called after EndFrame")
	}
	fbo, _, _, _, err := d.framebufferSub(handle.Cast[glTexture](th), sub, handle.Handle[glTexture]{}, gpu.TextureSubresource{})
	if err != nil {
		return errs.Wrap(err, errs.GPUResource, "read pixels")
	}
	d.call(d.f.BindFramebuffer, glReadFramebuffer, d.obj.get(fbo))
	d.cache.Invalidate()
	d.call(d.f.ReadBuffer, glColorAttachment0)
	d.call(d.f.PixelStorei, glPackAlignment, 1)
	for i := 0; i < 16 && num(d.call(d.f.GetError)) != glNoError; i++ {
	}
	return nil
}

func (d *Device) ReadTexturePixels(th gpu.TextureHandle, sub gpu.TextureSubresource, x, y, w, h int, out []byte) error {
	if err := d.readTarget(th, sub, x, y, w, h, len(out), false); err != nil {
		return err
	}
	n := w * h * 4
	dst := d.scratch(n)
	d.call(d.f.ReadPixels, x, y, w, h, glRGBA, glUnsignedByte, dst, 0)
	if e := uint32(num(d.call(d.f.GetError))); e != glNoError {
		return errs.New(errs.GPUResource, "readPixels failed", log.F("gl", glErrorName(e)))
	}
	js.CopyBytesToGo(out[:n], dst)
	return nil
}

func (d *Device) ReadTexturePixelsFloat(th gpu.TextureHandle, sub gpu.TextureSubresource, x, y, w, h int, out []float32) error {
	if err := d.readTarget(th, sub, x, y, w, h, len(out), true); err != nil {
		return err
	}
	n := w * h * 4
	dst := d.scratch(n * 4)
	fv := d.f32.New(dst.Get("buffer"), 0, n)
	d.call(d.f.ReadPixels, x, y, w, h, glRGBA, glFloat, fv, 0)
	if e := uint32(num(d.call(d.f.GetError))); e != glNoError {
		return errs.New(errs.GPUResource, "readPixels float failed", log.F("gl", glErrorName(e)))
	}
	// WebAssembly and JS typed arrays have the same little-endian float32 layout.
	js.CopyBytesToGo(unsafe.Slice((*byte)(unsafe.Pointer(&out[0])), n*4), dst)
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

func (d *Device) TextureInfo(h gpu.TextureHandle) (gpu.TextureDesc, bool) {
	t, ok := d.textures.Get(handle.Cast[glTexture](h))
	if !ok {
		return gpu.TextureDesc{}, false
	}
	return t.desc, true
}

// OpaqueRegion proves alpha=1 for an immutable base-level rectangle from CPU
// upload bytes. Render attachments and mipmapped images require the usual blend.
func (d *Device) OpaqueRegion(h gpu.TextureHandle, u0, v0, u1, v1 float32) bool {
	t, ok := d.textures.Get(handle.Cast[glTexture](h))
	if !ok || t.rendered || t.mips || t.target != glTexture2D || (t.desc.Format != gpu.TexRGBA8 && t.desc.Format != gpu.TexSRGBA8) {
		return false
	}
	return t.opaqueRegion(u0, v0, u1, v1)
}

func (t *glTexture) opaqueRegion(u0, v0, u1, v1 float32) bool {
	loX, hiX, loY, hiY := min(u0, u1), max(u0, u1), min(v0, v1), max(v0, v1)
	if !(loX >= 0 && loY >= 0 && hiX <= 1 && hiY <= 1 && hiX > loX && hiY > loY) {
		return false
	}
	w, h := float64(t.w), float64(t.h)
	key := [4]int{int(math.Floor(float64(loX) * w)), int(math.Floor(float64(loY) * h)), int(math.Ceil(float64(hiX) * w)), int(math.Ceil(float64(hiY) * h))}
	if value, ok := t.opaqueRegions[key]; ok {
		return value
	}
	if len(t.opaqueRegions) >= 1024 {
		return false
	}
	base := t.shadow.Base()
	if len(base) < t.w*t.h*4 {
		return false
	}
	opaque := true
	for y := key[1]; y < key[3] && opaque; y++ {
		for i, end := 4*(y*t.w+key[0])+3, 4*(y*t.w+key[2]); i < end; i += 4 {
			if base[i] != 255 {
				opaque = false
				break
			}
		}
	}
	if t.opaqueRegions == nil {
		t.opaqueRegions = make(map[[4]int]bool)
	}
	t.opaqueRegions[key] = opaque
	return opaque
}
