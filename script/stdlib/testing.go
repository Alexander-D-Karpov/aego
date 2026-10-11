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
	}, bind.Doc("True fails the test with msg when v is false."), bind.Params("v", "msg"),
		bind.Example("testing.True(hp > 0, \"player should be alive\")"))
	bind.Proc2(m, "False", func(c *bind.Ctx, v bool, msg string) {
		if v {
			c.Fail(message(msg, "expected false"))
		}
	}, bind.Doc("False fails the test with msg when v is true."), bind.Params("v", "msg"),
		bind.Example("testing.False(dead, \"player should be alive\")"))
	bind.Proc2(m, "Equal", func(c *bind.Ctx, got, want int64) {
		if got != want {
			c.Failf("got %d, want %d", got, want)
		}
	}, bind.Doc("Equal fails the test when two ints differ."), bind.Params("got", "want"),
		bind.Example("testing.Equal(score(), 100)"))
	bind.Proc3(m, "EqualFloat", func(c *bind.Ctx, got, want, eps float32) {
		if got != got || mathx.Abs(got-want) > eps {
			c.Failf("got %g, want %g ± %g", got, want, eps)
		}
	}, bind.Doc("EqualFloat fails the test when got is further than eps from want, or is NaN."), bind.Params("got", "want", "eps"),
		bind.Example("testing.EqualFloat(math.Sqrt(2), 1.4142, 0.001)"))
	bind.Proc2(m, "EqualString", func(c *bind.Ctx, got, want string) {
		if got != want {
			c.Failf("got %q, want %q", got, want)
		}
	}, bind.Doc("EqualString fails the test when two strings differ."), bind.Params("got", "want"),
		bind.Example("testing.EqualString(name(), \"Player\")"))
	bind.Proc1(m, "Fail", func(c *bind.Ctx, msg string) {
		c.Fail(message(msg, "failed"))
	}, bind.Doc("Fail stops the test and reports msg."), bind.Params("msg"),
		bind.Example("testing.Fail(\"unreachable\")"))
}
