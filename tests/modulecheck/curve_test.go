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

func TestCurveFieldReachesScripts(t *testing.T) {
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
	c, ok := m.Component("fakephys.Body")
	if !ok {
		t.Fatal("fakephys.Body is not bound")
	}
	f, ok := c.Field("Falloff")
	if !ok || f.Type.Kind != abi.Curve || !f.ReadOnly {
		t.Fatalf("Falloff %+v %v", f, ok)
	}
	if _, ok := m.FuncDoc(stdlib.ModCurve, "Eval"); !ok {
		t.Fatal("aego/curve.Eval is missing")
	}
	back, err := manifest.Decode(manifest.Encode(m))
	if err != nil || back.ABIHash != m.ABIHash {
		t.Fatalf("manifest round trip: %v", err)
	}
}
