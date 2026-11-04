package particles

import (
	"cmp"
	"slices"
	"time"

	"aego/asset"
	"aego/core/mathx"
	"aego/engine"
	"aego/render/culling"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/sprite"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
)

type extract struct{}

func (extract) Run(ctx *engine.Context) {
	s := Of(ctx.World)
	r := frame.Of(ctx)
	if s != nil && r != nil && r.Scene() == s.scene {
		s.extract(r)
	}
}

func (s *Service) extract(r *frame.Runner) {
	s.submitted = 0
	*s.usage = sprite.SubmissionStats{}
	m := &s.metrics
	m.Submitted, m.VisibleEmitters, m.CulledEmitters, m.SortNS, m.ExtractNS = 0, 0, 0, 0, 0
	if !s.enabled || s.world.Replaying() || len(s.systems) == 0 || r.Renderer() == nil {
		return
	}
	start := time.Now()
	s.appendTo(r, s.usage, m)
	s.submitted = m.Submitted
	m.ExtractNS = uint64(time.Since(start).Nanoseconds()) - m.SortNS
}

// AppendToView draws an already-simulated PlayWorld in another view without
// advancing it or adding editor work to the game's particle counters.
func (s *Service) AppendToView(r *frame.Runner) {
	if r == nil || r.Scene() != s.scene || r.Renderer() == nil || !s.enabled || s.world.Replaying() || len(s.systems) == 0 {
		return
	}
	var m Stats
	s.appendTo(r, nil, &m)
}
func (s *Service) appendTo(r *frame.Runner, usage *sprite.SubmissionStats, m *Stats) {
	scene.Query1[ParticleSystem2D](s.scene).Each(func(h scene.NodeHandle, _ *ParticleSystem2D) {
		e := s.systems[h]
		if e == nil || !e.valid || len(e.pool.Particles()) == 0 || s.scene.Under3D(h) || r.Hidden(h) {
			return
		}
		e.extract(r, s.scene.Resolver(), s.scene.RenderWorld(h), s.scene.Layers(h), usage, m)
	})
}

type sortParticle struct {
	index int
	key   float32
	id    uint64
}

func compareParticles(a, b sortParticle) int {
	if c := cmp.Compare(b.key, a.key); c != 0 {
		return c
	}
	return cmp.Compare(a.id, b.id)
}

func (e *emitter) sortFor(v view.View, pose mathx.Mat3) {
	a := e.pool.Particles()
	e.order = e.order[:0]
	cam := v.CameraPosition()
	for i, p := range a {
		key := p.Age
		switch e.cfg.SortMode {
		case YoungestFirst:
			key = -key
		case Distance:
			pos := p.Position
			if e.cfg.Space == Local {
				pos = pose.Apply(pos)
			}
			dx, dy := pos.X-cam.X, pos.Y-cam.Y
			key = dx*dx + dy*dy
		}
		e.order = append(e.order, sortParticle{i, key, p.ID})
	}
	if !slices.IsSortedFunc(e.order, compareParticles) {
		slices.SortFunc(e.order, compareParticles)
	}
}

// drawMaterial returns nil for the renderer's ordinary sprite fallback. An
// invalid explicit asset still rejects the emitter instead of changing its look.
func drawMaterial(c *ParticleSystem2D, res asset.Resolver) (*material.Material, bool) {
	if !c.Material.IsZero() {
		m, ok := c.Material.Material(res)
		return m, ok && m != nil && m.Shader() != nil && m.Shader().Domain == shader.DomainSprite && m.Shader().Features&shader.FeatureLighting == 0
	}
	if src, ok := res.(asset.MaterialSource); ok {
		m, found := src.Material(asset.BuiltinMaterialUnlit)
		if found && m != nil && m.Shader() != nil && m.Shader().Features&shader.FeaturePixelCoverage != 0 {
			return m, true
		}
	}
	return nil, true
}

// Prepare the scene's particle programs while it is loading. Manual bursts
// also need a prepared material, even when nothing has been born yet.
func (s *Service) prepareMaterials() {
	if !s.enabled || s.world.Replaying() {
		return
	}
	r, ok := engine.ServiceOf[*frame.Runner](s.world, frame.ServiceID)
	if !ok || r == nil || r.Scene() != s.scene || r.Renderer() == nil {
		return
	}
	cache := r.Renderer().Materials()
	scene.Query1[ParticleSystem2D](s.scene).Each(func(h scene.NodeHandle, c *ParticleSystem2D) {
		if s.scene.Under3D(h) || r.Hidden(h) || s.reduced && c.ReducedMotion == Hide || c.Validate() != nil {
			return
		}
		if m, ok := drawMaterial(c, s.scene.Resolver()); ok && m != nil {
			cache.PrepareSpriteProgram(m)
		}
	})
}

func (e *emitter) extract(r *frame.Runner, res asset.Resolver, pose mathx.Mat3, layers uint64, usage *sprite.SubmissionStats, metrics *Stats) {
	c := &e.cfg
	a := e.pool.Particles()
	if !e.valid || len(a) == 0 || r == nil || r.Renderer() == nil {
		return
	}
	st := r.Counters()
	base := sprite.Draw{Profile: usage, VisibilityLayers: layers, Region: r.Renderer().White(), Size: mathx.V2(1, 1), Origin: mathx.V2(.5, .5), Layer: int16(c.Layer), Order: int16(c.Order)}
	if len(c.Frames) == 0 && !c.Sprite.IsZero() {
		reg, ok := c.Sprite.Region(res)
		if !ok {
			st.MissingAssets++
			return
		}
		base.Region = reg
	}
	mat, ok := drawMaterial(c, res)
	if !ok {
		st.MissingAssets++
		return
	}
	base.Material = mat
	base.PixelCoverage = true
	for i, v := range r.Views() {
		if v.Is3D() || layers&uint64(v.Layers) == 0 {
			continue
		}
		b := r.Bucket(i)
		bounds := v.VisibleBounds()
		start := time.Now()
		e.sortFor(v, pose)
		metrics.SortNS += uint64(time.Since(start).Nanoseconds())
		before := len(b.Sprites)
		for _, entry := range e.order {
			p := &a[entry.index]
			draw := base
			if idx := c.FrameIndex(p.Age); idx >= 0 {
				reg, ok := c.Frames[idx].Region(res)
				if !ok {
					st.MissingAssets++
					continue
				}
				draw.Region = reg
			}
			pos, linear, tint := p.Surface(c.Space, pose)
			draw.Tint = tint
			draw.SetTransform(mathx.Mat3{linear[0], linear[1], 0, linear[2], linear[3], 0, pos.X, pos.Y, 1})
			if (draw.Material == nil || !draw.Material.Shader().Vertex) && !culling.Visible(bounds, &draw, v.PixelsPerUnit) {
				continue
			}
			b.Sprites = append(b.Sprites, draw)
		}
		n := len(b.Sprites) - before
		if n > 0 {
			metrics.VisibleEmitters++
		} else {
			metrics.CulledEmitters++
		}
		metrics.Submitted += n
		st.Sprites += n
	}
}
