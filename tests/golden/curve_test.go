//go:build aego_golden

package golden

import (
	"testing"

	"aego/core/curve"
	"aego/core/mathx"
	"aego/ui/core"
	"aego/ui/widgets"
)

func TestEditorCurveEditor(t *testing.T) {
	env := newEditorEnv(t)
	ui := env.ui
	keyed := curve.Keyed(
		curve.Key{T: 0, V: 0},
		curve.Key{T: 0.4, V: 1.2, Tangent: curve.TangentFree, In: 3, Out: -0.5},
		curve.Key{T: 0.7, V: 0.6, Interp: curve.InterpStep},
		curve.Key{T: 1, V: 1, Interp: curve.InterpLinear},
	)
	keyed.Post = curve.WrapPingPong
	preset := curve.FromEasing(curve.OutBack)
	st := widgets.NewCurveState()
	st.Key = 1
	other := widgets.NewCurveState()
	open := core.IDOf("golden.curve.keyed")
	field := mathx.Rect{X: 40, Y: 40, W: 200, H: 24}
	img, err := env.draw(func(full mathx.Rect) {
		if !ui.PopupOpen(open) {
			ui.OpenPopup(open, mathx.Vec2{X: field.X, Y: field.Y + field.H})
		}
		widgets.CurveField(ui, open, field, &keyed, false, st)
		widgets.CurveField(ui, core.IDOf("golden.curve.preset"), mathx.Rect{X: 460, Y: 40, W: 200, H: 24}, &preset, false, other)
		widgets.CurveField(ui, core.IDOf("golden.curve.mixed"), mathx.Rect{X: 460, Y: 80, W: 200, H: 24}, &preset, true, other)
	})
	if st.Key != 1 || len(keyed.Keys) != 4 {
		t.Fatalf("drawing changed the curve: key %d, %d keys", st.Key, len(keyed.Keys))
	}
	checkGolden(t, "editor_curve_editor", img, err)
}

func TestEditorCurvePreset(t *testing.T) {
	env := newEditorEnv(t)
	ui := env.ui
	preset := curve.FromEasing(curve.OutQuad)
	st := widgets.NewCurveState()
	open := core.IDOf("golden.curve.preset.open")
	field := mathx.Rect{X: 40, Y: 40, W: 200, H: 24}
	img, err := env.draw(func(full mathx.Rect) {
		if !ui.PopupOpen(open) {
			ui.OpenPopup(open, mathx.Vec2{X: field.X, Y: field.Y + field.H})
		}
		widgets.CurveField(ui, open, field, &preset, false, st)
	})
	if preset.Ease != curve.OutQuad || len(preset.Keys) != 0 {
		t.Fatal("opening a preset must not convert it to keys")
	}
	checkGolden(t, "editor_curve_preset", img, err)
}
