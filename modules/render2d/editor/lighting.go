package editor

import (
	"slices"

	"aego/asset"
	"aego/contracts/shapes2d"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/shape"
	"aego/editor/ext"
	"aego/engine"
	"aego/engine/i18n"
	"aego/engine/problems"
	"aego/modules/render2d"
	"aego/render/lighting"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
)

const (
	CmdLighting                 = "render2d.lighting.toggle"
	CmdLightingNormals          = "render2d.lighting.normals"
	CmdLightingShadows          = "render2d.lighting.shadows"
	CmdLightingEnvironmentReset = "render2d.lighting.environment.reset"
)

type lightingEditor struct {
	inspectPasses      bool
	selectedOutput     view.ID
	ed                 *context.Editor
	preview            shadowResourcePreview
	diagnosticWorld    *engine.World
	worldRevision      uint64
	diagnosticRevision uint64
	diagnostics        []problems.Diagnostic
	dropped            []ids.NodeID
	shapes             shapes2d.Buffer
}

func lightingHooks(r *ext.Registry) error {
	p := &lightingEditor{ed: r.Editor()}
	light, ok := r.Schema().ByName("engine.Light2D")
	if !ok {
		return errs.New(errs.SchemaUnknownType, "Light2D is not registered")
	}
	occluder, ok := r.Schema().ByName("engine.LightOccluder2D")
	if !ok {
		return errs.New(errs.SchemaUnknownType, "LightOccluder2D is not registered")
	}
	if err := r.Gizmo(light.FullName, &lightGizmo{td: light}); err != nil {
		return err
	}
	if err := r.Gizmo(occluder.FullName, &occluderGizmo{td: occluder, p: p, activeIndex: -1, hoverIndex: -1}); err != nil {
		return err
	}
	for _, name := range []string{light.FullName, occluder.FullName, "engine.LightingEnvironment2D"} {
		if err := r.Inspector(name, &lightingInspector{p: p, kind: name}); err != nil {
			return err
		}
	}
	if err := r.Bounds(light.FullName, lightShape); err != nil {
		return err
	}
	if err := r.Bounds(occluder.FullName, occluderShape); err != nil {
		return err
	}
	r.Overlay(p.overlay)
	if p.ed == nil {
		return nil
	}
	r.Tick(p.tick)
	r.Profiler(i18n.T("editor.lighting.profiler", "Lighting"), false, p.profile)
	if p.ed.Problems != nil {
		p.ed.Problems.Add(problems.Source{Revision: func() uint64 { return p.diagnosticRevision }, Append: func(dst []problems.Diagnostic) []problems.Diagnostic { return append(dst, p.diagnostics...) }})
	}
	for index, cmd := range []struct {
		id, icon   string
		label, tip i18n.Text
		mode       lighting.DebugMode
	}{
		{CmdLighting, "render2d.point_light", i18n.T("editor.lighting.toggle", "Scene lighting"), i18n.T("editor.lighting.toggle_tip", "Toggle lighting in this Scene View without changing materials or Play."), lighting.DebugLighting},
		{CmdLightingNormals, "render2d.normals", i18n.T("editor.lighting.normals", "Surface normals"), i18n.T("editor.lighting.normals_tip", "Show transformed normals of Lit surfaces in this Scene View."), lighting.DebugNormals},
		{CmdLightingShadows, "render2d.shadow_view", i18n.T("editor.lighting.shadows", "Shadow visibility"), i18n.T("editor.lighting.shadows_tip", "Show shadow visibility on Lit surfaces in this Scene View."), lighting.DebugShadows},
	} {
		c := cmd
		if err := r.Command(command.Desc{ID: c.id, Name: c.label.String(), Label: c.label.String, TipFunc: c.tip.String, Category: "Scene", Scope: command.ScopeScene, Toolbar: true, Icon: c.icon, Order: 200 + index,
			Enabled: func() bool { return p.ed.SceneLighting != nil && p.ed.Docs != nil && p.ed.Doc() != nil },
			Checked: func() bool {
				if p.ed.SceneLighting == nil {
					return false
				}
				enabled, mode := p.ed.SceneLighting.LightingView()
				if c.id == CmdLighting {
					return enabled
				}
				return enabled && mode == c.mode
			},
			Run: func() error {
				enabled, mode := p.ed.SceneLighting.LightingView()
				if c.id == CmdLighting {
					enabled = !enabled
					if !enabled {
						// Debug views also work without lights at renderer level;
						// the toolbar's off state must actually leave them.
						mode = lighting.DebugLighting
					}
				} else {
					if enabled && mode == c.mode {
						mode = lighting.DebugLighting
					} else {
						mode = c.mode
					}
					enabled = true
				}
				p.ed.SceneLighting.SetLightingView(enabled, mode)
				return nil
			},
		}); err != nil {
			return err
		}
	}
	return r.Command(command.Desc{ID: CmdLightingEnvironmentReset,
		Name: "Reset Scene View lighting environment", Category: "Scene", Scope: command.ScopeScene,
		Enabled: func() bool {
			return p.ed.SceneLighting != nil && !p.ed.SceneLighting.LightingEnvironmentPreview().IsZero()
		},
		Run: func() error { p.ed.SceneLighting.SetLightingEnvironmentPreview(ids.NodeID{}); return nil },
	})
}

func (p *lightingEditor) tick() {
	if !p.preview.seen {
		p.preview.close()
	}
	p.preview.seen = false
	var world *engine.World
	if p.ed.SceneLighting != nil {
		world = p.ed.SceneLighting.LightingWorld()
	}
	diagnostics, revision := render2d.LightingDiagnostics(world)
	var dropped []ids.NodeID
	if p.ed.SceneLighting != nil && p.ed.Renderer != nil {
		dropped = p.ed.Renderer.LightingDropped(p.ed.SceneLighting.LightingOutput())
	}
	if world != p.diagnosticWorld || revision != p.worldRevision || !slices.Equal(dropped, p.dropped) {
		p.diagnosticWorld, p.worldRevision = world, revision
		p.diagnostics = append(p.diagnostics[:0], diagnostics...)
		p.dropped = append(p.dropped[:0], dropped...)
		if len(dropped) > 0 {
			p.diagnostics = append(p.diagnostics, problems.Diagnostic{Owner: render2d.ID, Code: "LGT002", Severity: problems.Warning,
				Message: i18n.Tr("editor.lighting.budget_diagnostic", "The Scene View light budget excluded {count} light candidates from at least one receiver set. Yellow source markers identify them; adjust priority, range or layers.", i18n.Int("count", int64(len(dropped))))})
		}
		p.diagnosticRevision++
	}
}

func lightShape(s *scene.Scene, _ asset.Resolver, h scene.NodeHandle) (shape.Shape, bool) {
	l, ok := scene.Get[render2d.Light2D](s, h)
	if !ok {
		return shape.Shape{}, false
	}
	// Pick the source icon rather than letting a large radius intercept every
	// sprite inside it. The full influence bounds are drawn by the overlay.
	return shape.Shape{Kind: shape.Rect, Local: mathx.Rect{X: -.15, Y: -.15, W: .3, H: .3}}, l.Type <= lighting.Sprite
}
func occluderShape(s *scene.Scene, _ asset.Resolver, h scene.NodeHandle) (shape.Shape, bool) {
	o, ok := scene.Get[render2d.LightOccluder2D](s, h)
	if !ok || o.Source != render2d.OccluderPolygon || len(o.Polygon) == 0 {
		return shape.Shape{}, false
	}
	lo, hi := o.Polygon[0], o.Polygon[0]
	for _, v := range o.Polygon[1:] {
		lo.X = min(lo.X, v.X)
		lo.Y = min(lo.Y, v.Y)
		hi.X = max(hi.X, v.X)
		hi.Y = max(hi.Y, v.Y)
	}
	return shape.Shape{Kind: shape.Rect, Local: mathx.Rect{X: lo.X, Y: lo.Y, W: max(hi.X-lo.X, .02), H: max(hi.Y-lo.Y, .02)}}, true
}

func lightPose(s *scene.Scene, h scene.NodeHandle) (mathx.Vec2, mathx.Vec2) {
	m := s.World(h)
	dir := mathx.Vec2{X: m[0], Y: m[1]}
	n := dir.Len()
	if n > 0 {
		dir = dir.Scale(1 / n)
	} else {
		dir = mathx.Vec2{X: 1}
	}
	return mathx.Vec2{X: m[6], Y: m[7]}, dir
}

func (p *lightingEditor) overlay(c *shape.OverlayCtx) {
	if c.Mode3D || c.Scene == nil {
		return
	}
	s := c.Scene
	world := c.PresentationWorld
	if world == nil {
		world = c.World
	}
	var dropped []ids.NodeID
	if p.ed != nil && p.ed.Renderer != nil {
		id := c.View.ID
		if runner, ok := engine.ServiceOf[*frame.Runner](world, frame.ServiceID); ok && id.Scope == 0 {
			id = runner.OutputID()
		}
		dropped = p.ed.Renderer.LightingDropped(id)
	}
	scene.Query1[render2d.Light2D](s).Each(func(h scene.NodeHandle, l *render2d.Light2D) {
		if !s.EffectiveEnabled(h) {
			return
		}
		m := s.RenderWorld(h)
		at := mathx.Vec2{X: m[6], Y: m[7]}
		dir := mathx.Vec2{X: m[0], Y: m[1]}
		if n := dir.Len(); finite(n) && n > 0 {
			dir = dir.Scale(1 / n)
		} else {
			dir = mathx.Vec2{X: 1}
		}
		col := c.Color
		if slices.Contains(dropped, s.ID(h)) && p.ed.UI != nil {
			col = p.ed.UI.Theme.Warn
			c.Debug.Circle(at, c.Px*10, 16, col)
		}
		col.A *= .65
		c.Debug.Cross(at, c.Px*6, col)
		if l.Type == lighting.Directional {
			c.Debug.Arrow(at, at.Add(dir.Scale(c.Px*42)), col)
			return
		}
		if l.Range <= 0 || !finite(l.Range) {
			return
		}
		c.Debug.Circle(at, l.Range, 48, col)
		if l.Type == lighting.Spot {
			for _, angle := range []float32{-l.OuterAngle / 2, l.OuterAngle / 2} {
				c.Debug.Line(at, at.Add(dir.Rotated(angle).Scale(l.Range)), col)
			}
		}
	})
	provider, _ := engine.ServiceOf[shapes2d.Provider](world, shapes2d.ServiceID)
	scene.Query1[render2d.LightOccluder2D](s).Each(func(h scene.NodeHandle, o *render2d.LightOccluder2D) {
		if !s.EffectiveEnabled(h) {
			return
		}
		if o.Source == render2d.OccluderPolygon {
			drawLightingContour(c, s.RenderWorld(h), o.Polygon, o.Closed)
			return
		}
		if provider == nil {
			return
		}
		owner := s.ID(h)
		if !o.ShapeNode.IsZero() {
			owner = o.ShapeNode.ID
		}
		p.shapes.Reset()
		if err := provider.Append(owner, o.Shape, &p.shapes); err != nil {
			return // The extraction diagnostic is published through Problems.
		}
		for _, contour := range p.shapes.Contours {
			handle := s.Handle(contour.Owner)
			if s.IsAlive(handle) {
				drawLightingContour(c, s.RenderWorld(handle), p.shapes.Vertices(contour), contour.Closed)
			}
		}
	})
}

func drawLightingContour(c *shape.OverlayCtx, m mathx.Mat3, points []mathx.Vec2, closed bool) {
	for i := 1; i < len(points); i++ {
		c.Debug.Line(m.Apply(points[i-1]), m.Apply(points[i]), c.Color)
	}
	if closed && len(points) > 2 {
		c.Debug.Line(m.Apply(points[len(points)-1]), m.Apply(points[0]), c.Color)
	}
}

func selectedLight(s *scene.Scene, id ids.NodeID) (scene.NodeHandle, *render2d.Light2D, bool) {
	h := s.Handle(id)
	if h.IsZero() || !s.IsAlive(h) {
		return h, nil, false
	}
	l, ok := scene.Get[render2d.Light2D](s, h)
	return h, l, ok
}
