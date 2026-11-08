package editor

import (
	"image"
	"math"

	"aego/art"
	"aego/core/mathx"
	"aego/ui/core"
)

type artSelectionEdge struct{ a, b image.Point }

// Coalesce adjacent boundary pixels into lines. A pathological checkerboard
// cannot allocate unbounded UI geometry: its bounding box is shown instead,
// with an explicit simplified-outline indication in the status bar.
func artSelectionOutline(sel *art.Selection) ([]artSelectionEdge, bool) {
	if sel == nil {
		return nil, false
	}
	bounds := sel.Bounds()
	if bounds.Empty() {
		return nil, false
	}
	const maxEdges = 8192
	var edges []artSelectionEdge
	runs := func(start, end int, on func(int) bool, edge func(int, int) artSelectionEdge) bool {
		for i := start; i < end; {
			if !on(i) {
				i++
				continue
			}
			from := i
			for i < end && on(i) {
				i++
			}
			if len(edges) >= maxEdges {
				return false
			}
			edges = append(edges, edge(from, i))
		}
		return true
	}
	simplified := false
	for y := bounds.Min.Y; y < bounds.Max.Y && !simplified; y++ {
		for _, dy := range []int{-1, 1} {
			yLine := y
			if dy > 0 {
				yLine++
			}
			if !runs(bounds.Min.X, bounds.Max.X, func(x int) bool { return sel.Contains(x, y) && !sel.Contains(x, y+dy) }, func(a, b int) artSelectionEdge {
				return artSelectionEdge{image.Pt(a, yLine), image.Pt(b, yLine)}
			}) {
				simplified = true
				break
			}
		}
	}
	for x := bounds.Min.X; x < bounds.Max.X && !simplified; x++ {
		for _, dx := range []int{-1, 1} {
			xLine := x
			if dx > 0 {
				xLine++
			}
			if !runs(bounds.Min.Y, bounds.Max.Y, func(y int) bool { return sel.Contains(x, y) && !sel.Contains(x+dx, y) }, func(a, b int) artSelectionEdge {
				return artSelectionEdge{image.Pt(xLine, a), image.Pt(xLine, b)}
			}) {
				simplified = true
				break
			}
		}
	}
	if simplified {
		return []artSelectionEdge{
			{bounds.Min, image.Pt(bounds.Max.X, bounds.Min.Y)},
			{image.Pt(bounds.Max.X, bounds.Min.Y), bounds.Max},
			{bounds.Max, image.Pt(bounds.Min.X, bounds.Max.Y)},
			{image.Pt(bounds.Min.X, bounds.Max.Y), bounds.Min},
		}, true
	}
	return edges, false
}

func (p *artPanel) drawSelection(ui *core.Context, area mathx.Rect, current image.Point) {
	v, sel := p.view, p.model.Session.Selection
	if v.outlineSelection != sel {
		v.outlineSelection = sel
		v.outline, v.outlineSimplified = artSelectionOutline(sel)
	}
	ui.PushClip(area)
	defer ui.PopClip()
	screen := func(x, y float32) mathx.Vec2 {
		return v.viewport.screenPoint(area, mathx.Vec2{X: x, Y: y})
	}
	line := func(a, b image.Point) {
		ui.Line(screen(float32(a.X), float32(a.Y)), screen(float32(b.X), float32(b.Y)), 1/ui.Scale(), ui.Theme.Accent)
	}
	for _, edge := range v.outline {
		line(edge.a, edge.b)
	}
	if !v.selecting {
		return
	}
	if v.selectionTool == artLasso {
		for i := 1; i < len(v.selectionPoints); i++ {
			line(v.selectionPoints[i-1], v.selectionPoints[i])
		}
		if len(v.selectionPoints) > 1 {
			line(v.selectionPoints[len(v.selectionPoints)-1], v.selectionPoints[0])
		}
		return
	}
	x0, x1 := min(v.selectionStart.X, current.X), max(v.selectionStart.X, current.X)+1
	y0, y1 := min(v.selectionStart.Y, current.Y), max(v.selectionStart.Y, current.Y)+1
	if v.selectionTool == artEllipseSelect {
		cx, cy := float32(x0+x1)/2, float32(y0+y1)/2
		rx, ry := float32(x1-x0)/2, float32(y1-y0)/2
		previous := screen(cx+rx, cy)
		for i := 1; i <= 48; i++ {
			angle := 2 * math.Pi * float64(i) / 48
			next := screen(cx+rx*float32(math.Cos(angle)), cy+ry*float32(math.Sin(angle)))
			ui.Line(previous, next, 1/ui.Scale(), ui.Theme.Accent)
			previous = next
		}
		return
	}
	a := screen(float32(x0), float32(y0))
	b := screen(float32(x1), float32(y1))
	ui.List().RectOutline(mathx.Rect{X: a.X, Y: a.Y, W: b.X - a.X, H: b.Y - a.Y}, ui.Theme.Accent, 1/ui.Scale())
}
