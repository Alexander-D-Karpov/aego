package render

import (
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/debug"
	"aego/render/mesh"
)

type meshOpaquePass struct{}

func (meshOpaquePass) Execute(ctx *PassContext) {
	ctx.Renderer.drawMeshes(ctx, ctx.Renderer.meshes.Opaque())
}

type meshBlendPass struct{}

func (meshBlendPass) Execute(ctx *PassContext) {
	ctx.Renderer.drawMeshes(ctx, ctx.Renderer.meshes.Transparent())
}

func (r *Renderer) drawMeshes(ctx *PassContext, items []mesh.Item) {
	if len(items) == 0 {
		return
	}
	p := ctx.Pass
	r.setUniform(p, ctx.View.ViewProjection(r.caps))
	for i := range items {
		it := &items[i]
		m := it.Blend
		if int(m) >= blendModes {
			m = 0
		}
		ds := 0
		if it.DoubleSided {
			ds = 1
		}
		p.SetPipeline(r.meshPipes[m][ds])
		p.SetUniformBuffer(0, r.ub)
		r.setDrawUniform(p, it)
		p.SetVertexBuffer(0, it.Mesh.VB)
		p.SetIndexBuffer(it.Mesh.IB)
		p.SetTexture(0, it.Texture.Texture, it.Sampler)
		p.DrawIndexed(it.Mesh.Count, 0)
		r.stats.MeshDraws++
	}
}

func (r *Renderer) setDrawUniform(p gpu.Pass, it *mesh.Item) {
	b := r.drawBuf
	for i, v := range it.World {
		putF32(b[i*4:], v)
	}
	c := it.Color
	if c == (mathx.Color{}) {
		c = mathx.Color{R: 1, G: 1, B: 1, A: 1}
	}
	c = c.Premultiplied()
	putF32(b[64:], c.R)
	putF32(b[68:], c.G)
	putF32(b[72:], c.B)
	putF32(b[76:], c.A)
	putF32(b[80:], it.Texture.UVMin.X)
	putF32(b[84:], it.Texture.UVMin.Y)
	putF32(b[88:], it.Texture.UVMax.X)
	putF32(b[92:], it.Texture.UVMax.Y)
	r.dev.UpdateBuffer(r.ubDraw, 0, b)
	p.SetUniformBuffer(1, r.ubDraw)
}

type sprite3DPass struct{}

func (sprite3DPass) Execute(ctx *PassContext) {
	r := ctx.Renderer
	batches := r.sprites3d.Build()
	if len(batches) == 0 {
		return
	}
	r.sprites3d.Upload()
	r.setUniform(ctx.Pass, ctx.View.ViewProjection(r.caps))
	for i := range batches {
		b := &batches[i]
		if b.Texture.IsZero() {
			continue
		}
		ctx.Pass.SetPipeline(r.sprite3dPipes[b.Blend])
		ctx.Pass.SetVertexBufferAt(0, r.sprites3d.Buffer(), b.First*sprite3DStride)
		ctx.Pass.SetUniformBuffer(0, r.ub)
		ctx.Pass.SetTexture(0, b.Texture, b.Sampler)
		ctx.Pass.DrawInstanced(4, b.Count)
		r.stats.Batches++
		r.stats.Instances += uint32(b.Count)
	}
}

func (r *Renderer) drawLines(p gpu.Pass, pipe gpu.PipelineHandle, vp mathx.Mat4, rg debug.Range) {
	p.SetPipeline(pipe)
	r.setUniform(p, vp)
	p.SetVertexBuffer(0, r.dbg.Buffer())
	p.Draw(rg.First, rg.Count)
	r.stats.DebugVerts += uint32(rg.Count)
}
