package sceneio

import (
	"unsafe"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/format/aetext"
	"aego/scene"
	"aego/scene/scriptcomp"
	"aego/schema"
)

func (l *loader) loadScripts(h scene.NodeHandle, n *aetext.Node) error {
	sc := scene.Add[scriptcomp.ScriptComponent](l.scene, h)
	if sc == nil {
		return nil
	}
	for _, b := range n.Blocks {
		if b.Head != "script" {
			l.report.warn(l.name, b.Pos, "unknown block "+b.Head+" in "+scriptcomp.ScriptName)
			continue
		}
		var refs []deferredRef
		att := scriptcomp.Attachment{TypeName: b.Label, Enabled: true}
		if v, ok := b.Attr("enabled"); ok {
			att.Enabled = v.Bool
		}
		td, known := l.reg.ByName(b.Label)
		if known && td.Kind == schema.TypeScript {
			att.TypeName = td.FullName
			for _, f := range b.Fields {
				fd, ok := td.Field(f.Name)
				if !ok {
					att.Unknown = append(att.Unknown, f)
					l.report.warn(l.name, f.Pos, "unknown property "+td.FullName+"."+f.Name+" kept as is")
					continue
				}
				v, err := scriptValue(fd, f.Value)
				if err != nil {
					if l.opt.Strict {
						return err
					}
					att.Unknown = append(att.Unknown, f)
					l.report.warn(l.name, f.Pos, err.Error())
					continue
				}
				if hasNodes(v) {
					refs = append(refs, deferredRef{node: h, value: v, fname: fd.Name})
				}
				att.Props.Set(fd.Name, v)
			}
		} else {
			att.Unknown = append(att.Unknown, b.Fields...)
			l.report.Unknown = append(l.report.Unknown, "script "+b.Label)
		}
		if sc.Index(att.TypeName) >= 0 {
			l.report.warn(l.name, b.Pos, "script "+att.TypeName+" is attached twice, the second copy is dropped")
			continue
		}
		sc.Attachments = append(sc.Attachments, att)
		for _, r := range refs {
			r.script, r.name = att.TypeName, att.TypeName
			l.deferred = append(l.deferred, r)
		}
	}
	return nil
}

func scriptValue(fd *schema.FieldDescriptor, v aetext.Value) (schema.Value, error) {
	bad := errs.New(errs.SceneIOParse, "value does not fit script property "+fd.Name)
	numeric := v.Kind == aetext.VInt || v.Kind == aetext.VFloat
	switch fd.Kind {
	case schema.KindBool:
		if v.Kind == aetext.VBool {
			return schema.Bool(v.Bool), nil
		}
	case schema.KindInt32:
		if numeric {
			return schema.Int32(int32(v.AsInt())), nil
		}
	case schema.KindInt64:
		if numeric {
			return schema.Int64(v.AsInt()), nil
		}
	case schema.KindFloat32:
		if numeric {
			return schema.Float32(float32(v.AsFloat())), nil
		}
	case schema.KindFloat64:
		if numeric {
			return schema.Float64(v.AsFloat()), nil
		}
	case schema.KindString:
		if v.Kind == aetext.VString {
			return schema.String(v.Str), nil
		}
	case schema.KindVec2:
		if v.Kind == aetext.VList && len(v.List) == 2 {
			return schema.Vec2(readVec2(v, schema.Value{}.V)), nil
		}
	case schema.KindVec3:
		if v.Kind == aetext.VList && len(v.List) == 3 {
			return schema.Vec3(readVec3(v, schema.Value{}.V3)), nil
		}
	case schema.KindVec4:
		if v.Kind == aetext.VList && len(v.List) == 4 {
			return schema.Vec4(readVec4(v, schema.Value{}.V4)), nil
		}
	case schema.KindQuat:
		if v.Kind == aetext.VList && len(v.List) == 4 {
			q := readVec4(v, schema.Value{}.V4)
			return schema.Quat(mathx.Quat(q)), nil
		}
	case schema.KindColor:
		if v.Kind == aetext.VColor {
			return schema.Color(unpackColor(v.Color)), nil
		}
	case schema.KindRect:
		if v.Kind == aetext.VList && len(v.List) == 4 {
			return schema.Rect(readRect(v)), nil
		}
	case schema.KindEnum:
		if v.Kind == aetext.VIdent {
			if n, ok := fd.EnumValueOf(v.Ident); ok {
				return schema.Enum(n), nil
			}
			return schema.Value{}, errs.New(errs.SceneIOParse, "unknown enum value "+v.Ident+" for "+fd.Name)
		}
		if numeric {
			return schema.Enum(v.AsInt()), nil
		}
	case schema.KindAssetRef:
		if v.Kind == aetext.VAsset || v.Kind == aetext.VString {
			if v.Str == "" {
				return schema.Asset(ids.AssetID{}), nil
			}
			id, err := ids.ParseAssetID(v.Str)
			if err != nil {
				return schema.Value{}, err
			}
			return schema.Asset(id), nil
		}
	case schema.KindNodeRef:
		if v.Kind == aetext.VNode || v.Kind == aetext.VString {
			if v.Str == "" {
				return schema.Node(ids.NodeID{}), nil
			}
			id, err := ids.ParseNodeID(v.Str)
			if err != nil {
				return schema.Value{}, err
			}
			return schema.Node(id), nil
		}
	}
	return schema.Value{}, bad
}

func (w *writer) scripts(sc *scriptcomp.ScriptComponent) *aetext.Node {
	n := &aetext.Node{Head: "component", Label: scriptcomp.ScriptName}
	n.Attrs = append(n.Attrs, aetext.Field{Name: "version", Value: aetext.Int(1)})
	for i := range sc.Attachments {
		a := &sc.Attachments[i]
		b := &aetext.Node{Head: "script", Label: a.TypeName}
		b.Attrs = append(b.Attrs, aetext.Field{Name: "enabled", Value: aetext.Bool(a.Enabled)})
		td, known := w.reg.ByName(a.TypeName)
		known = known && td.Kind == schema.TypeScript
		if known {
			b.Label = td.FullName
			for j := range td.Fields {
				fd := &td.Fields[j]
				var v schema.Value
				fd.Get(unsafe.Pointer(&a.Props), &v)
				b.Fields = append(b.Fields, aetext.Field{Name: fd.Name, Value: w.value(fd, v)})
			}
		}
		for _, f := range a.Unknown {
			if known {
				if _, dup := td.Field(f.Name); dup {
					continue
				}
			}
			b.Fields = append(b.Fields, f)
		}
		n.Blocks = append(n.Blocks, b)
	}
	return n
}
