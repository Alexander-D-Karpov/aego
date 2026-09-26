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
