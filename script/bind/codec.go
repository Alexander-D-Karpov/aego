package bind

import (
	"fmt"
	"reflect"
	"unsafe"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/scene"
	"aego/script/abi"
	"aego/script/vm"
)

type codec[T any] struct {
	ref   abi.TypeRef
	slots int
	get   func(m *vm.Machine, w []vm.Value) T
	put   func(m *vm.Machine, w []vm.Value, v T)
}

type integer interface {
	int | int8 | int16 | int32 | int64 | uint8 | uint16 | uint32
}

var (
	tVec2   = reflect.TypeFor[mathx.Vec2]()
	tVec3   = reflect.TypeFor[mathx.Vec3]()
	tColor  = reflect.TypeFor[mathx.Color]()
	tQuat   = reflect.TypeFor[mathx.Quat]()
	tRect   = reflect.TypeFor[mathx.Rect]()
	tT2     = reflect.TypeFor[scene.Transform2D]()
	tT3     = reflect.TypeFor[scene.Transform3D]()
	tNode   = reflect.TypeFor[scene.NodeHandle]()
	tTex    = reflect.TypeFor[asset.Ref[asset.Texture]]()
	tScene  = reflect.TypeFor[asset.Ref[asset.Scene]]()
	tPrefab = reflect.TypeFor[asset.Ref[asset.Prefab]]()
	tMesh   = reflect.TypeFor[asset.Ref[asset.Mesh]]()
)

func cast[T, U any](u U) T {
	return *(*T)(unsafe.Pointer(&u))
}

func Vec2Of(v vm.Value) mathx.Vec2 {
	return mathx.Vec2{X: v.Lane(0), Y: v.Lane(1)}
}

func Vec2Value(p mathx.Vec2) vm.Value {
	return vm.Vec(p.X, p.Y, 0, 0)
}

func Vec3Of(v vm.Value) mathx.Vec3 {
	return mathx.Vec3{X: v.Lane(0), Y: v.Lane(1), Z: v.Lane(2)}
}

func Vec3Value(p mathx.Vec3) vm.Value {
	return vm.Vec(p.X, p.Y, p.Z, 0)
}

func ColorOf(v vm.Value) mathx.Color {
	return mathx.Color{R: v.Lane(0), G: v.Lane(1), B: v.Lane(2), A: v.Lane(3)}
}

func ColorValue(c mathx.Color) vm.Value {
	return vm.Vec(c.R, c.G, c.B, c.A)
}

func QuatOf(v vm.Value) mathx.Quat {
	return mathx.Quat{X: v.Lane(0), Y: v.Lane(1), Z: v.Lane(2), W: v.Lane(3)}
}

func QuatValue(q mathx.Quat) vm.Value {
	return vm.Vec(q.X, q.Y, q.Z, q.W)
}

func RectOf(v vm.Value) mathx.Rect {
	return mathx.Rect{X: v.Lane(0), Y: v.Lane(1), W: v.Lane(2), H: v.Lane(3)}
}

func RectValue(r mathx.Rect) vm.Value {
	return vm.Vec(r.X, r.Y, r.W, r.H)
}

func NodeValue(h scene.NodeHandle) vm.Value {
	return vm.Value{A: uint64(h.Gen)<<32 | uint64(h.Index)}
}

func NodeOf(v vm.Value) scene.NodeHandle {
	return scene.NodeHandle{Index: uint32(v.A), Gen: uint32(v.A >> 32)}
}

func AssetValue(id ids.AssetID) vm.Value {
	return vm.Value{A: id.Hi, B: id.Lo}
}

func AssetOf(v vm.Value) ids.AssetID {
	return ids.AssetID{Hi: v.A, Lo: v.B}
}

func lane[T, U any](c *codec[T], k abi.Kind, of func(vm.Value) U, to func(U) vm.Value) {
	c.ref = abi.T(k)
	c.get = func(_ *vm.Machine, w []vm.Value) T { return cast[T](of(w[0])) }
	c.put = func(_ *vm.Machine, w []vm.Value, v T) { w[0] = to(cast[U](v)) }
}

func transform2[T any](c *codec[T]) {
	c.ref, c.slots = abi.T(abi.Transform2D), 3
	c.get = func(_ *vm.Machine, w []vm.Value) T {
		return cast[T](scene.Transform2D{Position: Vec2Of(w[0]), Rotation: w[1].Float(), Scale: Vec2Of(w[2])})
	}
	c.put = func(_ *vm.Machine, w []vm.Value, v T) {
		t := cast[scene.Transform2D](v)
		w[0], w[1], w[2] = Vec2Value(t.Position), vm.Float(t.Rotation), Vec2Value(t.Scale)
	}
}

func transform3[T any](c *codec[T]) {
	c.ref, c.slots = abi.T(abi.Transform3D), 3
	c.get = func(_ *vm.Machine, w []vm.Value) T {
		return cast[T](scene.Transform3D{Position: Vec3Of(w[0]), Rotation: QuatOf(w[1]), Scale: Vec3Of(w[2])})
	}
	c.put = func(_ *vm.Machine, w []vm.Value, v T) {
		t := cast[scene.Transform3D](v)
		w[0], w[1], w[2] = Vec3Value(t.Position), QuatValue(t.Rotation), Vec3Value(t.Scale)
	}
}

func assetCodec[T any, K asset.Kind](c *codec[T], kind string) {
	c.ref = abi.AssetOf(kind)
	c.get = func(_ *vm.Machine, w []vm.Value) T { return cast[T](asset.NewRef[K](AssetOf(w[0]))) }
	c.put = func(_ *vm.Machine, w []vm.Value, v T) { w[0] = AssetValue(cast[asset.Ref[K]](v).ID) }
}

func intCodec[T any, I integer](c *codec[T]) {
	c.get = func(_ *vm.Machine, w []vm.Value) T { return cast[T](I(w[0].Int())) }
	c.put = func(_ *vm.Machine, w []vm.Value, v T) { w[0] = vm.Int(int64(cast[I](v))) }
}

func codecOf[T any](r *Registry) (codec[T], error) {
	c := codec[T]{slots: 1}
	rt := reflect.TypeFor[T]()
	switch rt {
	case tVec2:
		lane(&c, abi.Vec2, Vec2Of, Vec2Value)
	case tVec3:
		lane(&c, abi.Vec3, Vec3Of, Vec3Value)
	case tColor:
		lane(&c, abi.Color, ColorOf, ColorValue)
	case tQuat:
		lane(&c, abi.Quat, QuatOf, QuatValue)
	case tRect:
		lane(&c, abi.Rect, RectOf, RectValue)
	case tNode:
		lane(&c, abi.NodeRef, NodeOf, NodeValue)
	case tT2:
		transform2(&c)
	case tT3:
		transform3(&c)
	case tTex:
		assetCodec[T, asset.Texture](&c, "texture")
	case tScene:
		assetCodec[T, asset.Scene](&c, "scene")
	case tPrefab:
		assetCodec[T, asset.Prefab](&c, "prefab")
	case tMesh:
		assetCodec[T, asset.Mesh](&c, "mesh")
	default:
		return c, scalarCodec(r, rt, &c)
	}
	return c, nil
}

func scalarCodec[T any](r *Registry, rt reflect.Type, c *codec[T]) error {
	switch rt.Kind() {
	case reflect.Bool:
		c.ref = abi.T(abi.Bool)
		c.get = func(_ *vm.Machine, w []vm.Value) T { return cast[T](w[0].Bool()) }
		c.put = func(_ *vm.Machine, w []vm.Value, v T) { w[0] = vm.Bool(cast[bool](v)) }
		return nil
	case reflect.Float32:
		c.ref = abi.T(abi.Float)
		c.get = func(_ *vm.Machine, w []vm.Value) T { return cast[T](w[0].Float()) }
		c.put = func(_ *vm.Machine, w []vm.Value, v T) { w[0] = vm.Float(cast[float32](v)) }
		return nil
	case reflect.Float64:
		c.ref = abi.T(abi.Float)
		c.get = func(_ *vm.Machine, w []vm.Value) T { return cast[T](float64(w[0].Float())) }
		c.put = func(_ *vm.Machine, w []vm.Value, v T) { w[0] = vm.Float(float32(cast[float64](v))) }
		return nil
	case reflect.String:
		c.ref = abi.T(abi.String)
		c.get = func(m *vm.Machine, w []vm.Value) T {
			s, _ := m.Str(w[0])
			return cast[T](s)
		}
		c.put = func(m *vm.Machine, w []vm.Value, v T) { w[0] = m.NewString(cast[string](v)) }
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		switch name, ok := r.enums[rt]; {
		case rt.PkgPath() == "":
			c.ref = abi.T(abi.Int)
		case ok:
			c.ref = abi.EnumOf(name)
		default:
			return fmt.Errorf("integer type %s is neither a builtin int nor a registered enum", rt)
		}
		switch rt.Kind() {
		case reflect.Int:
			intCodec[T, int](c)
		case reflect.Int8:
			intCodec[T, int8](c)
		case reflect.Int16:
			intCodec[T, int16](c)
		case reflect.Int32:
			intCodec[T, int32](c)
		case reflect.Int64:
			intCodec[T, int64](c)
		case reflect.Uint8:
			intCodec[T, uint8](c)
		case reflect.Uint16:
			intCodec[T, uint16](c)
		default:
			intCodec[T, uint32](c)
		}
		return nil
	}
	return fmt.Errorf("Go type %s cannot cross the script boundary", rt)
}
