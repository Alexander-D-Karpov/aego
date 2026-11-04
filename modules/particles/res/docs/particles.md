---
id: particles
try: try/ParticleSystem2D.aescene
try_select: particles.ParticleSystem2D
icon: comp.sprite
group: gameplay
title: CPU particle effects
order: 8
---
# CPU particle effects

In the Create palette, choose **Effects → Dust** for a manual burst, or **Smoke** for continuous emission. A burst works with Rate zero. World space keeps existing dust at the landing point; Local carries live particles with the emitter. Stop ends emission and lets the live particles fade. Clear removes them.

In the platformer, the player’s Landed signal calls Burst(8) on Dust. StartLantern has independent Smoke and Embers children. In the top-down example, footsteps come from presentation events in the existing Run clip.

The emitter uses its own seed. Changing the particle setting does not alter physics or gameplay RNG. Reduced Motion applies the policy selected by the author.

In a native Go system, use `sdk.ParticlesOf(ctx)` to get the particle service for that world. Call `fx.Burst(dustNode, 8)` on it; the service is nil when the module is absent.
