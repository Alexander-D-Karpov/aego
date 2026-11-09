package gpupreview

import (
	"fmt"

	"aego/art"
	"aego/asset"
	"aego/core/mathx"
	"aego/modules/particles"
	"aego/scene"
)

// configureParticles owns the preview's runtime emitter binding. The source
// document and the cached engine presets remain independent of presentation.
func (a *App) configureParticles(templateID, presetName string, preset particles.ParticleSystem2D, restart, clearRateOverride bool, spriteWidth float32) error {
	p, o := a.data, a.options
	refs := p.frames[a.status.Frame]
	pc, _ := scene.Get[particles.ParticleSystem2D](a.sc, a.emitter)
	changeTemplate := a.particleDocument != p.info.ID || a.particleTemplate != templateID
	restart = restart || changeTemplate
	if templateID != "" {
		// Keep the real engine preset's burst schedule, curves, materials and
		// lifecycle. The authored flipbook replaces only its image references.
		// Apply defaults only for a new document/template, not each edited pixel
		// or playback frame: host-controlled preview settings stay intact.
		if changeTemplate {
			frames := pc.Frames[:0]
			*pc = preset
			pc.Frames = frames
		}
		if o.RateOverride != nil {
			pc.Rate = *o.RateOverride
		} else if clearRateOverride {
			pc.Rate = preset.Rate
		}
		a.particleExtent = particleFitExtent(*pc)
	} else {
		if a.particleTemplate != "" {
			*pc = particles.DefaultParticleSystem2D()
		}
		pc.Material.ID, _ = asset.BuiltinMaterialNamed("Unlit")
		pc.Rate = o.Rate
		pc.Lifetime = mathx.V2(o.Lifetime, o.Lifetime)
		pc.Speed = mathx.V2(o.Speed*.6, o.Speed)
		pc.Size = mathx.V2(spriteWidth*.3, spriteWidth*.5)
		pc.MaxParticles = 512
		pc.Spread = mathx.Pi * 2
	}
	pc.Frames = pc.Frames[:0]
	duration := 0
	for _, fr := range p.info.Frames {
		pc.Frames = append(pc.Frames, asset.Ref[asset.Texture]{ID: p.frames[fr.ID][art.Albedo]})
		duration += fr.DurationMS
	}
	pc.Sprite.ID = refs[art.Albedo]
	pc.FrameFPS = float32(len(p.info.Frames)) * 1000 / float32(max(duration, 1))
	a.particleDocument, a.particleTemplate = p.info.ID, templateID
	a.status.EffectTemplate, a.status.ParticlePreset = templateID, presetName
	a.status.ParticleRate = pc.Rate
	a.sc.Flush()
	if restart && o.Mode == "particles" && !o.Paused {
		return a.RestartParticles()
	}
	return nil
}

func (a *App) effectPreset(id string) (particles.ParticleSystem2D, string, error) {
	template, ok := art.EffectTemplateOf(id)
	if !ok {
		return particles.ParticleSystem2D{}, "", fmt.Errorf("unknown effect template %q", id)
	}
	if preset, ok := a.presets[id]; ok {
		return preset, template.ParticlePreset, nil
	}
	data, err := particles.PresetSource(template.ParticlePreset)
	if err != nil {
		return particles.ParticleSystem2D{}, "", err
	}
	preset, err := particles.ReadPreset(a.types, a.loader, data)
	if err != nil {
		return particles.ParticleSystem2D{}, "", err
	}
	a.presets[id] = preset
	return preset, template.ParticlePreset, nil
}

// Fit world-space presets with the camera, leaving their simulation units,
// gravity and authored size curves unchanged.
func particleFitExtent(p particles.ParticleSystem2D) float32 {
	radius := max(mathx.Abs(p.Offset.X), mathx.Abs(p.Offset.Y)) + p.Size.Y
	switch p.Shape {
	case particles.Circle:
		radius += p.Radius
	case particles.Rect:
		radius += max(p.RectSize.X, p.RectSize.Y) * .5
	case particles.Edge:
		radius += max(mathx.Abs(p.EdgeA.X), mathx.Abs(p.EdgeA.Y), mathx.Abs(p.EdgeB.X), mathx.Abs(p.EdgeB.Y))
	}
	life := p.Lifetime.Y
	radius += p.Speed.Y*life + .5*max(mathx.Abs(p.Gravity.X), mathx.Abs(p.Gravity.Y))*life*life
	return max(1, radius*2.2)
}

// RestartParticles replays one-shot effects through the same CPU particle
// service used by a game. It does not edit or reset the source document.
func (a *App) RestartParticles() error {
	if a.closed || a.data == nil || a.options.Mode != "particles" || a.options.Paused {
		return fmt.Errorf("particle preview is not active")
	}
	p := particles.Of(a.world)
	if p == nil {
		return fmt.Errorf("particle preview service is unavailable")
	}
	// Service.target checks Scene.EffectiveEnabled before it asks for world
	// transforms. Fresh nodes and a just-enabled emitter have not yet passed
	// RenderPrepare, so Flush alone is insufficient and would drop this event.
	// Resolve the private world's activation/pose without advancing simulation.
	a.sc.Flush()
	a.sc.UpdateTransforms()
	p.Restart(a.emitter)
	// Match the existing native particle preview transport: an event-driven
	// Dust preset has neither a continuous rate nor authored burst keys. Replay
	// supplies its explicit input event, without rewriting the preset itself.
	if pc, ok := scene.Get[particles.ParticleSystem2D](a.sc, a.emitter); ok && !pc.PlayOnStart && pc.Rate == 0 && len(pc.Bursts) == 0 {
		p.Burst(a.emitter, 8)
	}
	return nil
}
