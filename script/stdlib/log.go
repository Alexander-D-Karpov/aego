package stdlib

import "aego/script/bind"

func installLog(r *bind.Registry) {
	m := r.Module(ModLog)
	bind.Proc1(m, "Debug", func(c *bind.Ctx, s string) {
		if c.Log != nil {
			c.Log.Debug(s)
		}
	})
	bind.Proc1(m, "Info", func(c *bind.Ctx, s string) {
		if c.Log != nil {
			c.Log.Info(s)
		}
	})
	bind.Proc1(m, "Warn", func(c *bind.Ctx, s string) {
		if c.Log != nil {
			c.Log.Warn(s)
		}
	})
	bind.Proc1(m, "Error", func(c *bind.Ctx, s string) {
		if c.Log != nil {
			c.Log.Error(s)
		}
	})
}
