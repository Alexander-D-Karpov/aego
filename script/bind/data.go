package bind

import (
	"fmt"
	"reflect"
	"strconv"
	"unsafe"

	"aego/core/ids"
	"aego/script/abi"
	"aego/script/vm"
)

const (
	DataModule  = "aego/data"
	DataPrefix  = "data:"
	TablePrefix = "table:"
	refMethods  = "#AssetRef"
)

var (
	idType     = reflect.TypeFor[ids.AssetID]()
	errNilData = &vm.Fault{Code: vm.CodeNil, Message: "data reference is nil"}
)

type DataLoaders[T any] struct {
	Record func(c *Ctx, id ids.AssetID) (*T, error)
	Row    func(c *Ctx, table ids.AssetID, row string) (*T, bool, error)
	Rows   func(c *Ctx, table ids.AssetID) (int, error)
}

func refLayout(rt reflect.Type) bool {
	return rt.Kind() == reflect.Struct && rt.NumField() == 1 && rt.Field(0).Type == idType && rt.Size() == idType.Size()
}

func AssetRefType[T any](r *Registry, kind string) error {
	rt := reflect.TypeFor[T]()
	if !refLayout(rt) {
		return fmt.Errorf("%s must be a struct with a single ids.AssetID field", rt)
	}
	if prev, dup := r.refs[rt]; dup {
		if prev == kind {
			return nil
		}
		return fmt.Errorf("%s is already an asset reference of kind %s", rt, prev)
	}
	r.refs[rt] = kind
	r.handles[rt] = codec[T]{
		ref:   abi.AssetOf(kind),
		slots: 1,
		get:   func(_ *vm.Machine, w []vm.Value) T { return cast[T](AssetOf(w[0])) },
		put:   func(_ *vm.Machine, w []vm.Value, v T) { w[0] = AssetValue(cast[ids.AssetID](v)) },
	}
	return nil
}

func (r *Registry) dataStruct(m *Module, name string, rt reflect.Type, opts []Option) (*StructDesc, []string, error) {
	if rt.Kind() != reflect.Struct {
		return nil, nil, fmt.Errorf("%s is not a struct", rt)
	}
	if _, dup := r.structs[rt]; dup {
		//lint:ignore ST1005 "Go" is a proper noun
		return nil, nil, fmt.Errorf("Go type %s is already a script struct", rt)
	}
	d := &StructDesc{Module: m.path, Name: name}
	def := &abi.StructDef{Module: m.path, Name: name}
	var skipped []string
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		if !sf.IsExported() || sf.Anonymous || sf.Tag.Get("aego") == "-" {
			continue
		}
		ref, ls, n, err := r.leafFor(sf.Type)
		if err != nil {
			skipped = append(skipped, sf.Name)
			continue
		}
		for _, l := range ls {
			l.off += sf.Offset
			l.off2 += sf.Offset
			l.slot += d.Slots
			d.leaves = append(d.leaves, l)
		}
		d.Fields = append(d.Fields, StructField{Name: sf.Name, Type: ref, Slot: d.Slots, Slots: n})
		def.Fields = append(def.Fields, abi.StructField{Name: sf.Name, Type: ref})
		d.Slots += n
	}
	def.Slots = d.Slots
	if err := r.table.AddStruct(def); err != nil {
		return nil, nil, err
	}
	r.structs[rt] = d
	if err := r.describe(m.path, name, opts, -1); err != nil {
		return nil, nil, err
	}
	return d, skipped, nil
}

func hostFault(err error) error {
	if f, ok := err.(*vm.Fault); ok {
		return f
	}
	return &vm.Fault{Code: vm.CodeHost, Message: err.Error()}
}

func DataAccess[T any](r *Registry, name string, ld DataLoaders[T], opts ...Option) ([]string, error) {
	d, skipped, err := r.dataStruct(r.Module(DataModule), name, reflect.TypeFor[T](), opts)
	if err != nil {
		return nil, fmt.Errorf("data type %s: %w", name, err)
	}
	q := d.Qualified()
	res := abi.StructOf(q)
	rec, tab := abi.AssetOf(DataPrefix+name), abi.AssetOf(TablePrefix+name)
	str := abi.T(abi.String)
	ls := d.leaves
	r.Intrinsic(refMethods, "Copy<"+q+">", abi.Sig(res, rec), func(mc *vm.Machine, w []vm.Value) error {
		x, err := ctxOf(mc)
		if err != nil {
			return err
		}
		id := AssetOf(w[0])
		if id.IsZero() {
			return errNilData
		}
		p, err := ld.Record(x, id)
		if err != nil {
			return hostFault(err)
		}
		encodeLeaves(unsafe.Pointer(p), ls, w)
		return x.finish()
	})
	r.Intrinsic(refMethods, "Row<"+q+">", abi.Sig(res, tab, str), func(mc *vm.Machine, w []vm.Value) error {
		x, err := ctxOf(mc)
		if err != nil {
			return err
		}
		id := AssetOf(w[0])
		key, _ := mc.Str(w[1])
		if id.IsZero() {
			return errNilData
		}
		p, ok, err := ld.Row(x, id, key)
		switch {
		case err != nil:
			return hostFault(err)
		case !ok:
			return &vm.Fault{Code: vm.CodeNil, Message: "table has no row " + strconv.Quote(key)}
		}
		encodeLeaves(unsafe.Pointer(p), ls, w)
		return x.finish()
	})
	r.Intrinsic(refMethods, "Has<"+q+">", abi.Sig(abi.T(abi.Bool), tab, str), func(mc *vm.Machine, w []vm.Value) error {
		x, err := ctxOf(mc)
		if err != nil {
			return err
		}
		id := AssetOf(w[0])
		key, _ := mc.Str(w[1])
		if id.IsZero() {
			return errNilData
		}
		_, ok, err := ld.Row(x, id, key)
		if err != nil {
			return hostFault(err)
		}
		w[0] = vm.Bool(ok)
		return x.finish()
	})
	r.Intrinsic(refMethods, "Len<"+q+">", abi.Sig(abi.T(abi.Int), tab), func(mc *vm.Machine, w []vm.Value) error {
		x, err := ctxOf(mc)
		if err != nil {
			return err
		}
		id := AssetOf(w[0])
		if id.IsZero() {
			return errNilData
		}
		n, err := ld.Rows(x, id)
		if err != nil {
			return hostFault(err)
		}
		w[0] = vm.Int(int64(n))
		return x.finish()
	})
	return skipped, nil
}
