package stdlib

import (
	"aego/core/clock"
	"aego/script/bind"
)

func span(c *bind.Ctx, d clock.Domain) (clock.Span, bool) {
	if c.Time == nil {
		return clock.Span{}, false
	}
	return c.Time.Span(d), true
}

func installTime(r *bind.Registry) {
	m := r.Module(ModTime)
	bind.Func0(m, "Delta", func(c *bind.Ctx) float32 { return c.Dt },
		bind.Doc("Delta returns the game seconds since the previous frame: real time × Scale, 0 while paused. Inside FixedUpdate it is the fixed step."),
		bind.Example("x += Speed * time.Delta()"))
	bind.Func0(m, "Unscaled", func(c *bind.Ctx) float32 {
		if s, ok := span(c, clock.Real); ok {
			return float32(s.Delta)
		}
		return c.Dt
	}, bind.Doc("Unscaled returns the real seconds since the previous frame, ignoring Scale and pause."),
		bind.Example("menuFade += time.Unscaled()"))
	bind.Func0(m, "Fixed", func(c *bind.Ctx) float32 {
		if c.Time != nil {
			return float32(c.Time.FixedDelta())
		}
		return c.Fixed
	}, bind.Doc("Fixed returns the fixed step length in seconds, 1/60 by default. Scale does not change it; it changes how many steps run."),
		bind.Example("steps := int(1 / time.Fixed())"))
	bind.Func0(m, "Elapsed", func(c *bind.Ctx) float64 { return c.Elapsed },
		bind.Doc("Elapsed returns the game seconds since Play started. It stops while paused and follows Scale."),
		bind.Example("y := math.Sin(time.Elapsed() * 3)"))
	bind.Func0(m, "UnscaledElapsed", func(c *bind.Ctx) float64 {
		if s, ok := span(c, clock.Real); ok {
			return s.Elapsed
		}
		return c.Elapsed
	}, bind.Doc("UnscaledElapsed returns the real seconds since Play started, pauses included."),
		bind.Example("blink := math.Sin(time.UnscaledElapsed() * 6)"))
	bind.Func0(m, "Scale", func(c *bind.Ctx) float32 {
		if c.Time == nil {
			return 1
		}
		return float32(c.Time.Scale())
	}, bind.Doc("Scale returns the time scale: 1 is normal speed, 0.5 is half speed, 0 freezes game time without pausing."),
		bind.Example("if time.Scale() < 1 {"))
	bind.Proc1(m, "SetScale", func(c *bind.Ctx, s float32) {
		if c.Time == nil {
			c.Fail("this script has no world clock")
			return
		}
		if s < 0 || s > clock.MaxScale {
			c.Failf("time scale %g is outside 0..8", s)
			return
		}
		c.Time.SetScale(float64(s))
	}, bind.Doc("SetScale sets the time scale from 0 to 8. The fixed step stays the same, so the simulation stays stable; only the number of steps changes."),
		bind.Params("scale"), bind.Example("time.SetScale(0.25)"))
	bind.Func0(m, "Frame", func(c *bind.Ctx) int64 { return int64(c.Frame) },
		bind.Doc("Frame returns the number of frames this world has run, paused frames included."),
		bind.Example("if time.Frame() % 60 == 0 {\n\tlog.Info(\"tick\")\n}"))
	bind.Func0(m, "Tick", func(c *bind.Ctx) int64 {
		if c.Time == nil {
			return 0
		}
		return int64(c.Time.Tick())
	}, bind.Doc("Tick returns the number of the current or last fixed step. It is the same number the profiler and the Event Monitor show."),
		bind.Example("log.Info(\"landed on tick \" + string(time.Tick()))"))
	bind.Func0(m, "IsPaused", func(c *bind.Ctx) bool {
		return c.Time != nil && c.Time.Paused()
	}, bind.Doc("IsPaused reports whether game time is paused."),
		bind.Example("if time.IsPaused() {"))
	bind.Proc1(m, "SetPaused", func(c *bind.Ctx, on bool) {
		if c.Time == nil {
			c.Fail("this script has no world clock")
			return
		}
		c.Time.SetPaused(on)
	}, bind.Doc("SetPaused pauses or resumes game time. While paused, Update, FixedUpdate and game-time coroutines of ordinary scripts do not run."),
		bind.Params("paused"), bind.Example("time.SetPaused(true)"))
}
