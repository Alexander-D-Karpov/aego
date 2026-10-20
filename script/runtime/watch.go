package runtime

import (
	"strconv"

	"aego/scene"
	"aego/script/abi"
	"aego/script/bind"
	"aego/script/vm"
)

func lanes(v vm.Value, n int) string {
	b := make([]byte, 0, 48)
	b = append(b, '(')
	for i := range n {
		if i > 0 {
			b = append(b, ", "...)
		}
		b = strconv.AppendFloat(b, float64(v.Lane(i)), 'g', 5, 32)
	}
	return string(append(b, ')'))
}

func (r *Runtime) WatchField(h scene.NodeHandle, script, field string, lane int) (string, float64, bool, bool) {
	for _, iid := range r.byNode[h] {
		in := &r.inst[iid-1]
		if in.state == dead || in.base < 0 || (in.typ.name != script && shortName(in.typ.name) != script) {
			continue
		}
		f := in.typ.def.Field(field)
		if f == nil {
			return "", 0, false, false
		}
		t, _ := abi.Parse(f.Type)
		return r.describe(t, in.typ.vals[in.base+int(f.Slot)], lane)
	}
	return "", 0, false, false
}

func (r *Runtime) describe(t abi.TypeRef, v vm.Value, lane int) (string, float64, bool, bool) {
	switch t.Kind {
	case abi.Bool:
		n := 0.0
		if v.Bool() {
			n = 1
		}
		return strconv.FormatBool(v.Bool()), n, true, true
	case abi.Int, abi.Enum:
		return strconv.FormatInt(v.Int(), 10), float64(v.Int()), true, true
	case abi.Float:
		f := float64(v.Float())
		return strconv.FormatFloat(f, 'g', 5, 64), f, true, true
	case abi.Vec2, abi.Vec3, abi.Color, abi.Quat, abi.Rect:
		n := 4
		switch t.Kind {
		case abi.Vec2:
			n = 2
		case abi.Vec3:
			n = 3
		}
		if lane >= 0 && lane < n {
			f := float64(v.Lane(lane))
			return strconv.FormatFloat(f, 'g', 5, 64), f, true, true
		}
		return lanes(v, n), 0, false, true
	case abi.String:
		s, _ := r.m.Str(v)
		return strconv.Quote(s), 0, false, true
	case abi.NodeRef:
		if v.IsZero() {
			return "nil", 0, false, true
		}
		if h := bind.NodeOf(v); r.scene != nil && r.scene.IsAlive(h) {
			return r.scene.Name(h), 0, false, true
		}
		return "destroyed", 0, false, true
	}
	return t.String(), 0, false, true
}
