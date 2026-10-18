package module

import (
	"errors"
	"reflect"

	"aego/core/errs"
	"aego/core/log"
	"aego/schema"
)

type DataHook struct {
	Refs func(*Registry) error
	Bind func(*Registry) error
}

type liveDecl struct {
	typ    reflect.Type
	fields []string
	owner  string
}

func (r *Registry) AddDataHook(h DataHook) {
	r.dataHooks = append(r.dataHooks, h)
}

func Live[T any](r *Registry, fields ...string) {
	r.live = append(r.live, liveDecl{typ: reflect.TypeFor[T](), fields: fields, owner: r.Owner()})
}

func FinalizeData(r *Registry) error {
	var list []error
	for _, d := range r.live {
		td, ok := r.Schema().ByGoType(d.typ)
		if !ok || td.Kind != schema.TypeAsset {
			list = append(list, errs.New(errs.EngineModule, "Live names "+d.typ.String()+", which is not a registered data type", log.F("module", d.owner)))
			continue
		}
		for _, name := range d.fields {
			fd, ok := td.Field(name)
			if !ok {
				list = append(list, errs.New(errs.EngineModule, "Live names an unknown field "+name+" of "+td.FullName, log.F("module", d.owner)))
				continue
			}
			fd.Flags |= schema.FlagLive
		}
	}
	for _, h := range r.dataHooks {
		if h.Refs != nil {
			list = append(list, h.Refs(r))
		}
	}
	for _, h := range r.dataHooks {
		if h.Bind != nil {
			list = append(list, h.Bind(r))
		}
	}
	return errors.Join(list...)
}
