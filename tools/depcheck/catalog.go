package main

import (
	"reflect"
	"strings"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/engine/base"
	"aego/engine/module"
	"aego/script/bind"
)

func builtinCatalog() (*Catalog, error) {
	types, err := assembly.NewTypes()
	if err != nil {
		return nil, err
	}
	cat := builtin.Default()
	man, err := cat.Load(types, bind.New(), assembly.Options{})
	if err != nil {
		return nil, err
	}
	mods, err := cat.Modules(assembly.Options{})
	if err != nil {
		return nil, err
	}
	dirs := map[string]string{}
	for _, m := range mods {
		dirs[m.Info().ID] = strings.TrimPrefix(modulePkg(m), "aego/")
	}
	out := &Catalog{}
	for _, info := range man.Modules {
		if info.ID == base.CoreID {
			continue
		}
		cm := CatalogModule{ID: info.ID, Dir: dirs[info.ID]}
		for _, td := range types.Schema().Components() {
			if td.Owner == info.ID {
				cm.Tokens = append(cm.Tokens, td.FullName)
			}
		}
		for mod, owner := range man.ScriptOwners {
			if owner == info.ID {
				cm.Tokens = append(cm.Tokens, mod)
			}
		}
		out.Modules = append(out.Modules, cm)
	}
	return out, nil
}

func modulePkg(m module.Module) string {
	t := reflect.TypeOf(m)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.PkgPath()
}
