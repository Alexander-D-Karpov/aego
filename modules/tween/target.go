package tween

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"aego/contracts/scriptfields"
	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/scene"
	"aego/schema"
	"aego/script/abi"
	"aego/script/vm"
)

const (
	ownerTransform = "transform"
	scriptPrefix   = "script/"
)

type owner uint8

const (
	ownComponent owner = iota
	ownTransform
	ownScript
)

type shape uint8

const (
	shFloat shape = iota
	shInt
	shVec2
	shVec3
	shVec4
	shQuat
)

type target struct {
	own   owner
	typ   schema.TypeID
	off   uintptr
	kind  schema.Kind
	part  uint8
	three bool
	ref   scriptfields.Ref
	shape shape
	lane  int8
}

func (t *target) lanes() int {
	if t.lane >= 0 {
		return 1
	}
	switch t.shape {
	case shVec2:
		return 2
	case shVec3:
		return 3
	case shVec4, shQuat:
		return 4
	}
	return 1
}

func (t *target) same(o *target) bool {
	return t.own == o.own && t.typ == o.typ && t.off == o.off && t.part == o.part && t.lane == o.lane &&
		t.ref.Instance == o.ref.Instance && t.ref.Field == o.ref.Field
}

func fits(t *target, want shape) bool {
	if want == shFloat {
		return t.lanes() == 1
	}
	return t.lane < 0 && t.shape == want
}

func schemaShape(k schema.Kind) (shape, bool) {
	switch k {
	case schema.KindFloat32, schema.KindFloat64:
		return shFloat, true
	case schema.KindInt32, schema.KindInt64:
		return shInt, true
	case schema.KindVec2:
		return shVec2, true
	case schema.KindVec3:
		return shVec3, true
	case schema.KindVec4, schema.KindColor, schema.KindRect:
		return shVec4, true
	case schema.KindQuat:
		return shQuat, true
	}
	return 0, false
}

func abiShape(k abi.Kind) (shape, bool) {
	switch k {
	case abi.Float:
		return shFloat, true
	case abi.Int:
		return shInt, true
	case abi.Vec2:
		return shVec2, true
	case abi.Vec3:
		return shVec3, true
	case abi.Color, abi.Rect:
		return shVec4, true
	case abi.Quat:
		return shQuat, true
	}
	return 0, false
}

func laneOf(s string) int8 {
	if len(s) != 1 {
		return -1
	}
	switch s[0] {
	case 'x', 'r':
		return 0
	case 'y', 'g':
		return 1
	case 'z', 'b':
		return 2
	case 'w', 'a':
		return 3
	}
	return -1
}

func lowerFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(r)) + s[n:]
}

func badPath(path, msg string) error {
	return errs.New(errs.EngineModule, "tween: "+path+": "+msg, log.F("path", path))
}

func (s *Service) resolve(h scene.NodeHandle, path string) (target, error) {
	t, err := s.resolveAt(h, path, path, -1)
	if err == nil {
		return t, nil
	}
	if i := strings.LastIndexByte(path, '.'); i > 0 {
		if l := laneOf(path[i+1:]); l >= 0 {
			if lt, lerr := s.resolveAt(h, path, path[:i], l); lerr == nil {
				return lt, nil
			}
		}
	}
	return t, err
}

func (s *Service) resolveAt(h scene.NodeHandle, path, p string, lane int8) (target, error) {
	t := target{lane: lane}
	i := strings.LastIndexByte(p, '.')
	if i <= 0 || i == len(p)-1 {
		return t, badPath(path, "needs Owner.Field")
	}
	own, field := p[:i], p[i+1:]
	var err error
	switch {
	case own == ownerTransform:
		err = s.transformTarget(&t, h, field)
	case strings.HasPrefix(own, scriptPrefix):
		err = s.scriptTarget(&t, h, own[len(scriptPrefix):], field)
	default:
		err = s.componentTarget(&t, h, own, field)
	}
	if err != nil {
		return t, badPath(path, err.Error())
	}
	if lane >= 0 {
		full := t
		full.lane = -1
		if int(lane) >= full.lanes() || full.shape == shQuat {
			return t, badPath(path, "the value has no such lane")
		}
	}
	return t, nil
}

func (s *Service) componentTarget(t *target, h scene.NodeHandle, own, field string) error {
	td, ok := s.reg.ByName(own)
	if !ok {
		if id := s.short[own]; id != 0 {
			td, ok = s.reg.ByID(id)
		}
	}
	if !ok || td.Kind != schema.TypeComponent {
		return errs.New(errs.SchemaUnknownType, "unknown or ambiguous component "+own)
	}
	fd, ok := td.Field(field)
	if !ok {
		fd, ok = td.Field(lowerFirst(field))
	}
	if !ok {
		return errs.New(errs.SchemaBadField, td.FullName+" has no field "+field)
	}
	if fd.Flags.Has(schema.FlagReadOnly) {
		return errs.New(errs.SchemaBadField, td.FullName+"."+fd.Name+" is read-only")
	}
	sh, ok := schemaShape(fd.Kind)
	if !ok {
		return errs.New(errs.SchemaBadField, td.FullName+"."+fd.Name+" has kind "+fd.Kind.String()+", which cannot be tweened")
	}
	if _, has := s.scene.GetByType(h, td.ID); !has {
		return errs.New(errs.SceneNoComponent, "the node has no "+td.FullName)
	}
	t.own, t.typ, t.off, t.kind, t.shape = ownComponent, td.ID, fd.Offset, fd.Kind, sh
	return nil
}

func (s *Service) transformTarget(t *target, h scene.NodeHandle, field string) error {
	t.own = ownTransform
	t.three = s.scene.Space(h) == scene.Space3D
	vec := shVec2
	if t.three {
		vec = shVec3
	}
	switch field {
	case "position", "Position":
		t.part, t.shape = 0, vec
	case "rotation", "Rotation":
		t.part, t.shape = 1, shFloat
		if t.three {
			t.shape = shQuat
		}
	case "scale", "Scale":
		t.part, t.shape = 2, vec
	default:
		return errs.New(errs.SchemaBadField, "transform has position, rotation and scale")
	}
	return nil
}

func (s *Service) scriptTarget(t *target, h scene.NodeHandle, name, field string) error {
	f := s.fields()
	if f == nil {
		return errs.New(errs.EngineModule, "scripts are not running in this world")
	}
	ref, ok := f.ResolveField(h, name, field)
	if !ok {
		return errs.New(errs.SchemaBadField, "the node has no script "+name+" with field "+field)
	}
	sh, ok := abiShape(ref.Kind)
	if !ok {
		return errs.New(errs.SchemaBadField, name+"."+field+" has type "+ref.Kind.String()+", which cannot be tweened")
	}
	t.own, t.ref, t.shape = ownScript, ref, sh
	return nil
}

func loadPOD(q unsafe.Pointer, k schema.Kind) [4]float32 {
	var l [4]float32
	switch k {
	case schema.KindFloat32:
		l[0] = *(*float32)(q)
	case schema.KindFloat64:
		l[0] = float32(*(*float64)(q))
	case schema.KindInt32:
		l[0] = float32(*(*int32)(q))
	case schema.KindInt64:
		l[0] = float32(*(*int64)(q))
	case schema.KindVec2:
		v := *(*[2]float32)(q)
		l[0], l[1] = v[0], v[1]
	case schema.KindVec3:
		v := *(*[3]float32)(q)
		l[0], l[1], l[2] = v[0], v[1], v[2]
	default:
		l = *(*[4]float32)(q)
	}
	return l
}

func round(f float32) float64 {
	return math.Round(float64(f))
}

func storePOD(q unsafe.Pointer, k schema.Kind, l [4]float32) {
	switch k {
	case schema.KindFloat32:
		*(*float32)(q) = l[0]
	case schema.KindFloat64:
		*(*float64)(q) = float64(l[0])
	case schema.KindInt32:
		*(*int32)(q) = int32(round(l[0]))
	case schema.KindInt64:
		*(*int64)(q) = int64(round(l[0]))
	case schema.KindVec2:
		*(*[2]float32)(q) = [2]float32{l[0], l[1]}
	case schema.KindVec3:
		*(*[3]float32)(q) = [3]float32{l[0], l[1], l[2]}
	default:
		*(*[4]float32)(q) = l
	}
}

func vmLanes(v vm.Value, sh shape) [4]float32 {
	switch sh {
	case shFloat:
		return [4]float32{v.Float()}
	case shInt:
		return [4]float32{float32(v.Int())}
	}
	return [4]float32{v.Lane(0), v.Lane(1), v.Lane(2), v.Lane(3)}
}

func lanesVM(l [4]float32, sh shape) vm.Value {
	switch sh {
	case shFloat:
		return vm.Float(l[0])
	case shInt:
		return vm.Int(int64(round(l[0])))
	case shVec2:
		return vm.Vec(l[0], l[1], 0, 0)
	case shVec3:
		return vm.Vec(l[0], l[1], l[2], 0)
	}
	return vm.Vec(l[0], l[1], l[2], l[3])
}

func (s *Service) readFull(t *target, h scene.NodeHandle) ([4]float32, bool) {
	var l [4]float32
	switch t.own {
	case ownComponent:
		p, ok := s.scene.GetByType(h, t.typ)
		if !ok {
			return l, false
		}
		return loadPOD(unsafe.Add(p, t.off), t.kind), true
	case ownTransform:
		if (s.scene.Space(h) == scene.Space3D) != t.three {
			return l, false
		}
		if t.three {
			tr := s.scene.Local3D(h)
			switch t.part {
			case 0:
				l = [4]float32{tr.Position.X, tr.Position.Y, tr.Position.Z}
			case 1:
				q := tr.Rotation
				l = [4]float32{q.X, q.Y, q.Z, q.W}
			default:
				l = [4]float32{tr.Scale.X, tr.Scale.Y, tr.Scale.Z}
			}
			return l, true
		}
		tr := s.scene.Local(h)
		switch t.part {
		case 0:
			l = [4]float32{tr.Position.X, tr.Position.Y}
		case 1:
			l = [4]float32{tr.Rotation}
		default:
			l = [4]float32{tr.Scale.X, tr.Scale.Y}
		}
		return l, true
	}
	f := s.fields()
	if f == nil {
		return l, false
	}
	v, ok := f.LoadField(&t.ref)
	if !ok {
		return l, false
	}
	return vmLanes(v, t.shape), true
}

func (s *Service) writeFull(t *target, h scene.NodeHandle, l [4]float32) bool {
	switch t.own {
	case ownComponent:
		p, ok := s.scene.GetByType(h, t.typ)
		if !ok {
			return false
		}
		storePOD(unsafe.Add(p, t.off), t.kind, l)
		return true
	case ownTransform:
		if (s.scene.Space(h) == scene.Space3D) != t.three {
			return false
		}
		if t.three {
			tr := s.scene.Local3D(h)
			switch t.part {
			case 0:
				tr.Position = mathx.Vec3{X: l[0], Y: l[1], Z: l[2]}
			case 1:
				tr.Rotation = mathx.Quat{X: l[0], Y: l[1], Z: l[2], W: l[3]}
			default:
				tr.Scale = mathx.Vec3{X: l[0], Y: l[1], Z: l[2]}
			}
			s.scene.SetLocal3D(h, tr)
			return true
		}
		switch t.part {
		case 0:
			s.scene.SetPosition(h, mathx.Vec2{X: l[0], Y: l[1]})
		case 1:
			s.scene.SetRotation(h, l[0])
		default:
			s.scene.SetScale(h, mathx.Vec2{X: l[0], Y: l[1]})
		}
		return true
	}
	f := s.fields()
	return f != nil && f.StoreField(&t.ref, lanesVM(l, t.shape))
}

func (s *Service) read(t *target, h scene.NodeHandle) ([4]float32, bool) {
	l, ok := s.readFull(t, h)
	if ok && t.lane >= 0 {
		l = [4]float32{l[t.lane]}
	}
	return l, ok
}

func (s *Service) write(t *target, h scene.NodeHandle, l [4]float32) bool {
	if t.lane >= 0 {
		full, ok := s.readFull(t, h)
		if !ok {
			return false
		}
		full[t.lane] = l[0]
		l = full
	}
	return s.writeFull(t, h, l)
}
