package render2d

import (
	"slices"

	"aego/engine/problems"
	"aego/render/postfx"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
)

type postProcessPlan struct {
	reg        *postfx.Registry
	revision   uint64
	builds     uint64
	coverage   view.OutputCoverage
	profile    postfx.Profile
	stack      PostProcess
	plan       postfx.Plan
	buildError string
	seen       uint64
}

func (p *postProcessPlan) copyStack(c *PostProcess) {
	p.stack.Enabled = c.Enabled
	prev := p.stack.Effects
	if cap(prev) < len(c.Effects) {
		prev = append(prev, make([]postfx.Instance, len(c.Effects)-len(prev))...)
	}
	if len(prev) > len(c.Effects) {
		clear(prev[len(c.Effects):])
	}
	prev = prev[:len(c.Effects)]
	for i := range c.Effects {
		params := copyEffectParameters(prev[i].Parameters, c.Effects[i].Parameters)
		prev[i] = c.Effects[i]
		prev[i].Parameters = params
	}
	p.stack.Effects = prev
}

func copyEffectParameters(dst, src []postfx.Parameter) []postfx.Parameter {
	if len(src) < len(dst) {
		clear(dst[len(src):])
	}
	return append(dst[:0], src...)
}

func (p *postProcessPlan) refresh(c *PostProcess, reg *postfx.Registry, profile postfx.Profile, materials postfx.MaterialSource) bool {
	if p.reg != reg || p.revision != reg.Revision() || p.profile != profile || p.stack.Enabled != c.Enabled || len(p.stack.Effects) != len(c.Effects) {
		return false
	}
	for i := range c.Effects {
		a, b := &c.Effects[i], &p.stack.Effects[i]
		if a.TypeID != b.TypeID || a.InstanceID != b.InstanceID || a.Enabled != b.Enabled || a.Version != b.Version || a.LUT.ID != b.LUT.ID || a.Material.ID != b.Material.ID {
			return false
		}
		d, known := reg.Lookup(a.TypeID)
		if known && d.UsesMaterial && d.PhaseFor(a, materials) != p.plan.States[i].Phase {
			return false
		}
		if !slices.Equal(a.Parameters, b.Parameters) {
			if p.buildError != "" || !p.plan.UpdateParameters(i, d, a) {
				return false
			}
			b.Parameters = copyEffectParameters(b.Parameters, a.Parameters)
		}
	}
	return true
}

func (s *lightingState) preparePostProcess(r *frame.Runner, reg *postfx.Registry) {
	if s.scene.Store[PostProcess]().Len() == 0 || (s.postProcessingKey != 0 && !s.settings.Bool(s.postProcessingKey)) {
		clear(s.postProcess)
		return
	}
	views := r.Views()
	for i := range views {
		v, b := &views[i], r.Bucket(i)
		if v.Is3D() || v.ID.Node.IsZero() {
			continue
		}
		h := s.scene.Handle(v.ID.Node)
		c, ok := scene.Get[PostProcess](s.scene, h)
		if !ok || !c.Enabled || !s.scene.EffectiveEnabled(h) || !scene.Has[Camera2D](s.scene, h) {
			continue
		}
		b.PostProcessOwner = true
		if len(c.Effects) == 0 {
			continue
		}
		if reg == nil {
			if s.fallbackEffects == nil {
				s.fallbackEffects = postfx.NewRegistry()
			}
			reg = s.fallbackEffects
		}
		if s.postProcess == nil {
			s.postProcess = make(map[view.ID]*postProcessPlan)
		}
		p := s.postProcess[v.ID]
		if p == nil {
			p = &postProcessPlan{}
			s.postProcess[v.ID] = p
		}
		p.seen = s.epoch
		profile := postfx.Profile{ReducedMotion: s.settings != nil && s.reducedKey != 0 && s.settings.Bool(s.reducedKey)}
		if len(views) == 1 {
			profile.LinearOutput = v.OutputPolicy == view.OutputLinear
			profile.Transparent = !v.DoClear || v.Clear.A < 1
		} else {
			p.coverage.Reset()
			for j := range views {
				w := &views[j]
				if w.OutputID == v.OutputID {
					profile.LinearOutput = profile.LinearOutput || w.OutputPolicy == view.OutputLinear
					p.coverage.Add(*w, r.ViewOrder(j))
				}
			}
			profile.Transparent = !p.coverage.Opaque()
		}
		materials, _ := r.Resolver().(postfx.MaterialSource)
		if !p.refresh(c, reg, profile, materials) {
			p.builds++
			p.copyStack(c)
			p.profile, p.reg, p.revision = profile, reg, reg.Revision()
			p.buildError = ""
			if err := reg.BuildWithMaterials(&p.plan, c.Enabled, c.Effects, profile, materials); err != nil {
				p.buildError = err.Error()
			}
		}
		if p.buildError != "" {
			s.diagnostic(v.ID.Node, NamePostProcess+".effects", "PFX001", p.buildError, problems.Error)
			continue
		}
		if err := resolvePostAssets(&p.plan, r.Resolver(), true); err != nil {
			s.diagnostic(v.ID.Node, NamePostProcess+".effects", "PFX001", err.Error(), problems.Error)
			continue
		}
		b.PostProcess = &p.plan
	}
	for i := range views {
		if !r.Bucket(i).PostProcessOwner {
			continue
		}
		for j := 0; j < i; j++ {
			if !r.Bucket(j).PostProcessOwner || views[j].OutputID != views[i].OutputID {
				continue
			}
			const msg = "Multiple active Post Process components claim this output. Disable all but one camera's stack."
			for _, idx := range [2]int{j, i} {
				s.diagnostic(views[idx].ID.Node, NamePostProcess, "PFX002", msg, problems.Error)
			}
			break
		}
	}
	for k, p := range s.postProcess {
		if p.seen != s.epoch {
			delete(s.postProcess, k)
		}
	}
}
