package gpu

import "aego/core/mathx"

type Device interface {
	Caps() Capabilities
	CreateBuffer(BufferDesc) (BufferHandle, error)
	UpdateBuffer(h BufferHandle, offset int, data []byte)
	DestroyBuffer(BufferHandle)
	CreateTexture(TextureDesc) (TextureHandle, error)
	UpdateTexture(t TextureHandle, x, y, w, h int, data []byte)
	DestroyTexture(TextureHandle)
	CreateSampler(SamplerDesc) (SamplerHandle, error)
	DestroySampler(SamplerHandle)
	CreateShader(ShaderDesc) (ShaderHandle, error)
	DestroyShader(ShaderHandle)
	CreatePipeline(PipelineDesc) (PipelineHandle, error)
	DestroyPipeline(PipelineHandle)
	BeginFrame(pixelW, pixelH int) Frame
	EndFrame()

	// ReadPixels copies a region of t into out as tightly packed RGBA8, rows
	// ordered top-down regardless of the backend's native origin: backends
	// reporting OriginBottomLeft flip internally. Call it outside a pass, after
	// EndFrame. out must hold at least w*h*4 bytes.
	ReadPixels(t TextureHandle, x, y, w, h int, out []byte) error

	Close()
}

type Frame interface {
	BeginPass(PassDesc) Pass
	Stats() FrameStats
}

type Pass interface {
	SetViewport(mathx.Rect)
	SetScissor(r mathx.Rect, enabled bool)
	SetPipeline(PipelineHandle)
	SetVertexBuffer(slot int, h BufferHandle)
	SetIndexBuffer(h BufferHandle)
	SetUniformBuffer(slot int, h BufferHandle)
	SetTexture(slot int, t TextureHandle, s SamplerHandle)
	Draw(first, count int)
	DrawIndexed(count, first int)
	DrawInstanced(count, instances int)
	DrawIndexedInstanced(count, first, instances int)
	End()
}
