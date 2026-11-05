package widgets

import (
	"math"
	"testing"

	"aego/core/curve"
	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
)

func TestPresetBakePreservesEveryEasing(t *testing.T) {
	for easing := curve.Linear; easing < curve.EasingCount; easing++ {
		t.Run(easing.String(), func(t *testing.T) {
			original := curve.FromEasing(easing)
			original.Pre, original.Post = curve.WrapLoop, curve.WrapPingPong
			baked := curveBakePreset(&original, -1)
			if baked.Ease != curve.None || baked.Pre != original.Pre || baked.Post != original.Post {
				t.Fatal("baking did not preserve the wrap modes")
			}
			var maxErr float64
			for i := range 10001 {
				x := float32(i) / 10000
				maxErr = math.Max(maxErr, math.Abs(float64(original.Eval(x)-baked.Eval(x))))
			}
			t.Logf("maximum shape error: %.6f", maxErr)
			if maxErr > 0.003 {
				t.Fatalf("baked curve differs from the preset by %g", maxErr)
			}
		})
	}
}

func TestPresetGraphMouseDragIsOneGesture(t *testing.T) {
	ui, in := core.New(nil), &input.State{}
	cv := curve.FromEasing(curve.OutQuad)
	cv.Pre, cv.Post = curve.WrapLoop, curve.WrapPingPong
	original := cv.Clone()
	st := NewCurveState()
	r := mathx.Rect{X: 10, Y: 10, W: 340, H: 180}
	frame := func(at mathx.Vec2, event string) Edit {
		in.BeginFrame()
		in.ApplyMouseMove(at, mathx.Vec2{})
		switch event {
		case "down":
			in.ApplyMouseDown(at, input.MouseLeft)
		case "up":
			in.ApplyMouseUp(at, input.MouseLeft)
		}
		ui.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
		e := CurveGraph(ui, core.IDOf("preset"), r, &cv, st)
		ui.End()
		return e
	}
	frame(mathx.Vec2{}, "")
	g := curveFrame{r: mathx.Rect{X: r.X + CurvePad, Y: r.Y + CurvePad, W: r.W - 2*CurvePad, H: r.H - 2*CurvePad}, v: st.view}
	at := g.at(.5, cv.Eval(.5))
	if e := frame(at, "down"); !e.Begin || e.Changed || !cv.Equal(original) {
		t.Fatalf("press changed the preset: %+v", e)
	}
	if e := frame(at, "up"); !e.End || e.Changed || !cv.Equal(original) {
		t.Fatalf("click changed the preset: %+v", e)
	}
	frame(at, "")
	if e := frame(at, "down"); !e.Begin || e.Changed {
		t.Fatalf("drag did not begin cleanly: %+v", e)
	}
	end := at.Add(mathx.Vec2{X: 8, Y: -25})
	if e := frame(end, ""); !e.Changed || e.Begin || cv.Ease != curve.None || cv.Pre != original.Pre || cv.Post != original.Post {
		t.Fatalf("drag failed to convert and edit the preset: %+v, curve=%+v", e, cv)
	}
	if e := frame(end, "up"); !e.End || e.Changed {
		t.Fatalf("drag did not end cleanly: %+v", e)
	}
}

func TestNormalizedCurveKeyCannotLeaveLifetime(t *testing.T) {
	cv := curve.Line(0, 1, 1, 0)
	st := NewCurveState()
	st.Key = 0
	st.SetNormalizedTime(true)
	curveDrag(&cv, st, -2, 0.8, false)
	if cv.Keys[0].T != 0 {
		t.Fatalf("first key escaped: %v", cv.Keys[0].T)
	}
	st.Key = 1
	curveDrag(&cv, st, 3, 0.2, false)
	if cv.Keys[1].T != 1 {
		t.Fatalf("last key escaped: %v", cv.Keys[1].T)
	}
	st.SetNormalizedTime(false)
	curveDrag(&cv, st, 3, 0.2, false)
	if cv.Keys[1].T != 3 {
		t.Fatal("unbounded animation curves should keep their independent time range")
	}
}
