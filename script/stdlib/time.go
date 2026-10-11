package stdlib

import "aego/script/bind"

func installTime(r *bind.Registry) {
	m := r.Module(ModTime)
	bind.Func0(m, "Delta", func(c *bind.Ctx) float32 { return c.Dt },
		bind.Doc("Delta returns the seconds since the previous frame. Inside FixedUpdate it is the fixed step."),
		bind.Example("x += Speed * time.Delta()"))
	bind.Func0(m, "Fixed", func(c *bind.Ctx) float32 { return c.Fixed },
		bind.Doc("Fixed returns the fixed step length in seconds, 1/60 by default."),
		bind.Example("steps := int(1 / time.Fixed())"))
	bind.Func0(m, "Elapsed", func(c *bind.Ctx) float64 { return c.Elapsed },
		bind.Doc("Elapsed returns the seconds since Play started."),
		bind.Example("y := math.Sin(time.Elapsed() * 3)"))
	bind.Func0(m, "Frame", func(c *bind.Ctx) int64 { return int64(c.Frame) },
		bind.Doc("Frame returns the number of frames since Play started."),
		bind.Example("if time.Frame() % 60 == 0 {\n\tlog.Info(\"tick\")\n}"))
}
