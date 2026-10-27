package render

import (
	"strings"

	"aego/core/errs"
	"aego/core/log"
	"aego/gpu"
	"aego/render/shader"
	"aego/render/sprite"
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
	if len(batches) == 0 {
		return
	}
	r.batcher.Upload()
	lanes := r.batcher.UploadLanes()
	p := ctx.Pass
	vp := ctx.View.LetterboxViewport()
	for i := range batches {
		b := &batches[i]
		if b.Texture.IsZero() {
			continue
		}
		r.bindSprite(p, b, lanes)
		p.SetVertexBufferAt(0, r.batcher.Buffer(), b.First*spriteStride)
		p.SetTexture(shader.TexMain, b.Texture, b.Sampler)
		if b.Scissor {
			p.SetScissor(b.Clip, true)
		} else {
			p.SetScissor(vp, false)
		}
		p.DrawInstanced(4, b.Count)
		r.stats.Batches++
		r.stats.Instances += uint32(b.Count)
	}
	p.SetScissor(vp, false)
	r.stats.InstanceBytes += uint32(r.batcher.Len() * spriteStride)
}

// bindSprite binds the batch's material, or the built-in sprite pipeline when there is none or it cannot draw.
func (r *Renderer) bindSprite(p gpu.Pass, b *sprite.Batch, lanes bool) {
	if m := b.Material; m != nil && (b.LaneFirst < 0 || lanes) && r.mats.Bind(p, m) {
		if b.LaneFirst >= 0 {
			p.SetVertexBufferAt(shader.LaneSlot, r.batcher.LaneBuffer(), b.LaneFirst*shader.LaneStride)
		}
		return
	}
	p.SetPipeline(r.spritePipes[b.Blend])
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
