package editor

import (
	"fmt"
	"math"
	"slices"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/ext"
	"aego/engine/i18n"
	"aego/modules/particles"
	"aego/platform/input"
	"aego/scene"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

type fieldGroup struct {
	title  i18n.Text
	fields []string
}

var groups = []fieldGroup{
	{i18n.T("editor.particles.playback", "Playback"), []string{"playOnStart", "duration", "loop", "prewarm", "timeDomain", "seed", "maxParticles"}},
	{i18n.T("editor.particles.emission", "Emission"), []string{"rate", "bursts"}},
	{i18n.T("editor.particles.shape", "Shape"), []string{"shape", "offset", "radius", "rectSize", "edgeA", "edgeB", "mask", "maskThreshold"}},
	{i18n.T("editor.particles.initial", "Initial"), []string{"lifetime", "speed", "size", "rotation", "direction", "spread", "color", "space"}},
	{i18n.T("editor.particles.life", "Over Life"), []string{"sizeOverLife", "velocityOverLife", "alphaOverLife", "colorOverLife", "rotationSpeed"}},
	{i18n.T("editor.particles.forces", "Forces"), []string{"gravity", "drag", "noiseStrength", "noiseFrequency", "killYEnabled", "killY", "planeEnabled", "planeNormal", "planeDistance"}},
	{i18n.T("editor.particles.render", "Render"), []string{"sprite", "frames", "frameFPS", "frameLoop", "material", "layer", "order", "sortMode"}},
	{i18n.T("editor.particles.access", "Accessibility"), []string{"reducedMotion", "reducedRate"}},
}

type previewControls interface {
	controls(*context.Editor, ids.NodeID)
}

type particleInspector struct {
	parent  previewControls
	section int
	closed  [8]bool
	grad    GradientEditor
	doc     document.ID
	node    ids.NodeID
	td      *schema.TypeDescriptor
}

// Emission and shape are the starting controls for an authored effect.
func defaultParticleFolds() [8]bool {
	return [8]bool{true, false, false, true, true, true, true, true}
}

func registerInspector(r *ext.Registry, p previewControls) error {
	td, ok := r.Schema().ByName(particles.NameSystem)
	if !ok {
		return fmt.Errorf("%s is not registered", particles.NameSystem)
	}
	if err := r.Inspector(particles.NameSystem, &particleInspector{parent: p, td: td, section: -1, closed: defaultParticleFolds()}); err != nil {
		return err
	}
	return r.Gizmo(particles.NameSystem, &shapeGizmo{td: td})
}
func (s *particleInspector) FieldVisible(ctx *context.Editor, nodes []ids.NodeID, f *schema.FieldDescriptor) bool {
	if s.section < 0 {
		return true
	}
	if !slices.Contains(groups[s.section].fields, f.Name) {
		return false
	}
	if ctx == nil || ctx.Doc() == nil || len(nodes) != 1 {
		return true
	}
	d := ctx.Doc()
	c, ok := scene.Get[particles.ParticleSystem2D](d.Scene(), d.Scene().Handle(nodes[0]))
	if !ok {
		return false
	}
	switch f.Name {
	case "colorOverLife":
		return false
	case "radius":
		return c.Shape == particles.Circle
	case "rectSize":
		return c.Shape == particles.Rect
	case "edgeA", "edgeB":
		return c.Shape == particles.Edge
	case "mask", "maskThreshold":
		return c.Shape == particles.SpriteMask
	case "killY":
		return c.KillYEnabled
	case "planeNormal", "planeDistance":
		return c.PlaneEnabled
	case "reducedRate":
		return c.ReducedMotion == particles.Reduce
	}
	return true
}
func (s *particleInspector) Draw(ctx *context.Editor, nodes []ids.NodeID, fields func()) {
	if ctx == nil || ctx.UI == nil || ctx.Doc() == nil || len(nodes) == 0 {
		fields()
		return
	}
	ui, d := ctx.UI, ctx.Doc()
	var c *particles.ParticleSystem2D
	if len(nodes) == 1 {
		c, _ = scene.Get[particles.ParticleSystem2D](d.Scene(), d.Scene().Handle(nodes[0]))
		if d.ID != s.doc || s.node != nodes[0] {
			s.grad = GradientEditor{}
			s.doc, s.node = d.ID, nodes[0]
		}
		if c != nil {
			s.parent.controls(ctx, nodes[0])
		}
	}
	defer func() { s.section = -1 }()
	for i, g := range groups {
		s.section = i
		open := !s.closed[i]
		ui.PushIDInt(i)
		if widgets.TreeNode(ui, ui.ID("section"), g.title.String(), &open, false) {
			open = !open
		}
		s.closed[i] = !open
		if open {
			fields()
			if i == 4 && c != nil {
				s.grad.Draw(ui, c.ColorOverLife, func(keys []particles.ColorKey) {
					if err := setGradient(d, s.td, nodes, keys); err != nil {
						ctx.Notify(context.LevelError, err.Error())
					}
				})
			}
		}
		ui.PopID()
	}
}
func setGradient(d *document.SceneDocument, td *schema.TypeDescriptor, nodes []ids.NodeID, keys []particles.ColorKey) error {
	f, ok := td.Field("colorOverLife")
	if !ok {
		return fmt.Errorf("colorOverLife is unavailable")
	}
	values := make([]schema.Value, len(keys))
	for i, k := range keys {
		values[i] = schema.Value{Kind: schema.KindStruct, Obj: &schema.Object{Type: f.Struct, Fields: []schema.Value{schema.Float32(k.Time), schema.Color(k.Color)}}}
	}
	return d.Props.Set(nil, nodes, td.ID, f.ID, schema.Value{Kind: schema.KindList, List: values})
}

type GradientEditor struct {
	sel     int
	editing bool
	draft   []particles.ColorKey
	base    []particles.ColorKey
}

func (w *GradientEditor) begin(keys []particles.ColorKey) {
	if !w.editing {
		w.base = slices.Clone(keys)
		w.draft = slices.Clone(keys)
		w.editing = true
	}
}
func (w *GradientEditor) commit(keys []particles.ColorKey, apply func([]particles.ColorKey)) {
	if w.editing && slices.Equal(keys, w.base) && !slices.Equal(w.draft, w.base) {
		apply(slices.Clone(w.draft))
	}
	w.editing = false
}
func addColor(keys []particles.ColorKey) ([]particles.ColorKey, int) {
	if len(keys) >= 256 {
		return keys, 0
	}
	t := float32(0)
	if len(keys) > 0 && keys[0].Time == 0 {
		if keys[len(keys)-1].Time < 1 {
			t = 1
		} else {
			var gap float32
			for i := 1; i < len(keys); i++ {
				a, b := keys[i-1].Time, keys[i].Time
				if b-a > gap {
					gap = b - a
					t = (a + b) / 2
				}
			}
		}
	}
	i, _ := slices.BinarySearchFunc(keys, t, func(k particles.ColorKey, t float32) int {
		if k.Time < t {
			return -1
		}
		if k.Time > t {
			return 1
		}
		return 0
	})
	return slices.Insert(slices.Clone(keys), i, particles.ColorKey{Time: t, Color: particles.GradientAt(keys, t)}), i
}
func (w *GradientEditor) move(t float32) {
	lo, hi := float32(0), float32(1)
	if w.sel > 0 {
		lo = math.Nextafter32(w.draft[w.sel-1].Time, 1)
	}
	if w.sel+1 < len(w.draft) {
		hi = math.Nextafter32(w.draft[w.sel+1].Time, 0)
	}
	if lo <= hi {
		w.draft[w.sel].Time = mathx.Clamp(t, lo, hi)
	}
}
func (w *GradientEditor) Draw(ui *core.Context, keys []particles.ColorKey, apply func([]particles.ColorKey)) {
	if w.editing && !slices.Equal(w.base, keys) {
		w.editing = false
	}
	v := keys
	if w.editing {
		v = w.draft
	}
	w.sel = min(max(0, len(v)-1), max(0, w.sel))
	widgets.Label(ui, i18n.Tr("editor.particles.gradient", "Color over life"))
	r := ui.Cursor().Next(28)
	const n = 48
	for j := range n {
		x := float32(j) / n
		tile := mathx.Rect{X: r.X + x*r.W, Y: r.Y, W: r.W/n + .5, H: r.H * .65}
		widgets.Swatch(ui, tile, particles.GradientAt(v, (float32(j)+.5)/n), false)
	}
	id := ui.ID("gradient-strip")
	for i, k := range v {
		x := r.X + k.Time*r.W
		col := ui.Theme.TextDim
		if i == w.sel {
			col = ui.Theme.Text
		}
		ui.Rect(mathx.Rect{X: x - 2, Y: r.Y + r.H*.58, W: 4, H: r.H * .42}, col)
	}
	if ui.Pressed(id, r) && len(v) > 0 {
		t := mathx.Clamp((ui.Mouse().X-r.X)/max(1, r.W), 0, 1)
		best := float32(2)
		for i, k := range v {
			if d := mathx.Abs(k.Time - t); d < best {
				best = d
				w.sel = i
			}
		}
		w.begin(keys)
		ui.CaptureMouse(id)
	}
	if ui.Captured() == id && w.editing {
		if ui.MouseDown(input.MouseLeft) {
			w.move((ui.Mouse().X - r.X) / max(1, r.W))
		}
		if ui.MouseReleased(input.MouseLeft) {
			w.commit(keys, apply)
			ui.ReleaseMouse()
			return
		}
	}
	if len(v) > 0 {
		col := v[w.sel].Color
		edit := widgets.ColorField(ui, ui.ID("gradient-key"), ui.Cursor().Next(0), &col, false)
		if edit.Begin || edit.Changed {
			w.begin(keys)
			w.draft[w.sel].Color = col
		}
		if edit.End {
			w.commit(keys, apply)
		}
	}
	row := ui.Cursor().Next(0)
	half := row.W / 2
	left, right := row, row
	left.W = half - 2
	right.X += half
	right.W = half
	if widgets.ButtonRect(ui, ui.ID("add-key"), left, i18n.Tr("editor.particles.add_key", "Add key")) && len(keys) < 256 {
		next, sel := addColor(keys)
		w.sel = sel
		w.editing = false
		apply(next)
	}
	if widgets.ButtonRect(ui, ui.ID("del-key"), right, i18n.Tr("editor.particles.delete_key", "Delete key")) && len(keys) > 0 {
		w.editing = false
		apply(slices.Delete(slices.Clone(keys), w.sel, w.sel+1))
	}
}
