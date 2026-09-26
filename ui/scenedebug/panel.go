package scenedebug

import (
	"strconv"

	"aego/asset"
	"aego/core/mathx"
	"aego/scene"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/layout"
	"aego/ui/widgets"
)

type Panel struct {
	scene    *scene.Scene
	res      asset.Resolver
	selected scene.NodeHandle
	open     map[scene.NodeHandle]bool
	scroll   float32
	buf      []byte
}

func New(s *scene.Scene, res asset.Resolver) *Panel {
	return &Panel{
		scene: s,
		res:   res,
		open:  map[scene.NodeHandle]bool{},
		buf:   make([]byte, 0, 64),
	}
}

func (p *Panel) Selected() scene.NodeHandle {
	return p.selected
}

func (p *Panel) Draw(c *core.Context) {
	s := p.scene
	widgets.LabelDim(c, "hierarchy")
	rows := 0
	s.Children(s.Root(), func(h scene.NodeHandle) bool {
		rows += p.visibleRows(h)
		return true
	})
	rowH := c.Theme.RowHeight + c.Theme.Spacing
	height := min(float32(rows)*rowH, 200)
	layout.ScrollArea(c, core.IDOf("scenedebug.tree"), height, &p.scroll, func() {
		s.Children(s.Root(), func(h scene.NodeHandle) bool {
			p.drawNode(c, h, 0)
			return true
		})
	})
	widgets.Separator(c)

	if p.selected.IsZero() || !s.IsAlive(p.selected) {
		widgets.LabelDim(c, "no selection")
		return
	}
	widgets.Label(c, s.Path(p.selected))
	p.drawTransform(c)
	s.ComponentsOf(p.selected, func(id schema.TypeID) {
		p.drawComponent(c, id)
	})
}

func (p *Panel) visibleRows(h scene.NodeHandle) int {
	n := 1
	if !p.open[h] {
		return n
	}
	p.scene.Children(h, func(child scene.NodeHandle) bool {
		n += p.visibleRows(child)
		return true
	})
	return n
}

func (p *Panel) drawNode(c *core.Context, h scene.NodeHandle, depth int) {
	s := p.scene
	id := core.IDSeq(core.IDOf("node"), int(h.Index))
	open := p.open[h]
	label := s.Name(h)
	if !s.EffectiveEnabled(h) {
		label = "(" + label + ")"
	}
	hasChildren := s.ChildCount(h) > 0
	if !hasChildren {
		if widgets.Selectable(c, id, label, h == p.selected) {
			p.selected = h
		}
		return
	}
	if widgets.TreeNode(c, id, label, &open, h == p.selected) {
		p.selected = h
	}
	p.open[h] = open
	if !open {
		return
	}
	layout.Indent(c, c.Theme.RowHeight, func() {
		s.Children(h, func(child scene.NodeHandle) bool {
			p.drawNode(c, child, depth+1)
			return true
		})
	})
}

func (p *Panel) drawTransform(c *core.Context) {
	s := p.scene
	t := s.Local(p.selected)
	changed := false
	if widgets.DragFloat(c, core.IDOf("tr.x"), "x", &t.Position.X, 0.05) {
		changed = true
	}
	if widgets.DragFloat(c, core.IDOf("tr.y"), "y", &t.Position.Y, 0.05) {
		changed = true
	}
	if widgets.DragFloat(c, core.IDOf("tr.rot"), "rotation", &t.Rotation, 0.01) {
		changed = true
	}
	if changed {
		s.SetLocal(p.selected, t)
	}
	enabled := s.Enabled(p.selected)
	if widgets.Checkbox(c, "enabled", &enabled) {
		s.SetEnabled(p.selected, enabled)
	}
}

func (p *Panel) drawComponent(c *core.Context, typeID schema.TypeID) {
	s := p.scene
	td, ok := s.Registry().ByID(typeID)
	if !ok {
		return
	}
	widgets.Separator(c)
	widgets.LabelDim(c, td.FullName)
	for i := range td.Fields {
		fd := &td.Fields[i]
		if !fd.Flags.Has(schema.FlagEditor) {
			continue
		}
		p.drawField(c, typeID, fd)
	}
}

func (p *Panel) drawField(c *core.Context, typeID schema.TypeID, fd *schema.FieldDescriptor) {
	s := p.scene
	v, err := s.GetProperty(p.selected, typeID, fd.ID)
	if err != nil {
		return
	}
	id := core.IDSeq(core.ID(fd.ID), int(p.selected.Index))
	readOnly := fd.Flags.Has(schema.FlagReadOnly)
	switch fd.Kind {
	case schema.KindFloat32, schema.KindFloat64:
		f := float32(v.AsFloat())
		if widgets.DragFloat(c, id, fd.Name, &f, 0.05) && !readOnly {
			s.SetProperty(p.selected, typeID, fd.ID, schema.Float32(f))
		}
	case schema.KindInt32, schema.KindInt64:
		n := int32(v.AsInt())
		if widgets.DragInt(c, id, fd.Name, &n) && !readOnly {
			s.SetProperty(p.selected, typeID, fd.ID, schema.Int32(n))
		}
	case schema.KindBool:
		b := v.I != 0
		if widgets.Checkbox(c, fd.Name, &b) && !readOnly {
			s.SetProperty(p.selected, typeID, fd.ID, schema.Bool(b))
		}
	case schema.KindVec2:
		vec := v.V
		changed := widgets.DragFloat(c, core.IDSeq(id, 1), fd.Name+".x", &vec.X, 0.05)
		if widgets.DragFloat(c, core.IDSeq(id, 2), fd.Name+".y", &vec.Y, 0.05) {
			changed = true
		}
		if changed && !readOnly {
			s.SetProperty(p.selected, typeID, fd.ID, schema.Vec2(vec))
		}
	default:
		p.buf = p.buf[:0]
		p.buf = append(p.buf, fd.Name...)
		p.buf = append(p.buf, ": "...)
		p.buf = appendValue(p.buf, v)
		widgets.LabelDim(c, string(p.buf))
	}
}

func appendValue(b []byte, v schema.Value) []byte {
	switch v.Kind {
	case schema.KindString:
		return append(b, v.S...)
	case schema.KindAssetRef:
		if v.Asset.IsZero() {
			return append(b, "none"...)
		}
		return append(b, v.Asset.String()[:8]...)
	case schema.KindNodeRef:
		if v.Node.IsZero() {
			return append(b, "none"...)
		}
		return append(b, v.Node.String()[:8]...)
	case schema.KindColor:
		return append(b, "color"...)
	case schema.KindList:
		b = append(b, '[')
		b = strconv.AppendInt(b, int64(len(v.List)), 10)
		return append(b, ']')
	}
	return append(b, "…"...)
}

var _ = mathx.Vec2{}
