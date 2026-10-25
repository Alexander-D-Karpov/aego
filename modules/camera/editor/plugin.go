package editor

import (
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/ext"
	"aego/editor/ext/inspect"
	"aego/engine/i18n"
	"aego/modules/camera"
	"aego/scene"
	"aego/schema"
	"aego/ui/widgets"
)

const (
	CmdFrames       = camera.ID + ".frames"
	partBase        = 32
	cameraComponent = "engine.Camera2D"
	defaultZoom     = 64
)

var (
	colFrame    = mathx.Color{R: 0.55, G: 0.75, B: 1, A: 0.9}
	colDeadzone = mathx.Color{R: 1, G: 0.85, B: 0.3, A: 0.9}
	colBounds   = mathx.Color{R: 0.4, G: 1, B: 0.6, A: 0.8}
	colZone     = mathx.Color{R: 0.95, G: 0.5, B: 0.9, A: 0.8}
	colDim      = mathx.Color{R: 0.7, G: 0.7, B: 0.7, A: 0.35}
)

type plugin struct {
	ed *context.Editor
}

func lookup(sr *schema.Registry, name string) (*schema.TypeDescriptor, error) {
	td, ok := sr.ByName(name)
	if !ok {
		return nil, errs.New(errs.SchemaUnknownType, "camera component is not registered", log.F("type", name))
	}
	return td, nil
}

func Hook(e *ext.Registry) error {
	p := &plugin{ed: e.Editor()}
	sr := e.Schema()
	vc, err := lookup(sr, camera.NameVirtual)
	if err != nil {
		return err
	}
	zone, err := lookup(sr, camera.NameZone)
	if err != nil {
		return err
	}
	if err := e.Gizmo(camera.NameVirtual, &rectGizmo{td: vc, field: "bounds", col: colBounds, read: readBounds, extra: drawFrame}); err != nil {
		return err
	}
	if err := e.Gizmo(camera.NameZone, &rectGizmo{td: zone, field: "area", local: true, col: colZone, read: readArea}); err != nil {
		return err
	}
	if err := e.Gizmo(camera.NameFollow, followGizmo{}); err != nil {
		return err
	}
	if err := e.Inspector(camera.NameVirtual, inspect.CustomFunc(p.vcInspector)); err != nil {
		return err
	}
	if err := e.Inspector(camera.NameFollow, inspect.CustomFunc(p.followInspector)); err != nil {
		return err
	}
	e.Overlay(p.sceneOverlay)
	e.GameOverlay(p.gameOverlay)
	e.Profiler(i18n.T("i18n.camera.profiler", "Camera"), true, p.profile)
	if p.ed == nil {
		return nil
	}
	o := &p.ed.Overlay
	o.Set(CmdFrames, true)
	return e.Command(command.Desc{
		ID: CmdFrames, Name: "Camera Frames", Category: camera.Category, Order: 100,
		Checked: func() bool { return o.On(CmdFrames) },
		Run: func() error {
			o.Toggle(CmdFrames)
			return nil
		},
	})
}

func (p *plugin) svc() *camera.Service {
	if p.ed == nil || p.ed.Docs == nil {
		return nil
	}
	return camera.Of(p.ed.Docs.ActiveWorld())
}

func (p *plugin) vcInspector(ctx *context.Editor, nodes []ids.NodeID, def func()) {
	def()
	d := ctx.Doc()
	s := p.svc()
	if d == nil || s == nil || len(nodes) != 1 {
		return
	}
	st, ok := s.Status(d.Scene().Handle(nodes[0]))
	if !ok {
		return
	}
	ui := ctx.UI
	widgets.LabelDim(ui, st.Text())
	if widgets.Button(ui, i18n.Tr("i18n.camera.look_through", "Look Through")) {
		zoom := st.Pose.Zoom
		if zoom <= 0 {
			zoom = defaultZoom
		}
		d.View.Camera.Position = st.Pose.Position
		d.View.Camera.Zoom = zoom
		d.View.Camera.Rotation = st.Pose.Rotation
	}
}

func (p *plugin) followInspector(ctx *context.Editor, nodes []ids.NodeID, def func()) {
	def()
	d := ctx.Doc()
	s := p.svc()
	if d == nil || s == nil {
		return
	}
	if widgets.Button(ctx.UI, i18n.Tr("i18n.camera.snap_to_target", "Snap to Target")) {
		sc := d.Scene()
		for _, n := range nodes {
			s.Snap(sc.Handle(n))
		}
	}
}

func camProps(s *scene.Scene, h scene.NodeHandle) (float32, mathx.Vec2, bool) {
	td, ok := s.Registry().ByName(cameraComponent)
	if !ok {
		return 0, mathx.Vec2{}, false
	}
	zf, ok1 := td.Field("zoom")
	vf, ok2 := td.Field("virtual")
	if !ok1 || !ok2 {
		return 0, mathx.Vec2{}, false
	}
	z, err := s.GetProperty(h, td.ID, zf.ID)
	if err != nil {
		return 0, mathx.Vec2{}, false
	}
	v, err := s.GetProperty(h, td.ID, vf.ID)
	if err != nil {
		return 0, mathx.Vec2{}, false
	}
	return float32(z.AsFloat()), v.V, true
}

func editorSize(s *scene.Scene, h scene.NodeHandle) mathx.Vec2 {
	zoom, virt, ok := camProps(s, h)
	if !ok {
		for _, n := range s.PreOrder() {
			if zoom, virt, ok = camProps(s, n); ok {
				break
			}
		}
	}
	if vc, has := scene.Get[camera.VirtualCamera2D](s, h); has && vc.Zoom > 0 {
		zoom = vc.Zoom
	}
	if zoom <= 0 {
		zoom = defaultZoom
	}
	if virt.X <= 0 || virt.Y <= 0 {
		virt = mathx.Vec2{X: 1280, Y: 720}
	}
	return mathx.Vec2{X: virt.X / zoom, Y: virt.Y / zoom}
}
