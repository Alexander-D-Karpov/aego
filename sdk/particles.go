package sdk

import "aego/modules/particles"

type (
	Particles        = particles.Service
	ParticleSystem2D = particles.ParticleSystem2D
	ParticleStats    = particles.Stats
	ParticleColorKey = particles.ColorKey
)

const (
	ParticlePoint         = particles.Point
	ParticleCircle        = particles.Circle
	ParticleRect          = particles.Rect
	ParticleEdge          = particles.Edge
	ParticleSpriteMask    = particles.SpriteMask
	ParticleLocal         = particles.Local
	ParticleWorld         = particles.World
	ParticleGameTime      = particles.Game
	ParticleUnscaled      = particles.Unscaled
	ParticleAge           = particles.Age
	ParticleYoungestFirst = particles.YoungestFirst
	ParticleDistance      = particles.Distance
	ParticleKeep          = particles.Keep
	ParticleReduce        = particles.Reduce
	ParticleHide          = particles.Hide
)

func ParticlesOf(ctx *Context) *Particles {
	if ctx == nil {
		return nil
	}
	return particles.Of(ctx.World)
}

func DefaultParticleSystem2D() ParticleSystem2D { return particles.DefaultParticleSystem2D() }
