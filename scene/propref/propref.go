package propref

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"aego/contracts/scriptfields"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/core/rewind"
	"aego/scene"
	"aego/schema"
	"aego/script/abi"
	"aego/script/vm"
)

const (
	OwnerTransform = "transform"
	ScriptPrefix   = "script/"
)

type Owner uint8

const (
	OwnComponent Owner = iota
	OwnTransform
	OwnScript
)

type Shape uint8

const (
	Float Shape = iota
	Int
	Vec2
	Vec3
	Vec4
	Quat
	Discrete
)

type Part uint8

const (
	PartPosition Part = iota
	PartRotation
	PartScale
)

type Ref struct {
	own   Owner
	typ   schema.TypeID
	off   uintptr
	kind  schema.Kind
	part  uint8
	three bool
	field scriptfields.Ref
	fd    *schema.FieldDescriptor
	shape Shape
	lane  int8
}

func (r *Ref) Owner() Owner        { return r.own }
func (r *Ref) Shape() Shape        { return r.shape }
func (r *Ref) Lane() int           { return int(r.lane) }
func (r *Ref) Type() schema.TypeID { return r.typ }
func (r *Ref) Kind() schema.Kind   { return r.kind }
func (r *Ref) Part() Part          { return Part(r.part) }
func (r *Ref) Three() bool         { return r.three }

func (r *Ref) Lanes() int {
	if r.lane >= 0 {
		return 1
	}
	switch r.shape {
	case Vec2:
		return 2
	case Vec3:
		return 3
	case Vec4, Quat:
		return 4
	}
	return 1
}

func (r *Ref) Fits(want Shape) bool {
	switch {
	case r.shape == Discrete || want == Discrete:
		return false
	case want == Float:
		return r.Lanes() == 1
	}
	return r.lane < 0 && r.shape == want
}

func (r Ref) WithLane(l int) Ref {
	r.lane = int8(l)
	return r
}

func Transform(three bool, p Part) Ref {
	t := Ref{own: OwnTransform, part: uint8(p), three: three, lane: -1}
	switch {
	case p == PartRotation && three:
		t.shape = Quat
	case p == PartRotation:
		t.shape = Float
	case three:
		t.shape = Vec3
	default:
		t.shape = Vec2
	}
	return t
}

func TransformRef(s *scene.Scene, h scene.NodeHandle, p Part) Ref {
	return Transform(s.Space(h) == scene.Space3D, p)
}

func (r *Ref) sameProperty(o *Ref) bool {
	return r.own == o.own && r.typ == o.typ && r.off == o.off && r.part == o.part &&
		r.field.Instance == o.field.Instance && r.field.Field == o.field.Field
}

func (r *Ref) Same(o *Ref) bool {
	return r.sameProperty(o) && r.lane == o.lane
}

func (r *Ref) Overlaps(o *Ref) bool {
	return r.sameProperty(o) && (r.lane < 0 || o.lane < 0 || r.lane == o.lane)
}

const (
	fnvOffset = 14695981039346656037
	fnvPrime  = 1099511628211
)

func mix(h, v uint64) uint64 {
	for range 8 {
		h ^= v & 0xff
		h *= fnvPrime
		v >>= 8
	}
	return h
}

// Hash keys the property without its lane, so overlapping lanes share a key; Overlaps tells them apart.
func (r *Ref) Hash() uint64 {
	h := uint64(fnvOffset)
	h = mix(h, uint64(r.own)|uint64(r.part)<<8)
	h = mix(h, uint64(r.typ))
	h = mix(h, uint64(r.off))
	h = mix(h, uint64(r.field.Instance))
	for i := 0; i < len(r.field.Field); i++ {
		h ^= uint64(r.field.Field[i])
		h *= fnvPrime
	}
	return h
}

func (r *Ref) HashTo(h *rewind.Hasher) {
	h.U64(uint64(r.own))
	h.U64(uint64(r.typ))
	h.U64(uint64(r.off))
	h.U64(uint64(r.kind))
	h.U64(uint64(r.part))
	h.Bool(r.three)
	h.U64(uint64(r.shape))
	h.I64(int64(r.lane))
	h.U32(uint32(r.field.Instance))
	h.String(r.field.Field)
	h.Int(r.field.Slot)
}

type PathError struct {
	Path   string
	Reason string
}

func (e *PathError) Error() string {
	return e.Path + ": " + e.Reason
}

type Resolver struct {
	scene  *scene.Scene
	reg    *schema.Registry
	short  map[string]schema.TypeID
	fields func() scriptfields.Fields
	tmp    schema.Value
}

// New takes the script fields as a getter: the scripting service may attach to the world after the caller.
func New(s *scene.Scene, fields func() scriptfields.Fields) *Resolver {
	r := &Resolver{scene: s, reg: s.Registry(), short: map[string]schema.TypeID{}, fields: fields}
	for _, td := range r.reg.Components() {
		n := schema.ShortName(td.FullName)
		if _, done := r.short[n]; done {
			continue
		}
		r.short[n] = 0
		if c, ok := r.reg.ComponentNamed(n); ok {
			r.short[n] = c.ID
		}
	}
	return r
}

func (r *Resolver) access() scriptfields.Fields {
	if r.fields == nil {
		return nil
	}
	return r.fields()
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
	c, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(c)) + s[n:]
}

func (r *Resolver) Resolve(h scene.NodeHandle, path string) (Ref, error) {
	return r.resolve(h, path, false)
}

func (r *Resolver) ResolveAny(h scene.NodeHandle, path string) (Ref, error) {
	return r.resolve(h, path, true)
}

func (r *Resolver) resolve(h scene.NodeHandle, path string, discrete bool) (Ref, error) {
	t, reason := r.at(h, path, -1, discrete)
	if reason == "" {
		return t, nil
	}
	if i := strings.LastIndexByte(path, '.'); i > 0 {
		if l := laneOf(path[i+1:]); l >= 0 {
			if lt, lr := r.at(h, path[:i], l, discrete); lr == "" {
				return lt, nil
			}
		}
	}
	return t, &PathError{Path: path, Reason: reason}
}

func (r *Resolver) at(h scene.NodeHandle, p string, lane int8, discrete bool) (Ref, string) {
	t := Ref{lane: lane}
	i := strings.LastIndexByte(p, '.')
	if i <= 0 || i == len(p)-1 {
		return t, "needs Owner.Field"
	}
	own, field := p[:i], p[i+1:]
	var err error
	switch {
	case own == OwnerTransform:
		err = r.transform(&t, h, field)
	case strings.HasPrefix(own, ScriptPrefix):
		err = r.script(&t, h, own[len(ScriptPrefix):], field)
	default:
		err = r.component(&t, h, own, field, discrete)
	}
	if err != nil {
		return t, err.Error()
	}
	if lane >= 0 {
		full := t
		full.lane = -1
		if int(lane) >= full.Lanes() || full.shape == Quat || full.shape == Discrete {
			return t, "the value has no such lane"
		}
	}
	return t, ""
}

func discreteKind(k schema.Kind) bool {
	return k == schema.KindBool || k == schema.KindEnum || k == schema.KindAssetRef
}

func schemaShape(k schema.Kind) (Shape, bool) {
	switch k {
	case schema.KindFloat32, schema.KindFloat64:
		return Float, true
	case schema.KindInt32, schema.KindInt64:
		return Int, true
	case schema.KindVec2:
		return Vec2, true
	case schema.KindVec3:
		return Vec3, true
	case schema.KindVec4, schema.KindColor, schema.KindRect:
		return Vec4, true
	case schema.KindQuat:
		return Quat, true
	}
	return 0, false
}

func abiShape(k abi.Kind) (Shape, bool) {
	switch k {
	case abi.Float:
		return Float, true
	case abi.Int:
		return Int, true
	case abi.Vec2:
		return Vec2, true
	case abi.Vec3:
		return Vec3, true
	case abi.Color, abi.Rect:
		return Vec4, true
	case abi.Quat:
		return Quat, true
	}
	return 0, false
}

func (r *Resolver) component(t *Ref, h scene.NodeHandle, own, field string, discrete bool) error {
	td, ok := r.reg.ByName(own)
	if !ok {
		if id := r.short[own]; id != 0 {
			td, ok = r.reg.ByID(id)
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
	if !ok && discrete && discreteKind(fd.Kind) {
		sh, ok = Discrete, true
	}
	if !ok {
		return errs.New(errs.SchemaBadField, td.FullName+"."+fd.Name+" has kind "+fd.Kind.String()+", which cannot be tweened")
	}
	if _, has := r.scene.GetByType(h, td.ID); !has {
		return errs.New(errs.SceneNoComponent, "the node has no "+td.FullName)
	}
	t.own, t.typ, t.off, t.kind, t.shape, t.fd = OwnComponent, td.ID, fd.Offset, fd.Kind, sh, fd
	return nil
}

func (r *Resolver) transform(t *Ref, h scene.NodeHandle, field string) error {
	var p Part
	switch field {
	case "position", "Position":
		p = PartPosition
	case "rotation", "Rotation":
		p = PartRotation
	case "scale", "Scale":
		p = PartScale
	default:
		t.own = OwnTransform
		return errs.New(errs.SchemaBadField, "transform has position, rotation and scale")
	}
	lane := t.lane
	*t = TransformRef(r.scene, h, p)
	t.lane = lane
	return nil
}

func (r *Resolver) script(t *Ref, h scene.NodeHandle, name, field string) error {
	f := r.access()
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
	t.own, t.field, t.shape = OwnScript, ref, sh
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

func vmLanes(v vm.Value, sh Shape) [4]float32 {
	switch sh {
	case Float:
		return [4]float32{v.Float()}
	case Int:
		return [4]float32{float32(v.Int())}
	}
	return [4]float32{v.Lane(0), v.Lane(1), v.Lane(2), v.Lane(3)}
}

func lanesVM(l [4]float32, sh Shape) vm.Value {
	switch sh {
	case Float:
		return vm.Float(l[0])
	case Int:
		return vm.Int(int64(round(l[0])))
	case Vec2:
		return vm.Vec(l[0], l[1], 0, 0)
	case Vec3:
		return vm.Vec(l[0], l[1], l[2], 0)
	}
	return vm.Vec(l[0], l[1], l[2], l[3])
}

func (r *Resolver) readFull(t *Ref, h scene.NodeHandle) ([4]float32, bool) {
	var l [4]float32
	s := r.scene
	switch t.own {
	case OwnComponent:
		p, ok := s.GetByType(h, t.typ)
		if !ok {
			return l, false
		}
		return loadPOD(unsafe.Add(p, t.off), t.kind), true
	case OwnTransform:
		if (s.Space(h) == scene.Space3D) != t.three {
			return l, false
		}
		if t.three {
			tr := s.Local3D(h)
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
		tr := s.Local(h)
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
	f := r.access()
	if f == nil {
		return l, false
	}
	v, ok := f.LoadField(&t.field)
	if !ok {
		return l, false
	}
	return vmLanes(v, t.shape), true
}

func (r *Resolver) writeFull(t *Ref, h scene.NodeHandle, l [4]float32) bool {
	s := r.scene
	switch t.own {
	case OwnComponent:
		p, ok := s.GetByType(h, t.typ)
		if !ok {
			return false
		}
		storePOD(unsafe.Add(p, t.off), t.kind, l)
		return true
	case OwnTransform:
		if (s.Space(h) == scene.Space3D) != t.three {
			return false
		}
		if t.three {
			tr := s.Local3D(h)
			switch t.part {
			case 0:
				tr.Position = mathx.Vec3{X: l[0], Y: l[1], Z: l[2]}
			case 1:
				tr.Rotation = mathx.Quat{X: l[0], Y: l[1], Z: l[2], W: l[3]}
			default:
				tr.Scale = mathx.Vec3{X: l[0], Y: l[1], Z: l[2]}
			}
			s.SetLocal3D(h, tr)
			return true
		}
		switch t.part {
		case 0:
			s.SetPosition(h, mathx.Vec2{X: l[0], Y: l[1]})
		case 1:
			s.SetRotation(h, l[0])
		default:
			s.SetScale(h, mathx.Vec2{X: l[0], Y: l[1]})
		}
		return true
	}
	f := r.access()
	return f != nil && f.StoreField(&t.field, lanesVM(l, t.shape))
}

func (r *Resolver) Read(t *Ref, h scene.NodeHandle) ([4]float32, bool) {
	if t.shape == Discrete {
		return [4]float32{}, false
	}
	l, ok := r.readFull(t, h)
	if ok && t.lane >= 0 {
		l = [4]float32{l[t.lane]}
	}
	return l, ok
}

func (r *Resolver) Write(t *Ref, h scene.NodeHandle, l [4]float32) bool {
	if t.shape == Discrete {
		return false
	}
	if t.lane >= 0 {
		full, ok := r.readFull(t, h)
		if !ok {
			return false
		}
		full[t.lane] = l[0]
		l = full
	}
	return r.writeFull(t, h, l)
}

func (r *Resolver) ReadDiscrete(t *Ref, h scene.NodeHandle) (int64, ids.AssetID, bool) {
	if t.shape != Discrete || t.fd == nil {
		return 0, ids.AssetID{}, false
	}
	p, ok := r.scene.GetByType(h, t.typ)
	if !ok {
		return 0, ids.AssetID{}, false
	}
	t.fd.Get(p, &r.tmp)
	return r.tmp.I, r.tmp.Asset, true
}

func (r *Resolver) WriteDiscrete(t *Ref, h scene.NodeHandle, i int64, a ids.AssetID) bool {
	if t.shape != Discrete || t.fd == nil {
		return false
	}
	p, ok := r.scene.GetByType(h, t.typ)
	if !ok {
		return false
	}
	var v schema.Value
	switch t.kind {
	case schema.KindBool:
		v = schema.Bool(i != 0)
	case schema.KindEnum:
		v = schema.Enum(i)
	default:
		v = schema.Asset(a)
	}
	return t.fd.Set(p, v) == nil
}
