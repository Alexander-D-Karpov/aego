package stdlib

import "aego/script/bind"

func installTime(r *bind.Registry) {
	m := r.Module(ModTime)
	bind.Func0(m, "Delta", func(c *bind.Ctx) float32 { return c.Dt })
	bind.Func0(m, "Fixed", func(c *bind.Ctx) float32 { return c.Fixed })
	bind.Func0(m, "Elapsed", func(c *bind.Ctx) float64 { return c.Elapsed })
	bind.Func0(m, "Frame", func(c *bind.Ctx) int64 { return int64(c.Frame) })
}
