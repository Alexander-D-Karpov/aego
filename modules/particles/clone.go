package particles

import "slices"

// cloneParticleSystem detaches every mutable field while preserving scalar
// state and cached AssetRefs. Keep this in sync with ParticleSystem2D.
func cloneParticleSystem(dst, src *ParticleSystem2D) {
	*dst = *src
	dst.Frames = slices.Clone(src.Frames)
	dst.ColorOverLife = slices.Clone(src.ColorOverLife)
	dst.Bursts = slices.Clone(src.Bursts)
	dst.VelocityOverLife = src.VelocityOverLife.Clone()
	dst.SizeOverLife = src.SizeOverLife.Clone()
	dst.AlphaOverLife = src.AlphaOverLife.Clone()
}
