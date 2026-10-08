package render

import (
	"strings"

	"aego/core/errs"
	"aego/core/log"
	"aego/gpu"
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
	r.setUniform(ctx.Pass, ctx.View.ViewProjection(r.caps))
	vp := ctx.View.LetterboxViewport()
	for i := range batches {
		b := &batches[i]
		if b.Texture.IsZero() {
			continue
		}
		ctx.Pass.SetPipeline(r.spritePipes[b.Blend])
		ctx.Pass.SetVertexBufferAt(0, r.batcher.Buffer(), b.First*spriteStride)
		ctx.Pass.SetUniformBuffer(0, r.ub)
		ctx.Pass.SetTexture(0, b.Texture, b.Sampler)
		if b.Scissor {
			ctx.Pass.SetScissor(b.Clip, true)
		} else {
			ctx.Pass.SetScissor(vp, false)
		}
		ctx.Pass.DrawInstanced(4, b.Count)
		r.stats.Batches++
		r.stats.Instances += uint32(b.Count)
	}
	ctx.Pass.SetScissor(vp, false)
	r.stats.InstanceBytes += uint32(r.batcher.Len() * spriteStride)
}

type debugPass struct{}

func (debugPass) Execute(ctx *PassContext) {
	r := ctx.Renderer
	world, overlay, screen := r.dbg.Upload()
	if world.Count == 0 && overlay.Count == 0 && screen.Count == 0 {
		return
	}
	p := ctx.Pass
	if world.Count > 0 || overlay.Count > 0 {
		vp := ctx.View.ViewProjection(r.caps)
		if world.Count > 0 {
			pipe := r.debugPipe
			if ctx.View.UsesDepth() {
				pipe = r.debugDepthPipe
			}
			r.drawLines(p, pipe, vp, world)
		}
		if overlay.Count > 0 {
			r.drawLines(p, r.debugPipe, vp, overlay)
		}
	}
	if screen.Count > 0 {
		sv := view.ScreenView(ctx.View.TargetW, ctx.View.TargetH, ctx.View.PixelScale)
		sv.Target = ctx.View.Target
		r.drawLines(p, r.debugPipe, sv.ViewProjection(r.caps), screen)
	}
}
