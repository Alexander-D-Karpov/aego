package sceneio

import (
	"unsafe"

	"aego/core/mathx"
	"aego/format/aetext"
	"aego/scene"
	"aego/scene/scriptcomp"
	"aego/schema"
)

func Save(s *scene.Scene) []byte {
	return SaveWith(s, nil)
}

func SaveWith(s *scene.Scene, lib *Library) []byte {
	return save(s, s.Root(), "scene", false, lib)
}

func SavePrefab(s *scene.Scene) []byte {
	return save(s, s.Root(), "prefab", false, nil)
}

func SaveSubtree(s *scene.Scene, root scene.NodeHandle, asPrefab bool) []byte {
	head := "scene"
	if asPrefab {
		head = "prefab"
	}
	return save(s, root, head, true, nil)
}

func save(s *scene.Scene, root scene.NodeHandle, head string, includeRoot bool, lib *Library) []byte {
	if lib == nil {
		lib = NewLibrary(s.Registry(), s.Resolver())
	}
	w := &writer{reg: s.Registry(), lib: lib}
	doc := &aetext.Node{Head: head}
	doc.Attrs = append(doc.Attrs, aetext.Field{Name: "version", Value: aetext.Int(FormatVersion)})
	if !includeRoot && s.DefaultSpace() == scene.Space3D {
		doc.Attrs = append(doc.Attrs, aetext.Field{Name: "space", Value: aetext.String(scene.Space3D.String())})
	}
	if includeRoot {
		if n := w.node(s, root); n != nil {
			doc.Blocks = append(doc.Blocks, n)
		}
	} else {
		s.Children(root, func(h scene.NodeHandle) bool {
			if n := w.node(s, h); n != nil {
				doc.Blocks = append(doc.Blocks, n)
			}
			return true
		})
	}
	return aetext.Write(doc)
}

type writer struct {
	reg     *schema.Registry
	lib     *Library
	flatten scene.NodeHandle
	plain   bool
}

func (w *writer) node(s *scene.Scene, h scene.NodeHandle) *aetext.Node {
	if !s.IsAlive(h) {
		return nil
	}
	pi, marked := InstanceOf(s, h)
	inner := marked && h != w.flatten && !w.plain
	if inner && (pi.Owner || pi.Missing) {
		if b := w.instance(s, h, pi); b != nil {
			return b
		}
	}
	n := &aetext.Node{Head: "node", Label: s.ID(h).String()}
	n.Attrs = append(n.Attrs,
		aetext.Field{Name: "name", Value: aetext.String(s.Name(h))},
		aetext.Field{Name: "enabled", Value: aetext.Bool(s.Enabled(h))},
		aetext.Field{Name: "layers", Value: aetext.Int(int64(s.Layers(h)))},
	)
	if s.Space(h) == scene.Space3D {
		n.Attrs = append(n.Attrs, aetext.Field{Name: "space", Value: aetext.String(scene.Space3D.String())})
		n.Blocks = append(n.Blocks, saveTransform3D(s.Local3D(h)))
	} else {
		n.Blocks = append(n.Blocks, saveTransform(s.Local(h)))
	}
	if inner && !pi.Owner && !pi.Missing {
		n.Attrs = append(n.Attrs, aetext.Field{Name: "instanceOf", Value: aetext.AssetRef(pi.Prefab.String())})
	}
	s.ComponentsOf(h, func(id schema.TypeID) {
		td, ok := w.reg.ByID(id)
		if !ok || td.FullName == "engine.Unknown" || td.FullName == scriptcomp.ScriptName || td.FullName == PrefabName {
			return
		}
		p, ok := s.GetByType(h, id)
		if !ok {
			return
		}
		n.Blocks = append(n.Blocks, w.component(td, p))
	})
	if sc, ok := scriptcomp.ScriptsOf(s, h); ok && len(sc.Attachments) > 0 {
		n.Blocks = append(n.Blocks, w.scripts(sc))
	}
	if u, ok := scene.Get[Unknown](s, h); ok {
		for i := range u.Components {
			c := &u.Components[i]
			block := &aetext.Node{Head: "component", Label: c.TypeName}
			block.Attrs = append(block.Attrs, aetext.Field{Name: "version", Value: aetext.Int(int64(c.Version))})
			block.Fields = c.Fields
			block.Blocks = c.Blocks
			n.Blocks = append(n.Blocks, block)
		}
	}
	s.Children(h, func(child scene.NodeHandle) bool {
		if c := w.node(s, child); c != nil {
			n.Blocks = append(n.Blocks, c)
		}
		return true
	})
	return n
}

func saveTransform(t scene.Transform2D) *aetext.Node {
	n := &aetext.Node{Head: "transform"}
	n.Fields = append(n.Fields,
		aetext.Field{Name: "position", Value: vec2Value(t.Position)},
		aetext.Field{Name: "rotation", Value: aetext.Float32(t.Rotation)},
		aetext.Field{Name: "scale", Value: vec2Value(t.Scale)},
	)
	return n
}

func saveTransform3D(t scene.Transform3D) *aetext.Node {
	q := t.Rotation
	n := &aetext.Node{Head: "transform3d"}
	n.Fields = append(n.Fields,
		aetext.Field{Name: "position", Value: vec3Value(t.Position)},
		aetext.Field{Name: "rotation", Value: vec4Value(mathx.Vec4(q))},
		aetext.Field{Name: "scale", Value: vec3Value(t.Scale)},
	)
	return n
}

func (w *writer) component(td *schema.TypeDescriptor, p unsafe.Pointer) *aetext.Node {
	n := &aetext.Node{Head: "component", Label: td.FullName}
	n.Attrs = append(n.Attrs, aetext.Field{Name: "version", Value: aetext.Int(int64(td.Version))})
	td.Serializable(func(fd *schema.FieldDescriptor) {
		var v schema.Value
		fd.Get(p, &v)
		n.Fields = append(n.Fields, aetext.Field{Name: fd.Name, Value: w.value(fd, v)})
	})
	return n
}

func (w *writer) value(fd *schema.FieldDescriptor, v schema.Value) aetext.Value {
	switch v.Kind {
	case schema.KindBool:
		return aetext.Bool(v.I != 0)
	case schema.KindInt32, schema.KindInt64:
		return aetext.Int(v.I)
	case schema.KindFloat32:
		return aetext.Float32(float32(v.F))
	case schema.KindFloat64:
		return aetext.Float64(v.F)
	case schema.KindString:
		return aetext.String(v.S)
	case schema.KindVec2:
		return vec2Value(v.V)
	case schema.KindVec3:
		return vec3Value(v.V3)
	case schema.KindVec4:
		return vec4Value(v.V4)
	case schema.KindQuat:
		return vec4Value(mathx.Vec4{X: v.Q.X, Y: v.Q.Y, Z: v.Q.Z, W: v.Q.W})
	case schema.KindColor:
		return aetext.Color(packColor(v.C))
	case schema.KindRect:
		return aetext.List(
			aetext.Float32(v.R.X), aetext.Float32(v.R.Y),
			aetext.Float32(v.R.W), aetext.Float32(v.R.H),
		)
	case schema.KindEnum:
		if name, ok := fd.EnumNameOf(v.I); ok {
			return aetext.Ident(name)
		}
		return aetext.Int(v.I)
	case schema.KindAssetRef:
		return aetext.AssetRef(v.Asset.String())
	case schema.KindNodeRef:
		return aetext.NodeRef(v.Node.String())
	case schema.KindList:
		out := aetext.Value{Kind: aetext.VList}
		elem := schema.FieldDescriptor{Kind: fd.Elem, Struct: fd.Struct, Enum: fd.Enum}
		for _, item := range v.List {
			out.List = append(out.List, w.value(&elem, item))
		}
		return out
	case schema.KindStruct:
		out := aetext.Value{Kind: aetext.VStruct}
		if v.Obj == nil {
			return out
		}
		td, ok := w.reg.ByID(v.Obj.Type)
		if !ok {
			return out
		}
		for i := range td.Fields {
			if i >= len(v.Obj.Fields) {
				break
			}
			out.Obj = append(out.Obj, aetext.Field{
				Name:  td.Fields[i].Name,
				Value: w.value(&td.Fields[i], v.Obj.Fields[i]),
			})
		}
		return out
	}
	return aetext.Bool(false)
}

func vec2Value(v mathx.Vec2) aetext.Value {
	return aetext.List(aetext.Float32(v.X), aetext.Float32(v.Y))
}

func vec3Value(v mathx.Vec3) aetext.Value {
	return aetext.List(aetext.Float32(v.X), aetext.Float32(v.Y), aetext.Float32(v.Z))
}

func vec4Value(v mathx.Vec4) aetext.Value {
	return aetext.List(aetext.Float32(v.X), aetext.Float32(v.Y), aetext.Float32(v.Z), aetext.Float32(v.W))
}

func packColor(c mathx.Color) uint32 {
	enc := func(f float32) uint32 {
		return uint32(mathx.Clamp(mathx.LinearToSRGB(mathx.Clamp(f, 0, 1)), 0, 1)*255 + 0.5)
	}
	alpha := uint32(mathx.Clamp(c.A, 0, 1)*255 + 0.5)
	return enc(c.R)<<24 | enc(c.G)<<16 | enc(c.B)<<8 | alpha
}
