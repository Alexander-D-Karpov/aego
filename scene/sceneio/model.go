package sceneio

import (
	"maps"
	"slices"
	"strings"
	"unsafe"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/scene"
	"aego/scene/prefab"
	"aego/scene/scriptcomp"
	"aego/schema"
)

func pseudo(name string, k schema.Kind) *schema.FieldDescriptor {
	return &schema.FieldDescriptor{Name: name, Kind: k}
}

func (l *loader) localOf(n *aetext.Node) ids.NodeID {
	if id, err := ids.ParseNodeID(n.Label); err == nil {
		return id
	}
	return ids.NewNodeID()
}

func transformKind(field string, v aetext.Value) (schema.Kind, bool) {
	list := v.Kind == aetext.VList
	switch field {
	case prefab.FieldPosition, prefab.FieldScale:
		switch {
		case list && len(v.List) == 2:
			return schema.KindVec2, true
		case list && len(v.List) == 3:
			return schema.KindVec3, true
		}
	case prefab.FieldRotation:
		switch {
		case list && len(v.List) == 4:
			return schema.KindQuat, true
		case v.Kind == aetext.VInt || v.Kind == aetext.VFloat:
			return schema.KindFloat32, true
		}
	}
	return schema.KindInvalid, false
}

func (l *loader) propField(p prefab.PropKey, v aetext.Value) (*schema.FieldDescriptor, prefab.PropKey, bool) {
	switch {
	case p.Owner == prefab.OwnerNode:
		switch p.Field {
		case prefab.FieldName:
			return pseudo(p.Field, schema.KindString), p, true
		case prefab.FieldEnabled:
			return pseudo(p.Field, schema.KindBool), p, true
		case prefab.FieldLayers:
			return pseudo(p.Field, schema.KindInt64), p, true
		}
	case p.Owner == prefab.OwnerTransform:
		if k, ok := transformKind(p.Field, v); ok {
			return pseudo(p.Field, k), p, true
		}
	case strings.HasPrefix(p.Owner, prefab.ScriptPrefix):
		td, ok := l.reg.ByName(strings.TrimPrefix(p.Owner, prefab.ScriptPrefix))
		if !ok || td.Kind != schema.TypeScript {
			break
		}
		owner := prefab.ScriptOwner(td.FullName)
		if p.Field == prefab.FieldActive {
			return pseudo(p.Field, schema.KindBool), prefab.PropKey{Owner: owner, Field: p.Field}, true
		}
		if fd, ok := td.Field(p.Field); ok {
			return fd, prefab.PropKey{Owner: owner, Field: fd.Name}, true
		}
	default:
		td, ok := l.reg.ByName(p.Owner)
		if !ok || td.Kind != schema.TypeComponent {
			break
		}
		if fd, ok := td.Field(p.Field); ok {
			return fd, prefab.PropKey{Owner: td.FullName, Field: fd.Name}, true
		}
	}
	return nil, p, false
}

func (l *loader) readModel(n *aetext.Node) (*prefab.Node, error) {
	local := l.localOf(n)
	if _, ok := n.Attr("prefab"); ok {
		inst, err := l.readInstance(n)
		if err != nil {
			return nil, err
		}
		return &prefab.Node{Local: local, Instance: inst}, nil
	}
	space3D := false
	if v, ok := n.Attr("space"); ok {
		sp, _ := scene.ParseSpace(v.Str)
		space3D = sp == scene.Space3D
	}
	name := "Node"
	if v, ok := n.Attr("name"); ok {
		name = v.Str
	}
	nd := prefab.NewNode(local, name, space3D)
	base := nd.Comp(prefab.OwnerNode)
	if v, ok := n.Attr("enabled"); ok {
		base.Set(prefab.FieldEnabled, schema.Bool(v.Bool))
	}
	if v, ok := n.Attr("layers"); ok {
		base.Set(prefab.FieldLayers, schema.Int64(v.AsInt()))
	}
	tr := nd.Comp(prefab.OwnerTransform)
	for _, b := range n.Blocks {
		switch b.Head {
		case "transform":
			if !space3D {
				t := readTransform(b)
				tr.Set(prefab.FieldPosition, schema.Vec2(t.Position))
				tr.Set(prefab.FieldRotation, schema.Float32(t.Rotation))
				tr.Set(prefab.FieldScale, schema.Vec2(t.Scale))
			}
		case "transform3d":
			if space3D {
				t := readTransform3D(b)
				tr.Set(prefab.FieldPosition, schema.Vec3(t.Position))
				tr.Set(prefab.FieldRotation, schema.Quat(t.Rotation))
				tr.Set(prefab.FieldScale, schema.Vec3(t.Scale))
			}
		case "component":
			if b.Label == scriptcomp.ScriptName {
				for _, sb := range b.Blocks {
					if sb.Head == "script" {
						nd.Comps = append(nd.Comps, l.modelScript(sb))
					}
				}
				continue
			}
			c, err := l.modelComp(b)
			if err != nil {
				return nil, err
			}
			nd.Comps = append(nd.Comps, c)
		case "node":
			child, err := l.readModel(b)
			if err != nil {
				return nil, err
			}
			nd.Children = append(nd.Children, child)
		}
	}
	return nd, nil
}

func (l *loader) modelComp(b *aetext.Node) (prefab.Comp, error) {
	td, ok := l.reg.ByName(b.Label)
	if !ok || td.Kind != schema.TypeComponent {
		return prefab.Comp{Owner: b.Label, Kind: prefab.CompUnknown, Raw: b.Clone()}, nil
	}
	p := td.New()
	if td.Default != nil && td.Copy != nil {
		td.Copy(p, td.Default())
	}
	c := prefab.Comp{Owner: td.FullName, Kind: prefab.CompComponent}
	td.Serializable(func(fd *schema.FieldDescriptor) {
		var v schema.Value
		fd.Get(p, &v)
		c.Fields = append(c.Fields, prefab.Field{Name: fd.Name, Value: v})
	})
	for _, f := range b.Fields {
		fd, ok := td.Field(f.Name)
		if !ok {
			l.report.warn(l.name, f.Pos, "unknown field "+td.FullName+"."+f.Name)
			continue
		}
		v, err := l.convert(fd, f.Value)
		if err != nil {
			if l.opt.Strict {
				return prefab.Comp{}, err
			}
			l.report.warn(l.name, f.Pos, err.Error())
			continue
		}
		c.Set(fd.Name, v)
	}
	return c, nil
}

func (l *loader) modelScript(sb *aetext.Node) prefab.Comp {
	enabled := true
	if v, ok := sb.Attr("enabled"); ok {
		enabled = v.Bool
	}
	active := prefab.Field{Name: prefab.FieldActive, Value: schema.Bool(enabled)}
	td, known := l.reg.ByName(sb.Label)
	if !known || td.Kind != schema.TypeScript {
		return prefab.Comp{Owner: prefab.ScriptOwner(sb.Label), Kind: prefab.CompScript, Fields: []prefab.Field{active}, Raw: slices.Clone(sb.Fields)}
	}
	c := prefab.Comp{Owner: prefab.ScriptOwner(td.FullName), Kind: prefab.CompScript, Fields: []prefab.Field{active}}
	var bag schema.PropertyBag
	for i := range td.Fields {
		var v schema.Value
		td.Fields[i].Get(unsafe.Pointer(&bag), &v)
		c.Fields = append(c.Fields, prefab.Field{Name: td.Fields[i].Name, Value: v})
	}
	var unknown []aetext.Field
	for _, f := range sb.Fields {
		fd, ok := td.Field(f.Name)
		if !ok {
			unknown = append(unknown, f)
			continue
		}
		v, err := scriptValue(fd, f.Value)
		if err != nil {
			unknown = append(unknown, f)
			continue
		}
		c.Set(fd.Name, v)
	}
	if len(unknown) > 0 {
		c.Raw = unknown
	}
	return c
}

func (l *loader) readInstance(n *aetext.Node) (*prefab.Instance, error) {
	v, _ := n.Attr("prefab")
	id, err := ids.ParseAssetID(v.Str)
	if err != nil {
		return nil, errs.Wrap(err, errs.SceneIOParse, "bad prefab reference").With(log.F("path", l.name), log.F("line", n.Pos.Line))
	}
	inst := &prefab.Instance{Prefab: id, IDs: map[prefab.Key]ids.NodeID{}}
	var extra []*aetext.Node
	for _, b := range n.Blocks {
		if !l.instanceEntry(inst, b) {
			extra = append(extra, b.Clone())
		}
	}
	if len(extra) > 0 {
		inst.Raw = extra
	}
	return inst, nil
}

func (l *loader) instanceEntry(inst *prefab.Instance, b *aetext.Node) bool {
	orphan := false
	if v, ok := b.Attr("orphan"); ok {
		orphan = v.Bool
	}
	k, okKey := prefab.ParseKey(b.Label)
	if !okKey {
		return false
	}
	o := &inst.Overrides
	switch b.Head {
	case "id":
		v, ok := b.Attr("node")
		if !ok {
			return false
		}
		id, err := ids.ParseNodeID(v.Str)
		if err != nil {
			return false
		}
		inst.IDs[k] = id
	case "override":
		pv, okp := b.Attr("prop")
		val, okv := b.Attr("value")
		if !okp || !okv {
			return false
		}
		pk, ok := prefab.ParsePropKey(pv.Str)
		if !ok {
			return false
		}
		fd, canon, ok := l.propField(pk, val)
		if !ok {
			return false
		}
		v, err := l.convert(fd, val)
		if err != nil {
			return false
		}
		o.Props = append(o.Props, prefab.PropOverride{Node: k, Prop: canon, Value: v, Orphan: orphan})
	case "addComponent":
		if len(b.Blocks) != 1 {
			return false
		}
		cb := b.Blocks[0]
		var c prefab.Comp
		switch cb.Head {
		case "component":
			mc, err := l.modelComp(cb)
			if err != nil {
				return false
			}
			c = mc
		case "script":
			c = l.modelScript(cb)
		default:
			return false
		}
		o.AddComps = append(o.AddComps, prefab.CompAdd{Node: k, Comp: c, Orphan: orphan})
	case "removeComponent":
		v, ok := b.Attr("owner")
		if !ok {
			return false
		}
		o.RemoveComps = append(o.RemoveComps, prefab.CompRemove{Node: k, Owner: v.Str})
	case "addNode":
		if len(b.Blocks) != 1 || b.Blocks[0].Head != "node" {
			return false
		}
		nd, err := l.readModel(b.Blocks[0])
		if err != nil {
			return false
		}
		o.AddNodes = append(o.AddNodes, prefab.NodeAdd{Parent: k, Node: nd, Orphan: orphan})
	case "remove":
		o.Remove = append(o.Remove, k)
	case "order":
		v, ok := b.Attr("children")
		if !ok {
			return false
		}
		ord := prefab.ChildOrder{Parent: k}
		for _, item := range v.List {
			ck, ok := prefab.ParseKey(item.Str)
			if !ok {
				return false
			}
			ord.Children = append(ord.Children, ck)
		}
		o.Order = append(o.Order, ord)
	default:
		return false
	}
	return true
}

func transform2DOf(c *prefab.Comp) scene.Transform2D {
	t := scene.IdentityTransform()
	if c == nil {
		return t
	}
	if v, ok := c.Get(prefab.FieldPosition); ok && v.Kind == schema.KindVec2 {
		t.Position = v.V
	}
	if v, ok := c.Get(prefab.FieldRotation); ok && v.Kind == schema.KindFloat32 {
		t.Rotation = float32(v.F)
	}
	if v, ok := c.Get(prefab.FieldScale); ok && v.Kind == schema.KindVec2 {
		t.Scale = v.V
	}
	return t
}

func transform3DOf(c *prefab.Comp) scene.Transform3D {
	t := scene.IdentityTransform3D()
	if c == nil {
		return t
	}
	if v, ok := c.Get(prefab.FieldPosition); ok && v.Kind == schema.KindVec3 {
		t.Position = v.V3
	}
	if v, ok := c.Get(prefab.FieldRotation); ok && v.Kind == schema.KindQuat {
		t.Rotation = v.Q
	}
	if v, ok := c.Get(prefab.FieldScale); ok && v.Kind == schema.KindVec3 {
		t.Scale = v.V3
	}
	return t
}

func (w *writer) fieldOf(p prefab.PropKey, v schema.Value) *schema.FieldDescriptor {
	if p.Field != prefab.FieldActive && p.Owner != prefab.OwnerNode && p.Owner != prefab.OwnerTransform {
		if td, ok := w.reg.ByName(strings.TrimPrefix(p.Owner, prefab.ScriptPrefix)); ok {
			if fd, ok := td.Field(p.Field); ok {
				return fd
			}
		}
	}
	return pseudo(p.Field, v.Kind)
}

func (w *writer) modelNode(nd *prefab.Node, idOf func(*prefab.Node) ids.NodeID, owned map[prefab.Key]bool) *aetext.Node {
	if nd.Instance != nil {
		return w.instanceBlock(idOf(nd), nd.Instance)
	}
	if nd.Origin != nil && owned != nil && !owned[nd.Key] {
		return w.instanceBlock(idOf(nd), nd.Origin)
	}
	n := &aetext.Node{Head: "node", Label: idOf(nd).String()}
	enabled, layers := true, int64(1)
	if v, ok := nd.Get(prefab.PropKey{Owner: prefab.OwnerNode, Field: prefab.FieldEnabled}); ok {
		enabled = v.I != 0
	}
	if v, ok := nd.Get(prefab.PropKey{Owner: prefab.OwnerNode, Field: prefab.FieldLayers}); ok {
		layers = v.AsInt()
	}
	n.Attrs = append(n.Attrs,
		aetext.Field{Name: "name", Value: aetext.String(nd.Name())},
		aetext.Field{Name: "enabled", Value: aetext.Bool(enabled)},
		aetext.Field{Name: "layers", Value: aetext.Int(layers)},
	)
	tr := nd.Comp(prefab.OwnerTransform)
	if nd.Space3D {
		n.Attrs = append(n.Attrs, aetext.Field{Name: "space", Value: aetext.String(scene.Space3D.String())})
		n.Blocks = append(n.Blocks, saveTransform3D(transform3DOf(tr)))
	} else {
		n.Blocks = append(n.Blocks, saveTransform(transform2DOf(tr)))
	}
	if nd.Origin != nil && !nd.Missing {
		n.Attrs = append(n.Attrs, aetext.Field{Name: "instanceOf", Value: aetext.AssetRef(nd.Origin.Prefab.String())})
	}
	var scripts *aetext.Node
	for i := range nd.Comps {
		c := &nd.Comps[i]
		switch c.Kind {
		case prefab.CompComponent, prefab.CompUnknown:
			if b := w.modelComp(c); b != nil {
				n.Blocks = append(n.Blocks, b)
			}
		case prefab.CompScript:
			if scripts == nil {
				scripts = &aetext.Node{Head: "component", Label: scriptcomp.ScriptName,
					Attrs: []aetext.Field{{Name: "version", Value: aetext.Int(1)}}}
			}
			scripts.Blocks = append(scripts.Blocks, w.modelScript(c))
		}
	}
	if scripts != nil {
		n.Blocks = append(n.Blocks, scripts)
	}
	for _, c := range nd.Children {
		n.Blocks = append(n.Blocks, w.modelNode(c, idOf, owned))
	}
	return n
}

func (w *writer) modelComp(c *prefab.Comp) *aetext.Node {
	if c.Kind == prefab.CompUnknown {
		if raw, ok := c.Raw.(*aetext.Node); ok {
			return raw.Clone()
		}
		return nil
	}
	td, ok := w.reg.ByName(c.Owner)
	if !ok {
		return nil
	}
	n := &aetext.Node{Head: "component", Label: td.FullName}
	n.Attrs = append(n.Attrs, aetext.Field{Name: "version", Value: aetext.Int(int64(td.Version))})
	td.Serializable(func(fd *schema.FieldDescriptor) {
		if v, ok := c.Get(fd.Name); ok {
			n.Fields = append(n.Fields, aetext.Field{Name: fd.Name, Value: w.value(fd, v)})
		}
	})
	return n
}

func (w *writer) modelScript(c *prefab.Comp) *aetext.Node {
	full := strings.TrimPrefix(c.Owner, prefab.ScriptPrefix)
	enabled := true
	if v, ok := c.Get(prefab.FieldActive); ok {
		enabled = v.I != 0
	}
	b := &aetext.Node{Head: "script", Label: full}
	b.Attrs = append(b.Attrs, aetext.Field{Name: "enabled", Value: aetext.Bool(enabled)})
	td, known := w.reg.ByName(full)
	known = known && td.Kind == schema.TypeScript
	if known {
		for i := range td.Fields {
			fd := &td.Fields[i]
			if v, ok := c.Get(fd.Name); ok {
				b.Fields = append(b.Fields, aetext.Field{Name: fd.Name, Value: w.value(fd, v)})
			}
		}
	}
	if raw, ok := c.Raw.([]aetext.Field); ok {
		for _, f := range raw {
			if known {
				if _, dup := td.Field(f.Name); dup {
					continue
				}
			}
			b.Fields = append(b.Fields, f)
		}
	}
	return b
}

func (w *writer) compBlock(c *prefab.Comp) *aetext.Node {
	if c.Kind == prefab.CompScript {
		return w.modelScript(c)
	}
	return w.modelComp(c)
}

func entry(head, label string, attrs ...aetext.Field) *aetext.Node {
	return &aetext.Node{Head: head, Label: label, Attrs: attrs}
}

func orphaned(b *aetext.Node, orphan bool) *aetext.Node {
	if orphan {
		b.Attrs = append(b.Attrs, aetext.Field{Name: "orphan", Value: aetext.Bool(true)})
	}
	return b
}

func localID(n *prefab.Node) ids.NodeID {
	return n.Local
}

func (w *writer) instanceBlock(label ids.NodeID, inst *prefab.Instance) *aetext.Node {
	n := &aetext.Node{Head: "node", Label: label.String()}
	n.Attrs = append(n.Attrs, aetext.Field{Name: "prefab", Value: aetext.AssetRef(inst.Prefab.String())})
	for _, k := range slices.Sorted(maps.Keys(inst.IDs)) {
		n.Blocks = append(n.Blocks, entry("id", string(k), aetext.Field{Name: "node", Value: aetext.NodeRef(inst.IDs[k].String())}))
	}
	o := inst.Overrides.Clone()
	o.Sort()
	for _, p := range o.Props {
		b := entry("override", string(p.Node),
			aetext.Field{Name: "prop", Value: aetext.String(p.Prop.String())},
			aetext.Field{Name: "value", Value: w.value(w.fieldOf(p.Prop, p.Value), p.Value)})
		n.Blocks = append(n.Blocks, orphaned(b, p.Orphan))
	}
	for i := range o.AddComps {
		a := &o.AddComps[i]
		cb := w.compBlock(&a.Comp)
		if cb == nil {
			continue
		}
		b := orphaned(entry("addComponent", string(a.Node)), a.Orphan)
		b.Blocks = append(b.Blocks, cb)
		n.Blocks = append(n.Blocks, b)
	}
	for _, r := range o.RemoveComps {
		n.Blocks = append(n.Blocks, entry("removeComponent", string(r.Node), aetext.Field{Name: "owner", Value: aetext.String(r.Owner)}))
	}
	for _, a := range o.AddNodes {
		b := orphaned(entry("addNode", string(a.Parent)), a.Orphan)
		b.Blocks = append(b.Blocks, w.modelNode(a.Node, localID, nil))
		n.Blocks = append(n.Blocks, b)
	}
	for _, k := range o.Remove {
		n.Blocks = append(n.Blocks, entry("remove", string(k)))
	}
	for _, ord := range o.Order {
		items := make([]aetext.Value, len(ord.Children))
		for i, k := range ord.Children {
			items[i] = aetext.String(string(k))
		}
		n.Blocks = append(n.Blocks, entry("order", string(ord.Parent), aetext.Field{Name: "children", Value: aetext.List(items...)}))
	}
	if extra, ok := inst.Raw.([]*aetext.Node); ok {
		for _, e := range extra {
			n.Blocks = append(n.Blocks, e.Clone())
		}
	}
	return n
}
