package stdlib

import (
	"aego/core/mathx"
	"aego/script/bind"
)

func message(msg, def string) string {
	if msg == "" {
		return def
	}
	return msg
}

func installTesting(r *bind.Registry) {
	m := r.Module(ModTesting)
	bind.Proc2(m, "True", func(c *bind.Ctx, v bool, msg string) {
		if !v {
			c.Fail(message(msg, "expected true"))
		}
	})
	bind.Proc2(m, "False", func(c *bind.Ctx, v bool, msg string) {
		if v {
			c.Fail(message(msg, "expected false"))
		}
	})
	bind.Proc2(m, "Equal", func(c *bind.Ctx, got, want int64) {
		if got != want {
			c.Failf("got %d, want %d", got, want)
		}
	})
	bind.Proc3(m, "EqualFloat", func(c *bind.Ctx, got, want, eps float32) {
		if got != got || mathx.Abs(got-want) > eps {
			c.Failf("got %g, want %g ± %g", got, want, eps)
		}
	})
	bind.Proc2(m, "EqualString", func(c *bind.Ctx, got, want string) {
		if got != want {
			c.Failf("got %q, want %q", got, want)
		}
	})
	bind.Proc1(m, "Fail", func(c *bind.Ctx, msg string) {
		c.Fail(message(msg, "failed"))
	})
}
