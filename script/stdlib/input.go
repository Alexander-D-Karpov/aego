package stdlib

import (
	"aego/core/mathx"
	"aego/platform/input"
	"aego/script/abi"
	"aego/script/bind"
)

func installInput(r *bind.Registry, axes input.AxisConfig) {
	m := r.Module(ModInput)
	var keys []abi.EnumMember
	for k := input.Scancode(1); k < input.ScancodeCount; k++ {
		if name, ok := input.KeyName(k); ok {
			keys = append(keys, bind.Member(name, k))
		}
	}
	buttons := []abi.EnumMember{
		bind.Member("Left", input.MouseLeft),
		bind.Member("Right", input.MouseRight),
		bind.Member("Middle", input.MouseMiddle),
		bind.Member("X1", input.MouseX1),
		bind.Member("X2", input.MouseX2),
	}
	bind.Enum[input.Scancode](m, "Key", keys...)
	bind.Enum[input.Button](m, "Button", buttons...)
	for _, k := range keys {
		bind.Const(m, "Key"+k.Name, input.Scancode(k.Value))
	}
	for _, b := range buttons {
		bind.Const(m, "Mouse"+b.Name, input.Button(b.Value))
	}
	bind.Func1(m, "Down", func(c *bind.Ctx, k input.Scancode) bool {
		return c.Input != nil && c.Input.KeyDown(k)
	})
	bind.Func1(m, "Pressed", func(c *bind.Ctx, k input.Scancode) bool {
		return c.Input != nil && c.Input.KeyPressed(k)
	})
	bind.Func1(m, "Released", func(c *bind.Ctx, k input.Scancode) bool {
		return c.Input != nil && c.Input.KeyReleased(k)
	})
	bind.Func1(m, "MouseDown", func(c *bind.Ctx, b input.Button) bool {
		return c.Input != nil && c.Input.ButtonDown(b)
	})
	bind.Func1(m, "MousePressed", func(c *bind.Ctx, b input.Button) bool {
		return c.Input != nil && c.Input.ButtonPressed(b)
	})
	bind.Func0(m, "MousePos", func(c *bind.Ctx) mathx.Vec2 {
		if c.Input == nil {
			return mathx.Vec2{}
		}
		return c.Input.MousePos()
	})
	bind.Func1(m, "Axis", func(c *bind.Ctx, name string) float32 {
		a, ok := axes.Lookup(name)
		if !ok {
			c.Failf("unknown input axis %q, define it in project.aego", name)
			return 0
		}
		if c.Input == nil {
			return 0
		}
		return a.Value(c.Input)
	})
}
