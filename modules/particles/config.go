package particles

import (
	"errors"
	"slices"

	"aego/asset"
	"aego/core/curve"
	"aego/core/mathx"
)

func sameCurve(a, b curve.Curve) bool {
	return a.Ease == b.Ease && a.Pre == b.Pre && a.Post == b.Post && slices.Equal(a.Keys, b.Keys)
}

func (c *ParticleSystem2D) same(b *ParticleSystem2D) bool {
	return c.PlayOnStart == b.PlayOnStart && c.Loop == b.Loop && c.Duration == b.Duration && c.TimeDomain == b.TimeDomain && c.Seed == b.Seed && c.MaxParticles == b.MaxParticles &&
		c.Rate == b.Rate && c.Shape == b.Shape && c.Offset == b.Offset && c.RectSize == b.RectSize && c.Radius == b.Radius && c.Lifetime == b.Lifetime && c.Speed == b.Speed &&
		c.Size == b.Size && c.Rotation == b.Rotation && c.Direction == b.Direction && c.Spread == b.Spread && c.Color == b.Color && c.Gravity == b.Gravity &&
		c.RotationSpeed == b.RotationSpeed && c.Space == b.Space && c.Layer == b.Layer && c.Order == b.Order && c.Material.ID == b.Material.ID && c.Sprite.ID == b.Sprite.ID &&
		c.Prewarm == b.Prewarm && c.EdgeA == b.EdgeA && c.EdgeB == b.EdgeB && c.Mask.ID == b.Mask.ID && c.MaskThreshold == b.MaskThreshold && c.FrameFPS == b.FrameFPS &&
		c.FrameLoop == b.FrameLoop && c.SortMode == b.SortMode && c.ReducedMotion == b.ReducedMotion && c.ReducedRate == b.ReducedRate && c.Drag == b.Drag &&
		c.NoiseStrength == b.NoiseStrength && c.NoiseFrequency == b.NoiseFrequency && c.KillYEnabled == b.KillYEnabled && c.KillY == b.KillY && c.PlaneEnabled == b.PlaneEnabled &&
		c.PlaneNormal == b.PlaneNormal && c.PlaneDistance == b.PlaneDistance && sameCurve(c.SizeOverLife, b.SizeOverLife) && sameCurve(c.AlphaOverLife, b.AlphaOverLife) &&
		sameCurve(c.VelocityOverLife, b.VelocityOverLife) && slices.Equal(c.ColorOverLife, b.ColorOverLife) && slices.Equal(c.Bursts, b.Bursts) &&
		slices.EqualFunc(c.Frames, b.Frames, func(a, b asset.Ref[asset.Texture]) bool { return a.ID == b.ID })
}

func (dst *ParticleSystem2D) copyFrom(c *ParticleSystem2D) {
	next := *c
	next.Bursts = append(dst.Bursts[:0], c.Bursts...)
	next.Frames = append(dst.Frames[:0], c.Frames...)
	next.ColorOverLife = append(dst.ColorOverLife[:0], c.ColorOverLife...)
	next.SizeOverLife.Keys = append(dst.SizeOverLife.Keys[:0], c.SizeOverLife.Keys...)
	next.AlphaOverLife.Keys = append(dst.AlphaOverLife.Keys[:0], c.AlphaOverLife.Keys...)
	next.VelocityOverLife.Keys = append(dst.VelocityOverLife.Keys[:0], c.VelocityOverLife.Keys...)
	*dst = next
}

func hasFrame(ref *asset.Ref[asset.Texture], res asset.Resolver) bool {
	if meta, ok := res.(asset.TextureMetadataSource); ok {
		_, found := meta.TextureMetadata(ref.ID)
		return found
	}
	_, found := ref.Region(res)
	return found
}

func resources(c *ParticleSystem2D, res asset.Resolver) ([]mathx.Vec2, error) {
	if res != nil {
		if !c.Sprite.IsZero() && !hasFrame(&c.Sprite, res) {
			return nil, errors.New("particle sprite is unresolved")
		}
		for i := range c.Frames {
			if !hasFrame(&c.Frames[i], res) {
				return nil, errors.New("particle flipbook contains an unresolved frame")
			}
		}
		if !c.Material.IsZero() {
			if _, ok := drawMaterial(c, res); !ok {
				return nil, errors.New("particle material must resolve to an unlit sprite-domain shader")
			}
		}
	}
	if c.Shape == SpriteMask {
		if src, ok := res.(asset.ParticleMaskSource); ok {
			threshold := mathx.Clamp(c.MaskThreshold, 0, 1)
			minAlpha := uint8(threshold * 255)
			if float32(minAlpha)/255 < threshold {
				minAlpha++
			}
			if points, ok := src.ParticleMask(c.Mask.ID, minAlpha); ok {
				return points, nil
			}
		}
		return nil, errors.New("particle sprite mask or its prepared alpha table is missing")
	}
	return nil, nil
}

func (e *emitter) configure(c *ParticleSystem2D, res asset.Resolver) (bool, error) {
	var gen uint32
	if res != nil {
		gen = res.Generation()
	}
	if !e.dirty && e.tried && e.generation == gen && e.attempt.same(c) {
		return false, e.err
	}
	e.dirty = false
	e.tried = true
	e.generation = gen
	if e.err = c.Validate(); e.err != nil {
		e.tried = false
		return false, e.err
	}
	e.attempt.copyFrom(c)
	mask, err := resources(c, res)
	e.err = err
	if err != nil {
		return false, err
	}
	reset := !e.valid || e.cfg.Space != c.Space || e.cfg.MaxParticles != c.MaxParticles || e.cfg.Seed != c.Seed
	if reset {
		if err := e.pool.Configure(int(c.MaxParticles)); err != nil {
			e.err = err
			return false, err
		}
	}
	e.cfg.copyFrom(c)
	if cap(e.order) != int(c.MaxParticles) {
		e.order = make([]sortParticle, 0, c.MaxParticles)
	}
	e.valid = true
	e.mask = mask
	e.pool.SetMask(mask)
	return reset, nil
}

func (e *emitter) start(seed uint64, reduced bool, pose mathx.Mat3, play bool) {
	e.pool.Reset(seed)
	e.pool.SetMask(e.mask)
	e.pool.SetReduced(&e.cfg, reduced)
	if play {
		e.play(pose)
	}
}

func (e *emitter) play(pose mathx.Mat3) {
	if e.pool.Emitting() {
		return
	}
	e.pool.Play()
	if len(e.pool.Particles()) == 0 {
		e.pool.Prewarm(&e.cfg, pose)
	}
}

func CheckResources(c *ParticleSystem2D, res asset.Resolver) error {
	_, err := resources(c, res)
	return err
}
