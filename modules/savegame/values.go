package savegame

import (
	"aego/contracts/persistence"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/format/aetext"
	"aego/save"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/script/abi"
	"aego/script/bind"
	"aego/script/vm"
)

func laneWidth(k abi.Kind) int {
	switch k {
	case abi.Vec2:
		return 2
	case abi.Vec3:
		return 3
	case abi.Color, abi.Quat, abi.Rect:
		return 4
	}
	return 0
}

func floatList(v vm.Value, n int) aetext.Value {
	out := make([]aetext.Value, n)
	for i := range out {
		out[i] = aetext.Float32(v.Lane(i))
	}
	return aetext.List(out...)
}

func lanesOf(v aetext.Value, n int) ([4]float32, bool) {
	var out [4]float32
	if v.Kind != aetext.VList || len(v.List) != n {
		return out, false
	}
	for i := 0; i < n; i++ {
		out[i] = float32(v.List[i].AsFloat())
	}
	return out, true
}

func (s *Service) scriptValue(sv persistence.ScriptValue) (aetext.Value, bool) {
	v := sv.Value
	switch sv.Kind {
	case abi.Bool:
		return aetext.Bool(v.Bool()), true
	case abi.Int, abi.Enum:
		return aetext.Int(v.Int()), true
	case abi.Float:
		return aetext.Float32(v.Float()), true
	case abi.String:
		return aetext.String(sv.Text), true
	case abi.AssetRef:
		return aetext.AssetRef(bind.AssetOf(v).String()), true
	case abi.NodeRef:
		return s.RefValue(bind.NodeOf(v))
	}
	if n := laneWidth(sv.Kind); n > 0 {
		return floatList(v, n), true
	}
	return aetext.Value{}, false
}

func (s *Service) fromScript(kind abi.Kind, a aetext.Value) (vm.Value, string, bool) {
	switch kind {
	case abi.Bool:
		return vm.Bool(a.Bool), "", a.Kind == aetext.VBool
	case abi.Int, abi.Enum:
		return vm.Int(a.AsInt()), "", a.Kind == aetext.VInt || a.Kind == aetext.VFloat
	case abi.Float:
		return vm.Float(float32(a.AsFloat())), "", a.Kind == aetext.VInt || a.Kind == aetext.VFloat
	case abi.String:
		return vm.Value{}, a.Str, a.Kind == aetext.VString
	case abi.AssetRef:
		id, err := ids.ParseAssetID(a.Str)
		return bind.AssetValue(id), "", err == nil
	case abi.NodeRef:
		h, ok := s.ResolveRef(a)
		return bind.NodeValue(h), "", ok
	}
	if n := laneWidth(kind); n > 0 {
		l, ok := lanesOf(a, n)
		return vm.Vec(l[0], l[1], l[2], l[3]), "", ok
	}
	return vm.Value{}, "", false
}

func (s *Service) fieldValue(reg *schema.Registry, fd *schema.FieldDescriptor, v schema.Value) (aetext.Value, bool) {
	if v.Kind == schema.KindNodeRef {
		if v.Node.IsZero() {
			return aetext.NodeRef(v.Node.String()), true
		}
		return s.RefValue(s.scene.Handle(v.Node))
	}
	return sceneio.WriteValue(reg, fd, v), true
}

func (s *Service) readField(reg *schema.Registry, fd *schema.FieldDescriptor, a aetext.Value) (schema.Value, bool) {
	if fd.Kind == schema.KindNodeRef {
		h, ok := s.ResolveRef(a)
		if !ok {
			return schema.Value{}, false
		}
		if h.IsZero() {
			return schema.Node(ids.NodeID{}), true
		}
		return schema.Node(s.scene.ID(h)), true
	}
	v, err := sceneio.ReadValue(reg, fd, a)
	return v, err == nil
}

func transformNode(sc *scene.Scene, h scene.NodeHandle) *aetext.Node {
	if sc.Space(h) == scene.Space3D {
		t := sc.Local3D(h)
		n := &aetext.Node{Head: "transform3d"}
		w := save.NewWriter(n, nil)
		w.Vec3("position", t.Position)
		w.Vec4("rotation", mathx.Vec4(t.Rotation))
		w.Vec3("scale", t.Scale)
		return n
	}
	t := sc.Local(h)
	n := &aetext.Node{Head: "transform"}
	w := save.NewWriter(n, nil)
	w.Vec2("position", t.Position)
	w.Float("rotation", float64(t.Rotation))
	w.Vec2("scale", t.Scale)
	return n
}

func applyTransform(sc *scene.Scene, h scene.NodeHandle, r *save.Reader) {
	if b := r.Block("transform", ""); b != nil && sc.Space(h) == scene.Space2D {
		t := sc.Local(h)
		t.Position = b.Vec2("position", t.Position)
		t.Rotation = float32(b.Float("rotation", float64(t.Rotation)))
		t.Scale = b.Vec2("scale", t.Scale)
		sc.SetLocal(h, t)
	}
	if b := r.Block("transform3d", ""); b != nil && sc.Space(h) == scene.Space3D {
		t := sc.Local3D(h)
		t.Position = b.Vec3("position", t.Position)
		t.Rotation = mathx.Quat(b.Vec4("rotation", mathx.Vec4(t.Rotation)))
		t.Scale = b.Vec3("scale", t.Scale)
		sc.SetLocal3D(h, t)
	}
}

func cloneFields(fs []aetext.Field) []aetext.Field {
	if len(fs) == 0 {
		return nil
	}
	out := make([]aetext.Field, len(fs))
	for i, f := range fs {
		out[i] = aetext.Field{Name: f.Name, Value: f.Value.Clone()}
	}
	return out
}
