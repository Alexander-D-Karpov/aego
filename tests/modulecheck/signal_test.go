package modulecheck

import (
	"testing"

	"aego/app/assembly"
	"aego/engine/base"
	"aego/engine/module"
	"aego/modules/scripting"
	"aego/script/abi"
	"aego/script/bind"
	"aego/script/bind/manifest"
	"aego/script/stdlib"
	"aego/tests/modulecheck/fakephys"
)

func TestComponentSignalReachesScripts(t *testing.T) {
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	reg := bind.New()
	cat := assembly.Catalog{
		{ID: base.CoreID, New: func(assembly.Options) module.Module { return base.Core{} }},
		{ID: scripting.ID, New: func(assembly.Options) module.Module { return scripting.New(stdlib.Options{}) }},
		{ID: fakephys.ID, New: func(assembly.Options) module.Module { return &fakephys.Module{} }},
	}
	if _, err := cat.Load(types, reg, assembly.Options{}); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.FromRegistry(reg, types.Schema())
	if err != nil {
		t.Fatal(err)
	}
	var found *manifest.CompSignal
	for i := range m.CompSignals {
		if m.CompSignals[i].Comp == "fakephys.Body" && m.CompSignals[i].Name == "Hit" {
			found = &m.CompSignals[i]
		}
	}
	if found == nil || len(found.Params) != 1 || found.Params[0].Kind != abi.Float || len(found.Names) != 1 || found.Doc == "" {
		t.Fatalf("Hit signal %+v", found)
	}
	back, err := manifest.Decode(manifest.Encode(m))
	if err != nil {
		t.Fatal(err)
	}
	tab, err := back.Table()
	if err != nil {
		t.Fatal(err)
	}
	comps := tab.Components("fakephys.Body")
	if len(comps) != 1 {
		t.Fatal("fakephys.Body missing after the round trip")
	}
	if _, ok := comps[0].Signal("Hit"); !ok || tab.Hash() != reg.ABIHash() {
		t.Fatal("component signal lost or the ABI hash changed in the round trip")
	}
	if _, def, ok := tab.HandleSignal("aego/tween.Tween", "Completed"); ok && def.Params[0].Kind != abi.Bool {
		t.Fatal("Completed carries a bool")
	}
}
