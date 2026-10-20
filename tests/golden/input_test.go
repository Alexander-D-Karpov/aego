//go:build aego_golden

package golden

import (
	"testing"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/services"
	"aego/engine"
	"aego/modules/input"
	inputeditor "aego/modules/input/editor"
	rawinput "aego/platform/input"
)

func inputGoldenConfig(t *testing.T) input.Config {
	t.Helper()
	c := input.Config{Players: 1, Deadzone: input.DefaultDeadzone, Stack: []string{"Gameplay"}}
	add := func(name string, k input.Kind, binds ...string) {
		a := input.Action{Name: name, Kind: k}
		for _, b := range binds {
			x, err := input.ParseBinding(b)
			if err != nil {
				t.Fatal(err)
			}
			a.Bindings = append(a.Bindings, x)
		}
		c.Actions = append(c.Actions, a)
	}
	add("Move", input.Axis1D, "Key.A/Key.D", "Pad.LeftX")
	add("Jump", input.Button, "Key.Space", "Pad.A")
	add("Dash", input.Button, "Key.Space")
	add("Pause", input.Button, "Key.Escape", "Pad.Start")
	add("Back", input.Button, "Key.Escape", "Pad.B")
	c.Actions[1].Buffer = 0.1
	c.Contexts = []input.Context{
		{Name: "Gameplay", Actions: []string{"Move", "Jump", "Dash", "Pause"}},
		{Name: "Menu", Priority: 10, Consume: true, Actions: []string{"Back"}},
	}
	return c
}

func TestEditorInputSettings(t *testing.T) {
	env := newEditorEnvSize(t, 640, 760)
	ctx := &context.Editor{UI: env.ui}
	pg := inputeditor.NewPage(ctx)
	pg.Select(1)
	c := inputGoldenConfig(t)
	img, err := env.draw(func(mathx.Rect) { pg.Draw(ctx, &c) })
	checkGolden(t, "editor_input_settings", img, err)
}

func TestEditorInputFocus(t *testing.T) {
	env := newEditorEnvSize(t, 640, 260)
	c := inputGoldenConfig(t)
	c.Stack = []string{"Gameplay", "Menu"}
	s := input.NewService(c, input.UserKeys{})
	in := &rawinput.State{}
	in.ApplyKeyDown(rawinput.KeyEscape, 0, 0, false)
	s.Frame(in, input.Clock{Real: 1.0 / 60, Dt: 1.0 / 60, Game: 1.0 / 60, Next: 1})
	w := engine.NewWorld("golden")
	w.SetService(input.ServiceID, s)
	p := inputeditor.NewFocus(func() *engine.World { return w })
	img, err := env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui}, full) })
	checkGolden(t, "editor_input_focus", img, err)
}

func TestEditorInputDevices(t *testing.T) {
	env := newEditorEnvSize(t, 640, 300)
	in := &rawinput.State{}
	p := inputeditor.NewDevices(func() *engine.World { return nil }, func() *rawinput.State { return in })
	p.SetVirtual(in, true)
	img, err := env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui}, full) })
	checkGolden(t, "editor_input_devices", img, err)
}
