package props

import (
	"math"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/history"
	"aego/editor/core/history/commands"
	"aego/engine/i18n"
	"aego/scene"
	"aego/schema"
)

type Access struct {
	s *scene.Scene
	h *history.History
}

func New(s *scene.Scene, h *history.History) *Access {
	return &Access{s: s, h: h}
}

func (a *Access) Scene() *scene.Scene {
	return a.s
}

func (a *Access) History() *history.History {
	return a.h
}

func (a *Access) Descriptor(t schema.TypeID, f schema.FieldID) (*schema.TypeDescriptor, *schema.FieldDescriptor, error) {
	td, ok := a.s.Registry().ByID(t)
	if !ok {
		return nil, nil, errs.New(errs.SchemaUnknownType, "component type is not registered")
	}
	fd, ok := td.FieldByID(f)
	if !ok {
		return td, nil, errs.New(errs.SchemaBadField, "unknown field", log.F("type", td.FullName))
	}
	return td, fd, nil
}

func (a *Access) handle(node ids.NodeID) (scene.NodeHandle, error) {
	h := a.s.Handle(node)
	if h.IsZero() || !a.s.IsAlive(h) {
		return h, errs.New(errs.SceneDeadNode, "node is not in the scene", log.F("node", node.String()))
	}
	return h, nil
}

func (a *Access) Get(node ids.NodeID, t schema.TypeID, f schema.FieldID) (schema.Value, error) {
	h, err := a.handle(node)
	if err != nil {
		return schema.Value{}, err
	}
	return a.s.GetProperty(h, t, f)
}

func (a *Access) Common(nodes []ids.NodeID, t schema.TypeID, f schema.FieldID) (schema.Value, bool, error) {
	var first schema.Value
	for i, n := range nodes {
		v, err := a.Get(n, t, f)
		if err != nil {
			return schema.Value{}, false, err
		}
		if i == 0 {
			first = v
			continue
		}
		if !Equal(first, v) {
			return first, true, nil
		}
	}
	return first, false, nil
}

func (a *Access) Validate(t schema.TypeID, f schema.FieldID, v schema.Value) error {
	_, err := a.Normalize(t, f, v)
	return err
}

func (a *Access) Normalize(t schema.TypeID, f schema.FieldID, v schema.Value) (schema.Value, error) {
	_, fd, err := a.Descriptor(t, f)
	if err != nil {
		return v, err
	}
	return Normalize(fd, v)
}

func Label(fd *schema.FieldDescriptor) string {
	if fd.Hints.DisplayName != "" {
		return fd.Hints.DisplayName
	}
	return fd.Name
}

func (a *Access) within(tx *history.Transaction, label string, fn func(*history.Transaction) error) error {
	own := tx == nil
	if own {
		tx = a.h.Begin(label)
	}
	if err := fn(tx); err != nil {
		if own {
			tx.Abort()
		}
		return err
	}
	if own {
		tx.Commit()
	}
	return nil
}

func (a *Access) Set(tx *history.Transaction, nodes []ids.NodeID, t schema.TypeID, f schema.FieldID, v schema.Value) error {
	td, fd, err := a.Descriptor(t, f)
	if err != nil {
		return err
	}
	nv, err := Normalize(fd, v)
	if err != nil {
		return err
	}
	name := Label(fd)
	return a.within(tx, i18n.Tr("editor.history.set", "Set {name}", i18n.Str("name", name)), func(tx *history.Transaction) error {
		for _, n := range nodes {
			before, err := a.Get(n, t, f)
			if err != nil {
				return err
			}
			if Equal(before, nv) {
				continue
			}
			if err := tx.Exec(commands.SetField(a.s, n, td, fd, nv, name)); err != nil {
				return err
			}
		}
		return nil
	})
}

func numeric(k schema.Kind) bool {
	switch k {
	case schema.KindInt32, schema.KindInt64, schema.KindFloat32, schema.KindFloat64, schema.KindEnum:
		return true
	}
	return false
}

func badKind(fd *schema.FieldDescriptor, v schema.Value) error {
	return errs.New(errs.SchemaBadValue, "value kind does not match the field",
		log.F("field", fd.Name), log.F("got", v.Kind.String()), log.F("want", fd.Kind.String()))
}

func clampFloat(fd *schema.FieldDescriptor, f float64) float64 {
	if fd.HasMin && f < fd.Min {
		f = fd.Min
	}
	if fd.HasMax && f > fd.Max {
		f = fd.Max
	}
	return f
}

func Normalize(fd *schema.FieldDescriptor, v schema.Value) (schema.Value, error) {
	if fd.Flags.Has(schema.FlagReadOnly) {
		return v, errs.New(errs.SceneReadOnly, "field is read-only", log.F("field", fd.Name))
	}
	switch fd.Kind {
	case schema.KindBool:
		if v.Kind != schema.KindBool {
			return v, badKind(fd, v)
		}
		return v, nil
	case schema.KindInt32, schema.KindInt64:
		if !numeric(v.Kind) {
			return v, badKind(fd, v)
		}
		n := int64(math.Round(clampFloat(fd, v.AsFloat())))
		if fd.Kind == schema.KindInt32 {
			n = max(min(n, math.MaxInt32), math.MinInt32)
			return schema.Int32(int32(n)), nil
		}
		return schema.Int64(n), nil
	case schema.KindFloat32, schema.KindFloat64:
		if !numeric(v.Kind) {
			return v, badKind(fd, v)
		}
		f := v.AsFloat()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return v, errs.New(errs.SchemaBadValue, "value is not a finite number", log.F("field", fd.Name))
		}
		f = clampFloat(fd, f)
		if fd.Kind == schema.KindFloat32 {
			return schema.Float32(float32(f)), nil
		}
		return schema.Float64(f), nil
	case schema.KindEnum:
		if !numeric(v.Kind) {
			return v, badKind(fd, v)
		}
		n := v.AsInt()
		if len(fd.Enum) > 0 {
			if _, ok := fd.EnumNameOf(n); !ok {
				return v, errs.New(errs.SchemaBadValue, "value is not a member of the enum", log.F("field", fd.Name))
			}
		}
		return schema.Enum(n), nil
	}
	if v.Kind != fd.Kind {
		return v, badKind(fd, v)
	}
	return v, nil
}

func Equal(a, b schema.Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case schema.KindBool, schema.KindInt32, schema.KindInt64, schema.KindEnum:
		return a.I == b.I
	case schema.KindFloat32, schema.KindFloat64:
		return a.F == b.F
	case schema.KindVec2:
		return a.V == b.V
	case schema.KindVec3:
		return a.V3 == b.V3
	case schema.KindVec4:
		return a.V4 == b.V4
	case schema.KindQuat:
		return a.Q == b.Q
	case schema.KindColor:
		return a.C == b.C
	case schema.KindRect:
		return a.R == b.R
	case schema.KindCurve:
		return a.Cv.Equal(b.Cv)
	case schema.KindString:
		return a.S == b.S
	case schema.KindAssetRef:
		return a.Asset == b.Asset
	case schema.KindNodeRef:
		return a.Node == b.Node
	case schema.KindList:
		if len(a.List) != len(b.List) {
			return false
		}
		for i := range a.List {
			if !Equal(a.List[i], b.List[i]) {
				return false
			}
		}
		return true
	case schema.KindStruct:
		if a.Obj == nil || b.Obj == nil {
			return a.Obj == b.Obj
		}
		if a.Obj.Type != b.Obj.Type || len(a.Obj.Fields) != len(b.Obj.Fields) {
			return false
		}
		for i := range a.Obj.Fields {
			if !Equal(a.Obj.Fields[i], b.Obj.Fields[i]) {
				return false
			}
		}
		return true
	}
	return false
}
