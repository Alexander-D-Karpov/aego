package bind

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"aego/script/abi"
	"aego/script/bytecode"
	"aego/script/vm"
)

type host struct {
	sig abi.Signature
	fn  vm.HostFunc
}

type Registry struct {
	table   *abi.Table
	hosts   map[string]host
	mods    map[string]*Module
	enums   map[reflect.Type]string
	structs map[reflect.Type]*StructDesc
	handles map[reflect.Type]any
	refs    map[reflect.Type]string
	docs    map[string]Info
	errs    []error
	frozen  bool
	locate  bool
}

func New() *Registry {
	return &Registry{
		table:   abi.NewTable(),
		hosts:   map[string]host{},
		mods:    map[string]*Module{},
		enums:   map[reflect.Type]string{},
		structs: map[reflect.Type]*StructDesc{},
		handles: map[reflect.Type]any{},
		refs:    map[reflect.Type]string{},
		docs:    map[string]Info{},
	}
}

func key(module, name string) string {
	return module + "\x00" + name
}

func (r *Registry) Fail(err error) {
	r.fail(err)
}

func (r *Registry) fail(err error) {
	r.errs = append(r.errs, err)
}

func (r *Registry) Module(path string) *Module {
	if m, ok := r.mods[path]; ok {
		return m
	}
	m := &Module{reg: r, path: path}
	r.mods[path] = m
	if strings.HasPrefix(path, "#") {
		r.fail(fmt.Errorf("module %s: paths starting with # are reserved for methods", path))
		return m
	}
	if err := r.table.AddModule(path); err != nil {
		r.fail(fmt.Errorf("module %s: %w", path, err))
	}
	return m
}

func (r *Registry) Methods(recv abi.Kind) *Module {
	path := "#" + recv.String()
	if m, ok := r.mods[path]; ok {
		return m
	}
	m := &Module{reg: r, path: path, recv: recv, method: true}
	r.mods[path] = m
	return m
}

func (r *Registry) ComponentMethods(comp string) *Module {
	path := "#" + comp
	if m, ok := r.mods[path]; ok {
		return m
	}
	m := &Module{reg: r, path: path, comp: comp}
	r.mods[path] = m
	return m
}

func (r *Registry) HandleMethods(handle string) *Module {
	path := "#" + handle
	if m, ok := r.mods[path]; ok {
		return m
	}
	m := &Module{reg: r, path: path, handle: handle}
	r.mods[path] = m
	return m
}

func (r *Registry) Component(c *abi.Component) {
	if err := r.table.AddComponent(c); err != nil {
		r.fail(err)
	}
}

func (r *Registry) Intrinsic(module, name string, sig abi.Signature, fn vm.HostFunc) {
	k := key(module, name)
	switch _, dup := r.hosts[k]; {
	case r.frozen:
		r.fail(fmt.Errorf("intrinsic %s.%s: registry is frozen", module, name))
	case dup:
		r.fail(fmt.Errorf("intrinsic %s.%s already registered", module, name))
	default:
		r.hosts[k] = host{sig: sig, fn: fn}
	}
}

func (r *Registry) Surface() abi.Surface {
	return r.table
}

func (r *Registry) Table() *abi.Table {
	return r.table
}

func (r *Registry) Frozen() bool {
	return r.frozen
}

func (r *Registry) Freeze() error {
	if !r.frozen {
		r.frozen = true
		r.table.Freeze()
	}
	return errors.Join(r.errs...)
}

func (r *Registry) ABIHash() [32]byte {
	return r.table.Hash()
}

func (r *Registry) Resolve(imp bytecode.HostImport) (vm.HostFunc, bool) {
	h, ok := r.hosts[key(imp.Module, imp.Name)]
	if !ok || !matches(h.sig, imp.Sig, strings.HasPrefix(imp.Module, "#")) {
		return nil, false
	}
	return h.fn, true
}

func matches(have, want abi.Signature, method bool) bool {
	if !method {
		return have.Equal(want)
	}
	if len(have.Params) == 0 || len(want.Params) != len(have.Params) || have.Params[0].Kind != want.Params[0].Kind {
		return false
	}
	return abi.Signature{Params: have.Params[1:], Result: have.Result}.Equal(abi.Signature{Params: want.Params[1:], Result: want.Result})
}
