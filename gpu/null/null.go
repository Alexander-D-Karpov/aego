package null

import (
	"aego/core/errs"
	"aego/core/handle"
	"aego/core/log"
	"aego/core/mathx"
	"aego/gpu"
)

const textureSlots = 16

type buffer struct{ size int }

type texture struct{ desc gpu.TextureDesc }

type sampler struct{}

type shader struct{}

type pipeline struct{ prim gpu.Primitive }

type resourceKind uint8

const (
	kindBuffer resourceKind = iota
	kindTexture
	kindSampler
	kindShader
	kindPipeline
)

type pending struct {
	kind       resourceKind
	index, gen uint32
}

type Device struct {
	buffers   *handle.Arena[buffer]
	textures  *handle.Arena[texture]
	samplers  *handle.Arena[sampler]
	shaders   *handle.Arena[shader]
	pipelines *handle.Arena[pipeline]
	frame     frame
	pass      pass
	inFrame   bool
	pending   []pending
}

func New() *Device {
	d := &Device{
		buffers:   handle.NewArena[buffer](64),
		textures:  handle.NewArena[texture](64),
		samplers:  handle.NewArena[sampler](16),
		shaders:   handle.NewArena[shader](16),
		pipelines: handle.NewArena[pipeline](16),
		pending:   make([]pending, 0, 64),
	}
	d.frame.d = d
	d.pass.d = d
	return d
}

func (d *Device) Caps() gpu.Capabilities {
	return gpu.Capabilities{
		MaxTextureSize:          16384,
		MaxVertexAttributes:     16,
		MaxUniformBlockSize:     65536,
		MaxTextureUnits:         textureSlots - 1,
		MaxFragmentTextureUnits: textureSlots,
		MaxArrayTextureLayers:   2048,
		// Simulated support allows deterministic resource tests, not GPU verification.
		RGBA16FRenderable: true,
		Instancing:        true,
		SRGB:              true,
		TextureArrays:     true,
		ClipZeroToOne:     false,
		OriginBottomLeft:  true,
		Vendor:            "aego",
		Renderer:          "null",
		Version:           "null",
		GLSL:              "none",
	}
}

func (d *Device) CreateBuffer(desc gpu.BufferDesc) (gpu.BufferHandle, error) {
	size := max(desc.Size, len(desc.Data))
	if size <= 0 {
		return gpu.BufferHandle{}, errs.New(errs.GPUResource, "buffer size must be positive", log.F("label", desc.Label))
	}
	h, b := d.buffers.Alloc()
	b.size = size
	return gpu.BufferHandle(h), nil
}

func (d *Device) UpdateBuffer(h gpu.BufferHandle, offset int, data []byte) {}

func (d *Device) DestroyBuffer(h gpu.BufferHandle) {
	d.destroy(kindBuffer, h.Index, h.Gen)
}

func (d *Device) CreateTexture(desc gpu.TextureDesc) (gpu.TextureHandle, error) {
	if err := gpu.ValidateTextureDesc(desc, d.Caps()); err != nil {
		return gpu.TextureHandle{}, errs.Wrap(err, errs.GPUResource, "create texture")
	}
	h, t := d.textures.Alloc()
	desc.Data = nil
	t.desc = desc
	return gpu.TextureHandle(h), nil
}

func (d *Device) UpdateTexture(t gpu.TextureHandle, x, y, w, h int, data []byte) {
	if len(data) > 0 {
		_ = d.UpdateTextureRegion(t, gpu.TextureUpdate{X: x, Y: y, Width: w, Height: h, Data: data})
	}
}

func (d *Device) UpdateTextureRegion(th gpu.TextureHandle, u gpu.TextureUpdate) error {
	t, ok := d.textures.Get(handle.Cast[texture](th))
	if !ok {
		return errs.New(errs.GPUInvalidHandle, "texture update: invalid texture handle")
	}
	if err := gpu.ValidateTextureUpdate(t.desc, u); err != nil {
		return errs.Wrap(err, errs.GPUResource, "texture update")
	}
	return nil
}

func (d *Device) DestroyTexture(h gpu.TextureHandle) { d.destroy(kindTexture, h.Index, h.Gen) }

func (d *Device) ValidateRenderTarget(desc gpu.PassDesc) error {
	if desc.Target.IsZero() {
		return nil
	}
	t, ok := d.textures.Get(handle.Cast[texture](desc.Target))
	if !ok {
		return errs.New(errs.GPUInvalidHandle, "render target: invalid texture handle")
	}
	w, h, ok := t.desc.SubresourceSize(desc.TargetSubresource)
	if !ok {
		return errs.New(errs.GPUResource, "render target: invalid layer or mip")
	}
	if !desc.DepthTarget.IsZero() {
		dt, valid := d.textures.Get(handle.Cast[texture](desc.DepthTarget))
		if !valid || dt.desc.Format != gpu.TexDepth24Stencil8 || t.desc.Format == gpu.TexDepth24Stencil8 {
			return errs.New(errs.GPUResource, "render target: invalid depth attachment")
		}
		dw, dh, valid := dt.desc.SubresourceSize(desc.DepthSubresource)
		if !valid || dw != w || dh != h {
			return errs.New(errs.GPUResource, "render target: depth dimensions mismatch")
		}
	}
	return nil
}

// ReadPixels validates like a real backend and returns transparent black.
func (d *Device) ReadPixels(th gpu.TextureHandle, x, y, w, h int, out []byte) error {
	return d.ReadTexturePixels(th, gpu.TextureSubresource{}, x, y, w, h, out)
}

func (d *Device) validateRead(th gpu.TextureHandle, sub gpu.TextureSubresource, x, y, w, h, n int, floating bool) error {
	t, ok := d.textures.Get(handle.Cast[texture](th))
	if !ok {
		return errs.New(errs.GPUInvalidHandle, "read pixels: invalid texture handle")
	}
	if t.desc.Format == gpu.TexDepth24Stencil8 {
		return errs.New(errs.GPUResource, "read pixels: depth textures are not readable")
	}
	if (t.desc.Format == gpu.TexRGBA16F) != floating {
		return errs.New(errs.GPUResource, "read pixels: RGBA16F requires float readback; normalized targets require byte readback")
	}
	tw, thh, valid := t.desc.SubresourceSize(sub)
	if !valid || x < 0 || y < 0 || w <= 0 || h <= 0 || w > tw || h > thh || x > tw-w || y > thh-h {
		return errs.New(errs.GPUResource, "read pixels: invalid region or subresource")
	}
	if uint64(n) < uint64(w)*uint64(h)*4 {
		return errs.New(errs.GPUResource, "read pixels: destination too small")
	}
	if d.inFrame {
		return errs.New(errs.GPUResource, "read pixels: must be called after EndFrame")
	}
	return nil
}

func (d *Device) ReadTexturePixels(th gpu.TextureHandle, sub gpu.TextureSubresource, x, y, w, h int, out []byte) error {
	if err := d.validateRead(th, sub, x, y, w, h, len(out), false); err != nil {
		return err
	}
	clear(out[:w*h*4])
	return nil
}

func (d *Device) ReadTexturePixelsFloat(th gpu.TextureHandle, sub gpu.TextureSubresource, x, y, w, h int, out []float32) error {
	if err := d.validateRead(th, sub, x, y, w, h, len(out), true); err != nil {
		return err
	}
	clear(out[:w*h*4])
	return nil
}

func (d *Device) CreateSampler(desc gpu.SamplerDesc) (gpu.SamplerHandle, error) {
	h, _ := d.samplers.Alloc()
	return gpu.SamplerHandle(h), nil
}

func (d *Device) DestroySampler(h gpu.SamplerHandle) {
	d.destroy(kindSampler, h.Index, h.Gen)
}

func (d *Device) CreateShader(desc gpu.ShaderDesc) (gpu.ShaderHandle, error) {
	if len(desc.Sources) == 0 {
		return gpu.ShaderHandle{}, errs.New(errs.GPUShaderLink, "shader has no stages", log.F("label", desc.Label))
	}
	h, _ := d.shaders.Alloc()
	return gpu.ShaderHandle(h), nil
}

func (d *Device) DestroyShader(h gpu.ShaderHandle) {
	d.destroy(kindShader, h.Index, h.Gen)
}

func (d *Device) CreatePipeline(desc gpu.PipelineDesc) (gpu.PipelineHandle, error) {
	if err := gpu.ValidateVertexLayout(desc.Layout, d.Caps()); err != nil {
		return gpu.PipelineHandle{}, err
	}
	if !d.shaders.Valid(handle.Cast[shader](desc.Shader)) {
		return gpu.PipelineHandle{}, errs.New(errs.GPUInvalidHandle, "pipeline shader handle is invalid", log.F("label", desc.Label))
	}
	h, p := d.pipelines.Alloc()
	p.prim = desc.Primitive
	return gpu.PipelineHandle(h), nil
}

func (d *Device) DestroyPipeline(h gpu.PipelineHandle) {
	d.destroy(kindPipeline, h.Index, h.Gen)
}

func (d *Device) CreateQuery() (gpu.QueryHandle, error) {
	return gpu.QueryHandle{}, errs.New(errs.GPUResource, "null backend does not support timer queries")
}

func (d *Device) DestroyQuery(h gpu.QueryHandle) {}

func (d *Device) destroy(kind resourceKind, index, gen uint32) {
	if d.inFrame {
		d.pending = append(d.pending, pending{kind: kind, index: index, gen: gen})
		return
	}
	d.destroyNow(kind, index, gen)
}

func (d *Device) destroyNow(kind resourceKind, index, gen uint32) {
	switch kind {
	case kindBuffer:
		d.buffers.Free(handle.Handle[buffer]{Index: index, Gen: gen})
	case kindTexture:
		d.textures.Free(handle.Handle[texture]{Index: index, Gen: gen})
	case kindSampler:
		d.samplers.Free(handle.Handle[sampler]{Index: index, Gen: gen})
	case kindShader:
		d.shaders.Free(handle.Handle[shader]{Index: index, Gen: gen})
	case kindPipeline:
		d.pipelines.Free(handle.Handle[pipeline]{Index: index, Gen: gen})
	}
}

func (d *Device) BeginFrame(pixelW, pixelH int) gpu.Frame {
	d.frame.stats = gpu.FrameStats{}
	d.inFrame = true
	return &d.frame
}

func (d *Device) EndFrame() {
	d.pass.active = false
	d.inFrame = false
	for _, p := range d.pending {
		d.destroyNow(p.kind, p.index, p.gen)
	}
	d.pending = d.pending[:0]
}

func (d *Device) Close() {
	d.buffers.Clear()
	d.textures.Clear()
	d.samplers.Clear()
	d.shaders.Clear()
	d.pipelines.Clear()
	d.pending = d.pending[:0]
}

type frame struct {
	d     *Device
	stats gpu.FrameStats
}

func (f *frame) BeginPass(desc gpu.PassDesc) gpu.Pass {
	p := &f.d.pass
	err := f.d.ValidateRenderTarget(desc)
	if err != nil && desc.OmitMismatchedDepth(f.d) {
		err = f.d.ValidateRenderTarget(desc)
	}
	p.active = err == nil
	p.pipe = handle.Handle[pipeline]{}
	p.textures = [textureSlots]gpu.TextureHandle{}
	return p
}

func (f *frame) Stats() gpu.FrameStats {
	return f.stats
}

func (f *frame) QueryResult(h gpu.QueryHandle) (uint64, bool) {
	return 0, false
}

type pass struct {
	d        *Device
	active   bool
	pipe     handle.Handle[pipeline]
	textures [textureSlots]gpu.TextureHandle
}

func (p *pass) SetViewport(mathx.Rect) {}

func (p *pass) SetScissor(mathx.Rect, bool) {}

func (p *pass) SetPipeline(h gpu.PipelineHandle) {
	ah := handle.Cast[pipeline](h)
	if ah != p.pipe {
		p.d.frame.stats.PipelineSwitches++
		p.pipe = ah
	}
}

func (p *pass) SetVertexBuffer(int, gpu.BufferHandle) {}

func (p *pass) SetVertexBufferAt(int, gpu.BufferHandle, int) {}

func (p *pass) SetIndexBuffer(gpu.BufferHandle) {}

func (p *pass) SetUniformBuffer(int, gpu.BufferHandle) {}

func (p *pass) SetTexture(slot int, t gpu.TextureHandle, s gpu.SamplerHandle) {
	if slot < 0 || slot >= textureSlots {
		return
	}
	if p.textures[slot] != t {
		p.d.frame.stats.TexSwitches++
		p.textures[slot] = t
	}
}

func (p *pass) Draw(first, count int) {
	p.count(count, 1)
}

func (p *pass) DrawIndexed(count, first int) {
	p.count(count, 1)
}

func (p *pass) DrawInstanced(count, instances int) {
	p.count(count, instances)
}

func (p *pass) DrawIndexedInstanced(count, first, instances int) {
	p.count(count, instances)
}

func (p *pass) count(n, instances int) {
	if !p.active {
		return
	}
	prim := gpu.PrimTriangles
	if pl, ok := p.d.pipelines.Get(p.pipe); ok {
		prim = pl.prim
	}
	s := &p.d.frame.stats
	s.DrawCalls++
	s.Vertices += uint32(n * instances)
	s.Triangles += uint32(gpu.TriangleCount(prim, n) * instances)
}

func (p *pass) BeginQuery(gpu.QueryHandle) {}

func (p *pass) EndQuery() {}

func (p *pass) End() {
	p.active = false
}

func (d *Device) ResourceStats() gpu.ResourceStats {
	st := gpu.ResourceStats{Buffers: d.buffers.Len(), Textures: d.textures.Len(), Samplers: d.samplers.Len(), Shaders: d.shaders.Len(), Pipelines: d.pipelines.Len()}
	d.textures.Each(func(_ handle.Handle[texture], t *texture) { st.TextureStorageBytes += t.desc.StorageBytes() })
	return st
}

func (d *Device) TextureInfo(h gpu.TextureHandle) (gpu.TextureDesc, bool) {
	t, ok := d.textures.Get(handle.Cast[texture](h))
	if !ok {
		return gpu.TextureDesc{}, false
	}
	return t.desc, true
}
