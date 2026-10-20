package bind

import (
	"fmt"
	"reflect"

	"aego/script/abi"
	"aego/script/vm"
)

func Handle[T any](m *Module, name string, await bool, of func(uint64) T, key func(T) uint64, opts ...Option) {
	rt := reflect.TypeFor[T]()
	switch {
	case m.method || m.comp != "" || m.handle != "":
		m.valid(name, fmt.Errorf("handle types cannot live in a method module"))
		return
	case of == nil || key == nil:
		m.valid(name, fmt.Errorf("handle type needs its key conversions"))
		return
	}
	if _, dup := m.reg.handles[rt]; dup {
		//lint:ignore ST1005 "Go" is a proper noun
		m.valid(name, fmt.Errorf("Go type %s is already a script handle", rt))
		return
	}
	q := m.path + "." + name
	if !m.valid(name, m.reg.table.AddHandle(abi.HandleDef{Name: q, Await: await})) {
		return
	}
	id := abi.HandleID(q)
	m.reg.handles[rt] = codec[T]{
		ref:   abi.HandleOf(q),
		slots: 1,
		get: func(_ *vm.Machine, w []vm.Value) T {
			if w[0].B != id {
				var zero T
				return zero
			}
			return of(w[0].A)
		},
		put: func(_ *vm.Machine, w []vm.Value, v T) {
			if k := key(v); k != 0 {
				w[0] = vm.Value{A: k, B: id}
				return
			}
			w[0] = vm.Value{}
		},
	}
	m.valid(name, m.reg.describe(m.path, name, opts, -1))
}
