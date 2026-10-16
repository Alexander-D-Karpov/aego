package stdlib

import (
	"aego/core/mathx"
	"aego/engine"
	"aego/platform/input"
	"aego/scene/frame"
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
	bind.Describe(m, "Key", bind.Doc("Key is a physical key, independent of the keyboard layout. Use input.Key.Space or the constant input.KeySpace."))
	bind.Enum[input.Button](m, "Button", buttons...)
	bind.Describe(m, "Button", bind.Doc("Button is a mouse button. Use input.Button.Left or the constant input.MouseLeft."))
	for _, k := range keys {
		bind.Const(m, "Key"+k.Name, input.Scancode(k.Value), bind.Doc("The "+k.Name+" key."))
	}
	for _, b := range buttons {
		bind.Const(m, "Mouse"+b.Name, input.Button(b.Value), bind.Doc("The "+b.Name+" mouse button."))
	}
	bind.Func1(m, "Down", func(c *bind.Ctx, k input.Scancode) bool {
		return c.Input != nil && c.Input.KeyDown(k)
	}, bind.Doc("Down reports whether the key is held this frame."), bind.Params("key"),
		bind.Example("if input.Down(input.KeyShift) {\n\tspeed *= 2\n}"))
	bind.Func1(m, "Pressed", func(c *bind.Ctx, k input.Scancode) bool {
		return c.Input != nil && c.Input.KeyPressed(k)
	}, bind.Doc("Pressed reports whether the key went down this frame. It is true for exactly one frame per press."), bind.Params("key"),
		bind.Example("if input.Pressed(input.KeySpace) {\n\tjump()\n}"))
	bind.Func1(m, "Released", func(c *bind.Ctx, k input.Scancode) bool {
		return c.Input != nil && c.Input.KeyReleased(k)
	}, bind.Doc("Released reports whether the key went up this frame."), bind.Params("key"),
		bind.Example("if input.Released(input.KeySpace) {\n\tcutJump()\n}"))
	bind.Func1(m, "MouseDown", func(c *bind.Ctx, b input.Button) bool {
		return c.Input != nil && c.Input.ButtonDown(b)
	}, bind.Doc("MouseDown reports whether the mouse button is held."), bind.Params("button"),
		bind.Example("if input.MouseDown(input.MouseLeft) {\n\tfire()\n}"))
	bind.Func1(m, "MousePressed", func(c *bind.Ctx, b input.Button) bool {
		return c.Input != nil && c.Input.ButtonPressed(b)
	}, bind.Doc("MousePressed reports whether the mouse button went down this frame."), bind.Params("button"),
		bind.Example("if input.MousePressed(input.MouseRight) {\n\taim()\n}"))
	bind.Func0(m, "MousePos", func(c *bind.Ctx) mathx.Vec2 {
		if c.Input == nil {
			return mathx.Vec2{}
		}
		return c.Input.MousePos()
	}, bind.Doc("MousePos returns the cursor position in logical pixels of the game area, (0, 0) at its top-left corner. Use MouseWorld for world coordinates."),
		bind.Example("p := input.MousePos()"))
	bind.Func0(m, "MouseWorld", func(c *bind.Ctx) mathx.Vec2 {
		if c.Input == nil {
			return mathx.Vec2{}
		}
		r, ok := engine.ServiceOf[*frame.Runner](c.World, frame.ServiceID)
		if !ok || r == nil {
			return mathx.Vec2{}
		}
		p, _ := r.ScreenToWorld(c.Input.MousePos().Scale(r.PixelScale()))
		return p
	}, bind.Doc("MouseWorld returns the cursor position in world units, seen through the camera under the cursor. Letterboxing and window or panel resizes are accounted for."),
		bind.Example("target := input.MouseWorld()"))
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
	}, bind.Doc("Axis returns -1, 0 or 1 for a named axis.\nAxes are declared in project.aego → input → axes; horizontal and vertical exist by default."),
		bind.Params("name"), bind.Example("x := input.Axis(\"horizontal\")"))
}
