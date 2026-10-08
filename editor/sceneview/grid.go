package sceneview

import (
	"math"

	"aego/core/mathx"
	"aego/editor/gizmo"
	"aego/render/debug"
	"aego/render/view"
	"aego/ui/core"
)

const maxGridLines = 400

func onMultiple(v, m float32) bool {
	r := math.Mod(math.Abs(float64(v))+float64(m)*0.01, float64(m))
	return r < float64(m)*0.02
}

func drawGrid(dbg *debug.Draw, v *view.View, sp gizmo.Space, base float32, th *core.Theme) {
	b := v.VisibleBounds()
	step := base
	for step*sp.Zoom < 14 {
		step *= 2
	}
	for step*sp.Zoom > 140 {
		step /= 2
	}
	fade := mathx.Clamp((step*sp.Zoom-14)/40, 0.35, 1)
	minor := mathx.Color{R: 1, G: 1, B: 1, A: 0.018 * fade}
	major := mathx.Color{R: 1, G: 1, B: 1, A: 0.045}
	axisX := th.AxisX
	axisX.A = 0.28
	axisY := th.AxisY
	axisY.A = 0.28
	majorStep := step * 5
	n := 0
	for x := float32(math.Floor(float64(b.Min.X/step))) * step; x <= b.Max.X && n < maxGridLines; x += step {
		col := minor
		switch {
		case mathx.Abs(x) < step*0.01:
			col = axisY
		case onMultiple(x, majorStep):
			col = major
		}
		dbg.Line(mathx.Vec2{X: x, Y: b.Min.Y}, mathx.Vec2{X: x, Y: b.Max.Y}, col)
		n++
	}
	for y := float32(math.Floor(float64(b.Min.Y/step))) * step; y <= b.Max.Y && n < maxGridLines*2; y += step {
		col := minor
		switch {
		case mathx.Abs(y) < step*0.01:
			col = axisX
		case onMultiple(y, majorStep):
			col = major
		}
		dbg.Line(mathx.Vec2{X: b.Min.X, Y: y}, mathx.Vec2{X: b.Max.X, Y: y}, col)
		n++
	}
}
