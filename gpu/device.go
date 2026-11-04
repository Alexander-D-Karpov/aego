package gpu

import "aego/core/mathx"

type Device interface {
	Caps() Capabilities
	CreateBuffer(BufferDesc) (BufferHandle, error)
	UpdateBuffer(h BufferHandle, offset int, data []byte)
	DestroyBuffer(BufferHandle)
	CreateTexture(TextureDesc) (TextureHandle, error)
	UpdateTexture(t TextureHandle, x, y, w, h int, data []byte)
	UpdateTextureRegion(t TextureHandle, update TextureUpdate) error
	DestroyTexture(TextureHandle)
	CreateSampler(SamplerDesc) (SamplerHandle, error)
	DestroySampler(SamplerHandle)
	CreateShader(ShaderDesc) (ShaderHandle, error)
	DestroyShader(ShaderHandle)
	CreatePipeline(PipelineDesc) (PipelineHandle, error)
	DestroyPipeline(PipelineHandle)
	CreateQuery() (QueryHandle, error)
	DestroyQuery(QueryHandle)
	ReadPixels(t TextureHandle, x, y, w, h int, out []byte) error
	BeginFrame(pixelW, pixelH int) Frame
	EndFrame()
	Close()
}

type Frame interface {
	BeginPass(PassDesc) Pass
	Stats() FrameStats
	QueryResult(QueryHandle) (uint64, bool)
}

type Pass interface {
	SetViewport(mathx.Rect)
	SetScissor(r mathx.Rect, enabled bool)
	SetPipeline(PipelineHandle)
	SetVertexBuffer(slot int, h BufferHandle)
	SetVertexBufferAt(slot int, h BufferHandle, offsetBytes int)
	SetIndexBuffer(h BufferHandle)
	SetUniformBuffer(slot int, h BufferHandle)
	SetTexture(slot int, t TextureHandle, s SamplerHandle)
	BeginQuery(QueryHandle)
	EndQuery()
	Draw(first, count int)
	DrawIndexed(count, first int)
	DrawInstanced(count, instances int)
	DrawIndexedInstanced(count, first, instances int)
	End()
}

type BackbufferReader interface {
	ReadBackbuffer(w, h int, out []byte) error
}

// TextureReader exposes subresource readback for diagnostics and integration
// tests. Bytes are RGBA8; float data is RGBA float32 for a RGBA16F target. Row
// orientation matches ReadPixels; callers producing images retain their usual
// top-down conversion. Public screenshots use the display-resolved RGBA8 target.
type TextureReader interface {
	ReadTexturePixels(t TextureHandle, sub TextureSubresource, x, y, w, h int, out []byte) error
	ReadTexturePixelsFloat(t TextureHandle, sub TextureSubresource, x, y, w, h int, out []float32) error
}

// RenderTargetValidator checks the exact attachment subresources (including
// framebuffer completeness on real devices) before a caller commits an output
// mode. A failed HDR target can then fall back before any draw is submitted.
type RenderTargetValidator interface{ ValidateRenderTarget(PassDesc) error }

// TextureDescriber exposes an owned texture's immutable allocation description
// without reading GPU memory. Data is always nil; stale handles return false.
type TextureDescriber interface {
	TextureInfo(TextureHandle) (TextureDesc, bool)
}

// OmitMismatchedDepth preserves the original BeginPass behavior for a pair of
// ordinary 2D base-level textures of different sizes. It may only drop depth;
// callers must still validate the original color target and never redirect a
// failed draw to the window. ValidateRenderTarget always validates both inputs.
func (p *PassDesc) OmitMismatchedDepth(d TextureDescriber) bool {
	if p.Target.IsZero() || p.DepthTarget.IsZero() || p.TargetSubresource != (TextureSubresource{}) || p.DepthSubresource != (TextureSubresource{}) {
		return false
	}
	color, ok := d.TextureInfo(p.Target)
	if !ok || color.Dimension != Texture2D || color.Format == TexDepth24Stencil8 {
		return false
	}
	depth, ok := d.TextureInfo(p.DepthTarget)
	if !ok || depth.Dimension != Texture2D || depth.Format != TexDepth24Stencil8 || color.Width == depth.Width && color.Height == depth.Height {
		return false
	}
	p.DepthTarget = TextureHandle{}
	return true
}

type BridgeStats struct {
	Calls         uint64 `json:"calls"`
	Queries       uint64 `json:"queries"`
	Draws         uint64 `json:"draws"`
	BufferUploads uint64 `json:"bufferUploads"`
	BufferBytes   uint64 `json:"bufferBytes"`
	TexUploads    uint64 `json:"texUploads"`
	TexBytes      uint64 `json:"texBytes"`
	Programs      uint64 `json:"programs"`
	Targets       uint64 `json:"targets"`
	BridgeNs      uint64 `json:"bridgeNs"`
}

type BridgeReporter interface {
	BridgeStats() BridgeStats
}

// ShaderCompilation is polled on the device's render thread. A false completion
// never blocks on shader status; Cancel releases an unfinished candidate.
type ShaderCompilation interface {
	Poll() (ShaderHandle, bool, error)
	Cancel()
}

type AsyncShaderCompiler interface {
	BeginShaderCompile(ShaderDesc) (ShaderCompilation, error)
}

// ContextState invalidates pending GPU work as soon as the context is lost, not
// only when the later context-restored event arrives.
type ContextState interface {
	ContextGeneration() uint64
	ContextLost() bool
}

// UniformMember describes an active linked field. An optimized-out field is not
// present here; source descriptors remain the authority for material schemas.
type UniformMember struct {
	Name   string
	Type   string
	Offset int
}

type UniformBlockLayout struct {
	Size    int
	Members []UniformMember
}

type UniformBlockInspector interface {
	UniformBlockLayout(ShaderHandle, int) (UniformBlockLayout, bool, error)
}

// ResourceStats counts live logical GPU objects, including restored generations.
// It is intended for leak checks and profiler displays, not driver memory estimates.
type ResourceStats struct {
	Buffers, Textures, Samplers, Shaders, Pipelines, PendingCompiles int
	Framebuffers                                                     int
	// TextureStorageBytes is format/layer/mip pixel storage, not driver VRAM.
	TextureStorageBytes uint64
}
type ResourceReporter interface{ ResourceStats() ResourceStats }
