package bind

import (
	"errors"
	"fmt"
	"reflect"

	"aego/asset"
	"aego/core/mathx"
	"aego/scene"
	"aego/schema"
	"aego/script/abi"
	"aego/script/vm"
)

var (
	tCtx        = reflect.TypeFor[*Ctx]()
	errNoTarget = errors.New("component method called without a scene")
)

type dyn struct {
	ref   abi.TypeRef
	slots int
	get   func(m *vm.Machine, w []vm.Value) reflect.Value
	put   func(m *vm.Machine, w []vm.Value, v reflect.Value)
}

func dynOf[T any](r *Registry) (dyn, error) {
	c, err := codecOf[T](r)
	if err != nil {
		return dyn{}, err
	}
	return dyn{
		ref:   c.ref,
		slots: c.slots,
		get:   func(m *vm.Machine, w []vm.Value) reflect.Value { return reflect.ValueOf(c.get(m, w)) },
		put:   func(m *vm.Machine, w []vm.Value, v reflect.Value) { c.put(m, w, v.Interface().(T)) },
	}, nil
}

func dynFor(r *Registry, rt reflect.Type) (dyn, error) {
	switch rt {
	case tVec2:
		return dynOf[mathx.Vec2](r)
	case tVec3:
		return dynOf[mathx.Vec3](r)
	case tColor:
		return dynOf[mathx.Color](r)
	case tQuat:
		return dynOf[mathx.Quat](r)
	case tRect:
		return dynOf[mathx.Rect](r)
	case tNode:
		return dynOf[scene.NodeHandle](r)
	case tT2:
		return dynOf[scene.Transform2D](r)
	case tT3:
		return dynOf[scene.Transform3D](r)
	case tTex:
		return dynOf[asset.Ref[asset.Texture]](r)
	case tScene:
		return dynOf[asset.Ref[asset.Scene]](r)
	case tPrefab:
		return dynOf[asset.Ref[asset.Prefab]](r)
	case tMesh:
		return dynOf[asset.Ref[asset.Mesh]](r)
	}
	return scalarDyn(r, rt)
}

func scalarDyn(r *Registry, rt reflect.Type) (dyn, error) {
	d := dyn{slots: 1}
	switch rt.Kind() {
	case reflect.Bool:
		d.ref = abi.T(abi.Bool)
		d.get = func(_ *vm.Machine, w []vm.Value) reflect.Value { return reflect.ValueOf(w[0].Bool()).Convert(rt) }
		d.put = func(_ *vm.Machine, w []vm.Value, v reflect.Value) { w[0] = vm.Bool(v.Bool()) }
	case reflect.Float32, reflect.Float64:
		d.ref = abi.T(abi.Float)
		d.get = func(_ *vm.Machine, w []vm.Value) reflect.Value {
			return reflect.ValueOf(float64(w[0].Float())).Convert(rt)
		}
		d.put = func(_ *vm.Machine, w []vm.Value, v reflect.Value) { w[0] = vm.Float(float32(v.Float())) }
	case reflect.String:
		d.ref = abi.T(abi.String)
		d.get = func(m *vm.Machine, w []vm.Value) reflect.Value {
			s, _ := m.Str(w[0])
			return reflect.ValueOf(s).Convert(rt)
		}
		d.put = func(m *vm.Machine, w []vm.Value, v reflect.Value) { w[0] = m.NewString(v.String()) }
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		ref, err := intRef(r, rt)
		if err != nil {
			return d, err
		}
		d.ref = ref
		d.get = func(_ *vm.Machine, w []vm.Value) reflect.Value {
			out := reflect.New(rt).Elem()
			out.SetInt(w[0].Int())
			return out
		}
		d.put = func(_ *vm.Machine, w []vm.Value, v reflect.Value) { w[0] = vm.Int(v.Int()) }
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		ref, err := intRef(r, rt)
		if err != nil {
			return d, err
		}
		d.ref = ref
		d.get = func(_ *vm.Machine, w []vm.Value) reflect.Value {
			out := reflect.New(rt).Elem()
			out.SetUint(uint64(w[0].Int()))
			return out
		}
		d.put = func(_ *vm.Machine, w []vm.Value, v reflect.Value) { w[0] = vm.Int(int64(v.Uint())) }
	default:
		//lint:ignore ST1005 "Go" is a proper noun
		return d, fmt.Errorf("Go type %s cannot cross the script boundary", rt)
	}
	return d, nil
}

func intRef(r *Registry, rt reflect.Type) (abi.TypeRef, error) {
	if rt.PkgPath() == "" {
		return abi.T(abi.Int), nil
	}
	if name, ok := r.enums[rt]; ok {
		return abi.EnumOf(name), nil
	}
	return abi.TypeRef{}, fmt.Errorf("integer type %s is neither a builtin int nor a registered enum", rt)
}

func Method(m *Module, name string, typ schema.TypeID, fn any, opts ...Option) error {
	fv := reflect.ValueOf(fn)
	if !fv.IsValid() || fv.Kind() != reflect.Func {
		return fmt.Errorf("%s is not a function", name)
	}
	ft := fv.Type()
	if ft.IsVariadic() || ft.NumIn() < 2 || ft.In(0) != tCtx || ft.In(1).Kind() != reflect.Pointer || ft.NumOut() > 1 {
		return fmt.Errorf("%s must be func(*bind.Ctx, *Component, args...) with at most one result", name)
	}
	if m.method {
		return fmt.Errorf("%s: component methods cannot live in a method module", name)
	}
	self := ft.In(1).Elem()
	args := make([]dyn, ft.NumIn()-2)
	offs := make([]int, len(args))
	params := []abi.TypeRef{abi.T(abi.NodeRef)}
	off := 1
	for i := range args {
		d, err := dynFor(m.reg, ft.In(i+2))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		args[i], offs[i] = d, off
		off += d.slots
		params = append(params, d.ref)
	}
	result := abi.T(abi.Void)
	var out dyn
	hasOut := ft.NumOut() == 1
	if hasOut {
		d, err := dynFor(m.reg, ft.Out(0))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		out, result = d, d.ref
	}
	miss := name + ": the " + self.Name() + " reference is nil, its node is destroyed or the component was removed"
	before := len(m.reg.errs)
	m.addRecv(name, params, result, func(mc *vm.Machine, w []vm.Value) error {
		x, err := ctxOf(mc)
		if err != nil {
			return err
		}
		if x.Scene == nil {
			return errNoTarget
		}
		h := NodeOf(w[0])
		if !x.Scene.IsAlive(h) {
			return &vm.Fault{Code: vm.CodeNil, Message: miss}
		}
		p, ok := x.Scene.GetByType(h, typ)
		if !ok {
			return &vm.Fault{Code: vm.CodeNil, Message: miss}
		}
		x.Recv = h
		in := make([]reflect.Value, len(args)+2)
		in[0] = reflect.ValueOf(x)
		in[1] = reflect.NewAt(self, p)
		for i := range args {
			in[i+2] = args[i].get(mc, w[offs[i]:])
		}
		res := fv.Call(in)
		if hasOut {
			out.put(mc, w, res[0])
		}
		return x.finish()
	}, opts)
	if len(m.reg.errs) > before {
		err := errors.Join(m.reg.errs[before:]...)
		m.reg.errs = m.reg.errs[:before]
		return err
	}
	return nil
}
