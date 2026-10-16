package scripting

import (
	"errors"
	"fmt"
	"reflect"

	"aego/core/errs"
	"aego/core/log"
	"aego/engine/module"
	"aego/schema"
	"aego/script/bind"
)

func bindMethods(r *module.Registry) error {
	if r.Bindings() == nil {
		return nil
	}
	var all []error
	for _, td := range r.Components() {
		for _, sm := range td.ScriptMethods {
			if err := bindMethod(r, td, sm); err != nil {
				all = append(all, errs.New(errs.EngineModule,
					"script method "+td.FullName+"."+sm.Name+" of module "+td.Owner+": "+err.Error(),
					log.F("module", td.Owner)))
			}
		}
	}
	return errors.Join(all...)
}

func bindMethod(r *module.Registry, td *schema.TypeDescriptor, sm schema.ScriptMethod) error {
	mod := r.Bindings().ComponentMethods(td.FullName)
	if adapt, ok := sm.Bind.(func(*bind.Module, schema.TypeID)); ok {
		return bind.Guard(mod, func() { adapt(mod, td.ID) })
	}
	if !reflectFallback {
		return errors.New("needs a typed adapter (Method0..2 or Proc0..2) in a runtime build")
	}
	opts := make([]bind.Option, 0, len(sm.Opts))
	for i, o := range sm.Opts {
		bo, ok := o.(bind.Option)
		if !ok {
			return fmt.Errorf("option %d is %T, not a bind.Option", i, o)
		}
		opts = append(opts, bo)
	}
	if ft := reflect.TypeOf(sm.Fn); ft != nil && ft.Kind() == reflect.Func && ft.NumIn() >= 2 && ft.In(1).Kind() == reflect.Pointer {
		if got, ok := r.Schema().ByGoType(ft.In(1).Elem()); !ok || got != td {
			return fmt.Errorf("receiver %s is not %s", ft.In(1), td.FullName)
		}
	}
	r.Warn(td.Owner, "script method "+td.FullName+"."+sm.Name+" uses the reflection fallback; declare it with Method0..2 or Proc0..2 for zero allocations")
	return bind.Method(mod, sm.Name, td.ID, sm.Fn, opts...)
}
