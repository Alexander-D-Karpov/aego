package particles

import (
	"aego/engine/module"
	"aego/script/bind"
)

func registerMethods(b *module.ComponentBuilder[ParticleSystem2D]) {
	b.Proc0("Play", func(c *bind.Ctx, _ *ParticleSystem2D) {
		if s := Of(c.World); s != nil {
			s.Play(c.Recv)
		}
	}, bind.Doc("Starts automatic emission. Repeating Play while emitting does nothing."))
	b.Proc0("Restart", func(c *bind.Ctx, _ *ParticleSystem2D) {
		if s := Of(c.World); s != nil {
			s.Restart(c.Recv)
		}
	}, bind.Doc("Clears live particles and starts a new emission schedule."))
	b.Proc1("Burst", func(c *bind.Ctx, _ *ParticleSystem2D, n int64) {
		if s := Of(c.World); s != nil && n > 0 {
			s.Burst(c.Recv, int(min(n, int64(^uint(0)>>1))))
		}
	}, bind.Doc("Requests count births at the emitter. Does not start automatic emission."), bind.Params("count"), bind.Example("Dust.Get<ParticleSystem2D>().Burst(8)"))
	b.Proc0("Stop", func(c *bind.Ctx, _ *ParticleSystem2D) {
		if s := Of(c.World); s != nil {
			s.Stop(c.Recv, false)
		}
	}, bind.Doc("Stops new emissions and lets live particles drain."))
	b.Proc0("StopAndClear", func(c *bind.Ctx, _ *ParticleSystem2D) {
		if s := Of(c.World); s != nil {
			s.Stop(c.Recv, true)
		}
	}, bind.Doc("Stops automatic emission and immediately removes living particles."))
	b.Proc0("Clear", func(c *bind.Ctx, _ *ParticleSystem2D) {
		if s := Of(c.World); s != nil {
			s.Clear(c.Recv)
		}
	}, bind.Doc("Removes live particles without changing the emission schedule."))
	b.Method0("Count", func(c *bind.Ctx, _ *ParticleSystem2D) int64 {
		if s := Of(c.World); s != nil {
			return int64(s.Count(c.Recv))
		}
		return 0
	}, bind.Doc("Returns the current number of living particles on the emitter."))
	b.Method0("Playing", func(c *bind.Ctx, _ *ParticleSystem2D) bool { s := Of(c.World); return s != nil && s.Playing(c.Recv) }, bind.Doc("Reports whether the schedule is running or particles remain."))
	b.Method0("Emitting", func(c *bind.Ctx, _ *ParticleSystem2D) bool { s := Of(c.World); return s != nil && s.Emitting(c.Recv) }, bind.Doc("Reports whether automatic emission is running."))
}
