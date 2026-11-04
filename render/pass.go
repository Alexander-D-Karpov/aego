package render

import (
	"strings"

	"aego/core/errs"
	"aego/core/log"
	"aego/gpu"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/render/view"
)

type PassContext struct {
	Renderer *Renderer
	View     *view.View
	Pass     gpu.Pass
	Frame    *ViewFrame
}

type Pass interface {
	Execute(*PassContext)
}

type PassDesc struct {
	ID     string
	Before []string
	After  []string
	Pass   Pass
}

func sortPasses(descs []PassDesc) ([]PassDesc, error) {
	n := len(descs)
	index := make(map[string]int, n)
	for i, d := range descs {
		if d.ID == "" || d.Pass == nil {
			return nil, errs.New(errs.EngineSystemBad, "pass needs an id and an implementation", log.F("pass", d.ID))
		}
		if _, dup := index[d.ID]; dup {
			return nil, errs.New(errs.EngineSystemDup, "duplicate pass id", log.F("pass", d.ID))
		}
		index[d.ID] = i
	}
	adj := make([][]int, n)
	indeg := make([]int, n)
	edge := func(from, to int) {
		adj[from] = append(adj[from], to)
		indeg[to]++
	}
	for i, d := range descs {
		for _, id := range d.Before {
			j, ok := index[id]
			if !ok {
				return nil, errs.New(errs.EngineSystemDep, "unknown pass in ordering constraint",
					log.F("pass", d.ID), log.F("dependency", id))
			}
			edge(i, j)
		}
		for _, id := range d.After {
			j, ok := index[id]
			if !ok {
				return nil, errs.New(errs.EngineSystemDep, "unknown pass in ordering constraint",
					log.F("pass", d.ID), log.F("dependency", id))
			}
			edge(j, i)
		}
	}
	done := make([]bool, n)
	out := make([]PassDesc, 0, n)
	for len(out) < n {
		pick := -1
		for i := 0; i < n; i++ {
			if !done[i] && indeg[i] == 0 {
				pick = i
				break
			}
		}
		if pick < 0 {
			var cyc []string
			for i := 0; i < n; i++ {
				if !done[i] {
					cyc = append(cyc, descs[i].ID)
				}
			}
			return nil, errs.New(errs.EngineSystemCycle, "render pass order has a cycle",
				log.F("passes", strings.Join(cyc, ",")))
		}
		done[pick] = true
		out = append(out, descs[pick])
		for _, to := range adj[pick] {
			indeg[to]--
		}
	}
	return out, nil
}

type spritePass struct{}

func (spritePass) Execute(ctx *PassContext) {
	r := ctx.Renderer
	batches := r.batcher.Build()
	meshes := r.meshes.TwoD()
	if len(batches) == 0 && len(meshes) == 0 {
		return
	}
	r.batcher.Upload()
	lanes := r.batcher.UploadLanes()
	trim := r.batcher.UploadTrim()
	aux := r.batcher.UploadAux()
	r.batcher.ProfileUploads(batches, lanes, trim, aux)
	if aux {
		r.stats.InstanceBytes += uint32(r.batcher.AuxBytes())
	}
	if trim {
		r.stats.InstanceBytes += uint32(r.batcher.TrimBytes())
	}
	if lanes {
		r.stats.MaterialLaneBytes += uint64(r.batcher.LaneBytes())
	}
	breaks := r.batcher.Breaks()
	r.stats.MaterialBreaks += breaks.Material
	r.stats.TextureBreaks += breaks.Texture
	r.stats.PipelineBreaks += breaks.Pipeline
	r.stats.ClipBreaks += breaks.Clip
	r.stats.LightingBreaks += breaks.Lighting
	p := ctx.Pass
	vp := ctx.View.LetterboxViewport()
	mi := 0
	for i := range batches {
		batch := &batches[i]
		if batch.Texture.IsZero() {
			continue
		}
		end := batch.First + batch.Count
		for from := batch.First; from < end; {
			for mi < len(meshes) && meshes[mi].Key < r.batcher.Key(from) {
				p.SetScissor(vp, false)
				r.drawMeshes(ctx, meshes[mi:mi+1])
				mi++
			}
			to := end
			if mi < len(meshes) {
				to = from + 1
				for to < end && r.batcher.Key(to) < meshes[mi].Key {
					to++
				}
			}
			part := *batch
			part.First, part.Count = from, to-from
			if from > batch.First {
				r.stats.MeshBreaks++
			}
			if part.LaneFirst >= 0 {
				part.LaneFirst += from - batch.First
			}
			if part.TrimFirst >= 0 {
				part.TrimFirst += from - batch.First
			}
			if part.AuxFirst >= 0 {
				part.AuxFirst += from - batch.First
			}
			if (part.TrimFirst < 0 || trim) && (part.AuxFirst < 0 || aux) && r.bindSprite(p, &part, lanes) {
				p.SetVertexBufferAt(0, r.batcher.Buffer(), from*spriteStride)
				p.SetTexture(shader.TexMain, part.Texture, part.Sampler)
				if part.Scissor {
					p.SetScissor(part.Clip, true)
				} else {
					p.SetScissor(vp, false)
				}
				p.DrawInstanced(4, part.Count)
				r.batcher.ProfileDraw(from, to)
				r.stats.Batches++
				r.stats.Instances += uint32(part.Count)
			}
			from = to
		}
	}
	p.SetScissor(vp, false)
	if mi < len(meshes) {
		r.drawMeshes(ctx, meshes[mi:])
	}
	r.stats.InstanceBytes += uint32(r.batcher.Len() * spriteStride)
}

// bindSprite binds the batch's material, or the built-in sprite pipeline when there is none or it cannot draw.
func (r *Renderer) bindSprite(p gpu.Pass, b *sprite.Batch, lanes bool) bool {
	if m := b.Material; m != nil && (b.LaneFirst < 0 || lanes) && r.bindSpriteMaterial(p, b) {
		if b.LaneFirst >= 0 {
			p.SetVertexBufferAt(shader.LaneSlot, r.batcher.LaneBuffer(), b.LaneFirst*shader.LaneStride)
		}
		if b.TrimFirst >= 0 {
			p.SetVertexBufferAt(shader.TrimSlot, r.batcher.TrimBuffer(), b.TrimFirst*shader.TrimStride)
		}
		if b.AuxFirst >= 0 {
			p.SetVertexBufferAt(sprite.AuxSlot, r.batcher.AuxBuffer(), b.AuxFirst*sprite.AuxStride)
		}
		if m.Shader().Features&shader.FeatureLighting != 0 {
			channels := m.Shader().Features&shader.FeatureSurfaceChannels != 0
			if !r.bindLighting(p, b.LightSet, b.Normal, b.Emission, channels) {
				return false
			}
			if channels {
				r.bindSurface(p, texture.Region{}, texture.Emission{}, b.EmissionMode)
			}
		}
		return true
	}
	if b.Material != nil {
		return false
	}
	if b.AuxFirst >= 0 {
		h, ok := r.spriteAuxPipeline(b.Blend)
		if !ok {
			return false
		}
		p.SetPipeline(h)
		p.SetVertexBufferAt(sprite.AuxSlot, r.batcher.AuxBuffer(), b.AuxFirst*sprite.AuxStride)
	} else {
		p.SetPipeline(r.spritePipes[b.Blend])
	}
	return true
}

func (r *Renderer) bindSpriteMaterial(p gpu.Pass, b *sprite.Batch) bool {
	f := &r.vf
	if !r.caps.ESSL || b.LightSet == 0 || int(b.LightSet) > len(f.sets) || f.lighting == nil {
		return r.mats.BindSpriteCoverage(p, b.Material, b.TrimFirst >= 0, b.AuxFirst >= 0, b.Opaque)
	}
	set := f.sets[b.LightSet-1]
	sig := material.LightSignature{Count: set.Count}
	for i := 0; i < set.Count; i++ {
		idx := set.Indices[i]
		sig.Lights[i] = uint8(f.lighting.Lights[idx].Type)
		if f.lightResources != nil && f.lightResources.shadowFor[idx] >= 0 {
			sig.Lights[i] |= 4
		}
	}
	return r.mats.BindSpriteLightSet(p, b.Material, b.TrimFirst >= 0, b.AuxFirst >= 0, b.Opaque, sig)
}

func (r *Renderer) spriteAuxPipeline(blend sprite.BlendMode) (gpu.PipelineHandle, bool) {
	if !r.spriteAuxPipes[blend].IsZero() {
		return r.spriteAuxPipes[blend], true
	}
	h, err := r.dev.CreatePipeline(gpu.PipelineDesc{Shader: r.spriteShader, Layout: sprite.AuxLayout(sprite.Layout()), Blend: sprite.BlendState(blend), Primitive: gpu.PrimTriangleStrip, Label: "sprite.affine"})
	if err != nil {
		return gpu.PipelineHandle{}, false
	}
	r.spriteAuxPipes[blend] = h
	return h, true
}

type debugPass struct{}

func (debugPass) Execute(ctx *PassContext) {
	r := ctx.Renderer
	world, overlay, screen := r.dbg.Upload()
	if world.Count == 0 && overlay.Count == 0 && screen.Count == 0 {
		return
	}
	p := ctx.Pass
	if world.Count > 0 {
		pipe := r.debugPipe
		if ctx.View.UsesDepth() {
			pipe = r.debugDepthPipe
		}
		r.drawLines(p, pipe, world)
	}
	if overlay.Count > 0 {
		r.drawLines(p, r.debugPipe, overlay)
	}
	if screen.Count > 0 {
		sv := view.ScreenView(ctx.View.TargetW, ctx.View.TargetH, ctx.View.PixelScale)
		sv.Target = ctx.View.Target
		r.putScreen(sv.ViewProjection(r.caps))
		p.SetUniformBuffer(shader.SlotView, r.ub)
		r.drawLines(p, r.debugPipe, screen)
		p.SetUniformBuffer(shader.SlotView, r.ubView)
	}
}
