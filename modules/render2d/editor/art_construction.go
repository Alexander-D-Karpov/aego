package editor

import (
	"image"

	"aego/art"
	"aego/ui/core"
)

// The host collects control points only. All rasterization, draft state,
// cancellation and the one-command commit remain in the shared Go Session.
func (p *artPanel) constructionClick(ui *core.Context, point image.Point, brush art.Brush) {
	v, s := p.view, p.model.Session
	if len(v.construction) == 0 {
		tool := "polygon"
		if v.tool == artCurve {
			tool = "curve"
		}
		if err := s.BeginGesture(tool, point, brush, v.filled); err != nil {
			p.report(err)
			return
		}
		v.constructionTool = v.tool
		v.construction = append(v.construction, point)
		ui.CaptureMouse(ui.ID("art.paint"))
		v.refresh = true
		return
	}
	if len(v.construction) >= 2048 {
		p.report(art.ErrLimit)
		return
	}
	if v.tool == artPolygon && len(v.construction) >= 3 && point == v.construction[0] {
		p.finishConstruction(ui)
		return
	}
	v.construction = append(v.construction, point)
	if v.tool == artCurve && len(v.construction) == 4 {
		p.finishConstruction(ui)
	} else {
		p.previewConstruction(point)
	}
}

func (p *artPanel) previewConstruction(point image.Point) {
	v, s := p.view, p.model.Session
	if len(v.construction) == 0 {
		return
	}
	points := append([]image.Point{}, v.construction...)
	if v.tool == artCurve {
		switch len(points) {
		case 1:
			points = []image.Point{points[0], points[0], point, point}
		case 2:
			points = []image.Point{points[0], point, point, points[1]}
		case 3:
			points = []image.Point{points[0], points[2], point, points[1]}
		default:
			points = []image.Point{points[0], points[2], points[3], points[1]}
		}
	} else {
		if len(points) < 2 {
			return
		}
		points = append(points, point)
	}
	p.report(s.PreviewGesturePoints(points))
	v.refresh = true
}

func (p *artPanel) finishConstruction(ui *core.Context) {
	v, s := p.view, p.model.Session
	points := append([]image.Point{}, v.construction...)
	if v.tool == artCurve {
		if len(points) != 4 {
			return
		}
		points = []image.Point{points[0], points[2], points[3], points[1]}
	} else if len(points) < 3 {
		return
	}
	if err := s.PreviewGesturePoints(points); err != nil {
		p.report(err)
		s.CancelGesture()
	} else {
		p.report(s.EndGesture())
	}
	v.construction = nil
	v.refresh = true
	ui.ReleaseMouse()
}
