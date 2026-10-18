package input

import (
	"aego/core/mathx"
	"aego/engine"
	raw "aego/platform/input"
	"aego/scene/frame"
	"aego/script/abi"
	"aego/script/bind"
)

func InstallBindings(r *bind.Registry) {
	installScript(r.Module(ScriptModule))
}

func installScript(m *bind.Module) {
	installRaw(m)
	installActions(m)
}

func installRaw(m *bind.Module) {
	var keys []abi.EnumMember
	for k := raw.Scancode(1); k < raw.ScancodeCount; k++ {
		if name, ok := raw.KeyName(k); ok {
			keys = append(keys, bind.Member(name, k))
		}
	}
	buttons := []abi.EnumMember{
		bind.Member("Left", raw.MouseLeft),
		bind.Member("Right", raw.MouseRight),
		bind.Member("Middle", raw.MouseMiddle),
		bind.Member("X1", raw.MouseX1),
		bind.Member("X2", raw.MouseX2),
	}
	bind.Enum[raw.Scancode](m, "Key", keys...)
	bind.Describe(m, "Key", bind.Doc("Key is a physical key, independent of the keyboard layout. Prefer actions; raw keys are for prototypes."))
	bind.Enum[raw.Button](m, "Button", buttons...)
	bind.Describe(m, "Button", bind.Doc("Button is a mouse button."))
	for _, k := range keys {
		bind.Const(m, "Key"+k.Name, raw.Scancode(k.Value), bind.Doc("The "+k.Name+" key."))
	}
	for _, b := range buttons {
		bind.Const(m, "Mouse"+b.Name, raw.Button(b.Value), bind.Doc("The "+b.Name+" mouse button."))
	}
	bind.Func1(m, "Down", func(c *bind.Ctx, k raw.Scancode) bool {
		return c.Input != nil && c.Input.KeyDown(k)
	}, bind.Doc("Down reports whether a raw key is held. For prototypes; release builds use actions."), bind.Params("key"),
		bind.Example("if input.Down(input.KeyShift) {"))
	bind.Func1(m, "KeyPressed", func(c *bind.Ctx, k raw.Scancode) bool {
		return c.Input != nil && c.Input.KeyPressed(k)
	}, bind.Doc("KeyPressed reports whether a raw key went down this frame. For prototypes."), bind.Params("key"),
		bind.Example("if input.KeyPressed(input.KeyF5) {"))
	bind.Func1(m, "KeyReleased", func(c *bind.Ctx, k raw.Scancode) bool {
		return c.Input != nil && c.Input.KeyReleased(k)
	}, bind.Doc("KeyReleased reports whether a raw key went up this frame. For prototypes."), bind.Params("key"),
		bind.Example("if input.KeyReleased(input.KeySpace) {"))
	bind.Func1(m, "MouseDown", func(c *bind.Ctx, b raw.Button) bool {
		return c.Input != nil && c.Input.ButtonDown(b)
	}, bind.Doc("MouseDown reports whether a mouse button is held."), bind.Params("button"),
		bind.Example("if input.MouseDown(input.MouseLeft) {"))
	bind.Func1(m, "MousePressed", func(c *bind.Ctx, b raw.Button) bool {
		return c.Input != nil && c.Input.ButtonPressed(b)
	}, bind.Doc("MousePressed reports whether a mouse button went down this frame."), bind.Params("button"),
		bind.Example("if input.MousePressed(input.MouseRight) {"))
	bind.Func0(m, "MousePos", func(c *bind.Ctx) mathx.Vec2 {
		if c.Input == nil {
			return mathx.Vec2{}
		}
		return c.Input.MousePos()
	}, bind.Doc("MousePos returns the cursor in logical pixels of the game area."), bind.Example("p := input.MousePos()"))
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
	}, bind.Doc("MouseWorld returns the cursor in world units through the camera under it."), bind.Example("target := input.MouseWorld()"))
}

func lookup(c *bind.Ctx, p int64, name string) (*Service, int, bool) {
	s := Of(c.World)
	if s == nil {
		return nil, 0, false
	}
	if !s.Has(name) {
		c.Failf("unknown input action %q, declare it in Project Settings → Input", name)
		return nil, 0, false
	}
	if p < 0 || int(p) >= s.Players() {
		c.Failf("input player %d is out of range, the project has %d", p, s.Players())
		return nil, 0, false
	}
	return s, int(p), true
}

func installActions(m *bind.Module) {
	bind.Func1(m, "Pressed", func(c *bind.Ctx, a string) bool {
		s, p, ok := lookup(c, 0, a)
		return ok && s.PressedP(p, a)
	}, bind.Doc("Pressed reports whether the action started. In Update it is true for one frame; in FixedUpdate for the first fixed step after the press."),
		bind.Params("action"), bind.Example("if input.Pressed(\"Attack\") {"))
	bind.Func1(m, "Released", func(c *bind.Ctx, a string) bool {
		s, p, ok := lookup(c, 0, a)
		return ok && s.ReleasedP(p, a)
	}, bind.Doc("Released reports whether the action ended this frame or fixed step."), bind.Params("action"),
		bind.Example("if input.Released(\"Jump\") {"))
	bind.Func1(m, "Held", func(c *bind.Ctx, a string) bool {
		s, p, ok := lookup(c, 0, a)
		return ok && s.HeldP(p, a)
	}, bind.Doc("Held reports whether the action is active."), bind.Params("action"), bind.Example("if input.Held(\"Run\") {"))
	bind.Func1(m, "Axis", func(c *bind.Ctx, a string) float32 {
		s, p, ok := lookup(c, 0, a)
		if !ok {
			return 0
		}
		return s.AxisP(p, a)
	}, bind.Doc("Axis returns the value of an axis action from -1 to 1, or the press strength of a button action."), bind.Params("action"),
		bind.Example("x := input.Axis(\"Move\")"))
	bind.Func1(m, "Axis2", func(c *bind.Ctx, a string) mathx.Vec2 {
		s, p, ok := lookup(c, 0, a)
		if !ok {
			return mathx.Vec2{}
		}
		return s.Axis2P(p, a)
	}, bind.Doc("Axis2 returns a 2D action as a vector no longer than 1; Y grows downward like the 2D world."), bind.Params("action"),
		bind.Example("v := input.Axis2(\"Move\") * Speed"))
	bind.Func0(m, "Pointer", func(c *bind.Ctx) mathx.Vec2 {
		if s := Of(c.World); s != nil {
			return s.Pointer()
		}
		return mathx.Vec2{}
	}, bind.Doc("Pointer returns the cursor in logical pixels."), bind.Example("p := input.Pointer()"))
	bind.Func0(m, "PointerDelta", func(c *bind.Ctx) mathx.Vec2 {
		if s := Of(c.World); s != nil {
			return s.PointerDelta()
		}
		return mathx.Vec2{}
	}, bind.Doc("PointerDelta returns how far the cursor moved this frame."), bind.Example("look += input.PointerDelta()"))
	bind.Func2(m, "Buffered", func(c *bind.Ctx, a string, window float32) bool {
		s, p, ok := lookup(c, 0, a)
		return ok && s.BufferedP(p, a, window)
	}, bind.Doc("Buffered reports whether the action was pressed in the last window game seconds and not consumed. A window of 0 uses the action's buffer setting."),
		bind.Params("action", "window"), bind.Example("if grounded && input.Buffered(\"Jump\", 0) {\n\tinput.Consume(\"Jump\")\n}"))
	bind.Proc1(m, "Consume", func(c *bind.Ctx, a string) {
		if s, p, ok := lookup(c, 0, a); ok {
			s.ConsumeP(p, a)
		}
	}, bind.Doc("Consume clears the press and the buffer of the action, so later readers this frame do not see it."), bind.Params("action"),
		bind.Example("input.Consume(\"Jump\")"))
	bind.Proc1(m, "PushContext", func(c *bind.Ctx, name string) {
		if s := Of(c.World); s != nil && !s.PushContext(name) {
			c.Failf("unknown input context %q", name)
		}
	}, bind.Doc("PushContext puts an input context on top of the stack."), bind.Params("context"), bind.Example("input.PushContext(\"Menu\")"))
	bind.Proc0(m, "PopContext", func(c *bind.Ctx) {
		if s := Of(c.World); s != nil {
			s.PopContext()
		}
	}, bind.Doc("PopContext removes the top input context."), bind.Example("input.PopContext()"))
	bind.Proc1(m, "SetContext", func(c *bind.Ctx, name string) {
		if s := Of(c.World); s != nil && !s.SetContext(name) {
			c.Failf("unknown input context %q", name)
		}
	}, bind.Doc("SetContext replaces the whole stack with one context."), bind.Params("context"), bind.Example("input.SetContext(\"Gameplay\")"))
	bind.Func2(m, "PressedP", func(c *bind.Ctx, p int64, a string) bool {
		s, pl, ok := lookup(c, p, a)
		return ok && s.PressedP(pl, a)
	}, bind.Doc("PressedP is Pressed for one player."), bind.Params("player", "action"), bind.Example("if input.PressedP(1, \"Jump\") {"))
	bind.Func2(m, "ReleasedP", func(c *bind.Ctx, p int64, a string) bool {
		s, pl, ok := lookup(c, p, a)
		return ok && s.ReleasedP(pl, a)
	}, bind.Doc("ReleasedP is Released for one player."), bind.Params("player", "action"), bind.Example("if input.ReleasedP(1, \"Jump\") {"))
	bind.Func2(m, "HeldP", func(c *bind.Ctx, p int64, a string) bool {
		s, pl, ok := lookup(c, p, a)
		return ok && s.HeldP(pl, a)
	}, bind.Doc("HeldP is Held for one player."), bind.Params("player", "action"), bind.Example("if input.HeldP(1, \"Run\") {"))
	bind.Func2(m, "AxisP", func(c *bind.Ctx, p int64, a string) float32 {
		s, pl, ok := lookup(c, p, a)
		if !ok {
			return 0
		}
		return s.AxisP(pl, a)
	}, bind.Doc("AxisP is Axis for one player."), bind.Params("player", "action"), bind.Example("x := input.AxisP(1, \"Move\")"))
	bind.Func2(m, "Axis2P", func(c *bind.Ctx, p int64, a string) mathx.Vec2 {
		s, pl, ok := lookup(c, p, a)
		if !ok {
			return mathx.Vec2{}
		}
		return s.Axis2P(pl, a)
	}, bind.Doc("Axis2P is Axis2 for one player."), bind.Params("player", "action"), bind.Example("v := input.Axis2P(1, \"Move\")"))
	bind.Func0(m, "ActiveDevice", func(c *bind.Ctx) int64 {
		if s := Of(c.World); s != nil {
			return int64(s.ActiveDevice(0))
		}
		return DeviceNone
	}, bind.Doc("ActiveDevice returns the device player 0 used last: -1 none, 0 keyboard and mouse, 1 to 4 a gamepad."),
		bind.Example("onPad := input.ActiveDevice() > 0"))
	bind.Func0(m, "DeviceChanged", func(c *bind.Ctx) bool {
		s := Of(c.World)
		return s != nil && s.DeviceChanged()
	}, bind.Doc("DeviceChanged is true for the frame a gamepad was connected or removed."), bind.Example("if input.DeviceChanged() {"))
	bind.Func1(m, "Glyph", func(c *bind.Ctx, a string) string {
		s, p, ok := lookup(c, 0, a)
		if !ok {
			return ""
		}
		return s.GlyphP(p, a)
	}, bind.Doc("Glyph returns the binding of the action for the active device as text, like [Space] or [A]."), bind.Params("action"),
		bind.Example("hint.Text = \"Jump \" + input.Glyph(\"Jump\")"))
	bind.Func2(m, "BeginRebind", func(c *bind.Ctx, a string, slot int64) bool {
		s, _, ok := lookup(c, 0, a)
		return ok && s.BeginRebind(a, int(slot))
	}, bind.Doc("BeginRebind waits for the next key, button or stick and binds it to the slot of the action. Escape cancels."),
		bind.Params("action", "slot"), bind.Example("input.BeginRebind(\"Jump\", 0)"))
	bind.Func0(m, "Rebinding", func(c *bind.Ctx) bool {
		s := Of(c.World)
		return s != nil && s.Rebinding()
	}, bind.Doc("Rebinding reports whether a rebind waits for input."), bind.Example("if input.Rebinding() {"))
	bind.Func0(m, "RebindConflict", func(c *bind.Ctx) string {
		if s := Of(c.World); s != nil {
			return s.RebindConflict()
		}
		return ""
	}, bind.Doc("RebindConflict names the action that already used the new control, or explains why it was refused."),
		bind.Example("if input.RebindConflict() != \"\" {"))
	bind.Proc1(m, "ResetBinding", func(c *bind.Ctx, a string) {
		if s, _, ok := lookup(c, 0, a); ok {
			s.ResetBinding(a)
		}
	}, bind.Doc("ResetBinding restores the project bindings of the action."), bind.Params("action"), bind.Example("input.ResetBinding(\"Jump\")"))
	bind.Proc0(m, "ResetBindings", func(c *bind.Ctx) {
		if s := Of(c.World); s != nil {
			s.ResetBindings()
		}
	}, bind.Doc("ResetBindings restores every project binding."), bind.Example("input.ResetBindings()"))
	bind.Proc0(m, "SaveBindings", func(c *bind.Ctx) {
		if s := Of(c.World); s != nil {
			if err := s.SaveBindings(); err != nil {
				c.Fail(err.Error())
			}
		}
	}, bind.Doc("SaveBindings writes the player's settings profile now; changes are also saved on their own."), bind.Example("input.SaveBindings()"))
}
