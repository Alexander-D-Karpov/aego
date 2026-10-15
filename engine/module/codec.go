package module

import (
	"errors"
	"unsafe"

	"aego/core/errs"
	"aego/core/mathx"
	"aego/format/aetext"
	"aego/schema"
)

func codecFits(td *schema.TypeDescriptor) error {
	for i := range td.Fields {
		fd := &td.Fields[i]
		if !fd.Flags.Has(schema.FlagSerialize) {
			continue
		}
		switch fd.Kind {
		case schema.KindBool, schema.KindInt32, schema.KindInt64, schema.KindFloat32, schema.KindFloat64,
			schema.KindVec2, schema.KindColor, schema.KindString, schema.KindEnum:
		default:
			return errs.New(errs.EngineModule, "field "+td.FullName+"."+fd.Name+" of kind "+fd.Kind.String()+" needs a custom section codec")
		}
	}
	return nil
}

func encodeFields(td *schema.TypeDescriptor, p unsafe.Pointer, n *aetext.Node) {
	for i := range td.Fields {
		fd := &td.Fields[i]
		if !fd.Flags.Has(schema.FlagSerialize) {
			continue
		}
		var v schema.Value
		fd.Get(p, &v)
		n.Fields = append(n.Fields, aetext.Field{Name: fd.Name, Value: toText(fd, v)})
	}
}

func decodeFields(td *schema.TypeDescriptor, p unsafe.Pointer, n *aetext.Node) error {
	if len(n.Blocks) > 0 {
		return sectionErr(n.Head, "unknown block "+n.Blocks[0].Head)
	}
	for _, f := range n.Fields {
		fd, ok := td.Field(f.Name)
		if !ok || !fd.Flags.Has(schema.FlagSerialize) {
			return sectionErr(n.Head, "unknown setting "+f.Name)
		}
		v, err := fromText(fd, f.Value)
		if err != nil {
			return sectionErr(n.Head, f.Name+": "+err.Error())
		}
		if err := fd.Set(p, v); err != nil {
			return sectionErr(n.Head, f.Name+": "+err.Error())
		}
	}
	return nil
}

func toText(fd *schema.FieldDescriptor, v schema.Value) aetext.Value {
	switch fd.Kind {
	case schema.KindBool:
		return aetext.Bool(v.I != 0)
	case schema.KindInt32, schema.KindInt64:
		return aetext.Int(v.I)
	case schema.KindFloat32:
		return aetext.Float32(float32(v.F))
	case schema.KindFloat64:
		return aetext.Float64(v.F)
	case schema.KindVec2:
		return aetext.List(aetext.Float32(v.V.X), aetext.Float32(v.V.Y))
	case schema.KindColor:
		return aetext.List(aetext.Float32(v.C.R), aetext.Float32(v.C.G), aetext.Float32(v.C.B), aetext.Float32(v.C.A))
	case schema.KindEnum:
		if name, ok := fd.EnumNameOf(v.I); ok {
			return aetext.Ident(name)
		}
		return aetext.Int(v.I)
	}
	return aetext.String(v.S)
}

func numbers(a aetext.Value, n int) ([]float32, error) {
	if a.Kind != aetext.VList || len(a.List) != n {
		return nil, errors.New("expected a list of numbers")
	}
	out := make([]float32, n)
	for i, x := range a.List {
		if x.Kind != aetext.VInt && x.Kind != aetext.VFloat {
			return nil, errors.New("expected a list of numbers")
		}
		out[i] = float32(x.AsFloat())
	}
	return out, nil
}

func fromText(fd *schema.FieldDescriptor, a aetext.Value) (schema.Value, error) {
	num := a.Kind == aetext.VInt || a.Kind == aetext.VFloat
	switch fd.Kind {
	case schema.KindBool:
		if a.Kind != aetext.VBool {
			return schema.Value{}, errors.New("expected true or false")
		}
		return schema.Bool(a.Bool), nil
	case schema.KindInt32, schema.KindInt64:
		if !num {
			return schema.Value{}, errors.New("expected a number")
		}
		if fd.Kind == schema.KindInt32 {
			return schema.Int32(int32(a.AsInt())), nil
		}
		return schema.Int64(a.AsInt()), nil
	case schema.KindFloat32, schema.KindFloat64:
		if !num {
			return schema.Value{}, errors.New("expected a number")
		}
		if fd.Kind == schema.KindFloat32 {
			return schema.Float32(float32(a.AsFloat())), nil
		}
		return schema.Float64(a.AsFloat()), nil
	case schema.KindVec2:
		f, err := numbers(a, 2)
		if err != nil {
			return schema.Value{}, err
		}
		return schema.Vec2(mathx.Vec2{X: f[0], Y: f[1]}), nil
	case schema.KindColor:
		f, err := numbers(a, 4)
		if err != nil {
			return schema.Value{}, err
		}
		return schema.Color(mathx.Color{R: f[0], G: f[1], B: f[2], A: f[3]}), nil
	case schema.KindEnum:
		switch a.Kind {
		case aetext.VIdent:
			if v, ok := fd.EnumValueOf(a.Ident); ok {
				return schema.Enum(v), nil
			}
			return schema.Value{}, errors.New("unknown value " + a.Ident)
		case aetext.VInt:
			return schema.Enum(a.Int), nil
		}
		return schema.Value{}, errors.New("expected an enum name")
	case schema.KindString:
		if a.Kind != aetext.VString {
			return schema.Value{}, errors.New("expected a string")
		}
		return schema.String(a.Str), nil
	}
	return schema.Value{}, errors.New("unsupported kind " + fd.Kind.String())
}
